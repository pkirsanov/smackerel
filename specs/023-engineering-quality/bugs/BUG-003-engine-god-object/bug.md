# Bug: [BUG-003] Engine god-object — 1256 LOC with 4 mixed responsibilities

## Summary
`internal/intelligence/engine.go` is a god-object with 17 methods mixing synthesis, alert CRUD, alert production (4 producers), meeting briefs, and weekly synthesis on a single struct. At 1256 LOC it's the largest file in the codebase and the #2 coupling hotspot (16 changes in 30 days, 88% bug ratio).

## Severity
- [ ] Critical
- [ ] High
- [x] Medium - Feature works, but the file accumulates every intelligence responsibility
- [ ] Low

## Status
- [x] Reported
- [x] Confirmed (reproduced via retro hotspot analysis)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps
1. Run `wc -l internal/intelligence/engine.go` → 1256 LOC
2. Run `grep "^func (e \*Engine)" internal/intelligence/engine.go | wc -l` → 17 methods
3. Observe 4 distinct responsibility groups:
   - Synthesis: RunSynthesis, GenerateWeeklySynthesis, detectCapturePatterns, GetLastSynthesisTime
   - Alert CRUD: CreateAlert, DismissAlert, SnoozeAlert, GetPendingAlerts, MarkAlertDelivered
   - Alert producers: ProduceBillAlerts, ProduceTripPrepAlerts, ProduceReturnWindowAlerts, ProduceRelationshipCoolingAlerts
   - Briefs/commitments: GeneratePreMeetingBriefs, buildAttendeeBrief, CheckOverdueCommitments, collectOverdueItems
4. Note: the REST of the intelligence package already follows split pattern (expertise.go, subscriptions.go, monthly.go, etc.) — engine.go is the outlier

## Expected Behavior
Each responsibility group should be in its own file, following the pattern established by `expertise.go`, `subscriptions.go`, `monthly.go`, `resurface.go`, etc.

## Actual Behavior
All 17 methods live in a single 1256-LOC file. Any change to alert production, synthesis, or briefs shows as a modification to engine.go, inflating the hotspot metrics.

## Environment
- Service: smackerel-core
- File: `internal/intelligence/engine.go` (1256 LOC, 17 methods)
- Evidence: retro 2026-04-15 hotspot analysis

## Root Cause
Organic growth — new methods were added to engine.go as features were implemented rather than following the package's own split convention. The Engine struct and constructor belong in engine.go; the method implementations should be distributed.

## Related
- Feature: `specs/023-engineering-quality/`
- Retro: `.specify/memory/retros/2026-04-15-hotspots.md`
- Co-hotspot: BUG-002 (scheduler.go), BUG-004 (main.go)
