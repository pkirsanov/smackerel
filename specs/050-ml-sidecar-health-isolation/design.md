# Design: ML Sidecar Health Isolation

## Current Truth

The ML sidecar performs CPU-bound embedding work. The readiness review flagged that health endpoints need isolation so a busy worker pool cannot starve health checks.

## Proposed Design

### Request Isolation

- Keep health routes on a fast request path that does not wait behind embedding work.
- Execute embedding and extraction work through an explicit worker pool or executor.
- Bound worker concurrency through generated configuration.

### SLA and Metrics

- Define health response SLA under load.
- Expose active worker count, queue depth, and rejected work count metrics.

### Test Harness

- Create a deterministic CPU-bound workload fixture for the sidecar.
- Fire concurrent embedding requests and health requests.
- Assert health latency stays below SLA and active workers stay bounded.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-050-001 | unit | Worker pool accepts configured concurrency and rejects invalid values. |
| T-050-002 | integration | Health endpoint responds within SLA during embedding load. |
| T-050-003 | stress | Sustained embedding pressure does not starve health route. |
| T-050-004 | metrics | Active worker and queue pressure metrics are emitted. |
| T-050-005 | artifact | Artifact lint passes for this feature. |

## Risk Controls

- Avoid unbounded worker creation.
- Avoid tests that pass by sleeping rather than measuring latency.
- Keep health checks lightweight and free of model calls.
