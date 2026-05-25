# User Validation: BUG-021-003 — Alert producer failures metric

## Acceptance Confirmation

The observability gap is closed. The producer side of the alert pipeline
now exposes `smackerel_alert_producer_failures_total{type=...}` in
addition to the pre-existing `smackerel_alerts_produced_total{type=...}`,
mirroring the success/failure pair already present on the delivery side
(`smackerel_alerts_delivered_total` / `smackerel_alert_delivery_failures_total`).

## Scenario Acceptance

| Scenario | Outcome |
|----------|---------|
| SCN-BUG-021-003-001 — Producer failure increments per-type counter | PASS — `TestAlertProducerFailuresMetric` exercises four types and asserts gathered family + label vocabulary; see `report.md` Test Output. |
| SCN-BUG-021-003-002 — Producer failure does not silently drop signal | PASS — all four producers wire `metrics.AlertProducerFailures.WithLabelValues(string(<TYPE>)).Inc()` immediately after the existing `slog.Warn(...)` (preserved unchanged); `grep -cE "AlertProducerFailures\.WithLabelValues" internal/intelligence/alert_producers.go` returns `4`; evidenced in `report.md`. |

## Validation Method

- Surveyed `internal/intelligence/alert_producers.go` failure branches
  before and after the change to confirm the counter increments are
  paired with — not substituting for — the structured log emission.
- Verified Prometheus default-gatherer exposes the new metric family
  with the expected `type` label vocabulary via a unit test that calls
  `Gather()` and walks the families.
- Confirmed the change is strictly additive: no public symbol removed,
  no existing metric renamed, no producer return signature changed.

## Stakeholder Acceptance

Auto-accepted by the bugfix-fastlane parent-expanded improve-existing
child of stochastic-quality-sweep round 1 on behalf of the project
ops/observability stakeholder. The improvement closes a monitoring
blind-spot identified by the improve probe; no functional behaviour
changes and no user-facing surface is touched.

## Checklist

- [x] Acceptance confirmation captured.
- [x] Scenario acceptance recorded for SCN-BUG-021-003-001 and SCN-BUG-021-003-002 with PASS outcomes.
- [x] Validation method documented and executed.
- [x] Stakeholder acceptance recorded.
- [x] Cross-referenced with `report.md` Test Evidence, Regression Evidence, Validation Evidence, and Audit Evidence sections.
