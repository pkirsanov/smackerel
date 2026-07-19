"""SCN-002-056 / SCN-002-057: Python NATS payload validation tests."""

import pytest  # noqa: I001
from app.validation import (
    PayloadValidationError,
    validate_crosssource_result,
    validate_process_payload,
    validate_processed_result,
)


VALID_CROSSSOURCE_RESULT = {
    "concept_id": "concept-1",
    "has_genuine_connection": True,
    "insight_text": "Two independent sources describe one decision.",
    "confidence": 0.91,
    "artifact_ids": ["artifact-1", "artifact-2"],
    "prompt_contract_version": "cross-source-connection-v1",
    "processing_time_ms": 12,
    "model_used": "test-model",
}


def _crosssource_result(**overrides):
    result = {**VALID_CROSSSOURCE_RESULT, "artifact_ids": list(VALID_CROSSSOURCE_RESULT["artifact_ids"])}
    result.update(overrides)
    return result


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


def test_validate_crosssource_result_accepts_valid_genuine_connection():
    """SCN-B0255-001: the documented concept-centric response is valid."""
    validate_crosssource_result(_crosssource_result())


def test_validate_crosssource_result_accepts_empty_insight_for_non_connection():
    """A surface-level overlap may carry no insight text."""
    validate_crosssource_result(
        _crosssource_result(
            has_genuine_connection=False,
            insight_text="",
            confidence=0.0,
            model_used="",
        )
    )


@pytest.mark.parametrize("concept_id", [None, "", "   ", 7])
def test_validate_crosssource_result_rejects_invalid_concept_id(concept_id):
    with pytest.raises(PayloadValidationError, match="concept_id is required"):
        validate_crosssource_result(_crosssource_result(concept_id=concept_id))


@pytest.mark.parametrize("value", [None, 0, 1, "true"])
def test_validate_crosssource_result_rejects_non_boolean_connection_flag(value):
    with pytest.raises(PayloadValidationError, match="has_genuine_connection must be a boolean"):
        validate_crosssource_result(_crosssource_result(has_genuine_connection=value))


@pytest.mark.parametrize("value", [None, 7, [], {}])
def test_validate_crosssource_result_rejects_non_string_insight(value):
    with pytest.raises(PayloadValidationError, match="insight_text must be a string"):
        validate_crosssource_result(_crosssource_result(insight_text=value))


def test_validate_crosssource_result_requires_insight_for_genuine_connection():
    with pytest.raises(PayloadValidationError, match="insight_text is required for a genuine connection"):
        validate_crosssource_result(_crosssource_result(insight_text="  "))


@pytest.mark.parametrize("confidence", [None, True, "0.5", float("nan"), float("inf"), -0.01, 1.01])
def test_validate_crosssource_result_rejects_invalid_confidence(confidence):
    with pytest.raises(PayloadValidationError, match="confidence must"):
        validate_crosssource_result(_crosssource_result(confidence=confidence))


@pytest.mark.parametrize(
    "artifact_ids",
    [
        None,
        "artifact-1,artifact-2",
        [],
        ["artifact-1"],
        ["artifact-1", ""],
        ["artifact-1", 2],
        ["artifact-1", "artifact-1"],
    ],
)
def test_validate_crosssource_result_rejects_malformed_artifact_ids(artifact_ids):
    with pytest.raises(PayloadValidationError, match="artifact_ids must"):
        validate_crosssource_result(_crosssource_result(artifact_ids=artifact_ids))


@pytest.mark.parametrize("prompt_contract_version", [None, "", "   ", 1])
def test_validate_crosssource_result_rejects_invalid_prompt_version(prompt_contract_version):
    with pytest.raises(PayloadValidationError, match="prompt_contract_version is required"):
        validate_crosssource_result(_crosssource_result(prompt_contract_version=prompt_contract_version))


@pytest.mark.parametrize("processing_time_ms", [None, True, -1, 1.5, float("nan")])
def test_validate_crosssource_result_rejects_invalid_processing_time(processing_time_ms):
    with pytest.raises(PayloadValidationError, match="processing_time_ms must be a non-negative integer"):
        validate_crosssource_result(_crosssource_result(processing_time_ms=processing_time_ms))


@pytest.mark.parametrize("model_used", [None, 7, []])
def test_validate_crosssource_result_rejects_non_string_model(model_used):
    with pytest.raises(PayloadValidationError, match="model_used must be a string"):
        validate_crosssource_result(_crosssource_result(model_used=model_used))
