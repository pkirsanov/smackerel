"""Google Keep gkeepapi bridge for the ML sidecar.

Handles keep.sync.request NATS messages using the gkeepapi library.
This is an OPTIONAL, opt-in feature that uses an UNOFFICIAL Google API.
"""

import logging
import os
from typing import Any

logger = logging.getLogger("smackerel-ml.keep-bridge")

# Cached gkeepapi session
_keep_session = None
_session_email = None


def authenticate() -> Any:
    """Authenticate with Google Keep via gkeepapi.

    Uses KEEP_GOOGLE_EMAIL and KEEP_GOOGLE_APP_PASSWORD env vars.
    Caches the authenticated session for reuse across sync cycles.
    """
    global _keep_session, _session_email

    email = os.environ.get("KEEP_GOOGLE_EMAIL")
    password = os.environ.get("KEEP_GOOGLE_APP_PASSWORD")

    if not email or not password:
        raise ValueError("KEEP_GOOGLE_EMAIL and KEEP_GOOGLE_APP_PASSWORD must be set for gkeepapi")

    # Reuse cached session if same email
    if _keep_session is not None and _session_email == email:
        logger.info("Reusing cached gkeepapi session for %s", email)
        return _keep_session

    try:
        import gkeepapi  # noqa: F811

        keep = gkeepapi.Keep()
        keep.login(email, password)
        _keep_session = keep
        _session_email = email
        logger.info("Authenticated with gkeepapi for %s", email)
        return keep
    except ImportError:
        raise RuntimeError("gkeepapi is not installed. Install with: pip install gkeepapi")
    except Exception as exc:
        _keep_session = None
        _session_email = None
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
    except Exception:
        pass

    collaborators = []
    try:
        collaborators = [c.email for c in gnote.collaborators.all()]
    except Exception:
        pass

    list_items = []
    try:
        if hasattr(gnote, "items"):
            for item in gnote.items:
                list_items.append(
                    {
                        "text": item.text or "",
                        "is_checked": item.checked,
                    }
                )
    except Exception:
        pass

    timestamps = gnote.timestamps
    modified_usec = 0
    created_usec = 0
    try:
        if timestamps.updated:
            modified_usec = int(timestamps.updated.timestamp() * 1_000_000)
        if timestamps.created:
            created_usec = int(timestamps.created.timestamp() * 1_000_000)
    except Exception:
        pass

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
    cursor = data.get("cursor", "")

    try:
        keep = authenticate()
    except Exception as exc:
        logger.error("gkeepapi authentication failed: %s", exc)
        return {
            "status": "error",
            "notes": [],
            "cursor": cursor,
            "error": str(exc),
        }

    try:
        # Sync to get latest state
        keep.sync()

        notes = []
        for gnote in keep.all():
            serialized = serialize_note(gnote)

            # Filter by cursor if provided
            if cursor and serialized["modified_usec"] > 0:
                import datetime

                cursor_time = datetime.datetime.fromisoformat(cursor.replace("Z", "+00:00"))
                cursor_usec = int(cursor_time.timestamp() * 1_000_000)
                if serialized["modified_usec"] <= cursor_usec:
                    continue

            notes.append(serialized)

        # Determine new cursor from latest modified time
        new_cursor = cursor
        if notes:
            latest_usec = max(n["modified_usec"] for n in notes)
            if latest_usec > 0:
                import datetime

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
        }

    except Exception as exc:
        logger.error("gkeepapi sync failed: %s", exc)
        return {
            "status": "error",
            "notes": [],
            "cursor": cursor,
            "error": str(exc),
        }
