"""Photo-library ML contract validators and Scope-1 stub handlers."""

from __future__ import annotations

from typing import Any

LLM_DECISION_RESULT_SUBJECTS = {
    "photos.classified",
    "photos.lifecycle.result",
    "photos.dedupe.result",
    "photos.sensitivity.result",
    "photos.aesthetic.result",
    "photos.removal.reviewed",
}

REQUEST_TO_RESPONSE = {
    "photos.classify": "photos.classified",
    "photos.ocr": "photos.ocred",
    "photos.embed": "photos.embedded",
    "photos.lifecycle": "photos.lifecycle.result",
    "photos.dedupe": "photos.dedupe.result",
    "photos.sensitivity": "photos.sensitivity.result",
    "photos.aesthetic": "photos.aesthetic.result",
    "photos.removal.review": "photos.removal.reviewed",
}


class PhotoPayloadValidationError(ValueError):
    """Raised when a photo ML payload violates the cross-language contract."""


def validate_photo_result(subject: str, payload: dict[str, Any]) -> dict[str, Any]:
    """Validate a photo ML result payload.

    Stable signals may be carried in requests and echoed in results, but any
    LLM-owned decision result must include both confidence and rationale so the
    Go core can surface uncertain work instead of silently accepting it.
    """
    _require_non_empty(payload, "request_id")
    _require_non_empty(payload, "photo_id")
    _require_non_empty(payload, "artifact_id")

    result = payload.get("result")
    if not isinstance(result, dict):
        raise PhotoPayloadValidationError("result must be an object")

    if subject in LLM_DECISION_RESULT_SUBJECTS:
        missing: list[str] = []
        confidence = result.get("confidence")
        rationale = result.get("rationale")
        if not isinstance(confidence, (int, float)) or confidence < 0 or confidence > 1:
            missing.append("confidence")
        if not isinstance(rationale, str) or not rationale.strip():
            missing.append("rationale")
        if missing:
            raise PhotoPayloadValidationError("photo LLM decision result missing required " + ", ".join(missing))

    return payload


async def handle_photo_request(subject: str, payload: dict[str, Any]) -> dict[str, Any]:
    """Scope-1 contract canary handler for photo requests.

    Real provider scanning and model inference are delivered in later scopes;
    this boundary keeps the response subjects and validation shape executable.
    """
    response_subject = REQUEST_TO_RESPONSE.get(subject)
    if response_subject is None:
        raise PhotoPayloadValidationError(f"unsupported photo subject {subject!r}")

    result: dict[str, Any]
    if subject == "photos.ocr":
        result = {"text": "", "confidence": 1.0, "rationale": "No OCR text extracted by Scope-1 contract canary."}
    elif subject == "photos.embed":
        result = {
            "embedding": [],
            "confidence": 1.0,
            "rationale": "Scope-1 contract canary does not run embedding inference.",
        }
    else:
        result = {
            "decision": "needs_model",
            "confidence": 0.0,
            "rationale": "Scope-1 contract canary requires a later ML implementation.",
        }

    response = {
        "request_id": payload.get("request_id", ""),
        "photo_id": payload.get("photo_id", ""),
        "artifact_id": payload.get("artifact_id", ""),
        "result": result,
    }
    return validate_photo_result(response_subject, response)


def _require_non_empty(payload: dict[str, Any], field: str) -> None:
    value = payload.get(field)
    if not isinstance(value, str) or not value.strip():
        raise PhotoPayloadValidationError(f"{field} is required")
