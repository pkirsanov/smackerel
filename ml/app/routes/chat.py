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
    """Spec 096 SCOPE-03 — provider-aware dispatch fork (mirrors the
    ``synthesis.py`` provider fork). ``provider`` absent or ``"ollama"`` takes
    the byte-for-byte Ollama path (spec 088/089 parity invariant); any other
    provider takes the hosted credential branch. A hosted dispatch NEVER falls
    back to Ollama — a missing credential fails loud (SCN-096-G01).
    """
    messages = _translate_messages(req.messages)
    tools = _translate_tools(req.tools)
    provider = (req.provider or "").strip().lower()
    if provider in ("", "ollama"):
        return await _dispatch_ollama(req, messages, tools)
    return await _dispatch_hosted(req, messages, tools, provider)


async def _dispatch_ollama(
    req: ChatRequest,
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]] | None,
) -> ChatResponse:
    """The local Ollama dispatch — BYTE-FOR-BYTE the pre-096 ``_dispatch_live``
    behaviour (spec 088/089 parity, proven by
    ``test_chat_dispatch_parity_spec096``). The ``litellm.acompletion`` kwargs
    for a fixed Ollama request are unchanged and NO ``api_key`` is ever
    attached.

    ``api_base`` carries today's ``OLLAMA_URL``: the spec 064 caller omits it,
    so we read ``OLLAMA_URL`` from the environment exactly as before; the spec
    096 Ollama caller sends the same value explicitly. Either way the composed
    kwargs are identical.
    """
    ollama_url = req.api_base or os.environ.get("OLLAMA_URL")
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

    return _build_chat_response(response)


# Spec 096 SCOPE-03 — litellm provider-route prefix per smackerel connection
# kind (design §4 routing column). The kind names map 1:1 to litellm route
# prefixes except azure-foundry→azure and google→gemini.
_LITELLM_PROVIDER_PREFIX: dict[str, str] = {
    "anthropic": "anthropic",
    "openai": "openai",
    "azure-foundry": "azure",
    "google": "gemini",
    "bedrock": "bedrock",
}


def _compose_hosted_model(provider: str, backend_model: str) -> str:
    """Compose the litellm provider-qualified model from the connection kind +
    the BARE backend id (design §6.2). The Go core sends the bare ``model`` +
    the ``provider`` discriminant; the sidecar recomposes ``<prefix>/<model>``.
    """
    prefix = _LITELLM_PROVIDER_PREFIX.get(provider)
    if prefix is None:
        raise HTTPException(
            status_code=500,
            detail={
                "error": "llm_misconfigured",
                "message": f"unknown hosted provider {provider!r}; cannot compose a dispatch model",
            },
        )
    return f"{prefix}/{backend_model}"


def _scrub_secret(text: str, secret: str | None) -> str:
    """Spec 096 SCN-096-G05 — redact any occurrence of the cleartext credential
    from ``text`` before it crosses the wire or reaches a log. litellm
    exceptions can embed the ``api_key`` in a URL/header; this removes the
    substring. Load-bearing: without it the hosted error path would leak the
    key (the adversarial scrub test fails if this is removed).
    """
    if secret and secret in text:
        return text.replace(secret, "***")
    return text


def _hosted_dispatch_error(
    status_code: int,
    error_code: str,
    provider: str,
    model: str,
    exc: Exception,
    api_key: str | None,
) -> HTTPException:
    """Build a secret-safe HTTPException for a hosted dispatch failure
    (design §11.5 / SCN-096-G05). The wire ``detail`` is built from the
    exception TYPE + provider + model and then scrubbed of any ``api_key``
    substring as defense in depth; the ``api_key`` is NEVER logged (the log
    line carries only the exception type + provider + model).
    """
    safe = _scrub_secret(f"{type(exc).__name__}: {exc}", api_key)
    message = f"provider={provider} model={model}: {safe}"
    logger.warning(
        "open_knowledge hosted dispatch error (%s): %s provider=%s model=%s",
        error_code,
        type(exc).__name__,
        provider,
        model,
    )
    return HTTPException(status_code=status_code, detail={"error": error_code, "message": message})


async def _dispatch_hosted(
    req: ChatRequest,
    messages: list[dict[str, Any]],
    tools: list[dict[str, Any]] | None,
    provider: str,
) -> ChatResponse:
    """The hosted-provider dispatch: compose ``<prefix>/<backend-id>`` and route
    with the per-request cleartext ``api_key`` the Go core decrypted. A missing
    required ``api_key`` fails loud with a typed ``llm_misconfigured`` and NEVER
    substitutes the local Ollama model (SCN-096-G01). Secrets are scrubbed from
    every error/log (SCN-096-G05).
    """
    api_key = req.api_key
    if not api_key:
        # NO-DEFAULTS / fail-loud (G028): a hosted dispatch with no credential
        # is a misconfiguration — it is NEVER silently re-routed to Ollama.
        raise HTTPException(
            status_code=500,
            detail={
                "error": "llm_misconfigured",
                "message": (
                    f"hosted provider {provider!r} model {req.model!r} requires an api_key "
                    f"but none was supplied; refusing to substitute a local model"
                ),
            },
        )

    import litellm
    from litellm.exceptions import (  # type: ignore[import-not-found]
        APIConnectionError,
        APIError,
        ServiceUnavailableError,
        Timeout,
    )

    kwargs: dict[str, Any] = {
        "model": _compose_hosted_model(provider, req.model),
        "messages": messages,
        "timeout": _LIVE_DISPATCH_TIMEOUT_SECONDS,
        "temperature": req.temperature if req.temperature is not None else 0.0,
        "api_key": api_key,
    }
    if req.api_base:
        kwargs["api_base"] = req.api_base
    # Non-secret per-kind routing extras (Azure api_version+deployment, OpenAI
    # organization, Vertex project+location, Bedrock region). setdefault so they
    # never clobber model/messages/api_key.
    for key, value in (req.provider_params or {}).items():
        kwargs.setdefault(key, value)
    if tools:
        kwargs["tools"] = tools
    if req.max_tokens is not None:
        kwargs["max_tokens"] = req.max_tokens

    try:
        response = await litellm.acompletion(**kwargs)
    except (APIConnectionError, ServiceUnavailableError, Timeout) as e:
        raise _hosted_dispatch_error(502, "llm_provider_unreachable", provider, req.model, e, api_key)
    except APIError as e:
        raise _hosted_dispatch_error(502, "llm_provider_error", provider, req.model, e, api_key)
    except Exception as e:  # pragma: no cover — defensive
        raise _hosted_dispatch_error(500, "llm_dispatch_failed", provider, req.model, e, api_key)

    return _build_chat_response(response)


def _build_chat_response(response: Any) -> ChatResponse:
    """Shared litellm-response → ChatResponse translation (provider-agnostic).
    Unchanged from the pre-096 ``_dispatch_live`` tail.
    """
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
