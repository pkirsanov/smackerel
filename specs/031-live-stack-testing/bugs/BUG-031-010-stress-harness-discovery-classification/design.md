# Bug Fix Design: BUG-031-010 Stress Harness Discovery and Classification Fail-Open

## Root Cause Analysis

### Investigation Summary

The analysis inspected the current working-tree versions of `scripts/runtime/go-stress.sh` and `tests/stress/readiness/canary_test.go`, then compared their ownership and behavioral boundary with terminal BUG-031-005 and BUG-031-006 artifacts.

BUG-031-005 established the checked-package wrapper, readiness canary, output capture, workload failure propagation, and basic skip/warning rejection. BUG-031-006 added per-package run-selector discovery and the current fake-`go` canary coverage. Both bugs are `done`; this packet is a narrow follow-on and does not reopen either state record.

### Root Cause

There are three coupled root causes.

1. **Process substitution discards discovery status.** `go_stress_package_has_selected_tests` consumes `go test -list` with `done < <(...)`. The `while` loop status does not carry the process-substitution producer status. A compile or discovery failure can therefore leave `found_match=false` and reach the normal no-match return.
2. **The caller collapses all helper failures into skip.** The package loop uses `if ! go_stress_package_has_selected_tests; then continue; fi`. Even if the helper began returning the discovery status, this caller shape would still treat it as a benign skip and would lose the original status through `!`.
3. **Classifier predicates collapse no-match and tool failure.** Each `if grep -Eq ...; then` branch treats grep exit 1 and grep exit greater than 1 identically. If every predicate fails this way, the function prints `clean`. The patterns also search warning tokens anywhere in a line, which misses JSON key/value syntax and can match tokens embedded in quoted prose.

The existing tests do not inject these producer and classifier failures. They therefore protect successful selection, workload status, skip output, logfmt warnings, timestamp warnings, benign Go's own no-tests warning, and capture cleanup only for the paths they already exercise.

### Impact Analysis

- Affected component: `scripts/runtime/go-stress.sh`
- Affected validation surface: `tests/stress/readiness/canary_test.go`
- Affected data: none
- Affected runtime state: test-only temporary capture files
- Affected users: maintainers and delivery agents relying on the stress gate to distinguish no tests, compile failure, runtime warning, and harness failure
- Risk: a false-green or misleading failure classification can invalidate stress evidence and route a defect to the wrong owner

## Fix Design

### 1. Make Package Selection Tri-State Without Sentinel Exit Collisions

Keep discovery failure in the shell status channel and keep selector match state in an explicit variable:

- Capture `go test -list` output with an `if assignment; then ... else ... fi` form so `set -e` does not abort before the status is recorded.
- On discovery failure, stream the captured diagnostic, print one package-specific harness diagnostic, and return the exact `go test -list` status.
- On successful discovery, set an explicit selected/not-selected value and return zero for both outcomes.
- In the package loop, capture a helper failure without `!`, exit with its exact status, and use the explicit selected value to decide whether to continue.

This avoids reserving an exit code for no-match. A real `go test` failure can use any non-zero status without colliding with harness control flow.

### 2. Make Classifier Matching Status-Aware

Introduce one small classifier predicate helper that treats grep statuses as follows:

- exit 0: pattern matched
- exit 1: pattern did not match
- any other exit: classifier failure

The classifier must stop on the first execution failure and return a non-zero status or a dedicated failure class that the checked-package wrapper rejects. It must never continue to `clean` after a matcher error.

### 3. Classify Line Formats, Not Free Text

Use anchored, format-specific expressions:

- Keep the Go test skip marker anchored at line start.
- Recognize direct logfmt warning fields only in a logger-shaped line, including the existing `time=... level=WARN ...` form.
- Keep timestamp warning recognition anchored to the beginning of a logger-shaped line.
- Add a JSON warning expression for an unescaped `"level"` key whose exact string value is `WARN` in an object line.
- Do not match warning-shaped tokens that occur only inside quoted logfmt messages, quoted prose, or escaped JSON text inside a message value.

No JSON parser or new runtime package is introduced. The supported JSON contract is the direct logger record shape exercised by the regression matrix, not arbitrary recursive JSON interpretation.

### 4. Preserve Capture Failure Ordering and Cleanup

Keep classification after both the `go test` and capture-writer statuses have been checked. If capture writing fails, report that failure and return before classification. Every branch after capture allocation must call the existing cleanup helper, with the `EXIT` trap retained as the final safety net.

### 5. Add Scenario-First Adversarial Harness Tests

Extend only `tests/stress/readiness/canary_test.go`:

- Fake `go test -list` exits with a distinctive status after writing a diagnostic; assert exact harness exit propagation and absence of selector-miss language.
- Classification table includes compact JSON WARN, JSON WARN after a time field, quoted `level=WARN`, quoted timestamp-WARN, and escaped JSON WARN text.
- Fake `grep` exits with a distinctive error; assert fail-loud classification.
- Fake `tee` exits with a distinctive error; assert capture failure is visible and classification is not trusted.
- Every fixture receives its own `TMPDIR` and calls `assertGoStressCaptureDirectoryEmpty` before returning.
- Existing workload-failure and output-contract tests remain intact and pass unchanged except for additive table cases or shared fixture plumbing required by the new cases.

## Change Boundary

### Allowed

- `scripts/runtime/go-stress.sh`
- `tests/stress/readiness/canary_test.go`
- `specs/031-live-stack-testing/bugs/BUG-031-010-stress-harness-discovery-classification/**`

### Excluded

- `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/**`
- `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/**`
- All other `tests/**`, `scripts/**`, `internal/**`, `cmd/**`, `config/**`, `docs/**`, Compose files, generated files, and framework-managed `.github/bubbles/**`

## Alternative Approaches Considered

1. **Check `$?` after the existing process-substitution loop.** Rejected because that status belongs to the loop, not reliably to `go test -list`.
2. **Use non-zero helper return 1 for selector no-match and other values for failure.** Rejected because `go test` commonly returns 1 for real compile or discovery failure, so the sentinel would collide with a producer status.
3. **Treat any line containing `WARN` as a warning.** Rejected because it preserves the quoted-literal false positive and weakens diagnostic trust.
4. **Parse all JSON with a new runtime dependency.** Rejected because the bounded logger record contract does not justify changing the stress image or dependency surface.
5. **Rewrite the stress harness around another language.** Rejected because two local shell control-flow repairs and additive adversarial tests address the root cause.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|----------|------------------------|--------------|
| Explicit selection state plus exact failure status | Return 1 for no-match and another code for failure | Producer failures can legitimately return 1, so a sentinel would lose exact status integrity |
| Status-aware matcher helper | Keep independent `if grep` statements | Independent conditionals cannot distinguish grep no-match from grep execution failure consistently |

## Ownership And Routing

- Next owner: `bubbles.implement`
- Test owner after implementation: `bubbles.test`
- Validation and certification owner: `bubbles.validate`
- BUG-031-005 and BUG-031-006 remain read-only terminal references