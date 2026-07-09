"""LLM-backed drive classification handlers for Spec 038 Scope 3."""

from __future__ import annotations

import json

import litellm

from .ollama_thinking import apply_structured_extraction_thinking

DRIVE_CLASSIFICATION_PROMPT = """Classify this cloud-drive file for Smackerel.

Return only JSON with these fields:
- classification: one concrete type such as recipe, expense, list, annotation, action_item, note
- topic: short human topic
- audience: personal, household, business, public, or unknown
- sensitivity: none, personal, confidential, restricted
- confidence: number from 0.0 to 1.0
- evidence: at least one specific phrase from content or folder context
- domain_routes: zero or more of recipes, expenses, lists, annotations, action_items, meal_plan, digest
- action_items: optional list of extracted actions
- summary: short summary

Title: {title}
MIME type: {mime_type}
Folder path: {folder_path}
Folder summary: {folder_summary}

Extracted text:
{extracted_text}
"""


async def handle_drive_classify_request(data: dict, provider: str, model: str, api_key: str) -> dict:
    """NATS entrypoint wrapper for drive classification."""
    return await classify_drive_file(data, provider=provider, model=model, api_key=api_key)


async def classify_drive_file(data: dict, provider: str, model: str, api_key: str) -> dict:
    """Classify one extracted drive file using the configured LLM provider."""
    artifact_id = str(data.get("artifact_id", ""))
    prompt = DRIVE_CLASSIFICATION_PROMPT.format(
        title=str(data.get("title", "")),
        mime_type=str(data.get("mime_type", "")),
        folder_path=str(data.get("folder_path", "")),
        folder_summary=json.dumps(data.get("folder_summary") or {}, sort_keys=True),
        extracted_text=str(data.get("extracted_text", ""))[:12000],
    )
    model_name = f"{provider}/{model}" if provider not in ("", "openai") else model
    # BUG-026-007 (redteam F2, latency half) — drive-classify runs on LLM_MODEL
    # (qwen3:30b-a3b in prod, dispatched from nats_client); disable qwen3 thinking
    # on this structured-JSON classification when SST says so. This call routes
    # via the legacy ollama/ (/api/generate) transform, so /no_think in the
    # messages is the mechanism that reaches the model. No-op otherwise.
    messages = apply_structured_extraction_thinking([{"role": "user", "content": prompt}], provider)
    response = await litellm.acompletion(
        model=model_name,
        messages=messages,
        api_key=api_key,
        temperature=0.1,
        max_tokens=1000,
        response_format={"type": "json_object"},
        timeout=30,
    )
    raw = response.choices[0].message.content
    payload = json.loads(raw)
    validated = validate_drive_classification_result(payload)
    validated["artifact_id"] = artifact_id
    validated["success"] = True
    validated["model_used"] = model_name
    validated["tokens_used"] = getattr(getattr(response, "usage", None), "total_tokens", 0)
    return validated


def validate_drive_classification_result(payload: dict) -> dict:
    """Validate and normalize the drive classification contract."""
    required = ["classification", "topic", "audience", "sensitivity", "confidence", "evidence"]
    missing = [key for key in required if key not in payload or payload[key] in (None, "", [])]
    if missing:
        raise ValueError("drive classification missing required fields: " + ", ".join(missing))

    confidence = payload["confidence"]
    if not isinstance(confidence, (int, float)) or confidence < 0 or confidence > 1:
        raise ValueError("drive classification confidence must be a number from 0.0 to 1.0")

    sensitivity = str(payload["sensitivity"])
    if sensitivity not in {"none", "personal", "confidential", "restricted"}:
        raise ValueError("drive classification sensitivity is not recognized")

    evidence = payload["evidence"]
    if not isinstance(evidence, list) or not evidence:
        raise ValueError("drive classification evidence must be a non-empty list")
    for item in evidence:
        if not isinstance(item, str) or len(item.strip()) < 8:
            raise ValueError("drive classification evidence must contain specific textual support")
        if item.strip().lower() in {"file name", "title", "folder"}:
            raise ValueError("drive classification evidence cannot rely only on metadata labels")

    routes = payload.get("domain_routes") or []
    if not isinstance(routes, list):
        raise ValueError("drive classification domain_routes must be a list")

    return {
        "classification": str(payload["classification"]),
        "topic": str(payload["topic"]),
        "audience": str(payload["audience"]),
        "sensitivity": sensitivity,
        "confidence": float(confidence),
        "evidence": [str(item).strip() for item in evidence],
        "domain_routes": [str(item) for item in routes],
        "action_items": [str(item) for item in payload.get("action_items") or []],
        "summary": str(payload.get("summary") or ""),
    }
