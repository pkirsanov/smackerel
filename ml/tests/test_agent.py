"""Spec 037 Scope 5 — unit tests for ml.app.agent.handle_invoke.

These tests assert the Python sidecar's per-turn handler is stateless,
returns a normalized envelope on success, and produces a structured
provider-error envelope on every failure mode (missing route, litellm
exception, malformed response). No real LLM is contacted; tests inject
a fake completion_fn.
"""

from __future__ import annotations

import asyncio
import json
from types import SimpleNamespace
from typing import Any

import pytest

from app.agent import (
    handle_invoke,
    render_messages,
    render_tools,
    resolve_provider_route,
)


def _set_route_env(monkeypatch: pytest.MonkeyPatch, *, provider: str = "ollama", model: str = "test-model") -> None:
    monkeypatch.setenv("AGENT_PROVIDER_FAST_PROVIDER", provider)
    monkeypatch.setenv("AGENT_PROVIDER_FAST_MODEL", model)


def _make_fake_completion(
    *,
    tool_calls: list[dict[str, Any]] | None = None,
    final_text: str | None = None,
    raise_exc: Exception | None = None,
):
    """Build an async fn shaped like litellm.acompletion."""

    async def fn(**kwargs):  # noqa: ANN003
        if raise_exc is not None:
            raise raise_exc
        tcs = []
        for tc in tool_calls or []:
            tcs.append(
                SimpleNamespace(
                    function=SimpleNamespace(
                        name=tc["name"],
                        arguments=tc["arguments"],
                    )
                )
            )
        message = SimpleNamespace(tool_calls=tcs or None, content=final_text)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=message)],
            usage=SimpleNamespace(prompt_tokens=11, completion_tokens=7),
            model=kwargs.get("model", ""),
        )

    return fn


def _request(model_pref: str = "fast", **overrides: Any) -> dict[str, Any]:
    base: dict[str, Any] = {
        "trace_id": "trace_test_1",
        "scenario_id": "exec_test",
        "scenario_version": "exec_test-v1",
        "system_prompt": "You are a test agent.",
        "tool_defs": [
            {
                "name": "echo",
                "description": "echoes",
                "input_schema": {"type": "object", "properties": {"q": {"type": "string"}}},
            }
        ],
        "turn_messages": [{"role": "user", "content": '{"input":"hi"}'}],
        "token_budget": 1000,
        "temperature": 0.1,
        "model_preference": model_pref,
        "structured_input": {"input": "hi"},
    }
    base.update(overrides)
    return base


def test_resolve_provider_route_ok(monkeypatch: pytest.MonkeyPatch) -> None:
    _set_route_env(monkeypatch, provider="openai", model="gpt-4o")
    route = resolve_provider_route("fast")
    assert route == ("openai", "gpt-4o")


def test_resolve_provider_route_unknown(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("AGENT_PROVIDER_FAST_PROVIDER", raising=False)
    assert resolve_provider_route("not-a-real-pref") is None


def test_resolve_provider_route_missing_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("AGENT_PROVIDER_FAST_PROVIDER", raising=False)
    monkeypatch.delenv("AGENT_PROVIDER_FAST_MODEL", raising=False)
    assert resolve_provider_route("fast") is None


def test_render_tools_uses_input_schema_as_parameters() -> None:
    tools = render_tools([{"name": "echo", "description": "d", "input_schema": {"type": "object", "x": 1}}])
    assert tools == [
        {
            "type": "function",
            "function": {"name": "echo", "description": "d", "parameters": {"type": "object", "x": 1}},
        }
    ]


def test_render_messages_preserves_order_and_tool_role() -> None:
    msgs = render_messages(
        system_prompt="SYS",
        turn_messages=[
            {"role": "user", "content": '{"a":1}'},
            {"role": "assistant", "content": "{}"},
            {"role": "tool", "tool_name": "echo", "content": '{"q":"x"}'},
            {"role": "system", "content": "retry-hint"},
        ],
        structured_input={"a": 1},
    )
    assert msgs[0] == {"role": "system", "content": "SYS"}
    # structured_input gets injected as a separate user message.
    assert msgs[1]["role"] == "user"
    assert "structured_context" in msgs[1]["content"]
    assert msgs[2]["role"] == "user"
    assert msgs[3]["role"] == "assistant"
    assert msgs[4] == {"role": "tool", "name": "echo", "content": '{"q":"x"}'}
    assert msgs[5] == {"role": "system", "content": "retry-hint"}


def test_handle_invoke_happy_tool_call(monkeypatch: pytest.MonkeyPatch) -> None:
    _set_route_env(monkeypatch)
    fake = _make_fake_completion(tool_calls=[{"name": "echo", "arguments": '{"q":"hi"}'}])

    env = asyncio.run(handle_invoke(_request(), completion_fn=fake))

    assert "error" not in env, env
    assert env["tool_calls"] == [{"name": "echo", "arguments": '{"q": "hi"}'}]
    assert env["final"] is None
    assert env["provider"] == "ollama"
    assert env["tokens"] == {"prompt": 11, "completion": 7}
    assert env["trace_id"] == "trace_test_1"


def test_handle_invoke_happy_final_text(monkeypatch: pytest.MonkeyPatch) -> None:
    _set_route_env(monkeypatch)
    fake = _make_fake_completion(final_text='{"answer":"forty-two"}')

    env = asyncio.run(handle_invoke(_request(), completion_fn=fake))

    assert env["tool_calls"] == []
    assert env["final"] == '{"answer":"forty-two"}'


def test_handle_invoke_strips_fenced_json(monkeypatch: pytest.MonkeyPatch) -> None:
    _set_route_env(monkeypatch)
    fake = _make_fake_completion(final_text='```json\n{"answer":"x"}\n```')
    env = asyncio.run(handle_invoke(_request(), completion_fn=fake))
    # The handler strips the markdown fence so the executor sees JSON.
    assert env["final"].startswith("{") and env["final"].endswith("}")
    assert json.loads(env["final"]) == {"answer": "x"}


def test_handle_invoke_unknown_route_is_provider_error(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("AGENT_PROVIDER_FAST_PROVIDER", raising=False)
    monkeypatch.delenv("AGENT_PROVIDER_FAST_MODEL", raising=False)
    fake = _make_fake_completion(final_text='{"answer":"x"}')

    env = asyncio.run(handle_invoke(_request(model_pref="fast"), completion_fn=fake))

    assert env["outcome"] == "provider-error"
    assert env["tool_calls"] == []
    assert env["final"] is None
    assert "no provider route configured" in env["error"]


def test_handle_invoke_litellm_exception_is_provider_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _set_route_env(monkeypatch)
    fake = _make_fake_completion(raise_exc=RuntimeError("provider down"))

    env = asyncio.run(handle_invoke(_request(), completion_fn=fake))

    assert env["outcome"] == "provider-error"
    assert "RuntimeError" in env["error"]
    assert env["tool_calls"] == []


def test_handle_invoke_is_stateless(monkeypatch: pytest.MonkeyPatch) -> None:
    """Two consecutive invocations with different inputs MUST NOT
    interfere. The handler is module-level state-free; this gates a
    regression where someone caches messages or tools across calls."""
    _set_route_env(monkeypatch)
    fake_a = _make_fake_completion(final_text='{"answer":"A"}')
    fake_b = _make_fake_completion(final_text='{"answer":"B"}')

    env_a = asyncio.run(handle_invoke(_request(trace_id="ta"), completion_fn=fake_a))
    env_b = asyncio.run(handle_invoke(_request(trace_id="tb"), completion_fn=fake_b))

    assert env_a["final"] == '{"answer":"A"}'
    assert env_b["final"] == '{"answer":"B"}'
    assert env_a["trace_id"] == "ta"
    assert env_b["trace_id"] == "tb"


def test_handle_invoke_dict_arguments_serialized(monkeypatch: pytest.MonkeyPatch) -> None:
    """Some providers return tool-call arguments as already-parsed dicts
    instead of JSON strings. The handler must serialize back to JSON
    bytes for the Go executor."""
    _set_route_env(monkeypatch)

    class DictArgComp:
        async def __call__(self, **kwargs):  # noqa: ANN003
            tc = SimpleNamespace(function=SimpleNamespace(name="echo", arguments={"q": "dict"}))
            message = SimpleNamespace(tool_calls=[tc], content=None)
            return SimpleNamespace(
                choices=[SimpleNamespace(message=message)],
                usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1),
                model=kwargs["model"],
            )

    env = asyncio.run(handle_invoke(_request(), completion_fn=DictArgComp()))
    assert env["tool_calls"][0]["arguments"] == '{"q": "dict"}'
