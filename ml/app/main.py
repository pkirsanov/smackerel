"""Smackerel ML Sidecar — FastAPI application."""

import logging
import os
import sys
from contextlib import asynccontextmanager

from fastapi import APIRouter, Depends, FastAPI
from fastapi.responses import PlainTextResponse
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from .auth import verify_auth
from .embedder import _model
from .nats_client import NATSClient
from .nats_contract import validate_runtime_streams_on_startup

logger = logging.getLogger("smackerel-ml")


def _check_required_config() -> dict[str, str]:
    """Validate required environment variables. Fail loudly if missing."""
    keys = [
        "NATS_URL",
        "LLM_PROVIDER",
        "LLM_MODEL",
        "OLLAMA_URL",
        "ML_PROCESSING_DEGRADED_FALLBACK_ENABLED",
        "SMACKEREL_ENV",
        # Spec 050 FR-050-001/002/003 — ML sidecar health/worker isolation
        # contract. All three values are SST-owned and required (fail-loud).
        # The sidecar refuses to start if any is missing, empty, or invalid.
        "ML_EMBEDDING_WORKERS",
        "ML_EMBEDDING_QUEUE_MAX",
        "ML_HEALTH_LATENCY_SLA_MS",
    ]
    required: dict[str, str] = {}
    missing: list[str] = []
    for k in keys:
        try:
            val = os.environ[k]
        except KeyError:
            missing.append(k)
            continue
        if not val:
            missing.append(k)
        else:
            required[k] = val

    llm_provider = required.get("LLM_PROVIDER", "").lower()
    if llm_provider != "ollama":
        val = os.environ.get("LLM_API_KEY")
        if not val:
            missing.append("LLM_API_KEY")
        else:
            required["LLM_API_KEY"] = val

    if missing:
        logger.error("Missing required configuration: %s", ", ".join(missing))
        sys.exit(1)

    fallback_enabled = required["ML_PROCESSING_DEGRADED_FALLBACK_ENABLED"].lower()
    if fallback_enabled not in ("true", "false"):
        logger.error(
            "Invalid ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=%r; expected true or false",
            required["ML_PROCESSING_DEGRADED_FALLBACK_ENABLED"],
        )
        sys.exit(1)

    # Spec 050 FR-050-002 — embedding worker pool size MUST be a positive
    # integer. Zero or negative values would deadlock the executor; non-
    # integer values are an SST contract violation.
    try:
        workers = int(required["ML_EMBEDDING_WORKERS"])
    except ValueError:
        logger.error(
            "ML_EMBEDDING_WORKERS must be a positive integer, got %r",
            required["ML_EMBEDDING_WORKERS"],
        )
        sys.exit(1)
    if workers < 1:
        logger.error(
            "ML_EMBEDDING_WORKERS must be a positive integer, got %d",
            workers,
        )
        sys.exit(1)

    try:
        queue_max = int(required["ML_EMBEDDING_QUEUE_MAX"])
    except ValueError:
        logger.error(
            "ML_EMBEDDING_QUEUE_MAX must be a positive integer, got %r",
            required["ML_EMBEDDING_QUEUE_MAX"],
        )
        sys.exit(1)
    if queue_max < 1:
        logger.error(
            "ML_EMBEDDING_QUEUE_MAX must be a positive integer, got %d",
            queue_max,
        )
        sys.exit(1)

    # Cross-validate: queue_max must be ≥ workers so the pool can stay
    # saturated without immediately rejecting active work.
    if queue_max < workers:
        logger.error(
            "ML_EMBEDDING_QUEUE_MAX (%d) must be ≥ ML_EMBEDDING_WORKERS (%d)",
            queue_max,
            workers,
        )
        sys.exit(1)

    # Spec 050 FR-050-003 — health SLA budget in milliseconds. Required for
    # documentation and the adversarial regression. MUST be a positive
    # integer.
    try:
        sla_ms = int(required["ML_HEALTH_LATENCY_SLA_MS"])
    except ValueError:
        logger.error(
            "ML_HEALTH_LATENCY_SLA_MS must be a positive integer (ms), got %r",
            required["ML_HEALTH_LATENCY_SLA_MS"],
        )
        sys.exit(1)
    if sla_ms < 1:
        logger.error(
            "ML_HEALTH_LATENCY_SLA_MS must be a positive integer (ms), got %d",
            sla_ms,
        )
        sys.exit(1)

    # MIT-040-S-004 — SMACKEREL_ENV allowlist enforcement (development | test
    # | production). Any other value is a configuration error and the sidecar
    # exits with sys.exit(1) so uvicorn returns a non-zero exit code.
    environment = required["SMACKEREL_ENV"]
    if environment not in {"development", "test", "production"}:
        logger.error(
            "SMACKEREL_ENV must be one of development|test|production, got %r",
            environment,
        )
        sys.exit(1)

    # MIT-040-S-004 — production-mode auth-token fail-fast. SMACKEREL_AUTH_TOKEN
    # is required only when SMACKEREL_ENV=production. In development/test, an
    # empty token logs a warning and the sidecar continues in dev-mode
    # bypass (auth.py allows all requests through verify_auth when the
    # module-level _AUTH_TOKEN is empty).
    auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
    if not auth_token and environment == "production":
        logger.error("SMACKEREL_AUTH_TOKEN must be set when SMACKEREL_ENV=production")
        sys.exit(1)
    if not auth_token:
        logger.warning(
            "SMACKEREL_AUTH_TOKEN is empty — auth bypassed (dev-mode)",
            extra={"environment": environment},
        )
    required["SMACKEREL_AUTH_TOKEN"] = auth_token

    return required


nats_client: NATSClient | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan: connect to NATS on startup, disconnect on shutdown."""
    global nats_client

    config = _check_required_config()

    nats_url = config["NATS_URL"]
    validate_runtime_streams_on_startup()
    logger.info("Connecting to NATS at %s", nats_url)

    nats_client = NATSClient(nats_url)
    await nats_client.connect()

    # Subscribe to processing subjects
    await nats_client.subscribe_all()
    logger.info("NATS subscriptions active")

    yield

    # Shutdown
    if nats_client:
        await nats_client.close()
        logger.info("NATS connection closed")


app = FastAPI(
    title="Smackerel ML Sidecar",
    version="0.1.0",
    lifespan=lifespan,
)


@app.get("/health")
async def health():
    """Health check endpoint for the ML sidecar."""
    nats_connected = nats_client is not None and nats_client.is_connected
    return {
        "status": "up" if nats_connected else "degraded",
        "nats": "connected" if nats_connected else "disconnected",
        "model_loaded": _model is not None,
    }


@app.get("/metrics")
async def metrics_endpoint():
    """Prometheus metrics endpoint — unauthenticated (standard scrape pattern)."""
    return PlainTextResponse(generate_latest(), media_type=CONTENT_TYPE_LATEST)


# Authenticated router — all non-health HTTP endpoints go here.
# This ensures any future HTTP endpoint is protected by default.
authed_router = APIRouter(dependencies=[Depends(verify_auth)])
app.include_router(authed_router)
