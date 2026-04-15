## Checklist

### [Bug Fix] [BUG-002] Scheduler god-orchestrator refactoring
- [x] **What:** Extract 13 inline cron callbacks into separate Job files with a shared interface
  - **Steps:**
    1. Verify all cron jobs still registered after refactoring (CronEntryCount)
    2. Verify Start/Stop lifecycle unchanged
    3. Verify scheduler.go no longer imports domain packages
  - **Expected:** Same behavior, fewer lines, decoupled packages
  - **Verify:** `./smackerel.sh test unit`
  - **Evidence:** report.md#scope-1
  - **Notes:** Bug fix for [BUG-002] — refactoring, no behavior change
