"""Spec 096 SCOPE-03 — SCN-096-G05 secret-scrub tests (ADVERSARIAL).

Provider secrets never leak through the dispatch seam. ``_dispatch_live`` MUST
NOT log the ``api_key``, and the error ``detail`` that crosses the wire is built
from the exception TYPE + provider + model with any ``api_key`` substring
scrubbed (litellm exceptions can embed the key in a URL/header). These tests
assert the cleartext key appears in NO log line and NO HTTP error body.

The scrub is the load-bearing control: a build that folded the raw exception
text into the error/log (the pre-096 ``f"{type(e).__name__}: {e}"`` shape) would
leak the key and FAIL these tests.

UNIT tests — ``litellm.acompletion`` is patched to raise an exception that
embeds the key, without a live network call.
"""

from __future__ import annotations

import asyncio
import logging
import sys
import types

import pytest
from fastapi import HTTPException

from app.routes.chat import _dispatch_live
from app.schemas import ChatMessage, ChatRequest, Role


def _install_fake_litellm(monkeypatch, acompletion_impl) -> None:
    """Inject a fake ``litellm`` + ``litellm.exceptions`` into sys.modules so the
    dispatch code's lazy imports resolve WITHOUT the heavy real litellm (the
    [dev] test env does not install it — the repo's other LLM tests mock it the
    same way via sys.modules)."""
    fake = types.ModuleType("litellm")
    fake.acompletion = acompletion_impl  # type: ignore[attr-defined]
    exc = types.ModuleType("litellm.exceptions")
    for name in ("APIConnectionError", "APIError", "ServiceUnavailableError", "Timeout"):
        setattr(exc, name, type(name, (Exception,), {}))
    fake.exceptions = exc  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "litellm", fake)
    monkeypatch.setitem(sys.modules, "litellm.exceptions", exc)


def _hosted_request(secret: str) -> ChatRequest:
    return ChatRequest(
        model="claude-3-5-sonnet",
        messages=[ChatMessage(role=Role.USER, content="hi")],
        provider="anthropic",
        api_base="https://api.anthropic.test",
        api_key=secret,
    )


def test_error_detail_scrubs_api_key_substring(monkeypatch) -> None:
    """ADVERSARIAL — a litellm exception that embeds the key in a URL is
    scrubbed: the wire ``detail`` does NOT contain the cleartext key and shows a
    redaction marker where it was.
    """
    secret = "sk-SECRET-anthropic-096-url"  # gitleaks:allow

    async def _raise_with_key(**kwargs):
        # litellm commonly surfaces the key inside a request URL/header.
        raise RuntimeError(f"401 Unauthorized for url https://api.anthropic.test/v1/messages?api_key={secret}")

    _install_fake_litellm(monkeypatch, _raise_with_key)

    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(_dispatch_live(_hosted_request(secret)))

    detail_str = str(exc_info.value.detail)
    assert secret not in detail_str, f"cleartext api_key leaked into the wire error detail: {detail_str}"
    assert "***" in detail_str, "the scrub redaction marker MUST replace the key substring"


def test_api_key_never_logged(monkeypatch, caplog) -> None:
    """ADVERSARIAL — no log line emitted while dispatching contains the
    cleartext ``api_key``. Fails if the key appears in any captured log record.
    """
    secret = "sk-SECRET-anthropic-096-logline"

    async def _raise_with_key(**kwargs):
        raise RuntimeError(f"connection failed presenting key {secret} in the Authorization header")

    _install_fake_litellm(monkeypatch, _raise_with_key)

    caplog.set_level(logging.DEBUG)
    with pytest.raises(HTTPException):
        asyncio.run(_dispatch_live(_hosted_request(secret)))

    assert secret not in caplog.text, "cleartext api_key leaked into a captured log line"
    for record in caplog.records:
        assert secret not in record.getMessage(), f"api_key leaked into log record: {record.getMessage()}"
