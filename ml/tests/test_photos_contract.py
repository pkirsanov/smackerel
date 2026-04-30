"""SCN-040-001/003 photo NATS and ML decision contract tests."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.nats_contract import (
    REQUIRED_PHOTOS_STREAM_NAME,
    REQUIRED_PHOTOS_STREAM_PATTERN,
    REQUIRED_PHOTOS_SUBJECTS,
    ContractValidationError,
    load_contract,
    validate_photos_stream,
    validate_photos_stream_on_startup,
)
from app.photos import PhotoPayloadValidationError, validate_photo_result


def _real_contract() -> dict:
    return load_contract()


def _write_mutated_contract(tmp_path: Path, mutator) -> Path:
    contract = _real_contract()
    mutator(contract)
    out = tmp_path / "nats_contract.mutated.json"
    out.write_text(json.dumps(contract), encoding="utf-8")
    return out


class TestRealPhotosContractPasses:
    def test_real_contract_has_photos_stream(self):
        contract = validate_photos_stream_on_startup()
        assert contract["streams"][REQUIRED_PHOTOS_STREAM_NAME]["subjects_pattern"] == REQUIRED_PHOTOS_STREAM_PATTERN
        for subject in REQUIRED_PHOTOS_SUBJECTS:
            assert contract["subjects"][subject]["stream"] == REQUIRED_PHOTOS_STREAM_NAME


class TestPhotosContractRejectsMutations:
    def test_missing_photos_stream_raises(self, tmp_path):
        def remove_stream(contract: dict) -> None:
            del contract["streams"][REQUIRED_PHOTOS_STREAM_NAME]

        path = _write_mutated_contract(tmp_path, remove_stream)
        with pytest.raises(ContractValidationError) as exc:
            validate_photos_stream_on_startup(path=path)
        assert REQUIRED_PHOTOS_STREAM_NAME in str(exc.value)
        assert "missing required stream" in str(exc.value)

    def test_wrong_photos_pattern_raises(self, tmp_path):
        def break_pattern(contract: dict) -> None:
            contract["streams"][REQUIRED_PHOTOS_STREAM_NAME]["subjects_pattern"] = "images.>"

        path = _write_mutated_contract(tmp_path, break_pattern)
        with pytest.raises(ContractValidationError) as exc:
            validate_photos_stream_on_startup(path=path)
        assert REQUIRED_PHOTOS_STREAM_PATTERN in str(exc.value)

    @pytest.mark.parametrize("subject", REQUIRED_PHOTOS_SUBJECTS)
    def test_missing_required_photo_subject_raises(self, tmp_path, subject):
        def remove_subject(contract: dict) -> None:
            del contract["subjects"][subject]

        path = _write_mutated_contract(tmp_path, remove_subject)
        with pytest.raises(ContractValidationError) as exc:
            validate_photos_stream_on_startup(path=path)
        assert subject in str(exc.value)
        assert "missing required photo subject" in str(exc.value)

    @pytest.mark.parametrize("subject", REQUIRED_PHOTOS_SUBJECTS)
    def test_wrong_stream_binding_raises(self, tmp_path, subject):
        def rebind(contract: dict) -> None:
            contract["subjects"][subject]["stream"] = "ARTIFACTS"

        path = _write_mutated_contract(tmp_path, rebind)
        with pytest.raises(ContractValidationError) as exc:
            validate_photos_stream(load_contract(path))
        assert subject in str(exc.value)
        assert "ARTIFACTS" in str(exc.value)


def test_photo_result_requires_confidence_and_rationale():
    base = {
        "request_id": "req-1",
        "photo_id": "00000000-0000-0000-0000-000000000040",
        "artifact_id": "photo-artifact-1",
        "result": {"classification": {"primary_category": "document/receipt"}},
    }
    with pytest.raises(PhotoPayloadValidationError) as exc:
        validate_photo_result("photos.classified", base)
    assert "confidence" in str(exc.value)
    assert "rationale" in str(exc.value)


def test_photo_result_accepts_complete_decision():
    payload = {
        "request_id": "req-1",
        "photo_id": "00000000-0000-0000-0000-000000000040",
        "artifact_id": "photo-artifact-1",
        "result": {
            "classification": {"primary_category": "document/receipt"},
            "confidence": 0.86,
            "rationale": "The image contains a register receipt with vendor and total fields.",
        },
    }
    assert validate_photo_result("photos.classified", payload)["result"]["confidence"] == 0.86
