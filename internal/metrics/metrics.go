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

// QFPacketIngestTotal counts successfully ingested QF decision packets with
// the exact label parity required by QF design 063.
var QFPacketIngestTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_packet_ingest_total",
		Help: "QF Companion decision packet ingest attempts by event type, decision type, approval state, and source surface",
	},
	[]string{"event_type", "decision_type", "approval_state", "source_surface"},
)

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

// QFActionBoundaryAttemptsTotal counts rejected attempts to use the passive
// companion bridge as a financial-action surface.
var QFActionBoundaryAttemptsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_action_boundary_attempts_total",
		Help: "QF Companion rejected action-boundary attempts by attempted action type",
	},
	[]string{"attempted_action_type"},
)

// QFPacketValidationFailures counts QF companion packet and polling contract
// failures by bounded reason. The Scope 2 page-size path emits
// reason="page_size_out_of_range" when QF rejects the connector's explicit,
// capability-clamped decision-events limit.
var QFPacketValidationFailures = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_packet_validation_failures_total",
		Help: "QF Companion packet validation and polling contract failures by reason",
	},
	[]string{"reason"},
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

// QFTrustObjectRenderFailures counts QF trust objects that cannot be rendered
// because the public rendering contract is incomplete. Bounded reason labels
// currently use missing_required_field for absent label/severity values.
var QFTrustObjectRenderFailures = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_trust_object_render_failures_total",
		Help: "QF Companion trust object render failures by bounded reason",
	},
	[]string{"reason"},
)

// QFDeepLinkRenderTotal counts QF deep-link render decisions by surface and
// bounded status: signed_used, signed_expired_fallback_unsigned, unsigned_only.
var QFDeepLinkRenderTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_deep_link_render_total",
		Help: "QF Companion deep-link render decisions by surface and status",
	},
	[]string{"surface", "status"},
)

// QFEvidenceExportAttempts counts personal evidence bundle export attempts by
// terminal status and bounded context labels.
var QFEvidenceExportAttempts = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_evidence_export_attempts_total",
		Help: "QF Companion personal evidence bundle export attempts by status, target context, and sensitivity tier",
	},
	[]string{"status", "target_context_type", "sensitivity_tier"},
)

// QFEvidenceRevokedTotal counts completed evidence-export revocations by
// bounded reason. The pre-MVP Scope 4 path emits consent_revoked for user
// consent removal.
var QFEvidenceRevokedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_evidence_revoked_total",
		Help: "QF Companion personal evidence bundle revocations by reason",
	},
	[]string{"reason"},
)

// QFEngagementSignalAttemptsTotal counts pre-MVP engagement-signal flush
// attempts without implementing downstream Scope 6 transport.
var QFEngagementSignalAttemptsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_engagement_signal_attempts_total",
		Help: "QF Companion engagement signal flush attempts by event, surface, and status",
	},
	[]string{"event", "surface", "status"},
)

// QFCallbackAttemptsTotal counts callback attempts without accepting QF-side
// financial action callbacks in the pre-MVP bridge.
var QFCallbackAttemptsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_callback_attempts_total",
		Help: "QF Companion callback attempts by action and status",
	},
	[]string{"action", "status"},
)

// QFCallbackSignatureFailuresTotal counts local signing-stage rejections of
// callback envelopes BEFORE any HTTP transport. The `reason` label
// vocabulary is bounded to spec 041 Scope 8 / SCN-SM-041-030:
// {NO_ACTIVE_KEY, MALFORMED_CANONICAL_PAYLOAD, EXPIRES_AT_OUTSIDE_TOLERANCE}.
// Every increment is paired with a Cross-Product Audit Envelope v1 record
// (action=callback_attempt, outcome=rejected, reason=<vocabulary>). The
// network is never reached when this metric fires — see
// internal/connector/qfdecisions/callback.go CallbackSigner.Sign.
var QFCallbackSignatureFailuresTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_callback_signature_failures_total",
		Help: "QF Companion callback signature failures by documented reason vocabulary",
	},
	[]string{"reason"},
)

// QFWatchProposalAttemptsTotal counts diagnostic watch-proposal POST
// attempts made by the Scope 9 connector-internal client. The `status`
// label vocabulary is bounded to spec 041 Scope 9 / SCN-SM-041-033:
// {rejected_v1_deferred, rejected_local, degraded}. Pre-MVP `accepted`
// is never emitted; every Scope 9 attempt is expected to be rejected
// by QF with `WATCH_PROPOSALS_DEFERRED_TO_V1`. Every increment is
// paired with a Cross-Product Audit Envelope v1 record
// (action=watch_proposal, outcome=rejected|error, reason=<vocabulary>).
var QFWatchProposalAttemptsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_watch_proposal_attempts_total",
		Help: "QF Companion watch-proposal POST attempts by Scope 9 status vocabulary",
	},
	[]string{"status"},
)

// QFPersonalContextReadsTotal counts personal-context read attempts by
// outcome and sensitivity tier (spec 041 Scope 7, SCN-SM-041-027). The
// outcome vocabulary is bounded to
// {ok, rejected, degraded, rate_limited, capability_disabled}; the
// sensitivity_tier vocabulary is bounded to {low, medium, high}.
var QFPersonalContextReadsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_qf_personal_context_reads_total",
		Help: "QF Companion personal-context read attempts by outcome and requested sensitivity tier",
	},
	[]string{"outcome", "sensitivity_tier"},
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
		QFPacketIngestTotal,
		QFCapabilityMismatch,
		QFUnknownDecisionType,
		QFCursorLagSeconds,
		QFCursorFastForwardEventsSkipped,
		QFActionBoundaryAttemptsTotal,
		QFPacketValidationFailures,
		QFFreshnessP95Seconds,
		QFTrustObjectRenderFailures,
		QFDeepLinkRenderTotal,
		QFEvidenceExportAttempts,
		QFEvidenceRevokedTotal,
		QFEngagementSignalAttemptsTotal,
		QFCallbackAttemptsTotal,
		QFCallbackSignatureFailuresTotal,
		QFWatchProposalAttemptsTotal,
		QFPersonalContextReadsTotal,
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
