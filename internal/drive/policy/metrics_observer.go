package policy

import "github.com/smackerel/smackerel/internal/metrics"

// MetricsObserver implements Observer by emitting one
// smackerel_drive_policy_decisions_total counter increment per verdict.
// The metric labels match the Verdict fields so dashboards can rebuild
// the decision table from output alone.
type MetricsObserver struct{}

// NewMetricsObserver constructs a MetricsObserver.
func NewMetricsObserver() *MetricsObserver { return &MetricsObserver{} }

// Observe increments the decision counter for v.
func (o *MetricsObserver) Observe(v Verdict) {
	metrics.DrivePolicyDecisionsTotal.WithLabelValues(
		string(v.Surface),
		string(v.Decision),
		string(v.Sensitivity),
	).Inc()
}
