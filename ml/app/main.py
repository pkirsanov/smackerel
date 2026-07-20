"""Smackerel ML Sidecar — FastAPI application."""

import asyncio
import logging
import os
import sys
from contextlib import asynccontextmanager

from fastapi import APIRouter, Depends, FastAPI, HTTPException
from fastapi.responses import JSONResponse, PlainTextResponse
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from pydantic import BaseModel, Field

from .auth import _AUTH_TOKEN, verify_auth
from .embedder import _model_name, generate_embedding, is_model_loaded
from .nats_client import NATSClient
from .nats_contract import validate_runtime_streams_on_startup

# ruff: noqa: E501
# Bootstrap logging is configured at import WITHOUT a level so the module never
# reads a NO-DEFAULTS config fallback at import scope (importing this module in
# tests must not require ML_LOG_LEVEL). The SST-owned log level
# (config/smackerel.yaml services.ml.log_level -> ML_LOG_LEVEL) is a required
# key validated in _check_required_config() and applied fail-loud at startup
# (the sidecar exits if ML_LOG_LEVEL is missing/empty/invalid). Spec 067
# BUG-067-001.
logging.basicConfig(
    format="%(asctime)s %(levelname)s %(name)s %(message)s",
    stream=sys.stdout,
    force=True,
)
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
        # Spec 067 BUG-067-001 — ML sidecar log level (services.ml.log_level).
        # SST-owned and required (fail-loud); allowlist-validated and applied
        # via logging.getLogger().setLevel below.
        "ML_LOG_LEVEL",
        # Spec 050 FR-050-001/002/003 — ML sidecar health/worker isolation
        # contract. All three values are SST-owned and required (fail-loud).
        # The sidecar refuses to start if any is missing, empty, or invalid.
        "ML_EMBEDDING_WORKERS",
        "ML_EMBEDDING_QUEUE_MAX",
        "ML_HEALTH_LATENCY_SLA_MS",
        # BUG-026-008 — exactly one corrective call after parsed synthesis JSON
        # fails schema validation. SST-owned and required for every provider.
        "ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS",
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

    # Spec 102 SCOPE-102-03 (BUG-026-006) — ML_DOMAIN_OUTPUT_TOKEN_BUDGET
    # (services.ml.domain_output_token_budget) is the SST-owned output-token
    # budget for the structured-JSON domain/synthesis extraction path,
    # replacing the hardcoded 2000. Used on EVERY extraction regardless of
    # provider (ollama OR hosted), so it is required unconditionally
    # (fail-loud, no default; mirrors resolve_domain_output_token_budget()).
    try:
        budget = os.environ["ML_DOMAIN_OUTPUT_TOKEN_BUDGET"].strip()
    except KeyError:
        missing.append("ML_DOMAIN_OUTPUT_TOKEN_BUDGET")
    else:
        if not budget:
            missing.append("ML_DOMAIN_OUTPUT_TOKEN_BUDGET")
        required["ML_DOMAIN_OUTPUT_TOKEN_BUDGET"] = budget

    # F2 (redteam LLM-enrichment cold-load) — ML_OLLAMA_KEEP_ALIVE
    # (services.ml.ollama_keep_alive) keeps the domain/synthesis model resident
    # across a capture session so sparse captures skip the cold-load. Required
    # (fail-loud, no default) ONLY when the provider is ollama — it is
    # meaningless for hosted providers. Mirrors the LLM_API_KEY conditional.
    if llm_provider == "ollama":
        try:
            keep_alive = os.environ["ML_OLLAMA_KEEP_ALIVE"].strip()
        except KeyError:
            missing.append("ML_OLLAMA_KEEP_ALIVE")
        else:
            if not keep_alive:
                missing.append("ML_OLLAMA_KEEP_ALIVE")
            required["ML_OLLAMA_KEEP_ALIVE"] = keep_alive

        try:
            profiles_json = os.environ["ML_MODEL_MEMORY_PROFILES_JSON"].strip()
        except KeyError:
            missing.append("ML_MODEL_MEMORY_PROFILES_JSON")
        else:
            if not profiles_json:
                missing.append("ML_MODEL_MEMORY_PROFILES_JSON")
            required["ML_MODEL_MEMORY_PROFILES_JSON"] = profiles_json

        # BUG-026-007 (redteam F2, latency half) — ML_STRUCTURED_EXTRACTION_THINKING
        # (services.ml.structured_extraction_thinking) gates whether qwen3 keeps
        # its hidden reasoning block on the structured-JSON extraction path.
        # Required (fail-loud, no default) ONLY when the provider is ollama — it
        # is meaningless for hosted providers. Mirrors the ML_OLLAMA_KEEP_ALIVE
        # conditional above.
        try:
            thinking = os.environ["ML_STRUCTURED_EXTRACTION_THINKING"].strip()
        except KeyError:
            missing.append("ML_STRUCTURED_EXTRACTION_THINKING")
        else:
            if not thinking:
                missing.append("ML_STRUCTURED_EXTRACTION_THINKING")
            required["ML_STRUCTURED_EXTRACTION_THINKING"] = thinking

    if missing:
        logger.error("Missing required configuration: %s", ", ".join(missing))
        sys.exit(1)

    if llm_provider == "ollama":
        from .ollama_keepalive import OllamaProfileConfigError, resolve_ollama_request_profile

        try:
            resolve_ollama_request_profile(required["LLM_MODEL"])
        except OllamaProfileConfigError as exc:
            logger.error(
                "Invalid Ollama request profile configuration (category=%s)",
                exc.category,
            )
            sys.exit(1)

    if llm_provider == "ollama":
        # BUG-026-007 — the structured-extraction thinking switch must be exactly
        # true/false (mirrors the ML_PROCESSING_DEGRADED_FALLBACK_ENABLED
        # true/false contract). A bad value fails loud rather than silently
        # coercing qwen's thinking mode and re-opening the latency bug.
        thinking = required["ML_STRUCTURED_EXTRACTION_THINKING"].lower()
        if thinking not in ("true", "false"):
            logger.error(
                "ML_STRUCTURED_EXTRACTION_THINKING must be true or false, got %r",
                required["ML_STRUCTURED_EXTRACTION_THINKING"],
            )
            sys.exit(1)

    # Spec 067 BUG-067-001 — apply the SST-owned log level fail-loud. ML_LOG_LEVEL
    # is in the required keys above, so a missing/empty value already exited.
    # Allowed values mirror the Go core LOG_LEVEL contract (debug|info|warn|error).
    log_level = required["ML_LOG_LEVEL"].upper()
    if log_level not in ("DEBUG", "INFO", "WARN", "ERROR"):
        logger.error(
            "ML_LOG_LEVEL must be one of debug|info|warn|error, got %r",
            required["ML_LOG_LEVEL"],
        )
        sys.exit(1)
    logging.getLogger().setLevel(log_level)

    fallback_enabled = required["ML_PROCESSING_DEGRADED_FALLBACK_ENABLED"].lower()
    if fallback_enabled not in ("true", "false"):
        logger.error(
            "Invalid ML_PROCESSING_DEGRADED_FALLBACK_ENABLED=%r; expected true or false",
            required["ML_PROCESSING_DEGRADED_FALLBACK_ENABLED"],
        )
        sys.exit(1)

    try:
        schema_repair_attempts = int(required["ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS"])
    except ValueError:
        logger.error("ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS must be the integer 1")
        sys.exit(1)
    if schema_repair_attempts != 1:
        logger.error("ML_SYNTHESIS_SCHEMA_REPAIR_ATTEMPTS must be the integer 1")
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
    auth_token = _AUTH_TOKEN
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


# F2 — best-effort startup warmup timeout. A cold gemma-class load is ~22-45s;
# the warmup runs in the BACKGROUND (never blocks boot) and absorbs that
# cold-load so the FIRST post-deploy capture is already warm. Bounded so a
# never-arriving model can't leak a forever-pending task.
_WARMUP_TIMEOUT_SECONDS = 180


async def _warmup_domain_model(config: dict[str, str]) -> None:
    """Fire ONE keep_alive'd completion at the domain model so the first capture
    after boot skips the Ollama cold-load. Best-effort: any failure (model not
    pulled yet, Ollama unreachable at boot) is logged and swallowed — this MUST
    NEVER block or fail sidecar startup (F2)."""
    if config["LLM_PROVIDER"].lower() != "ollama":
        return
    from .ollama_keepalive import OllamaProfileConfigError

    try:
        from .ollama_keepalive import dispatch_litellm, resolve_ollama_request_profile

        model = config["LLM_MODEL"]
        ollama_url = config["OLLAMA_URL"]

        import litellm

        warmup_kwargs = {
            "model": f"ollama_chat/{model}",
            "api_base": ollama_url,
            "messages": [{"role": "user", "content": "warmup"}],
            "max_tokens": 1,
            "timeout": _WARMUP_TIMEOUT_SECONDS,
        }
        profile = resolve_ollama_request_profile(model)
        await dispatch_litellm(
            warmup_kwargs,
            provider="ollama",
            model=model,
            profile=profile,
            litellm_module=litellm,
        )
        logger.info(
            "ollama domain-model warmup complete (model=%s keep_alive=%s)",
            model,
            profile.keep_alive,
        )
    except OllamaProfileConfigError as exc:
        logger.warning(
            "ollama domain-model warmup skipped (non-fatal profile error category=%s)",
            exc.category,
        )
    except Exception as exc:  # noqa: BLE001 — warmup is best-effort, never fatal
        logger.warning(
            "ollama domain-model warmup skipped (non-fatal): %s",
            type(exc).__name__,
        )


nats_client: NATSClient | None = None
_warmup_task: "asyncio.Task | None" = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan: connect to NATS on startup, disconnect on shutdown."""
    global nats_client, _warmup_task

    config = _check_required_config()

    nats_url = config["NATS_URL"]
    validate_runtime_streams_on_startup()
    logger.info("Connecting to NATS at %s", nats_url)

    nats_client = NATSClient(nats_url)
    await nats_client.connect()

    # Subscribe to processing subjects
    await nats_client.subscribe_all()
    logger.info("NATS subscriptions active")

    # F2 — kick off a non-blocking best-effort warmup so the FIRST post-deploy
    # capture doesn't pay the Ollama cold-load. Fire-and-forget: boot proceeds
    # immediately while the domain model loads in the background.
    _warmup_task = asyncio.create_task(_warmup_domain_model(config))

    # Spec 059 Scope 3 — Google Keep request/reply bridge (handshake + sync).
    # Uses core-NATS subscribe (not JetStream pull) for synchronous reply
    # semantics matching internal/connector/keep/keep.go's Request() calls.
    from . import keep_bridge

    await keep_bridge.register_nats_handler(nats_client._nc)

    yield

    # Shutdown
    if _warmup_task is not None and not _warmup_task.done():
        _warmup_task.cancel()
    if nats_client:
        await nats_client.close()
        logger.info("NATS connection closed")


app = FastAPI(
    title="Smackerel ML Sidecar",
    version="0.1.0",
    lifespan=lifespan,
)


@app.get("/health")
async def health(strict: str = ""):
    """Health check endpoint for the ML sidecar.

    Liveness vs readiness (redteam F1 / BUG-050-002, ML-sidecar half): the
    DEFAULT ``GET /health`` is ALWAYS 200 because it is the container liveness
    probe. The Docker ``HEALTHCHECK`` (docker-compose.yml) calls
    ``urllib.request.urlopen('.../health')``, which RAISES on ANY non-2xx, so a
    503 here would mark a still-ALIVE-but-degraded sidecar unhealthy and
    flap/restart it. Callers on the operator / monitoring path OPT IN to a
    status-aware signal via ``?strict=true|1|yes``, which returns 503 when the
    sidecar status is not ``"up"`` (e.g. NATS disconnected). The default (no
    param — exactly what the healthcheck sends) is byte-for-byte unchanged and
    stays a plain 200 dict. This mirrors the Go core ``healthStrictRequested``
    contract in ``internal/api/health.go`` so both health surfaces spec 050 owns
    share one opt-in semantics.
    """
    nats_connected = nats_client is not None and nats_client.is_connected
    status = "up" if nats_connected else "degraded"
    body = {
        "status": status,
        "nats": "connected" if nats_connected else "disconnected",
        # Read the embedder's CURRENT state at call time (redteam F8): the model
        # is lazily loaded on the first generate_embedding(), long after this
        # module was imported.
        "model_loaded": is_model_loaded(),
    }
    # Default liveness path: return a plain dict ⇒ FastAPI serialises it as an
    # unconditional HTTP 200, byte-for-byte unchanged. Kept subscriptable so
    # in-process callers (tests, BUG-050-001 regressions) can index it.
    if not _health_strict_requested(strict):
        return body
    # Opt-in readiness path: surface a degraded state via the HTTP status code
    # so a status-code consumer (operator / monitoring) can see it, WITHOUT
    # touching the container liveness contract above.
    return JSONResponse(
        content=body,
        status_code=200 if status == "up" else 503,
    )


def _health_strict_requested(strict: str) -> bool:
    """Whether the caller opted into a status-aware ``/health`` via
    ``?strict=true|1|yes`` (case-insensitive). Only the operator / monitoring
    path sets it; the Docker liveness ``HEALTHCHECK`` never does, so container
    liveness is unaffected. Mirrors the Go ``healthStrictRequested`` contract
    (``internal/api/health.go``) — redteam F1 / BUG-050-002."""
    return strict.strip().lower() in {"1", "true", "yes"}


@app.get("/metrics")
async def metrics_endpoint():
    """Prometheus metrics endpoint — unauthenticated (standard scrape pattern)."""
    return PlainTextResponse(generate_latest(), media_type=CONTENT_TYPE_LATEST)


# Authenticated router — all non-health HTTP endpoints go here.
# This ensures any future HTTP endpoint is protected by default.
authed_router = APIRouter(dependencies=[Depends(verify_auth)])


# BUG-061-004 — assistant NL routing embedder endpoint. The Go core's
# assistant router calls POST /embed with a single text string and
# expects a {vector, dim, model} response. Delegates to the shared
# sentence-transformer pool via generate_embedding().
class EmbedRequest(BaseModel):
    text: str = Field(min_length=1)


class EmbedResponse(BaseModel):
    vector: list[float]
    dim: int
    model: str


@authed_router.post("/embed", response_model=EmbedResponse)
async def embed(req: EmbedRequest) -> EmbedResponse:
    try:
        vec = await generate_embedding(req.text)
    except Exception as exc:  # noqa: BLE001 — surface upstream failures
        raise HTTPException(status_code=503, detail=f"embedder unavailable: {exc}") from exc
    if not vec:
        raise HTTPException(status_code=503, detail="embedder returned empty vector")
    # Spec 067 BUG-067-001 — report the embedder's actual loaded model name
    # (no os.getenv default fallback). The sidecar always encodes with
    # embedder._model_name, so reporting it is both fail-loud-clean and more
    # truthful than echoing a possibly-divergent EMBEDDING_MODEL env value.
    return EmbedResponse(vector=vec, dim=len(vec), model=_model_name)


app.include_router(authed_router)

# Spec 064 SCOPE-04 — open-ended knowledge agent LLM bridge. Mounted
# through verify_auth so the new /llm/chat endpoint inherits the same
# auth contract as the rest of the sidecar surface.
from .routes.chat import router as openknowledge_chat_router  # noqa: E402

app.include_router(openknowledge_chat_router, dependencies=[Depends(verify_auth)])

# Spec 083 Scope 05 — card-rewards strict-schema rotating-category extraction.
# Mounted under verify_auth so POST /extract-card-categories inherits the same
# Bearer-auth contract. The model-gateway call lives here (Constitution C2); the
# Go orchestrator only sends page text + candidate and re-validates the response.
from .card_categories import router as card_categories_router  # noqa: E402

app.include_router(card_categories_router, dependencies=[Depends(verify_auth)])
