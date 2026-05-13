# Scopes: NATS Production Hardening

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: ML sidecar reconnect contract

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-046-N01 ML sidecar survives NATS restart
  Given the ML sidecar is connected to NATS
  When NATS restarts during deployment operation
  Then the sidecar keeps reconnecting until NATS returns
  And embeddings and extraction workers resume without manual restart
```

### Implementation Plan

1. Identify the ML sidecar NATS client construction point.
2. Configure indefinite reconnect behavior with explicit interval settings.
3. Add a disposable-stack integration test that restarts NATS and verifies sidecar recovery.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-046-001 | unit | ML sidecar NATS config | SCN-046-N01 | Client options include indefinite reconnect. |
| T-046-002 | integration | disposable NATS stack | SCN-046-N01 | Sidecar resumes work after NATS restart. |

### Definition of Done

- [ ] T-046-001 passes and proves ML client reconnect attempts are indefinite.
- [ ] T-046-002 passes and proves reconnect behavior against a disposable NATS restart.

## Scope 2: NATS server and stream storage caps

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-046-N02 NATS server limits are explicit
  Given generated runtime configuration is inspected
  When NATS service settings are rendered
  Then max_payload, max_file_store, and max_mem_store are present
  And missing values fail configuration validation

Scenario: SCN-046-N03 Streams cannot grow without bound
  Given Smackerel creates JetStream streams
  When the stream configuration is inspected
  Then each stream has a MaxBytes cap and bounded retention policy
```

### Implementation Plan

1. Add SST-backed NATS limit values.
2. Generate NATS server configuration with `max_payload`, `max_file_store`, and `max_mem_store`.
3. Inventory stream creation and set `MaxBytes` on each stream.
4. Add config and stream inspection tests.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-046-003 | unit/config | config generator tests | SCN-046-N02 | Missing NATS limit values fail validation. |
| T-046-004 | integration | JetStream setup tests | SCN-046-N03 | Every stream has MaxBytes and retention set. |
| T-046-005 | stress | disposable NATS stack | SCN-046-N03 | Message burst respects stream caps without unbounded disk growth. |
| T-046-006 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] T-046-003 passes and proves NATS server limits are required.
- [ ] T-046-004 passes and proves every stream is capped.
- [ ] T-046-005 passes against disposable NATS state.
- [ ] T-046-006 passes and this planning packet remains lint-clean.
