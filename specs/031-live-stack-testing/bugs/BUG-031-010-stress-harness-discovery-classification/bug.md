# Bug: BUG-031-010 Stress Harness Discovery and Classification Fail-Open

## Summary

The Go stress harness can turn test discovery or output-classification failures into benign selector results, and its warning classifier does not distinguish structured JSON warnings from quoted example text.

## Severity

- [ ] Critical - System unusable or data loss
- [x] High - A required stress gate can report an untrustworthy result without a reliable workaround
- [ ] Medium - Feature broken with a reliable workaround
- [ ] Low - Minor or cosmetic issue

## Status

- [ ] Reported
- [ ] Confirmed by an executed red-stage regression
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Discovery Context

Static inspection of the current dirty working tree identified three related defects in the same harness contract:

1. `go_stress_package_has_selected_tests` reads `go test -list` through process substitution. The producer status is not the `while` loop status, and the caller treats every non-zero helper result as a selector miss.
2. `go_stress_classify_output` recognizes logfmt and timestamp warning lines but not JSON warning records such as `{"level":"WARN"}`. Its unanchored token searches can also match warning-shaped text inside a quoted benign message.
3. `tests/stress/readiness/canary_test.go` does not yet inject list, grep, or capture failures that prove fail-loud status propagation and temporary-file cleanup.

**Claim Source:** interpreted from current source and test files. No runtime reproduction is claimed in this packet.

## Reproduction Steps

These steps define the required red stage for the implementation owner:

1. Extend the fake `go` executable in `tests/stress/readiness/canary_test.go` so the readiness canary succeeds and the workload `go test -list` invocation prints a compile/discovery diagnostic and exits with a distinctive non-zero status.
2. Execute the harness through the repository unit-test command and assert that the harness exits with the discovery status instead of printing a selector miss or zero-package summary.
3. Feed a direct JSON warning record to the output classifier and assert that the harness rejects it.
4. Feed quoted warning-shaped example text to the output classifier and assert that the harness accepts it as benign.
5. Put fake `grep` and `tee` executables first on `PATH`, make each fail independently, and assert a non-zero harness result with an actionable diagnostic.
6. After each failure path, assert that the test-owned capture directory is empty.

## Expected Behavior

- A failed `go test -list` invocation terminates the harness and preserves its exit status and diagnostic.
- A selector miss remains a normal per-package skip and is not confused with discovery failure.
- Direct JSON warning records are rejected.
- Warning-shaped text inside a quoted benign message does not trigger the runtime-warning classifier.
- Classifier-command and output-capture failures terminate the harness because the classification result is not trustworthy.
- Every allocated capture file is removed on success and failure.

## Actual Behavior

- The process-substitution producer status is not checked, and the caller converts every non-zero helper result into `continue`.
- JSON warning records fall through to `clean`.
- Unanchored warning patterns can classify quoted example text as a runtime warning.
- A `grep` error follows the same branch as a normal no-match and can ultimately return `clean`.
- Existing tests prove workload status propagation and selected output classes, but not the five adversarial conditions above as one cleanup-preserving contract.

## Environment

- Repository: Smackerel
- Parent feature: `specs/031-live-stack-testing`
- Release train: `mvp`
- Platform: Linux
- Working-tree condition: `scripts/runtime/go-stress.sh` and `tests/stress/readiness/canary_test.go` already contain user-owned dirty changes

## Error Output

No runtime error output was captured. The planning invocation intentionally ran no project tests and no Docker command because an E2E lane was active.

## Root Cause

The harness uses binary shell statuses for contracts that require three distinct outcomes. Process substitution hides the discovery producer status from the consuming loop, the caller treats every helper failure as a benign selector miss, and classifier conditionals treat both grep no-match and grep execution failure as false. The warning regexes are also token-oriented rather than line-format-oriented, so they neither cover JSON records nor exclude quoted examples reliably.

## Related

- Parent feature: `specs/031-live-stack-testing/`
- Terminal predecessor: `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/` (`done`, read-only reference)
- Terminal predecessor: `specs/031-live-stack-testing/bugs/BUG-031-006-strict-guard-gate-drift/` (`done`, read-only reference)
- Controlling script: `scripts/runtime/go-stress.sh`
- Required adversarial test surface: `tests/stress/readiness/canary_test.go`

## Change Boundary

Implementation is limited to `scripts/runtime/go-stress.sh`, `tests/stress/readiness/canary_test.go`, and this bug packet. BUG-031-005, BUG-031-006, all other source and test files, configuration, generated files, and framework-managed files are excluded.