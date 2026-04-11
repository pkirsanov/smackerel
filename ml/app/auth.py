"""Authentication dependency for the ML sidecar."""

import hmac
import logging
import os

from fastapi import HTTPException, Request

logger = logging.getLogger("smackerel-ml.auth")

_AUTH_TOKEN = os.environ.get("SMACKEREL_AUTH_TOKEN", "")


async def verify_auth(request: Request) -> None:
    """Validate auth token on non-health endpoints.

    When SMACKEREL_AUTH_TOKEN is empty, all requests pass (dev mode).
    When configured, checks Authorization: Bearer or X-Auth-Token header.
    Uses hmac.compare_digest for constant-time comparison.
    """
    if not _AUTH_TOKEN:
        return

    token = None

    auth_header = request.headers.get("authorization", "")
    if auth_header.lower().startswith("bearer "):
        token = auth_header[7:]

    if token is None:
        token = request.headers.get("x-auth-token")

    if token is None or not hmac.compare_digest(token, _AUTH_TOKEN):
        client_host = request.client.host if request.client else "unknown"
        logger.warning("Auth failure: %s %s from %s", request.method, request.url.path, client_host)
        raise HTTPException(status_code=401, detail="Unauthorized")
