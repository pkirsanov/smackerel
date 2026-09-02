# Bug Fix Design: [BUG-001] QF Stress Cleanup Integrity

## Root Cause Analysis

### Investigation Summary

Current-tree inspection covered the dirty diff and the complete helper/resource setup in:

- `tests/stress/qf_decisions_sync_stress_test.go`
- `tests/stress/qf_decision_event_replay_test.go`

The sync stress test's dirty edit moves pool and NATS closure from function-level `defer` to `t.Cleanup`, registered before data cleanup. That is the correct direction because `t.Cleanup` is LIFO. Both freshness tests still use function-level resource defers while their data cleanup uses `t.Cleanup`. The shared helper still performs a query-plus-loop deletion and logs every failure.

### Root Cause

Three independent test-harness defects combine:

1. Function defers execute while the test function is returning, before the testing package invokes `t.Cleanup`. A data cleanup callback registered with `t.Cleanup` therefore sees a pool already closed by `defer pool.Close()`.
2. `qfDecisionsStressCleanup` treats query and delete failures as informational logs. A teardown error does not fail the test, so owned rows can leak without changing the test result.
3. `stressEnvelope` and `stressEvent` used `2026-05-06T00:00:00Z`. Freshness metrics subtract those values from current observation time, so the fixture eventually measures calendar age rather than connector latency.

This analysis is grounded by executed source/diff inspection. Runtime reproduction remains required before implementation. **Claim Source:** interpreted.

### Impact Analysis

- Affected tests: three QF live stress functions.
- Affected resources: disposable PostgreSQL rows and NATS/pool lifecycle.
- Affected tables: `artifacts`, `annotations`, `edges`, and `sync_state`.
- Affected evidence: ingest, render, and combined freshness measurements.
- Production users and production data: no direct impact established or claimed.

## Fix Design

### Deterministic Cleanup Registration

Each affected test uses one cleanup mechanism for live resources and data:

```go
pool, err := pgxpool.New(ctx, databaseURL)
// fail on err
t.Cleanup(pool.Close)

natsClient, err := smacknats.Connect(ctx, natsURL, cfg.AuthToken)
// fail on err
t.Cleanup(natsClient.Close)

sourceID := uniqueQFSourceID()
t.Cleanup(func() {
	qfDecisionsStressCleanup(t, pool, sourceID)
})
```

Registration order is intentionally the reverse of execution order. LIFO executes source-owned row cleanup first, NATS close second, and pool close last. The two freshness functions replace their function-level pool/NATS defers with this pattern. HTTP server and context teardown remain function-local because the database cleanup does not depend on them.

### Set-Based Transactional Cleanup

Refactor the helper into a small error-returning cleanup core plus a testing wrapper:

1. Create a ten-second cleanup context independent of the test's expiring work context.
2. Begin a PostgreSQL transaction.
3. Delete every `edges` row whose `src_id` or `dst_id` is in the set of artifacts selected by `source_id`.
4. Delete every `annotations` row whose `artifact_id` is in the same set.
5. Delete `artifacts` rows for `source_id`.
6. Delete `sync_state` rows for `source_id`.
7. Verify zero owned rows remain for the known source/artifact set.
8. Commit.

The SQL uses `DELETE ... WHERE ... IN (SELECT ...)` or equivalent `EXISTS` predicates. It does not scan IDs into Go and does not issue one delete per artifact. Child deletes precede the parent delete. On any failure, rollback is attempted and the contextual error is returned. The `qfDecisionsStressCleanup` testing wrapper calls `t.Fatalf` with the source ID and wrapped error.

### Independent Parent/Child Proof

Add `TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows` to `tests/stress/qf_decisions_sync_stress_test.go`:

1. The parent opens the pool and owns its close callback.
2. The parent creates an unowned control artifact and records its ID.
3. A child subtest creates two artifacts for a unique source, one annotation, one edge with an owned endpoint, and one sync-state row. It records both owned artifact IDs and registers `qfDecisionsStressCleanup`.
4. The child returns, which runs its cleanup while the parent pool remains open.
5. The parent independently counts rows by source and known owned IDs. Counts for owned artifacts, annotations, edges, and sync state must all be zero.
6. The parent verifies the unowned control artifact still exists, then removes the control through its own fail-loud cleanup.

This pattern proves post-cleanup state. An assertion inside the callback alone would only prove what the callback believes it did.

### Adversarial Failure Proof

Add `TestQFDecisionsStressCleanupReturnsErrorForClosedPool` beside the helper. It closes a dedicated pool before calling the error-returning cleanup core and directly asserts a non-nil contextual error. This test fails if the core returns nil or resumes log-only handling. The normal wrapper remains responsible for translating any such error into `t.Fatalf` during real test cleanup.

### Timestamp Policy

- Stable replay/sync identity test: capture one `runTimestamp := time.Now().UTC().Format(time.RFC3339Nano)` before building its event and envelope maps, and inject that exact value into both builders.
- Ingest freshness test: create event timestamps at page-response time and packet-envelope timestamps at packet-response time.
- Render/combined freshness test: use response-time event and envelope timestamps consistent with its current latency model.
- Remove the fixed `2026-05-06T00:00:00Z` builder values. Do not replace freshness response times with one timestamp captured before a long stress run because that would reintroduce age into the latency sample.

## Exact Files

| Path | Planned change |
|---|---|
| `tests/stress/qf_decisions_sync_stress_test.go` | Set-based error-returning cleanup core, fail-loud wrapper, parent/child residue regression, forced-failure regression, correct LIFO registration, and run-scoped sync timestamp. |
| `tests/stress/qf_decision_event_replay_test.go` | Replace pool/NATS function defers with ordered `t.Cleanup` callbacks and retain response-time freshness timestamps for both freshness functions. |

No other source or test file is in the implementation boundary.

## Test Design

- RED: the parent/child test must expose residue or cleanup failure before the helper/order repair.
- GREEN: the same test observes zero owned parent/child rows and a surviving control.
- Adversarial: a closed pool produces an asserted error rather than a passing log.
- Timestamp regression: the stable sync fixture's event/envelope times equal the injected run timestamp; the freshness tests' existing p95 assertions reject historical fixed values.
- Consumer canary: existing QF connector E2E tests prove test-only changes did not alter live connector behavior.
- Broad regressions: full QF E2E and stress suites run through `./smackerel.sh` against disposable state.

## Security And Data Isolation

- Source IDs are unique per run.
- Cleanup predicates are source/known-ID scoped and never truncate shared tables.
- Error messages name the source and operation but do not print database URLs, NATS credentials, auth tokens, or row payloads.
- Only the disposable test stack is permitted for live execution.

## Observability

The repaired tests continue asserting `smackerel_qf_freshness_p95_seconds` through the existing stress flows. This packet adds no product metric or trace workflow because the changed behavior is test teardown and fixture time, not a new service-bearing product workflow.

## Alternative Approaches Considered

1. Keep function-level resource defers and move row cleanup to another defer - rejected because teardown order would remain split across mechanisms and cleanup errors would still be easy to hide.
2. Continue per-artifact delete loops but replace `t.Logf` with `t.Fatalf` - rejected because it preserves unnecessary round trips and partial-cleanup risk.
3. Truncate the four tables after each test - rejected because it destroys isolation and can delete rows owned by other tests.
4. Keep fixed historical timestamps for deterministic fixtures - rejected because freshness is explicitly an elapsed-time contract and historical age dominates the measurement.

## Complexity Tracking

None - the design uses one transaction, four set-based delete/verification surfaces, one wrapper, and two focused regressions.

## Risks And Open Questions

- The RED run may expose additional foreign-key child tables. If it does, record a new finding and route it rather than broadening cleanup silently.
- Exact SQL types for known artifact ID arrays must use the repository's existing PostgreSQL types; no schema change is authorized.
- No runtime result is known until the RED and GREEN stress executions occur.
