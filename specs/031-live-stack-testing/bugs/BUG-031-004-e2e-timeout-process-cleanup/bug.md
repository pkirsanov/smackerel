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
- [ ] Confirmed (targeted red-stage output to be captured by DevOps owner)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

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

## Root Cause (initial analysis)
Root cause is not yet proven. Candidate surfaces include timeout not killing the whole process group, child bash runners not receiving forwarded termination, Docker container execution outliving the parent shell, or cleanup traps not firing consistently under timeout signal delivery.

## Related
- Feature: `specs/031-live-stack-testing/`
- Runtime entrypoint: `./smackerel.sh test e2e`
- E2E runner: `scripts/runtime/go-e2e.sh`
- Existing related but non-covering bug: `BUG-031-001-integration-stack-volume-and-migration-hang`
