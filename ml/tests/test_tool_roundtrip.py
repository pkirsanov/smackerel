"""Spec 064 SCOPE-04 — tool-use round-trip tests for the ML sidecar
LLM bridge plus the schema-parity fixture shared with the Go client.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.routes.chat import _MODE_FINAL_TEXT, _MODE_TOOL_USE, _TEST_MODE_HEADER, router
from app.schemas import (
    ChatMessage,
    ChatRequest,
    ChatResponse,
    Role,
    StopReason,
    Tool,
    ToolCall,
)

FIXTURE_PATH = (
    Path(__file__).resolve().parents[2]
    / "internal"
    / "assistant"
    / "openknowledge"
    / "llm"
    / "testdata"
    / "chat_fixture.json"
)


@pytest.fixture(scope="module")
def fixture() -> dict:
    return json.loads(FIXTURE_PATH.read_text())


@pytest.fixture(scope="module")
def client() -> TestClient:
    app = FastAPI()
    app.include_router(router)
    return TestClient(app)


def _request_payload(fixture: dict) -> dict:
    # Strip leading underscore comment keys before sending.
    return {k: v for k, v in fixture["request"].items() if not k.startswith("_")}


def test_fixture_request_decodes(fixture: dict) -> None:
    """Parity guard: the shared fixture's request half MUST decode
    cleanly through ChatRequest. The Go client_test.go round-trips the
    same JSON; any drift here will break the Go side too.
    """
    req = ChatRequest.model_validate(_request_payload(fixture))
    assert len(req.messages) == 4
    assert req.messages[2].role is Role.TOOL_CALL
    assert req.messages[3].role is Role.TOOL_RESULT
    assert req.tools and req.tools[0].name == "unit_convert"


def test_fixture_responses_decode(fixture: dict) -> None:
    end_turn = ChatResponse.model_validate(fixture["response_end_turn"])
    assert end_turn.stop_reason is StopReason.END_TURN
    assert end_turn.text == "5 km is about 3.11 miles."

    tool_use = ChatResponse.model_validate(fixture["response_tool_use"])
    assert tool_use.stop_reason is StopReason.TOOL_USE
    assert tool_use.tool_calls and tool_use.tool_calls[0].id == "call-2"


def test_route_returns_final_text(client: TestClient, fixture: dict) -> None:
    resp = client.post(
        "/llm/chat",
        json=_request_payload(fixture),
        headers={_TEST_MODE_HEADER: _MODE_FINAL_TEXT},
    )
    assert resp.status_code == 200, resp.text
    body = ChatResponse.model_validate(resp.json())
    assert body.stop_reason is StopReason.END_TURN
    # last substantive message is the tool_result content "3.10686".
    assert body.text == "echo:3.10686"


def test_route_returns_tool_use(client: TestClient, fixture: dict) -> None:
    resp = client.post(
        "/llm/chat",
        json=_request_payload(fixture),
        headers={_TEST_MODE_HEADER: _MODE_TOOL_USE},
    )
    assert resp.status_code == 200, resp.text
    body = ChatResponse.model_validate(resp.json())
    assert body.stop_reason is StopReason.TOOL_USE
    assert body.tool_calls and body.tool_calls[0].name == "unit_convert"
    assert body.tool_calls[0].id == f"call-{len(fixture['request']['messages'])}"


def test_route_rejects_unknown_role(client: TestClient, fixture: dict) -> None:
    payload = _request_payload(fixture)
    payload = json.loads(json.dumps(payload))  # deep copy
    payload["messages"].append({"role": "operator", "content": "noop"})
    resp = client.post(
        "/llm/chat",
        json=payload,
        headers={_TEST_MODE_HEADER: _MODE_FINAL_TEXT},
    )
    assert resp.status_code == 422
    assert "role" in resp.text.lower()


def test_route_rejects_malformed_tools(client: TestClient, fixture: dict) -> None:
    payload = json.loads(json.dumps(_request_payload(fixture)))
    payload["tools"][0]["parameters"] = {"type": "array"}  # not an object schema
    resp = client.post(
        "/llm/chat",
        json=payload,
        headers={_TEST_MODE_HEADER: _MODE_TOOL_USE},
    )
    assert resp.status_code == 422


def test_route_no_test_mode_header_dispatches_live(
    client: TestClient, fixture: dict, monkeypatch: pytest.MonkeyPatch
) -> None:
    """Without the fixture header the route MUST attempt a live LLM
    dispatch. With OLLAMA_URL unset we expect a 500 ``llm_misconfigured``
    error (G028 fail-loud). The route MUST NOT silently fall back to a
    fixture mode and MUST NOT return the obsolete 501.
    """
    monkeypatch.delenv("OLLAMA_URL", raising=False)
    resp = client.post("/llm/chat", json=_request_payload(fixture))
    assert resp.status_code == 500, resp.text
    body = resp.json()
    assert body["detail"]["error"] == "llm_misconfigured"


# --- Adversarial schema cases (G021) ---------------------------------


def test_tool_call_requires_id() -> None:
    """G021 anti-fabrication: a tool_call without an id MUST be rejected
    by the schema, not silently passed through as a fabricated call.
    """
    with pytest.raises(ValidationError):
        ChatMessage(
            role=Role.TOOL_CALL,
            tool_calls=[ToolCall.model_construct(id="", name="x", arguments={})],
        )


def test_tool_response_id_round_trips(fixture: dict) -> None:
    tu = ChatResponse.model_validate(fixture["response_tool_use"])
    encoded = tu.model_dump(mode="json")
    decoded = ChatResponse.model_validate(encoded)
    assert decoded.tool_calls[0].id == tu.tool_calls[0].id


def test_followup_tool_result_request_decodes(fixture: dict) -> None:
    """Case (c): a follow-up request carrying tool_result followed by an
    assistant final-text triggers end_turn shape end-to-end.
    """
    payload = json.loads(json.dumps(_request_payload(fixture)))
    payload["messages"].append({"role": "assistant", "content": "Answer ready."})
    req = ChatRequest.model_validate(payload)
    assert req.messages[-1].role is Role.ASSISTANT


def test_tool_parameters_must_be_object() -> None:
    with pytest.raises(ValidationError):
        Tool(name="t", description="d", parameters={"type": "string"})
