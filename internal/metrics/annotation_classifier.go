// Spec 076 SCOPE-4b — divergence telemetry for the
// `internal/annotation` dual-write shadow comparator.
//
// Metrics:
//
//   - smackerel_annotation_classifier_shadow_calls_total{channel, outcome}
//     — counts every annotation call routed through the dual-write
//     comparator. `outcome` ∈ {match, divergence, shadow_error,
//     shadow_below_floor}.
//   - smackerel_annotation_classifier_divergence_total{channel,
//     primary_type, shadow_type} — counts only the divergence subset
//     so dashboards can break it down by which type pair flipped.
//
// Label cardinality is bounded: `channel` is the closed
// SourceChannel enum, `primary_type` / `shadow_type` are the closed
// InteractionType enum (plus `""` sentinel), `outcome` is the closed
// set listed above.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// AnnotationClassifierShadowCalls counts every annotation call routed
// through the SCOPE-4b dual-write shadow comparator.
//
// Outcome label vocabulary (closed set):
//
//   - "match"               — primary == shadow
//   - "divergence"          — primary != shadow (both non-error)
//   - "shadow_error"        — shadow classifier returned a non-floor error
//   - "shadow_below_floor"  — shadow returned ErrBelowConfidenceFloor
//     (treated as "shadow declined to answer", NOT as divergence)
var AnnotationClassifierShadowCalls = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_annotation_classifier_shadow_calls_total",
		Help: "Spec 076 SCOPE-4b — annotation calls routed through the dual-write shadow comparator, by channel and comparator outcome.",
	},
	[]string{"channel", "outcome"},
)

// AnnotationClassifierDivergence counts the divergence subset of
// shadow-comparator calls. Use this for the "shadow agrees with
// inline literal" SLO that gates SCOPE-4c (interactionMap removal).
var AnnotationClassifierDivergence = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_annotation_classifier_divergence_total",
		Help: "Spec 076 SCOPE-4b — annotation classifier divergence count: shadow result disagreed with the inline interactionMap primary result.",
	},
	[]string{"channel", "primary_type", "shadow_type"},
)

func init() {
	prometheus.MustRegister(
		AnnotationClassifierShadowCalls,
		AnnotationClassifierDivergence,
	)
}
