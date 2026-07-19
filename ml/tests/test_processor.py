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
        assert result["model_used"] == "ollama_chat/llama3"

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
        """Ollama routes via the ollama_chat/ (/api/chat) prefix (F2 keep_alive)."""
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

        assert result["model_used"] == "ollama_chat/llama3"


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

    def test_missing_artifact_type_degrades_to_default(self):
        """BUG-061-002: LLM omitting 'artifact_type' no longer hard-fails.

        Short / low-signal inputs and the prompt's "light" tier legitimately
        cause the model to omit artifact_type. The processor MUST derive a
        default from content_type (or 'note' for generic) instead of raising
        ValueError and silently dropping the capture.
        """
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

        assert result["success"] is True
        assert result["result"]["artifact_type"] == "article"
        assert result["result"]["title"] == "Missing artifact_type"

    def test_missing_title_degrades_to_default(self):
        """BUG-061-002: LLM omitting 'title' no longer hard-fails."""
        llm_result = {"artifact_type": "article"}
        mock_response = _make_llm_response(llm_result)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="A meaningful capture about supper plans.",
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
        assert result["result"]["artifact_type"] == "article"
        # title derived from first 100 chars of content
        assert result["result"]["title"] == "A meaningful capture about supper plans."

    def test_bug_061_002_short_text_with_partial_llm_payload_does_not_silently_drop(self):
        """ADVERSARIAL REGRESSION (BUG-061-002): short text + partial LLM
        payload (no artifact_type, no title) must NOT return
        success=False. Pre-fix this returned the opaque
        {'success': False, 'error': 'LLM processing failed'} with a
        ValueError at processor.py:178 swallowed by the outer except.

        Adversarial property: the asserted shape (success=True, derived
        title from input, derived artifact_type from generic→note) is the
        EXACT shape that the broken code path could NEVER produce — the
        broken path always returned success=False with no 'result' key.
        """
        llm_partial = {
            "summary": "A brief greeting.",
            "topics": ["greeting"],
            "sentiment": "neutral",
        }
        mock_response = _make_llm_response(llm_partial)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="hi",
                    content_type="generic",
                    source_id="s1",
                    processing_tier="light",
                    user_context="",
                    model="gemma3:4b",
                    api_key="k",
                    provider="ollama",
                )
            )

        # Pre-fix: success was False and 'result' key was absent.
        assert result["success"] is True, f"Pre-fix regression: short text caused silent drop. Got: {result!r}"
        assert "result" in result, "Pre-fix regression: no 'result' key"
        # Title derived from short content
        assert result["result"]["title"] == "hi"
        # artifact_type derived: generic content_type → 'note'
        assert result["result"]["artifact_type"] == "note"
        # Other LLM-supplied fields preserved
        assert result["result"]["summary"] == "A brief greeting."
        assert result["result"]["topics"] == ["greeting"]

    def test_bug_061_002_empty_content_derives_untitled(self):
        """ADVERSARIAL REGRESSION (BUG-061-002): truly empty content with
        partial LLM payload derives 'Untitled' rather than raising."""
        llm_partial = {"summary": "Empty input."}
        mock_response = _make_llm_response(llm_partial)

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="",
                    content_type="generic",
                    source_id="s2",
                    processing_tier="light",
                    user_context="",
                    model="m",
                    api_key="k",
                    provider="ollama",
                )
            )

        assert result["success"] is True
        assert result["result"]["title"] == "Untitled"
        assert result["result"]["artifact_type"] == "note"

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

    def test_malformed_json_uses_sst_gated_degraded_fallback(self, monkeypatch):
        """redteam F2 / BUG-026-006 adversarial regression: a TRUNCATED LLM
        payload ("Unterminated string") preserves the capture via the SST-gated
        degraded fallback instead of hard-dropping it.

        FAILS against the pre-fix code, whose except-JSONDecodeError branch
        returned {"success": False} regardless of the SST gate, silently
        dropping the capture.
        """
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "true")
        message = MagicMock()
        # Truncated mid-string: no closing brace to salvage -> JSONDecodeError.
        message.content = '{"artifact_type": "article", "title": "Unterminated stri'
        choice = MagicMock()
        choice.message = message
        mock_response = MagicMock()
        mock_response.choices = [choice]
        mock_response.usage = None

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="A meaningful capture the model truncated.",
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
        assert result["result"]["topics"] == ["degraded-fallback-malformed-json"]
        assert result["result"]["artifact_type"] == "article"

    def test_malformed_json_hard_fails_when_fallback_disabled(self, monkeypatch):
        """SST-gate integrity: when the degraded fallback is DISABLED, a
        malformed LLM payload still returns a hard error (no silent success)."""
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
        message = MagicMock()
        message.content = '{"artifact_type": "article", "title": "Unterminated stri'
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

        assert result["success"] is False
        assert "Invalid JSON" in result["error"]

    def test_json_with_prose_wrapper_is_salvaged(self):
        """redteam F2 / BUG-026-006: a valid JSON object wrapped in a prose
        preamble/suffix (which litellm's json_object mode does not suppress for
        every Ollama model) is salvaged and parsed with its real fields.

        FAILS against the pre-fix code, which fed the whole prose string to
        json.loads and hard-failed with JSONDecodeError.
        """
        message = MagicMock()
        message.content = (
            "Sure, here is the JSON you requested:\n"
            '{"artifact_type": "recipe", "title": "Pizza Dough", '
            '"summary": "A simple dough recipe."}\n'
            "Hope that helps!"
        )
        choice = MagicMock()
        choice.message = message
        mock_response = MagicMock()
        mock_response.choices = [choice]
        mock_response.usage = None

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="Ingredients: flour, water.",
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
        assert result["result"]["artifact_type"] == "recipe"
        assert result["result"]["title"] == "Pizza Dough"

    def test_none_llm_content_uses_sst_gated_degraded_fallback(self, monkeypatch):
        """redteam F2 / BUG-026-006 adversarial regression: an EMPTY / None LLM
        message content (some Ollama-served models return content=None on an
        overrun/aborted generation) preserves the capture via the SST-gated
        degraded fallback instead of hard-dropping it.

        FAILS against the pre-completion code, whose _parse_llm_json(None) raised
        a TypeError (json.loads(None)) that bypassed the except-JSONDecodeError
        degraded-fallback branch and fell through to the generic "LLM processing
        failed" hard drop (the TypeError is not an unavailable-LLM error, so the
        unavailable-branch fallback did not catch it either).
        """
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "true")
        message = MagicMock()
        # Model returned no content at all (None), not even an empty string.
        message.content = None
        choice = MagicMock()
        choice.message = message
        mock_response = MagicMock()
        mock_response.choices = [choice]
        mock_response.usage = None

        with patch("app.processor.litellm") as mock_litellm:
            mock_litellm.acompletion = AsyncMock(return_value=mock_response)

            result = asyncio.run(
                process_content(
                    content="A meaningful capture the model returned empty for.",
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
        assert result["result"]["topics"] == ["degraded-fallback-malformed-json"]
        assert result["result"]["artifact_type"] == "article"
        # The capture is preserved: the title is derived from the raw content,
        # not dropped.
        assert result["result"]["title"] == "A meaningful capture the model returned empty for."

    def test_none_llm_content_hard_fails_when_fallback_disabled(self, monkeypatch):
        """SST-gate integrity: when the degraded fallback is DISABLED, a None /
        empty LLM content still returns a hard error (no silent success) and is
        classified as the same Invalid-JSON failure as a truncated payload —
        never the opaque generic "LLM processing failed"."""
        monkeypatch.setenv("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED", "false")
        message = MagicMock()
        message.content = None
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


def test_output_budget_read_from_sst_not_hardcoded_spec102(monkeypatch):
    """SCN-102-C3-05 (BUG-026-006) — the domain/synthesis output-token budget is
    read from SST (ML_DOMAIN_OUTPUT_TOKEN_BUDGET), NOT the hardcoded 2000.

    A distinct non-2000 value proves the max_tokens flows from SST; the
    ADVERSARIAL assert (!= 2000) fails if the literal 2000 is ever re-hardcoded
    back into ml/app/processor.py.
    """
    import asyncio
    import json
    from unittest.mock import AsyncMock, MagicMock, patch

    monkeypatch.setenv("ML_DOMAIN_OUTPUT_TOKEN_BUDGET", "1234")  # distinct, != 2000
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    from app.processor import process_content

    captured: dict = {}

    def _capture(**kwargs):
        captured.update(kwargs)
        resp = MagicMock()
        resp.choices = [MagicMock(message=MagicMock(content=json.dumps({"artifact_type": "article", "title": "T"})))]
        resp.usage = MagicMock(total_tokens=20)
        return resp

    with patch("app.processor.litellm") as mock_litellm:
        mock_litellm.acompletion = AsyncMock(side_effect=_capture)
        result = asyncio.run(
            process_content(
                content="hello world",
                content_type="article",
                source_id="s1",
                processing_tier="standard",
                user_context="",
                model="gemma4:26b",
                api_key="",
                provider="ollama",
            )
        )

    assert result["success"] is True
    assert captured["max_tokens"] == 1234, (
        "SCN-102-C3-05: max_tokens must be the SST ML_DOMAIN_OUTPUT_TOKEN_BUDGET "
        f"(1234), got {captured.get('max_tokens')}"
    )
    # ADVERSARIAL: a re-hardcoded 2000 would flip this assertion red.
    assert captured["max_tokens"] != 2000


def test_process_content_applies_ollama_profile_spec102(monkeypatch):
    """TP-C3-10: processor directly emits the selected profile while
    preserving output budget, response format, timeout, and think=False."""
    import asyncio
    import json
    from unittest.mock import AsyncMock, MagicMock, patch

    monkeypatch.setenv("ML_DOMAIN_OUTPUT_TOKEN_BUDGET", "1234")
    monkeypatch.setenv("OLLAMA_URL", "http://ollama:11434")
    from app.processor import process_content

    captured: dict = {}

    def capture(**kwargs):
        captured.update(kwargs)
        response = MagicMock()
        response.choices = [MagicMock(message=MagicMock(content=json.dumps({"artifact_type": "note", "title": "T"})))]
        response.usage = MagicMock(total_tokens=9)
        return response

    with patch("app.processor.litellm.acompletion", new_callable=AsyncMock, side_effect=capture):
        result = asyncio.run(process_content("hello", "note", "source", "standard", "", "qwen3:30b-a3b", "", "ollama"))

    assert result["success"] is True
    assert result["result"]["artifact_type"] == "note"
    assert captured["options"]["num_ctx"] == 32768
    assert captured["keep_alive"] == "30m"
    assert captured["think"] is False
    assert captured["max_tokens"] == 1234
    assert captured["response_format"] == {"type": "json_object"}
    assert captured["timeout"] == 600
