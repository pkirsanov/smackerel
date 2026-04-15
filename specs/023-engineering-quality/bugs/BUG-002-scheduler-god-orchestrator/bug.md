# Bug: [BUG-002] Scheduler god-orchestrator — 610 LOC coupling hub

## Summary
`internal/scheduler/scheduler.go` is a god-orchestrator with 13 inline cron jobs, 12 dedicated mutexes, and direct imports of 4 domain packages. It co-changes with 6+ files across 4 packages in every modification, making it the highest-coupling file in the codebase.

## Severity
- [ ] Critical
- [ ] High
- [x] Medium - Feature works, but every change ripples across packages
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced via retro hotspot analysis)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run `git log --since="30 days ago" --name-only` and count scheduler.go co-changes
2. Observe 22 changes in 30 days, co-changing with router.go (15), main.go (15), engine.go (13), search.go (13), health.go (13), bot.go (11)
3. Count inline cron callbacks: 13 distinct jobs, each 20-50 LOC with copy-pasted TryLock/timeout/error patterns
4. Count dedicated mutexes: 12 (`muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muBriefs`, `muAlerts`, `muAlertProd`, `muResurface`, `muLookups`, `muSubs`, `muRelCool`)

## Expected Behavior
Adding a new scheduled job should require creating one file with a defined interface, not modifying the 610-LOC scheduler monolith.

## Actual Behavior
Every new cron job requires: adding a mutex field to the struct, adding a cron callback closure to `Start()`, adding timeout/error/delivery boilerplate, and importing the target package — all in scheduler.go.

## Environment
- Service: smackerel-core
- File: `internal/scheduler/scheduler.go` (610 LOC)
- Evidence: retro 2026-04-15 hotspot analysis

## Root Cause
Scheduler uses a procedural design where each job is an inline closure registered in `Start()`. No job abstraction exists — each callback is a bespoke block of code with its own mutex, context, timeout, and error handling that follows the same pattern but isn't factored.

## Related
- Feature: `specs/023-engineering-quality/`
- Retro: `.specify/memory/retros/2026-04-15-hotspots.md`
- Co-hotspot: BUG-003 (engine.go), BUG-004 (main.go)
