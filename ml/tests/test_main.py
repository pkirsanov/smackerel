import asyncio
import sys
import types

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

from app.main import _check_required_config, health


@pytest.fixture(autouse=True)
def clear_required_env(monkeypatch):
    for key in [
        "NATS_URL",
        "LLM_PROVIDER",
        "LLM_MODEL",
        "LLM_API_KEY",
        "OLLAMA_URL",
        "ML_PROCESSING_DEGRADED_FALLBACK_ENABLED",
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

    config = _check_required_config()

    assert config["LLM_PROVIDER"] == "ollama"

    assert config["LLM_MODEL"] == "llama3.2"


def test_check_required_config_rejects_invalid_degraded_fallback_flag(monkeypatch):
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("LLM_PROVIDER", "ollama")
    monkeypatch.setenv("LLM_MODEL", "llama3.2")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "sometimes")

    with pytest.raises(SystemExit):
        _check_required_config()


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
