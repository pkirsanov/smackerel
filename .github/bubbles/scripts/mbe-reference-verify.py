#!/usr/bin/env python3
"""Verify additive IMP-055 references against one immutable MBE/ECF graph."""
from __future__ import annotations

import argparse
import importlib.util
import json
from pathlib import Path
from typing import Any

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("bubbles_mbe_reference", HERE / "measured-budget-runtime.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("mbe-reference-verify: measured-budget runtime unavailable")
mbe = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mbe)


class ReferenceError(ValueError):
    pass


def one(records: list[dict[str, Any]], kind: str, field: str, value: str) -> dict[str, Any]:
    matches = [row for row in records if row.get("contractType") == kind and row.get(field) == value]
    if len(matches) != 1:
        raise ReferenceError(f"{field} must identify exactly one {kind} record")
    return matches[0]


def require_text(context: dict[str, Any], name: str) -> str:
    value = context.get(name)
    if not isinstance(value, str) or not value:
        raise ReferenceError(f"{name} must be a non-empty identifier")
    return value


def verify(store_root: str, context: dict[str, Any], purpose: str, action_digest: str | None) -> dict[str, Any]:
    if not isinstance(context, dict):
        raise ReferenceError("context must be an object")
    runtime = mbe.MeasuredBudgetRuntime(store_root)
    records = runtime.records()

    budget_id = require_text(context, "budgetId")
    epoch_id = require_text(context, "epochId")
    verification_id = require_text(context, "epochVerificationId")
    budget = one(records, "goal-budget-policy", "budgetId", budget_id)
    epoch = one(records, "session-epoch", "epochId", epoch_id)
    verification = one(records, "epoch-verification", "epochVerificationId", verification_id)
    boundary = one(records, "epoch-boundary-receipt", "boundaryId", verification.get("boundaryId"))
    if epoch.get("budgetId") != budget_id or epoch.get("goalId") != budget.get("goalId"):
        raise ReferenceError("epoch and budget lineage differ")
    if epoch.get("openedByBoundaryId") != boundary.get("boundaryId") or verification.get("verdict") != "verified":
        raise ReferenceError("epoch is not opened by the referenced verified boundary")
    if epoch.get("epochVerificationId") != verification_id:
        raise ReferenceError("epoch verification reference does not match the opened epoch")

    result: dict[str, Any] = {
        "budgetId": budget_id,
        "epochId": epoch_id,
        "epochVerificationId": verification_id,
        "epochClass": epoch.get("epochClass"),
        "verified": True,
    }
    if purpose == "epoch":
        return result

    decision_id = require_text(context, "admissionDecisionId")
    reservation_id = require_text(context, "reservationId")
    occurrence_id = require_text(context, "occurrenceId")
    attempt_id = require_text(context, "attemptId")
    decision = one(records, "admission-decision", "decisionId", decision_id)
    reservation = one(records, "budget-reservation", "reservationId", reservation_id)
    for field, expected in (("budgetId", budget_id), ("epochId", epoch_id), ("occurrenceId", occurrence_id), ("attemptId", attempt_id)):
        if decision.get(field) != expected or reservation.get(field) != expected:
            raise ReferenceError(f"{field} lineage differs across admission and reservation")
    if decision.get("reservationId") != reservation_id or decision.get("epochVerificationId") != verification_id:
        raise ReferenceError("admission decision references a different reservation or epoch verification")
    if decision.get("verdict") != "permit-eligible":
        raise ReferenceError("admission decision is not permit-eligible")

    permit_id = require_text(context, "permitId")
    permit = one(records, "dispatch-permit", "permitId", permit_id)
    if permit.get("decisionId") != decision_id or permit.get("reservationId") != reservation_id:
        raise ReferenceError("permit lineage differs from admission and reservation")
    if permit.get("enforcementKind") != "repository-reference":
        raise ReferenceError("permit is not repository-reference enforceable")
    if action_digest is not None and permit.get("actionDigest") != action_digest:
        raise ReferenceError("permit action digest differs from the requested action")

    settlements = [row for row in records if row.get("contractType") == "budget-settlement" and row.get("reservationId") == reservation_id]
    current_settlement = max(settlements, key=lambda row: int(row.get("revision", 0)), default=None)
    unresolved_hold = bool(current_settlement and current_settlement.get("terminalState") == "hold")
    result.update({
        "admissionDecisionId": decision_id,
        "reservationId": reservation_id,
        "permitId": permit_id,
        "occurrenceId": occurrence_id,
        "attemptId": attempt_id,
        "actionDigest": permit.get("actionDigest"),
        "unresolvedHold": unresolved_hold,
    })
    return result


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--store-root", required=True)
    context_group = parser.add_mutually_exclusive_group(required=True)
    context_group.add_argument("--context")
    context_group.add_argument("--context-json")
    parser.add_argument("--purpose", choices=("epoch", "dispatch"), required=True)
    parser.add_argument("--action-digest")
    args = parser.parse_args()
    try:
        context = json.loads(
            Path(args.context).read_text(encoding="utf-8")
            if args.context is not None else args.context_json)
        result = verify(args.store_root, context, args.purpose, args.action_digest)
    except (OSError, json.JSONDecodeError, ReferenceError, mbe.MbeError) as error:
        print(f"mbe-reference-verify: {error}", file=__import__("sys").stderr)
        return 3
    print(json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
