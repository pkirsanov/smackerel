"""Spec 037 Scope 5 — stateless per-turn LLM dispatcher for the agent loop.

The Go executor (internal/agent/executor.go) owns scenario routing,
allowlist enforcement, JSON-Schema validation, the loop limit, the
schema-retry budget, the timeout, and trace persistence. This module
owns the Python side of one ``agent.invoke.request`` ↔
``agent.invoke.response`` exchange: render the system prompt + tool
defs into the provider's tool-calling format, call the LLM via
``litellm``, and return a normalized envelope of either tool calls or a
final answer. Nothing in this module decides which tool to run, retries
on failure, or persists state.

The handler is fully stateless — every call receives all context it
needs in the request payload. Concurrency-safety is therefore trivial:
no shared mutable state.

Outcome envelope (request → response):

* On success: ``{"tool_calls": [...], "final": null|str|obj, "provider":
  "...", "model": "...", "tokens": {"prompt": N, "completion": N}}``.
* On provider error: ``{"tool_calls": [], "final": null, "provider":
  "...", "model": "...", "tokens": {...}, "error": "...",
  "outcome": "provider-error"}``. The Go executor maps this to its
  ``provider-error`` terminal outcome.

The handler never raises — every failure mode is a structured envelope
the executor can record on the trace.
"""

from __future__ import annotations

import json
import logging
import os
import time
from typing import Any

logger = logging.getLogger("smackerel-ml.agent")

# Default provider routing keys mirror AGENT_PROVIDER_*_PROVIDER /
# AGENT_PROVIDER_*_MODEL. The Go side sends the symbolic preference
# (``"default" | "fast" | "reasoning" | "vision" | "ocr"``) and we
# resolve it here. There are NO Python-side fallback defaults: if the
# operator did not configure a route the request asks for, we return a
# structured ``provider-error`` envelope so the executor records it.
_PROVIDER_ENV_KEYS: dict[str, tuple[str, str]] = {
    "default": ("AGENT_PROVIDER_DEFAULT_PROVIDER", "AGENT_PROVIDER_DEFAULT_MODEL"),
    "reasoning": ("AGENT_PROVIDER_REASONING_PROVIDER", "AGENT_PROVIDER_REASONING_MODEL"),
    "fast": ("AGENT_PROVIDER_FAST_PROVIDER", "AGENT_PROVIDER_FAST_MODEL"),
    "vision": ("AGENT_PROVIDER_VISION_PROVIDER", "AGENT_PROVIDER_VISION_MODEL"),
    "ocr": ("AGENT_PROVIDER_OCR_PROVIDER", "AGENT_PROVIDER_OCR_MODEL"),
}


def resolve_provider_route(model_preference: str) -> tuple[str, str] | None:
    """Look up (provider, model) for a scenario's model_preference key.

    Returns None if the preference is unknown or the env vars are not
    set. Callers translate None into a provider-error response.
    """
    keys = _PROVIDER_ENV_KEYS.get(model_preference)
    if keys is None:
        return None
    provider = os.environ.get(keys[0], "")
    model = os.environ.get(keys[1], "")
    if not provider or not model:
        return None
    return provider, model


def render_messages(
    system_prompt: str,
    turn_messages: list[dict[str, Any]],
    structured_input: Any | None,
) -> list[dict[str, Any]]:
    """Render the executor's accumulating conversation into the chat
    message format litellm expects.

    The executor already labels each turn message by role
    (user|assistant|tool|system); we map these into chat-completions
    shape verbatim. We do NOT re-prompt-engineer: the system prompt
    comes from the scenario, and the rest is the LLM's own running
    state plus structured tool results.
    """
    messages: list[dict[str, Any]] = [{"role": "system", "content": system_prompt or ""}]
    if structured_input is not None:
        messages.append(
            {
                "role": "user",
                "content": json.dumps({"structured_context": structured_input}),
            }
        )
    for m in turn_messages:
        role = m.get("role", "")
        content = m.get("content")
        if isinstance(content, (dict, list)):
            text = json.dumps(content)
        elif content is None:
            text = ""
        else:
            text = str(content)
        if role == "tool":
            # The tool name from the executor is recorded so the LLM
            # can correlate the result back to the call it issued.
            messages.append(
                {
                    "role": "tool",
                    "name": m.get("tool_name") or "result",
                    "content": text,
                }
            )
        elif role in ("user", "assistant", "system"):
            messages.append({"role": role, "content": text})
        # Unknown roles are ignored; the executor controls the role set
        # so an unknown role here is a future-compat issue, not user
        # data we should faithfully forward.
    return messages


def render_tools(tool_defs: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Render the executor's tool defs into the OpenAI tools format.

    litellm normalizes this format across providers; the same payload
    works for Ollama (function-calling models), OpenAI, and Anthropic.
    """
    out: list[dict[str, Any]] = []
    for t in tool_defs:
        out.append(
            {
                "type": "function",
                "function": {
                    "name": t.get("name", ""),
                    "description": t.get("description", ""),
                    "parameters": t.get("input_schema") or {"type": "object"},
                },
            }
        )
    return out


def _provider_error(message: str, provider: str = "", model: str = "") -> dict[str, Any]:
    """Build a structured provider-error envelope. The executor maps
    this to its terminal ``provider-error`` outcome."""
    return {
        "tool_calls": [],
        "final": None,
        "provider": provider,
        "model": model,
        "tokens": {"prompt": 0, "completion": 0},
        "error": message,
        "outcome": "provider-error",
    }


def _parse_arguments(raw: Any) -> Any:
    """LLM tool-call arguments may arrive as a JSON string (OpenAI/
    function-call format) or as a parsed object (some providers).
    Normalize to a parsed object; on parse failure return the raw
    string so the executor's argument-schema validation surfaces a
    structured rejection."""
    if isinstance(raw, str):
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            return raw
    return raw


async def handle_invoke(
    request: dict[str, Any],
    *,
    completion_fn: Any | None = None,
) -> dict[str, Any]:
    """Handle one ``agent.invoke.request``.

    ``completion_fn`` is an injection point used by tests to bypass
    litellm; production passes ``None`` and we resolve litellm at call
    time so the import is lazy (matching the pattern already used in
    ml/app/synthesis.py).
    """
    start = time.time()
    trace_id = request.get("trace_id", "")
    model_pref = request.get("model_preference", "")

    route = resolve_provider_route(model_pref)
    if route is None:
        env = _provider_error(f"no provider route configured for model_preference={model_pref!r}")
        env["trace_id"] = trace_id
        env["processing_time_ms"] = int((time.time() - start) * 1000)
        return env
    provider, model = route

    messages = render_messages(
        system_prompt=request.get("system_prompt", ""),
        turn_messages=request.get("turn_messages") or [],
        structured_input=request.get("structured_input"),
    )
    tools = render_tools(request.get("tool_defs") or [])
    temperature = request.get("temperature", 0.0)
    token_budget = request.get("token_budget", 1000)

    if completion_fn is None:
        try:
            import litellm  # type: ignore[import-untyped]

            completion_fn = litellm.acompletion
            if provider == "ollama":
                ollama_url = os.environ.get("OLLAMA_URL")
                if ollama_url:
                    litellm.api_base = ollama_url
        except ImportError as exc:  # pragma: no cover — import is lazy
            env = _provider_error(f"litellm import failed: {exc}", provider, model)
            env["trace_id"] = trace_id
            env["processing_time_ms"] = int((time.time() - start) * 1000)
            return env

    llm_model = model if provider != "ollama" else f"ollama/{model}"

    api_key = None
    if provider != "ollama":
        api_key = os.environ.get("LLM_API_KEY") or os.environ.get(f"{provider.upper()}_API_KEY", "")

    try:
        response = await completion_fn(
            model=llm_model,
            messages=messages,
            tools=tools or None,
            temperature=temperature,
            max_tokens=token_budget,
            api_key=api_key,
        )
    except Exception as exc:  # noqa: BLE001 — provider errors must not crash the sidecar
        logger.warning(
            "agent.invoke completion failed",
            extra={"trace_id": trace_id, "error": type(exc).__name__},
        )
        env = _provider_error(f"{type(exc).__name__}: {exc}", provider, model)
        env["trace_id"] = trace_id
        env["processing_time_ms"] = int((time.time() - start) * 1000)
        return env

    # Normalise the litellm response into the executor envelope.
    # litellm exposes choices[0].message which has .content (final
    # text) and optionally .tool_calls (a list of objects whose
    # function.name and function.arguments we need).
    try:
        choice = response.choices[0]
        message = choice.message
        tool_calls_raw = getattr(message, "tool_calls", None) or []
        content = getattr(message, "content", None)
        usage = getattr(response, "usage", None)
        prompt_tokens = int(getattr(usage, "prompt_tokens", 0) or 0)
        completion_tokens = int(getattr(usage, "completion_tokens", 0) or 0)
        provider_label = provider
        model_label = getattr(response, "model", llm_model) or llm_model
    except Exception as exc:  # noqa: BLE001
        env = _provider_error(
            f"malformed completion response: {type(exc).__name__}: {exc}",
            provider,
            model,
        )
        env["trace_id"] = trace_id
        env["processing_time_ms"] = int((time.time() - start) * 1000)
        return env

    tool_calls: list[dict[str, Any]] = []
    for tc in tool_calls_raw:
        if isinstance(tc, dict):
            fn = tc.get("function", {})
        else:
            fn = getattr(tc, "function", None)
        name = getattr(fn, "name", None) if fn is not None else None
        args = getattr(fn, "arguments", None) if fn is not None else None
        if isinstance(tc, dict):
            fn_dict = tc.get("function", {})
            name = name or fn_dict.get("name")
            args = args if args is not None else fn_dict.get("arguments")
        parsed_args = _parse_arguments(args)
        # The executor validates arguments against the tool's input
        # schema, so we serialize back to JSON bytes here. If parsing
        # failed and we kept the raw string, the executor's argument
        # schema validation will produce the structured rejection.
        if isinstance(parsed_args, str):
            arg_json = parsed_args
        else:
            arg_json = json.dumps(parsed_args if parsed_args is not None else {})
        tool_calls.append({"name": name or "", "arguments": arg_json})

    final: Any = None
    if not tool_calls and content is not None:
        # The Go side will JSON-decode `final` and validate against
        # output_schema. We pass a string through verbatim; if it's
        # already JSON-shaped that is fine, if not the schema retry
        # path catches it.
        if isinstance(content, str):
            stripped = content.strip()
            # Strip common ```json fenced wrappers some models emit.
            if stripped.startswith("```"):
                stripped = stripped.strip("`")
                if stripped.lower().startswith("json"):
                    stripped = stripped[4:]
                stripped = stripped.strip()
            final = stripped
        else:
            final = content

    envelope: dict[str, Any] = {
        "tool_calls": tool_calls,
        "final": final,
        "provider": provider_label,
        "model": model_label,
        "tokens": {"prompt": prompt_tokens, "completion": completion_tokens},
        "trace_id": trace_id,
        "processing_time_ms": int((time.time() - start) * 1000),
    }
    return envelope
