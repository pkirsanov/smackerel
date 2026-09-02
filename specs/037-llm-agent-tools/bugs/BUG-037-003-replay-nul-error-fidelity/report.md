# Report: [BUG-037-003] Replay NUL Error Fidelity

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

This artifact-only invocation recorded a test-oracle defect under its owning spec. It did not modify production source, tests, Docker, configuration, or deployment files.

## Completion Statement

The bug is not fixed or certified. The packet remains `in_progress` until the owning design, plan, test, validation, and audit phases complete.

## Bug Reproduction - Before Fix

**Phase:** bug
**Command:** not run
**Exit Code:** not applicable
**Claim Source:** not-run

The source inspection found an open-set error predicate. No regression command ran because the operator required artifact-only work and prohibited Docker.

## Source-Grounded Findings

**Claim Source:** interpreted

| Source | Observed fact | Finding |
|---|---|---|
| `tests/stress/agent/chaos_round1_test.go::TestChaos_037_ReplayCLIErrorPaths` | The NUL branch accepts any non-nil error except `*exec.ExitError` | Unrelated pre-exec launch errors can pass |
| Same function | The binary build is checked, but the built path is not statted before subtests | A later missing-path condition is not excluded by the NUL oracle |
| Same function | The branch does not inspect `ctx.Err()` | Cancellation can be mistaken for NUL rejection |
| Same function | The branch does not call `errors.Is` for `syscall.EINVAL` | The expected OS cause is never proven |

## Code Diff Evidence

No implementation diff is claimed. Only files inside this new bug directory are created by this invocation.

## Test Evidence

**Phase:** test
**Command:** none
**Exit Code:** not applicable
**Claim Source:** not-run

No test result is claimed.

## Uncertainty Declaration

- **What was attempted:** Source and artifact inspection only.
- **What was observed:** The current branch accepts an open class of non-exit errors.
- **Why this is uncertain:** No missing-binary mutation or focused stress command ran in this artifact-only invocation.
- **What would resolve this:** Execute TP-037-003-01 before the repair, then TP-037-003-02 with the same mutation after the repair.

## Planned Evidence Anchors

- `tp-037-003-01-pre-fix-mutation-survival`
- `tp-037-003-02-focused-einval-identity`
- `tp-037-003-03-replay-error-companion`
- `tp-037-003-04-broader-e2e`
- `tp-037-003-05-broader-stress`
- `tp-037-003-06-regression-quality`

## Validation Evidence

**Executed:** NO
**Command:** none
**Phase Agent:** `bubbles.validate`
**Claim Source:** not-run

No completion validation or certification occurred.

## Audit Evidence

**Executed:** NO
**Command:** none
**Phase Agent:** `bubbles.audit`
**Claim Source:** not-run

No audit verdict is claimed.

## Chaos Evidence

**Executed:** NO
**Command:** none
**Phase Agent:** `bubbles.chaos`
**Claim Source:** not-run

No chaos result is claimed. The planned adversarial case belongs to the test phase.

## Test Implementation - Static Verification

### Implemented Test Surface

**Phase:** test
**Claim Source:** interpreted
**Interpretation:** The scoped test now checks a regular executable immediately before each command run, requires an active context and an error wrapping `syscall.EINVAL` for the NUL case, and contains a standalone adversarial oracle test for unrelated launch results.

Only `tests/stress/agent/chaos_round1_test.go` was edited. No production source, Docker, configuration, database, deployment, or UI file was changed by this test invocation.

### Editor Diagnostics

**Phase:** test
**Command:** VS Code `get_errors` on `tests/stress/agent/chaos_round1_test.go`
**Exit Code:** 0
**Claim Source:** executed

```text
<errors path="tests/stress/agent/chaos_round1_test.go">
No errors found
</errors>
```

### Static Contract Evidence

**Phase:** test
**Command:** `git diff --check`, required-oracle grep, adversarial-control grep, and scoped `git status` for the allowed test and bug packet
**Exit Code:** 0
**Claim Source:** executed

```text
STATIC CHECK: scoped diff whitespace
git_diff_check_exit=0
STATIC CHECK: required oracle and preconditions
405:func validateReplayExecutable(binPath string) error {
419:func validateReplayNULLaunchError(runErr error) error {
427:    if !errors.Is(runErr, syscall.EINVAL) {
540:                    if err := validateReplayExecutable(binPath); err != nil {
545:                            if ctxErr := ctx.Err(); ctxErr != nil {
required_oracle_scan_exit=0
STATIC CHECK: adversarial launch-result controls
443:            {name: "successful_launch", runErr: nil},
444:            {name: "missing_binary", runErr: &os.PathError{Op: "fork/exec", Path: missingBinPath, Err: syscall.ENOENT}},
445:            {name: "permission_denied", runErr: &os.PathError{Op: "fork/exec", Path: missingBinPath, Err: syscall.EACCES}},
446:            {name: "canceled_context", runErr: context.Canceled},
447:            {name: "expired_context", runErr: context.DeadlineExceeded},
448:            {name: "process_exit", runErr: &exec.ExitError{}},
458:    wrappedEINVAL := &os.PathError{Op: "fork/exec", Path: "smackerel-chaos-cli", Err: syscall.EINVAL}
459:    if err := validateReplayNULLaunchError(wrappedEINVAL); err != nil {
adversarial_control_scan_exit=0
STATIC CHECK: scoped working-tree paths
 M tests/stress/agent/chaos_round1_test.go
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/bug.md
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/design.md
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/report.md
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/scenario-manifest.json
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/scopes.md
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/spec.md
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/state.json
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/test-plan.json
?? specs/037-llm-agent-tools/bugs/BUG-037-003-replay-nul-error-fidelity/uservalidation.md
scoped_status_exit=0
```

### Runtime Verification Boundary

**Phase:** test
**Command:** not run
**Exit Code:** not applicable
**Claim Source:** not-run

Per operator instruction, this invocation did not run Docker, the focused stress test, the broader stress lane, or the broader E2E lane. No runtime pass claim is made, no Definition of Done item is checked, and the packet remains `in_progress`.

### Uncertainty Declaration - Runtime Behavior

- **What was attempted:** Editor diagnostics plus static source-contract and diff checks.
- **What was observed:** The edited file has no editor diagnostics, the scoped diff has no whitespace errors, and the required predicates and adversarial controls are present.
- **Why this is uncertain:** Static checks do not execute the Go test, build the replay binary, or observe the operating system returning `EINVAL` for the NUL-bearing argument.
- **What would resolve this:** Execute the packet's focused and broader repository-standard test commands when runtime execution is authorized.