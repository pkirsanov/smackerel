# Bug: [BUG-001] QF Stress Cleanup Integrity

## Summary

The QF sync and freshness stress fixtures can close PostgreSQL and NATS resources before database cleanup, while the shared cleanup helper reports deletion failures with `t.Logf` and allows owned test rows to survive. The baseline fixture also used the fixed timestamp `2026-05-06T00:00:00Z`, which no longer represents a fresh event and invalidates freshness measurements.

## Severity

High (S2 test-integrity defect). The defect can contaminate later live-stack tests and can make a leaking cleanup path look successful. No production-runtime impact is claimed.

## Status And Provenance

Reported and analyzed from current-tree inspection at Smackerel revision `7ce32d703a17f3cb5a2481a1ebc0cc084a1fac61` on 2026-09-02. The dirty diff was inspected in this session. No stress test, Docker command, source edit, or test edit was executed. **Claim Source:** executed for repository inspection; interpreted for the root-cause analysis; not-run for runtime reproduction.

## Reproduction Steps

1. Use the disposable Smackerel test stack required by the stress suite.
2. Run `TestQFDecisionsFreshnessSLAP95IngestRender` or `TestQFDecisionsFreshnessSLAP95RenderAndCombined` from `tests/stress/qf_decision_event_replay_test.go`.
3. Allow the test body to return after it has published rows for its unique `sourceID`.
4. Observe that the function-level `defer pool.Close()` runs before the testing package invokes the registered `t.Cleanup` callback, so `qfDecisionsStressCleanup` receives a closed pool.
5. Observe that cleanup query and delete errors are logged instead of failing the test, allowing artifacts, annotations, edges, or `sync_state` rows owned by that source to remain.
6. In the baseline form of `tests/stress/qf_decisions_sync_stress_test.go`, retain `2026-05-06T00:00:00Z` in `stressEnvelope` and `stressEvent`; the resulting observations no longer measure current-run freshness.

These steps define the future RED execution. They were not run during packet creation because the request forbids Docker and asks for artifacts only.

## Expected Behavior

- PostgreSQL and NATS close callbacks use `t.Cleanup` and are registered before the data-cleanup callback. Go's LIFO cleanup order must therefore be: owned-row cleanup, NATS close, PostgreSQL pool close.
- QF cleanup deletes child and parent rows with set-based statements inside one bounded transaction.
- Every begin, query, delete, verification, and commit error fails the owning test with source-scoped context.
- An independent parent/child regression observes zero owned artifacts, annotations, edges, and sync-state rows after the child cleanup returns, while an unowned control row survives.
- The stable sync fixture uses one run-scoped UTC timestamp. Freshness fixtures use response-time UTC timestamps so measured latency represents current execution rather than a historical date.

## Actual Behavior

- Both freshness tests currently combine function-level resource `defer` calls with a later `t.Cleanup` database callback. Function defers run before testing cleanup callbacks.
- `qfDecisionsStressCleanup` queries artifact IDs, loops over them, and uses `t.Logf` for query and delete errors.
- The baseline sync fixture hard-codes `2026-05-06T00:00:00Z` for packet and event creation.
- The current dirty sync-file edit changes resource closes to `t.Cleanup` and injects a run timestamp. Those changes are diagnostic input and are not treated as a completed fix.

## Environment

- Repository: `smackerel`
- Parent feature: `specs/041-qf-companion-connector`
- Revision inspected: `7ce32d703a17f3cb5a2481a1ebc0cc084a1fac61`
- Platform: Linux
- Test category: live `stress` against disposable PostgreSQL and NATS
- Runtime execution in this packet: none

## Inspection Output

The current dirty paths are:

```text
 M tests/stress/qf_decision_event_replay_test.go
 M tests/stress/qf_decisions_sync_stress_test.go
```

The inspected diff changes `stressEnvelope` and `stressEvent` to accept runtime timestamps and changes the sync stress test's resource closes from `defer` to `t.Cleanup`. It does not yet make the shared cleanup helper set-based or fail-loud, and the freshness tests still use function-level resource defers.

## Root Cause

The test harness mixes two teardown mechanisms with different execution points. Function-level defers close resources as the test function returns; `t.Cleanup` callbacks run afterward. The shared data cleanup therefore cannot rely on those resources. Separately, error-only logging converts teardown failure into a successful test outcome, and historical fixture timestamps make freshness assertions measure fixture age rather than processing latency.

## Exact Change Boundary

Future implementation is limited to:

- `tests/stress/qf_decisions_sync_stress_test.go`
- `tests/stress/qf_decision_event_replay_test.go`
- this bug packet

Production source, migrations, configuration, parent feature artifacts, deployment files, the QuantitativeFinance repository, and unrelated tests are excluded.

## Related

- Parent feature: `specs/041-qf-companion-connector/`
- Parent scenarios: `SCN-SM-041-003` and `SCN-SM-041-008`
- Existing stress functions: `TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity`, `TestQFDecisionsFreshnessSLAP95IngestRender`, and `TestQFDecisionsFreshnessSLAP95RenderAndCombined`
- Shared helper: `qfDecisionsStressCleanup`
