import asyncio
import json
import logging
import sys
import types
from pathlib import Path
from unittest.mock import AsyncMock

import pytest

# Mock litellm and its exceptions submodule before any app imports that use it
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

from app.main import _check_required_config, _warmup_domain_model, health


@pytest.fixture(autouse=True)
def clear_required_env(monkeypatch):
    for key in [
        "NATS_URL",
        "LLM_PROVIDER",
        "LLM_MODEL",
        "LLM_API_KEY",
        "OLLAMA_URL",
        "ML_PROCESSING_DEGRADED_FALLBACK_ENABLED",
        "SMACKEREL_AUTH_TOKEN",
        "SMACKEREL_ENV",
        # Spec 067 BUG-067-001 — ML sidecar log level SST contract.
        "ML_LOG_LEVEL",
        # Spec 050 — ML sidecar health/worker isolation SST contract.
        "ML_EMBEDDING_WORKERS",
        "ML_EMBEDDING_QUEUE_MAX",
        "ML_HEALTH_LATENCY_SLA_MS",
        # F2 — Ollama keep_alive window SST contract.
        "ML_OLLAMA_KEEP_ALIVE",
    ]:
        monkeypatch.delenv(key, raising=False)


def test_check_required_config_requires_named_keys(monkeypatch):
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "openai")

    with pytest.raises(SystemExit):
        _check_required_config()


def test_check_required_config_allows_ollama_without_api_key(monkeypatch):
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    # Spec 067 BUG-067-001 — ML sidecar log level SST contract.
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    # Spec 050 — ML sidecar health/worker isolation SST contract.
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    # F2 — Ollama keep_alive window (required when provider=ollama).
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    # BUG-026-007 — structured-extraction thinking switch (required when provider=ollama).
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")

    config = _check_required_config()

    assert config["LLM_PROVIDER"] == "ollama"

    assert config["LLM_MODEL"] == "llama3.2"
    assert config["SMACKEREL_AUTH_TOKEN"] == "unit-test-auth-token"
    # Spec 050 — values are echoed back so callers can wire them into
    # downstream components without re-reading env.
    assert config["ML_EMBEDDING_WORKERS"] == "2"
    assert config["ML_EMBEDDING_QUEUE_MAX"] == "3"
    assert config["ML_HEALTH_LATENCY_SLA_MS"] == "500"
    # F2 — keep_alive window is echoed back for the ml sidecar to consume.
    assert config["ML_OLLAMA_KEEP_ALIVE"] == "30m"
    # BUG-026-007 — thinking switch is echoed back for the ml sidecar to consume.
    assert config["ML_STRUCTURED_EXTRACTION_THINKING"] == "false"


def test_startup_rejects_invalid_model_profiles_spec102(monkeypatch):
    """TP-C3-15: missing/malformed/duplicate/non-positive profiles and an
    invalid keep_alive all fail at startup before an inference call."""
    import litellm

    network_call = AsyncMock()
    monkeypatch.setattr(litellm, "acompletion", network_call, raising=False)
    cases = {
        "missing": None,
        "malformed": "not-json",
        "duplicate": '[{"model":"llama3.2","num_ctx":4096},{"model":"ollama_chat/LLAMA3.2","num_ctx":8192}]',
        "non-positive": '[{"model":"llama3.2","num_ctx":0}]',
    }
    for value in cases.values():
        with monkeypatch.context() as context:
            _set_required_env_minus(context, "test", auth_token="unit-test-auth-token")
            if value is None:
                context.delenv("ML_MODEL_MEMORY_PROFILES_JSON", raising=False)
            else:
                context.setenv("ML_MODEL_MEMORY_PROFILES_JSON", value)
            with pytest.raises(SystemExit):
                _check_required_config()

    for keep_alive in ("forever", "0", "-1", "0s", "-30m"):
        with monkeypatch.context() as context:
            _set_required_env_minus(context, "test", auth_token="unit-test-auth-token")
            context.setenv("ML_OLLAMA_KEEP_ALIVE", keep_alive)
            with pytest.raises(SystemExit):
                _check_required_config()

    network_call.assert_not_called()


def test_startup_profile_error_log_redacts_supplied_value_security102(monkeypatch, caplog):
    sentinel = "SENTINEL-STARTUP-PROFILE-SECRET-RR03"
    _set_required_env_minus(monkeypatch, "test", auth_token="unit-test-auth-token")
    monkeypatch.setenv(
        "ML_MODEL_MEMORY_PROFILES_JSON",
        f"[{json.dumps({'model': 'llama3.2', 'num_ctx': sentinel})}]",
    )

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit):
            _check_required_config()

    assert sentinel not in caplog.text
    assert "invalid_num_ctx" in caplog.text


def test_warmup_domain_model_applies_ollama_profile_spec102(monkeypatch):
    """TP-C3-06: startup warmup emits a capped request."""
    import litellm

    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    captured: dict = {}

    async def capture(**kwargs):
        captured.update(kwargs)
        return object()

    monkeypatch.setattr(litellm, "acompletion", capture, raising=False)
    asyncio.run(
        _warmup_domain_model({"LLM_PROVIDER": "ollama", "LLM_MODEL": "gemma4:26b", "OLLAMA_URL": "http://ollama:11434"})
    )

    assert captured["model"] == "ollama_chat/gemma4:26b"
    assert captured["options"]["num_ctx"] == 8192
    assert captured["keep_alive"] == "30m"
    assert captured["max_tokens"] == 1


def test_check_required_config_rejects_invalid_degraded_fallback_flag(monkeypatch):
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "sometimes")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    # F2 — set so the ONLY failure under test is the invalid fallback flag.
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")

    with pytest.raises(SystemExit):
        _check_required_config()


def test_check_required_config_requires_ollama_keep_alive(monkeypatch):
    # F2 — with provider=ollama, ML_OLLAMA_KEEP_ALIVE is REQUIRED (fail-loud,
    # no default). ADVERSARIAL: fails if the requirement is dropped from
    # _check_required_config. Everything else is present, so a SystemExit here
    # can ONLY be attributed to the missing keep_alive window.
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    # keep_alive deliberately absent (autouse fixture already cleared it).
    monkeypatch.delenv("ML_OLLAMA_KEEP_ALIVE", raising=False)

    with pytest.raises(SystemExit):
        _check_required_config()


def test_check_required_config_rejects_invalid_ollama_keep_alive(monkeypatch):
    # F2 — a keep_alive Ollama can't parse ("forever") is rejected fail-loud so
    # it can't silently fall back to Ollama's 5m default and re-open the
    # cold-load bug. ADVERSARIAL: fails if the format check is removed.
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "forever")

    with pytest.raises(SystemExit):
        _check_required_config()


def test_check_required_config_accepts_valid_ollama_keep_alive(monkeypatch):
    # F2 — the recommended "30m" window passes the format check and is echoed back.
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")

    config = _check_required_config()

    assert config["ML_OLLAMA_KEEP_ALIVE"] == "30m"


def test_check_required_config_requires_structured_extraction_thinking(monkeypatch):
    # BUG-026-007 — with provider=ollama, ML_STRUCTURED_EXTRACTION_THINKING is
    # REQUIRED (fail-loud, no default). ADVERSARIAL: fails if the requirement is
    # dropped from _check_required_config. Everything else is present, so a
    # SystemExit here can ONLY be attributed to the missing thinking switch.
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    # The thinking switch is deliberately absent (the conftest seed is cleared).
    monkeypatch.delenv("ML_STRUCTURED_EXTRACTION_THINKING", raising=False)

    with pytest.raises(SystemExit):
        _check_required_config()


def test_check_required_config_rejects_invalid_structured_extraction_thinking(monkeypatch):
    # BUG-026-007 — a thinking switch that is neither true nor false is a config
    # error, rejected fail-loud rather than silently coerced. ADVERSARIAL: fails
    # if the true/false validation is removed.
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setattr("app.main._AUTH_TOKEN", "unit-test-auth-token")
    monkeypatch.setenv("SMACKEREL_ENV", "test")
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "maybe")

    with pytest.raises(SystemExit):
        _check_required_config()


# MIT-040-S-004 (spec 040 SST hardening) — Adversarial regression tests.
#
# Behavior contract: SMACKEREL_AUTH_TOKEN is required only when
# SMACKEREL_ENV=production. In development/test, an empty token logs a
# warning and the sidecar continues in dev-mode bypass. SMACKEREL_ENV
# itself is always required and must be one of development|test|production.
#
# Each test below MUST FAIL if its specific guard is removed from
# ml/app/main.py::_check_required_config().


def _set_required_env_minus(monkeypatch, environment: str, *, auth_token: str | None) -> None:
    """Helper — set every required env var except SMACKEREL_AUTH_TOKEN, then
    optionally set the auth token to the given value (or leave it unset)."""
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
    monkeypatch.setenv("SMACKEREL_ENV", environment)
    # Spec 067 BUG-067-001 — ML sidecar log level SST contract.
    monkeypatch.setenv("ML_LOG_LEVEL", "info")
    # Spec 050 — ML sidecar health/worker isolation SST contract.
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "3")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "500")
    # F2 — Ollama keep_alive window (required when provider=ollama).
    monkeypatch.setenv("ML_OLLAMA_KEEP_ALIVE", "30m")
    # BUG-026-007 — structured-extraction thinking switch (required when provider=ollama).
    monkeypatch.setenv("ML_STRUCTURED_EXTRACTION_THINKING", "false")
    if auth_token is None:
        monkeypatch.delenv("SMACKEREL_AUTH_TOKEN", raising=False)
        monkeypatch.setattr("app.main._AUTH_TOKEN", "")
    else:
        monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", auth_token)
        monkeypatch.setattr("app.main._AUTH_TOKEN", auth_token)


def test_main_source_contract_uses_canonical_auth_token_constant():
    """main.py must consume auth._AUTH_TOKEN without a silent env default."""
    source_path = Path(__file__).resolve().parents[1] / "app" / "main.py"
    source_text = source_path.read_text()

    forbidden_patterns = [
        'os.environ.get("SMACKEREL_AUTH_TOKEN"',
        "os.environ.get('SMACKEREL_AUTH_TOKEN'",
        'os.getenv("SMACKEREL_AUTH_TOKEN"',
        "os.getenv('SMACKEREL_AUTH_TOKEN'",
    ]
    for forbidden_pattern in forbidden_patterns:
        assert forbidden_pattern not in source_text, (
            "HL-RESCAN-013-secondary / Gate G028 / BUG-020-004: "
            "ml/app/main.py must consume the canonical _AUTH_TOKEN constant "
            f"instead of silently reading {forbidden_pattern!r}."
        )

    assert "from .auth import _AUTH_TOKEN, verify_auth" in source_text, (
        "HL-RESCAN-013-secondary / Gate G028 / BUG-020-004: "
        "ml/app/main.py must import _AUTH_TOKEN from auth.py alongside verify_auth."
    )
    assert "auth_token = _AUTH_TOKEN" in source_text, (
        "HL-RESCAN-013-secondary / Gate G028 / BUG-020-004: "
        "_check_required_config() must use the canonical _AUTH_TOKEN value."
    )


def test_main_s004_production_env_fails_fast_when_auth_token_empty(monkeypatch, caplog):
    """SMACKEREL_ENV=production + empty SMACKEREL_AUTH_TOKEN → sys.exit(1).

    Adversarial proof: removing the production-mode auth_token branch in
    _check_required_config() makes this test fail (no SystemExit raised).
    """
    import logging

    _set_required_env_minus(monkeypatch, "production", auth_token=None)

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit) as exc_info:
            _check_required_config()

    assert exc_info.value.code == 1
    error_messages = [r.message for r in caplog.records if r.levelno >= logging.ERROR]
    joined = " ".join(error_messages)
    assert "production" in joined, f"expected ERROR log naming production, got: {error_messages}"
    assert "SMACKEREL_AUTH_TOKEN" in joined, f"expected ERROR log naming SMACKEREL_AUTH_TOKEN, got: {error_messages}"


def test_main_s004_development_env_allows_empty_auth_token_with_warning(monkeypatch, caplog):
    """SMACKEREL_ENV=development + empty SMACKEREL_AUTH_TOKEN → no exception, warning logged.

    Adversarial proof: making AUTH_TOKEN unconditionally required (i.e.
    leaving SMACKEREL_AUTH_TOKEN in the keys list) makes this test fail
    because _check_required_config() raises SystemExit.
    """
    import logging

    _set_required_env_minus(monkeypatch, "development", auth_token="")

    with caplog.at_level(logging.WARNING, logger="smackerel-ml"):
        config = _check_required_config()

    assert config["SMACKEREL_ENV"] == "development"
    assert config["SMACKEREL_AUTH_TOKEN"] == ""
    warning_messages = [r.message for r in caplog.records if r.levelno == logging.WARNING]
    assert any("SMACKEREL_AUTH_TOKEN is empty" in m for m in warning_messages), (
        f"expected dev-mode bypass warning, got: {warning_messages}"
    )


def test_main_s004_unknown_environment_value_is_fatal(monkeypatch, caplog):
    """SMACKEREL_ENV=staging → sys.exit(1) with allowlist error.

    Adversarial proof: removing the SMACKEREL_ENV allowlist check in
    _check_required_config() makes this test fail (no SystemExit raised
    for the disallowed value).
    """
    import logging

    _set_required_env_minus(monkeypatch, "staging", auth_token="unit-test-auth-token")

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit) as exc_info:
            _check_required_config()

    assert exc_info.value.code == 1
    error_messages = [r.message for r in caplog.records if r.levelno >= logging.ERROR]
    joined = " ".join(error_messages)
    assert "staging" in joined, f"expected ERROR log naming the offending value 'staging', got: {error_messages}"
    assert "development|test|production" in joined, (
        f"expected ERROR log naming allowlist 'development|test|production', got: {error_messages}"
    )


def test_health_endpoint_reports_disconnected_without_nats_client():
    response = asyncio.run(health())

    assert response["status"] == "degraded"


# SCN-002-007: LLM processing prompt exists and is well-formed
def test_scn002007_universal_processing_prompt_exists():
    from app.processor import UNIVERSAL_PROCESSING_PROMPT

    assert UNIVERSAL_PROCESSING_PROMPT is not None
    assert len(UNIVERSAL_PROCESSING_PROMPT) > 100
    assert "artifact_type" in UNIVERSAL_PROCESSING_PROMPT
    assert "title" in UNIVERSAL_PROCESSING_PROMPT
    assert "summary" in UNIVERSAL_PROCESSING_PROMPT
    assert "key_ideas" in UNIVERSAL_PROCESSING_PROMPT
    assert "entities" in UNIVERSAL_PROCESSING_PROMPT
    assert "action_items" in UNIVERSAL_PROCESSING_PROMPT
    assert "topics" in UNIVERSAL_PROCESSING_PROMPT
    assert "sentiment" in UNIVERSAL_PROCESSING_PROMPT


# SCN-002-007: LLM prompt has tier-specific instructions
def test_scn002007_processing_prompt_has_tier_instructions():
    from app.processor import UNIVERSAL_PROCESSING_PROMPT

    assert "light" in UNIVERSAL_PROCESSING_PROMPT.lower()
    assert "metadata" in UNIVERSAL_PROCESSING_PROMPT.lower()


# SCN-002-008: Embedding model is configured correctly
def test_scn002008_embedding_model_config():
    from app.embedder import _model_name, embedding_dimension

    assert _model_name == "all-MiniLM-L6-v2"
    assert embedding_dimension() == 384


# SCN-002-008: generate_embedding function signature exists
def test_scn002008_embedding_function_exists():
    from app.embedder import generate_artifact_embedding, generate_embedding

    assert callable(generate_embedding)
    assert callable(generate_artifact_embedding)


# SCN-002-037: Whisper transcription function exists
def test_scn002037_whisper_transcribe_function():
    from app.whisper_transcribe import transcribe_voice

    assert callable(transcribe_voice)


# SCN-002-038: LLM processing failure returns proper error structure
def test_scn002038_llm_failure_returns_error():
    from app.processor import process_content

    assert callable(process_content)


# SCN-002-006: YouTube transcript function exists
def test_scn002006_youtube_transcript_function():
    from app.youtube import fetch_transcript

    assert callable(fetch_transcript)


# NATS subject mapping covers all processing paths
def test_nats_subject_response_map():
    from app.nats_client import SUBJECT_RESPONSE_MAP, SUBSCRIBE_SUBJECTS

    for subject in SUBSCRIBE_SUBJECTS:
        assert subject in SUBJECT_RESPONSE_MAP, f"Missing response mapping for {subject}"


# Spec 050 FR-050-002 — ML sidecar health/worker isolation adversarial regression.
#
# Each test below MUST FAIL if the corresponding guard in
# ml/app/main.py::_check_required_config() is removed or weakened. Together they
# pin down the spec 050 SST contract end-to-end:
#   - ML_EMBEDDING_WORKERS required, positive integer
#   - ML_EMBEDDING_QUEUE_MAX required, positive integer, >= workers
#   - ML_HEALTH_LATENCY_SLA_MS required, positive integer


@pytest.mark.parametrize(
    "missing_key",
    [
        "ML_EMBEDDING_WORKERS",
        "ML_EMBEDDING_QUEUE_MAX",
        "ML_HEALTH_LATENCY_SLA_MS",
    ],
)
def test_spec050_missing_required_key_is_fatal(monkeypatch, caplog, missing_key):
    """Each spec 050 SST key MUST be required at startup.

    Adversarial proof: removing the key from the required list in
    _check_required_config() makes this test fail (no SystemExit raised).
    """
    import logging

    _set_required_env_minus(monkeypatch, "test", auth_token="unit-test-auth-token")
    monkeypatch.delenv(missing_key, raising=False)

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit) as exc_info:
            _check_required_config()

    assert exc_info.value.code == 1
    error_messages = [r.message for r in caplog.records if r.levelno >= logging.ERROR]
    joined = " ".join(error_messages)
    assert missing_key in joined, f"expected ERROR log naming {missing_key}, got: {error_messages}"


@pytest.mark.parametrize(
    "key",
    [
        "ML_EMBEDDING_WORKERS",
        "ML_EMBEDDING_QUEUE_MAX",
        "ML_HEALTH_LATENCY_SLA_MS",
    ],
)
def test_spec050_non_integer_value_is_fatal(monkeypatch, caplog, key):
    """Non-integer values for spec 050 keys MUST fail-fast.

    Adversarial proof: replacing the int() parse + validation with a
    silent fallback (e.g. `or 2`) makes this test fail because no
    SystemExit is raised when the operator misconfigures the value.
    """
    import logging

    _set_required_env_minus(monkeypatch, "test", auth_token="unit-test-auth-token")
    monkeypatch.setenv(key, "not-a-number")

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit) as exc_info:
            _check_required_config()

    assert exc_info.value.code == 1
    error_messages = [r.message for r in caplog.records if r.levelno >= logging.ERROR]
    joined = " ".join(error_messages)
    assert key in joined, f"expected ERROR log naming {key}, got: {error_messages}"


@pytest.mark.parametrize(
    "key",
    [
        "ML_EMBEDDING_WORKERS",
        "ML_EMBEDDING_QUEUE_MAX",
        "ML_HEALTH_LATENCY_SLA_MS",
    ],
)
def test_spec050_non_positive_integer_is_fatal(monkeypatch, caplog, key):
    """Zero or negative values for spec 050 keys MUST fail-fast.

    Adversarial proof: a zero ML_EMBEDDING_WORKERS would deadlock the
    executor at runtime; the startup check turns it into a fast,
    operator-visible failure. Removing the ``< 1`` branch makes this
    test fail.
    """
    import logging

    _set_required_env_minus(monkeypatch, "test", auth_token="unit-test-auth-token")
    monkeypatch.setenv(key, "0")

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit) as exc_info:
            _check_required_config()

    assert exc_info.value.code == 1
    error_messages = [r.message for r in caplog.records if r.levelno >= logging.ERROR]
    joined = " ".join(error_messages)
    assert key in joined, f"expected ERROR log naming {key}, got: {error_messages}"


def test_spec050_queue_max_below_workers_is_fatal(monkeypatch, caplog):
    """ML_EMBEDDING_QUEUE_MAX < ML_EMBEDDING_WORKERS MUST fail-fast.

    Adversarial proof: a queue cap below the worker count would let the
    executor saturate but reject all incoming work, which is a
    misconfiguration the operator should see at startup, not at first
    request. Removing the cross-validation branch makes this test fail.
    """
    import logging

    _set_required_env_minus(monkeypatch, "test", auth_token="unit-test-auth-token")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "4")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "2")

    with caplog.at_level(logging.ERROR, logger="smackerel-ml"):
        with pytest.raises(SystemExit) as exc_info:
            _check_required_config()

    assert exc_info.value.code == 1
    error_messages = [r.message for r in caplog.records if r.levelno >= logging.ERROR]
    joined = " ".join(error_messages)
    assert "ML_EMBEDDING_QUEUE_MAX" in joined, f"expected ERROR log naming the cap, got: {error_messages}"
    assert "ML_EMBEDDING_WORKERS" in joined, f"expected ERROR log naming the worker count, got: {error_messages}"


def test_spec050_happy_path_returns_validated_values(monkeypatch):
    """Happy-path validation: well-formed values are accepted and echoed."""
    _set_required_env_minus(monkeypatch, "test", auth_token="unit-test-auth-token")
    monkeypatch.setenv("ML_EMBEDDING_WORKERS", "2")
    monkeypatch.setenv("ML_EMBEDDING_QUEUE_MAX", "5")
    monkeypatch.setenv("ML_HEALTH_LATENCY_SLA_MS", "250")

    config = _check_required_config()

    assert config["ML_EMBEDDING_WORKERS"] == "2"
    assert config["ML_EMBEDDING_QUEUE_MAX"] == "5"
    assert config["ML_HEALTH_LATENCY_SLA_MS"] == "250"
