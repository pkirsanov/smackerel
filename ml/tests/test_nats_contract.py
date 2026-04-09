"""SCN-002-055: Python NATS subjects match the shared nats_contract.json."""

import json
import os


def _load_contract():
    """Load the shared NATS contract from config/nats_contract.json."""
    # Resolve path relative to repo root (ml/tests/ -> repo root)
    here = os.path.dirname(os.path.abspath(__file__))
    contract_path = os.path.join(here, "..", "..", "config", "nats_contract.json")
    with open(contract_path) as f:
        return json.load(f)


def test_scn002055_subscribe_subjects_match_contract():
    """Every Python SUBSCRIBE_SUBJECTS entry must exist in the contract as a core_to_ml subject."""
    from app.nats_client import SUBSCRIBE_SUBJECTS

    contract = _load_contract()

    core_to_ml = {subj for subj, meta in contract["subjects"].items() if meta["direction"] == "core_to_ml"}

    for subject in SUBSCRIBE_SUBJECTS:
        assert subject in core_to_ml, (
            f"Python SUBSCRIBE_SUBJECTS contains '{subject}' which is not a core_to_ml subject in nats_contract.json"
        )

    # All core_to_ml contract subjects must be in SUBSCRIBE_SUBJECTS
    for subject in core_to_ml:
        assert subject in SUBSCRIBE_SUBJECTS, (
            f"Contract core_to_ml subject '{subject}' is missing from Python SUBSCRIBE_SUBJECTS in nats_client.py"
        )


def test_scn002055_publish_subjects_match_contract():
    """Every Python PUBLISH_SUBJECTS entry must exist in the contract as an ml_to_core subject."""
    from app.nats_client import PUBLISH_SUBJECTS

    contract = _load_contract()

    ml_to_core = {subj for subj, meta in contract["subjects"].items() if meta["direction"] == "ml_to_core"}

    for subject in PUBLISH_SUBJECTS:
        assert subject in ml_to_core, (
            f"Python PUBLISH_SUBJECTS contains '{subject}' which is not an ml_to_core subject in nats_contract.json"
        )

    for subject in ml_to_core:
        assert subject in PUBLISH_SUBJECTS, (
            f"Contract ml_to_core subject '{subject}' is missing from Python PUBLISH_SUBJECTS in nats_client.py"
        )


def test_scn002055_response_map_matches_contract():
    """Python SUBJECT_RESPONSE_MAP must match the contract's request_response_pairs."""
    from app.nats_client import SUBJECT_RESPONSE_MAP

    contract = _load_contract()

    contract_pairs = {p["request"]: p["response"] for p in contract["request_response_pairs"]}

    for req, resp in SUBJECT_RESPONSE_MAP.items():
        assert req in contract_pairs, f"Python SUBJECT_RESPONSE_MAP key '{req}' not in contract pairs"
        assert contract_pairs[req] == resp, (
            f"Python SUBJECT_RESPONSE_MAP['{req}'] = '{resp}' but contract says '{contract_pairs[req]}'"
        )

    for req in contract_pairs:
        assert req in SUBJECT_RESPONSE_MAP, f"Contract pair request '{req}' missing from Python SUBJECT_RESPONSE_MAP"


def test_scn002055_critical_subjects_match_contract():
    """Python CRITICAL_SUBJECTS must match subjects marked critical in the contract."""
    from app.nats_client import CRITICAL_SUBJECTS

    contract = _load_contract()

    contract_critical = {
        subj
        for subj, meta in contract["subjects"].items()
        if meta.get("critical", False) and meta["direction"] == "core_to_ml"
    }

    assert CRITICAL_SUBJECTS == contract_critical, (
        f"Python CRITICAL_SUBJECTS {CRITICAL_SUBJECTS} does not match contract critical subjects {contract_critical}"
    )
