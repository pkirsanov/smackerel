"""NATS JetStream client for the ML sidecar."""

import asyncio
import json
import logging
import os
import time
from datetime import datetime, timezone

import httpx
import nats
from nats.aio.client import Client as NATSConn
from nats.js.api import ConsumerConfig
from nats.js.client import JetStreamContext

# HL-RESCAN-013 / Gate G028 (NO-DEFAULTS / fail-loud SST policy) — re-use
# the canonical fail-loud-read module-level constant from auth.py instead
# of re-reading os.environ here. auth.py raises RuntimeError at import
# time if SMACKEREL_AUTH_TOKEN is unset, so by the time this module is
# imported the constant is guaranteed to be defined (empty string is the
# dev-mode auth-bypass signal honoured by both verify_auth() and the
# NATS server's no-auth dev mode).
from .auth import _AUTH_TOKEN
from .metrics import (
    llm_tokens_used,
    nats_consume_fetch_errors_total,
    nats_deadletter_publish_failures_total,
    nats_deadletter_total,
    processing_latency,
    sanitize_model,
)
from .url_validator import validate_fetch_url
from .validation import (
    PayloadValidationError,
    validate_process_payload,
    validate_processed_result,
)

logger = logging.getLogger("smackerel-ml.nats")

# Subjects this sidecar subscribes to
SUBSCRIBE_SUBJECTS = [
    "artifacts.process",
    "search.embed",
    "search.rerank",
    "digest.generate",
    "keep.sync.request",
    "keep.ocr.request",
    "learning.classify",
    "content.analyze",
    "monthly.generate",
    "quickref.generate",
    "seasonal.analyze",
    "synthesis.extract",
    "synthesis.crosssource",
    "domain.extract",
    "photos.classify",
    "photos.ocr",
    "photos.embed",
    "photos.lifecycle",
    "photos.dedupe",
    "photos.sensitivity",
    "photos.aesthetic",
    "photos.removal.review",
    "agent.invoke.request",
    "drive.extract.request",
    "drive.classify.request",
]

# Subjects this sidecar publishes to
PUBLISH_SUBJECTS = [
    "artifacts.processed",
    "search.embedded",
    "search.reranked",
    "digest.generated",
    "keep.sync.response",
    "keep.ocr.response",
    "learning.classified",
    "content.analyzed",
    "monthly.generated",
    "quickref.generated",
    "seasonal.analyzed",
    "synthesis.extracted",
    "synthesis.crosssource.result",
    "domain.extracted",
    "photos.classified",
    "photos.ocred",
    "photos.embedded",
    "photos.lifecycle.result",
    "photos.dedupe.result",
    "photos.sensitivity.result",
    "photos.aesthetic.result",
    "photos.removal.reviewed",
    "agent.invoke.response",
    "drive.extract.result",
    "drive.classify.result",
]

# Map of subscribe subject -> publish response subject
SUBJECT_RESPONSE_MAP = {
    "artifacts.process": "artifacts.processed",
    "search.embed": "search.embedded",
    "search.rerank": "search.reranked",
    "digest.generate": "digest.generated",
    "keep.sync.request": "keep.sync.response",
    "keep.ocr.request": "keep.ocr.response",
    "learning.classify": "learning.classified",
    "content.analyze": "content.analyzed",
    "monthly.generate": "monthly.generated",
    "quickref.generate": "quickref.generated",
    "seasonal.analyze": "seasonal.analyzed",
    "synthesis.extract": "synthesis.extracted",
    "synthesis.crosssource": "synthesis.crosssource.result",
    "domain.extract": "domain.extracted",
    "photos.classify": "photos.classified",
    "photos.ocr": "photos.ocred",
    "photos.embed": "photos.embedded",
    "photos.lifecycle": "photos.lifecycle.result",
    "photos.dedupe": "photos.dedupe.result",
    "photos.sensitivity": "photos.sensitivity.result",
    "photos.aesthetic": "photos.aesthetic.result",
    "photos.removal.review": "photos.removal.reviewed",
    "agent.invoke.request": "agent.invoke.response",
    "drive.extract.request": "drive.extract.result",
    "drive.classify.request": "drive.classify.result",
}


# Subjects that are critical — failure to subscribe is fatal
CRITICAL_SUBJECTS = {"artifacts.process", "search.embed", "synthesis.extract", "photos.classify", "photos.embed"}


# Spec 081 FR-081-003 / design §3.1 — subject → JetStream stream lookup.
# Mirrors internal/nats/client.go::AllStreams subject bindings so the
# Smackerel-Original-Stream header on dead-letter messages matches the
# Go envelope byte-for-byte. Missing entry MUST fail loud at module
# import (fail-loud parity with the SST philosophy).
SUBJECT_TO_STREAM = {
    "artifacts.process": "ARTIFACTS",
    "search.embed": "SEARCH",
    "search.rerank": "SEARCH",
    "digest.generate": "DIGEST",
    "keep.sync.request": "KEEP",
    "keep.ocr.request": "KEEP",
    "learning.classify": "INTELLIGENCE",
    "content.analyze": "INTELLIGENCE",
    "monthly.generate": "INTELLIGENCE",
    "quickref.generate": "INTELLIGENCE",
    "seasonal.analyze": "INTELLIGENCE",
    "synthesis.extract": "SYNTHESIS",
    "synthesis.crosssource": "SYNTHESIS",
    "domain.extract": "DOMAIN",
    "photos.classify": "PHOTOS",
    "photos.ocr": "PHOTOS",
    "photos.embed": "PHOTOS",
    "photos.lifecycle": "PHOTOS",
    "photos.dedupe": "PHOTOS",
    "photos.sensitivity": "PHOTOS",
    "photos.aesthetic": "PHOTOS",
    "photos.removal.review": "PHOTOS",
    "agent.invoke.request": "AGENT",
    "drive.extract.request": "DRIVE",
    "drive.classify.request": "DRIVE",
}

_missing_stream_subjects = [s for s in SUBSCRIBE_SUBJECTS if s not in SUBJECT_TO_STREAM]
if _missing_stream_subjects:
    raise RuntimeError(
        "SUBJECT_TO_STREAM is missing entries for subjects "
        f"{_missing_stream_subjects} — every SUBSCRIBE_SUBJECTS entry MUST "
        "resolve to the JetStream stream declared in "
        "internal/nats/client.go::AllStreams (spec 081 design §3.1)."
    )


def _utf8_truncate(s: str, max_bytes: int) -> str:
    """Truncate a string to at most max_bytes when UTF-8 encoded.

    Mirrors the Go-side stringutil.TruncateUTF8 invariant: never split
    a multi-byte codepoint at the tail. Implementation: encode, slice
    on byte boundary, decode with errors='ignore' which drops any
    partial trailing codepoint.
    """
    encoded = s.encode("utf-8")
    if len(encoded) <= max_bytes:
        return s
    return encoded[:max_bytes].decode("utf-8", errors="ignore")


class NATSClient:
    """Manages NATS JetStream connection and subscriptions for the ML sidecar."""

    def __init__(self, url: str) -> None:
        self.url = url
        self._nc: NATSConn | None = None
        self._js: JetStreamContext | None = None
        self._subscriptions: list = []
        self._tasks: list[asyncio.Task] = []
        # Spec 081 FR-081-001 — populated by subscribe_all() from the
        # SST-required NATS_CONSUMER_MAX_DELIVER / _ACK_WAIT_SECONDS
        # env vars. Cached on the instance so _consume_loop can read
        # max_deliver without re-reading env on every message.
        self._consumer_max_deliver: int | None = None
        self._consumer_ack_wait_seconds: int | None = None

    @property
    def is_connected(self) -> bool:
        return self._nc is not None and self._nc.is_connected

    async def connect(self) -> None:
        """Connect to NATS and initialize JetStream.

        Spec 046 FR-046-001 — reconnect parameters MUST flow from SST. The
        ML sidecar runs in always-on deployment and MUST survive transient
        NATS restarts indefinitely, so ``max_reconnect_attempts`` is set to
        ``-1`` in ``infrastructure.nats.client.max_reconnect_attempts`` in
        ``config/smackerel.yaml``. Missing or non-integer SST values fail
        loud here; there is no hidden default.
        """
        # Read SST-resolved reconnect contract. Both keys are REQUIRED — the
        # generator (scripts/commands/config.sh) writes them into the env
        # file. Empty/missing values would be a deployment misconfiguration.
        try:
            raw_max = os.environ["NATS_MAX_RECONNECT_ATTEMPTS"]
        except KeyError as exc:
            raise RuntimeError(
                "NATS_MAX_RECONNECT_ATTEMPTS is required (spec 046 FR-046-001) — "
                "set infrastructure.nats.client.max_reconnect_attempts in "
                "config/smackerel.yaml (use -1 for indefinite reconnect) and "
                "run `./smackerel.sh config generate`."
            ) from exc
        try:
            max_reconnect_attempts = int(raw_max)
        except ValueError as exc:
            raise RuntimeError(f"NATS_MAX_RECONNECT_ATTEMPTS must be an integer; got {raw_max!r}") from exc

        try:
            raw_wait = os.environ["NATS_RECONNECT_TIME_WAIT_SECONDS"]
        except KeyError as exc:
            raise RuntimeError(
                "NATS_RECONNECT_TIME_WAIT_SECONDS is required (spec 046 FR-046-001) — "
                "set infrastructure.nats.client.reconnect_time_wait_seconds in "
                "config/smackerel.yaml and run `./smackerel.sh config generate`."
            ) from exc
        try:
            reconnect_time_wait = int(raw_wait)
        except ValueError as exc:
            raise RuntimeError(f"NATS_RECONNECT_TIME_WAIT_SECONDS must be an integer; got {raw_wait!r}") from exc

        connect_opts: dict = dict(
            servers=[self.url],
            name="smackerel-ml",
            reconnect_time_wait=reconnect_time_wait,
            max_reconnect_attempts=max_reconnect_attempts,
            disconnected_cb=self._on_disconnect,
            reconnected_cb=self._on_reconnect,
        )
        # Token authentication — mirrors Go core's NATS auth enforcement.
        # HL-RESCAN-013 / Gate G028: re-use the canonical fail-loud-read
        # _AUTH_TOKEN constant from auth.py (which raises RuntimeError at
        # import if SMACKEREL_AUTH_TOKEN is unset). Empty-string here is
        # the legitimate dev-mode auth-bypass signal — the NATS connect
        # call simply omits the `token` kwarg and the dev NATS server
        # accepts the connection without auth.
        if _AUTH_TOKEN:
            connect_opts["token"] = _AUTH_TOKEN

        self._nc = await nats.connect(**connect_opts)
        self._js = self._nc.jetstream()
        logger.info(
            "Connected to NATS at %s (max_reconnect_attempts=%d, reconnect_time_wait=%ds)",
            self.url,
            max_reconnect_attempts,
            reconnect_time_wait,
        )

    async def subscribe_all(self) -> None:
        """Subscribe to all processing subjects and start consumer loops.

        Retries each subscription with exponential backoff in case the
        JetStream streams have not been created yet (e.g. core runtime
        is still initialising after a fresh-volume start).
        """
        if not self._js:
            raise RuntimeError("NATS not connected")

        # Spec 081 FR-081-001 / FR-081-002 — read consumer contract ONCE
        # at the top of subscribe_all (design §4.1); cache on the
        # instance and thread a single ConsumerConfig(...) into every
        # pull_subscribe call. No per-subject overrides; no re-reads
        # inside _consume_loop. Mirrors the spec 046 fail-loud pattern
        # in connect().
        try:
            raw_max_deliver = os.environ["NATS_CONSUMER_MAX_DELIVER"]
        except KeyError as exc:
            raise RuntimeError(
                "NATS_CONSUMER_MAX_DELIVER is required (spec 081 FR-081-001) — "
                "set infrastructure.nats.consumer.max_deliver in "
                "config/smackerel.yaml and run `./smackerel.sh config generate`."
            ) from exc
        try:
            max_deliver = int(raw_max_deliver)
        except ValueError as exc:
            raise RuntimeError(f"NATS_CONSUMER_MAX_DELIVER must be an integer; got {raw_max_deliver!r}") from exc
        if max_deliver < 1:
            raise RuntimeError(f"NATS_CONSUMER_MAX_DELIVER must be >= 1; got {max_deliver}")

        try:
            raw_ack_wait = os.environ["NATS_CONSUMER_ACK_WAIT_SECONDS"]
        except KeyError as exc:
            raise RuntimeError(
                "NATS_CONSUMER_ACK_WAIT_SECONDS is required (spec 081 FR-081-002) — "
                "set infrastructure.nats.consumer.ack_wait_seconds in "
                "config/smackerel.yaml and run `./smackerel.sh config generate`."
            ) from exc
        try:
            ack_wait_seconds = int(raw_ack_wait)
        except ValueError as exc:
            raise RuntimeError(f"NATS_CONSUMER_ACK_WAIT_SECONDS must be an integer; got {raw_ack_wait!r}") from exc
        if ack_wait_seconds < 1:
            raise RuntimeError(f"NATS_CONSUMER_ACK_WAIT_SECONDS must be >= 1; got {ack_wait_seconds}")

        self._consumer_max_deliver = max_deliver
        self._consumer_ack_wait_seconds = ack_wait_seconds

        # nats-py expects ack_wait as nanoseconds (Go-style time.Duration).
        consumer_config = ConsumerConfig(
            max_deliver=max_deliver,
            ack_wait=ack_wait_seconds * 1_000_000_000,
        )

        max_attempts = 30
        base_delay = 1.0  # seconds
        max_delay = 15.0

        for subject in SUBSCRIBE_SUBJECTS:
            sub = None
            for attempt in range(1, max_attempts + 1):
                try:
                    sub = await self._js.pull_subscribe(
                        subject,
                        durable=f"smackerel-ml-{subject.replace('.', '-')}",
                        config=consumer_config,
                    )
                    break
                except Exception as exc:
                    delay = min(base_delay * (2 ** (attempt - 1)), max_delay)
                    logger.warning(
                        "Subscribe to %s failed (attempt %d/%d): %s — retrying in %.1fs",
                        subject,
                        attempt,
                        max_attempts,
                        exc,
                        delay,
                    )
                    if attempt == max_attempts:
                        if subject in CRITICAL_SUBJECTS:
                            raise RuntimeError(
                                f"Failed to subscribe to {subject} after {max_attempts} attempts"
                            ) from exc
                        logger.warning(
                            "Non-critical subject %s: giving up after %d attempts — skipping",
                            subject,
                            max_attempts,
                        )
                        break
                    await asyncio.sleep(delay)

            if sub is None:
                continue  # non-critical subject failed, skip

            self._subscriptions.append(sub)
            logger.info("Subscribed to %s", subject)

            # Start a background consumer task for each subject
            task = asyncio.create_task(self._consume_loop(subject, sub))
            self._tasks.append(task)

    async def _consume_loop(self, subject: str, sub) -> None:
        """Background loop that fetches and processes messages from a subscription."""
        llm_provider = os.environ.get("LLM_PROVIDER")
        llm_model = os.environ.get("LLM_MODEL")
        llm_api_key = os.environ.get("LLM_API_KEY")
        ollama_url = os.environ.get("OLLAMA_URL")

        while True:
            try:
                msgs = await sub.fetch(batch=5, timeout=5)
            except nats.errors.TimeoutError:
                # Normal idle-poll cadence: no work in the stream right now.
                continue
            except Exception as fetch_err:
                # Spec 046 follow-up F4: transport/stream/auth errors must be
                # observable, not masked as ordinary idle timeouts.
                nats_consume_fetch_errors_total.labels(subject=subject).inc()
                logger.error("NATS fetch failed on subject=%s err=%s", subject, fetch_err)
                await asyncio.sleep(1)
                continue

            for msg in msgs:
                try:
                    data = json.loads(msg.data)
                    start = time.time()

                    if subject == "artifacts.process":
                        try:
                            validate_process_payload(data)
                        except PayloadValidationError as ve:
                            logger.error("Invalid artifacts.process payload: %s", ve)
                            result = {
                                "artifact_id": data.get("artifact_id", ""),
                                "success": False,
                                "error": f"Payload validation failed: {ve}",
                            }
                            elapsed_ms = int((time.time() - start) * 1000)
                            result["processing_time_ms"] = elapsed_ms
                            response_subject = SUBJECT_RESPONSE_MAP.get(subject)
                            if response_subject:
                                await self.publish(response_subject, result)
                            await msg.ack()
                            continue
                        result = await self._handle_artifact_process(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "search.embed":
                        result = await self._handle_search_embed(data)
                        # Reply directly to Go inbox if reply_subject is set
                        reply_subject = data.get("reply_subject")
                        if reply_subject and self._nc:
                            result["processing_time_ms"] = int((time.time() - start) * 1000)
                            payload = json.dumps(result).encode()
                            await self._nc.publish(reply_subject, payload)
                            await msg.ack()
                            continue
                    elif subject == "search.rerank":
                        result = await self._handle_search_rerank(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                        )
                        # Reply directly to Go inbox if reply_subject is set
                        reply_subject = data.get("reply_subject")
                        if reply_subject and self._nc:
                            result["processing_time_ms"] = int((time.time() - start) * 1000)
                            payload = json.dumps(result).encode()
                            await self._nc.publish(reply_subject, payload)
                            await msg.ack()
                            continue
                    elif subject == "digest.generate":
                        result = await self._handle_digest_generate(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                        )
                    elif subject == "keep.sync.request":
                        from .keep_bridge import handle_sync_request

                        result = await handle_sync_request(data)
                    elif subject == "keep.ocr.request":
                        from .ocr import handle_ocr_request

                        result = await handle_ocr_request(data)
                    elif subject == "synthesis.extract":
                        from .synthesis import handle_extract

                        result = await handle_extract(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "synthesis.crosssource":
                        from .synthesis import handle_crosssource

                        result = await handle_crosssource(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "domain.extract":
                        from .domain import handle_domain_extract

                        result = await handle_domain_extract(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject.startswith("photos."):
                        from .photos import handle_photo_request

                        result = await handle_photo_request(subject, data)
                    elif subject == "learning.classify":
                        from .intelligence import handle_learning_classify

                        result = await handle_learning_classify(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "content.analyze":
                        from .intelligence import handle_content_analyze

                        result = await handle_content_analyze(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "monthly.generate":
                        from .intelligence import handle_monthly_generate

                        result = await handle_monthly_generate(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "quickref.generate":
                        from .intelligence import handle_quickref_generate

                        result = await handle_quickref_generate(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "seasonal.analyze":
                        from .intelligence import handle_seasonal_analyze

                        result = await handle_seasonal_analyze(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                            ollama_url,
                        )
                    elif subject == "agent.invoke.request":
                        # Spec 037 Scope 5: dispatch to the stateless
                        # per-turn handler in ml/app/agent.py. The
                        # executor includes a `reply_subject` field so
                        # we publish the response directly to its
                        # ephemeral inbox (matches the search.embed
                        # pattern); JetStream-backed
                        # agent.invoke.response delivery is also
                        # honoured below for clients that prefer the
                        # stream subject.
                        from .agent import handle_invoke

                        result = await handle_invoke(data)
                        reply_subject = data.get("reply_subject")
                        if reply_subject and self._nc:
                            result["processing_time_ms"] = int((time.time() - start) * 1000)
                            payload = json.dumps(result).encode()
                            await self._nc.publish(reply_subject, payload)
                            await msg.ack()
                            continue
                    elif subject == "drive.extract.request":
                        from .drive_extract import handle_drive_extract_request

                        result = await handle_drive_extract_request(data)
                    elif subject == "drive.classify.request":
                        from .drive_classify import handle_drive_classify_request

                        result = await handle_drive_classify_request(
                            data,
                            llm_provider,
                            llm_model,
                            llm_api_key,
                        )
                    else:
                        logger.warning("Unknown subject: %s", subject)
                        await msg.ack()
                        continue

                    elapsed_ms = int((time.time() - start) * 1000)
                    result["processing_time_ms"] = elapsed_ms

                    # Record processing latency metric
                    processing_latency.labels(operation=subject).observe(elapsed_ms / 1000.0)

                    # Record LLM token usage if present
                    tokens = result.get("tokens_used", 0)
                    if tokens and tokens > 0:
                        model_label = sanitize_model(result.get("model_used", llm_model or "unknown"))
                        llm_tokens_used.labels(
                            provider=llm_provider or "unknown",
                            model=model_label,
                        ).inc(tokens)

                    response_subject = SUBJECT_RESPONSE_MAP.get(subject)
                    if subject.startswith("photos.") and response_subject:
                        from .photos import validate_photo_result

                        validate_photo_result(response_subject, result)
                    elif subject == "digest.generate":
                        # Digest results carry digest_date / text, not
                        # artifact_id. validate_processed_result enforces
                        # the artifact-ingestion schema and rejects valid
                        # digest payloads with "artifact_id is required",
                        # which blocked the digest.generated publish and
                        # left the daily-digest pipeline stuck on the
                        # storeFallbackDigest line ("N items processed
                        # overnight."). Skip the artifact-shape check for
                        # digest results; their shape is validated by the
                        # core-side Go subscriber on receipt instead.
                        pass
                    else:
                        # Validate outgoing result before publishing
                        try:
                            validate_processed_result(result)
                        except PayloadValidationError as ve:
                            logger.error("Invalid outgoing result on %s: %s", subject, ve)

                    if response_subject:
                        await self.publish(response_subject, result)

                    await msg.ack()

                except json.JSONDecodeError as e:
                    logger.error("Invalid JSON on %s: %s", subject, e)
                    await msg.ack()  # Don't redeliver malformed messages
                except Exception as e:
                    logger.error("Error processing %s message: %s", subject, e, exc_info=True)
                    await self._handle_poison(subject, msg, e)

    async def _handle_poison(self, subject: str, msg, exc: Exception) -> None:
        """Handle a handler exception per spec 081 design §4.

        Drives the poison-pill decision off msg.metadata.num_delivered
        (JetStream redelivery counter — the single source of truth, no
        local counter). On exhaustion (num_delivered >= max_deliver):
        publish the original payload + canonical 6-header envelope to
        deadletter.<subject>, THEN term() — only after the publish
        succeeds. If publish fails: nak() so JetStream redelivers and
        we retry the publish (publish-before-term invariant; design §4
        invariant 1). Otherwise: nak() for normal redelivery.
        """
        max_deliver = self._consumer_max_deliver
        if max_deliver is None:
            # subscribe_all() always populates this; if we get here it
            # means something subscribed without going through it. Fall
            # back to nak() to let JetStream keep redelivering rather
            # than silently term()ing without dead-letter forensics.
            await msg.nak()
            return

        md = msg.metadata
        num_delivered = md.num_delivered if md is not None else 0

        if num_delivered < max_deliver:
            await msg.nak()
            return

        # Exhausted: build canonical envelope and publish to dead-letter.
        dl_subject = "deadletter." + subject
        original_stream = SUBJECT_TO_STREAM[subject]
        headers: dict[str, str] = {
            "Smackerel-Original-Subject": subject,
            "Smackerel-Original-Stream": original_stream,
            "Smackerel-Failed-At": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "Smackerel-Delivery-Count": str(num_delivered),
        }
        last_err = _utf8_truncate(str(exc), 256)
        if last_err:  # parity with Go `if lastError != ""`
            headers["Smackerel-Last-Error"] = last_err
        consumer = ""
        if md is not None:
            consumer = getattr(md, "consumer", "") or ""
        if not consumer:
            consumer = f"smackerel-ml-{subject.replace('.', '-')}"
        if consumer:  # parity with Go `if md.Consumer != ""`
            headers["Smackerel-Original-Consumer"] = consumer

        try:
            await self._js.publish(dl_subject, msg.data, headers=headers)
        except Exception as pub_err:
            logger.error(
                "dead-letter publish failed; nak for retry subject=%s err=%s",
                subject,
                pub_err,
            )
            nats_deadletter_publish_failures_total.labels(subject=subject).inc()
            await msg.nak()
            return  # MUST NOT term() — preserve forensic evidence

        nats_deadletter_total.labels(stream=original_stream).inc()
        logger.warning(
            "ml message routed to dead-letter subject=%s dl_subject=%s num_delivered=%d",
            subject,
            dl_subject,
            num_delivered,
        )
        await msg.term()

    async def _handle_artifact_process(
        self,
        data: dict,
        provider: str,
        model: str,
        api_key: str,
        ollama_url: str,
    ) -> dict:
        """Process an artifact through LLM + embedding pipeline."""
        from .embedder import generate_artifact_embedding
        from .processor import process_content
        from .whisper_transcribe import transcribe_voice
        from .youtube import fetch_transcript

        artifact_id = data["artifact_id"]
        content_type = data.get("content_type", "")
        raw_text = data.get("raw_text", "")
        url = data.get("url", "")
        tier = data.get("processing_tier", "standard")
        user_context = data.get("user_context", "")
        source_id = data.get("source_id", "")

        # Handle YouTube — fetch transcript
        if content_type == "youtube" and url:
            video_id = url.split("v=")[-1].split("&")[0] if "v=" in url else url.split("/")[-1]
            transcript_result = await fetch_transcript(video_id)
            if transcript_result.get("success"):
                raw_text = transcript_result["text"]
            else:
                logger.warning("YouTube transcript failed for %s: %s", artifact_id, transcript_result.get("error"))

        # Handle voice notes — transcribe
        if content_type == "voice" and url:
            whisper_result = await transcribe_voice(url, ollama_url or None)
            if whisper_result.get("success"):
                raw_text = whisper_result["text"]
            else:
                logger.warning("Whisper transcription failed for %s: %s", artifact_id, whisper_result.get("error"))
                return {
                    "artifact_id": artifact_id,
                    "success": False,
                    "error": f"Transcription failed: {whisper_result.get('error', 'unknown')}",
                }

        # Handle image content — OCR extraction (R-003)
        if content_type == "image" and url:
            from .ocr import extract_text_tesseract

            try:
                validate_fetch_url(url)  # SSRF prevention (SEC-004-002)
                async with httpx.AsyncClient(timeout=30, follow_redirects=True) as client:
                    resp = await client.get(url)
                    resp.raise_for_status()
                    image_bytes = resp.content
                ocr_text = extract_text_tesseract(image_bytes)
                if ocr_text and len(ocr_text.strip()) >= 3:
                    raw_text = ocr_text.strip()
                    logger.info("Image OCR extracted %d chars for %s", len(raw_text), artifact_id)
                else:
                    logger.info("Image OCR produced no usable text for %s", artifact_id)
            except Exception as e:
                logger.warning("Image OCR failed for %s: %s", artifact_id, e)

        # Handle PDF content — text extraction (R-003)
        if content_type == "pdf" and url:
            from .pdf_extract import extract_pdf_text

            try:
                pdf_text = await extract_pdf_text(url)
                if pdf_text and len(pdf_text.strip()) >= 3:
                    raw_text = pdf_text.strip()
                    logger.info("PDF extracted %d chars for %s", len(raw_text), artifact_id)
                else:
                    logger.info("PDF extraction produced no usable text for %s", artifact_id)
            except Exception as e:
                logger.warning("PDF extraction failed for %s: %s", artifact_id, e)

        if not raw_text:
            return {
                "artifact_id": artifact_id,
                "success": False,
                "error": "No content to process",
            }

        # LLM processing
        llm_result = await process_content(
            content=raw_text,
            content_type=content_type,
            source_id=source_id,
            processing_tier=tier,
            user_context=user_context,
            model=model,
            api_key=api_key,
            provider=provider,
        )

        if not llm_result.get("success"):
            return {
                "artifact_id": artifact_id,
                "success": False,
                "error": llm_result.get("error", "LLM processing failed"),
            }

        # Generate embedding
        result = llm_result["result"]
        embedding = await generate_artifact_embedding(
            title=result.get("title", ""),
            summary=result.get("summary", ""),
            key_ideas=result.get("key_ideas", []),
        )

        return {
            "artifact_id": artifact_id,
            "success": True,
            "result": result,
            "embedding": embedding,
            "model_used": llm_result.get("model_used", ""),
            "tokens_used": llm_result.get("tokens_used", 0),
        }

    async def _handle_search_embed(self, data: dict) -> dict:
        """Embed a search query."""
        from .embedder import generate_embedding

        query_id = data.get("query_id", "")
        text = data.get("text", "")

        embedding = await generate_embedding(text)
        return {
            "query_id": query_id,
            "embedding": embedding,
            "model": "all-MiniLM-L6-v2",
        }

    async def _handle_search_rerank(
        self,
        data: dict,
        provider: str,
        model: str,
        api_key: str,
    ) -> dict:
        """Re-rank search candidates using LLM."""
        import litellm

        query_id = data.get("query_id", "")
        query_text = data.get("query", data.get("query_text", ""))
        candidates = data.get("candidates", [])

        if not candidates:
            return {"query_id": query_id, "ranked": []}

        # Build re-ranking prompt
        candidate_text = "\n".join(
            f"[{i + 1}] {c.get('title', '')} ({c.get('artifact_type', '')}): {c.get('summary', '')[:200]}"
            for i, c in enumerate(candidates[:20])
        )

        prompt = f"""Rank these items by relevance to the query: "{query_text}"

{candidate_text}

Return ONLY valid JSON: {{"ranked": [{{"index": 1, "relevance": "high|medium|low", "explanation": "..."}}]}}
Rank top 5 most relevant. Use 1-based index numbers matching the items above."""

        try:
            model_name = f"{provider}/{model}" if provider not in ("openai", "") else model
            # Pass OLLAMA_URL as api_base for Ollama provider; litellm otherwise
            # falls back to its own default which is wrong inside the ml-sidecar
            # container (see processor.py for the same fix).
            api_base = os.environ.get("OLLAMA_URL") if provider == "ollama" else None
            response = await litellm.acompletion(
                model=model_name,
                messages=[{"role": "user", "content": prompt}],
                api_key=api_key,
                api_base=api_base,
                temperature=0.1,
                max_tokens=1000,
                response_format={"type": "json_object"},
                timeout=600,
            )

            result = json.loads(response.choices[0].message.content)
            ranked = []
            for item in result.get("ranked", []):
                idx = item.get("index", 0) - 1
                if 0 <= idx < len(candidates):
                    ranked.append(
                        {
                            "artifact_id": candidates[idx].get("id", candidates[idx].get("artifact_id", "")),
                            "rank": len(ranked) + 1,
                            "relevance": item.get("relevance", "medium"),
                            "explanation": item.get("explanation", ""),
                        }
                    )

            return {
                "query_id": query_id,
                "ranked": ranked,
                "ranked_ids": [r["artifact_id"] for r in ranked],
            }

        except Exception as e:
            logger.error("Re-ranking failed: %s", e)
            # Fall back to returning candidates in original order
            fallback = [
                {
                    "artifact_id": c.get("id", c.get("artifact_id", "")),
                    "rank": i + 1,
                    "relevance": "medium",
                    "explanation": "LLM re-ranking unavailable",
                }
                for i, c in enumerate(candidates[:5])
            ]
            return {
                "query_id": query_id,
                "ranked": fallback,
                "ranked_ids": [r["artifact_id"] for r in fallback],
            }

    async def _handle_digest_generate(
        self,
        data: dict,
        provider: str,
        model: str,
        api_key: str,
    ) -> dict:
        """Generate a daily digest using LLM."""
        import litellm

        digest_date = data.get("digest_date", "")
        action_items = data.get("action_items", [])
        overnight_artifacts = data.get("overnight_artifacts", [])
        hot_topics = data.get("hot_topics", [])

        # Check for quiet day
        if not action_items and not overnight_artifacts and not hot_topics:
            return {
                "digest_date": digest_date,
                "text": "All quiet. Nothing needs your attention today.",
                "word_count": 9,
                "model_used": "none",
            }

        # Build digest context
        context_parts = []
        if action_items:
            items_text = "\n".join(
                f"- {a.get('text', '')} (from {a.get('person', 'unknown')}, waiting {a.get('days_waiting', 0)} days)"
                for a in action_items
            )
            context_parts.append(f"ACTION ITEMS:\n{items_text}")
        if overnight_artifacts:
            artifacts_text = "\n".join(f"- {a.get('title', '')} ({a.get('type', '')})" for a in overnight_artifacts)
            context_parts.append(f"OVERNIGHT ARTIFACTS:\n{artifacts_text}")
        if hot_topics:
            topics_text = "\n".join(
                f"- {t.get('name', '')} ({t.get('captures_this_week', 0)} this week)" for t in hot_topics
            )
            context_parts.append(f"HOT TOPICS:\n{topics_text}")

        prompt = f"""/no_think
Generate a daily briefing digest. Rules:
- Under 150 words total
- Calm, direct, warm tone — no fluff, no emoji
- Use text markers: ! for action needed, > for information, - for list items
- Structure: action items first, then overnight summary, then hot topics
- If action items exist, lead with the most urgent
- Do NOT emit <think>...</think> tags or any chain-of-thought preamble.
- Output the final digest prose ONLY.

Context:
{chr(10).join(context_parts)}

Write the digest text only, no JSON wrapper. Begin output now."""

        try:
            model_name = f"{provider}/{model}" if provider not in ("openai", "") else model
            # Pass OLLAMA_URL as api_base for Ollama provider; litellm otherwise
            # falls back to its own default which is wrong inside the ml-sidecar
            # container (see processor.py for the same fix).
            api_base = os.environ.get("OLLAMA_URL") if provider == "ollama" else None
            response = await litellm.acompletion(
                model=model_name,
                messages=[{"role": "user", "content": prompt}],
                api_key=api_key,
                api_base=api_base,
                temperature=0.3,
                # Budget large enough to absorb gemma4:26b's <think>...</think>
                # chain-of-thought preamble (typically 1.5-3K tokens) PLUS the
                # post-think digest text (~150 words = ~250 tokens). The prior
                # 300-token cap was being entirely consumed by the think block,
                # leaving the actual digest empty (the <think>/fence strip then
                # raised the "empty after strip" sentinel and forced the
                # metadata-only fallback every cycle). 4000 tokens is well below
                # gemma4:26b's 8K context limit and gives the model headroom to
                # think AND write.
                max_tokens=4000,
                timeout=600,
            )

            text = response.choices[0].message.content or ""

            # Strip reasoning-model preamble: gemma4:26b and other Ollama
            # models can emit <think>...</think> chain-of-thought BEFORE
            # the actual digest text. Without this strip, the digest text
            # was just the closing </think> tag (or empty) and the Go
            # subscriber rejected the publish with "text is required".
            # Mirrors processor.py's identical strip for the artifact
            # ingestion path.
            if "<think>" in text:
                close = text.find("</think>")
                if close != -1:
                    text = text[close + len("</think>") :]

            text = text.strip()

            # Some models wrap their digest in ``` fences despite the
            # explicit "no JSON wrapper" instruction. Strip the fence so
            # the operator sees prose, not markdown noise.
            if text.startswith("```"):
                nl = text.find("\n")
                if nl != -1:
                    text = text[nl + 1 :]
                if text.endswith("```"):
                    text = text[:-3].rstrip()

            # If the model returned nothing usable (empty, whitespace, or
            # ONLY a think block we stripped to nothing), fall through to
            # the fallback path so the operator still gets the metadata
            # summary instead of an empty/rejected digest.
            if not text:
                raw = response.choices[0].message.content or ""
                logger.error(
                    "Digest LLM returned no usable text. raw_len=%d raw_head=%r",
                    len(raw),
                    raw[:300],
                )
                raise ValueError(
                    "LLM digest response empty after <think>/fence strip; falling through to metadata fallback"
                )

            word_count = len(text.split())

            return {
                "digest_date": digest_date,
                "text": text,
                "word_count": word_count,
                "model_used": model_name,
            }

        except Exception as e:
            logger.error("Digest generation failed: %s", e)
            # Fallback: plain-text digest from metadata
            fallback_lines = []
            if action_items:
                fallback_lines.append(f"! {len(action_items)} action items need attention.")
            if overnight_artifacts:
                fallback_lines.append(f"> {len(overnight_artifacts)} items processed overnight.")
            if hot_topics:
                fallback_lines.append(f"> Hot topics: {', '.join(t.get('name', '') for t in hot_topics[:3])}")
            if not fallback_lines:
                fallback_lines.append("All quiet. Nothing needs your attention today.")

            text = "\n".join(fallback_lines)
            return {
                "digest_date": digest_date,
                "text": text,
                "word_count": len(text.split()),
                "model_used": "fallback",
            }

    async def publish(self, subject: str, data: dict) -> None:
        """Publish a message to a NATS subject."""
        if not self._js:
            raise RuntimeError("NATS not connected")
        payload = json.dumps(data).encode()
        await self._js.publish(subject, payload)
        logger.debug("Published to %s (%d bytes)", subject, len(payload))

    async def close(self) -> None:
        """Drain and close the NATS connection."""
        # Cancel background tasks
        for task in self._tasks:
            task.cancel()
        self._tasks.clear()

        if self._nc and self._nc.is_connected:
            await self._nc.drain()
            logger.info("NATS connection drained and closed")

    async def _on_disconnect(self) -> None:
        logger.warning("NATS disconnected")

    async def _on_reconnect(self) -> None:
        logger.info("NATS reconnected")
