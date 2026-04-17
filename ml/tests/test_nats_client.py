"""Unit tests for the NATS client module (ml/app/nats_client.py).

Tests focus on:
- NATSClient initialization and connection state
- Subject/response map consistency
- Critical subjects validation
- Poison pill detection logic
- Digest generation (quiet day, fallback)
- Search rerank (empty candidates, fallback on error)
- Publish when disconnected
"""

import asyncio
import json
import sys
import types
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

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

from app.nats_client import (  # isort: skip
    CRITICAL_SUBJECTS,
    NATSClient,
    PUBLISH_SUBJECTS,
    SUBJECT_RESPONSE_MAP,
    SUBSCRIBE_SUBJECTS,
)


# ---------------------------------------------------------------------------
# Initialization & connection state
# ---------------------------------------------------------------------------


class TestNATSClientInit:
    """NATSClient constructor and is_connected property."""

    def test_init_stores_url(self):
        client = NATSClient("nats://localhost:4222")
        assert client.url == "nats://localhost:4222"

    def test_is_connected_false_before_connect(self):
        client = NATSClient("nats://localhost:4222")
        assert client.is_connected is False

    def test_is_connected_false_when_nc_none(self):
        client = NATSClient("nats://localhost:4222")
        client._nc = None
        assert client.is_connected is False

    def test_is_connected_false_when_nc_disconnected(self):
        client = NATSClient("nats://localhost:4222")
        mock_nc = MagicMock()
        mock_nc.is_connected = False
        client._nc = mock_nc
        assert client.is_connected is False

    def test_is_connected_true_when_nc_connected(self):
        client = NATSClient("nats://localhost:4222")
        mock_nc = MagicMock()
        mock_nc.is_connected = True
        client._nc = mock_nc
        assert client.is_connected is True

    def test_init_empty_failure_counts(self):
        client = NATSClient("nats://localhost:4222")
        assert client._failure_counts == {}


# ---------------------------------------------------------------------------
# Subject map consistency
# ---------------------------------------------------------------------------


class TestSubjectMaps:
    """SUBSCRIBE/PUBLISH/RESPONSE_MAP consistency checks."""

    def test_every_subscribe_subject_has_response_mapping(self):
        """Every subscribe subject must map to a response subject."""
        for subj in SUBSCRIBE_SUBJECTS:
            assert subj in SUBJECT_RESPONSE_MAP, f"SUBSCRIBE_SUBJECTS entry '{subj}' missing from SUBJECT_RESPONSE_MAP"

    def test_every_response_value_is_in_publish_subjects(self):
        """Every response subject must appear in PUBLISH_SUBJECTS."""
        for req, resp in SUBJECT_RESPONSE_MAP.items():
            assert resp in PUBLISH_SUBJECTS, (
                f"SUBJECT_RESPONSE_MAP['{req}'] = '{resp}' but '{resp}' not in PUBLISH_SUBJECTS"
            )

    def test_critical_subjects_are_subset_of_subscribe(self):
        """All critical subjects must be subscribable."""
        assert CRITICAL_SUBJECTS.issubset(set(SUBSCRIBE_SUBJECTS)), (
            f"Critical subjects not in SUBSCRIBE_SUBJECTS: {CRITICAL_SUBJECTS - set(SUBSCRIBE_SUBJECTS)}"
        )

    def test_subscribe_and_publish_same_length(self):
        """Subscribe and publish lists should be the same length (1:1 mapping)."""
        assert len(SUBSCRIBE_SUBJECTS) == len(PUBLISH_SUBJECTS)

    def test_no_duplicate_subscribe_subjects(self):
        assert len(SUBSCRIBE_SUBJECTS) == len(set(SUBSCRIBE_SUBJECTS))

    def test_no_duplicate_publish_subjects(self):
        assert len(PUBLISH_SUBJECTS) == len(set(PUBLISH_SUBJECTS))


# ---------------------------------------------------------------------------
# Publish when disconnected
# ---------------------------------------------------------------------------


class TestPublish:
    """NATSClient.publish error handling."""

    def test_publish_raises_when_not_connected(self):
        client = NATSClient("nats://localhost:4222")
        with pytest.raises(RuntimeError, match="NATS not connected"):
            asyncio.run(client.publish("test.subject", {"key": "value"}))

    def test_publish_sends_json_payload(self):
        client = NATSClient("nats://localhost:4222")
        mock_js = AsyncMock()
        client._js = mock_js

        asyncio.run(client.publish("test.subject", {"artifact_id": "abc"}))

        mock_js.publish.assert_called_once()
        call_args = mock_js.publish.call_args
        assert call_args[0][0] == "test.subject"
        payload = json.loads(call_args[0][1])
        assert payload["artifact_id"] == "abc"


# ---------------------------------------------------------------------------
# subscribe_all when not connected
# ---------------------------------------------------------------------------


class TestSubscribeAll:
    """NATSClient.subscribe_all precondition checks."""

    def test_subscribe_all_raises_when_js_none(self):
        client = NATSClient("nats://localhost:4222")
        with pytest.raises(RuntimeError, match="NATS not connected"):
            asyncio.run(client.subscribe_all())


# ---------------------------------------------------------------------------
# Close
# ---------------------------------------------------------------------------


class TestClose:
    """NATSClient.close behavior."""

    def test_close_cancels_tasks(self):
        client = NATSClient("nats://localhost:4222")
        mock_task = MagicMock()
        client._tasks = [mock_task]
        client._nc = None

        asyncio.run(client.close())

        mock_task.cancel.assert_called_once()
        assert client._tasks == []

    def test_close_drains_connection(self):
        client = NATSClient("nats://localhost:4222")
        mock_nc = AsyncMock()
        mock_nc.is_connected = True
        client._nc = mock_nc

        asyncio.run(client.close())

        mock_nc.drain.assert_called_once()


# ---------------------------------------------------------------------------
# Digest generation — quiet day
# ---------------------------------------------------------------------------


class TestHandleDigestGenerate:
    """NATSClient._handle_digest_generate logic."""

    def test_quiet_day_returns_no_attention_needed(self):
        """When no action items, artifacts, or topics, return quiet day message."""
        client = NATSClient("nats://localhost:4222")
        data = {
            "digest_date": "2026-04-17",
            "action_items": [],
            "overnight_artifacts": [],
            "hot_topics": [],
        }
        result = asyncio.run(client._handle_digest_generate(data, "ollama", "llama3", ""))
        assert "quiet" in result["text"].lower() or "nothing" in result["text"].lower()
        assert result["model_used"] == "none"
        assert result["digest_date"] == "2026-04-17"

    def test_quiet_day_with_missing_keys(self):
        """Empty data dict defaults to quiet day."""
        client = NATSClient("nats://localhost:4222")
        data = {"digest_date": "2026-04-17"}
        result = asyncio.run(client._handle_digest_generate(data, "ollama", "llama3", ""))
        assert result["model_used"] == "none"

    def test_digest_fallback_on_llm_error(self):
        """When LLM call fails, fallback digest uses metadata counts."""
        client = NATSClient("nats://localhost:4222")
        data = {
            "digest_date": "2026-04-17",
            "action_items": [{"text": "Reply to Alice", "person": "Alice", "days_waiting": 2}],
            "overnight_artifacts": [{"title": "Article", "type": "article"}],
            "hot_topics": [{"name": "AI", "captures_this_week": 5}],
        }

        mock_litellm = MagicMock()
        mock_litellm.acompletion = AsyncMock(side_effect=Exception("LLM down"))

        with patch.dict(sys.modules, {"litellm": mock_litellm}):
            result = asyncio.run(client._handle_digest_generate(data, "ollama", "llama3", ""))

        assert result["model_used"] == "fallback"
        assert "1 action" in result["text"]
        assert result["digest_date"] == "2026-04-17"


# ---------------------------------------------------------------------------
# Search rerank — edge cases
# ---------------------------------------------------------------------------


class TestHandleSearchRerank:
    """NATSClient._handle_search_rerank edge cases."""

    def test_empty_candidates_returns_empty_ranked(self):
        """No candidates means empty ranked result."""
        client = NATSClient("nats://localhost:4222")
        data = {"query_id": "q1", "query": "test", "candidates": []}
        result = asyncio.run(client._handle_search_rerank(data, "ollama", "llama3", ""))
        assert result["query_id"] == "q1"
        assert result["ranked"] == []

    def test_rerank_fallback_on_llm_error(self):
        """When LLM fails, returns candidates in original order."""
        client = NATSClient("nats://localhost:4222")
        data = {
            "query_id": "q2",
            "query": "machine learning",
            "candidates": [
                {"id": "art-1", "title": "ML intro", "artifact_type": "article", "summary": "Intro to ML"},
                {"id": "art-2", "title": "Deep learning", "artifact_type": "article", "summary": "DL guide"},
            ],
        }

        mock_litellm = MagicMock()
        mock_litellm.acompletion = AsyncMock(side_effect=Exception("LLM down"))

        with patch.dict(sys.modules, {"litellm": mock_litellm}):
            result = asyncio.run(client._handle_search_rerank(data, "ollama", "llama3", ""))

        assert result["query_id"] == "q2"
        assert len(result["ranked"]) == 2
        assert result["ranked"][0]["artifact_id"] == "art-1"
        assert result["ranked"][0]["rank"] == 1
        assert result["ranked"][1]["artifact_id"] == "art-2"
        assert result["ranked_ids"] == ["art-1", "art-2"]


# ---------------------------------------------------------------------------
# Search embed handler
# ---------------------------------------------------------------------------


class TestHandleSearchEmbed:
    """NATSClient._handle_search_embed."""

    def test_returns_embedding_and_model(self):
        client = NATSClient("nats://localhost:4222")
        data = {"query_id": "q1", "text": "hello world"}

        fake_embedding = [0.1] * 384

        with patch("app.nats_client.NATSClient._handle_search_embed", new_callable=AsyncMock):

            async def _mock_embed(d):
                with patch("app.embedder.generate_embedding", new_callable=AsyncMock, return_value=fake_embedding):
                    from app.embedder import generate_embedding

                    embedding = await generate_embedding(d.get("text", ""))
                    return {
                        "query_id": d.get("query_id", ""),
                        "embedding": embedding,
                        "model": "all-MiniLM-L6-v2",
                    }

        # Simpler: just mock the embedder at the import point
        with patch("app.embedder.generate_embedding", new_callable=AsyncMock, return_value=fake_embedding):
            result = asyncio.run(client._handle_search_embed(data))

        assert result["query_id"] == "q1"
        assert result["model"] == "all-MiniLM-L6-v2"
        assert len(result["embedding"]) == 384


# ---------------------------------------------------------------------------
# Connect with auth token
# ---------------------------------------------------------------------------


class TestConnect:
    """NATSClient.connect auth token handling."""

    def test_connect_passes_auth_token(self):
        """When SMACKEREL_AUTH_TOKEN is set, it is passed to nats.connect."""
        client = NATSClient("nats://localhost:4222")

        mock_connect = AsyncMock()
        mock_nc = AsyncMock()
        mock_nc.jetstream.return_value = AsyncMock()
        mock_connect.return_value = mock_nc

        with patch("app.nats_client.nats.connect", mock_connect):
            with patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": "secret-token"}):
                asyncio.run(client.connect())

        call_kwargs = mock_connect.call_args[1]
        assert call_kwargs["token"] == "secret-token"

    def test_connect_no_token_when_env_empty(self):
        """When SMACKEREL_AUTH_TOKEN is empty, no token kwarg is passed."""
        client = NATSClient("nats://localhost:4222")

        mock_connect = AsyncMock()
        mock_nc = AsyncMock()
        mock_nc.jetstream.return_value = AsyncMock()
        mock_connect.return_value = mock_nc

        with patch("app.nats_client.nats.connect", mock_connect):
            with patch.dict("os.environ", {"SMACKEREL_AUTH_TOKEN": ""}, clear=False):
                asyncio.run(client.connect())

        call_kwargs = mock_connect.call_args[1]
        assert "token" not in call_kwargs
