"""Spec 096 parity plus Spec 102 profiled Ollama dispatch (ADVERSARIAL).

The provider-aware ``_dispatch_live`` retains every pre-096 caller-owned field
and never leaks provider credentials. Spec 102 deliberately adds the selected
SST profile's ``options.num_ctx`` and top-level ``keep_alive``.

These are UNIT tests: ``litellm.acompletion`` is patched to CAPTURE the kwargs
without a live network call — we assert the COMPOSED arguments the real
``_dispatch_live`` code path produces, not a mocked response. There is no
request interception of a live call (correctly classified ``unit`` per the
scopes.md live-stack note).
"""

from __future__ import annotations

import asyncio
import logging
import sys
import types
from unittest.mock import MagicMock

import pytest
from fastapi import HTTPException

from app.routes.chat import (
    _LIVE_DISPATCH_TIMEOUT_SECONDS,
    _dispatch_live,
    _translate_messages,
    _translate_tools,
)
from app.schemas import ChatMessage, ChatRequest, Role, Tool

OLLAMA_URL = "http://ollama.test:11434"


def _fake_response() -> MagicMock:
    """A minimal litellm response object for the shared response builder
    (end_turn, no tool calls)."""
    resp = MagicMock()
    resp.choices = [MagicMock(message=MagicMock(content="ok", tool_calls=None))]
    resp.usage = MagicMock(total_tokens=7)
    return resp


def _install_fake_litellm(monkeypatch, acompletion_impl) -> None:
    """Inject a fake ``litellm`` + ``litellm.exceptions`` into sys.modules so the
    dispatch code's lazy ``import litellm`` / ``from litellm.exceptions import
    ...`` resolve WITHOUT the heavy real litellm — which the [dev] test env
    deliberately does not install (the repo's other LLM tests mock it the same
    way via sys.modules)."""
    fake = types.ModuleType("litellm")
    fake.acompletion = acompletion_impl  # type: ignore[attr-defined]
    exc = types.ModuleType("litellm.exceptions")
    for name in ("APIConnectionError", "APIError", "ServiceUnavailableError", "Timeout"):
        setattr(exc, name, type(name, (Exception,), {}))
    fake.exceptions = exc  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "litellm", fake)
    monkeypatch.setitem(sys.modules, "litellm.exceptions", exc)


def _capture_acompletion(monkeypatch) -> dict:
    """Install a fake litellm whose acompletion captures its kwargs (no live
    call)."""
    captured: dict = {}

    async def _capture(**kwargs):
        captured["kwargs"] = kwargs
        return _fake_response()

    _install_fake_litellm(monkeypatch, _capture)
    return captured


def _fixed_ollama_request(**overrides) -> ChatRequest:
    base: dict = dict(
        model="gemma3:4b",
        messages=[
            ChatMessage(role=Role.SYSTEM, content="You are the planner."),
            ChatMessage(role=Role.USER, content="Convert 5 km to miles."),
        ],
        tools=[
            Tool(
                name="unit_convert",
                description="Convert a value between two units.",
                parameters={
                    "type": "object",
                    "properties": {"value": {"type": "number"}},
                },
            )
        ],
        max_tokens=256,
        temperature=0.0,
    )
    base.update(overrides)
    return ChatRequest(**base)


def _expected_ollama_kwargs(req: ChatRequest) -> dict:
    """The pre-096 fields plus the mandatory Spec-102 profile fields."""
    return {
        "model": "ollama_chat/gemma3:4b",
        "api_base": OLLAMA_URL,
        "messages": _translate_messages(req.messages),
        "timeout": _LIVE_DISPATCH_TIMEOUT_SECONDS,
        "temperature": 0.0,
        "tools": _translate_tools(req.tools),
        "max_tokens": 256,
        "options": {"num_ctx": 8192},
        "keep_alive": "30m",
    }


def test_dispatch_ollama_applies_profile_spec102(monkeypatch) -> None:
    """TP-C3-11 — both typed caller forms preserve their payload while adding
    the same SST-selected num_ctx and top-level keep_alive.
    """
    # Case 1 — the spec 064 caller shape: NO provider, NO api_base; OLLAMA_URL
    # rides the environment exactly as before.
    monkeypatch.setenv("OLLAMA_URL", OLLAMA_URL)
    captured = _capture_acompletion(monkeypatch)
    req = _fixed_ollama_request()
    resp = asyncio.run(_dispatch_live(req))
    assert resp.stop_reason.value == "end_turn"
    assert captured["kwargs"] == _expected_ollama_kwargs(req)
    for leaked in ("api_key", "provider", "provider_params"):
        assert leaked not in captured["kwargs"], f"provider field {leaked!r} leaked into the Ollama path"

    # Case 2 — the explicit spec-096 Ollama caller: provider='ollama',
    # api_base carries the SAME value with NO OLLAMA_URL in the env. The
    # composed kwargs MUST remain identical to case 1.
    monkeypatch.delenv("OLLAMA_URL", raising=False)
    captured2 = _capture_acompletion(monkeypatch)
    req2 = _fixed_ollama_request(provider="ollama", api_base=OLLAMA_URL)
    asyncio.run(_dispatch_live(req2))
    assert captured2["kwargs"] == _expected_ollama_kwargs(req2)
    assert "api_key" not in captured2["kwargs"]


def test_hosted_dispatch_carries_no_ollama_profile_options_spec102(monkeypatch) -> None:
    """TP-C3-19: hosted dispatch never reads or emits Ollama-only fields."""
    monkeypatch.delenv("ML_MODEL_MEMORY_PROFILES_JSON", raising=False)
    monkeypatch.delenv("ML_OLLAMA_KEEP_ALIVE", raising=False)
    captured = _capture_acompletion(monkeypatch)
    req = _fixed_ollama_request(
        provider="openai",
        model="gpt-4o-mini",
        api_key="unit-test-provider-key",
    )

    response = asyncio.run(_dispatch_live(req))

    assert response.stop_reason.value == "end_turn"
    assert captured["kwargs"]["model"] == "openai/gpt-4o-mini"
    assert captured["kwargs"]["api_key"] == "unit-test-provider-key"
    assert "options" not in captured["kwargs"]
    assert "keep_alive" not in captured["kwargs"]


def test_ollama_branch_carries_no_api_key(monkeypatch) -> None:
    """ADVERSARIAL — even if a request mistakenly carries an ``api_key`` while
    the provider is ``ollama``, the Ollama branch MUST drop it: no ``api_key``
    ever reaches litellm on a local dispatch.
    """
    monkeypatch.setenv("OLLAMA_URL", OLLAMA_URL)
    captured = _capture_acompletion(monkeypatch)
    req = _fixed_ollama_request(provider="ollama", api_key="sk-should-be-ignored-096")

    asyncio.run(_dispatch_live(req))
    assert "api_key" not in captured["kwargs"], "an api_key leaked onto the Ollama dispatch path"
    assert captured["kwargs"]["model"] == "ollama_chat/gemma3:4b"


def test_ollama_profile_error_redacts_supplied_value_from_http_and_logs(monkeypatch, caplog) -> None:
    sentinel = "SENTINEL-CHAT-PROFILE-SECRET-RR03"
    monkeypatch.setenv("OLLAMA_URL", OLLAMA_URL)
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", sentinel)
    called = {"count": 0}

    async def capture(**kwargs):
        called["count"] += 1
        return _fake_response()

    _install_fake_litellm(monkeypatch, capture)
    with caplog.at_level(logging.ERROR):
        with pytest.raises(HTTPException) as exc_info:
            asyncio.run(_dispatch_live(_fixed_ollama_request()))

    assert exc_info.value.detail["error"] == "llm_misconfigured"
    assert sentinel not in str(exc_info.value.detail)
    assert sentinel not in caplog.text
    assert called["count"] == 0
