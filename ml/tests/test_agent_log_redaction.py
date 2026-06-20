"""BUG-076-001 — ML agent dispatcher MUST NOT log raw conversational content.

Spec 076 Hard Constraint 6 (Privacy) and Principle 8 forbid raw user
turn text from reaching logs. The Go agent turn-log already enforces
this (``TestAgentTurnLog_RedactsSecrets`` logs ``prompt_sha256``, never
the raw prompt). The Python sidecar's ``agent.invoke.request`` and
``agent.invoke.envelope`` diagnostic INFO logs MUST honour the same
discipline: they may record shape/length/type metadata, but never the
raw user message content or the raw LLM final answer.

This is an ADVERSARIAL regression: it plants unique canary strings in
the user turn and in the LLM final answer, then asserts that neither
canary appears in ANY captured INFO log record. If the content-leaking
``first_user_msg=%r`` / ``final_preview=%r`` formatting were
reintroduced, the canaries would surface in the log and this test would
fail (RED), which is exactly the bug it guards.
"""

from __future__ import annotations

import asyncio
import logging
from types import SimpleNamespace
from typing import Any

import pytest

from app.agent import handle_invoke

AGENT_LOGGER = "smackerel-ml.agent"

# Unique, grep-stable canaries. They must never appear in any log line.
CANARY_USER = "CANARY_USER_TURN_TEXT_pii_home_address_42_elm_street_zzz"
CANARY_FINAL = "CANARY_FINAL_ANSWER_synthesized_personal_knowledge_yyy"


def _set_default_route(monkeypatch: pytest.MonkeyPatch) -> None:
    # Non-ollama provider so the request log path runs in full; the
    # injected completion_fn means litellm is never actually called.
    monkeypatch.setenv("AGENT_PROVIDER_DEFAULT_PROVIDER", "openai")
    monkeypatch.setenv("AGENT_PROVIDER_DEFAULT_MODEL", "gpt-4o")


def _fake_completion_with_final(final_text: str):
    async def fn(**kwargs: Any):  # noqa: ANN003
        message = SimpleNamespace(tool_calls=None, content=final_text)
        return SimpleNamespace(
            choices=[SimpleNamespace(message=message)],
            usage=SimpleNamespace(prompt_tokens=11, completion_tokens=7),
            model=kwargs.get("model", ""),
        )

    return fn


def _request_with_canaries() -> dict[str, Any]:
    return {
        "trace_id": "trace_redaction_1",
        "scenario_id": "redaction_test",
        "scenario_version": "redaction_test-v1",
        "system_prompt": "You are a test agent.",
        "tool_defs": [],
        "turn_messages": [{"role": "user", "content": '{"input":"' + CANARY_USER + '"}'}],
        "token_budget": 1000,
        "temperature": 0.0,
        "model_preference": "default",
        "structured_input": {"input": CANARY_USER},
    }


def test_agent_invoke_does_not_log_raw_user_or_final_content(
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    _set_default_route(monkeypatch)
    request = _request_with_canaries()
    completion_fn = _fake_completion_with_final(CANARY_FINAL)

    with caplog.at_level(logging.INFO, logger=AGENT_LOGGER):
        envelope = asyncio.run(handle_invoke(request, completion_fn=completion_fn))

    # Sanity: the dispatcher ran the happy path (not a provider-error
    # bail-out), so the diagnostic logs we are guarding actually fired.
    assert "error" not in envelope, envelope
    messages = [rec.getMessage() for rec in caplog.records]
    joined = "\n".join(messages)
    assert "agent.invoke.request" in joined, "request diagnostic log did not fire"
    assert "agent.invoke.envelope" in joined, "envelope diagnostic log did not fire"

    # Adversarial assertions: neither canary may appear in ANY log line.
    for rec in caplog.records:
        rendered = rec.getMessage()
        assert CANARY_USER not in rendered, (
            f"PII LEAK: raw user turn text reached log record {rec.name}/{rec.levelname}: {rendered!r}"
        )
        assert CANARY_FINAL not in rendered, (
            f"PII LEAK: raw LLM final answer reached log record {rec.name}/{rec.levelname}: {rendered!r}"
        )

    # The diagnostic logs must still carry useful non-content metadata
    # (trace_id) so the fix redacts content without blinding operators.
    assert "trace_redaction_1" in joined, "trace_id metadata missing from diagnostic logs"
