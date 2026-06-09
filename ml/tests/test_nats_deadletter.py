"""SEC-081-R1 / BUG-081-001 — dead-letter Smackerel-Last-Error CR/LF sanitization
(Python sidecar parity mirror).

Proves the Python `_handle_poison` dead-letter builder CR/LF/C0/DEL-sanitizes the
Smackerel-Last-Error value before writing it into a single-line wire header, so an
untrusted `str(exc)` cannot inject an extra header line (CWE-113 header injection).

CROSS-RUNTIME PARITY PIN: the shared input and expected sanitized output below are
pinned IDENTICALLY by the Go tests so the Go core and the Python sidecar produce
byte-for-byte equal dead-letter values (spec 081 parity invariant):
  - internal/stringutil/stringutil_test.go::TestSanitizeHeaderValue
  - internal/pipeline/subscriber_test.go::TestDeadLetterLastErrorCRLFSanitized

Tests use asyncio.run() rather than pytest-asyncio (matches the project's existing
test style — see ml/tests/test_nats_consumer_config.py).
"""

from __future__ import annotations

import asyncio
import sys
import types
from unittest.mock import AsyncMock, MagicMock

# Mock litellm before importing app modules (same pattern as
# test_nats_consumer_config.py / test_nats_client.py).
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

from app.nats_client import (  # noqa: E402
    NATSClient,
    _sanitize_header_value,
    _utf8_truncate,
)

# CROSS-RUNTIME PARITY PIN (SEC-081-R1 / BUG-081-001): this exact input/output pair
# is asserted IDENTICALLY by the Go tests named in this module's docstring. Two
# control bytes (CR, LF) each collapse to a single space, so byte length is
# preserved and the 256-byte UTF-8 truncation boundary is identical on both
# runtimes.
CRLF_INPUT = "boom\r\nNats-Msg-Id: forged"
WANT_LAST_ERROR = "boom  Nats-Msg-Id: forged"

CANONICAL_HEADERS = {
    "Smackerel-Original-Subject",
    "Smackerel-Original-Stream",
    "Smackerel-Failed-At",
    "Smackerel-Delivery-Count",
    "Smackerel-Last-Error",
    "Smackerel-Original-Consumer",
}


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


def test_last_error_crlf_sanitized():
    """Adversarial RED->GREEN: a CR/LF-laden str(exc) routed through
    _handle_poison MUST yield exactly the canonical six-header envelope with a
    sanitized Smackerel-Last-Error (no CR/LF, no injected key). Without the
    _sanitize_header_value call at the sink the raw value survives and this fails.
    """
    client = NATSClient("nats://localhost:4222")
    client._consumer_max_deliver = 3
    mock_js = MagicMock()
    mock_js.publish = AsyncMock()
    client._js = mock_js

    msg = _make_msg(num_delivered=3)
    asyncio.run(client._handle_poison("artifacts.process", msg, RuntimeError(CRLF_INPUT)))

    mock_js.publish.assert_awaited_once()
    headers = mock_js.publish.call_args.kwargs["headers"]

    # Exactly the canonical six headers — never a seventh, injected line.
    assert set(headers.keys()) == CANONICAL_HEADERS, f"header set drifted: {set(headers.keys())}"
    # The forged header name must NOT appear as its own key.
    assert "Nats-Msg-Id" not in headers

    last_err = headers["Smackerel-Last-Error"]
    # RED->GREEN discriminator: no CR/LF survives; value equals the parity pin.
    assert "\r" not in last_err and "\n" not in last_err, repr(last_err)
    assert last_err == WANT_LAST_ERROR

    # Publish succeeded (mock) so the poison path terminates the message.
    msg.term.assert_awaited_once()


def test_sanitize_header_value_parity_pin():
    """Helper-level cross-runtime parity pin: sanitize-then-truncate of the shared
    CR/LF input equals the exact value the Go SanitizeHeaderValue rule produces
    (asserted identically in internal/stringutil/stringutil_test.go and
    internal/pipeline/subscriber_test.go). This is the byte-for-byte contract that
    keeps the Go core and Python sidecar dead-letter values equal.
    """
    out = _utf8_truncate(_sanitize_header_value(CRLF_INPUT), 256)
    assert out == WANT_LAST_ERROR
    # Byte length preserved (control byte -> space byte) — identical truncation
    # boundary on both runtimes.
    assert len(out.encode("utf-8")) == len(CRLF_INPUT.encode("utf-8"))


def test_sanitize_rule_matches_go_byte_oriented_rule():
    """Codepoint-vs-byte parity: every char < 0x20 or == 0x7F is replaced by a
    single space; all other codepoints (incl. multi-byte UTF-8) pass through —
    the same set of positions the byte-oriented Go rule replaces.
    """
    assert _sanitize_header_value("a\tb\nc\rd") == "a b c d"  # TAB/LF/CR all -> space
    assert _sanitize_header_value("x\x00\x1f\x7fy") == "x   y"  # NUL, US, DEL -> space
    assert _sanitize_header_value("café\x00naïve") == "café naïve"  # multi-byte preserved
    assert _sanitize_header_value("clean") == "clean"  # fast path, unchanged
    assert _sanitize_header_value("\x85nel") == "\x85nel"  # C1 (U+0085) NOT in C0/DEL set


def test_sanitize_then_truncate_preserves_256_byte_invariant():
    """The 256-byte UTF-8 truncation invariant still holds after sanitization
    (parity with Go TestSanitizeHeaderValue_TruncationInvariant)."""
    raw = "\r\n" * 150  # 300 bytes of CR/LF
    out = _utf8_truncate(_sanitize_header_value(raw), 256)
    assert out == " " * 256
    assert len(out.encode("utf-8")) == 256
    assert "\r" not in out and "\n" not in out
