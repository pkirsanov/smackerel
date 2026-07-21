"""Schema-bound assistant intent compiler route."""

from __future__ import annotations

import json
import os
import time
from typing import Any, Literal

import httpx
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, ConfigDict, Field, ValidationError

router = APIRouter(prefix="/assistant/intent", tags=["assistant_intent"])


class CompileTurn(BaseModel):
    model_config = ConfigDict(extra="forbid")

    user_id: str = Field(min_length=1)
    transport: str = Field(min_length=1)
    transport_message_id: str
    text: str = Field(min_length=1)


class CompileContextTurn(BaseModel):
    model_config = ConfigDict(extra="forbid")

    role: Literal["user", "assistant"]
    text: str


class CompileRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    schema_version: str = Field(min_length=1)
    model_role: str = Field(min_length=1)
    prompt_contract_version: str = Field(min_length=1)
    raw_turn: CompileTurn
    conversation_context: list[CompileContextTurn]
    response_schema: str = Field(min_length=1)
    max_output_bytes: int = Field(gt=0)


class SourcePolicy(BaseModel):
    model_config = ConfigDict(extra="forbid")

    requires_citations: bool
    allowed_source_kinds: list[str]


class CompiledIntent(BaseModel):
    model_config = ConfigDict(extra="forbid")

    version: str = Field(min_length=1)
    language: str = Field(min_length=1)
    user_goal: str = Field(min_length=1)
    action_class: Literal[
        "answer",
        "retrieve",
        "external_lookup",
        "internal_action",
        "state_mutation",
        "clarify",
        "capture_only",
        "refuse",
    ]
    side_effect_class: Literal["none", "read", "write", "external_read", "external_write"]
    scenario_hint: str | None
    tool_hints: list[str]
    normalized_request: dict[str, Any]
    slots: dict[str, Any]
    missing_slots: list[str]
    confidence: float = Field(ge=0, le=1)
    clarification_prompt: str | None
    safety_flags: list[str]
    source_policy: SourcePolicy


class CompileResponse(BaseModel):
    schema_version: str
    compiled_intent: CompiledIntent
    provider: str
    model: str
    latency_ms: int


def _required(name: str) -> str:
    try:
        value = os.environ[name].strip()
    except KeyError as exc:
        raise HTTPException(
            status_code=500,
            detail={"error": "intent_compiler_misconfigured", "key": name},
        ) from exc
    if not value:
        raise HTTPException(status_code=500, detail={"error": "intent_compiler_misconfigured", "key": name})
    return value


def _provider_request(req: CompileRequest, model: str) -> dict[str, Any]:
    system = (
        "Return only one JSON object matching the requested compiled-intent schema. "
        "Do not execute tools or perform writes."
    )
    user = json.dumps(
        {
            "prompt_contract_version": req.prompt_contract_version,
            "response_schema": req.response_schema,
            "raw_turn": {
                "text": req.raw_turn.text,
                "transport": req.raw_turn.transport,
            },
            "conversation_context": [turn.model_dump() for turn in req.conversation_context],
        },
        separators=(",", ":"),
    )
    return {
        "model": model,
        "stream": False,
        "format": CompiledIntent.model_json_schema(),
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": user},
        ],
    }


@router.post("/compile", response_model=CompileResponse)
async def compile_intent(req: CompileRequest) -> CompileResponse:
    expected_role = _required("ASSISTANT_INTENT_COMPILER_MODEL_ROLE")
    expected_prompt = _required("ASSISTANT_INTENT_COMPILER_PROMPT_CONTRACT_VERSION")
    expected_schema = _required("ASSISTANT_INTENT_COMPILER_SCHEMA_VERSION")
    provider_name = _required("ASSISTANT_INTENT_COMPILER_PROVIDER_NAME")
    provider_url = _required("ASSISTANT_INTENT_COMPILER_PROVIDER_URL").rstrip("/")
    model = _required("LLM_MODEL")
    if req.model_role != expected_role:
        raise HTTPException(status_code=422, detail={"error": "model_role_mismatch"})
    if req.prompt_contract_version != expected_prompt:
        raise HTTPException(status_code=422, detail={"error": "prompt_contract_mismatch"})
    if req.schema_version != expected_schema or req.response_schema != f"compiled-intent-{expected_schema}":
        raise HTTPException(status_code=422, detail={"error": "schema_version_mismatch"})

    try:
        timeout_ms = int(_required("ASSISTANT_INTENT_COMPILER_TIMEOUT_MS"))
    except ValueError as exc:
        raise HTTPException(status_code=500, detail={"error": "intent_compiler_timeout_invalid"}) from exc
    started = time.perf_counter()
    try:
        async with httpx.AsyncClient(timeout=timeout_ms / 1000) as client:
            provider_response = await client.post(
                provider_url + "/api/chat",
                json=_provider_request(req, model),
            )
            provider_response.raise_for_status()
    except httpx.HTTPError as exc:
        raise HTTPException(status_code=502, detail={"error": "intent_provider_unavailable"}) from exc

    try:
        provider_payload = provider_response.json()
        content = provider_payload["message"]["content"]
        encoded = content.encode("utf-8")
        if len(encoded) > req.max_output_bytes:
            raise ValueError("provider output exceeds max_output_bytes")
        compiled = CompiledIntent.model_validate_json(content)
    except (KeyError, TypeError, ValueError, ValidationError, json.JSONDecodeError) as exc:
        raise HTTPException(status_code=502, detail={"error": "intent_provider_schema_invalid"}) from exc

    return CompileResponse(
        schema_version=req.schema_version,
        compiled_intent=compiled,
        provider=provider_name,
        model=model,
        latency_ms=int((time.perf_counter() - started) * 1000),
    )
