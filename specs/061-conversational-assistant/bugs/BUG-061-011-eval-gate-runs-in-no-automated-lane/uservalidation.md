# User Validation: BUG-061-011 Assistant acceptance gate executes in no automated lane

## Checklist

> Items are **checked `[x]` when validated**. Uncheck an item to report that the behaviour is broken.
>
> **Filing-time note:** the first item below is checked because it was verified by audit in the filing session. The second is left **unchecked** because the fix is not implemented yet — that state records absent behaviour, not a user-reported regression. The implementing agent checks it only after recording the corresponding evidence in `report.md` → *After Fix — Verification*.

### [Audit] BUG-061-011 The gate itself is intact — the defect is wiring only

- [x] **What:** The acceptance gate, its harness, and its corpus are present and structurally complete. Nothing needs to be rebuilt; only the lane wiring is missing.
  - **Steps:**
    1. `find tests/eval -type f`
    2. `grep -cE '^[[:space:]]*-[[:space:]]+id:' tests/eval/assistant/corpus.yaml`
    3. `grep -oE 'ground_truth_intent:[[:space:]]*[a-z-]+' tests/eval/assistant/corpus.yaml | sort | uniq -c`
    4. `grep -c 'ground_truth_capture_expected: true' tests/eval/assistant/corpus.yaml`
  - **Expected:** Five files present (`acceptance_test.go`, `corpus.yaml`, `corpus_validation_test.go`, `harness.go`, `harness_test.go`); 150 corpus rows; 30 rows per label across five labels; 60 capture-expected rows.
  - **Verify:** Observed exactly that — 150 rows, 30 each for `ambiguous-borderline` / `capture` / `notifications` / `retrieval` / `weather`, 60 capture-expected.
  - **Evidence:** report.md → Before Fix — Reproduction → *Supporting evidence gathered in the same session*
  - **Notes:** This bounds the fix. The gate is real and passes when invoked by hand (`docs/Testing.md:755`); it is simply never invoked automatically. The implementing agent must not rewrite the harness, the corpus, or the thresholds.

### [Bug Fix] BUG-061-011 The acceptance gate runs in CI and a skipped gate fails loudly

- [x] **What:** `./smackerel.sh test integration` executes `TestAcceptanceGate_RoutingAccuracyAndCaptureFallback` and reports a non-zero executed-assertion count; a run in which the gate did not execute fails instead of passing green.
  - **Steps:**
    1. `./smackerel.sh config generate && ./smackerel.sh config generate --env test`
    2. `./smackerel.sh --env test up`
    3. `./smackerel.sh test integration`
  - **Expected:** The lane output contains exactly one line beginning `ASSISTANT_ACCEPTANCE_GATE_V1` reporting `executed_assertions=210` for the shipped 150-row corpus, and the lane exits 0.
  - **Verify:** Exit code 0, plus one marker line in the lane output. Then confirm the guard has teeth: `./smackerel.sh test unit --go --go-run 'TestEvalLaneContract' --verbose` passes, including the adversarial cases that reject a lane with `./tests/eval/...` removed, a lane that accepts a zero count, and a gate that emits its marker only on pass.
  - **Evidence:** report.md → After Fix — Verification
  - **Notes:** Delivery-plan item `D27` (`docs/Product_Delivery_Plan.md` § P3), Stage 1 exit criterion. Scope of the claim: this proves **assistant routing quality** is measured in CI. It does **not** cover corpus-grant enforcement (D25/D28, spec 108) — that is a different axis and must not be reported as covered.

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue".

### What was verified before the box was checked

Both halves of the item were re-executed on 2026-08-27, not accepted on the directive alone.

The lane emitted exactly ONE marker line, and the count is the one the item names:

```text
$ ./smackerel.sh test integration
ASSISTANT_ACCEPTANCE_GATE_V1 executed_assertions=210 rows=150 capture_expected=60 routing_accuracy=1.0000 capture_fallback_rate=1.0000
marker lines matching ASSISTANT_ACCEPTANCE_GATE_V1: 1
FAIL lines: 0
Exit Code: 0
```

The guard's teeth were confirmed separately, because a lane that emits its marker only on
pass would satisfy the count while still hiding a skipped gate:

```text
$ ./smackerel.sh test unit --go --go-run 'TestEvalLaneContract' --verbose
--- PASS: TestEvalLaneContract_LaneRunsGateAndAssertsExecutedAssertions (0.00s)
--- PASS: TestEvalLaneContract_AdversarialRejectsMissingEvalPackage (0.00s)
--- PASS: TestEvalLaneContract_AdversarialRejectsMissingOrZeroAssertion (0.00s)
    --- PASS: TestEvalLaneContract_AdversarialRejectsMissingOrZeroAssertion/A3_assertion_accepts_zero (0.00s)
--- PASS: TestEvalLaneContract_AdversarialRejectsConditionalOrAbsentMarker (0.00s)
    --- PASS: TestEvalLaneContract_AdversarialRejectsConditionalOrAbsentMarker/A4_marker_conditional_on_passing (0.00s)
    --- PASS: TestEvalLaneContract_AdversarialRejectsConditionalOrAbsentMarker/A5_marker_never_emitted (0.00s)
--- PASS: TestEvalLaneContract_AdversarialRejectsBypassOrBroadenedSkip (0.00s)
--- PASS: TestEvalLaneContract_AdversarialRejectsMaskedFailureReporting (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  0.028s
Exit Code: 0
```

All three adversarial classes the item names are present and green: a lane with
`./tests/eval/...` removed is rejected, a lane accepting a zero count is rejected, and a gate
emitting its marker only on pass is rejected.
