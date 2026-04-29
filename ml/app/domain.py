"""Domain extraction handler for the ML sidecar.

Receives domain.extract messages, loads the prompt contract,
builds a domain-specific prompt, calls the LLM, validates output,
and publishes to domain.extracted.
"""

import asyncio
import json
import logging
import os
import re
import time
from typing import Any

import litellm
from litellm.exceptions import (
    InternalServerError,
    RateLimitError,
    ServiceUnavailableError,
)

logger = logging.getLogger("smackerel-ml.domain")

MAX_RETRIES = 2
RETRY_DELAYS = [2, 5]
# S-005: Overall budget for domain extraction per artifact (spec constraint: max 30s).
DOMAIN_EXTRACTION_TIMEOUT = 30


async def handle_domain_extract(
    data: dict[str, Any],
    provider: str,
    model: str,
    api_key: str,
    ollama_url: str,
) -> dict[str, Any]:
    """Process a domain extraction request and return structured domain data."""
    artifact_id = data.get("artifact_id", "")
    contract_version = data.get("contract_version", "")

    try:
        return await asyncio.wait_for(
            _do_domain_extract(data, provider, model, api_key, ollama_url),
            timeout=DOMAIN_EXTRACTION_TIMEOUT,
        )
    except asyncio.TimeoutError as exc:
        logger.error(
            "domain extraction exceeded %ds budget",
            DOMAIN_EXTRACTION_TIMEOUT,
            extra={"artifact_id": artifact_id},
        )
        fallback = _degraded_domain_fallback(
            data,
            model,
            DOMAIN_EXTRACTION_TIMEOUT * 1000,
            f"domain extraction exceeded {DOMAIN_EXTRACTION_TIMEOUT}s budget",
            exc,
        )
        if fallback is not None:
            return fallback
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": f"domain extraction exceeded {DOMAIN_EXTRACTION_TIMEOUT}s budget",
            "contract_version": contract_version,
            "processing_time_ms": DOMAIN_EXTRACTION_TIMEOUT * 1000,
            "model_used": model,
            "tokens_used": 0,
        }


async def _do_domain_extract(
    data: dict[str, Any],
    provider: str,
    model: str,
    api_key: str,
    ollama_url: str,
) -> dict[str, Any]:
    """Inner implementation of domain extraction, called under timeout."""
    artifact_id = data.get("artifact_id", "")
    contract_version = data.get("contract_version", "")
    content_type = data.get("content_type", "")
    title = data.get("title", "")
    summary = data.get("summary", "")
    content_raw = data.get("content_raw", "")

    logger.info(
        "domain extraction started",
        extra={
            "artifact_id": artifact_id,
            "contract_version": contract_version,
            "content_type": content_type,
        },
    )

    start_time = time.monotonic()

    # Build prompt from the content
    content = content_raw or summary or title
    if not content:
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": "no content to extract from",
            "contract_version": contract_version,
            "processing_time_ms": 0,
            "model_used": model,
            "tokens_used": 0,
        }

    system_prompt = _build_system_prompt(contract_version, content_type)
    user_prompt = _build_user_prompt(title, summary, content)

    # LLM call with retry
    result = None
    last_error = ""
    last_exception: Exception | None = None
    tokens_used = 0
    model_id = _resolve_model(model, provider, ollama_url)

    for attempt in range(MAX_RETRIES + 1):
        try:
            response = await litellm.acompletion(
                model=model_id,
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": user_prompt},
                ],
                api_key=api_key if api_key else None,
                temperature=0.1,
                response_format={"type": "json_object"},
                timeout=30,
            )

            raw_text = response.choices[0].message.content
            tokens_used = getattr(response.usage, "total_tokens", 0) if response.usage else 0

            result = json.loads(raw_text)

            # Ensure domain field is present
            if "domain" not in result:
                result["domain"] = _domain_from_contract(contract_version)

            # C026-CHAOS-02: Normalize ingredient names to lowercase for
            # case-insensitive search matching on the Go side.
            _normalize_ingredient_names(result)

            break

        except (json.JSONDecodeError, ValueError) as e:
            last_error = f"JSON parse error on attempt {attempt + 1}: {e}"
            last_exception = e
            logger.warning(last_error, extra={"artifact_id": artifact_id})
        except (RateLimitError, ServiceUnavailableError, InternalServerError) as e:
            last_error = f"LLM error on attempt {attempt + 1}: {e}"
            last_exception = e
            logger.warning(last_error, extra={"artifact_id": artifact_id})
        except Exception as e:
            last_error = f"Unexpected error on attempt {attempt + 1}: {e}"
            last_exception = e
            logger.error(last_error, extra={"artifact_id": artifact_id})
            break  # Don't retry unexpected errors

        if attempt < MAX_RETRIES:
            await asyncio.sleep(RETRY_DELAYS[attempt])

    elapsed_ms = int((time.monotonic() - start_time) * 1000)

    if result is None:
        fallback = _degraded_domain_fallback(data, model, elapsed_ms, last_error, last_exception)
        if fallback is not None:
            return fallback
        return {
            "artifact_id": artifact_id,
            "success": False,
            "error": last_error,
            "contract_version": contract_version,
            "processing_time_ms": elapsed_ms,
            "model_used": model,
            "tokens_used": tokens_used,
        }

    return {
        "artifact_id": artifact_id,
        "success": True,
        "domain_data": result,
        "contract_version": contract_version,
        "processing_time_ms": elapsed_ms,
        "model_used": model,
        "tokens_used": tokens_used,
    }


def _resolve_model(model: str, provider: str, ollama_url: str) -> str:
    """Resolve model identifier for litellm."""
    if provider == "ollama":
        return f"ollama/{model}"
    if provider == "openai":
        return model
    return f"{provider}/{model}"


def _domain_degraded_fallback_enabled() -> bool:
    raw = os.environ["ML_PROCESSING_DEGRADED_FALLBACK_ENABLED"].strip().lower()
    if raw == "true":
        return True
    if raw == "false":
        return False
    raise RuntimeError("ML_PROCESSING_DEGRADED_FALLBACK_ENABLED must be true or false")


def _is_llm_unavailable_error(exc: Exception | None, error_text: str) -> bool:
    combined = f"{exc or ''} {error_text}".lower()
    return any(indicator in combined for indicator in ["connection", "connect", "refused", "timeout"])


def _degraded_domain_fallback(
    data: dict[str, Any],
    model: str,
    elapsed_ms: int,
    error_text: str,
    exc: Exception | None,
) -> dict[str, Any] | None:
    try:
        fallback_enabled = _domain_degraded_fallback_enabled()
    except (KeyError, RuntimeError) as config_error:
        logger.error("domain degraded fallback config invalid: %s", config_error)
        return None

    if not fallback_enabled or not _is_llm_unavailable_error(exc, error_text):
        return None

    contract_version = data.get("contract_version", "")
    domain = _domain_from_contract(contract_version)
    if domain != "recipe":
        return None

    content = data.get("content_raw", "") or data.get("summary", "") or data.get("title", "")
    if not content:
        return None

    logger.warning(
        "domain extraction using SST-gated degraded recipe fallback",
        extra={"artifact_id": data.get("artifact_id", ""), "contract_version": contract_version},
    )
    return {
        "artifact_id": data.get("artifact_id", ""),
        "success": True,
        "domain_data": _extract_recipe_degraded(data.get("title", ""), content),
        "contract_version": contract_version,
        "processing_time_ms": elapsed_ms,
        "model_used": "fallback",
        "tokens_used": 0,
    }


def _domain_from_contract(contract_version: str) -> str:
    """Extract domain name from contract version (e.g., 'recipe-extraction-v1' -> 'recipe')."""
    parts = contract_version.split("-")
    return parts[0] if parts else "unknown"


def _build_system_prompt(contract_version: str, content_type: str) -> str:
    """Build domain-specific system prompt based on contract version."""
    domain = _domain_from_contract(contract_version)

    if domain == "recipe":
        return _RECIPE_SYSTEM_PROMPT
    if domain == "product":
        return _PRODUCT_SYSTEM_PROMPT

    return f"Extract structured {domain} data from the provided content. Return valid JSON."


def _build_user_prompt(title: str, summary: str, content: str) -> str:
    """Build the user prompt with artifact content."""
    parts = []
    if title:
        parts.append(f"Title: {title}")
    if summary:
        parts.append(f"Summary: {summary}")
    parts.append(f"\nContent:\n---\n{content}\n---")
    return "\n".join(parts)


def _extract_recipe_degraded(title: str, content: str) -> dict[str, Any]:
    ingredients_section = _extract_section(
        content,
        ["ingredients"],
        ["instructions", "directions", "method", "steps"],
    )
    steps_section = _extract_section(
        content,
        ["instructions", "directions", "method", "steps"],
        [],
    )

    result: dict[str, Any] = {
        "domain": "recipe",
        "title": title,
        "ingredients": [
            {"name": name, "quantity": "", "unit": "", "preparation": "", "group": ""}
            for name in _split_ingredients(ingredients_section)
        ],
        "steps": [
            {"number": index + 1, "instruction": step, "duration_minutes": None, "technique": ""}
            for index, step in enumerate(_split_steps(steps_section))
        ],
        "techniques": [],
        "prep_time_minutes": None,
        "cook_time_minutes": None,
        "total_time_minutes": None,
        "servings": None,
        "cuisine": None,
        "course": None,
        "dietary_tags": [],
    }
    _normalize_ingredient_names(result)
    return result


def _extract_section(content: str, starts: list[str], stops: list[str]) -> str:
    start_pattern = "|".join(re.escape(start) for start in starts)
    match = re.search(rf"\b(?:{start_pattern})\b\s*:?\s*", content, flags=re.IGNORECASE)
    if not match:
        return ""

    section = content[match.end() :]
    stop_positions = []
    for stop in stops:
        stop_match = re.search(rf"\b{re.escape(stop)}\b\s*:?", section, flags=re.IGNORECASE)
        if stop_match:
            stop_positions.append(stop_match.start())
    if stop_positions:
        section = section[: min(stop_positions)]
    return section.strip()


def _split_ingredients(section: str) -> list[str]:
    if not section:
        return []
    ingredients = []
    for raw_item in re.split(r"[,;\n]+", section):
        item = re.sub(r"^\s*(?:[-*]|\d+[.)])\s*", "", raw_item).strip(" .\t\r\n")
        if item:
            ingredients.append(item)
    return ingredients[:100]


def _split_steps(section: str) -> list[str]:
    if not section:
        return []
    numbered = [
        part.strip(" .\t\r\n") for part in re.split(r"(?:^|\s+)(?:step\s*)?\d+[.)]\s+", section, flags=re.IGNORECASE)
    ]
    steps = [part for part in numbered if part]
    if len(steps) <= 1:
        steps = [part.strip(" .\t\r\n") for part in re.split(r"[;\n]+", section) if part.strip(" .\t\r\n")]
    if len(steps) == 1:
        steps = [part.strip(" .\t\r\n") for part in re.split(r"\.\s+", steps[0]) if part.strip(" .\t\r\n")]
    return steps[:100]


_RECIPE_SYSTEM_PROMPT = """\
You are a recipe extraction engine. Extract structured recipe data from the \
provided content. Return ONLY valid JSON with these fields:

{
  "domain": "recipe",
  "ingredients": [{"name": "...", "quantity": "...", "unit": "...", "preparation": "...", "group": "..."}],
  "steps": [{"number": 1, "instruction": "...", "duration_minutes": null, "technique": "..."}],
  "techniques": ["sauteing", "braising", ...],
  "prep_time_minutes": null,
  "cook_time_minutes": null,
  "total_time_minutes": null,
  "servings": null,
  "cuisine": null,
  "course": null,
  "dietary_tags": [],
  "difficulty": null,
  "equipment": [],
  "tips": [],
  "nutrition_per_serving": null
}

RULES:
- Extract ALL ingredients with quantities, units, and preparation notes.
- Group ingredients by section if the recipe has sections (e.g., "for the sauce").
- Number steps sequentially. Estimate duration per step if not stated.
- Identify cooking techniques used (e.g., sauteing, braising, blind baking).
- For dietary tags, infer from ingredients (e.g., no meat -> vegetarian).
- If a field cannot be determined, use null (not empty string or zero).
"""

_PRODUCT_SYSTEM_PROMPT = """\
You are a product extraction engine. Extract structured product data from the \
provided content. Return ONLY valid JSON with these fields:

{
  "domain": "product",
  "product_name": "...",
  "brand": null,
  "model": null,
  "category": null,
  "price": {"amount": null, "currency": null},
  "specs": [{"name": "...", "value": "..."}],
  "pros": [],
  "cons": [],
  "rating": {"score": null, "max": 5, "count": null},
  "availability": null,
  "comparison_notes": null
}

RULES:
- Extract product name, brand, and model separately.
- Parse price with currency code (USD, EUR, GBP, etc.).
- Extract key specifications as name-value pairs.
- Identify pros and cons from reviews or descriptions.
- If a field cannot be determined, use null.
"""


def _normalize_ingredient_names(result: dict[str, Any]) -> None:
    """Normalize ingredient names to lowercase for case-insensitive search.

    C026-CHAOS-02: LLMs return mixed-case ingredient names (e.g., "Chicken Breast")
    but user searches are lowercased by parseDomainIntent. Normalizing here ensures
    JSONB containment and LIKE queries match regardless of LLM casing.
    """
    ingredients = result.get("ingredients")
    if not isinstance(ingredients, list):
        return
    for item in ingredients:
        if isinstance(item, dict) and isinstance(item.get("name"), str):
            item["name"] = item["name"].lower()
