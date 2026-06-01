"""Spec 064 SCOPE-04 — open-ended knowledge agent LLM bridge.

POST /llm/chat — extended tool-use chat endpoint shared by the Go agent
loop and the Python sidecar.

Three execution modes selected EXPLICITLY by the caller:

  - ``X-OpenKnowledge-Test-Mode: fixture-final-text``:
      Deterministic ``stop_reason='end_turn'`` response whose ``text``
      echoes the last user / tool_result / assistant content. Used by
      SCOPE-04 contract / parity tests.

  - ``X-OpenKnowledge-Test-Mode: fixture-tool-use``:
      Deterministic ``stop_reason='tool_use'`` response that issues a
      ToolCall for the FIRST entry of ``tools[]``.

  - Header absent: dispatch to the real LLM via litellm against the
      Ollama backend pointed to by ``OLLAMA_URL`` (G028 — no default,
      fail-loud if env var is missing). Wired here to close
      PKT-WORKFLOW-A finding #1.
"""

from __future__ import annotations

import json
import logging
import os
from typing import Any

from fastapi import APIRouter, HTTPException, Request

from ..schemas import (
    ChatMessage,
    ChatRequest,
    ChatResponse,
    Role,
    StopReason,
    Tool,
    ToolCall,
)

logger = logging.getLogger("smackerel-ml.openknowledge.chat")

router = APIRouter(prefix="/llm", tags=["open_knowledge"])

_TEST_MODE_HEADER = "x-openknowledge-test-mode"
_MODE_FINAL_TEXT = "fixture-final-text"
_MODE_TOOL_USE = "fixture-tool-use"

# Transport-level safety cap; the Go agent loop owns the higher-level
# per-turn token budget.
_LIVE_DISPATCH_TIMEOUT_SECONDS = 600


def _last_substantive_text(messages: list[ChatMessage]) -> str:
    """Return the content of the last user / tool_result / assistant
    message. Used by the fixture-final-text mode to make the echo
    response deterministic and inspectable.
    """
    for msg in reversed(messages):
        if msg.role in (Role.USER, Role.TOOL_RESULT, Role.ASSISTANT) and msg.content:
            return msg.content
    return ""


def _translate_messages(messages: list[ChatMessage]) -> list[dict[str, Any]]:
    """Translate Go-side ChatMessage[] to litellm OpenAI-shaped messages.

    Role mapping:
      system/user/assistant → same role, content passthrough
      tool_call             → assistant + tool_calls[] (OpenAI shape)
      tool_result           → tool + tool_call_id + content
    """
    out: list[dict[str, Any]] = []
    for m in messages:
        if m.role is Role.SYSTEM:
            out.append({"role": "system", "content": m.content or ""})
        elif m.role is Role.USER:
            out.append({"role": "user", "content": m.content or ""})
        elif m.role is Role.ASSISTANT:
            out.append({"role": "assistant", "content": m.content or ""})
        elif m.role is Role.TOOL_CALL:
            assert m.tool_calls  # schema-enforced
            out.append(
                {
                    "role": "assistant",
                    "content": m.content or "",
                    "tool_calls": [
                        {
                            "id": tc.id,
                            "type": "function",
                            "function": {
                                "name": tc.name,
                                "arguments": json.dumps(tc.arguments),
                            },
                        }
                        for tc in m.tool_calls
                    ],
                }
            )
        elif m.role is Role.TOOL_RESULT:
            assert m.tool_call_id is not None  # schema-enforced
            out.append(
                {
                    "role": "tool",
                    "tool_call_id": m.tool_call_id,
                    "content": m.content or "",
                }
            )
    return out


def _translate_tools(tools: list[Tool] | None) -> list[dict[str, Any]] | None:
    if not tools:
        return None
    return [
        {
            "type": "function",
            "function": {
                "name": t.name,
                "description": t.description,
                "parameters": t.parameters,
            },
        }
        for t in tools
    ]


def _parse_tool_call_arguments(raw: Any) -> dict[str, Any]:
    """OpenAI emits function.arguments as a JSON string; some providers
    pass through a dict. Schema requires dict; empty/invalid yields {}.
    """
    if isinstance(raw, dict):
        return raw
    if not raw:
        return {}
    try:
        parsed = json.loads(raw)
        return parsed if isinstance(parsed, dict) else {}
    except (TypeError, ValueError):
        return {}


async def _dispatch_live(req: ChatRequest) -> ChatResponse:
    ollama_url = os.environ.get("OLLAMA_URL")
    if not ollama_url:
        raise HTTPException(
            status_code=500,
            detail={
                "error": "llm_misconfigured",
                "message": "OLLAMA_URL not set in ml sidecar environment",
            },
        )

    import litellm
    from litellm.exceptions import (  # type: ignore[import-not-found]
        APIConnectionError,
        APIError,
        ServiceUnavailableError,
        Timeout,
    )

    messages = _translate_messages(req.messages)
    tools = _translate_tools(req.tools)

    kwargs: dict[str, Any] = {
        # Spec 064 — use ollama_chat/ prefix (litellm /api/chat path) so
        # OpenAI-shaped tool calls round-trip natively. The legacy
        # ollama/ prefix uses /api/generate which serializes messages
        # into a single prompt and loses tool_call structure.
        "model": f"ollama_chat/{req.model}",
        "api_base": ollama_url,
        "messages": messages,
        "timeout": _LIVE_DISPATCH_TIMEOUT_SECONDS,
        "temperature": req.temperature if req.temperature is not None else 0.0,
    }
    if tools:
        kwargs["tools"] = tools
    if req.max_tokens is not None:
        kwargs["max_tokens"] = req.max_tokens

    try:
        response = await litellm.acompletion(**kwargs)
    except (APIConnectionError, ServiceUnavailableError, Timeout) as e:
        logger.warning(
            "open_knowledge live dispatch unreachable: %s: %s",
            type(e).__name__,
            e,
        )
        raise HTTPException(
            status_code=502,
            detail={
                "error": "llm_provider_unreachable",
                "message": f"{type(e).__name__}: {e}",
            },
        )
    except APIError as e:
        logger.warning("open_knowledge live dispatch provider error: %s", e)
        raise HTTPException(
            status_code=502,
            detail={
                "error": "llm_provider_error",
                "message": f"{type(e).__name__}: {e}",
            },
        )
    except Exception as e:  # pragma: no cover — defensive
        logger.exception("open_knowledge live dispatch failed unexpectedly")
        raise HTTPException(
            status_code=500,
            detail={
                "error": "llm_dispatch_failed",
                "message": f"{type(e).__name__}: {str(e)[:200]}",
            },
        )

    choice_msg = response.choices[0].message
    raw_tool_calls = getattr(choice_msg, "tool_calls", None) or []
    tokens_used = 0
    usage = getattr(response, "usage", None)
    if usage is not None:
        tokens_used = getattr(usage, "total_tokens", 0) or 0

    if raw_tool_calls:
        decoded: list[ToolCall] = []
        for idx, tc in enumerate(raw_tool_calls):
            tc_id = getattr(tc, "id", None) or f"call-auto-{idx}"
            fn = getattr(tc, "function", None)
            name = getattr(fn, "name", "") if fn is not None else ""
            args = getattr(fn, "arguments", "") if fn is not None else ""
            if not name:
                raise HTTPException(
                    status_code=502,
                    detail={
                        "error": "llm_malformed_tool_call",
                        "message": f"tool_calls[{idx}] missing function.name",
                    },
                )
            decoded.append(
                ToolCall(
                    id=tc_id,
                    name=name,
                    arguments=_parse_tool_call_arguments(args),
                )
            )
        return ChatResponse(
            stop_reason=StopReason.TOOL_USE,
            tool_calls=decoded,
            tokens_used=tokens_used,
        )

    text = getattr(choice_msg, "content", None) or ""
    return ChatResponse(
        stop_reason=StopReason.END_TURN,
        text=text,
        tokens_used=tokens_used,
    )


@router.post("/chat", response_model=ChatResponse)
async def chat(req: ChatRequest, request: Request) -> ChatResponse:
    mode = request.headers.get(_TEST_MODE_HEADER, "").strip().lower()

    if mode == _MODE_FINAL_TEXT:
        echo = _last_substantive_text(req.messages)
        return ChatResponse(
            stop_reason=StopReason.END_TURN,
            text=f"echo:{echo}",
            tokens_used=len(echo),
        )

    if mode == _MODE_TOOL_USE:
        if not req.tools:
            raise HTTPException(
                status_code=422,
                detail="fixture-tool-use mode requires tools[] in the request",
            )
        first = req.tools[0]
        tool_call_id = f"call-{len(req.messages)}"
        return ChatResponse(
            stop_reason=StopReason.TOOL_USE,
            tool_calls=[ToolCall(id=tool_call_id, name=first.name, arguments={})],
            tokens_used=0,
        )

    return await _dispatch_live(req)
