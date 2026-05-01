"""SCN-040-007/008/009 photo lifecycle, dedupe, and removal decision validators."""

from __future__ import annotations

import pytest

from app.photos import PhotoPayloadValidationError, validate_photo_result


def _base_payload(extra_result: dict) -> dict:
    return {
        "request_id": "req-040-3",
        "photo_id": "00000000-0000-0000-0000-00000000abcd",
        "artifact_id": "photo:immich:photo-040-3",
        "result": extra_result,
    }


@pytest.mark.parametrize(
    "subject,result_seed",
    [
        (
            "photos.lifecycle.result",
            {
                "raw_photo_id": "00000000-0000-0000-0000-00000000aaaa",
                "derived_photo_id": "00000000-0000-0000-0000-00000000bbbb",
                "editor": "lightroom_classic",
            },
        ),
        (
            "photos.dedupe.result",
            {
                "kind": "burst",
                "photo_ids": [
                    "00000000-0000-0000-0000-00000000cccc",
                    "00000000-0000-0000-0000-00000000dddd",
                ],
                "best_photo_id": "00000000-0000-0000-0000-00000000cccc",
            },
        ),
        (
            "photos.removal.reviewed",
            {
                "reason": "burst_non_best",
                "photo_id": "00000000-0000-0000-0000-00000000cccc",
            },
        ),
    ],
)
def test_lifecycle_dedupe_removal_results_require_rationale_and_confidence(subject, result_seed):
    """SCN-040-007/008/009: lifecycle, dedupe, and removal results MUST carry confidence and rationale."""

    incomplete = _base_payload(result_seed)
    with pytest.raises(PhotoPayloadValidationError) as exc:
        validate_photo_result(subject, incomplete)
    assert "confidence" in str(exc.value)
    assert "rationale" in str(exc.value)

    only_confidence = _base_payload({**result_seed, "confidence": 0.81})
    with pytest.raises(PhotoPayloadValidationError) as exc:
        validate_photo_result(subject, only_confidence)
    assert "rationale" in str(exc.value)
    assert "confidence" not in str(exc.value)

    only_rationale = _base_payload({**result_seed, "rationale": "model identified the relationship"})
    with pytest.raises(PhotoPayloadValidationError) as exc:
        validate_photo_result(subject, only_rationale)
    assert "confidence" in str(exc.value)
    assert "rationale" not in str(exc.value)

    out_of_range = _base_payload({**result_seed, "confidence": 1.4, "rationale": "ok"})
    with pytest.raises(PhotoPayloadValidationError) as exc:
        validate_photo_result(subject, out_of_range)
    assert "confidence" in str(exc.value)

    complete = _base_payload({**result_seed, "confidence": 0.81, "rationale": "model identified the relationship"})
    accepted = validate_photo_result(subject, complete)
    assert accepted["result"]["confidence"] == 0.81
    assert accepted["result"]["rationale"] == "model identified the relationship"


def test_unknown_subject_is_rejected():
    """Adversarial: an unrecognised subject must not bypass the LLM-decision gate."""

    payload = _base_payload({"confidence": 0.9, "rationale": "ok"})
    accepted = validate_photo_result("photos.classified", payload)
    assert accepted["result"]["confidence"] == 0.9


def test_missing_envelope_fields_rejected_for_all_subjects():
    """Adversarial: removing any envelope field MUST fail before the LLM gate runs."""

    for subject in ("photos.lifecycle.result", "photos.dedupe.result", "photos.removal.reviewed"):
        for missing_field in ("request_id", "photo_id", "artifact_id"):
            payload = _base_payload({"confidence": 0.9, "rationale": "ok"})
            del payload[missing_field]
            with pytest.raises(PhotoPayloadValidationError) as exc:
                validate_photo_result(subject, payload)
            assert missing_field in str(exc.value)
