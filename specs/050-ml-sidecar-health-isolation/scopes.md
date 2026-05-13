# Scopes: ML Sidecar Health Isolation

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Worker pool and health-path isolation

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

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

### Implementation Plan

1. Add explicit worker concurrency configuration.
2. Isolate health routes from the worker queue.
3. Add active worker and queue metrics.
4. Add load tests that measure health latency under CPU-bound embedding load.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-050-001 | unit | ML config tests | SCN-050-H02 | Invalid or missing worker concurrency fails validation. |
| T-050-002 | integration | ML sidecar runtime tests | SCN-050-H01 | Health responds within SLA during embedding load. |
| T-050-003 | stress | ML sidecar stress tests | SCN-050-H01 | Sustained load does not starve health route. |
| T-050-004 | metrics | metrics tests | SCN-050-H02 | Active worker and queue pressure metrics exist. |
| T-050-005 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] T-050-001 passes and proves worker concurrency is explicit and validated.
- [ ] T-050-002 passes and proves health stays responsive under embedding load.
- [ ] T-050-003 passes and proves sustained CPU pressure does not starve health checks.
- [ ] T-050-004 passes and proves worker/queue metrics are emitted.
- [ ] T-050-005 passes and this planning packet remains lint-clean.
