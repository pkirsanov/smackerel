# Bug Fix Design: BUG-031-004

## Root Cause Analysis

### Investigation Summary
The 2026-04-27 workflow context reports that an outer timeout returned exit 143 for `./smackerel.sh test e2e` while child shell E2E work continued. Source inspection shows the E2E path starts a Go E2E container via `docker run`, shell E2E groups through `tests/e2e/run_all.sh`, and an `e2e_cleanup` trap inside the `test e2e` case.

### Root Cause
Unconfirmed at packetization time. The likely issue is signal/process-group handling: the parent shell exits under timeout before all child runners are signaled and reaped, or cleanup traps do not cover every child execution path.

### Impact Analysis
- Affected components: `./smackerel.sh test e2e`, E2E shell runners, Docker test stack lifecycle.
- Affected data: disposable test stack resources; persistent dev state must remain protected.
- Affected users: developers and workflow agents running broad E2E validation.

## Fix Design

### Solution Approach
The DevOps owner should reproduce with a controlled timeout and inspect the process tree. Implement process-group aware signal forwarding and cleanup so timeout/interruption terminates children and tears down only the disposable test stack through the repo CLI. Add a regression that proves no E2E child process survives after a controlled parent timeout.

### Alternative Approaches Considered
1. Increase the E2E timeout. Rejected because it does not solve leaked child work.
2. Use global Docker prune after failures. Rejected because Docker lifecycle policy requires project-scoped cleanup and persistent-store protection.

## Affected Files
- `smackerel.sh`
- `scripts/runtime/go-e2e.sh`
- Potential E2E runner scripts under `tests/e2e/` if signal forwarding must be coordinated
- DevOps regression test location selected by the owner

## Regression Test Design
- DevOps regression: controlled timeout against a harness fixture proves child processes are terminated.
- Lifecycle regression: disposable test stack cleanup executes after timeout/interruption.
- Broad E2E validation: `./smackerel.sh test e2e` no longer requires manual cleanup after timeout.

## Ownership
- Owning feature/spec: `specs/031-live-stack-testing`
- Primary fix owner: `bubbles.devops`
- Test owner: `bubbles.test`
- Validation owner: `bubbles.validate`
