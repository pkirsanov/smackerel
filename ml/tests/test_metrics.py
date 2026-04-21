"""Unit tests for ML sidecar Prometheus metrics (ml/app/metrics.py).

Tests cover:
- sanitize_model cardinality bounding (known models, unknown → "other")
- llm_tokens_used counter increments with labels
- processing_latency histogram observations
- /metrics endpoint returns Prometheus format
"""

import sys
import types

import pytest

# Ensure litellm mock is in place before importing app code
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})
    sys.modules["litellm.exceptions"] = _mock_exc

from app.metrics import (
    llm_tokens_used,
    processing_latency,
    sanitize_model,
)


class TestSanitizeModel:
    """Cardinality bounding: known models pass through, unknown → 'other'."""

    @pytest.mark.parametrize(
        "model",
        [
            "llama3.2",
            "llama3.1",
            "llama3",
            "mistral",
            "mixtral",
            "gpt-4",
            "gpt-4o",
            "gpt-3.5-turbo",
            "claude-3-opus",
            "claude-3-sonnet",
        ],
    )
    def test_known_models_pass_through(self, model: str) -> None:
        assert sanitize_model(model) == model

    def test_unknown_model_mapped_to_other(self) -> None:
        assert sanitize_model("custom-finetune-v7") == "other"

    def test_empty_string_mapped_to_other(self) -> None:
        assert sanitize_model("") == "other"

    def test_case_sensitive(self) -> None:
        # Known models set is exact-match; uppercase should map to "other"
        assert sanitize_model("GPT-4") == "other"


class TestLLMTokensUsedCounter:
    """smackerel_llm_tokens_used_total counter increments correctly."""

    def test_increment_with_labels(self) -> None:
        before = llm_tokens_used.labels(provider="ollama", model="llama3.2")._value.get()
        llm_tokens_used.labels(provider="ollama", model="llama3.2").inc(150)
        after = llm_tokens_used.labels(provider="ollama", model="llama3.2")._value.get()
        assert after - before == 150

    def test_different_labels_independent(self) -> None:
        llm_tokens_used.labels(provider="openai", model="gpt-4").inc(500)
        llm_tokens_used.labels(provider="openai", model="gpt-4o").inc(200)
        gpt4_val = llm_tokens_used.labels(provider="openai", model="gpt-4")._value.get()
        gpt4o_val = llm_tokens_used.labels(provider="openai", model="gpt-4o")._value.get()
        assert gpt4_val >= 500
        assert gpt4o_val >= 200
        # They must be independently tracked
        assert gpt4_val != gpt4o_val


class TestProcessingLatencyHistogram:
    """smackerel_ml_processing_latency_seconds histogram records observations."""

    def test_observe_records_sample(self) -> None:
        before = processing_latency.labels(operation="artifacts.process")._sum.get()
        processing_latency.labels(operation="artifacts.process").observe(1.5)
        after = processing_latency.labels(operation="artifacts.process")._sum.get()
        assert after - before == pytest.approx(1.5)

    def test_multiple_observations_accumulate(self) -> None:
        before_sum = processing_latency.labels(operation="search.embed")._sum.get()
        processing_latency.labels(operation="search.embed").observe(0.5)
        processing_latency.labels(operation="search.embed").observe(1.0)
        after_sum = processing_latency.labels(operation="search.embed")._sum.get()
        assert after_sum - before_sum == pytest.approx(1.5)


class TestMetricsEndpoint:
    """GET /metrics returns Prometheus-format output."""

    @pytest.fixture
    def client(self):
        from starlette.testclient import TestClient

        from app.main import app

        return TestClient(app)

    def test_metrics_returns_200(self, client) -> None:
        response = client.get("/metrics")
        assert response.status_code == 200

    def test_metrics_content_type(self, client) -> None:
        response = client.get("/metrics")
        ct = response.headers.get("content-type", "")
        assert "text/plain" in ct or "text/openmetrics" in ct

    def test_metrics_contains_llm_counter(self, client) -> None:
        # Ensure at least one sample exists
        llm_tokens_used.labels(provider="test", model="test").inc(1)
        response = client.get("/metrics")
        assert "smackerel_llm_tokens_used_total" in response.text

    def test_metrics_contains_processing_latency(self, client) -> None:
        processing_latency.labels(operation="test").observe(0.1)
        response = client.get("/metrics")
        assert "smackerel_ml_processing_latency_seconds" in response.text

    def test_metrics_unauthenticated(self, client) -> None:
        # /metrics must be accessible without auth header
        response = client.get("/metrics")
        assert response.status_code == 200
