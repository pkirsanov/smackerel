# Bug: [BUG-061-011] Assistant acceptance gate executes in no automated lane

## Summary
`tests/eval/assistant/acceptance_test.go` carries `//go:build integration`, and the integration lane's package list omits `./tests/eval/...`, so the routing-accuracy and capture-fallback acceptance gate runs in **no** automated lane.

## Severity
- [x] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

**Severity rationale.** The rating is not about a crash. It is inherited from the delivery plan (`docs/Product_Delivery_Plan.md` § P3, `D27` · **Critical** · **Stage 1**) and from what the gate protects: `config/smackerel.yaml` calls the thresholds a non-negotiable acceptance contract, and the assistant can measurably degrade against them while CI stays green. A quality gate that never executes is indistinguishable from no gate at all — but it *reads* as protection, so nobody looks.

## Status
- [x] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

**Fixed as of the working tree at HEAD `3af96a02`** (the fix is implemented but uncommitted). The gate now executes in the automated integration lane: a full `./smackerel.sh test integration` run compiled and executed `./tests/eval/assistant`, the gate emitted exactly one `ASSISTANT_ACCEPTANCE_GATE_V1` line reporting `executed_assertions=210`, and the lane's own enforced assertion accepted it. Raw evidence is in [`scopes.md`](scopes.md) DoD item **A9** and [`report.md`](report.md) → *After Fix — Verification*.

**Two things `Fixed` deliberately does not assert.** First, the integration lane is **not green** — it exited `1` on a single unrelated failure, `TestOpenKnowledgeRouting_FallbackToOpenKnowledge`, filed separately as `specs/064-open-ended-knowledge-agent/bugs/BUG-064-003-router-warmup-exceeds-fixed-deadline/`. The Stage 1 exit criterion therefore remains blocked on that bug, not on this one. Second, the R10-3 prose in `tests/e2e/assistant_regression_e2e_test.sh` (false claim 3 below) is still uncorrected; its outcome clause became true when the fix landed, but its causal clause — that the build tag is what makes the gate run — remains false.

`Verified` is not set. That transition is owned by validation, not by the implementing or testing agent.

## Reproduction Steps

All four steps were executed in the filing session against commit `6ad1e8c98ce6e1aa35df779306c6a8835db172be`. Every file named below is **identical at HEAD and in the working tree** (`git status --porcelain` on the four paths returned empty), so the finding holds for both trees.

1. Confirm the acceptance gate is tag-gated out of the untagged unit lane:
   ```
   sed -n '1,20p' tests/eval/assistant/acceptance_test.go
   ```
   Line 1 is `//go:build integration`.
2. Confirm the integration lane's package list:
   ```
   sed -n '1,55p' scripts/runtime/go-integration.sh
   ```
   Line 53 is
   `go_test_args+=(./tests/integration/... ./internal/notification/... ./internal/assistant/... ./internal/cardrewards/...)`.
3. Confirm the eval tree is absent from that lane entirely:
   ```
   grep -nE 'tests/eval|eval' scripts/runtime/go-integration.sh
   ```
   Exit code `1`, zero matching lines.
4. Confirm no other script executes the gate:
   ```
   grep -rn "TestAcceptanceGate" --include='*.go' --include='*.sh' --include='*.yaml' --include='*.yml' --include='*.md' .
   ```
   Every hit outside `acceptance_test.go` itself is prose: documentation, spec `report.md` transcripts, or `echo` lines. Nothing invokes it.

Raw output for all four steps is in [`report.md`](report.md) → *Before Fix — Reproduction*.

## Expected Behavior
Running the repository's automated integration lane executes `TestAcceptanceGate_RoutingAccuracyAndCaptureFallback` against the shipped 150-row corpus, and the lane fails loudly if the gate did not actually run or evaluated zero assertions. See [`spec.md`](spec.md).

## Actual Behavior
The gate executes in no lane:

| Lane | Command | Tags | Packages | Gate runs? |
|------|---------|------|----------|-----------|
| Unit (`ci.yml:92`) | `./smackerel.sh test unit --go` → `scripts/runtime/go-unit.sh:67` | none | `./...` | **No** — `//go:build integration` excludes the file |
| Integration (`ci.yml:241`) | `./smackerel.sh test integration` → `scripts/runtime/go-integration.sh:53` | `integration` | 4 explicit trees, `./tests/eval/...` absent | **No** — the package is never compiled |
| E2E | `tests/e2e/assistant_regression_e2e_test.sh` | n/a | n/a | **No** — lines 251–253 only `echo` the gate's name |
| Stress | `scripts/runtime/go-stress.sh:50` | `stress` | `./tests/stress/readiness` | **No** |

Only the untagged neighbours in the same package (`corpus_validation_test.go`, `harness_test.go`) run, via the unit lane's `go test ./...`. Those assert corpus *structure* and harness *primitives*. Neither asserts the accuracy thresholds. The gate itself is the only thing that does, and it is the only thing excluded.

## Three compounding false claims

This defect is unusually well camouflaged, because three separate surfaces assert the wiring exists:

1. **`tests/eval/assistant/acceptance_test.go:15-17`** — its own header comment: *"CI invokes `./smackerel.sh test integration` which sets `-tags integration` and the gate then runs."* The first clause is true (`ci.yml:241`); the conclusion is false, because the lane's package list excludes the package.
2. **`docs/Testing.md:748`** — presents `./smackerel.sh test integration --go-run TestAcceptanceGate_RoutingAccuracyAndCaptureFallback` as *"The standard invocation"*. `--go-run` reaches `go-integration.sh` as `-run`, which filters *within* the four listed package trees. The gate's package is not among them, so the selector can match nothing. `docs/Testing.md:755` gives a *direct* `go test ... ./tests/eval/assistant/...` form that names the package explicitly — that one does work, and is the only invocation in the repository that does.
3. **`tests/e2e/assistant_regression_e2e_test.sh:251-253`** — the R10-3 block states the gate *"runs as part of `./smackerel.sh test integration` not `unit`"* and cites the test by name. The script is `echo` prose end to end for that item; `grep -nE 'go test|tests/eval|assistanteval|TestAcceptanceGate'` against it matches only `echo` lines.

Spec 061's own `report.md:6839` repeats claim 2 as the gate's "primary path". The gate's historical PASS evidence (`report.md:5399`, `:5705`) was captured with the **direct** `go test ./tests/eval/assistant/...` form — a real run, but a manual one.

## Environment
- Service: repository test tooling — `scripts/runtime/go-integration.sh`, `tests/eval/assistant/`
- Version: commit `6ad1e8c98ce6e1aa35df779306c6a8835db172be` (HEAD at filing); the four implicated files are unmodified in the working tree
- Platform: Linux; lanes execute in `golang:1.25.10-bookworm` per `smackerel.sh:1206`

## Error Output

There is no error output, and that is the defect. The failure mode is a **silent green**. The closest thing to a diagnostic is the absence itself:

```
$ grep -nE 'tests/eval|eval' scripts/runtime/go-integration.sh
GREP_EXIT=1
```

## Root Cause (filled after analysis)

A package-selection list and a build tag were introduced by the same scope with complementary intent, and neither one references the other. `acceptance_test.go` opted *out* of the untagged lane on the stated assumption that the tagged lane would pick it up; the tagged lane enumerates packages explicitly and was never extended to include it. Nothing in the repository asserts the relationship, so the assumption printed in the file's own header became stale without any surface turning red.

Full analysis, including why adding the package to the list is only half a fix, is in [`design.md`](design.md).

## Impact

- **Stage 1 cannot be declared complete.** `docs/Product_Delivery_Plan.md` § *Stage 1 — Stop leaking and stop losing* (Done-when block, lines 662–678) requires the three listed commands green **and** *"the integration run reports the assistant acceptance gate with a non-zero executed-assertion count"*. No lane reports that count today — the count is not emitted in any machine-readable form at all. This criterion is unmet regardless of what else in Stage 1 lands.
- **Silent quality drift.** Routing accuracy and capture-fallback rate can fall below the SST floors (`assistant.eval.routing_accuracy_min: 0.85`, `assistant.eval.capture_fallback_min: 1.0` — `config/smackerel.yaml:1351-1353`) between manual runs with no signal.
- **Scope of the claim — do not overstate.** This gate measures **assistant routing quality** on an offline corpus using the harness's deterministic keyword classifier. It does **not** measure corpus-grant enforcement. The grant-enforcement holes tracked as D25/D28 under spec 108 are a separate axis; fixing this bug does not make those measurable and must not be reported as doing so. The harness is also explicitly a structural proxy for the production router, not a live-LLM quality measurement (`tests/eval/assistant/harness.go:1-14`).

## Related
- Feature: `specs/061-conversational-assistant/` (SCOPE-10 shipped the corpus, harness, and gate)
- Delivery plan: `docs/Product_Delivery_Plan.md` § P3 (`D27`, Critical, Stage 1)
- Product direction: `docs/Product_Direction_2026-07-31.md:230` (`D27` row)
- Runbook that documents the gate: `docs/Testing.md` § *Acceptance Gate* / *How To Run*
- Precedent for the enforcement shape: `internal/deploy/assistant_e2e_package_contract_test.go`, `internal/deploy/envsubst_wrapper_contract_test.go`, `internal/deploy/ci_integration_topology_contract_test.go`
