# Bug: BUG-031-004 E2E timeout leaves child processes running

## Summary
The E2E harness can return from `timeout 1800 ./smackerel.sh test e2e` with exit 143 while child shell/E2E work continues, requiring manual interruption and stack cleanup.

## Severity
- [ ] Critical - System unusable, data loss
- [x] High - E2E lifecycle can leak running work and block reliable validation
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Run the broad E2E suite through the repo CLI under an outer timeout.
2. Allow the harness to exceed the timeout while child E2E work is active.
3. Observe the parent command return 143.
4. Observe child shell/test processes or test-stack resources continuing after the parent timeout returns.
5. Manual interruption and/or `./smackerel.sh down` is required to clean up.

## Expected Behavior
When the E2E command times out or is interrupted, all child processes and the disposable test stack should be terminated through the repo lifecycle cleanup path.

## Actual Behavior
The parent timeout can return while child work continues, leaving cleanup to a manual operator action.

## Environment
- Service: `./smackerel.sh test e2e`, Docker-backed disposable test stack, Go E2E container, shell E2E runners
- Version: Workspace state on 2026-04-27 during 039 full-delivery e2e stabilization
- Platform: Linux

## Error Output
```text
Workflow context from bubbles.stabilize: E2E harness timeout/process-group cleanup issue.
Observed signature: child shell e2e continued after timeout 1800 ./smackerel.sh test e2e returned 143; cleanup required manual interruption/down.
Target likely specs/031-live-stack-testing / DevOps.
```

## Root Cause
The E2E cleanup trap terminated the active child with `TERM` and then waited. A child process in the E2E process group that ignored `TERM`, `INT`, and `HUP` could survive the parent interruption path. The regression reproduced that behavior with `tests/e2e/fixtures/test_timeout_child_fixture.sh`: the nested runner cleanup path ran, but the marker process stayed alive until manual cleanup.

## Resolution
- `smackerel.sh` now supports `./smackerel.sh test e2e --shell-run <path>` for targeted shell E2E execution through the repo CLI.
- `smackerel.sh` now sends `KILL` to the child process group after a short `TERM` grace during E2E cleanup, including the case where the process-group leader has exited but group members still exist.
- `tests/e2e/test_timeout_process_cleanup.sh` invokes the adversarial fixture through the E2E runner, verifies the survivor detector fails when a marker child exists, interrupts the nested runner, and asserts marker processes are absent after cleanup.
- `tests/e2e/run_all.sh` and the main E2E lifecycle script list include the new lifecycle regression.

## Verification
- `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` exited 0 after the fix and reported both BUG-031-004 scenarios passing.
- `./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist` exited 0 and completed project-scoped stack teardown.
- `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/test_timeout_process_cleanup.sh` exited 0.
- `./smackerel.sh check` exited 0.

## Related
- Feature: `specs/031-live-stack-testing/`
- Runtime entrypoint: `./smackerel.sh test e2e`
- E2E runner: `scripts/runtime/go-e2e.sh`
- Existing related but non-covering bug: `BUG-031-001-integration-stack-volume-and-migration-hang`
