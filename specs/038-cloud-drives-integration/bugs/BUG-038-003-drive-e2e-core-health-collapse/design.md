# Bug Fix Design: BUG-038-003

## Root Cause Analysis

### Investigation Summary

No Drive E2E source directly invokes Docker teardown, core shutdown, or network removal. The existing `waitForHealth` helper retries `/api/health` every two seconds but records neither the last status/body nor whether the target became terminal. The broad evidence therefore proves disappearance but does not identify its first cause.

### Discriminating Matrix

| Run | Observation | Classification |
|---|---|---|
| Observability test alone fails and core exits | Observability fixture or concurrent infrastructure exhausts core | Inspect core exit/log/resource evidence |
| Observability alone passes; preceding cross-feature sequence fails | Prior Drive test cleanup/request path contaminates shared state | Compare DB/container state around neighbor boundary |
| Both focused runs pass; whole Drive package fails at a stable predecessor | Earlier package-order contaminant | Bisect only within serialized Drive package |
| Core remains healthy while helper times out | Readiness URL/status interpretation defect | Fix helper against actual readiness contract |
| Core/network is removed by parent shell while Go test still runs | E2E lifecycle ownership defect | Fix parent stack lifetime/trap boundary |

### Root Cause

The observability fixture and its Drive neighbors do not stop or exhaust core. The broad signature is a parent-runner interruption cascade. `smackerel.sh` tracks `e2e_child_run_id` only in the host Docker CLI process tree. Docker daemon owns the Go runner container, which had no run-ID label and survived until `e2e_down_test_stack` began. The controlled BUG-031-009 regression observed that exact runner alive at teardown start. Product-level remediation is therefore routed to `specs/031-live-stack-testing/bugs/BUG-031-009-docker-e2e-runner-interruption-leak/`.

### Impact Analysis

- Affected components: Drive E2E observability scenario, readiness diagnostics, and possibly parent test-stack lifecycle.
- Affected data: disposable test data only.
- Affected users: contributors and release validation; any confirmed core runtime crash also affects runtime reliability.

## Fix Design

### Solution Selection Rule

Fix the first proven defect only:

1. Runtime resource defect: bound/release the resource and add a focused production-unit regression.
2. Test cleanup defect: make ownership explicit and delete only test-created rows.
3. Readiness defect: poll the canonical endpoint with bounded requests and preserve last observed diagnostics; no arbitrary sleep.
4. Parent lifecycle defect: keep stack teardown in the parent shell after all serialized packages finish.

### Regression Design

- Run observability alone on a fresh disposable stack.
- Run the predecessor, observability, and successor health scenario in package order without restarting the stack.
- Assert live metric families, exact counter/row reconciliation, and core health after cleanup.
- Capture container/network state on failure without mutating the stack before evidence is recorded.

## Change Boundary

Allowed after root-cause confirmation:

- this `BUG-038-003` routing packet
- owning remediation in `BUG-031-009`

Excluded:

- arbitrary sleeps or longer blind timeouts
- all-package E2E execution in this invocation
- synthesis/assistant packet edits
- evo-x2, `knb`, deploy adapters/manifests, secrets, and release-train bundles

## Complexity Tracking

None - Drive code requires no change; remediation stays in the owning parent E2E lifecycle.
