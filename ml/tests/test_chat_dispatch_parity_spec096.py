"""Spec 096 SCOPE-03 — SCN-096-A03 byte-for-byte Ollama parity (ADVERSARIAL).

The redesigned provider-aware ``_dispatch_live`` MUST leave the no-override /
Ollama-only path byte-for-byte identical to the pre-096 code: the
``litellm.acompletion(**kwargs)`` dict for a fixed Ollama request is unchanged
and NO provider field (``api_key`` / ``provider`` / ``provider_params``) leaks
into it (the spec 088 ``IsZero()`` baseline guarantee).

These are UNIT tests: ``litellm.acompletion`` is patched to CAPTURE the kwargs
without a live network call — we assert the COMPOSED arguments the real
``_dispatch_live`` code path produces, not a mocked response. There is no
request interception of a live call (correctly classified ``unit`` per the
scopes.md live-stack note).
"""

from __future__ import annotations

import asyncio
import sys
import types
from unittest.mock import MagicMock

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
    """The EXACT pre-096 _dispatch_live kwargs shape for a fixed Ollama
    request."""
    return {
        "model": "ollama_chat/gemma3:4b",
        "api_base": OLLAMA_URL,
        "messages": _translate_messages(req.messages),
        "timeout": _LIVE_DISPATCH_TIMEOUT_SECONDS,
        "temperature": 0.0,
        "tools": _translate_tools(req.tools),
        "max_tokens": 256,
    }


def test_dispatch_live_ollama_kwargs_byte_for_byte(monkeypatch) -> None:
    """ADVERSARIAL — for a fixed Ollama request the captured
    ``litellm.acompletion`` kwargs equal the pre-096 byte-for-byte shape, on
    BOTH the spec-064 env-driven caller (no provider/api_base) and the explicit
    spec-096 ``provider='ollama'`` + ``api_base`` caller. Fails if any provider
    field leaks in or any existing key changes.
    """
    # Case 1 — the spec 064 caller shape: NO provider, NO api_base; OLLAMA_URL
    # rides the environment exactly as before.
    monkeypatch.setenv("OLLAMA_URL", OLLAMA_URL)
    captured = _capture_acompletion(monkeypatch)
    req = _fixed_ollama_request()
    resp = asyncio.run(_dispatch_live(req))
    assert resp.stop_reason.value == "end_turn"
    assert captured["kwargs"] == _expected_ollama_kwargs(req), (
        "Ollama dispatch kwargs drifted from the pre-096 byte-for-byte shape"
    )
    for leaked in ("api_key", "provider", "provider_params"):
        assert leaked not in captured["kwargs"], f"provider field {leaked!r} leaked into the Ollama path"

    # Case 2 — the explicit spec-096 Ollama caller: provider='ollama',
    # api_base carries the SAME value with NO OLLAMA_URL in the env. The
    # composed kwargs MUST be byte-for-byte identical to case 1.
    monkeypatch.delenv("OLLAMA_URL", raising=False)
    captured2 = _capture_acompletion(monkeypatch)
    req2 = _fixed_ollama_request(provider="ollama", api_base=OLLAMA_URL)
    asyncio.run(_dispatch_live(req2))
    assert captured2["kwargs"] == _expected_ollama_kwargs(req2), (
        "explicit provider='ollama' + api_base diverged from the env-driven path"
    )
    assert "api_key" not in captured2["kwargs"]


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
