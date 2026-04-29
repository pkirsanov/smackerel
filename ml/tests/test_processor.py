"""Unit tests for the LLM processor module (ml/app/processor.py).

Tests focus on:
- Content truncation
- Prompt formatting with all parameters
- Successful LLM response parsing with required/optional fields
- Missing required fields in LLM response
- Invalid JSON from LLM
- Retry logic on transient errors
- Default value population for optional fields
- Provider prefix logic
"""

import asyncio
import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

# Ensure litellm mock is in place before importing app code
if "litellm" not in sys.modules:
    _mock_litellm = types.ModuleType("litellm")
    _mock_litellm.acompletion = None  # type: ignore[attr-defined]
    sys.modules["litellm"] = _mock_litellm

    _mock_exc = types.ModuleType("litellm.exceptions")
    _mock_exc.RateLimitError = type("RateLimitError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.ServiceUnavailableError = type("ServiceUnavailableError", (Exception,), {})  # type: ignore[attr-defined]
    _mock_exc.InternalServerError = type("InternalServerError", (Exception,), {})  # type: ignore[attr-defined]
    sys.modules["litellm.exceptions"] = _mock_exc

from app.processor import UNIVERSAL_PROCESSING_PROMPT, process_content


def _make_llm_response(result_dict: dict, total_tokens: int = 100) -> MagicMock:
    """Build a mock litellm response object."""
    message = MagicMock()
    message.content = json.dumps(result_dict)

    choice = MagicMock()
    choice.message = message

    usage = MagicMock()
    usage.total_tokens = total_tokens

    response = MagicMock()
    response.choices = [choice]
    response.usage = usage
    return response


# ---------------------------------------------------------------------------
# Prompt formatting
# ---------------------------------------------------------------------------


class TestPromptFormatting:
    """UNIVERSAL_PROCESSING_PROMPT format string validation."""

    def test_prompt_has_all_placeholders(self):
        """Prompt contains all expected format placeholders."""
        for placeholder in ["content_type", "source_id", "processing_tier", "user_context", "content"]:
            assert f"{{{placeholder}}}" in UNIVERSAL_PROCESSING_PROMPT

    def test_prompt_format_succeeds(self):
        """Prompt can be formatted without errors."""
        result = UNIVERSAL_PROCESSING_PROMPT.format(
            content_type="article",
            source_id="src-1",
            processing_tier="standard",
            user_context="none",
            content="Test content",
        )
        assert "article" in result
        assert "src-1" in result
        assert "Test content" in result


# ---------------------------------------------------------------------------
# Successful processing
# ---------------------------------------------------------------------------


class TestProcessContentSuccess:
    """process_content happy path."""

    def test_returns_success_with_all_fields(self):
        """Full LLM response with all fields produces success result."""
        llm_result = {
            "artifact_type": "article",
            "title": "Test Article",
            "summary": "A summary of the article content.",
            "key_ideas": ["idea 1", "idea 2"],
            "entities": {
                "people": ["Alice"],
                "orgs": ["Acme"],
                "places": [],
                "products": [],
                "dates": [],
            },
            "action_items": [],
            "topics": ["testing"],
            "sentiment": "positive",
            "temporal_relevance": {"relevant_from": None, "relevant_until": None},
            "source_quality": "high",
        }
        mock_response = _make_llm_response(llm_result, total_tokens=250)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="Test article body",
                    content_type="article",
                    source_id="src-1",
                    processing_tier="standard",
                    user_context="",
                    model="llama3",
                    api_key="",
                    provider="ollama",
                )
            )

        assert result["success"] is True
        assert result["result"]["title"] == "Test Article"
        assert result["result"]["artifact_type"] == "article"
        assert result["tokens_used"] == 250
        assert result["model_used"] == "ollama/llama3"

    def test_defaults_populated_for_optional_fields(self):
        """When LLM returns only required fields, defaults are populated."""
        llm_result = {
            "artifact_type": "note",
            "title": "Quick Note",
        }
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="A quick note",
                    content_type="note",
                    source_id="src-2",
                    processing_tier="light",
                    user_context="personal",
                    model="gpt-4",
                    api_key="key",
                    provider="openai",
                )
            )

        assert result["success"] is True
        r = result["result"]
        assert r["summary"] == ""
        assert r["key_ideas"] == []
        assert r["action_items"] == []
        assert r["topics"] == []
        assert r["sentiment"] == "neutral"
        assert r["source_quality"] == "medium"
        assert "people" in r["entities"]

    def test_openai_provider_no_prefix(self):
        """OpenAI provider does not get a model prefix."""
        llm_result = {"artifact_type": "article", "title": "T"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="gpt-4",
                    api_key="key",
                    provider="openai",
                )
            )

        assert result["model_used"] == "gpt-4"

    def test_empty_provider_no_prefix(self):
        """Empty provider string does not get a model prefix."""
        llm_result = {"artifact_type": "article", "title": "T"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="gpt-4",
                    api_key="key",
                    provider="",
                )
            )

        assert result["model_used"] == "gpt-4"

    def test_non_openai_provider_gets_prefix(self):
        """Non-openai providers get provider/ prefix on model name."""
        llm_result = {"artifact_type": "article", "title": "T"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="llama3",
                    api_key="",
                    provider="ollama",
                )
            )

        assert result["model_used"] == "ollama/llama3"


# ---------------------------------------------------------------------------
# Content truncation
# ---------------------------------------------------------------------------


class TestContentTruncation:
    """Content is truncated to 15000 characters."""

    def test_long_content_is_truncated(self):
        """Content longer than 15000 chars is truncated before sending to LLM."""
        long_content = "A" * 20000
        llm_result = {"artifact_type": "article", "title": "T"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            asyncio.run(
                process_content(
                    content=long_content,
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

            # Verify the prompt sent to LLM has truncated content
            call_args = mock_litellm.acompletion.call_args
            prompt_text = call_args[1]["messages"][0]["content"]
            # The content section should not contain the full 20000 chars
            assert "A" * 15000 in prompt_text
            assert "A" * 15001 not in prompt_text


# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------


class TestProcessContentErrors:
    """process_content error paths."""

    def test_missing_required_field_returns_error(self):
        """LLM response missing 'artifact_type' produces ValueError."""
        llm_result = {"title": "Missing artifact_type"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is False
        assert "error" in result

    def test_missing_title_returns_error(self):
        """LLM response missing 'title' produces ValueError."""
        llm_result = {"artifact_type": "article"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is False

    def test_invalid_json_returns_error(self):
        """LLM returning non-JSON text produces an error result."""
        message = MagicMock()
        message.content = "This is not valid JSON at all"
        choice = MagicMock()
        choice.message = message
        mock_response = MagicMock()
        mock_response.choices = [choice]

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is False
        assert "Invalid JSON" in result["error"]

    def test_total_llm_failure_returns_error(self, monkeypatch):
        """LLM connection failures fail when degraded fallback is disabled."""
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(side_effect=ConnectionError("network down"))

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is False

    def test_connection_failure_uses_sst_gated_degraded_fallback(self, monkeypatch):
        """LLM connection failures can only return fallback success when SST enables it."""
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "true")
        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(side_effect=ConnectionError("connection refused"))

            result = asyncio.run(
                process_content(
                    content="fallback content",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is True
        assert result["model_used"] == "fallback"
        assert result["result"]["topics"] == ["degraded-fallback"]

    def test_degraded_fallback_preserves_domain_content_type(self, monkeypatch):
        """Domain-eligible captures keep their content type when universal LLM fallback runs."""
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "true")
        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(side_effect=ConnectionError("connection refused"))

            result = asyncio.run(
                process_content(
                    content="Ingredients: dough, tomato sauce. Instructions: bake until crisp.",
                    content_type="recipe",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is True
        assert result["model_used"] == "fallback"
        assert result["result"]["artifact_type"] == "recipe"

    def test_degraded_fallback_maps_generic_to_note(self, monkeypatch):
        """Generic captures still get a concrete note type under fallback."""
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "true")
        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(side_effect=ConnectionError("connection refused"))

            result = asyncio.run(
                process_content(
                    content="A generic note",
                    content_type="generic",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is True
        assert result["result"]["artifact_type"] == "note"


# ---------------------------------------------------------------------------
# Retry logic
# ---------------------------------------------------------------------------


class TestRetryLogic:
    """Exponential backoff retries on transient errors."""

    def test_retries_on_rate_limit_then_succeeds(self):
        """RateLimitError triggers retries; success on second attempt."""
        llm_result = {"artifact_type": "article", "title": "Recovered"}
        mock_response = _make_llm_response(llm_result)

        RateLimitError = sys.modules["litellm.exceptions"].RateLimitError

        call_count = 0

        async def _mock_acompletion(**kwargs):
            nonlocal call_count
            call_count += 1
            if call_count == 1:
                raise RateLimitError("rate limited")
            return mock_response

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = _mock_acompletion
            # Patch asyncio.sleep to avoid real delays
            with patch("app.processor.asyncio.sleep", new_callable=AsyncMock):
                result = asyncio.run(
                    process_content(
                        content="x",
                        content_type="article",
                        source_id="s",
                        processing_tier="standard",
                        user_context="",
                        model="m",
                        api_key="k",
                        provider="ollama",
                    )
                )

        assert result["success"] is True
        assert result["result"]["title"] == "Recovered"
        assert call_count == 2

    def test_exhausted_retries_returns_error(self):
        """All 3 attempts failing on transient error returns error result."""
        RateLimitError = sys.modules["litellm.exceptions"].RateLimitError

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(side_effect=RateLimitError("always rate limited"))
            with patch("app.processor.asyncio.sleep", new_callable=AsyncMock):
                result = asyncio.run(
                    process_content(
                        content="x",
                        content_type="article",
                        source_id="s",
                        processing_tier="standard",
                        user_context="",
                        model="m",
                        api_key="k",
                        provider="ollama",
                    )
                )

                assert result["success"] is False
                assert "error" in result


# ---------------------------------------------------------------------------
# User context handling
# ---------------------------------------------------------------------------


class TestUserContext:
    """User context parameter handling."""

    def test_empty_user_context_becomes_none(self):
        """Empty user_context is replaced with 'none' in prompt."""
        llm_result = {"artifact_type": "article", "title": "T"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

            prompt_text = mock_litellm.acompletion.call_args[1]["messages"][0]["content"]
            assert "User context: none" in prompt_text

    def test_provided_user_context_appears_in_prompt(self):
        """Non-empty user_context is passed through to prompt."""
        llm_result = {"artifact_type": "article", "title": "T"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="personal research",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

            prompt_text = mock_litellm.acompletion.call_args[1]["messages"][0]["content"]
            assert "User context: personal research" in prompt_text


# ---------------------------------------------------------------------------
# Token usage when usage is None
# ---------------------------------------------------------------------------


class TestTokenUsage:
    """Token usage edge cases."""

    def test_tokens_zero_when_usage_none(self):
        """When response.usage is None, tokens_used should be 0."""
        llm_result = {"artifact_type": "article", "title": "T"}

        message = MagicMock()
        message.content = json.dumps(llm_result)
        choice = MagicMock()
        choice.message = message
        mock_response = MagicMock()
        mock_response.choices = [choice]
        mock_response.usage = None

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="x",
                    content_type="article",
                    source_id="s",
                    processing_tier="standard",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["tokens_used"] == 0
