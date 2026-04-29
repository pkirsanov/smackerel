"""SCN-038-001 / Spec 038 Scope 1 DoD-2 — Python sidecar refuses to start when
the shared NATS contract is missing the DRIVE stream or any Scope-1 drive
subject. Mirrors the Go cross-service guarantee in
``internal/nats/contract_test.go::TestSCN038001_DriveStreamAndSubjectsRequiredInContract``.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.nats_contract import (
    REQUIRED_DRIVE_STREAM_NAME,
    REQUIRED_DRIVE_STREAM_PATTERN,
    REQUIRED_DRIVE_SUBJECTS,
    ContractValidationError,
    load_contract,
    validate_drive_stream,
    validate_drive_stream_on_startup,
)


def _real_contract() -> dict:
    return load_contract()


def _write_mutated_contract(tmp_path: Path, mutator) -> Path:
    contract = _real_contract()
    mutator(contract)
    out = tmp_path / "nats_contract.mutated.json"
    out.write_text(json.dumps(contract), encoding="utf-8")
    return out


class TestRealContractPasses:
    def test_real_contract_has_drive_stream(self):
        """Positive baseline — the committed contract validates cleanly."""
        contract = validate_drive_stream_on_startup()
        assert contract["streams"][REQUIRED_DRIVE_STREAM_NAME]["subjects_pattern"] == REQUIRED_DRIVE_STREAM_PATTERN
        for subj in REQUIRED_DRIVE_SUBJECTS:
            assert contract["subjects"][subj]["stream"] == REQUIRED_DRIVE_STREAM_NAME


class TestDriveStreamRemovedRejects:
    def test_missing_drive_stream_raises(self, tmp_path):
        """Adversarial — DRIVE stream removed must fail validation."""

        def remove_drive_stream(contract: dict) -> None:
            del contract["streams"][REQUIRED_DRIVE_STREAM_NAME]

        path = _write_mutated_contract(tmp_path, remove_drive_stream)
        with pytest.raises(ContractValidationError) as exc:
            validate_drive_stream_on_startup(path=path)
        assert REQUIRED_DRIVE_STREAM_NAME in str(exc.value)
        assert "missing required stream" in str(exc.value)

    def test_wrong_drive_pattern_raises(self, tmp_path):
        """Adversarial — DRIVE stream present but wrong pattern must fail."""

        def break_pattern(contract: dict) -> None:
            contract["streams"][REQUIRED_DRIVE_STREAM_NAME]["subjects_pattern"] = "wrong.>"

        path = _write_mutated_contract(tmp_path, break_pattern)
        with pytest.raises(ContractValidationError) as exc:
            validate_drive_stream_on_startup(path=path)
        msg = str(exc.value)
        assert "subjects_pattern" in msg
        assert REQUIRED_DRIVE_STREAM_PATTERN in msg


class TestDriveSubjectsRemovedReject:
    @pytest.mark.parametrize("subject", REQUIRED_DRIVE_SUBJECTS)
    def test_missing_required_subject_raises(self, tmp_path, subject):
        """Adversarial — each Scope-1 drive subject removal must fail loudly."""

        def remove_subject(contract: dict) -> None:
            del contract["subjects"][subject]

        path = _write_mutated_contract(tmp_path, remove_subject)
        with pytest.raises(ContractValidationError) as exc:
            validate_drive_stream_on_startup(path=path)
        assert subject in str(exc.value)
        assert "missing required drive subject" in str(exc.value)

    @pytest.mark.parametrize("subject", REQUIRED_DRIVE_SUBJECTS)
    def test_wrong_stream_binding_raises(self, tmp_path, subject):
        """Adversarial — each subject mis-bound to a non-DRIVE stream must fail."""

        def rebind(contract: dict) -> None:
            contract["subjects"][subject]["stream"] = "OTHER"

        path = _write_mutated_contract(tmp_path, rebind)
        with pytest.raises(ContractValidationError) as exc:
            validate_drive_stream(load_contract(path))
        msg = str(exc.value)
        assert subject in msg
        assert "OTHER" in msg


class TestLoaderInputErrors:
    def test_missing_file_raises(self, tmp_path):
        nope = tmp_path / "does-not-exist.json"
        with pytest.raises(ContractValidationError) as exc:
            load_contract(nope)
        assert "not found" in str(exc.value)

    def test_invalid_json_raises(self, tmp_path):
        bad = tmp_path / "not-json.json"
        bad.write_text("{not valid json", encoding="utf-8")
        with pytest.raises(ContractValidationError) as exc:
            load_contract(bad)
        assert "not valid JSON" in str(exc.value)
