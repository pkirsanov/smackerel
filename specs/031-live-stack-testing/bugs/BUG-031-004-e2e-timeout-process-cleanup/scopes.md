# Scopes: BUG-031-004 E2E timeout process cleanup

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Make E2E timeout cleanup process-group safe

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Feature: BUG-031-004 prevent E2E timeout process leaks
  Scenario: E2E timeout terminates child processes
    Given the E2E harness has started child shell or container test work
    When the parent E2E command is interrupted by timeout
    Then all child E2E work is terminated
    And the disposable test stack cleanup path runs

  Scenario: E2E cleanup regression detects surviving child work
    Given a child E2E process continues after the parent exits
    When the cleanup regression inspects the E2E process group and test stack
    Then the regression fails and reports the surviving child work
```

### Implementation Plan
1. Reproduce the timeout signature with a controlled, bounded DevOps test.
2. Trace process-group ownership across `./smackerel.sh test e2e`, `docker run`, and shell E2E runners.
3. Add signal forwarding, process-group cleanup, or child reaping as required by the confirmed root cause.
4. Ensure cleanup calls the repo lifecycle path for the test environment and preserves persistent dev volumes.
5. Add regression coverage for timeout/interruption cleanup.

### Test Plan

Regression source note (2026-04-30): `tests/e2e/fixtures/test_timeout_child_fixture.sh` creates an adversarial child that ignores termination signals. `tests/e2e/test_timeout_process_cleanup.sh` now invokes that fixture through the repo E2E runner and asserts that interrupted parent cleanup leaves zero marker processes.

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-031-004-01 | Timeout terminates E2E process group | integration/devops | `tests/e2e/test_timeout_process_cleanup.sh` via `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` | Controlled timeout leaves no child E2E processes running | BUG-031-004-SCN-001 |
| T-BUG-031-004-02 | Cleanup regression detects surviving child work | integration/devops | `tests/e2e/test_timeout_process_cleanup.sh` via `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` | Test fails when a child process survives the parent timeout | BUG-031-004-SCN-002 |
| T-BUG-031-004-03 | Broader E2E lifecycle | e2e-api | `./smackerel.sh test e2e` | Broad E2E completion/failure returns with deterministic cleanup | BUG-031-004-SCN-001 |

### Definition of Done
- [x] Root cause confirmed and documented with pre-fix failure evidence
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` against the TERM-only cleanup path; **Exit Code:** 1; **Claim Source:** executed; the regression reported `Surviving child work for marker ...-runner` after the nested E2E runner interruption.
- [x] Parent timeout/interruption terminates child E2E shell/container work
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`; **Exit Code:** 0; **Claim Source:** executed; `BUG-031-004-SCN-001` passed and printed `Marker processes absent for ...-runner` after `kill -TERM` interrupted the nested runner.
- [x] Disposable test stack cleanup runs after timeout/interruption through the repo lifecycle path
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`; **Exit Code:** 0; **Claim Source:** executed; nested runner output included `Running project-scoped test stack teardown (exit cleanup, timeout 180s)`.
- [x] Pre-fix regression test fails for surviving child work
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh` before the KILL fallback; **Exit Code:** 1; **Claim Source:** executed; output ended with `FAIL: marker child process survived E2E timeout cleanup`.
- [x] Adversarial regression case exists for a child process that outlives the parent
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`; **Exit Code:** 0; **Claim Source:** executed; `BUG-031-004-SCN-002` starts `tests/e2e/fixtures/test_timeout_child_fixture.sh`, verifies the detector reports the live marker, then kills the marker process.
- [x] Post-fix timeout cleanup regression passes
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --shell-run test_timeout_process_cleanup.sh`; **Exit Code:** 0; **Claim Source:** executed; shell E2E summary reported `PASS: test_timeout_process_cleanup.sh`, `Total: 1`, `Failed: 0`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  - Evidence: **Phase:** implement; **Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-004-e2e-timeout-process-cleanup`; **Exit Code:** 0; **Claim Source:** executed after manifest/test-plan repair; both `BUG-031-004-SCN-001` and `BUG-031-004-SCN-002` link to `tests/e2e/test_timeout_process_cleanup.sh`.
- [x] Broader E2E regression suite passes or exits cleanly with no leaked child work when product regressions remain
  - Evidence: **Phase:** implement; **Command:** `./smackerel.sh test e2e --go-run TestKnowledgeStore_TablesExist`; **Exit Code:** 0; **Claim Source:** executed; output reported `PASS: go-e2e` followed by project-scoped test stack teardown, and `docker ps --filter label=com.docker.compose.project=smackerel-test` showed no running containers.
- [x] Regression tests contain no silent-pass bailout patterns
  - Evidence: **Phase:** implement; **Command:** `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/test_timeout_process_cleanup.sh`; **Exit Code:** 0; **Claim Source:** executed; guard reported `REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)`.
- [x] Bug marked as Fixed in bug.md by the validation owner
  - Evidence: **Phase:** implement; **Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing/bugs/BUG-031-004-e2e-timeout-process-cleanup` and `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing/bugs/BUG-031-004-e2e-timeout-process-cleanup`; **Exit Code:** 0 for both final gates; **Claim Source:** executed; `bug.md` records Reported, Confirmed, In Progress, Fixed, Verified, and Closed after the concrete regression and guard passes.
