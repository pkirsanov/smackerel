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
    resolve_ollama_determinism_options,
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


def test_profile_error_response_envelope_redacts_supplied_value_security102(monkeypatch: pytest.MonkeyPatch) -> None:
    sentinel = "SENTINEL-AGENT-PROFILE-SECRET-RR03"
    _set_route_env(monkeypatch, provider="ollama", model="test-model")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", sentinel)
    calls = {"count": 0}

    async def capture(**kwargs):
        calls["count"] += 1
        return SimpleNamespace()

    envelope = asyncio.run(handle_invoke(_request(), completion_fn=capture))

    assert envelope["outcome"] == "provider-error"
    assert sentinel not in json.dumps(envelope, sort_keys=True)
    assert calls["count"] == 0


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


# ---------------------------------------------------------------------------
# Spec 043 — Ollama determinism env-var plumbing
# ---------------------------------------------------------------------------


def _clear_ollama_determinism_env(monkeypatch: pytest.MonkeyPatch) -> None:
    for key in (
        "OLLAMA_TEST_REQUEST_TEMPERATURE",
        "OLLAMA_TEST_REQUEST_TOP_P",
        "OLLAMA_TEST_REQUEST_TOP_K",
        "OLLAMA_TEST_REQUEST_SEED",
        "OLLAMA_TEST_REQUEST_NUM_PREDICT",
    ):
        monkeypatch.delenv(key, raising=False)


def test_resolve_ollama_determinism_options_unset_is_empty(monkeypatch: pytest.MonkeyPatch) -> None:
    _clear_ollama_determinism_env(monkeypatch)
    assert resolve_ollama_determinism_options() == {}


def test_resolve_ollama_determinism_options_full_set(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TEMPERATURE", "0.0")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_P", "1.0")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_K", "1")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_SEED", "42")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_NUM_PREDICT", "256")

    options = resolve_ollama_determinism_options()

    assert options == {
        "temperature": 0.0,
        "top_p": 1.0,
        "top_k": 1,
        "seed": 42,
        "num_predict": 256,
    }


def test_resolve_ollama_determinism_options_skips_malformed(monkeypatch: pytest.MonkeyPatch) -> None:
    _clear_ollama_determinism_env(monkeypatch)
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_SEED", "not-a-number")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_P", "0.9")
    options = resolve_ollama_determinism_options()
    # Malformed seed is skipped; valid top_p is preserved.
    assert "seed" not in options
    assert options["top_p"] == 0.9


def test_handle_invoke_passes_ollama_determinism_kwargs(monkeypatch: pytest.MonkeyPatch) -> None:
    """When provider == ollama and OLLAMA_TEST_REQUEST_* are set, the
    handler MUST forward (top_p, top_k, seed, num_predict) as kwargs to
    the completion call AND override the request temperature with the
    env-sourced value."""
    _set_route_env(monkeypatch, provider="ollama", model="qwen2.5:0.5b-instruct")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TEMPERATURE", "0.0")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_P", "1.0")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_K", "1")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_SEED", "42")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_NUM_PREDICT", "256")

    seen_kwargs: dict[str, Any] = {}

    async def capture(**kwargs):  # noqa: ANN003
        seen_kwargs.update(kwargs)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=SimpleNamespace(tool_calls=None, content="ok"))],
            usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1),
            model=kwargs.get("model", ""),
        )

    # Request supplies temperature=0.7; the env var MUST override it to 0.0.
    env = asyncio.run(handle_invoke(_request(temperature=0.7), completion_fn=capture))
    assert "error" not in env, env

    assert seen_kwargs["model"] == "ollama_chat/qwen2.5:0.5b-instruct"
    assert seen_kwargs["temperature"] == 0.0
    assert seen_kwargs["top_p"] == 1.0
    assert seen_kwargs["top_k"] == 1
    assert seen_kwargs["seed"] == 42
    assert seen_kwargs["num_predict"] == 256


def test_handle_invoke_applies_ollama_profile_spec102(monkeypatch: pytest.MonkeyPatch) -> None:
    """TP-C3-01: the real injected-completion builder applies the selected
    profile without losing the determinism or tool-call envelope."""
    _set_route_env(monkeypatch, provider="ollama", model="qwen2.5:0.5b-instruct")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_K", "1")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_SEED", "42")
    captured: dict[str, Any] = {}

    async def capture(**kwargs):  # noqa: ANN003
        captured.update(kwargs)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=SimpleNamespace(tool_calls=None, content='{"answer":"ok"}'))],
            usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1),
            model=kwargs["model"],
        )

    result = asyncio.run(handle_invoke(_request(), completion_fn=capture))

    assert "error" not in result
    assert captured["model"] == "ollama_chat/qwen2.5:0.5b-instruct"
    assert captured["options"]["num_ctx"] == 4096
    assert captured["keep_alive"] == "30m"
    assert captured["top_k"] == 1
    assert captured["seed"] == 42
    assert captured["tools"][0]["function"]["name"] == "echo"


def test_handle_invoke_does_not_inject_ollama_kwargs_for_other_providers(monkeypatch: pytest.MonkeyPatch) -> None:
    """Adversarial — the determinism env vars are Ollama-scoped (their
    SST keys live under infrastructure.ollama.test.*). They MUST NOT
    leak into completion calls for OpenAI / Anthropic / etc., which
    would either error on the unknown ``num_predict``/``top_k`` kwargs
    or silently change the call shape."""
    _set_route_env(monkeypatch, provider="openai", model="gpt-4o-mini")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_TOP_K", "1")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_SEED", "42")
    monkeypatch.setenv("OLLAMA_TEST_REQUEST_NUM_PREDICT", "256")

    seen_kwargs: dict[str, Any] = {}

    async def capture(**kwargs):  # noqa: ANN003
        seen_kwargs.update(kwargs)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=SimpleNamespace(tool_calls=None, content="ok"))],
            usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1),
            model=kwargs.get("model", ""),
        )

    asyncio.run(handle_invoke(_request(temperature=0.7), completion_fn=capture))

    # Request temperature is preserved (no env override for non-ollama provider).
    assert seen_kwargs["temperature"] == 0.7
    # Ollama-scoped kwargs MUST NOT appear.
    for forbidden in ("top_k", "seed", "num_predict"):
        assert forbidden not in seen_kwargs, f"{forbidden} leaked into non-ollama completion call"


def test_handle_invoke_no_determinism_env_is_no_op(monkeypatch: pytest.MonkeyPatch) -> None:
    """Dev / self-hosted path: env vars are unset; the handler MUST pass
    the request temperature through unchanged and inject NO extra
    kwargs."""
    _set_route_env(monkeypatch, provider="ollama", model="some-other-model:7b")
    _clear_ollama_determinism_env(monkeypatch)

    seen_kwargs: dict[str, Any] = {}

    async def capture(**kwargs):  # noqa: ANN003
        seen_kwargs.update(kwargs)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=SimpleNamespace(tool_calls=None, content="ok"))],
            usage=SimpleNamespace(prompt_tokens=1, completion_tokens=1),
            model=kwargs.get("model", ""),
        )

    asyncio.run(handle_invoke(_request(temperature=0.42), completion_fn=capture))

    assert seen_kwargs["temperature"] == 0.42
    for forbidden in ("top_p", "top_k", "seed", "num_predict"):
        assert forbidden not in seen_kwargs
