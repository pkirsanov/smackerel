# Design: BUG-021-003 — Alert producer failures metric

## Approach

Add one Prometheus `CounterVec` named `AlertProducerFailures` with a single
`type` label, register it alongside the other alert metrics, and increment it
in the four existing `CreateAlert` failure branches inside
`internal/intelligence/alert_producers.go`. The change is purely additive:
no existing log line, metric, or control-flow path is modified.

## Symmetry Map

| Pipeline Half | Success Metric | Failure Metric |
|---------------|----------------|----------------|
| Delivery (`internal/scheduler/jobs.go::deliverAlertBatch`) | `AlertsDelivered` (CounterVec, label `type`) | `AlertDeliveryFailures` (Counter, no label) |
| Production (`internal/intelligence/alert_producers.go`, 4 producers) | `AlertsProduced` (CounterVec, label `type`) | **(missing — this bug)** `AlertProducerFailures` (CounterVec, label `type`) |

The production-side failure metric is labelled by `type` because each of the
four producers runs as a separate cron-job invocation and knows the alert
type up-front at compile-time, making per-type alerting cheap and useful.
The delivery side already operates on a heterogeneous batch and is
intentionally aggregated.

## Changes

### `internal/metrics/metrics.go` (additive)

Add immediately after the existing `AlertsProduced` declaration block
(currently lines 148-156):

```go
// AlertProducerFailures counts alert-producer CreateAlert failures by type
// (BUG-021-003 — improve R1 observability symmetry with AlertDeliveryFailures).
var AlertProducerFailures = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_alert_producer_failures_total",
		Help: "Alert-producer CreateAlert failures by type",
	},
	[]string{"type"},
)
```

Add `AlertProducerFailures,` to the `prometheus.MustRegister(...)` block in
`init()` (currently lines 440-470, immediately after `AlertsProduced,`).

### `internal/intelligence/alert_producers.go` (additive)

Inside each of the four producers, in the `if err := e.CreateAlert(...); err != nil`
branch, insert `metrics.AlertProducerFailures.WithLabelValues(string(<TYPE>)).Inc()`
immediately after the existing `slog.Warn(...)` call:

- `ProduceBillAlerts` → `AlertBill`
- `ProduceTripPrepAlerts` → `AlertTripPrep`
- `ProduceReturnWindowAlerts` → `AlertReturnWindow`
- `ProduceRelationshipCoolingAlerts` → `AlertRelationship`

Example for the bill producer:

```go
if err := e.CreateAlert(ctx, &Alert{...}); err != nil {
    slog.Warn("failed to create bill alert", "subscription", serviceName, "error", err)
    metrics.AlertProducerFailures.WithLabelValues(string(AlertBill)).Inc() // NEW
} else {
    metrics.AlertsProduced.WithLabelValues(string(AlertBill)).Inc()
    created++
}
```

### `internal/metrics/metrics_test.go` (additive)

Add a new test `TestAlertProducerFailuresMetric` that exercises the counter
for two alert types and asserts the gathered family is present with values
`>= 1`. Pattern mirrors the existing `TestAlertDeliveryMetrics` test
(lines 408-444).

## Non-Changes

- No producer control flow change. The `slog.Warn` log line, the `continue`-via-fall-through
  semantics, and the row-iteration error wrapping are all preserved.
- No new SST config knob. The metric is registered unconditionally; Prometheus
  scrape configuration is upstream of this package.
- No change to `internal/scheduler/jobs.go::deliverAlertBatch` (delivery half).
- No change to `AlertsProduced` semantics or labels.
- No change to producer function signatures.

## Backwards Compatibility

- Adding a new Prometheus metric is strictly additive for scrapers — existing
  dashboards continue to function unchanged.
- The new metric name `smackerel_alert_producer_failures_total` does not
  collide with any existing metric (verified via
  `grep -nE "alert_producer.*failures" internal/metrics/metrics.go` — zero matches).

## Test Plan

| Scenario | Test Function | File | Type |
|----------|---------------|------|------|
| SCN-BUG-021-003-001 Producer failure increments per-type counter | `TestAlertProducerFailuresMetric` | `internal/metrics/metrics_test.go` | unit |
| SCN-BUG-021-003-002 Producer failure does not silently drop signal | `TestAlertProducerFailuresMetric` (counter wired in producer failure branches; wiring verified by grep) | `internal/metrics/metrics_test.go` + survey | unit + structural survey |
| Regression: `internal/intelligence` package compiles after import-line addition | `go build ./internal/intelligence/...` | n/a | build |
| Regression: existing alert producer unit tests | `TestClampDay_*`, `TestCalendarDaysBetween_*`, `TestBillingTitleFormat_*`, `TestBillingDaysUntilRange`, `TestMonthlyBillingRollover`, `TestMonthlyBillingDecemberRollover`, `TestAnnualBillingDate*` | `internal/intelligence/alert_producers_test.go` | unit |
| Regression: existing delivery metrics test | `TestAlertDeliveryMetrics` | `internal/metrics/metrics_test.go` | unit |
| Regression: scheduler tests cover `deliverAlertBatch` paths | `TestDeliverAlertBatch_*` | `internal/scheduler/jobs_test.go` | unit |

## Risk

- **Risk:** Counter registration race — Prometheus `MustRegister` panics on
  duplicate registration. **Mitigation:** the new counter is added inside the
  same `init()` registration block as the other alert metrics; the package
  has exactly one `init` invocation and the metric name is unique.
- **Risk:** Test pollution across parallel test runs in the same process.
  **Mitigation:** the new test uses the same `prometheus.DefaultGatherer.Gather()`
  introspection pattern as the existing `TestAlertDeliveryMetrics` (which
  already runs in this package without isolation issues).

## Observability

- New metric exported on `/metrics` endpoint:
  `smackerel_alert_producer_failures_total{type="bill|trip_prep|return_window|relationship_cooling"}`.
- Existing log lines unchanged.
- No new structured-log fields, no new trace spans, no new histograms.
