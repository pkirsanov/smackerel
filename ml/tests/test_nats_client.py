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
import logging
import os
import sys
import types
from pathlib import Path
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
    OUTGOING_VALIDATION_MODES,
    PUBLISH_SUBJECTS,
    SUBJECT_RESPONSE_MAP,
    SUBSCRIBE_SUBJECTS,
    _validate_outgoing_result,
)
from app.validation import PayloadValidationError  # isort: skip


class _StopConsumer(BaseException):
    """End a one-message consumer-loop test after the next fetch."""


def _single_message_subscription(data):
    message = MagicMock()
    message.data = json.dumps(data).encode()
    message.ack = AsyncMock()
    message.nak = AsyncMock()
    subscription = MagicMock()
    subscription.fetch = AsyncMock(side_effect=[[message], _StopConsumer()])
    return message, subscription


def _crosssource_request(**overrides):
    request = {
        "concept_id": "concept-1",
        "concept_title": "Decision Making",
        "artifacts": [
            {
                "id": "artifact-1",
                "title": "Recommendation",
                "source_type": "email",
                "summary": "A recommendation was made.",
            },
            {
                "id": "artifact-2",
                "title": "Observed decision",
                "source_type": "calendar",
                "summary": "The recommendation influenced a later decision.",
            },
        ],
        "prompt_contract_version": "cross-source-connection-v1",
    }
    request.update(overrides)
    return request


def _external_crosssource_response(**overrides):
    payload = {
        "has_genuine_connection": True,
        "insight_text": "Two independent sources describe one decision.",
        "confidence": 0.91,
    }
    payload.update(overrides)
    return MagicMock(
        choices=[MagicMock(message=MagicMock(content=json.dumps(payload)))],
        model="test-model",
    )


def _run_crosssource_consumer(external_response, client, subscription):
    env = {
        "LLM_PROVIDER": "ollama",
        "LLM_MODEL": "test-model",
        "LLM_API_KEY": "",
        "OLLAMA_URL": "http://ollama.test",
        "PROMPT_CONTRACTS_DIR": "/workspace/config/prompt_contracts",
    }
    with (
        patch.dict(os.environ, env, clear=False),
        patch.object(sys.modules["litellm"], "acompletion", AsyncMock(return_value=external_response)),
        pytest.raises(_StopConsumer),
    ):
        asyncio.run(client._consume_loop("synthesis.crosssource", subscription))


def test_crosssource_dispatch_accepts_valid_concept_response(caplog):
    """SCN-B0255-001: valid concept output must not use artifact validation."""
    client = NATSClient("nats://localhost:4222")
    request = _crosssource_request()
    message, subscription = _single_message_subscription(request)
    client._js = AsyncMock()

    with caplog.at_level(logging.ERROR, logger="smackerel-ml.nats"):
        _run_crosssource_consumer(_external_crosssource_response(), client, subscription)

    client._js.publish.assert_awaited_once()
    assert client._js.publish.await_args.args[0] == "synthesis.crosssource.result"
    published = json.loads(client._js.publish.await_args.args[1])
    assert published["concept_id"] == "concept-1"
    assert published["artifact_ids"] == ["artifact-1", "artifact-2"]
    assert published["confidence"] == 0.91
    assert not any("artifact_id is required" in record.getMessage() for record in caplog.records)
    message.ack.assert_awaited_once()
    message.nak.assert_not_awaited()


@pytest.mark.parametrize(
    ("source", "field", "value"),
    [
        ("request", "concept_id", ""),
        ("external", "has_genuine_connection", 1),
        ("external", "confidence", float("nan")),
        ("external", "confidence", -0.01),
        ("external", "confidence", 1.01),
        ("request", "artifacts", []),
        ("request", "artifacts", [{"id": "artifact-1"}]),
        ("request", "artifacts", [{"id": "artifact-1"}, {"id": ""}]),
        ("request", "artifacts", [{"id": "artifact-1"}, {"id": 2}]),
    ],
)
def test_crosssource_dispatch_rejects_malformed_response_before_publish(source, field, value):
    """SCN-B0255-002: malformed output reaches poison handling before publish."""
    client = NATSClient("nats://localhost:4222")
    request_overrides = {field: value} if source == "request" else {}
    external_overrides = {field: value} if source == "external" else {}
    request = _crosssource_request(**request_overrides)
    message, subscription = _single_message_subscription(request)
    client._js = AsyncMock()

    _run_crosssource_consumer(
        _external_crosssource_response(**external_overrides),
        client,
        subscription,
    )

    client._js.publish.assert_not_awaited()
    message.ack.assert_not_awaited()
    message.nak.assert_awaited_once()


def test_crosssource_dispatch_rejects_missing_concept_id_before_publish():
    """SCN-B0255-002: an absent concept_id is poison, not publishable output."""
    client = NATSClient("nats://localhost:4222")
    request = _crosssource_request()
    del request["concept_id"]
    message, subscription = _single_message_subscription(request)
    client._js = AsyncMock()

    _run_crosssource_consumer(_external_crosssource_response(), client, subscription)

    client._js.publish.assert_not_awaited()
    message.ack.assert_not_awaited()
    message.nak.assert_awaited_once()


def test_crosssource_dispatch_invalid_response_naks_via_real_poison_handler():
    """SCN-B0255-002: the real poison path requests JetStream redelivery."""
    client = NATSClient("nats://localhost:4222")
    request = _crosssource_request(concept_id="")
    message, subscription = _single_message_subscription(request)
    client._js = AsyncMock()

    _run_crosssource_consumer(_external_crosssource_response(), client, subscription)

    client._js.publish.assert_not_awaited()
    message.ack.assert_not_awaited()
    message.nak.assert_awaited_once()


def test_artifact_dispatch_still_requires_artifact_id_before_publish():
    """SCN-B0255-003: artifact mode still enforces the real artifact validator."""
    with pytest.raises(PayloadValidationError, match="artifact_id is required"):
        _validate_outgoing_result("artifacts.process", "artifacts.processed", {"success": True})


def test_digest_dispatch_remains_exempt_from_artifact_validation():
    """SCN-B0255-003: a valid digest without artifact_id still publishes."""
    client = NATSClient("nats://localhost:4222")
    message, subscription = _single_message_subscription({"digest_date": "2026-07-19"})
    client._js = AsyncMock()

    with pytest.raises(_StopConsumer):
        asyncio.run(client._consume_loop("digest.generate", subscription))

    client._js.publish.assert_awaited_once()
    assert client._js.publish.await_args.args[0] == "digest.generated"
    message.ack.assert_awaited_once()
    message.nak.assert_not_awaited()


def test_photo_dispatch_remains_governed_by_photo_contract():
    """SCN-B0255-003: the existing photo handler and validator still publish."""
    client = NATSClient("nats://localhost:4222")
    message, subscription = _single_message_subscription(
        {"request_id": "request-1", "photo_id": "photo-1", "artifact_id": "artifact-1"}
    )
    client._js = AsyncMock()

    with pytest.raises(_StopConsumer):
        asyncio.run(client._consume_loop("photos.classify", subscription))

    client._js.publish.assert_awaited_once()
    assert client._js.publish.await_args.args[0] == "photos.classified"
    published = json.loads(client._js.publish.await_args.args[1])
    assert published["photo_id"] == "photo-1"
    message.ack.assert_awaited_once()
    message.nak.assert_not_awaited()


def test_unknown_subject_remains_acknowledged_without_publish():
    """SCN-B0255-003: unknown subject behavior is unchanged."""
    client = NATSClient("nats://localhost:4222")
    message, subscription = _single_message_subscription({"value": "ignored"})
    client._js = AsyncMock()

    with pytest.raises(_StopConsumer):
        asyncio.run(client._consume_loop("unknown.subject", subscription))

    client._js.publish.assert_not_awaited()
    message.ack.assert_awaited_once()
    message.nak.assert_not_awaited()


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

    def test_init_no_failure_counts_attribute(self):
        """Spec 081 FR-081-004 — _failure_counts removed; num_delivered
        is the sole source of truth for poison-pill detection."""
        client = NATSClient("nats://localhost:4222")
        assert not hasattr(client, "_failure_counts")


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

    def test_every_subscribe_subject_declares_outgoing_validation_mode(self):
        assert set(OUTGOING_VALIDATION_MODES) == set(SUBSCRIBE_SUBJECTS)

    def test_contract_specific_outgoing_validation_modes(self):
        assert OUTGOING_VALIDATION_MODES["artifacts.process"] == "artifact"
        assert OUTGOING_VALIDATION_MODES["synthesis.crosssource"] == "crosssource"
        assert OUTGOING_VALIDATION_MODES["digest.generate"] is None
        assert OUTGOING_VALIDATION_MODES["photos.classify"] == "photo"


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


def test_handle_search_rerank_applies_ollama_profile_spec102():
    """TP-C3-07: rerank retains think=False and transforms a profiled model
    response into ranked artifact IDs."""
    client = NATSClient("nats://localhost:4222")
    captured: dict = {}

    async def capture(**kwargs):
        captured.update(kwargs)
        return MagicMock(
            choices=[
                MagicMock(message=MagicMock(content='{"ranked":[{"index":2,"relevance":"high","explanation":"best"}]}'))
            ]
        )

    fake_litellm = MagicMock(acompletion=capture)
    with patch.dict(sys.modules, {"litellm": fake_litellm}):
        result = asyncio.run(
            client._handle_search_rerank(
                {
                    "query_id": "rerank-102",
                    "query": "best",
                    "candidates": [
                        {"id": "a", "title": "A", "summary": "first"},
                        {"id": "b", "title": "B", "summary": "second"},
                    ],
                },
                "ollama",
                "qwen3:30b-a3b",
                "",
            )
        )

    assert result["ranked_ids"] == ["b"]
    assert captured["model"] == "ollama_chat/qwen3:30b-a3b"
    assert captured["options"]["num_ctx"] == 32768
    assert captured["keep_alive"] == "30m"
    assert captured["think"] is False


def test_handle_digest_generate_applies_ollama_profile_spec102():
    """TP-C3-08: digest uses the Ollama chat route with nested num_ctx,
    top-level keep_alive, and native thinking control."""
    client = NATSClient("nats://localhost:4222")
    captured: dict = {}

    async def capture(**kwargs):
        captured.update(kwargs)
        return MagicMock(choices=[MagicMock(message=MagicMock(content="! Reply to Alice."))])

    fake_litellm = MagicMock(acompletion=capture)
    with patch.dict(sys.modules, {"litellm": fake_litellm}):
        result = asyncio.run(
            client._handle_digest_generate(
                {
                    "digest_date": "2026-07-10",
                    "action_items": [{"text": "Reply", "person": "Alice", "days_waiting": 2}],
                },
                "ollama",
                "qwen3:30b-a3b",
                "",
            )
        )

    assert result["text"] == "! Reply to Alice."
    assert captured["model"] == "ollama_chat/qwen3:30b-a3b"
    assert not captured["model"].startswith("ollama/")
    assert captured["options"]["num_ctx"] == 32768
    assert captured["keep_alive"] == "30m"
    assert captured["think"] is False


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

    _RECONNECT_ENV = {
        "NATS_MAX_RECONNECT_ATTEMPTS": "-1",
        "NATS_RECONNECT_TIME_WAIT_SECONDS": "2",
    }

    def test_connect_passes_auth_token(self):
        """When SMACKEREL_AUTH_TOKEN is set, it is passed to nats.connect.

        HL-RESCAN-013 / Gate G028: auth token now reads from the
        module-level constant ``app.nats_client._AUTH_TOKEN`` (which is
        re-exported from ``app.auth`` and fail-loud-read at module
        import), not from ``os.environ`` at connect time. The test
        therefore patches the constant directly via
        ``patch("app.nats_client._AUTH_TOKEN", ...)``
        instead of mutating ``os.environ``.
        """
        client = NATSClient("nats://localhost:4222")

        mock_connect = AsyncMock()
        mock_nc = MagicMock()
        mock_nc.is_connected = True
        mock_nc.jetstream.return_value = object()
        mock_connect.return_value = mock_nc

        with (
            patch("app.nats_client.nats.connect", mock_connect),
            patch("app.nats_client._AUTH_TOKEN", "secret-token"),
            patch.dict("os.environ", self._RECONNECT_ENV),
        ):
            asyncio.run(client.connect())

        call_kwargs = mock_connect.call_args[1]
        assert call_kwargs["token"] == "secret-token"

    def test_connect_no_token_when_env_empty(self):
        """When SMACKEREL_AUTH_TOKEN is empty, no token kwarg is passed.

        HL-RESCAN-013 / Gate G028: see ``test_connect_passes_auth_token``
        for the rationale on patching ``app.nats_client._AUTH_TOKEN``
        instead of ``os.environ``.
        """
        client = NATSClient("nats://localhost:4222")

        mock_connect = AsyncMock()
        mock_nc = MagicMock()
        mock_nc.is_connected = True
        mock_nc.jetstream.return_value = object()
        mock_connect.return_value = mock_nc

        with (
            patch("app.nats_client.nats.connect", mock_connect),
            patch("app.nats_client._AUTH_TOKEN", ""),
            patch.dict("os.environ", self._RECONNECT_ENV, clear=False),
        ):
            asyncio.run(client.connect())

        call_kwargs = mock_connect.call_args[1]
        assert "token" not in call_kwargs


class TestSecretReadContract:
    """HL-RESCAN-013-secondary / Gate G028 / BUG-020-004 — adversarial
    grep-contract regression that mechanically locks the absence of
    the FORBIDDEN silent-default os.environ.get("SMACKEREL_AUTH_TOKEN", "")
    form (and the os.getenv equivalent) from ml/app/nats_client.py source.

    Reverting the fix in ml/app/nats_client.py — i.e. re-introducing
    `auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")` anywhere
    in the file — would cause this test to FAIL with the failure
    message naming BUG-020-004, so a future maintainer can navigate
    back to this packet for context. The canonical fail-loud pattern
    is `from .auth import _AUTH_TOKEN` followed by `if _AUTH_TOKEN:
    connect_opts["token"] = _AUTH_TOKEN` (per design.md DD-1, DD-2).
    """

    def test_no_environ_get_smackerel_auth_token_in_nats_client_source(self):
        """Open ml/app/nats_client.py from disk and assert the FORBIDDEN
        substrings ``os.environ.get("SMACKEREL_AUTH_TOKEN"`` and
        ``os.getenv("SMACKEREL_AUTH_TOKEN"`` are BOTH absent. Source is
        read via ``pathlib.Path(...).read_text()`` (not ``inspect.getsource``)
        so the check catches comments and docstrings too. Failure message
        names HL-RESCAN-013-secondary, Gate G028, and BUG-020-004 so a
        future maintainer can navigate back to this packet (per FROZEN
        design DD-4 + DD-7 + DD-9).
        """
        source_path = Path(__file__).resolve().parents[1] / "app" / "nats_client.py"
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
                "ml/app/nats_client.py must consume the canonical _AUTH_TOKEN "
                f"constant instead of silently reading {forbidden_pattern!r}."
            )

        assert source_text.count("from .auth import _AUTH_TOKEN") == 1, (
            "HL-RESCAN-013-secondary / Gate G028 / BUG-020-004: "
            "ml/app/nats_client.py must import the canonical fail-loud _AUTH_TOKEN once."
        )
        assert source_text.count("if _AUTH_TOKEN:") == 1, (
            "HL-RESCAN-013-secondary / Gate G028 / BUG-020-004: NATS auth branching must be driven by _AUTH_TOKEN."
        )
        assert source_text.count('connect_opts["token"] = _AUTH_TOKEN') == 1, (
            "HL-RESCAN-013-secondary / Gate G028 / BUG-020-004: "
            "NATS connect options must pass the canonical _AUTH_TOKEN value."
        )


# ---------------------------------------------------------------------------
# Spec 046 FR-046-001 — ML sidecar indefinite reconnect contract
# ---------------------------------------------------------------------------


class TestConnectReconnectContract:
    """Spec 046 FR-046-001 — ML sidecar reconnect behavior is SST-driven and indefinite.

    These tests prove the env-driven SST plumbing for NATS_MAX_RECONNECT_ATTEMPTS
    and NATS_RECONNECT_TIME_WAIT_SECONDS. They are adversarial regression tests:
    if anyone reverts to a hardcoded finite value (e.g. max_reconnect_attempts=60
    which existed before spec 046), test_connect_passes_indefinite_reconnect_from_env
    fails because the env value would no longer reach nats.connect().
    """

    _BASE_ENV = {
        "SMACKEREL_AUTH_TOKEN": "",
        "NATS_MAX_RECONNECT_ATTEMPTS": "-1",
        "NATS_RECONNECT_TIME_WAIT_SECONDS": "2",
    }

    def _mock_nats(self):
        mock_connect = AsyncMock()
        mock_nc = MagicMock()
        mock_nc.is_connected = True
        mock_nc.jetstream.return_value = object()
        mock_connect.return_value = mock_nc
        return mock_connect

    def test_connect_passes_indefinite_reconnect_from_env(self):
        """SST value of -1 MUST be propagated to nats.connect()."""
        client = NATSClient("nats://localhost:4222")
        mock_connect = self._mock_nats()

        with patch("app.nats_client.nats.connect", mock_connect):
            with patch.dict("os.environ", self._BASE_ENV, clear=False):
                asyncio.run(client.connect())

        call_kwargs = mock_connect.call_args[1]
        assert call_kwargs["max_reconnect_attempts"] == -1, (
            "ML sidecar MUST configure indefinite reconnect "
            "(max_reconnect_attempts=-1) per spec 046 FR-046-001 — "
            "regression detected: hardcoded finite value at module level. "
            f"Got: {call_kwargs['max_reconnect_attempts']!r}"
        )

    def test_connect_passes_reconnect_time_wait_from_env(self):
        """SST value for reconnect_time_wait_seconds MUST be propagated."""
        client = NATSClient("nats://localhost:4222")
        mock_connect = self._mock_nats()

        env = dict(self._BASE_ENV)
        env["NATS_RECONNECT_TIME_WAIT_SECONDS"] = "5"
        with patch("app.nats_client.nats.connect", mock_connect):
            with patch.dict("os.environ", env, clear=False):
                asyncio.run(client.connect())

        call_kwargs = mock_connect.call_args[1]
        assert call_kwargs["reconnect_time_wait"] == 5

    def test_connect_honors_env_value_not_module_constant(self):
        """Adversarial: even an unusual env value must reach nats.connect.

        This catches regressions where someone hardcodes a value at module
        scope. If the env says 99 but the code passes 60, this test fails.
        """
        client = NATSClient("nats://localhost:4222")
        mock_connect = self._mock_nats()

        env = dict(self._BASE_ENV)
        env["NATS_MAX_RECONNECT_ATTEMPTS"] = "99"
        with patch("app.nats_client.nats.connect", mock_connect):
            with patch.dict("os.environ", env, clear=False):
                asyncio.run(client.connect())

        assert mock_connect.call_args[1]["max_reconnect_attempts"] == 99

    def test_connect_fails_loud_when_max_reconnect_attempts_missing(self, monkeypatch):
        """Missing env var → RuntimeError naming the key. NO silent default."""
        client = NATSClient("nats://localhost:4222")
        monkeypatch.delenv("NATS_MAX_RECONNECT_ATTEMPTS", raising=False)
        monkeypatch.setenv("NATS_RECONNECT_TIME_WAIT_SECONDS", "2")
        monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "")

        with pytest.raises(RuntimeError, match="NATS_MAX_RECONNECT_ATTEMPTS"):
            asyncio.run(client.connect())

    def test_connect_fails_loud_when_reconnect_time_wait_missing(self, monkeypatch):
        client = NATSClient("nats://localhost:4222")
        monkeypatch.setenv("NATS_MAX_RECONNECT_ATTEMPTS", "-1")
        monkeypatch.delenv("NATS_RECONNECT_TIME_WAIT_SECONDS", raising=False)
        monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "")

        with pytest.raises(RuntimeError, match="NATS_RECONNECT_TIME_WAIT_SECONDS"):
            asyncio.run(client.connect())

    def test_connect_fails_loud_on_non_integer_max_reconnect_attempts(self, monkeypatch):
        client = NATSClient("nats://localhost:4222")
        monkeypatch.setenv("NATS_MAX_RECONNECT_ATTEMPTS", "not-a-number")
        monkeypatch.setenv("NATS_RECONNECT_TIME_WAIT_SECONDS", "2")
        monkeypatch.setenv("SMACKEREL_AUTH_TOKEN", "")

        with pytest.raises(RuntimeError, match="NATS_MAX_RECONNECT_ATTEMPTS"):
            asyncio.run(client.connect())
