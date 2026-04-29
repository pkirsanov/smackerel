# Scopes: BUG-031-004 E2E timeout process cleanup

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Make E2E timeout cleanup process-group safe

**Status:** In Progress
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

| ID | Test Name | Type | Location | Assertion | Scenario ID |
|---|---|---|---|---|---|
| T-BUG-031-004-01 | Timeout terminates E2E process group | integration/devops | owner-selected DevOps regression | Controlled timeout leaves no child E2E processes running | BUG-031-004-SCN-001 |
| T-BUG-031-004-02 | Cleanup regression detects surviving child work | integration/devops | owner-selected DevOps regression | Test fails when a child process survives the parent timeout | BUG-031-004-SCN-002 |
| T-BUG-031-004-03 | Broader E2E lifecycle | e2e-api | `./smackerel.sh test e2e` | Broad E2E completion/failure returns with deterministic cleanup | BUG-031-004-SCN-001 |

### Definition of Done
- [ ] Root cause confirmed and documented with pre-fix failure evidence
- [ ] Parent timeout/interruption terminates child E2E shell/container work
- [ ] Disposable test stack cleanup runs after timeout/interruption through the repo lifecycle path
- [ ] Pre-fix regression test fails for surviving child work
- [ ] Adversarial regression case exists for a child process that outlives the parent
- [ ] Post-fix timeout cleanup regression passes
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes or exits cleanly with no leaked child work when product regressions remain
- [ ] Regression tests contain no silent-pass bailout patterns
- [ ] Bug marked as Fixed in bug.md by the validation owner
