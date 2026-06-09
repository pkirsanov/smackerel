"""Unit tests for spec 081 — NATS Python sidecar hardening parity.

Covers T-081-U1..U5:
- U1: ConsumerConfig(max_deliver, ack_wait) threaded into every pull_subscribe
- U2: fail-loud on missing NATS_CONSUMER_MAX_DELIVER / _ACK_WAIT_SECONDS
- U3: canonical 6-header dead-letter envelope parity with Go
- U4: publish-before-term invariant (publish failure → nak, NOT term)
- U5: _failure_counts attribute removed from NATSClient

Tests use asyncio.run() rather than pytest-asyncio (matches the
project's existing test style — see ml/tests/test_startup_warning.py
and ml/tests/test_keep_bridge_handshake.py).
"""

from __future__ import annotations

import asyncio
import inspect
import re
import sys
import types
from datetime import datetime
from unittest.mock import AsyncMock, MagicMock

import pytest

# Mock litellm before importing app modules (same pattern as test_nats_client.py).
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

from nats.js.api import ConsumerConfig  # noqa: E402

import app.nats_client as nc_mod  # noqa: E402
from app.nats_client import (  # noqa: E402
    SUBJECT_TO_STREAM,
    SUBSCRIBE_SUBJECTS,
    NATSClient,
)


def _make_msg(num_delivered: int, *, consumer: str = "smackerel-ml-artifacts-process", data: bytes = b'{"x":1}'):
    md = MagicMock()
    md.num_delivered = num_delivered
    md.consumer = consumer
    msg = MagicMock()
    msg.metadata = md
    msg.data = data
    msg.term = AsyncMock()
    msg.nak = AsyncMock()
    msg.ack = AsyncMock()
    return msg


# ---------------------------------------------------------------------------
# T-081-U1 — ConsumerConfig threaded into every pull_subscribe call
# ---------------------------------------------------------------------------


def test_subscribe_all_threads_consumer_config(monkeypatch):
    """Every pull_subscribe call must receive ConsumerConfig built from
    NATS_CONSUMER_MAX_DELIVER + NATS_CONSUMER_ACK_WAIT_SECONDS read ONCE
    at the top of subscribe_all (design §4.1)."""
    monkeypatch.setenv("NATS_CONSUMER_MAX_DELIVER", "5")
    monkeypatch.setenv("NATS_CONSUMER_ACK_WAIT_SECONDS", "120")

    client = NATSClient("nats://localhost:4222")
    mock_js = MagicMock()
    mock_js.pull_subscribe = AsyncMock(return_value=MagicMock())
    client._js = mock_js

    # Prevent background consume tasks from being scheduled during the
    # test (they would reference a non-existent running event loop).
    monkeypatch.setattr(nc_mod.asyncio, "create_task", lambda _coro: MagicMock())

    asyncio.run(client.subscribe_all())

    assert mock_js.pull_subscribe.await_count == len(SUBSCRIBE_SUBJECTS)
    for call in mock_js.pull_subscribe.await_args_list:
        cfg = call.kwargs.get("config")
        assert isinstance(cfg, ConsumerConfig), f"pull_subscribe called without ConsumerConfig: {call.kwargs}"
        assert cfg.max_deliver == 5
        # nats-py ConsumerConfig.ack_wait is a float in SECONDS; the
        # library performs the seconds→nanoseconds conversion before sending
        # the JSON to JetStream. Pre-multiplying by 1e9 here used to overflow
        # int64 on the wire (err_code=10025 "invalid JSON").
        assert cfg.ack_wait == 120.0

    assert client._consumer_max_deliver == 5
    assert client._consumer_ack_wait_seconds == 120


# ---------------------------------------------------------------------------
# T-081-U2 — fail-loud on missing env vars (G028 SST)
# ---------------------------------------------------------------------------


@pytest.mark.parametrize(
    "missing_key,present_key,present_val",
    [
        ("NATS_CONSUMER_MAX_DELIVER", "NATS_CONSUMER_ACK_WAIT_SECONDS", "120"),
        ("NATS_CONSUMER_ACK_WAIT_SECONDS", "NATS_CONSUMER_MAX_DELIVER", "5"),
    ],
)
def test_subscribe_all_fails_loud_when_consumer_env_missing(
    monkeypatch,
    missing_key,
    present_key,
    present_val,
):
    """Missing NATS_CONSUMER_* env var → RuntimeError naming the key and
    the YAML path it should be set from."""
    monkeypatch.delenv(missing_key, raising=False)
    monkeypatch.setenv(present_key, present_val)

    client = NATSClient("nats://localhost:4222")
    client._js = MagicMock()

    with pytest.raises(RuntimeError) as excinfo:
        asyncio.run(client.subscribe_all())

    msg = str(excinfo.value)
    assert missing_key in msg
    assert "infrastructure.nats.consumer." in msg
    assert "config/smackerel.yaml" in msg


def test_no_getenv_fallback_defaults_for_consumer_env():
    """Source-level guard: no os.getenv fallback for NATS_CONSUMER_* keys
    (G028 NO-DEFAULTS / smackerel-no-defaults policy)."""
    src = inspect.getsource(nc_mod)
    bad = re.findall(r'getenv\(\s*["\']NATS_CONSUMER_[^"\']+["\']\s*,', src)
    assert not bad, f"NATS_CONSUMER_* env reads must not have getenv defaults: {bad}"


@pytest.mark.parametrize(
    "max_deliver_val,ack_wait_val,offending_key,offending_val,reason_fragment",
    [
        # Non-integer values — spec 081 §2 Hard Constraint:
        # "Non-integer values fail loud with the offending value in the message".
        ("abc", "120", "NATS_CONSUMER_MAX_DELIVER", "abc", "must be an integer"),
        ("5", "xyz", "NATS_CONSUMER_ACK_WAIT_SECONDS", "xyz", "must be an integer"),
        # Below-minimum values — spec 081 FR-081-001: each key is int >= 1.
        ("0", "120", "NATS_CONSUMER_MAX_DELIVER", "0", "must be >= 1"),
        ("-3", "120", "NATS_CONSUMER_MAX_DELIVER", "-3", "must be >= 1"),
        ("5", "0", "NATS_CONSUMER_ACK_WAIT_SECONDS", "0", "must be >= 1"),
    ],
)
def test_subscribe_all_fails_loud_on_malformed_consumer_env(
    monkeypatch,
    max_deliver_val,
    ack_wait_val,
    offending_key,
    offending_val,
    reason_fragment,
):
    """Spec 081 §2 Hard Constraint + FR-081-001 — a present-but-malformed
    NATS_CONSUMER_* value (non-integer, or integer < 1) MUST raise
    RuntimeError that names the offending key, the reason, and the
    offending value.

    Gaps-probe (reconcile-to-doc 2026-06-07): the pre-existing suite only
    covered the *missing*-key path (test_subscribe_all_fails_loud_when_
    consumer_env_missing). The int()/`>= 1` validation branches in
    subscribe_all had no regression coverage, even though the design says
    this mirrors the spec 046 reconnect-contract pattern — and spec 046
    DID test its non-integer path
    (test_nats_client.py::test_connect_fails_loud_on_non_integer_max_reconnect_attempts).
    This closes that asymmetric ⬛ UNTESTED gap. Adversarial: deleting the
    int() guard or the `< 1` check makes this fail (no/other error, or the
    offending value/reason absent from the message)."""
    monkeypatch.setenv("NATS_CONSUMER_MAX_DELIVER", max_deliver_val)
    monkeypatch.setenv("NATS_CONSUMER_ACK_WAIT_SECONDS", ack_wait_val)

    client = NATSClient("nats://localhost:4222")
    client._js = MagicMock()

    with pytest.raises(RuntimeError) as excinfo:
        asyncio.run(client.subscribe_all())

    msg = str(excinfo.value)
    assert offending_key in msg, f"error must name the offending key: {msg!r}"
    assert reason_fragment in msg, f"error must state the reason: {msg!r}"
    assert offending_val in msg, f"error must include the offending value: {msg!r}"


# ---------------------------------------------------------------------------
# T-081-U3 — canonical 6-header dead-letter envelope (Go parity)
# ---------------------------------------------------------------------------


def test_deadletter_headers_match_go_envelope():
    """Captured headers passed to _js.publish must be exactly the Go 6-name
    envelope, byte-for-byte (subject names + value formats)."""
    client = NATSClient("nats://localhost:4222")
    client._consumer_max_deliver = 3
    mock_js = MagicMock()
    mock_js.publish = AsyncMock()
    client._js = mock_js

    msg = _make_msg(num_delivered=3)
    asyncio.run(client._handle_poison("artifacts.process", msg, RuntimeError("boom")))

    mock_js.publish.assert_awaited_once()
    args, kwargs = mock_js.publish.call_args
    assert args[0] == "deadletter.artifacts.process"
    assert args[1] == b'{"x":1}'

    headers = kwargs["headers"]
    expected_keys = {
        "Smackerel-Original-Subject",
        "Smackerel-Original-Stream",
        "Smackerel-Failed-At",
        "Smackerel-Last-Error",
        "Smackerel-Delivery-Count",
        "Smackerel-Original-Consumer",
    }
    assert set(headers.keys()) == expected_keys, f"header set drifted from Go envelope: {set(headers.keys())}"
    assert headers["Smackerel-Original-Subject"] == "artifacts.process"
    assert headers["Smackerel-Original-Stream"] == "ARTIFACTS"
    assert headers["Smackerel-Delivery-Count"] == "3"
    assert headers["Smackerel-Last-Error"] == "boom"
    assert headers["Smackerel-Original-Consumer"] == "smackerel-ml-artifacts-process"
    # RFC3339 UTC ending in Z (no fractional seconds; parity with Go time.RFC3339).
    fa = headers["Smackerel-Failed-At"]
    assert fa.endswith("Z")
    datetime.strptime(fa, "%Y-%m-%dT%H:%M:%SZ")  # round-trip parseable

    msg.term.assert_awaited_once()


def test_deadletter_last_error_omitted_when_empty():
    """Smackerel-Last-Error MUST be omitted when str(exc) == "" — parity
    with Go `if lastError != ""`."""
    client = NATSClient("nats://localhost:4222")
    client._consumer_max_deliver = 3
    mock_js = MagicMock()
    mock_js.publish = AsyncMock()
    client._js = mock_js

    msg = _make_msg(num_delivered=3)
    asyncio.run(client._handle_poison("artifacts.process", msg, Exception("")))

    headers = mock_js.publish.call_args.kwargs["headers"]
    assert "Smackerel-Last-Error" not in headers


def test_deadletter_original_consumer_falls_back_when_metadata_empty():
    """When md.consumer is empty, fall back to the durable name format
    so the header is still emitted (defensive — Go's md.Consumer is
    always set server-side)."""
    client = NATSClient("nats://localhost:4222")
    client._consumer_max_deliver = 3
    mock_js = MagicMock()
    mock_js.publish = AsyncMock()
    client._js = mock_js

    msg = _make_msg(num_delivered=3, consumer="")
    asyncio.run(client._handle_poison("search.embed", msg, RuntimeError("x")))

    headers = mock_js.publish.call_args.kwargs["headers"]
    assert headers["Smackerel-Original-Consumer"] == "smackerel-ml-search-embed"
    assert headers["Smackerel-Original-Stream"] == "SEARCH"


def test_subject_to_stream_covers_every_subscribe_subject():
    """T-081-U3 / D01-13 — table parity with internal/nats/client.go."""
    missing = [s for s in SUBSCRIBE_SUBJECTS if s not in SUBJECT_TO_STREAM]
    assert not missing, f"SUBJECT_TO_STREAM missing entries: {missing}"


# ---------------------------------------------------------------------------
# T-081-U4 — publish-before-term invariant
# ---------------------------------------------------------------------------


def test_deadletter_publish_failure_results_in_nak_not_term():
    """If _js.publish raises, _handle_poison MUST nak() and MUST NOT term()
    so JetStream redelivers and forensic evidence is preserved (design §4
    invariant 1)."""
    client = NATSClient("nats://localhost:4222")
    client._consumer_max_deliver = 3
    mock_js = MagicMock()
    mock_js.publish = AsyncMock(side_effect=RuntimeError("dl down"))
    client._js = mock_js

    msg = _make_msg(num_delivered=3)
    asyncio.run(client._handle_poison("artifacts.process", msg, RuntimeError("boom")))

    msg.nak.assert_awaited_once()
    msg.term.assert_not_awaited()


def test_below_max_deliver_naks_without_publishing():
    """If num_delivered < max_deliver, nak() — no dead-letter publish."""
    client = NATSClient("nats://localhost:4222")
    client._consumer_max_deliver = 5
    mock_js = MagicMock()
    mock_js.publish = AsyncMock()
    client._js = mock_js

    msg = _make_msg(num_delivered=2)
    asyncio.run(client._handle_poison("artifacts.process", msg, RuntimeError("boom")))

    msg.nak.assert_awaited_once()
    msg.term.assert_not_awaited()
    mock_js.publish.assert_not_awaited()


# ---------------------------------------------------------------------------
# T-081-U5 — _failure_counts attribute removed
# ---------------------------------------------------------------------------


def test_failure_counts_attribute_removed():
    """Spec 081 FR-081-004 — no _failure_counts attribute on NATSClient,
    no references anywhere in the class source."""
    client = NATSClient("nats://localhost:4222")
    assert not hasattr(client, "_failure_counts")
    assert not hasattr(NATSClient, "_failure_counts")
    src = inspect.getsource(NATSClient)
    assert src.count("_failure_counts") == 0, "_failure_counts must not appear anywhere in NATSClient source"
