"""Spec 064 SCOPE-04 — LLM bridge tool-use round-trip schemas.

Pydantic models that mirror the Go types in
``internal/assistant/openknowledge/llm/client.go``. The schema parity
contract test (``ml/tests/test_tool_roundtrip.py`` ↔
``internal/assistant/openknowledge/llm/client_test.go``) round-trips a
shared JSON fixture through both sides; any drift here MUST be
matched in Go.

NO-DEFAULTS (Gate G028): these models intentionally do not provide
field defaults for budget, endpoint, or model identifiers. Optional
arguments (``temperature``, ``max_tokens``) are typed ``X | None`` and
left to the caller (the Go agent loop) to populate from SST config.
"""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, ConfigDict, Field, model_validator

_PROVIDER_PARAM_ALLOWLIST: dict[str, frozenset[str]] = {
    "anthropic": frozenset(),
    "openai": frozenset({"organization"}),
    "azure-foundry": frozenset({"api_version", "deployment"}),
    "google": frozenset({"project", "location"}),
    "bedrock": frozenset({"region"}),
}
_API_BASE_PROVIDERS = frozenset({"", "ollama", "openai", "azure-foundry"})


def validate_provider_dispatch_controls(
    provider: str | None,
    api_base: str | None,
    provider_params: dict[str, Any] | None,
) -> dict[str, Any]:
    """Return a copy of provider-owned routing params or fail before dispatch."""
    normalized_provider = (provider or "").strip().lower()
    if api_base is not None and normalized_provider not in _API_BASE_PROVIDERS:
        raise ValueError(
            f"provider={normalized_provider or 'ollama'} key=api_base is not owned by this provider contract"
        )

    params = dict(provider_params or {})
    if normalized_provider in ("", "ollama"):
        allowed = frozenset()
    else:
        allowed = _PROVIDER_PARAM_ALLOWLIST.get(normalized_provider, frozenset())
    unsupported = sorted(set(params) - allowed)
    if unsupported:
        raise ValueError(
            f"provider={normalized_provider or 'ollama'} contains unsupported provider_params keys: "
            + ", ".join(unsupported)
        )
    for key, value in params.items():
        if not isinstance(value, str) or not value.strip():
            raise ValueError(f"provider={normalized_provider or 'ollama'} key={key} must be a non-empty string")
    return params


class Role(str, Enum):
    SYSTEM = "system"
    USER = "user"
    ASSISTANT = "assistant"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"


class StopReason(str, Enum):
    END_TURN = "end_turn"
    TOOL_USE = "tool_use"


class Tool(BaseModel):
    """JSONSchema-described tool descriptor the planner may invoke."""

    model_config = ConfigDict(extra="forbid")

    name: str = Field(min_length=1)
    description: str = Field(min_length=1)
    parameters: dict[str, Any] = Field(description="JSONSchema object describing the tool's params.")

    @model_validator(mode="after")
    def _validate_parameters(self) -> Tool:
        if not isinstance(self.parameters, dict) or self.parameters.get("type") != "object":
            raise ValueError(f"tool {self.name!r}: parameters must be a JSONSchema object (type='object')")
        return self


class ToolCall(BaseModel):
    """Planner-issued request to invoke a registered tool."""

    model_config = ConfigDict(extra="forbid")

    id: str = Field(min_length=1, description="Stable id round-tripped in tool_result.")
    name: str = Field(min_length=1)
    arguments: dict[str, Any]


class ToolResult(BaseModel):
    """Agent-loop reply to a previous ToolCall."""

    model_config = ConfigDict(extra="forbid")

    tool_call_id: str = Field(min_length=1)
    content: str = Field(
        description=(
            "Canonicalised tool output text. Wrapped by the agent loop in "
            "a <tool_output id=...> envelope before being passed back to "
            "the planner (design §LLM bridge extension)."
        )
    )


class ChatMessage(BaseModel):
    """Single message in the planner conversation.

    Required combinations by role:
      - system | user                : ``content`` non-empty.
      - assistant                    : ``content`` non-empty OR ``tool_calls`` non-empty
        (matches OpenAI tool-use convention — an assistant message that issues
        tool calls has no text content). Spec 064 SCOPE-17 — relaxed from
        "content always required" so the open-knowledge agent can replay its
        own prior tool-use turn into the planner conversation.
      - tool_call                    : ``tool_calls`` non-empty.
      - tool_result                  : ``tool_call_id`` + ``content`` set.
    """

    model_config = ConfigDict(extra="forbid")

    role: Role
    content: str | None = None
    tool_calls: list[ToolCall] | None = None
    tool_call_id: str | None = None

    @model_validator(mode="after")
    def _validate_role_shape(self) -> ChatMessage:
        match self.role:
            case Role.SYSTEM | Role.USER:
                if not self.content:
                    raise ValueError(f"role={self.role.value!r} requires non-empty content")
                if self.tool_calls or self.tool_call_id:
                    raise ValueError(f"role={self.role.value!r} must not carry tool_calls or tool_call_id")
            case Role.ASSISTANT:
                if not self.content and not self.tool_calls:
                    raise ValueError("role='assistant' requires non-empty content or non-empty tool_calls")
                if self.tool_call_id:
                    raise ValueError("role='assistant' must not carry tool_call_id")
            case Role.TOOL_CALL:
                if not self.tool_calls:
                    raise ValueError("role='tool_call' requires non-empty tool_calls[]")
                for tc in self.tool_calls:
                    if not tc.id:
                        raise ValueError("tool_call entries require non-empty id")
            case Role.TOOL_RESULT:
                if not self.tool_call_id:
                    raise ValueError("role='tool_result' requires tool_call_id")
                if self.content is None:
                    raise ValueError("role='tool_result' requires content")
        return self


class ChatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    model: str = Field(min_length=1)
    messages: list[ChatMessage] = Field(min_length=1)
    tools: list[Tool] | None = None
    max_tokens: int | None = None
    temperature: float | None = None
    # Spec 096 SCOPE-03 — the per-request provider-credential seam (mirrors the
    # four additive fields on the Go llm.ChatRequest). All optional, so the spec
    # 064/088/089 no-override caller (which omits every one) still validates and
    # takes the byte-for-byte Ollama branch in _dispatch_live. extra="forbid"
    # STILL holds — an UNDECLARED field is rejected; these four are now declared.
    provider: str | None = None
    api_base: str | None = None
    api_key: str | None = None
    provider_params: dict[str, Any] | None = None

    @model_validator(mode="after")
    def _validate_provider_controls(self) -> ChatRequest:
        validate_provider_dispatch_controls(self.provider, self.api_base, self.provider_params)
        return self


class ChatResponse(BaseModel):
    """Sidecar reply.

    Exactly one of ``text`` (when ``stop_reason=end_turn``) or
    ``tool_calls`` (when ``stop_reason=tool_use``) MUST be populated.
    """

    model_config = ConfigDict(extra="forbid")

    stop_reason: StopReason
    text: str | None = None
    tool_calls: list[ToolCall] | None = None
    tokens_used: int = 0

    @model_validator(mode="after")
    def _validate_stop_shape(self) -> ChatResponse:
        if self.stop_reason is StopReason.END_TURN:
            if self.text is None:
                raise ValueError("stop_reason='end_turn' requires text")
            if self.tool_calls:
                raise ValueError("stop_reason='end_turn' must not carry tool_calls")
        else:
            if not self.tool_calls:
                raise ValueError("stop_reason='tool_use' requires non-empty tool_calls[]")
            if self.text is not None:
                raise ValueError("stop_reason='tool_use' must not carry text")
            for tc in self.tool_calls:
                if not tc.id:
                    raise ValueError("tool_use response: each tool_call requires id")
        return self
