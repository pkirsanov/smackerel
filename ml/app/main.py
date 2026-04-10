"""Smackerel ML Sidecar — FastAPI application."""

import logging
import os
import sys
from contextlib import asynccontextmanager

from fastapi import APIRouter, Depends, FastAPI

from .auth import verify_auth
from .embedder import _model
from .nats_client import NATSClient

logger = logging.getLogger("smackerel-ml")


def _check_required_config() -> dict[str, str]:
    """Validate required environment variables. Fail loudly if missing."""
    required = {
        "NATS_URL": os.getenv("NATS_URL", ""),
        "LLM_PROVIDER": os.getenv("LLM_PROVIDER", ""),
        "LLM_MODEL": os.getenv("LLM_MODEL", ""),
        "OLLAMA_URL": os.getenv("OLLAMA_URL", ""),
    }

    llm_provider = required["LLM_PROVIDER"].lower()
    if llm_provider != "ollama":
        required["LLM_API_KEY"] = os.getenv("LLM_API_KEY", "")

    missing = [k for k, v in required.items() if not v]
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
        logger.warning(
            "SMACKEREL_AUTH_TOKEN is empty — ML sidecar running without authentication"
        )

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


# Authenticated router — all non-health HTTP endpoints go here.
# This ensures any future HTTP endpoint is protected by default.
authed_router = APIRouter(dependencies=[Depends(verify_auth)])
app.include_router(authed_router)
