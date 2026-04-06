import asyncio

import pytest

from app.main import _check_required_config, health


@pytest.fixture(autouse=True)
def clear_required_env(monkeypatch):
    for key in ["NATS_URL", "LLM_PROVIDER", "LLM_MODEL", "LLM_API_KEY", "OLLAMA_URL"]:
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

    config = _check_required_config()

    assert config["LLM_PROVIDER"] == "ollama"
    assert config["LLM_MODEL"] == "llama3.2"


def test_health_endpoint_reports_disconnected_without_nats_client():
    response = asyncio.run(health())

    assert response["status"] == "degraded"