# Design: BUG-031-007 R8 sweep test trigger artifact fidelity drift

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Current Truth (Objective Research Pass)

- `scripts/runtime/go-integration.sh` is 17 lines, no `function` declarations. Only top-level shell statements: `set -euo pipefail`, source `_ensure_envsubst.sh`, `ensure_envsubst "go-integration"`, `cd /workspace`, `go test -p 1 -tags integration ...`. Confirmed via `wc -l` and `grep -nE "^(function )?[a-z_]+\(\)"`.
- `smackerel.sh` invokes the script directly as a bash entry: `bash /workspace/scripts/runtime/go-integration.sh`. There is no dispatcher function named `test_integration` anywhere under `scripts/` (verified by `grep -rEn "test_integration|case integration" scripts/`).
- `tests/integration/helpers_test.go` defines `testPool`, `testNATSConn`, `testJetStream`, `testID`, `cleanupArtifact`, `cleanupList`, `cleanupAnnotation` — all 7 referenced functions exist.
- `tests/stress/ml_readiness_timeout_stress_test.go` is the new SLA stress test added by BUG-031-006 (file sha256 `50c589f3563f6cb75be286a627e59ab532ae84b684d743213a0288ef211bc292` per round 3 `bubbles.test` executionHistory). It defines exactly three top-level test functions:
  - `TestMLReadinessTimeoutBoundary` (line 17)
  - `TestMLReadinessTimeoutSilentBypass` (line 164)
  - `TestMLReadinessAlways200Regression` (line 241)
- Parent `specs/031-live-stack-testing/state.json` was updated in round 3 by `bubbles.validate` (executionHistory entry at `2026-05-23T09:00:00Z`) which textually claimed `BUG-031-006 moved from activeBugs to resolvedBugs`. The textual claim is contradicted by the actual field contents: `activeBugs: ["BUG-031-006-strict-guard-gate-drift"]`, `resolvedBugs: []`.
- BUG-031-006 own `state.json` is `status: done`, `certification.status: done` (verified by `python3 -c "import json; ..."`).
- `state-transition-guard.sh specs/031-live-stack-testing` exits with TRANSITION PERMITTED and two warnings (no completedAt timestamps, no concrete test file paths in Test Plan — both false-positive shaped against the current artifact set, since Test Plan rows clearly contain concrete file paths). No new BLOCKs are present.

## Design

### T-031-001: Remove phantom function reference

The cleanest fix is to drop the third entry from SCN-LST-005 `linkedTests` (the one pointing at `scripts/runtime/go-integration.sh` with `function: test_integration`). The remaining two entries (`testPool` + `testJetStream` in `tests/integration/helpers_test.go`) are the actual scenario-bearing implementations that prove SST-derived isolation. The integration entry-point shell wiring is already recorded as a source `evidenceRef` (`scripts/commands/test.sh`).

The alternative (rename `function` to a real symbol or invent a function inside `go-integration.sh`) would either require a code change (creating a function) or would still be misleading (no such function dispatches the integration tests — the script body itself does). Removal preserves manifest fidelity with zero production code change.

### T-031-002: Reconcile activeBugs/resolvedBugs

Edit `specs/031-live-stack-testing/state.json` to move `"BUG-031-006-strict-guard-gate-drift"` from `activeBugs` to `resolvedBugs`. Set `lastUpdatedAt` to the BUG-031-007 closure timestamp. Do not touch any certification field, executionHistory entry, or scope/phase counter. This is bookkeeping reconciliation, not certification.

### T-031-003: Link new SLA stress tests under SCN-LST-004

`SCN-LST-004` (`Search works after cold start`) is the scope-6 scenario anchor for the ML readiness gate. The new SLA stress tests are additive proof of the same gate at the SLA boundary, so the minimal change is to extend SCN-LST-004 with:

- Three additional `linkedTests` entries pointing at `tests/stress/ml_readiness_timeout_stress_test.go` with the three function names.
- One additional `evidenceRefs` entry of type `stress-test` for the same file.

This preserves a single scenario anchor for scope 6 (no SCN-LST-013 churn), keeps trace-guard one-to-one mapping intact, and surfaces the stress surface to any manifest-driven coverage report.

## Risks

- **Risk: state-transition-guard warning amplification.** Adding stress-test linkedTests may change the warning footprint. Mitigation: the existing two warnings are false-positive shaped; adding more linkedTests entries cannot introduce new BLOCKs because the guard checks structure, not count. Verified by re-running the guard after edits.
- **Risk: scenario-manifest schema drift.** `evidenceRefs` already supports `integration-test` and `e2e-test` types per the existing SCN-LST-003 / SCN-LST-004 entries. Adding `stress-test` follows the same shape; no schema migration needed.
- **Risk: Spec 055 ntfy adapter WIP contamination at commit time.** Mitigation: path-limited `git add` only on the four target paths (BUG packet + parent state.json + scenario-manifest.json), with `git diff --cached --name-status` verification before commit.
