# Spec: [BUG-002] Scheduler god-orchestrator refactoring

## Expected Behavior
The scheduler should be a thin cron lifecycle manager that registers jobs from a `Job` interface. Each job owns its own schedule, timeout, mutex, and execution logic in a separate file. Adding a new scheduled task should never require modifying `scheduler.go`.

## Acceptance Criteria
- [ ] `scheduler.go` is ≤150 LOC (down from 610)
- [ ] Each cron job is a separate file implementing a `Job` interface
- [ ] No direct imports of `intelligence`, `digest`, `telegram`, or `topics` in scheduler.go
- [ ] All existing tests pass unchanged
- [ ] No behavior change — same cron expressions, same timeouts, same delivery logic
- [ ] The `Job` interface supports: Name, Schedule, Timeout, Run
