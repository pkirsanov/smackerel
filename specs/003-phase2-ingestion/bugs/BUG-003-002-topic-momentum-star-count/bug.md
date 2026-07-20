# Bug: BUG-003-002 Topic momentum queries a missing star_count column

## Summary

The production topic lifecycle query reads `topics.star_count`, but the canonical schema stores explicit user stars on `artifacts.user_starred` and relates artifacts to topics through `BELONGS_TO` edges. PostgreSQL rejects every momentum run before any topic is updated.

## Severity

- [ ] Critical - System unusable, data loss
- [x] High - Major feature broken, no workaround
- [ ] Medium - Feature broken, workaround exists
- [ ] Low - Minor issue, cosmetic

The failure blocks all scheduled topic momentum and lifecycle updates while the wider service remains available.

## Status

- [x] Reported
- [x] Confirmed (live red-team counterexample)
- [x] In Progress
- [x] Fixed
- [ ] Verified
- [ ] Closed

## Reproduction Steps

1. Start Smackerel from source `f5f05450848630fe84c0a215429bdfc701c4bcd2` against PostgreSQL initialized from the canonical migrations.
2. Persist a topic; the canonical `topics` table has no `star_count` column.
3. Run `topics.Lifecycle.UpdateAllMomentum` directly or allow the hourly scheduler to invoke it.
4. Observe PostgreSQL reject the lifecycle SELECT before any momentum row is updated.

## Expected Behavior

The lifecycle query derives `explicit_star_count` from starred artifacts linked to each topic by canonical `artifact -> topic` `BELONGS_TO` edges. A topic with no linked starred artifacts receives a zero star contribution. PostgreSQL query failures remain returned by `UpdateAllMomentum` and logged by the scheduler as failures.

## Actual Behavior

The lifecycle SELECT reads a column that does not exist, so every run returns `query topics: ERROR: column t.star_count does not exist (SQLSTATE 42703)`. The scheduler logs the error and the process remains healthy, but no topic momentum or lifecycle state is recalculated.

## Environment

- Service: `smackerel-core`
- Source: `f5f05450848630fe84c0a215429bdfc701c4bcd2`
- Observed at: `2026-07-20T20:00:00Z`
- Store: PostgreSQL using `internal/db/migrations/001_initial_schema.sql`
- Scheduler path: `internal/scheduler/jobs.go::doTopicMomentumJob`

## Error Output

```text
topic momentum update failed: query topics: ERROR: column t.star_count does not exist (SQLSTATE 42703)
```

## Root Cause

`internal/topics/lifecycle.go::UpdateAllMomentum` selects `COALESCE(t.star_count, 0)`. No migration defines that column. R-208 defines the signal as explicit user stars, whose canonical persistence field is `artifacts.user_starred`; topic membership is represented by an `edges` row with `src_type='artifact'`, `dst_type='topic'`, and `edge_type='BELONGS_TO'`. The lifecycle implementation drifted from both the schema and the neighboring graph-query convention.

## Impact And Observability

- Momentum scores and state transitions stop for every topic.
- The scheduler emits an error log and does not emit `topic momentum updated` for the failed run.
- Public health remains available because this scheduled-job failure is not part of the current health contract.
- This repair preserves that error behavior and does not introduce a broader health feature.

## Fix

`UpdateAllMomentum` now counts distinct `artifacts.user_starred IS TRUE` rows joined through canonical `artifact -> topic` `BELONGS_TO` edges. No schema, scheduler, health, deployment, or configuration surface changed.

## Related

- Feature: [specs/003-phase2-ingestion](../../)
- Owning scope: Scope 6, Topic Lifecycle
- Requirement: R-208 Topic Lifecycle System
- Scenario: SCN-003-022 Topic momentum calculation
- Related bug: [BUG-003-001](../BUG-003-001-topic-lifecycle-duplicate-seed/bug.md)
