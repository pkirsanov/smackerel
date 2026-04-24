"""Adversarial regression tests for app.agent_config (spec 037 Scope 1).

Mirror the Go-side adversarial coverage in internal/agent/config_test.go:
the Python sidecar MUST refuse to start when any required AGENT_* variable
is missing, empty (where the design forbids empty), or malformed.
"""

from __future__ import annotations

import pytest

from app.agent_config import (
    ALL_AGENT_ENV_KEYS,
    REQUIRED_NON_EMPTY_KEYS,
    AgentConfigError,
    load_agent_config,
)


def _valid_env() -> dict[str, str]:
    return {
        "AGENT_SCENARIO_DIR": "config/prompt_contracts",
        "AGENT_SCENARIO_GLOB": "*.yaml",
        "AGENT_HOT_RELOAD": "true",
        "AGENT_ROUTING_CONFIDENCE_FLOOR": "0.65",
        "AGENT_ROUTING_CONSIDER_TOP_N": "5",
        "AGENT_ROUTING_FALLBACK_SCENARIO_ID": "",
        "AGENT_ROUTING_EMBEDDING_MODEL": "",
        "AGENT_TRACE_RETENTION_DAYS": "30",
        "AGENT_TRACE_RECORD_LLM_MESSAGES": "false",
        "AGENT_TRACE_REDACT_MARKER": "***",
        "AGENT_DEFAULTS_MAX_LOOP_ITERATIONS_CEILING": "32",
        "AGENT_DEFAULTS_TIMEOUT_MS_CEILING": "120000",
        "AGENT_DEFAULTS_SCHEMA_RETRY_BUDGET_CEILING": "5",
        "AGENT_DEFAULTS_PER_TOOL_TIMEOUT_MS_CEILING": "30000",
        "AGENT_PROVIDER_DEFAULT_PROVIDER": "ollama",
        "AGENT_PROVIDER_DEFAULT_MODEL": "gemma4:26b",
        "AGENT_PROVIDER_REASONING_PROVIDER": "ollama",
        "AGENT_PROVIDER_REASONING_MODEL": "deepseek-r1:32b",
        "AGENT_PROVIDER_FAST_PROVIDER": "ollama",
        "AGENT_PROVIDER_FAST_MODEL": "gpt-oss:20b",
        "AGENT_PROVIDER_VISION_PROVIDER": "ollama",
        "AGENT_PROVIDER_VISION_MODEL": "gemma4:26b",
        "AGENT_PROVIDER_OCR_PROVIDER": "ollama",
        "AGENT_PROVIDER_OCR_MODEL": "deepseek-ocr:3b",
    }


def _apply_env(monkeypatch: pytest.MonkeyPatch, env: dict[str, str]) -> None:
    # Wipe every AGENT_* var to a known absent state, then install env.
    for key in ALL_AGENT_ENV_KEYS:
        monkeypatch.delenv(key, raising=False)
    for key, value in env.items():
        monkeypatch.setenv(key, value)


def test_load_agent_config_happy_path(monkeypatch: pytest.MonkeyPatch) -> None:
    _apply_env(monkeypatch, _valid_env())

    cfg = load_agent_config()
    assert cfg.scenario_dir == "config/prompt_contracts"
    assert cfg.hot_reload is True
    assert cfg.routing_confidence_floor == 0.65
    assert cfg.routing_fallback_scenario_id == ""  # explicit opt-out
    assert cfg.routing_embedding_model == ""  # explicit opt-out
    assert cfg.defaults_timeout_ms_ceiling == 120000
    assert cfg.provider_routing["reasoning"].provider == "ollama"
    assert cfg.provider_routing["reasoning"].model == "deepseek-r1:32b"


@pytest.mark.parametrize("key", ALL_AGENT_ENV_KEYS)
def test_missing_required_env_fails_loud(monkeypatch: pytest.MonkeyPatch, key: str) -> None:
    """Removing any required AGENT_* var (even an opt-out) MUST be fatal."""
    env = _valid_env()
    env.pop(key)
    _apply_env(monkeypatch, env)

    with pytest.raises(AgentConfigError) as excinfo:
        load_agent_config()
    assert key in str(excinfo.value), f"error did not name missing var {key}: {excinfo.value}"


def test_empty_env_enumerates_every_missing_var(monkeypatch: pytest.MonkeyPatch) -> None:
    _apply_env(monkeypatch, {})

    with pytest.raises(AgentConfigError) as excinfo:
        load_agent_config()
    msg = str(excinfo.value)
    for key in ALL_AGENT_ENV_KEYS:
        assert key in msg, f"error does not enumerate {key}; error={msg}"


@pytest.mark.parametrize("key", REQUIRED_NON_EMPTY_KEYS)
def test_empty_value_fails_loud(monkeypatch: pytest.MonkeyPatch, key: str) -> None:
    """Setting any required-non-empty var to "" MUST be fatal — drift guard
    against config generate emitting AGENT_X= for a missing YAML value."""
    env = _valid_env()
    env[key] = ""
    _apply_env(monkeypatch, env)

    with pytest.raises(AgentConfigError) as excinfo:
        load_agent_config()
    assert key in str(excinfo.value), f"error did not name empty var {key}: {excinfo.value}"


@pytest.mark.parametrize(
    "key,value,want_substr",
    [
        ("AGENT_ROUTING_CONFIDENCE_FLOOR", "not-a-float", "AGENT_ROUTING_CONFIDENCE_FLOOR"),
        ("AGENT_ROUTING_CONFIDENCE_FLOOR", "1.5", "[0.0, 1.0]"),
        ("AGENT_ROUTING_CONSIDER_TOP_N", "0", ">= 1"),
        ("AGENT_DEFAULTS_TIMEOUT_MS_CEILING", "abc", "AGENT_DEFAULTS_TIMEOUT_MS_CEILING"),
        ("AGENT_HOT_RELOAD", "yes", "true or false"),
        ("AGENT_TRACE_RECORD_LLM_MESSAGES", "1", "true or false"),
    ],
)
def test_malformed_value_fails_loud(monkeypatch: pytest.MonkeyPatch, key: str, value: str, want_substr: str) -> None:
    env = _valid_env()
    env[key] = value
    _apply_env(monkeypatch, env)

    with pytest.raises(AgentConfigError) as excinfo:
        load_agent_config()
    assert want_substr in str(excinfo.value), f"error {excinfo.value!r} missing substring {want_substr!r}"


def test_optional_empty_opt_outs_accepted(monkeypatch: pytest.MonkeyPatch) -> None:
    env = _valid_env()
    env["AGENT_ROUTING_FALLBACK_SCENARIO_ID"] = ""
    env["AGENT_ROUTING_EMBEDDING_MODEL"] = ""
    _apply_env(monkeypatch, env)

    cfg = load_agent_config()
    assert cfg.routing_fallback_scenario_id == ""
    assert cfg.routing_embedding_model == ""
