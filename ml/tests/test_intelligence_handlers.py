"""Tests for Phase 5 intelligence handlers."""

import asyncio
from unittest.mock import patch

import pytest

from app.intelligence import (
    handle_content_analyze,
    handle_learning_classify,
    handle_monthly_generate,
    handle_quickref_generate,
    handle_seasonal_analyze,
)


class TestLearningClassify:
    """Tests for learning.classify handler."""

    def test_heuristic_beginner(self):
        result = asyncio.run(
            handle_learning_classify(
                {"artifact_id": "a1", "title": "Introduction to Python", "summary": "Getting started with Python"},
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["difficulty"] == "beginner"
        assert result["artifact_id"] == "a1"

    def test_heuristic_advanced(self):
        result = asyncio.run(
            handle_learning_classify(
                {"artifact_id": "a2", "title": "Advanced Go Internals", "summary": "Deep dive into Go runtime"},
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["difficulty"] == "advanced"

    def test_heuristic_intermediate_default(self):
        result = asyncio.run(
            handle_learning_classify(
                {"artifact_id": "a3", "title": "Building REST APIs", "summary": "A practical guide"},
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["difficulty"] == "intermediate"

    def test_empty_data(self):
        result = asyncio.run(handle_learning_classify({}, None, None, None))
        assert result["success"] is True
        assert result["difficulty"] == "intermediate"
        assert result["artifact_id"] == ""

    def test_has_processing_time(self):
        result = asyncio.run(
            handle_learning_classify(
                {"artifact_id": "a4", "title": "Test"},
                None,
                None,
                None,
            )
        )
        assert "processing_time_ms" in result
        assert isinstance(result["processing_time_ms"], int)


class TestContentAnalyze:
    """Tests for content.analyze handler."""

    def test_fallback_basic_angle(self):
        result = asyncio.run(
            handle_content_analyze(
                {"topic_id": "t1", "topic_name": "Python", "capture_count": 35, "source_diversity": 5},
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["topic_id"] == "t1"
        assert len(result["angles"]) >= 1
        assert "Python" in result["angles"][0]["title"]

    def test_fallback_format_blog_post(self):
        result = asyncio.run(
            handle_content_analyze(
                {"topic_id": "t1", "topic_name": "Go", "capture_count": 35, "source_diversity": 3},
                None,
                None,
                None,
            )
        )
        assert result["angles"][0]["format_suggestion"] == "blog post"

    def test_fallback_format_detailed_guide(self):
        result = asyncio.run(
            handle_content_analyze(
                {"topic_id": "t1", "topic_name": "Go", "capture_count": 60, "source_diversity": 3},
                None,
                None,
                None,
            )
        )
        assert result["angles"][0]["format_suggestion"] == "detailed guide"

    def test_fallback_format_essay(self):
        result = asyncio.run(
            handle_content_analyze(
                {"topic_id": "t1", "topic_name": "Go", "capture_count": 150, "source_diversity": 8},
                None,
                None,
                None,
            )
        )
        assert result["angles"][0]["format_suggestion"] == "long-form essay"

    def test_empty_data(self):
        result = asyncio.run(handle_content_analyze({}, None, None, None))
        assert result["success"] is True


class TestMonthlyGenerate:
    """Tests for monthly.generate handler."""

    def test_fallback_no_provider(self):
        result = asyncio.run(
            handle_monthly_generate(
                {"month": "2026-04", "expertise_shifts": [{"topic_name": "Go", "direction": "gained"}]},
                None,
                None,
                None,
            )
        )
        assert result["success"] is False
        assert result["report_text"] == ""

    def test_has_processing_time(self):
        result = asyncio.run(handle_monthly_generate({}, None, None, None))
        assert "processing_time_ms" in result


class TestQuickrefGenerate:
    """Tests for quickref.generate handler."""

    def test_fallback_with_sources(self):
        result = asyncio.run(
            handle_quickref_generate(
                {
                    "concept": "TypeScript generics",
                    "source_artifacts": [
                        {"id": "a1", "title": "TS Generics Guide", "summary": "How to use generics in TypeScript"},
                        {"id": "a2", "title": "Advanced TS", "summary": "Deep type patterns"},
                    ],
                },
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["concept"] == "TypeScript generics"
        assert "a1" in result["source_artifact_ids"]
        assert "a2" in result["source_artifact_ids"]
        assert "TS Generics Guide" in result["content"]

    def test_fallback_no_sources(self):
        result = asyncio.run(
            handle_quickref_generate(
                {"concept": "Python decorators", "source_artifacts": []},
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert "Python decorators" in result["content"]

    def test_empty_data(self):
        result = asyncio.run(handle_quickref_generate({}, None, None, None))
        assert result["success"] is True


class TestSeasonalAnalyze:
    """Tests for seasonal.analyze handler (BUG-021-009: LLM-judged significance)."""

    def test_no_llm_returns_empty_observations(self):
        # With raw signals but no LLM configured, there is NO hardcoded ratio
        # fallback — the handler returns zero observations.
        result = asyncio.run(
            handle_seasonal_analyze(
                {
                    "current_month": "January",
                    "data_days": 400,
                    "this_month_count": 120,
                    "last_year_same_month_count": 40,
                    "topic_candidates": [{"name": "taxes", "count": 9}],
                },
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["observations"] == []

    def test_no_signals_returns_empty(self):
        result = asyncio.run(
            handle_seasonal_analyze(
                {
                    "current_month": "March",
                    "data_days": 365,
                    "this_month_count": 0,
                    "last_year_same_month_count": 0,
                    "topic_candidates": [],
                },
                None,
                None,
                None,
            )
        )
        assert result["success"] is True
        assert result["observations"] == []

    def test_response_shape_has_observations_key(self):
        result = asyncio.run(
            handle_seasonal_analyze(
                {"current_month": "April"},
                None,
                None,
                None,
            )
        )
        assert "observations" in result
        assert isinstance(result["observations"], list)
        assert result["success"] is True


class _FakeHTTPResponse:
    def __init__(self, payload):
        self.status_code = 200
        self._payload = payload

    def json(self):
        return self._payload


class _CapturingAsyncClient:
    captured: dict = {}

    def __init__(self, **kwargs):
        self.kwargs = kwargs

    async def __aenter__(self):
        return self

    async def __aexit__(self, *args):
        return None

    async def post(self, url, **kwargs):
        type(self).captured = {"url": url, **kwargs}
        if url.endswith("/api/generate"):
            return _FakeHTTPResponse(
                {"response": '[{"title":"Angle","uniqueness_rationale":"Evidence","format_suggestion":"blog post"}]'}
            )
        return _FakeHTTPResponse({"choices": [{"message": {"content": '[{"title":"Hosted"}]'}}]})


def test_synthesis_generator_generate_applies_native_ollama_profile_spec102():
    """TP-C3-05: intelligence's shared native generator emits a profiled
    /api/generate payload and refuses to restore a hardcoded model fallback."""
    _CapturingAsyncClient.captured = {}
    with patch("httpx.AsyncClient", _CapturingAsyncClient):
        result = asyncio.run(
            handle_content_analyze(
                {"topic_id": "topic-102", "topic_name": "Ollama", "capture_count": 10, "source_diversity": 3},
                "ollama",
                "gemma4:26b",
                "configured",
                "http://ollama:11434",
            )
        )

    payload = _CapturingAsyncClient.captured["json"]
    assert result["success"] is True
    assert _CapturingAsyncClient.captured["url"].endswith("/api/generate")
    assert payload["model"] == "gemma4:26b"
    assert payload["options"]["num_ctx"] == 8192
    assert payload["keep_alive"] == "30m"
    assert payload["stream"] is False

    from app.intelligence import _call_llm

    with pytest.raises(RuntimeError, match="model is required"):
        asyncio.run(_call_llm("prompt", "ollama", "", "", "http://ollama:11434"))


def test_hosted_intelligence_carries_no_ollama_profile_options_spec102():
    """TP-C3-20: hosted intelligence never receives Ollama-only fields."""
    _CapturingAsyncClient.captured = {}
    with patch("httpx.AsyncClient", _CapturingAsyncClient):
        asyncio.run(
            handle_content_analyze(
                {"topic_id": "hosted-102", "topic_name": "Hosted", "capture_count": 4, "source_diversity": 2},
                "openai",
                "gpt-4o-mini",
                "secret-not-logged",
            )
        )

    payload = _CapturingAsyncClient.captured["json"]
    assert _CapturingAsyncClient.captured["url"] == "https://api.openai.com/v1/chat/completions"
    assert payload["model"] == "gpt-4o-mini"
    assert "options" not in payload
    assert "keep_alive" not in payload
