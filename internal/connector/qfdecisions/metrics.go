package qfdecisions

import (
	"strings"

	"github.com/smackerel/smackerel/internal/metrics"
)

const metricUnknown = "unknown"

func RecordQFPacketIngest(event QFDecisionEvent) {
	metrics.QFPacketIngestTotal.WithLabelValues(
		metricLabel(event.EventType),
		metricLabel(event.DecisionType),
		metricLabel(event.ApprovalState),
		metricLabel(event.SourceSurface),
	).Inc()
}

func RecordQFEvidenceExportAttempt(status, targetContextType, sensitivityTier string) {
	metrics.QFEvidenceExportAttempts.WithLabelValues(
		metricLabel(status),
		metricLabel(targetContextType),
		metricLabel(sensitivityTier),
	).Inc()
}

func RecordQFActionBoundaryAttempt(attemptedActionType string) {
	metrics.QFActionBoundaryAttemptsTotal.WithLabelValues(metricLabel(attemptedActionType)).Inc()
}

func RecordQFEngagementSignalAttempt(event, surface, status string) {
	metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(metricLabel(event), metricLabel(surface), metricLabel(status)).Inc()
}

func RecordQFCallbackAttempt(action, status string) {
	metrics.QFCallbackAttemptsTotal.WithLabelValues(metricLabel(action), metricLabel(status)).Inc()
}

// RecordQFCallbackSignatureFailure increments the Scope 8
// signature-failure counter. The `reason` label MUST be one of the
// documented vocabulary values:
//
//   - CallbackSignatureFailureNoActiveKey
//   - CallbackSignatureFailureMalformedCanonicalPayload
//   - CallbackSignatureFailureExpiresAtOutsideTolerance
//
// Empty or unknown reasons are recorded under the "unknown" label so
// the counter still increments and operators see a dashboard signal,
// but the documented vocabulary is the only intended input.
// SCN-SM-041-030.
func RecordQFCallbackSignatureFailure(reason string) {
	metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(metricLabel(reason)).Inc()
}

// RecordQFWatchProposalAttempt increments the Scope 9 watch-proposal
// attempt counter. The `status` label MUST be one of the
// documented vocabulary values:
//
//   - WatchProposalStatusRejectedV1Deferred ("rejected_v1_deferred")
//   - WatchProposalStatusRejectedLocal ("rejected_local")
//   - WatchProposalStatusDegraded ("degraded")
//
// Pre-MVP `accepted` is never emitted (every QF response is the
// rejection contract `WATCH_PROPOSALS_DEFERRED_TO_V1`). Empty or
// unknown values are recorded under the "unknown" label so the
// counter still increments and operators see a dashboard signal,
// but the documented vocabulary is the only intended input.
// SCN-SM-041-033.
func RecordQFWatchProposalAttempt(status string) {
	metrics.QFWatchProposalAttemptsTotal.WithLabelValues(metricLabel(status)).Inc()
}

func metricLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return metricUnknown
	}
	return trimmed
}
