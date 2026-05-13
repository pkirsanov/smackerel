# User Validation: BUG-004-H1

**Bug:** Silent query-error swallowing in WeeklySynthesis subqueries
**Status:** Fixed and Verified
**Date:** 2026-05-12

---

## Acceptance Checklist

- [x] **Operator visibility on subquery failure.** When any of the six optional subqueries inside `GenerateWeeklySynthesis` or `detectCapturePatterns` fails at the `Pool.Query` step, a `slog.Warn` line is emitted naming the section. Verified by inspection of the patched file (`grep -n "slog.Warn" internal/intelligence/synthesis.go` shows 12 sites = 6 pre-existing + 6 new) and by the structural regression test that fails if any `if err == nil {` swallowing site reappears.
- [x] **No functional regression.** The weekly synthesis still tolerates partial subquery failure and returns surviving sections. Word-cap, quiet-week, and assemble-text behavior unchanged. All previously green tests in `internal/intelligence` continue to pass.
- [x] **Fail-fast contract on nil pool preserved.** `TestGenerateWeeklySynthesis_NilPool` and `TestDetectCapturePatterns_NilPool` (both pre-existing) continue to PASS, confirming the upfront nil-pool guard still returns the documented error rather than panicking.
- [x] **Regression test cannot be bypassed by re-introducing the bug.** Adversarially validated: reverting one of the six call sites caused `TestSynthesisFile_NoSilentQueryErrorSwallowing` to fail with the exact line number and remediation hint, proving the test is not tautological. After restoring the fix, the test passes again.
- [x] **No collateral damage.** `go build ./...` is clean across the whole tree. `go vet ./internal/intelligence/...` is clean.
- [x] **Bug packet self-contained.** All edits to source code are limited to `internal/intelligence/synthesis.go` and `internal/intelligence/synthesis_test.go`. All artifact edits are limited to the BUG-004-H1 packet. The parent spec 004 certification fields were not modified.

## Verdict

**Closed.** The hardening trigger surfaced a real operator-visibility gap, the gap was patched at all six sites, the patch is structurally enforced by an adversarial regression test, and no behavior visible to end-users changed.
