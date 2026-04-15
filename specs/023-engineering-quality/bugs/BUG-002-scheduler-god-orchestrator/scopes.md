# Scopes: [BUG-002] Scheduler god-orchestrator

## Scope 1: Extract Job interface and migrate cron callbacks
**Status:** [ ] Not started

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: [Bug] Scheduler job extraction preserves behavior
  Scenario: All 13 cron jobs are registered after Start()
    Given the scheduler is constructed with the same dependencies
    When Start() is called with a valid cron expression
    Then CronEntryCount() returns the same count as before refactoring

  Scenario: Scheduler Stop() drains all jobs cleanly
    Given the scheduler has running jobs
    When Stop() is called
    Then all background goroutines complete within the timeout

  Scenario: New job can be added without modifying scheduler.go
    Given the Job interface is implemented by a new struct
    When the job is added to the jobs slice passed to the scheduler
    Then it is registered as a cron entry without changes to scheduler.go
```

### Implementation Plan
1. Create `internal/scheduler/job.go` with Job interface
2. Extract each inline closure to its own job file
3. Refactor `scheduler.go` to accept `[]Job` and register them in a loop
4. Update `cmd/core/main.go` to construct jobs and pass to scheduler
5. Verify all tests pass unchanged

### Test Plan
| Type | Label | Description |
|------|-------|-------------|
| Unit | Regression unit | Verify CronEntryCount, Stop, DigestPendingRetry all work identically |
| Unit | Job interface | Verify each extracted job implements the interface correctly |
| Integration | Regression E2E | Full scheduler start/stop cycle with mocked dependencies |

### Definition of Done — 3-Part Validation
- [ ] scheduler.go is ≤150 LOC
- [ ] Each cron job is in its own file under internal/scheduler/jobs/
- [ ] No direct domain-package imports in scheduler.go
- [ ] All existing scheduler tests pass unchanged
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
