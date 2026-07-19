"""LLM processing via litellm — Universal Processing Prompt."""

import asyncio
import json
import logging
import os
from typing import Any

import litellm
from litellm.exceptions import (
    InternalServerError,
    RateLimitError,
    ServiceUnavailableError,
)

from .ollama_keepalive import (
    OllamaProfileConfigError,
    dispatch_litellm,
    resolve_domain_output_token_budget,
    resolve_ollama_request_profile,
)
from .ollama_thinking import apply_structured_extraction_thinking

logger = logging.getLogger("smackerel-ml.processor")

UNIVERSAL_PROCESSING_PROMPT = """\
You are an intelligent content processor. \
Analyze the following content and return a structured JSON response.

Content type: {content_type}
Source: {source_id}
Processing tier: {processing_tier}
User context: {user_context}

Content:
---
{content}
---

Return ONLY valid JSON with these fields:
{{
  "artifact_type": "article|video|email|product|person|idea|place|book|recipe|bill|trip|trail|note|media|event",
  "title": "concise descriptive title",
  "summary": "2-4 sentence summary of the content",
  "key_ideas": ["idea 1", "idea 2", "..."],
  "entities": {{
    "people": ["name1", "name2"],
    "orgs": ["org1"],
    "places": ["place1"],
    "products": ["product1"],
    "dates": ["date1"]
  }},
  "action_items": ["action 1", "action 2"],
  "topics": ["topic1", "topic2"],
  "sentiment": "positive|neutral|negative|mixed",
  "temporal_relevance": {{
    "relevant_from": null,
    "relevant_until": null
  }},
  "source_quality": "high|medium|low"
}}

Rules:
- summary must be 2-4 sentences
- key_ideas: extract 2-5 main ideas
- entities: extract ALL named entities by category
- action_items: extract commitments, deadlines, to-dos (empty array if none)
- topics: 1-5 topic tags for categorization
- For "light" tier: only return title, summary, topics, sentiment
- For "metadata" tier: only return title, artifact_type
"""


def _processing_degraded_fallback_enabled() -> bool:
    raw = os.environ["ML_PROCESSING_DEGRADED_FALLBACK_ENABLED"].strip().lower()
    if raw == "true":
        return True
    if raw == "false":
        return False
    raise RuntimeError("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED must be true or false")


def _is_llm_unavailable_error(exc: Exception) -> bool:
    error_msg = str(exc).lower()
    return any(indicator in error_msg for indicator in ["connection", "connect", "refused", "timeout"])


def _parse_llm_json(text: str | None) -> Any:
    """Parse an LLM JSON payload, tolerating a prose preamble/trailing wrapper.

    litellm's response_format={"type":"json_object"} is not honored by every
    Ollama-served model, so a model may emit prose around the JSON object (the
    <think> and ```json cases are stripped by the caller; this catches the
    rest). First try a strict parse; on failure salvage the widest {...} span
    and retry. A TRUNCATED payload (the observed "Unterminated string" from an
    overrun output budget) has no closing brace to salvage, so it re-raises the
    JSONDecodeError — the caller then degrades gracefully rather than dropping
    the capture (redteam F2 / BUG-026-006).

    An EMPTY or None payload (some Ollama-served models return content=None on
    an overrun/aborted generation) is as unrecoverable as a truncated one and
    MUST route through the SAME except-JSONDecodeError degraded-fallback branch
    in the caller. Raising a plain json.JSONDecodeError here — instead of
    letting json.loads(None) raise a TypeError that would BYPASS that branch and
    hard-drop the capture via the generic exception handler — keeps every
    unparseable response on one capture-preserving path (redteam F2 /
    BUG-026-006).
    """
    if text is None or not text.strip():
        raise json.JSONDecodeError("empty LLM payload", text or "", 0)
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = text.find("{")
        end = text.rfind("}")
        if start != -1 and end > start:
            return json.loads(text[start : end + 1])
        raise


async def process_content(
    content: str,
    content_type: str,
    source_id: str,
    processing_tier: str,
    user_context: str,
    model: str,
    api_key: str,
    provider: str,
) -> dict[str, Any]:
    """Process content through LLM and return structured result."""
    prompt = UNIVERSAL_PROCESSING_PROMPT.format(
        content_type=content_type,
        source_id=source_id,
        processing_tier=processing_tier,
        user_context=user_context or "none",
        content=content[:15000],  # Limit content length to avoid token limits
    )

    try:
        model_name = f"{provider}/{model}" if provider not in ("openai", "") else model
        if provider == "ollama":
            # F2 — route Ollama through ollama_chat/ (/api/chat) so litellm
            # forwards keep_alive to the request TOP LEVEL; the legacy ollama/
            # (/api/generate) transform buries it under `options`, where Ollama
            # ignores it. model_used reflects the actual route taken.
            model_name = f"ollama_chat/{model}"

        # For Ollama (and any local provider exposed via OLLAMA_URL in SST),
        # pass api_base explicitly. litellm only reads OLLAMA_API_BASE /
        # OLLAMA_BASE_URL env vars and otherwise falls back to its own default,
        # which doesn't exist inside the smackerel-ml container — Ollama runs
        # in its own service. Reading OLLAMA_URL (the canonical SST key written
        # to *.env by scripts/commands/config.sh) keeps a single source of truth.
        api_base = os.environ.get("OLLAMA_URL") if provider == "ollama" else None

        completion_kwargs: dict[str, Any] = {
            "model": model_name,
            "messages": [{"role": "user", "content": prompt}],
            "api_key": api_key,
            "api_base": api_base,
            "temperature": 0.1,
            # Spec 102 SCOPE-102-03 (BUG-026-006) — SST-owned output-token
            # budget, replacing the hardcoded 2000 magic number that could
            # truncate a domain-extraction JSON mid-object (a BUG-026-006
            # malformed-JSON-drop contributor).
            "max_tokens": resolve_domain_output_token_budget(),
            "response_format": {"type": "json_object"},
            "timeout": 600,
        }
        # BUG-026-007 (redteam F2, latency half) — disable qwen3 thinking on this
        # universal-processing structured-JSON extraction call when SST says so
        # via native think=False (litellm forwards it top-level on the
        # ollama_chat/ route). No-op for non-ollama / when thinking stays on / on
        # non-qwen models.
        apply_structured_extraction_thinking(completion_kwargs, provider)
        profile = resolve_ollama_request_profile(model) if provider == "ollama" else None

        # Retry with exponential backoff for transient LLM errors
        max_attempts = 3
        backoff_delays = [1, 2, 4]  # seconds
        last_exc: Exception | None = None
        response = None

        for attempt in range(max_attempts):
            try:
                response = await dispatch_litellm(
                    completion_kwargs,
                    provider=provider,
                    model=model,
                    profile=profile,
                    litellm_module=litellm,
                )
                break
            except (RateLimitError, ServiceUnavailableError, InternalServerError) as exc:
                last_exc = exc
                if attempt < max_attempts - 1:
                    delay = backoff_delays[attempt]
                    logger.warning(
                        "LLM call failed (attempt %d/%d): %s — retrying in %ds",
                        attempt + 1,
                        max_attempts,
                        exc,
                        delay,
                    )
                    await asyncio.sleep(delay)

        if response is None:
            raise last_exc  # type: ignore[misc]

        result_text = response.choices[0].message.content

        # Reasoning-model preamble strip: deepseek-r1 and other reasoning
        # models emit a <think>...</think> chain-of-thought block BEFORE
        # the actual JSON payload. litellm's response_format={"type":
        # "json_object"} cannot suppress this for Ollama-served models —
        # the constraint only applies to OpenAI-compatible providers.
        # Strip the think block so json.loads() sees the real payload.
        if result_text and "<think>" in result_text:
            think_close = result_text.find("</think>")
            if think_close != -1:
                result_text = result_text[think_close + len("</think>") :].lstrip()

        # Some models also wrap JSON in ```json ... ``` fences despite
        # response_format being set. Strip the fence if present.
        if result_text:
            stripped = result_text.strip()
            if stripped.startswith("```"):
                # Drop the opening fence (with optional language tag) and the
                # trailing fence. Keep only the inner payload.
                first_nl = stripped.find("\n")
                if first_nl != -1:
                    stripped = stripped[first_nl + 1 :]
                if stripped.endswith("```"):
                    stripped = stripped[:-3].rstrip()
                result_text = stripped

        result = _parse_llm_json(result_text)

        # BUG-061-002: short / low-signal inputs (single tokens, emoji,
        # URL-only captures) and the prompt's own "light" / "metadata"
        # tier rules legitimately produce LLM payloads that omit
        # `artifact_type` and/or `title`. Previously this raised a
        # ValueError that the outer except-clause swallowed into an
        # opaque "LLM processing failed" — silently dropping the
        # capture. Degrade gracefully instead, mirroring the existing
        # unavailable-LLM fallback shape, and log which fields were
        # defaulted so the silent-drop is no longer silent.
        defaulted_fields: list[str] = []
        if "title" not in result or not str(result.get("title") or "").strip():
            result["title"] = content[:100].strip() or "Untitled"
            defaulted_fields.append("title")
        if "artifact_type" not in result or not str(result.get("artifact_type") or "").strip():
            result["artifact_type"] = content_type if content_type and content_type != "generic" else "note"
            defaulted_fields.append("artifact_type")
        if defaulted_fields:
            logger.warning(
                "LLM result missing required fields %s for source_id=%s tier=%s; "
                "derived defaults from content/content_type (BUG-061-002)",
                defaulted_fields,
                source_id,
                processing_tier,
            )

        # Set defaults for optional fields
        result.setdefault("summary", "")
        result.setdefault("key_ideas", [])
        result.setdefault(
            "entities",
            {"people": [], "orgs": [], "places": [], "products": [], "dates": []},
        )
        result.setdefault("action_items", [])
        result.setdefault("topics", [])
        result.setdefault("sentiment", "neutral")
        result.setdefault("temporal_relevance", {"relevant_from": None, "relevant_until": None})
        result.setdefault("source_quality", "medium")

        tokens_used = response.usage.total_tokens if response.usage else 0

        return {
            "success": True,
            "result": result,
            "model_used": model_name,
            "tokens_used": tokens_used,
        }

    except json.JSONDecodeError as e:
        # redteam F2 / BUG-026-006 — a malformed / TRUNCATED LLM payload (the
        # observed "Unterminated string" from an overrun output budget) is not
        # recoverable JSON, but it MUST NOT hard-drop the capture. Mirror the
        # unavailable-LLM branch below: when the SST gate enables it, degrade
        # gracefully (capture preserved with low-signal metadata) instead of
        # returning a hard failure. When the gate is disabled/misconfigured the
        # pre-existing hard error is preserved (no silent success).
        logger.error("LLM returned invalid JSON: %s", e)

        try:
            fallback_enabled = _processing_degraded_fallback_enabled()
        except (KeyError, RuntimeError) as config_error:
            logger.error("ML degraded fallback config invalid: %s", config_error)
            fallback_enabled = False

        if fallback_enabled:
            logger.warning(
                "LLM returned unparseable JSON - providing SST-gated degraded fallback result "
                "(capture preserved, source_id=%s)",
                source_id,
            )
            fallback_artifact_type = content_type if content_type and content_type != "generic" else "note"
            return {
                "success": True,
                "result": {
                    "artifact_type": fallback_artifact_type,
                    "title": content[:100].strip() or "Untitled",
                    "summary": "Processing completed with SST-gated degraded fallback - LLM returned malformed JSON",
                    "key_ideas": [],
                    "entities": {"people": [], "orgs": [], "places": [], "products": [], "dates": []},
                    "action_items": [],
                    "topics": ["degraded-fallback-malformed-json"],
                    "sentiment": "neutral",
                    "temporal_relevance": {"relevant_from": None, "relevant_until": None},
                    "source_quality": "low",
                },
                "model_used": "fallback",
                "tokens_used": 0,
            }

        return {"success": False, "error": f"Invalid JSON from LLM: {e}"}
    except OllamaProfileConfigError:
        raise
    except Exception as e:
        logger.error("LLM processing failed", exc_info=True)

        try:
            fallback_enabled = _processing_degraded_fallback_enabled()
        except (KeyError, RuntimeError) as config_error:
            logger.error("ML degraded fallback config invalid: %s", config_error)
            fallback_enabled = False

        if fallback_enabled and _is_llm_unavailable_error(e):
            logger.warning("LLM service unavailable - providing SST-gated degraded fallback result")
            fallback_artifact_type = content_type if content_type and content_type != "generic" else "note"
            fallback_result = {
                "artifact_type": fallback_artifact_type,
                "title": content[:100].strip() or "Untitled",
                "summary": "Processing completed with SST-gated degraded fallback - LLM service unavailable",
                "key_ideas": [],
                "entities": {"people": [], "orgs": [], "places": [], "products": [], "dates": []},
                "action_items": [],
                "topics": ["degraded-fallback"],
                "sentiment": "neutral",
                "temporal_relevance": {"relevant_from": None, "relevant_until": None},
                "source_quality": "low",
            }
            return {
                "success": True,
                "result": fallback_result,
                "model_used": "fallback",
                "tokens_used": 0,
            }

        return {"success": False, "error": "LLM processing failed"}
