# Feature: ML Sidecar Health Isolation

## Status

Resolved — implemented 2026-05-15 (matches `state.json::status` and `state.json::certification.status` = `done`)

## Review Finding

- STB-004: CPU-bound embedding work can starve ML sidecar health checks unless worker isolation or thread-pool control is planned and tested.

## Outcome Contract

**Intent:** Ensure ML sidecar health endpoints remain responsive under embedding and extraction load.

**Success Signal:** During CPU-bound embedding load, `/health` and readiness checks return within the defined SLA, worker concurrency is bounded by configuration, and regression tests fail if health handling shares a starved execution path.

**Hard Constraints:**

- Concurrency limits must be explicit and configuration-owned.
- Health checks must not depend on the saturated embedding queue.
- Tests must exercise realistic CPU-bound load without relying on sleeps as proof.

**Failure Condition:** Embedding load can block health responses long enough for the runtime or orchestrator to mark a healthy sidecar as dead.

## Requirements

- **FR-050-001:** ML sidecar MUST isolate health request handling from embedding and extraction CPU-bound workers.
- **FR-050-002:** Worker concurrency MUST be bounded by explicit configuration.
- **FR-050-003:** Health endpoints MUST have a documented latency SLA under load.
- **FR-050-004:** Integration or stress tests MUST prove health responsiveness during embedding load.
- **FR-050-005:** Observability MUST expose worker queue pressure or active worker counts.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-050-H01 Health remains responsive during embedding load
  Given the ML sidecar is processing CPU-bound embedding requests
  When the operator or health checker calls the health endpoint
  Then the endpoint responds within the configured SLA
  And the response does not wait behind the embedding queue

Scenario: SCN-050-H02 Worker concurrency is bounded
  Given embedding requests exceed the configured worker count
  When the sidecar accepts work
  Then active CPU-bound workers do not exceed the configured limit
  And excess work remains queued or rejected according to the contract
```

## Product Principle Alignment

This spec supports Principle 6 by preventing noisy false failures during heavy work, and Principle 8 by making health and queue pressure observable.

## Non-Goals

- Replacing the ML sidecar architecture.
- Changing model output semantics.
- Adding a user-facing ML control panel.
