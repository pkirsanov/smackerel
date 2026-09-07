#!/usr/bin/env python3
"""Compatibility entrypoint for the current IMP-055 typed-graph selftest."""
from __future__ import annotations

import importlib.util
import json
import tempfile
import unittest
from pathlib import Path
from typing import Any, Callable

HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("measured_budget_runtime", HERE / "measured-budget-runtime.py")
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("measured budget runtime module is unavailable")
mbe = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(mbe)

T0 = "2026-08-01T00:00:00.000Z"
T1 = "2026-08-01T00:00:01.000Z"
T2 = "2026-08-01T00:00:02.000Z"
T3 = "2026-08-01T00:00:03.000Z"
T4 = "2026-08-01T00:00:04.000Z"
T5 = "2026-08-01T00:00:05.000Z"
T6 = "2026-08-01T00:00:06.000Z"
EXPIRY = "2026-08-01T00:10:00.000Z"
D1 = mbe.ecf.typed_digest("selftest", {"value": 1})
D2 = mbe.ecf.typed_digest("selftest", {"value": 2})
D3 = mbe.ecf.typed_digest("selftest", {"value": 3})


def amount(name: str, value: int, currency: str = "USD", scale: int = 2) -> dict[str, Any]:
    definition = mbe.DIMENSION_DEFINITIONS[name]
    return {
        "dimension": name,
        "amount": value,
        "unit": definition["unit"],
        "currency": currency if name == "monetaryMinorUnits" else None,
        "scale": scale if name == "monetaryMinorUnits" else None,
    }


def policies(configured: dict[str, int]) -> list[dict[str, Any]]:
    rows = []
    for name in mbe.DIMENSIONS:
        enabled = name in configured
        rows.append({
            "dimension": name,
            "state": "configured" if enabled else "unconfigured",
            "limit": configured.get(name),
            "unit": mbe.DIMENSION_DEFINITIONS[name]["unit"],
            "currency": "USD" if enabled and name == "monetaryMinorUnits" else None,
            "scale": 2 if enabled and name == "monetaryMinorUnits" else None,
        })
    return rows


def capabilities(overrides: dict[str, dict[str, Any]] | None = None) -> list[dict[str, Any]]:
    changes = overrides or {}
    return [{
        "dimension": name,
        "mode": changes.get(name, {}).get("mode", "native"),
        "preDispatchBound": changes.get(name, {}).get("preDispatchBound", True),
        "postDispatchActual": changes.get(name, {}).get("postDispatchActual", True),
    } for name in mbe.DIMENSIONS]


def host_proof(previous: str, following: str, continuation: str = D1) -> dict[str, Any]:
    return {
        "contractType": "independently-verifiable-host-proof",
        "schemaVersion": 1,
        "proofType": "new-session",
        "proofId": f"proof:{previous}.{following}",
        "issuer": "host-verifier",
        "previousSessionIdentity": previous,
        "nextSessionIdentity": following,
        "continuationDigest": continuation,
        "verificationDigest": D2,
        "verified": True,
    }


class RuntimeCase(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="mbe-selftest-")
        self.runtime = mbe.MeasuredBudgetRuntime(self.temp.name)

    def tearDown(self) -> None:
        self.temp.cleanup()

    def expect_code(self, code: str, call: Callable[[], Any]) -> mbe.MbeError:
        with self.assertRaises(mbe.MbeError) as caught:
            call()
        self.assertEqual(caught.exception.code, code)
        return caught.exception

    def open_budget(self, configured: dict[str, int], goal: str = "goal:selftest") -> dict[str, Any]:
        return self.runtime.budget_open(
            goal_id=goal,
            policy_digest=D1,
            rollout_posture="reference-enforce",
            dimensions=policies(configured),
            opened_at=T0,
        )

    def reserve(self, budget: dict[str, Any], rows: list[dict[str, Any]], occurrence: str = "occ:one", attempt: str = "att:one", action_digest: str = D2) -> dict[str, Any]:
        snapshot = self.runtime.budget_snapshot(budget_id=budget["budgetId"], at=T1)
        return self.runtime.budget_reserve(
            budget_id=budget["budgetId"], occurrence_id=occurrence, attempt_id=attempt,
            action_digest=action_digest, quote_digest=D3, amounts=rows, expires_at=EXPIRY,
            expected_ecf_sequence=snapshot["ecfSequence"], expected_ecf_head_digest=snapshot["ecfHeadDigest"], at=T2,
        )

    def reconcile(self, method: Callable[..., dict[str, Any]], budget: dict[str, Any], reservation: dict[str, Any], rows: list[dict[str, Any]], occurrence: str, at: str, receipt: str | None = "receipt:one") -> dict[str, Any]:
        return method(
            budget_id=budget["budgetId"], reservation_id=reservation["reservationId"], amounts=rows,
            occurrence_id=occurrence, attempt_id=f"att:{occurrence.removeprefix('occ:')}", action_digest=reservation["actionDigest"],
            usage_receipt_id=receipt, at=at, reason="selftest",
        )

    def admission_fixture(self, *, authorized: bool = True, model_identity: str | None = None, model_verified: bool = False, model_class: str = "none", capability_rows: list[dict[str, Any]] | None = None) -> tuple[dict[str, Any], dict[str, Any], dict[str, Any]]:
        budget = self.open_budget({"modelRequestCount": 3})
        epoch = self.runtime.epoch_open(goal_id=budget["goalId"], epoch_class="planning", sequence=1, host_session_identity_id="session:one", continuation_digest=D1, opened_at=T1, host_proof=None)
        reservation = self.reserve(budget, [amount("modelRequestCount", 1)])
        decision = self.runtime.admission_evaluate(
            budget_id=budget["budgetId"], session_identity="session:one", epoch_id=epoch["epochId"],
            occurrence_id=reservation["occurrenceId"], attempt_id=reservation["attemptId"], action_digest=reservation["actionDigest"],
            quote_digest=reservation["quoteDigest"], reservation_id=reservation["reservationId"], adapter_id="adapter:selftest",
            policy_digest=D1, action_authorized=authorized, model_identity=model_identity, model_identity_verified=model_verified,
            requested_model_class=model_class, capabilities=capability_rows or capabilities(), evaluated_at=T3,
        )
        return budget, reservation, decision

    def issue(self, reservation: dict[str, Any], decision: dict[str, Any], nonce: str = "nonce:one") -> dict[str, Any]:
        return self.runtime.permit_issue(
            decision_id=decision["decisionId"], session_identity=decision["sessionIdentity"], epoch_id=decision["epochId"],
            occurrence_id=decision["occurrenceId"], attempt_id=decision["attemptId"], action_digest=decision["actionDigest"],
            quote_digest=decision["quoteDigest"], reservation_id=reservation["reservationId"], adapter_id=decision["adapterId"],
            policy_digest=decision["policyDigest"], nonce=nonce, expires_at=EXPIRY, issued_at=T4,
            enforcement_kind="repository-reference",
        )

    def consume(self, permit: dict[str, Any], **changes: Any) -> dict[str, Any]:
        values = {key: permit[key] for key in ("sessionIdentity", "epochId", "occurrenceId", "attemptId", "actionDigest", "quoteDigest", "reservationId", "adapterId", "policyDigest", "nonce")}
        values.update(changes)
        return self.runtime.permit_consume(
            permit_id=permit["permitId"], session_identity=values["sessionIdentity"], epoch_id=values["epochId"],
            occurrence_id=values["occurrenceId"], attempt_id=values["attemptId"], action_digest=values["actionDigest"],
            quote_digest=values["quoteDigest"], reservation_id=values["reservationId"], adapter_id=values["adapterId"],
            policy_digest=values["policyDigest"], nonce=values["nonce"], consumed_at=values.get("consumedAt", T5),
        )


class BudgetTests(RuntimeCase):
    def test_all_dimensions_below_exact_and_above(self) -> None:
        for index, name in enumerate(mbe.DIMENSIONS):
            with self.subTest(dimension=name):
                with tempfile.TemporaryDirectory(prefix=f"mbe-dimension-{index}-") as root:
                    runtime = mbe.MeasuredBudgetRuntime(root)
                    budget = runtime.budget_open(goal_id=f"goal:dimension.{index}", policy_digest=D1, rollout_posture="reference-enforce", dimensions=policies({name: 10}), opened_at=T0)
                    first_snapshot = runtime.budget_snapshot(budget_id=budget["budgetId"], at=T1)
                    first = runtime.budget_reserve(budget_id=budget["budgetId"], occurrence_id=f"occ:below.{index}", attempt_id=f"att:below.{index}", action_digest=D2, quote_digest=D3, amounts=[amount(name, 9)], expires_at=EXPIRY, expected_ecf_sequence=first_snapshot["ecfSequence"], expected_ecf_head_digest=first_snapshot["ecfHeadDigest"], at=T2)
                    runtime.budget_debit(budget_id=budget["budgetId"], reservation_id=first["reservationId"], amounts=[amount(name, 9)], occurrence_id=f"occ:debit.below.{index}", attempt_id=f"att:debit.below.{index}", action_digest=D2, usage_receipt_id=f"receipt:below.{index}", at=T3, reason="below")
                    second_snapshot = runtime.budget_snapshot(budget_id=budget["budgetId"], at=T3)
                    with self.assertRaises(mbe.MbeError) as exhausted_after_debit:
                        runtime.budget_reserve(budget_id=budget["budgetId"], occurrence_id=f"occ:exact.{index}", attempt_id=f"att:exact.{index}", action_digest=D2, quote_digest=D3, amounts=[amount(name, 10)], expires_at=EXPIRY, expected_ecf_sequence=second_snapshot["ecfSequence"], expected_ecf_head_digest=second_snapshot["ecfHeadDigest"], at=T4)
                    self.assertEqual(exhausted_after_debit.exception.code, "MBE-BUDGET-EXHAUSTED")
                with tempfile.TemporaryDirectory(prefix=f"mbe-boundary-{index}-") as boundary_root:
                    boundary_runtime = mbe.MeasuredBudgetRuntime(boundary_root)
                    boundary_budget = boundary_runtime.budget_open(goal_id=f"goal:boundary.{index}", policy_digest=D1, rollout_posture="reference-enforce", dimensions=policies({name: 10}), opened_at=T0)
                    boundary_snapshot = boundary_runtime.budget_snapshot(budget_id=boundary_budget["budgetId"], at=T1)
                    exact = boundary_runtime.budget_reserve(budget_id=boundary_budget["budgetId"], occurrence_id=f"occ:exact.{index}", attempt_id=f"att:exact.{index}", action_digest=D2, quote_digest=D3, amounts=[amount(name, 10)], expires_at=EXPIRY, expected_ecf_sequence=boundary_snapshot["ecfSequence"], expected_ecf_head_digest=boundary_snapshot["ecfHeadDigest"], at=T2)
                    self.assertEqual(exact["state"], "reserved")
                    after_exact = boundary_runtime.budget_snapshot(budget_id=boundary_budget["budgetId"], at=T3)
                    with self.assertRaises(mbe.MbeError) as above:
                        boundary_runtime.budget_reserve(budget_id=boundary_budget["budgetId"], occurrence_id=f"occ:above.{index}", attempt_id=f"att:above.{index}", action_digest=D2, quote_digest=D3, amounts=[amount(name, 1)], expires_at=EXPIRY, expected_ecf_sequence=after_exact["ecfSequence"], expected_ecf_head_digest=after_exact["ecfHeadDigest"], at=T4)
                    self.assertEqual(above.exception.code, "MBE-BUDGET-EXHAUSTED")

    def test_atomic_reservation_currency_scale_and_stale_snapshot(self) -> None:
        budget = self.open_budget({"inputTokens": 10, "outputTokens": 10, "monetaryMinorUnits": 100})
        snapshot = self.runtime.budget_snapshot(budget_id=budget["budgetId"], at=T1)
        self.expect_code("MBE-BUDGET-EXHAUSTED", lambda: self.runtime.budget_reserve(
            budget_id=budget["budgetId"], occurrence_id="occ:atomic", attempt_id="att:atomic", action_digest=D2, quote_digest=D3,
            amounts=[amount("inputTokens", 5), amount("outputTokens", 11)], expires_at=EXPIRY,
            expected_ecf_sequence=snapshot["ecfSequence"], expected_ecf_head_digest=snapshot["ecfHeadDigest"], at=T2))
        clean = self.runtime.budget_snapshot(budget_id=budget["budgetId"], at=T2)
        self.assertEqual(sum(row["reserved"] for row in clean["dimensions"]), 0)
        self.expect_code("MBE-DIMENSION-UNCONFIGURED", lambda: self.runtime.budget_reserve(
            budget_id=budget["budgetId"], occurrence_id="occ:currency", attempt_id="att:currency", action_digest=D2, quote_digest=D3,
            amounts=[amount("monetaryMinorUnits", 1, "CAD", 2)], expires_at=EXPIRY,
            expected_ecf_sequence=clean["ecfSequence"], expected_ecf_head_digest=clean["ecfHeadDigest"], at=T2))
        self.expect_code("MBE-DIMENSION-UNCONFIGURED", lambda: self.runtime.budget_reserve(
            budget_id=budget["budgetId"], occurrence_id="occ:scale", attempt_id="att:scale", action_digest=D2, quote_digest=D3,
            amounts=[amount("monetaryMinorUnits", 1, "USD", 3)], expires_at=EXPIRY,
            expected_ecf_sequence=clean["ecfSequence"], expected_ecf_head_digest=clean["ecfHeadDigest"], at=T2))
        self.runtime.epoch_open(goal_id=budget["goalId"], epoch_class="planning", sequence=1, host_session_identity_id="session:stale", continuation_digest=D1, opened_at=T2, host_proof=None)
        self.expect_code("MBE-RESERVATION-CONFLICT", lambda: self.runtime.budget_reserve(
            budget_id=budget["budgetId"], occurrence_id="occ:stale", attempt_id="att:stale", action_digest=D2, quote_digest=D3,
            amounts=[amount("inputTokens", 1)], expires_at=EXPIRY,
            expected_ecf_sequence=clean["ecfSequence"], expected_ecf_head_digest=clean["ecfHeadDigest"], at=T3))

    def test_lifecycle_hold_duplicate_retry_correction_and_close(self) -> None:
        budget = self.open_budget({"wallTimeMs": 100, "inputTokens": 100})
        reservation = self.reserve(budget, [amount("wallTimeMs", 10), amount("inputTokens", 20)])
        hold = self.reconcile(self.runtime.budget_hold, budget, reservation, [amount("wallTimeMs", 10), amount("inputTokens", 20)], "occ:hold", T3, None)
        stale = self.runtime.budget_snapshot(budget_id=budget["budgetId"], at=T3)
        self.expect_code("MBE-USAGE-UNRESOLVED", lambda: self.runtime.budget_reserve(
            budget_id=budget["budgetId"], occurrence_id=reservation["occurrenceId"], attempt_id="att:retry", action_digest=reservation["actionDigest"], quote_digest=D3,
            amounts=[amount("inputTokens", 1)], expires_at=EXPIRY, expected_ecf_sequence=stale["ecfSequence"], expected_ecf_head_digest=stale["ecfHeadDigest"], at=T4))
        debit = self.reconcile(self.runtime.budget_debit, budget, reservation, [amount("wallTimeMs", 10)], "occ:debit", T4)
        release = self.reconcile(self.runtime.budget_release, budget, reservation, [amount("inputTokens", 20)], "occ:release", T5)
        correction = self.runtime.budget_correct(budget_id=budget["budgetId"], reservation_id=reservation["reservationId"], amounts=[amount("inputTokens", 1)], occurrence_id="occ:correct", attempt_id="att:correct", action_digest=D2, usage_receipt_id="receipt:correct", supersedes_event_id=debit["budgetEventId"], at=T6, reason="correction")
        self.assertEqual((hold["eventType"], debit["eventType"], release["eventType"], correction["eventType"]), ("HOLD", "DEBIT", "RELEASE", "CORRECT"))
        final = self.runtime.budget_snapshot(budget_id=budget["budgetId"], at=T6)
        self.assertEqual(sum(row["held"] for row in final["dimensions"]), 0)
        self.runtime.budget_close(budget_id=budget["budgetId"], occurrence_id="occ:close", attempt_id="att:close", action_digest=D1, at=T6, reason="complete")
        closed = self.runtime.budget_snapshot(budget_id=budget["budgetId"], at=T6)
        self.assertEqual(closed["state"], "closed")


class AdmissionAndEpochTests(RuntimeCase):
    def test_configured_unmeasurable_unconfigured_truth_action_and_model_denials(self) -> None:
        _, _, unmeasurable = self.admission_fixture(capability_rows=capabilities({"modelRequestCount": {"mode": "unsupported", "preDispatchBound": False, "postDispatchActual": False}}))
        self.assertEqual((unmeasurable["verdict"], unmeasurable["reasonCode"]), ("refused", "MBE-DIMENSION-UNMEASURABLE"))
        snapshot = self.runtime.budget_snapshot(budget_id=unmeasurable["budgetId"], at=T4)
        states = {row["dimension"]: row["state"] for row in snapshot["dimensions"]}
        self.assertEqual(states["modelRequestCount"], "measured")
        self.assertTrue(all(states[name] == "unmeasured" for name in mbe.DIMENSIONS if name != "modelRequestCount"))
        with tempfile.TemporaryDirectory(prefix="mbe-action-") as root:
            other = AdmissionAndEpochTests(methodName="runTest")
            other.temp = tempfile.TemporaryDirectory(dir=root)
            other.runtime = mbe.MeasuredBudgetRuntime(other.temp.name)
            _, _, denied = other.admission_fixture(authorized=False)
            self.assertEqual(denied["reasonCode"], "MBE-ACTION-DENIED")
            other.temp.cleanup()
        with tempfile.TemporaryDirectory(prefix="mbe-model-") as root:
            runtime = mbe.MeasuredBudgetRuntime(root)
            budget = runtime.budget_open(goal_id="goal:model", policy_digest=D1, rollout_posture="reference-enforce", dimensions=policies({"modelRequestCount": 2}), opened_at=T0)
            epoch = runtime.epoch_open(goal_id=budget["goalId"], epoch_class="planning", sequence=1, host_session_identity_id="session:model", continuation_digest=D1, opened_at=T1, host_proof=None)
            snap = runtime.budget_snapshot(budget_id=budget["budgetId"], at=T1)
            reservation = runtime.budget_reserve(budget_id=budget["budgetId"], occurrence_id="occ:model", attempt_id="att:model", action_digest=D2, quote_digest=D3, amounts=[amount("modelRequestCount", 1)], expires_at=EXPIRY, expected_ecf_sequence=snap["ecfSequence"], expected_ecf_head_digest=snap["ecfHeadDigest"], at=T2)
            decision = runtime.admission_evaluate(budget_id=budget["budgetId"], session_identity="session:model", epoch_id=epoch["epochId"], occurrence_id="occ:model", attempt_id="att:model", action_digest=D2, quote_digest=D3, reservation_id=reservation["reservationId"], adapter_id="adapter:model", policy_digest=D1, action_authorized=True, model_identity="provider:model", model_identity_verified=False, requested_model_class="standard-reasoning", capabilities=capabilities(), evaluated_at=T3)
            self.assertEqual((decision["modelIdentityState"], decision["reasonCode"]), ("unverified", "MBE-MODEL-CLASS-UNAVAILABLE"))

    def test_permit_one_use_expiry_and_all_bindings(self) -> None:
        _, reservation, decision = self.admission_fixture()
        self.assertEqual(decision["verdict"], "permit-eligible")
        permit = self.issue(reservation, decision)
        for field, wrong in (("sessionIdentity", "session:wrong"), ("epochId", "sep:wrong"), ("occurrenceId", "occ:wrong"), ("attemptId", "att:wrong"), ("actionDigest", D1), ("nonce", "nonce:wrong")):
            with self.subTest(field=field):
                self.expect_code("MBE-PERMIT-INVALID", lambda field=field, wrong=wrong: self.consume(permit, **{field: wrong}))
        self.expect_code("MBE-PERMIT-INVALID", lambda: self.consume(permit, consumedAt="2026-08-01T00:11:00.000Z"))
        consumed = self.consume(permit)
        self.assertEqual(consumed["permitId"], permit["permitId"])
        self.expect_code("MBE-PERMIT-INVALID", lambda: self.consume(permit))
        self.expect_code("MBE-HOST-ENFORCEMENT-UNAVAILABLE", lambda: self.runtime.permit_issue(
            decision_id=decision["decisionId"], session_identity=decision["sessionIdentity"], epoch_id=decision["epochId"], occurrence_id=decision["occurrenceId"], attempt_id=decision["attemptId"], action_digest=decision["actionDigest"], quote_digest=decision["quoteDigest"], reservation_id=reservation["reservationId"], adapter_id=decision["adapterId"], policy_digest=D1, nonce="nonce:host", expires_at=EXPIRY, issued_at=T4, enforcement_kind="host-native"))

    def test_verified_epoch_chain_rejects_fake_fresh_context_and_preserves_budget(self) -> None:
        budget = self.open_budget({"modelRequestCount": 5})
        first = self.runtime.epoch_open(goal_id=budget["goalId"], epoch_class="planning", sequence=1, host_session_identity_id="session:first", continuation_digest=D1, opened_at=T1, host_proof=None)
        self.runtime.epoch_close(epoch_id=first["epochId"], continuation_digest=D1, closed_at=T2)
        self.expect_code("MBE-EPOCH-BOUNDARY-UNVERIFIED", lambda: self.runtime.epoch_open(goal_id=budget["goalId"], epoch_class="implementation", sequence=2, host_session_identity_id="session:fake", continuation_digest=D1, opened_at=T3, host_proof=None))
        wrong_predecessor = host_proof("session:unrelated", "session:second")
        self.expect_code("MBE-EPOCH-BOUNDARY-UNVERIFIED", lambda: self.runtime.epoch_open(goal_id=budget["goalId"], epoch_class="implementation", sequence=2, host_session_identity_id="session:second", continuation_digest=D1, opened_at=T3, host_proof=wrong_predecessor))
        proof = host_proof("session:first", "session:second")
        second = self.runtime.epoch_open(goal_id=budget["goalId"], epoch_class="implementation", sequence=2, host_session_identity_id="session:second", continuation_digest=D1, opened_at=T3, host_proof=proof)
        verification = self.runtime.epoch_verify(epoch_id=second["epochId"], host_proof=proof, verified_at=T4)
        self.assertEqual(verification["epochId"], second["epochId"])
        records = self.runtime.records()
        policies_found = [row for row in records if row.get("contractType") == "goal-budget-policy" and row.get("goalId") == budget["goalId"]]
        self.assertEqual([row["budgetId"] for row in policies_found], [budget["budgetId"]])


class CorpusAndContractTests(RuntimeCase):
    def test_frozen_corpus_counterfactual_only_and_contract_shapes(self) -> None:
        rows = [{"rowId": "row:one", "sessionDigest": D1, "mode": "implement", "phase": "implementation", "epochClass": "implementation", "riskClass": "bounded", "modelClass": "none", "toolFamily": "repository", "measurementStatus": "measured", "outcome": "complete", "evidenceDigest": D2, "exclusionReason": None},
                {"rowId": "row:two", "sessionDigest": D2, "mode": "implement", "phase": "verification", "epochClass": "verification", "riskClass": "bounded", "modelClass": "none", "toolFamily": "repository", "measurementStatus": "invalid", "outcome": "excluded", "evidenceDigest": D3, "exclusionReason": "invalid-measurement"}]
        corpus = self.runtime.corpus_seal(source_snapshot_digest=D1, adapter_versions=["adapter-v2"], rows=rows, sealed_at=T0)
        evaluation = self.runtime.corpus_evaluate(corpus_id=corpus["corpusId"], candidate_policy_digest=D2, quality_report_digest=D3, track="counterfactual", requested_reduction=False, evaluated_at=T1)
        self.assertEqual((evaluation["includedRows"], evaluation["excludedRows"], evaluation["causalSavingsClaim"], evaluation["reduction"]), (1, 1, "unsupported", None))
        self.assertEqual(evaluation["qualityReportDigest"], D3)
        self.assertEqual(evaluation["measurementCoverage"], {"basis": "frozen-corpus-included-rows", "includedRows": 1, "measuredRows": 1, "partiallyMeasuredRows": 0, "unmeasuredRows": 0, "invalidRows": 0, "supersededRows": 0, "coverageBasisPoints": 10000})
        self.assertEqual(evaluation["exclusions"], [{"reason": "invalid-measurement", "count": 1}])
        self.expect_code("MBE-LIVE-SAVINGS-UNSUPPORTED", lambda: self.runtime.corpus_evaluate(corpus_id=corpus["corpusId"], candidate_policy_digest=D2, quality_report_digest=D3, track="live-canary", requested_reduction=False, evaluated_at=T2))
        self.expect_code("MBE-LIVE-SAVINGS-UNSUPPORTED", lambda: self.runtime.corpus_evaluate(corpus_id=corpus["corpusId"], candidate_policy_digest=D2, quality_report_digest=D3, track="counterfactual", requested_reduction=True, evaluated_at=T2))
        facade = HERE / "cost-corpus-evaluate.sh"
        self.assertIn("corpus-evaluate", facade.read_text(encoding="utf-8"))
        schemas = [HERE.parent / "schemas" / name for name in ("dispatch-admission.schema.json", "usage-adapter-v2.schema.json", "session-epoch.schema.json")]
        for path in schemas:
            parsed = json.loads(path.read_text(encoding="utf-8"))
            self.assertEqual(parsed["$schema"], "http://json-schema.org/draft-07/schema#")
            self.assertIn("oneOf", parsed)
        dimensions = json.loads((HERE.parent / "registry" / "admission-dimensions.yaml").read_text(encoding="utf-8"))
        classes = json.loads((HERE.parent / "registry" / "model-classes.yaml").read_text(encoding="utf-8"))
        self.assertEqual(len(dimensions["dimensions"]), 15)
        self.assertEqual({row["name"] for row in classes["classes"]}, set(mbe.MODEL_CLASSES))
        self.assertNotIn("provider", json.dumps(classes).lower())


if __name__ == "__main__":
    suite_path = HERE / "measured-budget-runtime-v2-selftest.py"
    suite_spec = importlib.util.spec_from_file_location("measured_budget_runtime_v2_selftest", suite_path)
    if suite_spec is None or suite_spec.loader is None:
        raise RuntimeError("current measured-budget runtime selftest is unavailable")
    current_suite = importlib.util.module_from_spec(suite_spec)
    suite_spec.loader.exec_module(current_suite)
    tests = unittest.defaultTestLoader.loadTestsFromModule(current_suite)
    result = unittest.TextTestRunner(verbosity=2).run(tests)
    raise SystemExit(0 if result.wasSuccessful() else 1)
