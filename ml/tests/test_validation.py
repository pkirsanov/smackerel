"""SCN-002-056 / SCN-002-057: Python NATS payload validation tests."""

import pytest  # noqa: I001
from app.validation import (
    PayloadValidationError,
    validate_process_payload,
    validate_processed_result,
)


# --- SCN-002-056: validate_process_payload ---


def test_scn002056_valid_process_payload():
    """Valid payload with all required fields passes."""
    validate_process_payload(
        {
            "artifact_id": "01ABCDEF",
            "content_type": "article",
            "raw_text": "Hello world",
        }
    )


def test_scn002056_valid_process_payload_url_only():
    """Valid payload with URL instead of raw_text passes."""
    validate_process_payload(
        {
            "artifact_id": "01ABCDEF",
            "content_type": "youtube",
            "url": "https://youtube.com/watch?v=abc",
        }
    )


def test_scn002056_empty_artifact_id_rejected():
    """Empty artifact_id is rejected."""
    with pytest.raises(PayloadValidationError, match="artifact_id is required"):
        validate_process_payload(
            {
                "artifact_id": "",
                "content_type": "article",
                "raw_text": "text",
            }
        )


def test_scn002056_missing_artifact_id_rejected():
    """Missing artifact_id key is rejected."""
    with pytest.raises(PayloadValidationError, match="artifact_id is required"):
        validate_process_payload(
            {
                "content_type": "article",
                "raw_text": "text",
            }
        )


def test_scn002056_empty_content_type_rejected():
    """Empty content_type is rejected."""
    with pytest.raises(PayloadValidationError, match="content_type is required"):
        validate_process_payload(
            {
                "artifact_id": "01ABCDEF",
                "content_type": "",
                "raw_text": "text",
            }
        )


def test_scn002056_no_content_rejected():
    """Missing both raw_text and url is rejected."""
    with pytest.raises(PayloadValidationError, match="at least one of raw_text or url"):
        validate_process_payload(
            {
                "artifact_id": "01ABCDEF",
                "content_type": "article",
            }
        )


# --- SCN-002-057: validate_processed_result ---


def test_scn002057_valid_processed_result():
    """Valid result payload passes."""
    validate_processed_result(
        {
            "artifact_id": "01ABCDEF",
            "success": True,
            "result": {"title": "Test"},
        }
    )


def test_scn002057_empty_artifact_id_rejected():
    """Empty artifact_id in result is rejected."""
    with pytest.raises(PayloadValidationError, match="artifact_id is required"):
        validate_processed_result(
            {
                "artifact_id": "",
                "success": True,
            }
        )


def test_scn002057_missing_artifact_id_rejected():
    """Missing artifact_id key in result is rejected."""
    with pytest.raises(PayloadValidationError, match="artifact_id is required"):
        validate_processed_result(
            {
                "success": True,
            }
        )
