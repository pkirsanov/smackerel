# Scopes: [BUG-001] QF Stress Cleanup Integrity

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Deterministic QF Stress Fixture Teardown

**Status:** Not Started
**Priority:** P1
**Scope-Kind:** runtime-behavior
**Depends On:** None
**Goal Contribution:** Proves QF stress tests leave zero owned PostgreSQL residue, surface teardown errors, and measure freshness from current-run timestamps.

### Gherkin Scenarios

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

### Implementation Plan

1. Add the RED parent/child residue regression and the closed-pool adversarial regression to `tests/stress/qf_decisions_sync_stress_test.go`; execute the focused RED case before changing cleanup behavior.
2. Replace query-plus-loop/log-only cleanup with a bounded set-based transaction and an error-returning core whose test wrapper fails loud.
3. Keep pool and NATS close callbacks registered before the data cleanup callback in all three affected stress functions so LIFO execution preserves live resources through data cleanup.
4. Replace the two freshness functions' pool/NATS defers with ordered `t.Cleanup` callbacks.
5. Preserve the dirty timestamp direction: one injected runtime timestamp for stable sync identity fixtures and response-time timestamps for freshness fixtures.
6. Execute the identical focused regression GREEN, then the adversarial cleanup test, all three QF stress flows, scenario-specific QF E2E canaries, broader E2E, and broader stress suite.

### Shared Infrastructure Impact Sweep

This scope changes a helper shared by three live stress tests. The impact sweep covers:

- `TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity`
- `TestQFDecisionsFreshnessSLAP95IngestRender`
- `TestQFDecisionsFreshnessSLAP95RenderAndCombined`
- PostgreSQL parent/child ownership across `artifacts`, `annotations`, `edges`, and `sync_state`
- NATS and PostgreSQL cleanup ordering
- current-run timestamp consistency and existing QF freshness gauges
- unrelated test rows running concurrently in the disposable stack

### Change Boundary

**Allowed file families:**

- `tests/stress/qf_decisions_sync_stress_test.go`
- `tests/stress/qf_decision_event_replay_test.go`
- `specs/041-qf-companion-connector/bugs/BUG-001-qf-stress-cleanup-integrity/**`

**Excluded surfaces:**

- all production code under `cmd/`, `internal/`, `ml/`, `web/`, and `extensions/`
- database migrations and configuration
- parent artifacts directly under `specs/041-qf-companion-connector/`
- deployment and operator-owned configuration
- every other repository, including `quantitativeFinance/`
- unrelated test files

### Test Plan

| ID | Test Type | Category | Scenario | Exact File | Exact Test | Command | Live System |
|---|---|---|---|---|---|---|---|
| QFS01-TP01 | RED/GREEN Stress Regression | `stress` | SCN-041-BUG001-002 | `tests/stress/qf_decisions_sync_stress_test.go` | `TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows` | `./smackerel.sh test stress` | Yes |
| QFS01-TP02 | Adversarial Stress Regression | `stress` | SCN-041-BUG001-003 | `tests/stress/qf_decisions_sync_stress_test.go` | `TestQFDecisionsStressCleanupReturnsErrorForClosedPool` | `./smackerel.sh test stress` | Yes |
| QFS01-TP03 | Sync Stress Regression | `stress` | SCN-041-BUG001-001, SCN-041-BUG001-004 | `tests/stress/qf_decisions_sync_stress_test.go` | `TestQFDecisionsSyncStress_RepeatedCursorPagesDoNotDuplicatePacketIdentity` | `./smackerel.sh test stress` | Yes |
| QFS01-TP04 | Freshness Stress Regression | `stress` | SCN-041-BUG001-001, SCN-041-BUG001-004 | `tests/stress/qf_decision_event_replay_test.go` | `TestQFDecisionsFreshnessSLAP95IngestRender` | `./smackerel.sh test stress` | Yes |
| QFS01-TP05 | Combined Freshness Stress Regression | `stress` | SCN-041-BUG001-001, SCN-041-BUG001-004 | `tests/stress/qf_decision_event_replay_test.go` | `TestQFDecisionsFreshnessSLAP95RenderAndCombined` | `./smackerel.sh test stress` | Yes |
| QFS01-TP06 | Regression E2E API | `e2e-api` | SCN-041-BUG001-001 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsConnectorHealthAppearsInLiveAPI` | `./smackerel.sh test e2e` | Yes |
| QFS01-TP07 | Broader E2E Regression | `e2e-api` | SCN-041-BUG001-004 | `tests/e2e/qf_decisions_connector_api_test.go` | `TestQFDecisionsIncompatibleCapabilityBlocksPolling`, `TestQFDecisionsConnectorIngestsUnknownDecisionTypeWithMetadata` | `./smackerel.sh test e2e` | Yes |
| QFS01-TP08 | Broader QF Stress Regression | `stress` | SCN-041-BUG001-001 through SCN-041-BUG001-004 | `tests/stress/qf_decisions_sync_stress_test.go`, `tests/stress/qf_decision_event_replay_test.go` | all QF decision sync/freshness stress tests in both files | `./smackerel.sh test stress` | Yes |

### Adversarial Regression Contract

The parent/child case seeds both owned and unowned rows. Reintroducing wrong cleanup order, a missing child-table delete, a source predicate error, or log-only failure handling must leave an observable owned count or error and fail the test. The closed-pool case must fail if the cleanup core returns nil. The control row prevents a broad delete from satisfying the zero-owned assertion by deleting everything.

### Definition of Done - Tiered Validation

#### Core Outcomes

- [ ] Root cause is confirmed by an executed RED regression before cleanup implementation changes.
- [ ] SCN-041-BUG001-001: All three affected stress functions use one deterministic `t.Cleanup` LIFO chain: data cleanup, NATS close, pool close.
- [ ] `qfDecisionsStressCleanup` uses bounded set-based child-before-parent deletion with no per-artifact delete loop.
- [ ] SCN-041-BUG001-003: Every cleanup failure reaches a contextual error and fails the test; no cleanup error path uses log-only continuation.
- [ ] SCN-041-BUG001-002: Parent/child proof observes zero owned parent/child rows and a surviving unowned control row.
- [ ] SCN-041-BUG001-004: Stable sync timestamps are run-scoped; freshness timestamps are response-scoped; the fixed `2026-05-06T00:00:00Z` fixture value is absent.
- [ ] Change Boundary is respected and zero excluded file families are changed.
- [ ] Rollback path is verified by restoring the two test files to their pre-fix content without touching database schema or production code.

#### Test Evidence Items - One Per Test Plan Row

- [ ] QFS01-TP01 RED failure and post-fix GREEN success are recorded for `TestQFDecisionsStressCleanupChildReturnsZeroOwnedRows`.
- [ ] QFS01-TP02 adversarial closed-pool cleanup error regression passes.
- [ ] QFS01-TP03 sync replay/identity stress regression passes.
- [ ] QFS01-TP04 ingest freshness stress regression passes.
- [ ] QFS01-TP05 render/combined freshness stress regression passes.
- [ ] QFS01-TP06 Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass for the QF connector canary.
- [ ] QFS01-TP07 Broader E2E regression suite passes for the exact QF capability and unknown-decision tests.
- [ ] QFS01-TP08 broader QF stress regression suite passes across both exact stress files.

#### Build Quality Gate

- [ ] Build Quality Gate passes: `./smackerel.sh check`, `./smackerel.sh lint`, and `./smackerel.sh format --check` are clean; artifact lint, traceability, regression-quality, and state-transition guards are recorded; no warnings, skipped required tests, defaults, unrelated edits, or stale documentation remain.

No checkbox may be checked until its current-session raw evidence is recorded in `report.md` with a `Claim Source` tag.
