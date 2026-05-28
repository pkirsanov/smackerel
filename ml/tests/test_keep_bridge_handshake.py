"""Tests for spec 059 Scope 3 — NATS request/reply bridge wiring.

Covers SCN-059-007 (sync handler success / exception envelope) and
SCN-059-019 (sidecar handshake fail-loud on empty App Password env var,
ok when set, no value/length/hash echo).
"""

from __future__ import annotations

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock


class _FakeMsg:
    def __init__(self, data: bytes) -> None:
        self.data = data
        self.responded: list[bytes] = []

    async def respond(self, payload: bytes) -> None:
        self.responded.append(payload)


# ---------- SCN-059-019: handshake handler ----------


def test_handle_handshake_request_rejects_empty_app_password(monkeypatch):
    from app import keep_bridge

    monkeypatch.delenv(keep_bridge.APP_PASSWORD_ENV, raising=False)
    result = asyncio.run(keep_bridge.handle_handshake_request({"request_id": "k-1-aa"}))
    assert result["status"] == "error"
    assert result["error"] == f"{keep_bridge.APP_PASSWORD_ENV} is required"
    assert result["schema_version"] == keep_bridge.KEEP_SCHEMA_VERSION


def test_handle_handshake_request_accepts_non_empty_app_password(monkeypatch):
    from app import keep_bridge

    monkeypatch.setenv(keep_bridge.APP_PASSWORD_ENV, "fixture-password-not-real")
    result = asyncio.run(keep_bridge.handle_handshake_request({"request_id": "k-1-bb"}))
    assert result["status"] == "ok"
    assert result["schema_version"] == keep_bridge.KEEP_SCHEMA_VERSION
    # Adversarial: reply MUST NOT echo the value, its length, or any hash.
    serialized = json.dumps(result)
    assert "fixture-password-not-real" not in serialized
    assert "len=" not in serialized
    assert "sha" not in serialized.lower()


# ---------- SCN-059-007: sync handler envelopes ----------


def test_handle_sync_request_wraps_exception_as_error_envelope(monkeypatch):
    """When the sidecar handler raises, register_nats_handler's wrapper
    converts it to a fail-loud error envelope rather than dropping the
    request on the floor."""
    from app import keep_bridge

    nc = MagicMock()
    nc.subscribe = AsyncMock()

    async def boom(data):
        raise RuntimeError("synthetic test failure")

    monkeypatch.setattr(keep_bridge, "handle_sync_request", boom)
    asyncio.run(keep_bridge.register_nats_handler(nc))
    # Two subscribe calls (sync + handshake)
    assert nc.subscribe.await_count == 2
    sync_call = next(c for c in nc.subscribe.await_args_list if c.args[0] == keep_bridge.KEEP_SYNC_SUBJECT)
    cb = sync_call.kwargs["cb"]
    msg = _FakeMsg(json.dumps({"cursor": "c1"}).encode())
    asyncio.run(cb(msg))
    assert len(msg.responded) == 1
    payload = json.loads(msg.responded[0])
    assert payload["status"] == "error"
    assert payload["error"] == "RuntimeError"
    assert payload["cursor"] == "c1"
    assert payload["schema_version"] == keep_bridge.KEEP_SCHEMA_VERSION


def test_register_nats_handler_subscribes_handshake_and_sync(monkeypatch):
    from app import keep_bridge

    nc = MagicMock()
    nc.subscribe = AsyncMock()
    asyncio.run(keep_bridge.register_nats_handler(nc))
    subjects = [c.args[0] for c in nc.subscribe.await_args_list]
    assert keep_bridge.KEEP_SYNC_SUBJECT in subjects
    assert keep_bridge.KEEP_HANDSHAKE_SUBJECT in subjects


def test_handshake_callback_rejects_when_password_empty(monkeypatch):
    from app import keep_bridge

    monkeypatch.delenv(keep_bridge.APP_PASSWORD_ENV, raising=False)
    nc = MagicMock()
    nc.subscribe = AsyncMock()
    asyncio.run(keep_bridge.register_nats_handler(nc))
    handshake_call = next(c for c in nc.subscribe.await_args_list if c.args[0] == keep_bridge.KEEP_HANDSHAKE_SUBJECT)
    cb = handshake_call.kwargs["cb"]
    msg = _FakeMsg(json.dumps({"request_id": "k-9-cc"}).encode())
    asyncio.run(cb(msg))
    payload = json.loads(msg.responded[0])
    assert payload["status"] == "error"
    assert payload["error"] == f"{keep_bridge.APP_PASSWORD_ENV} is required"
