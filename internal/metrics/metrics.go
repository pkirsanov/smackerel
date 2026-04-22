// Package metrics provides Prometheus metric definitions for smackerel-core.
// All metrics use the "smackerel_" prefix and bounded label cardinality.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// --- Ingestion & Pipeline ---

// ArtifactsIngested counts total artifacts ingested by source and type.
var ArtifactsIngested = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_artifacts_ingested_total",
		Help: "Total artifacts ingested by source and type",
	},
	[]string{"source", "type"},
)

// CaptureTotal counts capture requests by source.
var CaptureTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_capture_total",
		Help: "Capture requests by source",
	},
	[]string{"source"},
)

// --- Search ---

// SearchLatency records search request latency in seconds.
var SearchLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_search_latency_seconds",
		Help:    "Search request latency",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	},
	[]string{"mode"},
)

// --- Domain Extraction ---

// DomainExtraction counts domain extraction attempts by schema and status.
var DomainExtraction = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_domain_extraction_total",
		Help: "Domain extraction attempts",
	},
	[]string{"schema", "status"},
)

// DomainExtractionLatency records domain extraction processing time in milliseconds.
var DomainExtractionLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_domain_extraction_duration_ms",
		Help:    "Domain extraction processing time in milliseconds",
		Buckets: []float64{100, 500, 1000, 2000, 5000, 10000, 20000, 30000},
	},
	[]string{"contract"},
)

// --- Connector Sync ---

// ConnectorSync counts connector sync attempts by connector name and status.
var ConnectorSync = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_connector_sync_total",
		Help: "Connector sync attempts",
	},
	[]string{"connector", "status"},
)

// --- NATS ---

// NATSDeadLetter counts messages routed to dead letter by stream.
var NATSDeadLetter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_nats_deadletter_total",
		Help: "Messages routed to dead letter",
	},
	[]string{"stream"},
)

// --- Database ---

// DBConnectionsActive tracks active database connections.
var DBConnectionsActive = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_db_connections_active",
		Help: "Active database connections",
	},
)

// --- Digest ---

// DigestGeneration counts digest generation attempts by status.
var DigestGeneration = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_digest_generation_total",
		Help: "Digest generation attempts",
	},
	[]string{"status"},
)

// --- Intelligence (Phase 5) ---

// IntelligenceLatency records Phase 5 intelligence endpoint latency in seconds.
var IntelligenceLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_intelligence_latency_seconds",
		Help:    "Intelligence endpoint latency by endpoint",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	},
	[]string{"endpoint"},
)

// IntelligenceErrors counts intelligence endpoint errors by endpoint.
var IntelligenceErrors = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_intelligence_errors_total",
		Help: "Intelligence endpoint errors by endpoint",
	},
	[]string{"endpoint"},
)

// --- Alert Delivery ---

// AlertsDelivered counts alerts successfully delivered via Telegram by alert type.
var AlertsDelivered = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_alerts_delivered_total",
		Help: "Alerts delivered via Telegram by type",
	},
	[]string{"type"},
)

// AlertDeliveryFailures counts alert delivery failures.
var AlertDeliveryFailures = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "smackerel_alert_delivery_failures_total",
		Help: "Alert delivery failures (Telegram send or mark-delivered)",
	},
)

// AlertsProduced counts alerts created by alert producers by type.
var AlertsProduced = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_alerts_produced_total",
		Help: "Alerts created by producers by type",
	},
	[]string{"type"},
)

// --- Actionable Lists (Spec 028) ---

// ListsGenerated counts list generation attempts by list_type and domain.
var ListsGenerated = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_lists_generated_total",
		Help: "List generation attempts by list type and domain",
	},
	[]string{"list_type", "domain"},
)

// ListGenerationLatency records list generation latency in seconds.
var ListGenerationLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_list_generation_latency_seconds",
		Help:    "List generation latency",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
	},
	[]string{"list_type"},
)

// ListItemStatusChanges counts list item status transitions by new status.
var ListItemStatusChanges = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_list_item_status_changes_total",
		Help: "List item status transitions",
	},
	[]string{"status"},
)

// ListsCompleted counts lists marked as completed by list_type.
var ListsCompleted = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_lists_completed_total",
		Help: "Lists marked as completed",
	},
	[]string{"list_type"},
)

func init() {
	prometheus.MustRegister(
		ArtifactsIngested,
		CaptureTotal,
		SearchLatency,
		DomainExtraction,
		DomainExtractionLatency,
		ConnectorSync,
		NATSDeadLetter,
		DBConnectionsActive,
		DigestGeneration,
		IntelligenceLatency,
		IntelligenceErrors,
		AlertsDelivered,
		AlertDeliveryFailures,
		AlertsProduced,
		ListsGenerated,
		ListGenerationLatency,
		ListItemStatusChanges,
		ListsCompleted,
	)
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}
