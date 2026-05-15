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

// DriveConfirmationsTotal counts spec 038 Scope 6 drive confirmation
// resolutions by terminal status (committed, rerouted, no_save, expired,
// already_resolved) and the channel that delivered the user choice
// (web, telegram). Used to detect a stuck pending backlog or a sudden
// spike in expired confirmations.
var DriveConfirmationsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_confirmations_total",
		Help: "Drive low-confidence confirmation resolutions by status and channel",
	},
	[]string{"status", "channel"},
)

// DrivePolicyDecisionsTotal counts spec 038 Scope 6 sensitivity policy
// decisions by enforcement surface, decision verdict, and sensitivity
// tier. The labels match the policy.Engine outputs so dashboards can
// reconstruct the decision table from metric output alone.
var DrivePolicyDecisionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_policy_decisions_total",
		Help: "Drive sensitivity policy decisions by surface, decision, and sensitivity",
	},
	[]string{"surface", "decision", "sensitivity"},
)

// DriveRuleConflictsTotal counts spec 038 Scope 6 Save Rule conflict
// audit rows. The label is the rule id of the stable winner so an
// operator can spot a single rule consistently colliding.
var DriveRuleConflictsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_drive_rule_conflicts_total",
		Help: "Drive save rule conflicts audited per stable-winner rule_id",
	},
	[]string{"rule_id"},
)

// --- QF Companion Connector (spec 041) ---

// QFCapabilityMismatch counts capability-handshake mismatches by required vs actual value.
// Bounded labels: `required` is the value the connector requires (a small fixed set:
// "v1", "recommendation", "policy_denial", "analysis_note", ">=1"); `actual` is the
// QF-advertised value or comma-joined list. Cardinality stays low because misconfig
// surfaces quickly and is corrected — it is not a runtime-hot label.
var QFCapabilityMismatch = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_capability_mismatch_total",
		Help: "QF Companion capability handshake mismatches by required vs actual value",
	},
	[]string{"required", "actual"},
)

// QFUnknownDecisionType counts decision events whose decision_type is not in
// the capability-advertised supported_decision_types list. The connector still
// emits the event into NATS for diagnostic visibility but flags
// metadata.unknown_decision_type=true so downstream consumers can filter.
var QFUnknownDecisionType = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_unknown_decision_type_total",
		Help: "QF Companion decision events received with unknown decision_type",
	},
	[]string{"value"},
)

// QFCursorLagSeconds reports the QF Companion connector's cursor lag in
// seconds (now - last_event.server_time). Emitted on every Sync tick so
// operators can plot drift and breach alerts. NEVER auto-advances; lag
// recovery is operator-initiated via POST /api/private/smackerel/v1/cursor:fast-forward.
// SCN-SM-041-007.
var QFCursorLagSeconds = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "smackerel_qf_cursor_lag_seconds",
		Help: "QF Companion connector cursor lag in seconds (now - last event server_time)",
	},
)

// QFCursorFastForwardEventsSkipped counts events skipped by operator-initiated
// QF cursor fast-forward (POST /api/private/smackerel/v1/cursor:fast-forward).
// Incremented by the events_skipped value reported in the QF diagnostic event
// when the connector detects the cursor has advanced beyond a normal page.
// SCN-SM-041-008.
var QFCursorFastForwardEventsSkipped = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "smackerel_qf_cursor_fast_forward_events_skipped_total",
		Help: "Total events skipped by operator-initiated QF cursor fast-forward",
	},
)

// QFFreshnessP95Seconds reports the rolling-window p95 freshness latency of the
// QF Companion connector per pipeline stage. Bounded labels: `stage` is one of
// "ingest" (QF event server_time → smackerel artifact persisted), "render"
// (artifact persisted → render surface emit), or "total" (server_time → render
// emit). Republished by Connector.recordFreshness() each time a window has
// enough samples to compute p95. Negative observations are clamped to zero so
// clock skew between QF and smackerel hosts cannot produce a misleading
// negative gauge value. Consumed by design.md §F12 freshness budget alerts.
// SCN-SM-041-009.
var QFFreshnessP95Seconds = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "smackerel_qf_freshness_p95_seconds",
		Help: "QF Companion connector freshness latency p95 in seconds, per pipeline stage (ingest|render|total)",
	},
	[]string{"stage"},
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
		DriveConfirmationsTotal,
		DrivePolicyDecisionsTotal,
		DriveRuleConflictsTotal,
		QFCapabilityMismatch,
		QFUnknownDecisionType,
		QFCursorLagSeconds,
		QFCursorFastForwardEventsSkipped,
		QFFreshnessP95Seconds,
		// Spec 039 Scope 6 recommendation metrics — defined in
		// recommendations.go; bounded labels enforced (no watch_id,
		// no recommendation_id, no request_id).
		RecommendationProviderRequests,
		RecommendationProviderLatency,
		RecommendationCandidates,
		RecommendationWatchRuns,
		RecommendationDelivery,
		RecommendationSuppression,
		RecommendationRankingConfidence,
		RecommendationLocationPrecision,
		// Spec 048 backup-status metrics — defined in backup.go.
		// Republished from BACKUP_STATUS_FILE by the
		// internal/backup.Watcher; consumed by the SmackerelBackupStale
		// alert in config/prometheus/alerts.yml.
		BackupLastSuccessUnixtime,
		BackupSizeBytes,
		BackupRunsTotal,
	)
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}
