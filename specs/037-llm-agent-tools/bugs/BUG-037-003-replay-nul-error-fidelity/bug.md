# Bug: [BUG-037-003] Replay NUL Error Fidelity

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Summary

The NUL-bearing replay CLI case treats every non-`*exec.ExitError` as proof that the operating system rejected the argument before process execution. Unrelated launch failures, including a missing binary or canceled context, can satisfy that predicate and make the test pass for the wrong reason.

## Severity

- [ ] Critical - System unusable or data loss
- [ ] High - Major behavior broken with no safe workaround
- [x] Medium - A chaos regression can false-pass on an unrelated launch failure
- [ ] Low - Minor or cosmetic issue

## Status

- [x] Reported
- [ ] Confirmed by an executed false-pass reproduction
- [x] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Read `TestChaos_037_ReplayCLIErrorPaths` in `tests/stress/agent/chaos_round1_test.go`.
2. Locate the `null_byte_in_trace_id` case.
3. Observe that the branch requires only a non-nil error that is not `*exec.ExitError`.
4. Observe that the branch does not assert `errors.Is(runErr, syscall.EINVAL)`.
5. Observe that the branch does not assert that `binPath` exists before execution.
6. Observe that the branch does not assert `ctx.Err() == nil` after execution.

## Expected Behavior

The NUL-bearing case must pass only when the real built binary exists, the command context remains active, and the returned error wraps `syscall.EINVAL`. An unrelated launch error must fail the test.

## Actual Behavior

Any non-`*exec.ExitError` passes the branch. A missing executable, permission error, canceled context, or another pre-exec failure can be misclassified as the expected NUL rejection.

## Environment

- Repository: `smackerel`
- Owning feature: `specs/037-llm-agent-tools`
- Source revision inspected: `7ce32d703a17f3cb5a2481a1ebc0cc084a1fac61`
- Platform: Linux
- Finding date: 2026-09-02

## Error Output

No runtime error output was captured. This packet records a source-grounded test-oracle finding. The operator prohibited Docker execution during packet creation.

## Root Cause

The test classifies errors by excluding one Go error type instead of requiring the expected OS error identity. Negative type checks define an open set. Every unrelated non-exit launch failure therefore qualifies.

## Related

- Feature: `specs/037-llm-agent-tools/`
- Test: `tests/stress/agent/chaos_round1_test.go::TestChaos_037_ReplayCLIErrorPaths`
- Product contract: replay CLI returns PASS 0, FAIL 1, and ERROR 2 after execution. The NUL case must fail before execution.

## Current Invocation Boundary

This invocation creates artifacts only. It does not modify source or tests, run Docker, execute regression tests, or claim that the bug is fixed.