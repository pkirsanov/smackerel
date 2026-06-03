package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// --- Spec 021 Scope 4 — Unified Surfacing Controller ---
//
// All seven metrics use bounded label sets per the M1a design notes:
// `producer` is an enum from internal/intelligence/surfacing.Producer,
// `channel` is an enum from surfacing.Channel, and `reason` is a fixed
// vocabulary owned by the controller (currently:
// {urgent_escalation} for overrides, {acknowledged-by-user} for
// suppression).

var SurfacingNudgesDelivered = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_nudges_delivered_total",
		Help: "Nudges permitted by the unified surfacing controller by producer and channel",
	},
	[]string{"producer", "channel"},
)

var SurfacingActedOn = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_acted_on_total",
		Help: "Surfaced nudges that received a positive user interaction (ack / open / completion) by producer",
	},
	[]string{"producer"},
)

var SurfacingFalsePositive = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_false_positive_total",
		Help: "Surfaced nudges explicitly flagged as not useful by the user, by producer",
	},
	[]string{"producer"},
)

var SurfacingDedupe = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_dedupe_total",
		Help: "Candidates suppressed by the cross-channel dedupe index, by producer",
	},
	[]string{"producer"},
)

var SurfacingSuppression = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_suppression_total",
		Help: "Candidates suppressed by ack-window enforcement, by bounded reason",
	},
	[]string{"reason"},
)

var SurfacingBudgetOverrides = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_budget_overrides_total",
		Help: "Urgent escalations that bypassed the exhausted daily budget, by bounded reason",
	},
	[]string{"reason"},
)

var SurfacingBudgetRemaining = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_surfacing_budget_remaining",
		Help: "Remaining nudge slots in the current daily surfacing budget",
	},
)

var SurfacingDeferredExhausted = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_surfacing_deferred_budget_exhausted_total",
		Help: "Non-urgent candidates held by the controller because the daily budget was exhausted, by producer",
	},
	[]string{"producer"},
)

func init() {
	prometheus.MustRegister(
		SurfacingNudgesDelivered,
		SurfacingActedOn,
		SurfacingFalsePositive,
		SurfacingDedupe,
		SurfacingSuppression,
		SurfacingBudgetOverrides,
		SurfacingBudgetRemaining,
		SurfacingDeferredExhausted,
	)
}

// SurfacingMetrics adapts the prometheus metrics to the
// surfacing.MetricsSink contract so the controller can stay free of any
// prometheus import dependency.
type SurfacingMetrics struct{}

func (SurfacingMetrics) IncDelivered(p surfacing.Producer, c surfacing.Channel) {
	SurfacingNudgesDelivered.WithLabelValues(string(p), string(c)).Inc()
}

func (SurfacingMetrics) IncDeduped(p surfacing.Producer) {
	SurfacingDedupe.WithLabelValues(string(p)).Inc()
}

func (SurfacingMetrics) IncSuppressed(reason string) {
	SurfacingSuppression.WithLabelValues(reason).Inc()
}

func (SurfacingMetrics) IncBudgetOverride(reason string) {
	SurfacingBudgetOverrides.WithLabelValues(reason).Inc()
}

func (SurfacingMetrics) IncDeferredBudgetExhausted(p surfacing.Producer) {
	SurfacingDeferredExhausted.WithLabelValues(string(p)).Inc()
}

func (SurfacingMetrics) SetBudgetRemaining(remaining int) {
	SurfacingBudgetRemaining.Set(float64(remaining))
}

// RecordSurfacingActedOn / RecordSurfacingFalsePositive are thin helpers
// that the annotation/feedback pipeline calls when the user marks a
// surfaced item as acted-on or not-useful. They live in metrics so the
// observability contract is in one place.
func RecordSurfacingActedOn(producer surfacing.Producer) {
	SurfacingActedOn.WithLabelValues(string(producer)).Inc()
}

func RecordSurfacingFalsePositive(producer surfacing.Producer) {
	SurfacingFalsePositive.WithLabelValues(string(producer)).Inc()
}
