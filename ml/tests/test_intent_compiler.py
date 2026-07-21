"""BUG-069-005 compiler route contract tests."""

from __future__ import annotations

import asyncio
import json

import httpx
import pytest
from fastapi import HTTPException

from app.routes.intent_compile import CompileRequest, compile_intent


def request_payload() -> CompileRequest:
    return CompileRequest.model_validate(
        {
            "schema_version": "v1",
            "model_role": "assistant_intent_compiler",
            "prompt_contract_version": "intent-compiler-v1",
            "raw_turn": {
                "user_id": "test-bug069005-user",
                "transport": "web",
                "transport_message_id": "test-bug069005-turn",
                "text": "what is the weather in Springfield",
            },
            "conversation_context": [],
            "response_schema": "compiled-intent-v1",
            "max_output_bytes": 4096,
        }
    )


def compiled_payload() -> dict[str, object]:
    return {
        "version": "v1",
        "language": "en",
        "user_goal": "check Springfield weather",
        "action_class": "clarify",
        "side_effect_class": "none",
        "scenario_hint": "weather_query",
        "tool_hints": ["location_normalize"],
        "normalized_request": {"query": "Springfield weather"},
        "slots": {"location": {"raw": "Springfield"}},
        "missing_slots": ["location"],
        "confidence": 0.99,
        "clarification_prompt": "Which Springfield did you mean?",
        "safety_flags": [],
        "source_policy": {"requires_citations": True, "allowed_source_kinds": ["tool"]},
    }


def configure(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("ASSISTANT_INTENT_COMPILER_MODEL_ROLE", "assistant_intent_compiler")
    monkeypatch.setenv("ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION", "intent-compiler-v1")
    monkeypatch.setenv("ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION", "v1")
    monkeypatch.setenv("ASSISTANT_INTENT_COMPILER_PROVIDER_NAME", "deterministic-e2e")
    monkeypatch.setenv("ASSISTANT_INTENT_COMPILER_PROVIDER_URL", "http://intent-provider:8082")
    monkeypatch.setenv("ASSISTANT_INTENT_COMPILER_TIMEOUT_MS", "5000")
    monkeypatch.setenv("LLM_MODEL", "intent-fixture-v1")


def install_provider(monkeypatch: pytest.MonkeyPatch, content: str, captured: list[dict[str, object]]) -> None:
    async def handler(request: httpx.Request) -> httpx.Response:
        payload = json.loads(request.content)
        captured.append(payload)
        return httpx.Response(200, json={"message": {"role": "assistant", "content": content}})

    transport = httpx.MockTransport(handler)
    real_client = httpx.AsyncClient

    def client_factory(*_args: object, **_kwargs: object) -> httpx.AsyncClient:
        return real_client(transport=transport)

    monkeypatch.setattr(httpx, "AsyncClient", client_factory)


def test_intent_compile_route_returns_schema_bound_fixture(monkeypatch: pytest.MonkeyPatch) -> None:
    configure(monkeypatch)
    captured: list[dict[str, object]] = []
    install_provider(monkeypatch, json.dumps(compiled_payload()), captured)

    response = asyncio.run(compile_intent(request_payload()))

    assert response.provider == "deterministic-e2e"
    assert response.compiled_intent.action_class == "clarify"
    assert response.compiled_intent.slots["location"] == {"raw": "Springfield"}
    assert len(captured) == 1
    assert "tools" not in captured[0]
    assert captured[0]["stream"] is False
    assert captured[0]["format"]["properties"]["action_class"]["enum"] == [
        "answer",
        "retrieve",
        "external_lookup",
        "internal_action",
        "state_mutation",
        "clarify",
        "capture_only",
        "refuse",
    ]
    provider_prompt = json.loads(captured[0]["messages"][-1]["content"])
    assert provider_prompt["raw_turn"] == {
        "text": "what is the weather in Springfield",
        "transport": "web",
    }
    assert "user_id" not in provider_prompt["raw_turn"]
    assert "transport_message_id" not in provider_prompt["raw_turn"]


def test_intent_compile_route_rejects_provider_schema_drift(monkeypatch: pytest.MonkeyPatch) -> None:
    configure(monkeypatch)
    captured: list[dict[str, object]] = []
    malformed = compiled_payload()
    malformed["action_class"] = "invented_action"
    install_provider(monkeypatch, json.dumps(malformed), captured)

    with pytest.raises(HTTPException) as exc_info:
        asyncio.run(compile_intent(request_payload()))

    assert exc_info.value.status_code == 502
    assert exc_info.value.detail == {"error": "intent_provider_schema_invalid"}
    assert len(captured) == 1
