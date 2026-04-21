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

func init() {
	prometheus.MustRegister(
		ArtifactsIngested,
		CaptureTotal,
		SearchLatency,
		DomainExtraction,
		ConnectorSync,
		NATSDeadLetter,
		DBConnectionsActive,
		DigestGeneration,
		IntelligenceLatency,
		IntelligenceErrors,
	)
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}
