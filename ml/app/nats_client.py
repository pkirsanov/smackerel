"""NATS JetStream client for the ML sidecar."""

import asyncio
import json
import logging
import os
import time

import nats
from nats.aio.client import Client as NATSConn
from nats.js.client import JetStreamContext

logger = logging.getLogger("smackerel-ml.nats")

# Subjects this sidecar subscribes to
SUBSCRIBE_SUBJECTS = [
    "artifacts.process",
    "search.embed",
    "search.rerank",
    "digest.generate",
    "keep.sync.request",
    "keep.ocr.request",
]

# Subjects this sidecar publishes to
PUBLISH_SUBJECTS = [
    "artifacts.processed",
    "search.embedded",
    "search.reranked",
    "digest.generated",
    "keep.sync.response",
    "keep.ocr.response",
]

# Map of subscribe subject -> publish response subject
SUBJECT_RESPONSE_MAP = {
    "artifacts.process": "artifacts.processed",
    "search.embed": "search.embedded",
    "search.rerank": "search.reranked",
    "digest.generate": "digest.generated",
    "keep.sync.request": "keep.sync.response",
    "keep.ocr.request": "keep.ocr.response",
}


# Subjects that are critical — failure to subscribe is fatal
CRITICAL_SUBJECTS = {"artifacts.process", "search.embed"}


class NATSClient:
    """Manages NATS JetStream connection and subscriptions for the ML sidecar."""

    def __init__(self, url: str) -> None:
        self.url = url
        self._nc: NATSConn | None = None
        self._js: JetStreamContext | None = None
        self._subscriptions: list = []
        self._tasks: list[asyncio.Task] = []
        self._failure_counts: dict[int, int] = {}  # msg seq -> nak count

    @property
    def is_connected(self) -> bool:
        return self._nc is not None and self._nc.is_connected

    async def connect(self) -> None:
        """Connect to NATS and initialize JetStream."""
        connect_opts: dict = dict(
            servers=[self.url],
            name="smackerel-ml",
            reconnect_time_wait=2,
            max_reconnect_attempts=60,
            disconnected_cb=self._on_disconnect,
            reconnected_cb=self._on_reconnect,
        )
        # Token authentication — mirrors Go core's NATS auth enforcement
        auth_token = os.environ.get("SMACKEREL_AUTH_TOKEN", "")
        if auth_token:
            connect_opts["token"] = auth_token

        self._nc = await nats.connect(**connect_opts)
        self._js = self._nc.jetstream()
        logger.info("Connected to NATS at %s", self.url)

    async def subscribe_all(self) -> None:
        """Subscribe to all processing subjects and start consumer loops.

        Retries each subscription with exponential backoff in case the
        JetStream streams have not been created yet (e.g. core runtime
        is still initialising after a fresh-volume start).
        """
        if not self._js:
            raise RuntimeError("NATS not connected")

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
            except Exception:
                await asyncio.sleep(1)
                continue

            for msg in msgs:
                try:
                    data = json.loads(msg.data)
                    start = time.time()

                    if subject == "artifacts.process":
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
                            result["processing_time_ms"] = int(
                                (time.time() - start) * 1000
                            )
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
                    else:
                        logger.warning("Unknown subject: %s", subject)
                        await msg.ack()
                        continue

                    elapsed_ms = int((time.time() - start) * 1000)
                    result["processing_time_ms"] = elapsed_ms

                    response_subject = SUBJECT_RESPONSE_MAP.get(subject)
                    if response_subject:
                        await self.publish(response_subject, result)

                    await msg.ack()

                except json.JSONDecodeError as e:
                    logger.error("Invalid JSON on %s: %s", subject, e)
                    await msg.ack()  # Don't redeliver malformed messages
                except Exception as e:
                    logger.error("Error processing %s message: %s", subject, e, exc_info=True)
                    seq = msg.metadata.sequence.stream if msg.metadata else 0
                    self._failure_counts[seq] = self._failure_counts.get(seq, 0) + 1
                    if self._failure_counts[seq] >= 5:
                        logger.critical(
                            "Poison pill detected on %s (seq=%d, failures=%d) — terminating message",
                            subject,
                            seq,
                            self._failure_counts[seq],
                        )
                        await msg.term()
                        del self._failure_counts[seq]
                    else:
                        await msg.nak()

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
            response = await litellm.acompletion(
                model=model_name,
                messages=[{"role": "user", "content": prompt}],
                api_key=api_key,
                temperature=0.1,
                max_tokens=1000,
                response_format={"type": "json_object"},
                timeout=15,
            )

            result = json.loads(response.choices[0].message.content)
            ranked = []
            for item in result.get("ranked", []):
                idx = item.get("index", 0) - 1
                if 0 <= idx < len(candidates):
                    ranked.append(
                        {
                            "artifact_id": candidates[idx].get(
                                "id", candidates[idx].get("artifact_id", "")
                            ),
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

        prompt = f"""Generate a daily briefing digest. Rules:
- Under 150 words total
- Calm, direct, warm tone — no fluff, no emoji
- Use text markers: ! for action needed, > for information, - for list items
- Structure: action items first, then overnight summary, then hot topics
- If action items exist, lead with the most urgent

Context:
{chr(10).join(context_parts)}

Write the digest text only, no JSON wrapper."""

        try:
            model_name = f"{provider}/{model}" if provider not in ("openai", "") else model
            response = await litellm.acompletion(
                model=model_name,
                messages=[{"role": "user", "content": prompt}],
                api_key=api_key,
                temperature=0.3,
                max_tokens=300,
                timeout=15,
            )

            text = response.choices[0].message.content.strip()
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
