"""Google Keep gkeepapi bridge for the ML sidecar.

Handles keep.sync.request NATS messages using the gkeepapi library.
This is an OPTIONAL, opt-in feature that uses an UNOFFICIAL Google API.
"""

import datetime
import json
import logging
import os
import time
from typing import Any

logger = logging.getLogger("smackerel-ml.keep-bridge")

# Spec 059 Scope 3 — single source of truth for the NATS bridge contract
# between the Go core and this sidecar. Mirrors internal/connector/keep/keep.go.
KEEP_SYNC_SUBJECT = "keep.sync.request"
KEEP_HANDSHAKE_SUBJECT = "keep.sidecar.handshake"
KEEP_SCHEMA_VERSION = 1

# The env-var name for the Google App Password is stored ONLY here in the
# sidecar (SCN-059-019, Scope 1 boundary). The Go core MUST NOT reference
# this name anywhere in non-test source.
APP_PASSWORD_ENV = "KEEP_GOOGLE_APP_PASSWORD"

# Cached gkeepapi session
_keep_session = None
_session_email = None
_session_authenticated_at: float = 0.0

# Session max age: re-authenticate after 50 minutes to avoid stale sessions.
# gkeepapi sessions can expire server-side after extended inactivity.
SESSION_MAX_AGE_SECONDS = 50 * 60

# Maximum notes to fetch per sync cycle to prevent memory exhaustion
# on accounts with very large note counts.
MAX_SYNC_NOTES = 10_000


def _is_session_expired() -> bool:
    """Check if the cached session has exceeded its max age."""
    if _session_authenticated_at == 0.0:
        return True
    return (time.time() - _session_authenticated_at) > SESSION_MAX_AGE_SECONDS


def authenticate() -> Any:
    """Authenticate with Google Keep via gkeepapi.

    Uses KEEP_GOOGLE_EMAIL and KEEP_GOOGLE_APP_PASSWORD env vars.
    Caches the authenticated session for reuse across sync cycles.
    Re-authenticates when the session exceeds SESSION_MAX_AGE_SECONDS.
    """
    global _keep_session, _session_email, _session_authenticated_at

    email = os.environ.get("KEEP_GOOGLE_EMAIL")
    password = os.environ.get("KEEP_GOOGLE_APP_PASSWORD")

    if not email or not password:
        raise ValueError("KEEP_GOOGLE_EMAIL and KEEP_GOOGLE_APP_PASSWORD must be set for gkeepapi")

    # Reuse cached session if same email and not expired
    if _keep_session is not None and _session_email == email and not _is_session_expired():
        logger.info("Reusing cached gkeepapi session for %s", email)
        return _keep_session

    if _keep_session is not None and _is_session_expired():
        logger.info("gkeepapi session expired after %ds, re-authenticating", SESSION_MAX_AGE_SECONDS)

    try:
        import gkeepapi  # noqa: F811

        keep = gkeepapi.Keep()
        keep.login(email, password)
        _keep_session = keep
        _session_email = email
        _session_authenticated_at = time.time()
        logger.info("Authenticated with gkeepapi for %s", email)
        return keep
    except ImportError:
        raise RuntimeError("gkeepapi is not installed. Install with: pip install gkeepapi")
    except Exception as exc:
        _keep_session = None
        _session_email = None
        _session_authenticated_at = 0.0
        raise RuntimeError(f"gkeepapi authentication failed: {exc}") from exc


def serialize_note(gnote: Any) -> dict:
    """Serialize a gkeepapi note into a JSON-compatible dict.

    Args:
        gnote: A gkeepapi TopLevelNode (note object).

    Returns:
        Dict with note fields matching the GkeepNote schema.
    """
    labels = []
    try:
        labels = [label.name for label in gnote.labels.all()]
    except Exception as exc:
        logger.warning("serialize_note: labels access failed: %s: %s", type(exc).__name__, exc)

    collaborators = []
    try:
        collaborators = [c.email for c in gnote.collaborators.all()]
    except Exception as exc:
        logger.warning("serialize_note: collaborators access failed: %s: %s", type(exc).__name__, exc)

    list_items = []
    try:
        for item in gnote.items:
            list_items.append(
                {
                    "text": item.text or "",
                    "is_checked": item.checked,
                }
            )
    except Exception as exc:
        logger.warning("serialize_note: list_items access failed: %s: %s", type(exc).__name__, exc)

    timestamps = gnote.timestamps
    modified_usec = 0
    created_usec = 0
    try:
        if timestamps.updated:
            modified_usec = int(timestamps.updated.timestamp() * 1_000_000)
    except Exception as exc:
        logger.warning("serialize_note: timestamps.updated access failed: %s: %s", type(exc).__name__, exc)
    try:
        if timestamps.created:
            created_usec = int(timestamps.created.timestamp() * 1_000_000)
    except Exception as exc:
        logger.warning("serialize_note: timestamps.created access failed: %s: %s", type(exc).__name__, exc)

    return {
        "note_id": gnote.id or "",
        "title": gnote.title or "",
        "text_content": gnote.text or "",
        "is_pinned": gnote.pinned,
        "is_archived": gnote.archived,
        "is_trashed": gnote.trashed,
        "color": str(gnote.color) if gnote.color else "DEFAULT",
        "labels": labels,
        "collaborators": collaborators,
        "list_items": list_items,
        "modified_usec": modified_usec,
        "created_usec": created_usec,
    }


async def handle_sync_request(data: dict) -> dict:
    """Handle a keep.sync.request NATS message.

    Args:
        data: Request payload with optional 'cursor' field.

    Returns:
        Response dict with 'status', 'notes', 'cursor', and optional 'error'.
    """
    global _keep_session, _session_email, _session_authenticated_at
    cursor = data.get("cursor", "")

    try:
        keep = authenticate()
    except Exception as exc:
        logger.error("gkeepapi authentication failed: %s", exc)
        return {
            "status": "error",
            "notes": [],
            "cursor": cursor,
            "error": "gkeepapi authentication failed",
        }

    for attempt in range(2):  # retry once on sync failure with re-auth
        try:
            # Sync to get latest state
            keep.sync()

            notes = []
            for gnote in keep.all():
                serialized = serialize_note(gnote)

                # Filter by cursor if provided
                if cursor and serialized["modified_usec"] > 0:
                    cursor_time = datetime.datetime.fromisoformat(cursor.replace("Z", "+00:00"))
                    cursor_usec = int(cursor_time.timestamp() * 1_000_000)
                    if serialized["modified_usec"] <= cursor_usec:
                        continue

                notes.append(serialized)
                if len(notes) >= MAX_SYNC_NOTES:
                    logger.warning(
                        "gkeepapi sync hit note limit (%d), remaining notes deferred to next cycle",
                        MAX_SYNC_NOTES,
                    )
                    break

            # Determine new cursor from latest modified time
            new_cursor = cursor
            if notes:
                latest_usec = max(n["modified_usec"] for n in notes)
                if latest_usec > 0:
                    new_cursor = (
                        datetime.datetime.fromtimestamp(
                            latest_usec / 1_000_000,
                            tz=datetime.timezone.utc,
                        )
                        .isoformat()
                        .replace("+00:00", "Z")
                    )

            logger.info(
                "gkeepapi sync complete: %d notes fetched, cursor=%s",
                len(notes),
                new_cursor,
            )

            return {
                "status": "ok",
                "notes": notes,
                "cursor": new_cursor,
                "schema_version": KEEP_SCHEMA_VERSION,
            }

        except Exception as exc:
            if attempt == 0:
                # First failure: invalidate session and retry with fresh auth
                logger.warning("gkeepapi sync failed (attempt %d), re-authenticating: %s", attempt + 1, exc)
                _keep_session = None
                _session_email = None
                _session_authenticated_at = 0.0
                try:
                    keep = authenticate()
                except Exception as reauth_exc:
                    logger.error("gkeepapi re-authentication failed: %s", reauth_exc)
                    return {
                        "status": "error",
                        "notes": [],
                        "cursor": cursor,
                        "error": "gkeepapi re-authentication failed",
                    }
            else:
                logger.error("gkeepapi sync failed after retry: %s", exc)
                return {
                    "status": "error",
                    "notes": [],
                    "cursor": cursor,
                    "error": "gkeepapi sync failed after retry",
                }

    # Should not reach here, but guard anyway
    return {"status": "error", "notes": [], "cursor": cursor, "error": "unexpected retry exhaustion"}


async def handle_handshake_request(data: dict) -> dict:
    """Handle a keep.sidecar.handshake NATS message (spec 059 SCN-059-019).

    Validates that ``APP_PASSWORD_ENV`` is set non-empty on the sidecar.
    The reply MUST NOT echo the value, its length, or any hash — only the
    field name appears in the error string.
    """
    # Fail-loud SST: no fallback default. Missing/empty env returns a
    # structured error envelope (sidecar contract) without leaking value.
    password = os.environ.get(APP_PASSWORD_ENV)
    if not password:
        return {
            "status": "error",
            "error": f"{APP_PASSWORD_ENV} is required",
            "schema_version": KEEP_SCHEMA_VERSION,
        }
    return {
        "status": "ok",
        "schema_version": KEEP_SCHEMA_VERSION,
    }


async def register_nats_handler(nc) -> None:
    """Subscribe the Keep sync + handshake request/reply handlers.

    ``nc`` must be a core-NATS client (or test double) that exposes
    ``subscribe(subject, cb=...)`` where ``cb`` receives a message with
    ``.data`` and an awaitable ``.respond(payload: bytes)``.

    Each handler wraps its dispatch in a try/except so that any unhandled
    exception is converted into a fail-loud error envelope rather than
    dropped on the floor (SCN-059-007).
    """

    async def _sync_cb(msg):
        try:
            req = json.loads(msg.data.decode()) if msg.data else {}
        except Exception as exc:
            await msg.respond(
                json.dumps(
                    {
                        "status": "error",
                        "notes": [],
                        "cursor": "",
                        "error": type(exc).__name__,
                        "schema_version": KEEP_SCHEMA_VERSION,
                    }
                ).encode()
            )
            return
        cursor = req.get("cursor", "")
        try:
            result = await handle_sync_request(req)
            result.setdefault("schema_version", KEEP_SCHEMA_VERSION)
        except Exception as exc:
            logger.exception("keep sync handler raised; returning error envelope")
            result = {
                "status": "error",
                "notes": [],
                "cursor": cursor,
                "error": type(exc).__name__,
                "schema_version": KEEP_SCHEMA_VERSION,
            }
        await msg.respond(json.dumps(result).encode())

    async def _handshake_cb(msg):
        try:
            req = json.loads(msg.data.decode()) if msg.data else {}
        except Exception as exc:
            await msg.respond(
                json.dumps(
                    {
                        "status": "error",
                        "error": type(exc).__name__,
                        "schema_version": KEEP_SCHEMA_VERSION,
                    }
                ).encode()
            )
            return
        try:
            result = await handle_handshake_request(req)
        except Exception as exc:
            logger.exception("keep handshake handler raised; returning error envelope")
            result = {
                "status": "error",
                "error": type(exc).__name__,
                "schema_version": KEEP_SCHEMA_VERSION,
            }
        await msg.respond(json.dumps(result).encode())

    await nc.subscribe(KEEP_SYNC_SUBJECT, cb=_sync_cb)
    await nc.subscribe(KEEP_HANDSHAKE_SUBJECT, cb=_handshake_cb)
    logger.info("Google Keep NATS bridge subscribed (%s, %s)", KEEP_SYNC_SUBJECT, KEEP_HANDSHAKE_SUBJECT)
