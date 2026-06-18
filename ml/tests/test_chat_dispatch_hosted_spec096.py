"""Spec 096 SCOPE-03 — SCN-096-A02 / SCN-096-G01 hosted-dispatch tests.

The provider-aware ``_dispatch_live`` hosted branch composes the litellm
provider model ``<prefix>/<backend-id>`` and routes with the per-request
cleartext ``api_key`` the Go core decrypted; the Python ``ChatRequest`` carries
the four new fields additively while STAYING ``extra="forbid"``; and a hosted
dispatch with a missing required ``api_key`` fails loud with a typed
``llm_misconfigured`` and NEVER substitutes the local Ollama model.

UNIT tests — ``litellm.acompletion`` is patched to capture/observe without a
live network call.
"""

from __future__ import annotations

import asyncio
import sys
import types
from unittest.mock import MagicMock

import pytest
from fastapi import HTTPException
from pydantic import ValidationError

from app.routes.chat import _dispatch_live
from app.schemas import ChatMessage, ChatRequest, Role


def _fake_response() -> MagicMock:
    resp = MagicMock()
    resp.choices = [MagicMock(message=MagicMock(content="hosted answer", tool_calls=None))]
    resp.usage = MagicMock(total_tokens=11)
    return resp


def _install_fake_litellm(monkeypatch, acompletion_impl) -> None:
    """Inject a fake ``litellm`` + ``litellm.exceptions`` into sys.modules so the
    dispatch code's lazy ``import litellm`` / ``from litellm.exceptions import
    ...`` resolve WITHOUT the heavy real litellm (the [dev] test env does not
    install it — the repo's other LLM tests mock it the same way)."""
    fake = types.ModuleType("litellm")
    fake.acompletion = acompletion_impl  # type: ignore[attr-defined]
    exc = types.ModuleType("litellm.exceptions")
    for name in ("APIConnectionError", "APIError", "ServiceUnavailableError", "Timeout"):
        setattr(exc, name, type(name, (Exception,), {}))
    fake.exceptions = exc  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "litellm", fake)
    monkeypatch.setitem(sys.modules, "litellm.exceptions", exc)


def _capture_acompletion(monkeypatch) -> dict:
    captured: dict = {}

    async def _capture(**kwargs):
        captured["kwargs"] = kwargs
        return _fake_response()

    _install_fake_litellm(monkeypatch, _capture)
    return captured


def test_dispatch_live_hosted_composes_model_and_api_key(monkeypatch) -> None:
    """The hosted branch composes ``<prefix>/<backend-id>`` + ``api_base`` +
    ``api_key`` + the non-secret ``provider_params`` and routes to the selected
    hosted model.
    """
    captured = _capture_acompletion(monkeypatch)
    req = ChatRequest(
        model="claude-3-5-sonnet",  # BARE backend id
        messages=[ChatMessage(role=Role.USER, content="What is the capital of France?")],
        provider="anthropic",
        api_base="https://api.anthropic.test",
        api_key="sk-hosted-synthetic-096",
        provider_params={"organization": "acme"},
    )
    resp = asyncio.run(_dispatch_live(req))
    assert resp.stop_reason.value == "end_turn"

    kw = captured["kwargs"]
    assert kw["model"] == "anthropic/claude-3-5-sonnet", "hosted model MUST be provider-qualified for litellm"
    assert kw["api_key"] == "sk-hosted-synthetic-096", "the per-request cleartext key MUST be routed"
    assert kw["api_base"] == "https://api.anthropic.test"
    assert kw["organization"] == "acme", "non-secret provider_params MUST be carried to litellm"
    # The Ollama-only ollama_chat/ prefix MUST NOT appear on a hosted dispatch.
    assert not kw["model"].startswith("ollama_chat/")


def test_chatrequest_extra_forbid_still_holds() -> None:
    """ADVERSARIAL — the four new fields validate, but ``extra='forbid'`` is
    intact: an UNDECLARED field is still rejected (422 / ValidationError).
    """
    # The four spec-096 fields are accepted.
    req = ChatRequest(
        model="claude-3-5-sonnet",
        messages=[ChatMessage(role=Role.USER, content="hi")],
        provider="anthropic",
        api_base="https://api.anthropic.test",
        api_key="sk-synthetic-096",  # gitleaks:allow
        provider_params={"organization": "acme"},
    )
    assert req.provider == "anthropic"
    assert req.api_key == "sk-synthetic-096"  # gitleaks:allow

    # An undeclared field MUST still be rejected — extra=forbid did not loosen.
    with pytest.raises(ValidationError):
        ChatRequest(
            model="claude-3-5-sonnet",
            messages=[ChatMessage(role=Role.USER, content="hi")],
            totally_undeclared_field="boom",  # type: ignore[call-arg]
        )


def test_hosted_missing_api_key_typed_error_no_ollama_substitution(monkeypatch) -> None:
    """ADVERSARIAL — a hosted dispatch with an absent required ``api_key``
    returns a typed ``llm_misconfigured`` and NEVER substitutes Ollama. The
    patched ``acompletion`` fails the test if it is EVER called (a fallback to a
    local model would call it), so this proves no silent re-route happens.
    """
    called = {"n": 0}

    async def _must_not_call(**kwargs):  # pragma: no cover — asserted not-called
        called["n"] += 1
        return _fake_response()

    _install_fake_litellm(monkeypatch, _must_not_call)

    req = ChatRequest(
        model="claude-3-5-sonnet",
        messages=[ChatMessage(role=Role.USER, content="hi")],
        provider="anthropic",  # NO api_key supplied
    )
    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(_dispatch_live(req))

    detail = exc_info.value.detail
    assert isinstance(detail, dict), f"error detail MUST be the typed envelope, got {detail!r}"
    assert detail["error"] == "llm_misconfigured", f"want typed llm_misconfigured, got {detail!r}"
    assert called["n"] == 0, "missing-key hosted dispatch MUST NOT call litellm (no Ollama substitution)"
