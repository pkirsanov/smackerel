"""LLM processing via litellm — Universal Processing Prompt."""

import asyncio
import json
import logging
from typing import Any

import litellm
from litellm.exceptions import (
    InternalServerError,
    RateLimitError,
    ServiceUnavailableError,
)

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

        # Retry with exponential backoff for transient LLM errors
        max_attempts = 3
        backoff_delays = [1, 2, 4]  # seconds
        last_exc: Exception | None = None
        response = None

        for attempt in range(max_attempts):
            try:
                response = await litellm.acompletion(
                    model=model_name,
                    messages=[{"role": "user", "content": prompt}],
                    api_key=api_key,
                    temperature=0.1,
                    max_tokens=2000,
                    response_format={"type": "json_object"},
                    timeout=30,
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
        result = json.loads(result_text)

        # Validate required fields
        required_fields = ["artifact_type", "title"]
        for field in required_fields:
            if field not in result:
                raise ValueError(f"Missing required field: {field}")

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
        logger.error("LLM returned invalid JSON: %s", e)
        return {"success": False, "error": f"Invalid JSON from LLM: {e}"}
    except Exception:
        logger.error("LLM processing failed", exc_info=True)
        return {"success": False, "error": "LLM processing failed"}
