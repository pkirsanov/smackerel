// Spec 059 Scope 5 — Google Keep live-sync observability metrics.
//
// All metrics share the `smackerel_keep_` prefix and bounded label
// cardinality. No metric label may carry the operator email or the
// Bucket-2 App Password value; this is asserted by the keep-log-
// redaction unit test (SCN-059-015).
package metrics

import "github.com/prometheus/client_golang/prometheus"

// KeepProtocolDriftDetected counts the number of times the drift
// circuit breaker transitions into OPEN. Per SCN-059-013 this MUST
// increment exactly once per OPEN entry — repeated Sync() calls
// while OPEN do not advance it. The label is bounded to a stable
// operator-controlled connector id (no per-request values).
var KeepProtocolDriftDetected = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_keep_protocol_drift_detected_total",
		Help: "Times the Google Keep drift circuit breaker entered OPEN (once per entry; reset only by drift_ack_token rotation + restart).",
	},
	[]string{"connector_id"},
)

// KeepGkeepSyncDuration records the observed wall-clock latency of a
// gkeepapi sidecar request/reply round trip. Buckets sized for the
// observed Python sidecar timings (200 ms .. 60 s) and capped by
// gkeepRequestTimeout in internal/connector/keep/keep.go.
var KeepGkeepSyncDuration = prometheus.NewHistogram(
	prometheus.HistogramOpts{
		Name:    "smackerel_keep_gkeep_sync_duration_seconds",
		Help:    "Wall-clock latency of a successful Google Keep sidecar sync request/reply.",
		Buckets: []float64{0.2, 0.5, 1, 2, 5, 10, 20, 30, 60},
	},
)

// KeepGkeepNotesReturned counts the total number of notes the sidecar
// returned on successful sync calls. Increments by the number of notes
// in each ok response (not per-call); operators chart rate per scrape
// window.
var KeepGkeepNotesReturned = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "smackerel_keep_gkeep_notes_returned_total",
		Help: "Total notes returned across all successful Google Keep sidecar sync responses.",
	},
)

func init() {
	prometheus.MustRegister(
		KeepProtocolDriftDetected,
		KeepGkeepSyncDuration,
		KeepGkeepNotesReturned,
	)
}
