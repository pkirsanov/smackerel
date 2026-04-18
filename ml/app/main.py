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

logger = logging.getLogger("smackerel-ml")


def _check_required_config() -> dict[str, str]:
    """Validate required environment variables. Fail loudly if missing."""
    keys = ["NATS_URL", "LLM_PROVIDER", "LLM_MODEL", "OLLAMA_URL"]
    required: dict[str, str] = {}
    missing: list[str] = []
    for k in keys:
        val = os.environ.get(k)
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
    return required


nats_client: NATSClient | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan: connect to NATS on startup, disconnect on shutdown."""
    global nats_client

    config = _check_required_config()

    auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
    if not auth_token:
        logger.warning("SMACKEREL_AUTH_TOKEN is empty — ML sidecar running without authentication")

    nats_url = config["NATS_URL"]
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
