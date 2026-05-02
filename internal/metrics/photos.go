// Spec 040 Scope 5 — Photo connector observability metrics.
//
// All metrics share the `smackerel_photos_` prefix and bounded label
// cardinality (provider/connector/phase/reason/capability/outcome).
// No metric label may carry photo bytes, preview URLs, or photo
// content; this is asserted by the photo-health observability test.
package metrics

import "github.com/prometheus/client_golang/prometheus"

// PhotosScanTotal counts photo scan progress events by phase. Phase
// values are bounded to: metadata, thumbnails, classify, embeddings,
// ocr, sensitivity, lifecycle, dedupe.
var PhotosScanTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_photos_scan_total",
		Help: "Photo scan progress counts by connector, provider, phase",
	},
	[]string{"connector", "provider", "phase"},
)

// PhotosScanSkippedTotal counts photo scan skips by reason. Reason
// values are bounded to: too_large, unsupported_format,
// permission_denied, provider_error, extraction_failed.
var PhotosScanSkippedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_photos_scan_skipped_total",
		Help: "Photo scan skip counts by connector, provider, reason",
	},
	[]string{"connector", "provider", "reason"},
)

// PhotosLLMCallsTotal counts photo ML pipeline calls by NATS subject
// and outcome (success, failed, dead_lettered).
var PhotosLLMCallsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_photos_llm_calls_total",
		Help: "Photo ML pipeline calls by NATS subject and outcome",
	},
	[]string{"subject", "outcome"},
)

// PhotosLLMLatencySeconds records the observed latency of photo ML
// pipeline calls. Buckets sized for OCR + classification timings
// (50 ms .. 30 s).
var PhotosLLMLatencySeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_photos_llm_latency_seconds",
		Help:    "Photo ML pipeline call latency by NATS subject",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	},
	[]string{"subject"},
)

// PhotosCapabilitiesLimitedTotal counts the number of API requests
// that hit a provider capability limit. Labels are bounded to the
// connector, provider, and the registered capability name.
var PhotosCapabilitiesLimitedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_photos_capabilities_limited_total",
		Help: "Photo provider capability limit hits by connector, provider, capability",
	},
	[]string{"connector", "provider", "capability"},
)

// PhotosDestructiveActionsTotal counts photo action confirmations by
// action (archive, delete, mark_sensitive, ...) and outcome (planned,
// confirmed, cancelled, failed).
var PhotosDestructiveActionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_photos_destructive_actions_total",
		Help: "Photo destructive action lifecycle by action and outcome",
	},
	[]string{"action", "outcome"},
)

// PhotosSensitivityRevealsTotal counts the number of reveal-token
// mints + redemptions by surface (preview, telegram, agent).
var PhotosSensitivityRevealsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_photos_sensitivity_reveals_total",
		Help: "Photo sensitivity reveal mints + redemptions by surface and outcome",
	},
	[]string{"surface", "outcome"},
)

func init() {
	prometheus.MustRegister(
		PhotosScanTotal,
		PhotosScanSkippedTotal,
		PhotosLLMCallsTotal,
		PhotosLLMLatencySeconds,
		PhotosCapabilitiesLimitedTotal,
		PhotosDestructiveActionsTotal,
		PhotosSensitivityRevealsTotal,
	)
}
