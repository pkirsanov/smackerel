"""Shared NATS contract loader for the Python ML sidecar.

Spec 038 Scope 1 DoD-2 requires that the DRIVE stream and every drive subject
defined in ``config/nats_contract.json`` are validated on Python sidecar
startup. The Go core has matching validation in ``internal/nats/contract_test.go``
(``TestSCN038001_DriveStreamAndSubjectsRequiredInContract``) and at runtime via
``EnsureStreams`` / ``AllStreams``.

The loader fails loud (raises ``ContractValidationError``) when:

* the contract file cannot be read or is not valid JSON, or
* the ``DRIVE`` stream is missing or has the wrong subjects pattern, or
* any required Scope-1 drive subject is missing or bound to the wrong stream.

The hook ``validate_drive_stream_on_startup`` is called from
``ml/app/main.py``'s lifespan handler before the NATS subscriptions are wired.
This means the sidecar refuses to start when the contract regresses, instead
of silently subscribing to a stale subject set.
"""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path

logger = logging.getLogger("smackerel-ml.nats_contract")


class ContractValidationError(RuntimeError):
    """Raised when the shared NATS contract fails Python-side validation."""


# Required DRIVE-stream subjects for Spec 038 Scope 1.
# Future scopes (extract / classify / save) will extend this list.
REQUIRED_DRIVE_SUBJECTS: tuple[str, ...] = (
    "drive.scan.request",
    "drive.scan.result",
    "drive.change.notify",
    "drive.health.report",
)

REQUIRED_DRIVE_STREAM_NAME = "DRIVE"
REQUIRED_DRIVE_STREAM_PATTERN = "drive.>"


def _default_contract_path() -> Path:
    """Resolve the contract path relative to the repo root.

    ``ml/app/nats_contract.py`` -> ``<repo>/config/nats_contract.json``.
    The path is overridable via the ``NATS_CONTRACT_PATH`` env var so tests
    can point the loader at a mutated copy.
    """
    override = os.environ.get("NATS_CONTRACT_PATH")
    if override:
        return Path(override)
    here = Path(__file__).resolve()
    return here.parent.parent.parent / "config" / "nats_contract.json"


def load_contract(path: Path | None = None) -> dict:
    """Load and JSON-parse the shared NATS contract.

    Raises ``ContractValidationError`` on missing file or invalid JSON.
    """
    contract_path = path or _default_contract_path()
    try:
        with contract_path.open(encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError as exc:
        raise ContractValidationError(f"NATS contract file not found at {contract_path}") from exc
    except json.JSONDecodeError as exc:
        raise ContractValidationError(f"NATS contract file at {contract_path} is not valid JSON: {exc}") from exc


def validate_drive_stream(contract: dict) -> None:
    """Validate the DRIVE stream + Scope-1 subjects in ``contract``.

    Raises ``ContractValidationError`` listing every missing or misconfigured
    required key. Surfaces all problems in a single error so an operator can
    fix them in one pass instead of one-at-a-time.
    """
    problems: list[str] = []

    streams = contract.get("streams") or {}
    drive_stream = streams.get(REQUIRED_DRIVE_STREAM_NAME)
    if drive_stream is None:
        problems.append(f"contract.streams is missing required stream {REQUIRED_DRIVE_STREAM_NAME!r}")
    else:
        actual_pattern = drive_stream.get("subjects_pattern")
        if actual_pattern != REQUIRED_DRIVE_STREAM_PATTERN:
            problems.append(
                f"contract.streams[{REQUIRED_DRIVE_STREAM_NAME!r}].subjects_pattern "
                f"= {actual_pattern!r}, want {REQUIRED_DRIVE_STREAM_PATTERN!r}"
            )

    subjects = contract.get("subjects") or {}
    for subj in REQUIRED_DRIVE_SUBJECTS:
        meta = subjects.get(subj)
        if meta is None:
            problems.append(f"contract.subjects is missing required drive subject {subj!r}")
            continue
        stream = meta.get("stream")
        if stream != REQUIRED_DRIVE_STREAM_NAME:
            problems.append(f"contract.subjects[{subj!r}].stream = {stream!r}, want {REQUIRED_DRIVE_STREAM_NAME!r}")

    if problems:
        raise ContractValidationError("NATS contract DRIVE validation failed:\n  - " + "\n  - ".join(problems))


def validate_drive_stream_on_startup(path: Path | None = None) -> dict:
    """Startup hook called from ``ml/app/main.py`` lifespan.

    Loads the contract, runs :func:`validate_drive_stream`, and returns the
    parsed contract for callers that want to inspect further. Re-raises
    ``ContractValidationError`` on failure so the sidecar refuses to start.
    """
    contract = load_contract(path)
    validate_drive_stream(contract)
    logger.info(
        "NATS contract validated: DRIVE stream + %d Scope-1 drive subjects present",
        len(REQUIRED_DRIVE_SUBJECTS),
    )
    return contract
