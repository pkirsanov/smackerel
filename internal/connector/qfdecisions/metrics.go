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

func metricLabel(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return metricUnknown
	}
	return trimmed
}
