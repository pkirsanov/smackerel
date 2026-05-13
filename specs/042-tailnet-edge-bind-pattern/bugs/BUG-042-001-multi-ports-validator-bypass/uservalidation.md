# User Validation: BUG-042-001 — Compose contract validator multi-ports bypass

## Checklist

- [x] **The defect (multi-ports bypass) is real.** Empirically reproduced via out-of-tree chaos probe at `/tmp/smackerel-chaos-round9/main.go`. The unmodified validator silently accepted a fixture where `core.ports[1]="0.0.0.0:8443:8080"` was paired with a compliant `core.ports[0]`.
- [x] **The fix is the minimum viable change.** Two `[0]`-indexed checks replaced with `for i, p := range ...` loops. No other files modified outside of the bug spec/design/scopes/report.
- [x] **The live `deploy/compose.deploy.yml` still PASSES the contract test.** `TestComposeContract_LiveFile` continues to PASS — confirms zero regression on the real deploy file.
- [x] **The two new adversarial tests are non-tautological.** Red-then-green proof recorded in [report.md](report.md#adversarial-red-then-green-proof): with the validator reverted to `[0]`-only form, both new tests FAIL with the explicit "BUG-042-001 reintroduced" message; with the fix restored, both tests PASS.
- [x] **The fix preserves the existing 3 contract tests.** `TestComposeContract_LiveFile` + `TestComposeContract_AdversarialLiteralBind` + `TestComposeContract_AdversarialInfraHasPorts` all still PASS.
- [x] **No cross-package regression.** Smoke test of `internal/deploy/...` + `internal/config/...` + `internal/api/...` all PASS.
- [x] **No flake.** 15/15 across 3 iterations of 5 sub-tests in 0.014s.
- [x] **No production code, compose, config, or doc files modified.** `git diff --stat HEAD -- ':!specs/'` shows only `internal/deploy/compose_contract_test.go` (1 file, +76/-4).
- [x] **Severity classification is correct.** MEDIUM — no live exposure today (the live compose is single-port and compliant), but the validator gap means a future regression would be silently accepted.
- [x] **Discovery context is accurate.** Stochastic-quality-sweep round 9 of 20, seed `20520512`, trigger=`chaos`, mapped child mode=`chaos-hardening`, executionModel=`parent-expanded-child-mode` (nested `runSubagent` unavailable in the chaos-hardening sub-agent runtime).
- [x] **Spec 042 parent is unaffected.** Parent spec status remains `done`. The bug fixes a test-integrity gap discovered AFTER the parent spec's bugfix-fastlane chain completed; it does not invalidate any of the 6 SCN-042-* scenarios — instead, it strengthens the protection of SCN-042-001 ("Backend ports use the configurable bind address").

## Sign-off

- **Bug:** BUG-042-001
- **Workflow mode:** chaos-hardening
- **Parent workflow:** stochastic-quality-sweep round 9 of 20 (seed 20520512)
- **Status:** Fixed and validated
- **Decision:** SHIP_IT
