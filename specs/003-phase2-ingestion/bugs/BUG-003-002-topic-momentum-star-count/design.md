# Bug Fix Design: BUG-003-002

## Root Cause Analysis

### Investigation Summary

The failing path is `internal/scheduler/jobs.go::doTopicMomentumJob` -> `internal/topics/lifecycle.go::UpdateAllMomentum` -> PostgreSQL. The scheduler correctly logs the returned error. The SQL fails before row iteration because it selects `t.star_count` from `topics`.

The canonical schema and neighboring repository queries establish a different model:

1. `internal/db/migrations/001_initial_schema.sql` persists explicit user intent as `artifacts.user_starred BOOLEAN`.
2. The same migration defines generic graph relationships in `edges` and defines no topic star counter.
3. `internal/graph/linker.go` and `internal/connector/bookmarks/topics.go` create membership as `artifact -> topic` edges with `edge_type='BELONGS_TO'`.
4. `internal/intelligence/learning.go` and `internal/intelligence/expertise.go` query topic artifacts by joining those `BELONGS_TO` edges to `artifacts`.
5. R-208 names the signal `explicit_star_count` and describes explicit stars as user pins.

### Root Cause

The lifecycle SQL introduced a read from a denormalized `topics.star_count` that was never part of the canonical data model. The pure momentum function accepts a star count correctly, but the repository boundary obtains that input from a nonexistent column rather than aggregating persisted starred artifacts through topic membership edges.

### Impact Analysis

- Affected component: topic lifecycle scheduled job.
- Affected data behavior: all topic momentum and state updates are skipped because the initial SELECT fails.
- Data corruption: none identified; the query fails before updates.
- Observability: the scheduler emits an error log, but unrelated public health checks remain healthy by current design.
- Blast radius: every topic in every PostgreSQL-backed runtime using this source revision.

## Fix Design

### Solution Approach

Replace only the invalid star expression in `UpdateAllMomentum` with a correlated aggregate over canonical relationships:

```sql
COALESCE((
    SELECT COUNT(DISTINCT a.id)
    FROM edges e
    JOIN artifacts a
      ON a.id = e.src_id
     AND e.src_type = 'artifact'
    WHERE e.dst_type = 'topic'
      AND e.dst_id = t.id
      AND e.edge_type = 'BELONGS_TO'
      AND a.user_starred IS TRUE
), 0)::int
```

This keeps `CalculateMomentum` and state-transition semantics unchanged. It uses the same edge direction and type as neighboring repository queries, counts each starred artifact once, excludes unstarred or unrelated artifacts, and naturally returns zero when no matching relationship exists.

### Scheduler And Error Semantics

No production scheduler or health behavior changes. `UpdateAllMomentum` continues to wrap initial query failures as `query topics: %w`; `doTopicMomentumJob` continues to log `topic momentum update failed` on error and `topic momentum updated` only on success. A focused unit regression uses a real `Lifecycle` with a closed PostgreSQL pool and captures structured logs to prove the failure path remains distinct.

### Regression Test Design

The RED/GREEN integration test runs through `./smackerel.sh test integration-light`, which creates disposable PostgreSQL/NATS state, applies migrations through the production `cmd/dbmigrate` entry point, injects `DATABASE_URL`, and tears down volumes on exit.

The test persists:

- a zero-star topic with one linked unstarred artifact;
- a multi-star topic with two linked starred artifacts and one linked unstarred artifact;
- a starred artifact linked to a different topic, proving unrelated stars are excluded.

It invokes the actual exported `topics.Lifecycle.UpdateAllMomentum`, then reads persisted momentum and state. Expected values include the existing connection contribution so the assertions distinguish star aggregation from graph connectivity.

The existing targeted topic lifecycle shell E2E remains a broader surface regression. No test-only endpoint or scheduler trigger is introduced.

### Alternative Approaches Considered

1. Add `topics.star_count` in a migration - rejected because it invents a denormalized source, creates synchronization obligations, and contradicts the established artifact-star model.
2. Count every topic edge - rejected because graph connectivity is already a separate R-208 signal and does not represent explicit user intent.
3. Count `artifacts.user_starred` without joining topic membership - rejected because stars belonging to unrelated topics would inflate every topic.
4. Swallow the missing-column error or substitute zero - rejected because it masks schema drift and violates fail-loud behavior.

## Change Boundary

Allowed file families:

- `internal/topics/lifecycle.go`
- focused tests under `internal/topics/`, `internal/scheduler/`, and `tests/integration/`
- this `BUG-003-002` packet

Excluded surfaces:

- database migrations and schema definitions
- deployment adapters, manifests, and host state
- release-train and feature-flag configuration
- secrets and generated configuration
- scheduler cadence and public health contracts
- unrelated source, tests, and documentation

## Complexity Tracking

None - simplest viable fix used.

## Risks And Open Questions

None found - the canonical persistence field, edge direction, edge type, and R-208 weighting are all established by current source and feature artifacts.
