"""NATS payload validation for the ML sidecar.

Mirrors Go-side ValidateProcessPayload and ValidateProcessedPayload
(internal/pipeline/processor.go) to catch schema drift at the
Python boundary.
"""

import logging

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
