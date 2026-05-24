# Scopes: BUG-CHAOS-20260524-001

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. Scope 1 - Make ntfy dead-letter replay side-effect idempotent before `SourceEventSink` submission and preserve redacted replay responses.

### New Types & Signatures

- Store behavior: `ReplayDeadLetter` must return an existing accepted replay result for repeated requests after the first accepted replay.
- API behavior: repeated replay API calls expose already-replayed state without raw payload bytes.
- Regression scenario: `SCN-BUG-CHAOS-20260524-001-001` protects chaos seed 20260524 replay burst idempotency.

### Validation Checkpoints

- Integration checkpoint: `./smackerel.sh test integration --go-run 'TestNtfyDeadLetterReplayBurstIsIdempotent'`.
- Live E2E checkpoint: `./smackerel.sh test e2e --go-run 'TestNtfyDeadLetterReplayAPIIsIdempotent'`.
- Boundary checkpoint: `./smackerel.sh test unit --go --go-run 'TestNtfyAdapterHasNoOutputChannelImports' --verbose`.
- Closure checkpoint: focused ntfy integration, E2E, and stress selectors plus chaos seed 20260524 closure rerun recorded in [report.md](report.md).

## Scope Summary

| Scope | Surfaces | Required Tests | DoD Summary | Status |
|-------|----------|----------------|-------------|--------|
| 1. Make ntfy Dead-Letter Replay Side-Effect Idempotent | ntfy replay store, replay API, source-sink boundary, redaction-state scan errors, integration/E2E regressions | unit, integration, e2e-api, unit/static, stress selector | replay side effects bounded, redaction-state decode errors propagated, regression E2E recorded, no output-channel coupling, change boundary respected | Done |

## Scope 1: Make ntfy Dead-Letter Replay Side-Effect Idempotent

**Status:** Done

Depends On: spec 055 ntfy dead-letter replay implementation and SEC-055-001 redacted dead-letter API fix remain present.

### Outcome

Repeated replay of the same ntfy dead-letter is idempotent before `SourceEventSink` side effects. Duplicate operator submits, retries, or concurrent replays cannot create multiple raw notification events from one dead-letter.

### Gherkin Scenarios

```gherkin
Feature: BUG-CHAOS-20260524-001 ntfy replay burst side-effect idempotency
  Scenario: SCN-BUG-CHAOS-20260524-001-001 repeated ntfy dead-letter replay does not duplicate source events
    Given an ntfy dead-letter is replay eligible
    And the dead-letter has not yet been replayed
    When an operator or retry path submits replay for the same dead-letter three times
    Then at most one replay attempt reaches SourceEventSink
    And at most one raw notification event is created for that dead-letter replay
    And later attempts return an already-replayed or existing-attempt result
    And no direct output-channel delivery is created by the ntfy adapter

  Scenario: SCN-BUG-CHAOS-20260524-001-002 malformed ntfy redaction state is not silently discarded
    Given persisted ntfy subscription-state or dead-letter row data contains malformed redaction_state bytes
    When the ntfy store scan helper reconstructs the row
    Then the helper returns a contextual decode error
    And no empty redaction map is fabricated for operator-facing state
```

### Implementation Plan

| Area | Work |
|------|------|
| Replay service | Move replay idempotency/claim checks before `SourceEventSink.SubmitSourceEvent`. |
| Store | Lock or atomically claim the dead-letter replay attempt so only one caller can execute the sink side effect. |
| API | Return explicit already-replayed/existing-attempt semantics without exposing raw payload bytes. |
| Tests | Add permanent integration and live API regression tests derived from chaos seed 20260524. |
| Boundary | Keep replay routed through `SourceEventSink`; do not add direct output-channel dispatch. |

### Change Boundary

| Boundary | Included |
|----------|----------|
| Allowed file families | ntfy replay store code, ntfy replay integration tests, ntfy live API E2E tests, no-output-coupling guard, this bug packet's artifacts. |
| Excluded surfaces | output channel dispatchers, unrelated notification sources, parent spec completion status, generated config, deploy/compose files, and unrelated specs. |
| Containment proof | Report evidence records no-output-coupling guard, focused ntfy selectors, and closure governance checks. This planning repair changes only bug-packet artifacts. |

### Implementation Files

- `internal/notification/source/ntfy/store.go`
- `internal/notification/source/ntfy/store_scan_test.go`
- `internal/notification/source/ntfy/replay_integration_test.go`
- `tests/e2e/notification_ntfy_source_api_test.go`
- `internal/notification/source/ntfy/no_output_coupling_test.go`
- `tests/stress/notification_ntfy_source_stress_test.go`

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Description | Command | Live System |
|----|-----------|----------|---------------|------------------|-------------|---------|-------------|
| T-BUG-CHAOS-001-INT | Integration Regression | `integration` | `internal/notification/source/ntfy/replay_integration_test.go` | SCN-BUG-CHAOS-20260524-001-001 | Replaying the same replay-eligible dead-letter three times creates exactly one source-sink raw event and one accepted replay side effect. | `./smackerel.sh test integration --go-run 'TestNtfyDeadLetterReplayBurstIsIdempotent'` | Yes |
| T-BUG-CHAOS-001-E2E | Regression E2E | `e2e-api` | `tests/e2e/notification_ntfy_source_api_test.go` | SCN-BUG-CHAOS-20260524-001-001 | Regression: repeated replay API calls for one dead-letter do not duplicate raw/normalized records and return explicit already-replayed state. | `./smackerel.sh test e2e --go-run 'TestNtfyDeadLetterReplayAPIIsIdempotent'` | Yes |
| T-BUG-CHAOS-001-UNIT-REDSTATE | Unit Regression | `unit` | `internal/notification/source/ntfy/store_scan_test.go` | SCN-BUG-CHAOS-20260524-001-002 | Malformed subscription-state and dead-letter `redaction_state` bytes return contextual decode errors instead of fabricated empty maps. | `./smackerel.sh test unit --go --go-run 'TestNtfyScan'` | No |
| T-BUG-CHAOS-001-UNIT | Static Boundary Guard | `unit` | `internal/notification/source/ntfy/no_output_coupling_test.go` | SCN-BUG-CHAOS-20260524-001-001 | ntfy adapter remains free of output-channel imports after replay refactor. | `./smackerel.sh test unit --go --go-run 'TestNtfyAdapterHasNoOutputChannelImports' --verbose` | No |
| T-BUG-CHAOS-001-STRESS | Focused Stress Regression | `stress` | `tests/stress/notification_ntfy_source_stress_test.go` | SCN-BUG-CHAOS-20260524-001-001 | Focused ntfy malformed/reconnect resilience remains bounded after replay idempotency repair. | `./smackerel.sh test stress --go-run 'TestNtfyMalformedReconnectAndDuplicateBurstCreatesBoundedOperationalRecords\|TestNtfyConfigValidationBurstDoesNotFabricateConnectedHealth'` | Yes |

### Definition of Done

- [x] SCN-BUG-CHAOS-20260524-001-001 root cause is identified and fixed before source-sink replay side effects. Evidence: `report.md#implementation-evidence`, `report.md#deterministic-red-evidence`, `report.md#green-evidence`.
- [x] SCN-BUG-CHAOS-20260524-001-001 permanent integration regression test covers repeated replay side-effect idempotency. Evidence: `report.md#replay-burst-integration-regression`.
- [x] SCN-BUG-CHAOS-20260524-001-001 original chaos reproduction trace is represented by failing-then-passing evidence. Evidence: `report.md#deterministic-red-evidence`, `report.md#replay-burst-integration-regression`, `report.md#seeded-replay-side-effect-closure-rerun`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior pass. Evidence: `report.md#replay-api-e2e-regression`, `report.md#live-replay-api-idempotency-rerun`.
- [x] Broader E2E regression suite passes. Evidence: `report.md#focused-ntfy-regression-selectors`, `report.md#live-replay-api-idempotency-rerun`.
- [x] No-output-coupling guard passes and the adapter does not dispatch output channels directly. Evidence: `report.md#no-output-coupling-unit-guard`.
- [x] Raw terminal output of each originally failing or newly added regression test now passing is recorded in [report.md](report.md). Evidence: `report.md#deterministic-red-evidence`, `report.md#replay-burst-integration-regression`, `report.md#replay-api-e2e-regression`.
- [x] Focused ntfy integration, E2E, and stress selectors pass without replay regressions. Evidence: `report.md#focused-ntfy-regression-selectors`, `report.md#focused-resilience-stress-rerun`.
- [x] Reproduction recipe from chaos trace is re-executed and no longer creates duplicate raw events. Evidence: `report.md#seeded-replay-side-effect-closure-rerun`.
- [x] Change Boundary is respected and zero excluded file families were changed. Evidence: `report.md#code-diff-evidence`, `report.md#no-output-coupling-unit-guard`, `report.md#closure-governance-evidence`.
- [x] SCN-BUG-CHAOS-20260524-001-002 malformed ntfy `redaction_state` decode errors are propagated with contextual source/topic or dead-letter/source identity. Evidence: `report.md#audit-rework-red-evidence`, `report.md#audit-rework-unit-regression`.
- [x] SCN-BUG-CHAOS-20260524-001-002 G048 silent decode scan no longer finds ignored `json.Unmarshal` in ntfy store scan helpers. Evidence: `report.md#audit-rework-g048-scan`, `report.md#audit-rework-implementation-reality-scan`.
