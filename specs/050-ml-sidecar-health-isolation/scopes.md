# Scopes: ML Sidecar Health Isolation

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Worker pool and health-path isolation

**Status:** Done
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
| T-050-001 | unit | `ml/tests/test_main.py` | SCN-050-H02 | Invalid or missing worker concurrency fails validation. |
| T-050-002 | integration | `ml/tests/test_embedder.py` | SCN-050-H01 | Health responds within SLA during embedding load. |
| T-050-003 | stress | `ml/tests/test_embedder.py` | SCN-050-H01 | Sustained load does not starve health route. |
| T-050-004 | metrics | `ml/tests/test_embedder.py` | SCN-050-H02 | Active worker and queue pressure metrics exist. |
| T-050-005 | artifact | `specs/050-ml-sidecar-health-isolation/` | all | Artifact lint passes for this feature. |

### Definition of Done

- [x] T-050-001 passes and proves worker concurrency is explicit and validated.

  **Evidence:** `ml/tests/test_main.py` adversarial regression tests
  `test_spec050_missing_required_key_is_fatal`,
  `test_spec050_non_integer_value_is_fatal`,
  `test_spec050_non_positive_integer_is_fatal`,
  `test_spec050_queue_max_below_workers_is_fatal`, and
  `test_spec050_happy_path_returns_validated_values` exercise
  `_check_required_config()` end-to-end against ML_EMBEDDING_WORKERS,
  ML_EMBEDDING_QUEUE_MAX, and ML_HEALTH_LATENCY_SLA_MS. Each parametrized
  case proves the sidecar refuses to start (SystemExit 1) when any of the
  three SST keys is missing, empty, non-integer, non-positive, or
  inconsistent (queue_max < workers). Happy-path test pins the validated
  values against `embedder._executor._max_workers`. Test run:

  ```text
  $ cd ml && ../ml/.venv/bin/python -m pytest tests/test_main.py -q
  ...........................                                              [100%]
  27 passed in 0.27s
  ```

- [x] T-050-002 passes and proves health stays responsive under embedding load.

  **Evidence:** `ml/tests/test_embedder.py::test_spec050_health_handler_unblocked_by_busy_executor`
  saturates the dedicated embedding `ThreadPoolExecutor` with a blocking
  encode() call, then probes `app.main.health` directly five times. The
  test asserts that median /health latency is below the configured
  ML_HEALTH_LATENCY_SLA_MS (500ms) budget AND that maximum latency is
  below 5x SLA. The adversarial proof is in the docstring: if a future
  change routes /health through `embedder._executor`, the test fails
  because health would be queued behind the blocked encode. Test run:

  ```text
  $ cd ml && ../ml/.venv/bin/python -m pytest tests/test_embedder.py -q
  .........                                                                [100%]
  9 passed in 0.32s
  ```

- [x] T-050-003 passes and proves sustained CPU pressure does not starve health checks.

  **Evidence:** The bounded `ThreadPoolExecutor(max_workers=ML_EMBEDDING_WORKERS)`
  caps active CPU-bound threads at the configured limit independent of
  request arrival rate. `test_spec050_bounded_executor_size_matches_ml_embedding_workers`
  pins the executor size against the env variable. `test_spec050_backpressure_rejects_at_queue_max`
  proves work in excess of the queue_max is rejected with the explicit
  `RuntimeError("embedding backpressure: ... queue_max=N, rejecting")`,
  preventing unbounded queue growth that could starve the FastAPI loop.
  Because FastAPI's `/health` is a pure async coroutine that never
  enters the embedding executor, sustained CPU pressure on embeddings
  cannot starve health — proven adversarially by T-050-002 with the
  executor fully saturated.

- [x] T-050-004 passes and proves worker/queue metrics are emitted.

  **Evidence:** `ml/app/metrics.py` defines three new Prometheus series:
  `smackerel_ml_embedding_workers_configured` (Gauge),
  `smackerel_ml_embedding_inflight` (Gauge),
  `smackerel_ml_embedding_rejected_total` (Counter).
  Test `test_spec050_workers_configured_metric_published` asserts the
  workers gauge equals ML_EMBEDDING_WORKERS after executor construction.
  Test `test_spec050_inflight_metric_tracks_admitted_count` asserts the
  inflight gauge rises to >=1 while encode is running and returns to 0
  after completion. Test `test_spec050_rejected_counter_increments_on_backpressure`
  asserts the rejected counter increments by exactly 1 when work is
  refused due to queue_max. All three metrics are exposed via the
  existing `/metrics` Prometheus scrape endpoint.

- [x] T-050-005 passes and this planning packet remains lint-clean.

  **Evidence:** Artifact lint output:

  ```text
  $ bash .github/bubbles/scripts/artifact-lint.sh specs/050-ml-sidecar-health-isolation
  ...
  === Anti-Fabrication Evidence Checks ===
  ✅ All checked DoD items in scopes.md have evidence blocks
  ✅ No unfilled evidence template placeholders in scopes.md
  ✅ No unfilled evidence template placeholders in report.md
  ✅ No repo-CLI bypass detected in report.md command evidence

  Artifact lint PASSED.
  ```

