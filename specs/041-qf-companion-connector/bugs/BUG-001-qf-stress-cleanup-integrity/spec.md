# Expected Behavior: [BUG-001] QF Stress Cleanup Integrity

## Problem Statement

QF stress tests own rows in PostgreSQL and use NATS while exercising live connector behavior. Their teardown must be deterministic and observable. A test that closes its pool before row cleanup, logs cleanup failures, or measures freshness from a fixed historical timestamp cannot prove isolation or the parent feature's freshness contract.

## Outcome Contract

**Intent:** Make QF stress fixture teardown deterministic, set-based, isolated, and fail-loud while preserving meaningful current-run freshness timestamps.

**Success Signal:** After a child stress fixture returns, its parent test queries through an open pool and observes zero rows owned by the child's source or artifact IDs across `artifacts`, `annotations`, `edges`, and `sync_state`; an unowned control row remains; forced cleanup failure is an asserted error; and all three QF stress flows retain current-run timestamps and pass their existing behavior assertions.

**Hard Constraints:** Data cleanup executes before NATS and pool closure; cleanup is set-based and bounded by a transaction/context; any cleanup error fails the test; no unowned row is deleted; freshness data uses UTC runtime timestamps; no production source, migration, configuration, deployment, parent feature artifact, or cross-repository file changes.

**Failure Condition:** Any owned parent or child row survives cleanup, a cleanup error is only logged, a close callback can run before data cleanup, an unowned control row is removed, or a fixed historical timestamp can satisfy the freshness fixture.

## Requirements

- **QFS-CLEAN-001:** Each affected test SHALL register `pool.Close` with `t.Cleanup` immediately after opening the pool and register `natsClient.Close` with `t.Cleanup` immediately after opening NATS.
- **QFS-CLEAN-002:** After creating `sourceID`, each affected test SHALL register the owned-row cleanup callback after both close callbacks. Under LIFO semantics the required execution order is owned-row cleanup, NATS close, then pool close.
- **QFS-CLEAN-003:** The affected tests SHALL NOT combine function-level `defer pool.Close()` or `defer natsClient.Close()` with a `t.Cleanup` callback that requires those resources.
- **QFS-CLEAN-004:** `qfDecisionsStressCleanup` SHALL delete edges whose `src_id` or `dst_id` belongs to the source-owned artifact set, annotations whose `artifact_id` belongs to that set, source-owned artifacts, and source-owned `sync_state` rows with set-based SQL rather than per-artifact delete loops.
- **QFS-CLEAN-005:** Child rows SHALL be deleted before parent artifacts inside one bounded transaction. Any begin, statement, verification, rollback, or commit error SHALL return contextual failure and cause the test cleanup wrapper to fail loud.
- **QFS-CLEAN-006:** Cleanup SHALL be idempotent. Calling it before fixture setup or after successful teardown SHALL succeed with zero owned rows.
- **QFS-CLEAN-007:** Cleanup SHALL preserve every artifact, annotation, edge, and sync-state row that is not owned by the target `sourceID` or its known artifact IDs.
- **QFS-CLEAN-008:** A parent/child regression SHALL keep the parent pool open, let the child register and execute cleanup, and then independently assert zero source-owned parent and child rows after `t.Run` returns.
- **QFS-CLEAN-009:** The parent/child regression SHALL seed at least two owned artifacts, one owned annotation, one edge with an owned endpoint, one owned sync-state row, and an unowned control artifact. The control artifact SHALL remain after cleanup.
- **QFS-CLEAN-010:** An adversarial regression SHALL force the cleanup core to encounter a closed or otherwise unusable pool and SHALL assert a non-nil contextual error. The public test cleanup wrapper SHALL convert such an error to `t.Fatalf`, never `t.Logf`.
- **QFS-TIME-001:** `TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity` SHALL capture one UTC RFC3339Nano `runTimestamp` and pass it to both `stressEnvelope` and `stressEvent` so packet and event fixture times agree within the run.
- **QFS-TIME-002:** `TestQFDecisionsFreshnessSLAP95IngestRender` and `TestQFDecisionsFreshnessSLAP95RenderAndCombined` SHALL stamp events and fetched packet envelopes from current response-time UTC values so jitter and connector processing, not historical fixture age, determine freshness.
- **QFS-TIME-003:** The fixed value `2026-05-06T00:00:00Z` SHALL not remain as the creation or update time of these QF stress fixtures.
- **QFS-REG-001:** Existing QF connector E2E behavior and the broader QF stress suite SHALL remain green after the test-harness repair.

## User Scenarios

```gherkin
Scenario: SCN-041-BUG001-001 Cleanup runs before live resources close
  Given a QF stress test has opened PostgreSQL and NATS resources
  And their close callbacks were registered before the source-owned cleanup callback
  When the testing package executes cleanup callbacks in LIFO order
  Then source-owned database cleanup completes first
  And NATS closes second
  And the PostgreSQL pool closes last

Scenario: SCN-041-BUG001-002 Parent observes zero owned parent and child rows
  Given a child subtest created two source-owned artifacts, an annotation, an edge, and sync state
  And an unowned control artifact exists
  When the child subtest returns and its cleanup completes
  Then the parent observes zero owned artifacts, annotations, edges, and sync-state rows
  And the unowned control artifact still exists

Scenario: SCN-041-BUG001-003 Cleanup failure cannot become a passing log
  Given the cleanup core receives a closed or unusable PostgreSQL pool
  When it attempts set-based cleanup
  Then it returns a contextual error
  And the test cleanup wrapper fails the test instead of logging and continuing

Scenario: SCN-041-BUG001-004 Freshness fixtures use current-run time
  Given a QF sync or freshness stress run starts now
  When it builds decision events and packet envelopes
  Then stable sync fixtures share one run-scoped UTC timestamp
  And freshness fixtures use response-time UTC timestamps
  And no historical fixed 2026 timestamp determines the measured freshness
```

## Acceptance Criteria

1. `SCN-041-BUG001-001` proves the exact LIFO registration and execution order in all three affected stress functions.
2. `SCN-041-BUG001-002` proves zero source-owned parent/child residue from an independent parent after child cleanup and proves control-row isolation.
3. `SCN-041-BUG001-003` is adversarial and fails if cleanup returns to log-only behavior.
4. `SCN-041-BUG001-004` fails if the historical fixed timestamp returns or packet/event fixture times drift from their required runtime policy.
5. The focused RED test is executed before implementation and the identical test is GREEN afterward.
6. Existing QF E2E and broader stress regressions run through `./smackerel.sh`; no direct Go or Docker command is used.
7. Only the two exact stress files and this bug packet change.

## Exposure Contract

This repair changes no product route, API, UI, connector contract, data schema, or operator surface. It is internal live-test infrastructure for the already-delivered QF Companion Connector.

## Product Principle Alignment

- **Principle 8 - Trust Through Transparency:** A cleanup failure must be visible as a failed test, and freshness evidence must reflect current execution rather than a historical literal.
- **Principle 10 - QF Companion Boundary:** The repair preserves QF packet identity, trace metadata, provenance fields, and the read-only companion boundary while validating only Smackerel-owned test data.
- No principle deviation is proposed. This packet does not claim new product behavior or roadmap delivery.

## Release Train

- Target train: `mvp`.
- Flags introduced: none.
- No behavior is enabled on another train because this is an unflagged test-integrity repair.

## Non-Goals

- Changing production connector, pipeline, storage, metric, or rendering code.
- Changing database migrations or table contracts.
- Changing QF freshness SLA thresholds.
- Modifying the QuantitativeFinance repository.
- Running Docker, stress, E2E, validation, or deployment commands during packet creation.
