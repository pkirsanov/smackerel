"""Authentication dependency for the ML sidecar."""

import hmac
import logging
import os

from fastapi import HTTPException, Request

logger = logging.getLogger("smackerel-ml.auth")

# HL-RESCAN-013 / Gate G028 (NO-DEFAULTS / fail-loud SST policy) — read
# SMACKEREL_AUTH_TOKEN at module-import time using the os.environ[KEY]
# form (NOT os.environ.get(KEY, "")), so an UNSET env var raises a
# clear RuntimeError at import. This is the canonical Python fail-loud
# pattern from `.github/copilot-instructions.md` (Secrets Management
# table). An EMPTY string is still allowed: it signals dev-mode auth
# bypass and is honoured by verify_auth() below; the production-vs-dev
# branching that converts empty + SMACKEREL_ENV=production to
# sys.exit(1) lives in ml/app/main.py:_check_required_config (which
# runs at lifespan startup AFTER this module-import-time read).
try:
    _AUTH_TOKEN = os.environ["SMACKEREL_AUTH_TOKEN"]
except KeyError as exc:
    raise RuntimeError(
        "ml/app/auth.py: SMACKEREL_AUTH_TOKEN must be set in the env file "
        "(run ./smackerel.sh config generate); empty value is allowed for "
        "dev-mode auth bypass when SMACKEREL_ENV=development|test "
        "(HL-RESCAN-013 / Gate G028 fail-loud SST contract)"
    ) from exc


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

    if token is None:
        client_host = request.client.host if request.client else "unknown"
        logger.warning("Auth failure: %s %s from %s", request.method, request.url.path, client_host)
        raise HTTPException(status_code=401, detail="Unauthorized")

    try:
        match = hmac.compare_digest(token, _AUTH_TOKEN)
    except TypeError:
        # hmac.compare_digest raises TypeError on non-ASCII str args (CWE-755).
        # Treat as auth failure instead of leaking a 500 Internal Server Error.
        match = False

    if not match:
        client_host = request.client.host if request.client else "unknown"
        logger.warning("Auth failure: %s %s from %s", request.method, request.url.path, client_host)
        raise HTTPException(status_code=401, detail="Unauthorized")
