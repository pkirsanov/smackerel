# Bug Fix Design: [BUG-037-003] Replay NUL Error Fidelity

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Ownership Routing

This is the initial root-cause packet created by `bubbles.bug`. `bubbles.design` owns design approval or revision before implementation. `bubbles.plan` owns final scope and test-plan approval.

## Root Cause Analysis

### Investigation Summary

The investigation traced the NUL case from binary build through `exec.CommandContext` and its current error branch.

- The test builds `./cmd/core` into `binPath`.
- The NUL case calls that path with an argument containing `\x00`.
- The branch rejects a nil error.
- The branch rejects `*exec.ExitError`.
- Every other error type is logged as the expected pre-exec rejection.
- The branch does not inspect the wrapped errno.
- The branch does not assert binary existence or context state.

### Root Cause

The oracle uses exclusion instead of identity. `runErr != nil` and `runErr` not being `*exec.ExitError` do not imply NUL rejection. They admit an unbounded set of unrelated launch failures.

### Impact Analysis

- Affected component: replay CLI chaos test.
- Affected production behavior: none expected.
- Affected data: none.
- User impact: a broken test environment can report a passing NUL regression without testing the intended boundary.

## Fix Design

### Solution Approach

1. Add the standard `errors` and `syscall` imports to the test file.
2. After the build succeeds, call `os.Stat(binPath)`.
3. Fail unless the path exists and identifies a regular file.
4. Run the NUL-bearing command with the existing bounded context.
5. Fail if `ctx.Err()` is non-nil after `cmd.Run`.
6. Fail unless `errors.Is(runErr, syscall.EINVAL)` is true.
7. Retain the explicit `*exec.ExitError` rejection for a clear diagnostic.
8. Keep all non-NUL cases on the existing exit-code 2 path.

### Expected Error Shape

The assertion must use `errors.Is` because Go may wrap `syscall.EINVAL` in an `*os.PathError`. Direct type equality or string matching would couple the test to wrapper details.

### Negative Controls

- A missing binary must fail at the binary precondition or fail the EINVAL assertion.
- A canceled context must fail the explicit context assertion.
- A process exit must fail the `*exec.ExitError` assertion.
- An unrelated pre-exec error must fail the EINVAL assertion.

### Mutation Proof

Before the repair, temporarily point only the NUL subtest at a guaranteed missing binary. The current open-set oracle should pass, demonstrating the false positive.

After the repair, apply the same temporary mutation. The binary assertion or EINVAL assertion must fail. Restore the test file and verify its original hash before normal execution.

### Alternative Approaches Considered

1. Match the error text for `invalid argument`. Rejected because wrapper text and localization are not stable contracts.
2. Keep only the non-`*exec.ExitError` check. Rejected because it is the open-set defect.
3. Remove the NUL case as an OS behavior test. Rejected because the case protects the harness boundary and prevents false replay-process claims.

## Testing Strategy

- Capture the pre-repair missing-binary mutant surviving the NUL subtest.
- Run the corrected NUL subtest and ordinary replay error cases.
- Run broader E2E and stress suites for collateral coverage.
- Run the bugfix regression-quality guard against the changed test file.

## Change Boundary

Allowed implementation paths:

- `tests/stress/agent/chaos_round1_test.go`
- This bug packet

Excluded paths:

- `internal/agent/replay.go`
- `cmd/core/cmd_agent.go`
- Docker, deployment, configuration, database, and UI files

## Complexity Tracking

None. The smallest viable fix adds one binary precondition and two exact error-cause assertions.

## Risks And Resolutions

- Platform wrappers may differ. Use `errors.Is` against the errno rather than a concrete wrapper type.
- A timeout may race the rejection. Assert the context remains active immediately after `cmd.Run`.
- A build failure may be mistaken for the test subject. Retain the existing fatal build check and add an explicit binary stat.