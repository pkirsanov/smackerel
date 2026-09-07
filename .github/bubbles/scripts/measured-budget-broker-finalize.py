#!/usr/bin/env python3
"""Expose broker-bound adapter context and construct settlement from adapter records."""
from __future__ import annotations

import argparse
import importlib.util
import pathlib
import sys
from typing import Any

HERE = pathlib.Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("measured_budget_runtime", HERE / "measured-budget-runtime.py")
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("measured budget runtime unavailable")
RUNTIME = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNTIME)


def find(records: list[dict[str, Any]], contract: str, field: str, value: str) -> dict[str, Any]:
    matches = [row for row in records if row.get("contractType") == contract and row.get(field) == value]
    if len(matches) != 1:
        raise ValueError(f"unique {contract} unavailable")
    return matches[0]


def main() -> int:
    parser = argparse.ArgumentParser(description="Resolve or finalize one reference-broker dispatch")
    parser.add_argument("operation", choices=("context", "settle"))
    parser.add_argument("--store-root", required=True)
    parser.add_argument("--permit-id", required=True)
    parser.add_argument("--child-exit-code", type=int)
    parser.add_argument("--usage-receipt-id")
    parser.add_argument("--receipt-verification-id")
    parser.add_argument("--at")
    args = parser.parse_args()
    runtime = RUNTIME.MeasuredBudgetRuntime(args.store_root)
    records = runtime.records()
    permit = find(records, "dispatch-permit", "permitId", args.permit_id)
    consumption = find(records, "permit-consumption", "permitId", args.permit_id)
    reservation = find(records, "budget-reservation", "reservationId", permit["reservationId"])
    if args.operation == "context":
        if args.child_exit_code is None:
            raise ValueError("context requires child exit code")
        context = {
            "actionDigest": permit["actionDigest"], "attemptId": permit["attemptId"], "budgetId": permit["budgetId"],
            "childExitCode": args.child_exit_code, "consumptionId": consumption["consumptionId"], "epochId": permit["epochId"],
            "finishedAt": args.at or consumption["consumedAt"], "goalId": permit["goalId"], "intentId": permit["intentId"],
            "measurement": reservation["amounts"], "monotonicFinishedNs": 0, "monotonicStartedNs": 0,
            "occurrenceId": permit["occurrenceId"], "permitId": permit["permitId"],
            "sessionIdentityId": permit["sessionIdentityId"], "startedAt": consumption["consumedAt"],
        }
        sys.stdout.buffer.write(RUNTIME.ecf.canonical_line(context))
        return 0
    if args.usage_receipt_id is None or args.receipt_verification_id is None or args.at is None:
        raise ValueError("settle requires receipt, verification, and settlement time")
    receipt = find(records, "usage-receipt", "usageReceiptId", args.usage_receipt_id)
    verification = find(records, "usage-receipt-verification", "verificationId", args.receipt_verification_id)
    if receipt["adapterId"] != "reference-test" or verification["usageReceiptId"] != receipt["usageReceiptId"]:
        raise ValueError("settlement requires adapter-originated reference-test evidence")
    measured = receipt["measurementStatus"] == "measured"
    terminal = "debit" if measured else "hold"
    partitions = []
    for row in reservation["amounts"]:
        partitions.append({"dimension": row["dimension"], "unit": row["unit"], "currency": row["currency"], "scale": row["scale"], "reserved": row["amount"], "debit": row["amount"] if measured else 0, "release": 0, "hold": 0 if measured else row["amount"]})
    settlement = runtime.budget_settle(budget_id=permit["budgetId"], reservation_id=permit["reservationId"], permit_id=permit["permitId"], consumption_id=consumption["consumptionId"], usage_receipt_id=receipt["usageReceiptId"], receipt_verification_id=verification["verificationId"], revision=1, supersedes_settlement_id=None, partitions=partitions, terminal_state=terminal, settled_at=args.at)
    sys.stdout.buffer.write(RUNTIME.ecf.canonical_line({"receipt": receipt["usageReceiptId"], "verification": verification["verificationId"], "settlement": settlement["settlementId"], "terminalState": terminal}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
