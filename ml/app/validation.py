"""NATS payload validation for the ML sidecar.

Mirrors Go-side ValidateProcessPayload and ValidateProcessedPayload
(internal/pipeline/processor.go) to catch schema drift at the
Python boundary.
"""

import logging
import math

logger = logging.getLogger("smackerel-ml.validation")


class PayloadValidationError(Exception):
    """Raised when a NATS payload fails validation."""


def validate_process_payload(data: dict) -> None:
    """Validate an incoming artifacts.process payload.

    Checks the same required fields as Go's ValidateProcessPayload:
    - artifact_id must be present and non-empty
    - content_type must be present and non-empty
    - At least one of raw_text or url must be present and non-empty

    Raises PayloadValidationError on failure.
    """
    artifact_id = data.get("artifact_id", "")
    if not artifact_id:
        raise PayloadValidationError("artifact_id is required")

    content_type = data.get("content_type", "")
    if not content_type:
        raise PayloadValidationError("content_type is required")

    raw_text = data.get("raw_text", "")
    url = data.get("url", "")
    if not raw_text and not url:
        raise PayloadValidationError("at least one of raw_text or url is required")


def validate_processed_result(data: dict) -> None:
    """Validate an outgoing artifacts.processed result payload.

    Checks the same required fields as Go's ValidateProcessedPayload:
    - artifact_id must be present and non-empty

    Raises PayloadValidationError on failure.
    """
    artifact_id = data.get("artifact_id", "")
    if not artifact_id:
        raise PayloadValidationError("artifact_id is required")


def validate_crosssource_result(data: dict) -> None:
    """Validate an outgoing synthesis.crosssource.result payload."""
    _require_non_empty_string(data, "concept_id")

    if type(data.get("has_genuine_connection")) is not bool:
        raise PayloadValidationError("has_genuine_connection must be a boolean")

    insight_text = data.get("insight_text")
    if not isinstance(insight_text, str):
        raise PayloadValidationError("insight_text must be a string")
    if data["has_genuine_connection"] and not insight_text.strip():
        raise PayloadValidationError("insight_text is required for a genuine connection")

    _require_finite_number_in_range(data, "confidence", 0.0, 1.0)
    _require_crosssource_artifact_ids(data)
    _require_non_empty_string(data, "prompt_contract_version")

    processing_time_ms = data.get("processing_time_ms")
    if type(processing_time_ms) is not int or processing_time_ms < 0:
        raise PayloadValidationError("processing_time_ms must be a non-negative integer")

    if not isinstance(data.get("model_used"), str):
        raise PayloadValidationError("model_used must be a string")


def _require_non_empty_string(data: dict, field: str) -> None:
    value = data.get(field)
    if not isinstance(value, str) or not value.strip():
        raise PayloadValidationError(f"{field} is required")


def _require_finite_number_in_range(data: dict, field: str, minimum: float, maximum: float) -> None:
    value = data.get(field)
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise PayloadValidationError(f"{field} must be a number")
    if not math.isfinite(value) or value < minimum or value > maximum:
        raise PayloadValidationError(f"{field} must be finite and between {minimum} and {maximum}")


def _require_crosssource_artifact_ids(data: dict) -> None:
    artifact_ids = data.get("artifact_ids")
    if not isinstance(artifact_ids, list):
        raise PayloadValidationError("artifact_ids must be a list")
    if len(artifact_ids) < 2:
        raise PayloadValidationError("artifact_ids must contain at least two IDs")
    if any(not isinstance(artifact_id, str) or not artifact_id.strip() for artifact_id in artifact_ids):
        raise PayloadValidationError("artifact_ids must contain non-empty strings")
    if len(set(artifact_ids)) != len(artifact_ids):
        raise PayloadValidationError("artifact_ids must contain distinct IDs")
