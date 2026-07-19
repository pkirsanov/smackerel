# Bug: BUG-031-009 Dockerized Go E2E runner survives parent interruption

## Summary

The E2E interruption reaper tracks host descendants but does not identify or stop the Docker container that runs Go E2E packages. Parent teardown can therefore remove core and its network while the test container continues, creating broad cascade failures.

## Severity

- [ ] Critical - System unusable or data loss
- [x] High - Interrupted validation produces misleading cross-package failures and leaked test work
- [ ] Medium - Feature broken with a reliable workaround
- [ ] Low - Minor issue

## Status

- [x] Reported
- [x] Confirmed
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Start a nested focused Go E2E invocation through `./smackerel.sh test e2e --go-run ...`.
2. Wait until its `golang:1.25.10-bookworm` runner container is attached to `smackerel-test_default` and executing a Drive test.
3. Send `TERM` to the parent E2E command.
4. Observe the trap reap the host Docker CLI and begin project-stack teardown.
5. Check whether the runner container remains active while core/network services are removed.
6. Observe any subsequent test output report core health or Docker DNS failures rather than stopping at the interruption boundary.

## Expected Behavior

The parent E2E runner gives every Dockerized test child a unique run identity and force-removes that exact container before project-stack teardown. No test output occurs against a stack that the parent is dismantling.

## Actual Behavior

`SMACKEREL_E2E_CHILD_RUN_ID` is assigned only to the host command environment. The `docker run` call has no name/label carrying that ID, and `e2e_stop_child` only kills host PIDs. Docker daemon ownership lets the test container outlive the Docker CLI.

## Environment

- Surface: `smackerel.sh test e2e` parent lifecycle
- Source baseline: `a6d2fb3ffd03e7b09e294f2cdac14816fb2f5d4f`
- Trigger: interrupted/timed-out Dockerized Go E2E execution
- Platform: Linux Docker runtime

## Error Output

```text
drive_observability_e2e_test.go:48: e2e: services not healthy after 2m0s at http://smackerel-core:8080
drive_policy_e2e_test.go:38: e2e: services not healthy after 30s at http://smackerel-core:8080
FAIL github.com/smackerel/smackerel/tests/e2e/drive 300.054s
spec_076_migrations_e2e_test.go:59: lookup postgres on 127.0.0.11:53: no such host
```

This inherited cascade signature is routing provenance. The controlled current-session interruption RED is recorded in `report.md` before implementation.

## Root Cause

The process cleanup added by BUG-031-004 cannot cross the Docker daemon ownership boundary. A container started by a descendant Docker CLI is not a host descendant of that CLI, and the run ID is not applied as a Docker label or container environment. The cleanup trap has no stable identifier with which to stop it before Compose teardown.

The fix injects the generated run ID as `com.smackerel.e2e-child-run-id` on every Docker child launched through `e2e_run_child`, force-removes exact-label containers before host-process and Compose cleanup, and preserves differently labeled canary containers.

## Related

- Owning feature: `specs/031-live-stack-testing/`
- Predecessor: `../BUG-031-004-e2e-timeout-process-cleanup/`
- Origin packet: `specs/038-cloud-drives-integration/bugs/BUG-038-003-drive-e2e-core-health-collapse/`
- Parent synthesis evidence: `specs/026-domain-extraction/bugs/BUG-026-008-synthesis-schema-repair/report.md#independent-broad-findings-routed-out-of-packet`
