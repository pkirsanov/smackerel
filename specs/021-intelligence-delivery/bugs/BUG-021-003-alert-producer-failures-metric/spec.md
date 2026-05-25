# Bug: BUG-021-003 — Alert producers silently drop CreateAlert failures (no observability metric)

## Classification

- **Type:** Improvement / Observability gap — producer-side failure path has no counter, asymmetric with delivery-side `smackerel_alert_delivery_failures_total`
- **Severity:** LOW (no functional defect; monitoring blind-spot only)
- **Parent Spec:** 021 — Intelligence Delivery
- **Workflow Mode:** bugfix-fastlane (parent-expanded child of stochastic-quality-sweep round 1, trigger `improve`, mapped mode `improve-existing`)
- **Status:** Open — discovered by improve R1 (sweep `sweep-2026-05-25-r10`)

## Problem Statement

`internal/intelligence/alert_producers.go` defines four alert producers
(`ProduceBillAlerts`, `ProduceTripPrepAlerts`, `ProduceReturnWindowAlerts`,
`ProduceRelationshipCoolingAlerts`). When the per-row `CreateAlert` call
inside each producer fails (DB write conflict, pool exhaustion, deadline
exceeded, etc.), the failure is recorded with `slog.Warn(...)` only — no
Prometheus counter is incremented:

```go
if err := e.CreateAlert(ctx, &Alert{...}); err != nil {
    slog.Warn("failed to create bill alert", "subscription", serviceName, "error", err)
} else {
    metrics.AlertsProduced.WithLabelValues(string(AlertBill)).Inc()
    created++
}
```

The successful-create path increments `smackerel_alerts_produced_total{type}`
but the failure path emits zero observable signal beyond a warning log line.

This creates an observability asymmetry with the delivery side of the same
pipeline. The delivery loop in `internal/scheduler/jobs.go::deliverAlertBatch`
already has BOTH success and failure counters:

```go
metrics.AlertDeliveryFailures.Inc()              // failure counter exists
metrics.AlertsDelivered.WithLabelValues(...).Inc() // success counter exists
```

Producer-side has only the success counter. Monitoring cannot alert on
"producer health degrading" — the only way to detect repeated `CreateAlert`
failures is to scrape and parse logs, which defeats the purpose of having
the Prometheus pipeline.

## Impact

| Axis | Impact |
|------|--------|
| **Observability** | Producer failure rate is invisible to Prometheus / Grafana / alerting. A misbehaving producer (DB write failures, schema drift, constraint violations) silently emits zero alerts and zero failure counts. |
| **Reliability** | A production regression that causes `CreateAlert` to fail for one type (e.g., a JSON encoding error specific to `relationship_cooling`) would manifest as "no alerts of that type ever delivered" with no leading indicator. |
| **Sibling-pattern consistency** | The delivery side has the success/failure pair (`AlertsDelivered`/`AlertDeliveryFailures`). The producer side has only `AlertsProduced`. Restoring symmetry makes the pipeline easier to reason about for ops. |
| **Severity** | LOW — no functional defect, no user-facing impact, no data loss. Pure monitoring blind-spot. |

## Why this is "improve" not "harden" or "stabilize"

Per `improve-existing` charter: API ergonomics / observability gap / structural
consistency improvement on an already-correct, already-hardened, already-stabilized
surface. The four producers are not buggy — they handle context cancellation,
deduplicate via SQL `NOT EXISTS`, scan errors `continue` to the next row, and
return wrapped row-iteration errors. The improvement is purely additive: emit
a per-type failure counter so monitoring can see producer health the same way
it sees delivery health.

## Why prior improve rounds missed it

- **R7 simplify (2026-05-13)** — focused on dead-code removal and import-graph
  simplification. Did not survey metric coverage symmetry.
- **R13 stabilize (2026-05-23)** — focused on uncached health-handler DB load
  (closed as BUG-021-002). Did not survey producer-side observability.
- **R6 harden (2026-05-12)** — focused on artifact-integrity governance gates
  (G041, G016, G026, etc.). Did not survey runtime metric coverage.

R1 improve (this round) extends the lens to "Prometheus metric symmetry between
sibling pipeline halves" and discovered the producer-side gap.

## Reproduction (pre-fix)

```text
$ grep -nE "metrics\.Alert(Producer|sProduced|sDelivered|DeliveryFailures)" internal/intelligence/alert_producers.go internal/scheduler/jobs.go
internal/intelligence/alert_producers.go:103: metrics.AlertsProduced.WithLabelValues(string(AlertBill)).Inc()
internal/intelligence/alert_producers.go:174: metrics.AlertsProduced.WithLabelValues(string(AlertTripPrep)).Inc()
internal/intelligence/alert_producers.go:236: metrics.AlertsProduced.WithLabelValues(string(AlertReturnWindow)).Inc()
internal/intelligence/alert_producers.go:296: metrics.AlertsProduced.WithLabelValues(string(AlertRelationship)).Inc()
internal/scheduler/jobs.go:459: metrics.AlertDeliveryFailures.Inc()
internal/scheduler/jobs.go:471: metrics.AlertDeliveryFailures.Inc()
internal/scheduler/jobs.go:477: metrics.AlertsDelivered.WithLabelValues(string(a.AlertType)).Inc()

$ grep -nE "AlertProducerFailures" internal/metrics/metrics.go
# (no matches — counter does not exist)
```

The asymmetry is concrete: 4 success increments on the producer side, 0 failure
increments. 2 failure increments on the delivery side, 1 success increment.

## Acceptance Criteria

- [ ] `internal/metrics/metrics.go` defines `AlertProducerFailures` as a
      `prometheus.NewCounterVec` with metric name
      `smackerel_alert_producer_failures_total` and a single `type` label,
      mirroring the `AlertsProduced` labelling convention.
- [ ] `AlertProducerFailures` is added to the `prometheus.MustRegister(...)`
      block in the package `init()`.
- [ ] Each of the four producers in `internal/intelligence/alert_producers.go`
      increments `AlertProducerFailures.WithLabelValues(string(<AlertType>))`
      in the `if err := e.CreateAlert(...); err != nil` branch — paired with
      the existing `slog.Warn(...)` call, not replacing it.
- [ ] A new unit test in `internal/metrics/metrics_test.go` exercises
      `AlertProducerFailures.WithLabelValues(...).Inc()` for at least two
      alert types and asserts the gathered family
      `smackerel_alert_producer_failures_total` is present and has
      `value >= 1` for each labelled type.
- [ ] `go test -count=1 -race ./internal/intelligence/... ./internal/metrics/... ./internal/scheduler/... ./internal/api/...` PASS.
- [ ] `go vet ./...` and `go build ./...` clean.
- [ ] `artifact-lint.sh` and `traceability-guard.sh` PASS for the parent spec
      and this bug folder.
- [ ] Pre-existing `AlertsProduced` increments and `slog.Warn` log lines are
      preserved unchanged — the change is strictly additive.

## Non-Goals

- Refactoring the four producers into a shared helper (separate potential
  improvement; out of scope for this surgical observability fix).
- Adding a `reason` label vocabulary for the failure counter (deferred until
  ops feedback indicates differentiation between `pool_exhausted`, `constraint_violation`,
  etc. is operationally useful).
- Touching `internal/scheduler/jobs.go::deliverAlertBatch` — the delivery
  side already has the success/failure pair.
- Changing producer return signatures from `error` to `(int, error)`
  (separate ergonomics consideration; no current consumer needs the count).

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-BUG-021-003-001 Producer failure increments per-type counter
  Given the AlertProducerFailures counter has been registered
  When the unit test invokes AlertProducerFailures.WithLabelValues("bill").Inc()
  Then the gathered family "smackerel_alert_producer_failures_total" exposes a
       counter with label type="bill" and value >= 1

Scenario: SCN-BUG-021-003-002 Producer failure does not silently drop signal
  Given the AlertProducerFailures counter is registered
  And ProduceBillAlerts encounters a CreateAlert failure for one row
  Then the failure path increments AlertProducerFailures with the "bill" type
  And the slog.Warn log line for the failure is still emitted
```

## Acceptance Test References

| Scenario | Test Function | File |
|----------|---------------|------|
| SCN-BUG-021-003-001 | `TestAlertProducerFailuresMetric` | `internal/metrics/metrics_test.go` |
| SCN-BUG-021-003-002 | `TestAlertProducerFailuresMetric` | `internal/metrics/metrics_test.go` (counter wired in `internal/intelligence/alert_producers.go` failure branches, surveyed via `grep`) |
