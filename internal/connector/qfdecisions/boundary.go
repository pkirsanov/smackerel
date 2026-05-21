package qfdecisions

import (
	"fmt"
	"strings"
	"time"
)

type ActionBoundaryAttempt struct {
	AttemptedActionType string
	TraceID             string
	PacketID            string
	ActorRef            string
	Surface             string
	Reason              string
	ObservedAt          time.Time
}

type ActionBoundaryDiagnostic struct {
	AttemptedActionType string
	Reason              string
	AuditEnvelope       EvidenceAuditEnvelope
}

func RejectQFActionBoundary(attempt ActionBoundaryAttempt) (ActionBoundaryDiagnostic, error) {
	actionType := metricLabel(attempt.AttemptedActionType)
	reason := strings.TrimSpace(attempt.Reason)
	if reason == "" {
		reason = "qf_companion_is_read_only"
	}
	RecordQFActionBoundaryAttempt(actionType)
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:    attempt.TraceID,
		PacketID:   attempt.PacketID,
		ActorRef:   attempt.ActorRef,
		Surface:    attempt.Surface,
		Action:     AuditActionActionBoundaryKick,
		Outcome:    AuditOutcomeRejected,
		Reason:     reason,
		ObservedAt: attempt.ObservedAt,
	})
	EmitConnectorAuditEnvelope(envelope)
	diagnostic := ActionBoundaryDiagnostic{AttemptedActionType: actionType, Reason: reason, AuditEnvelope: envelope}
	return diagnostic, fmt.Errorf("QF companion action boundary rejected %s: %s", actionType, reason)
}

func IsForbiddenQFActionType(actionType string) bool {
	switch strings.TrimSpace(actionType) {
	case ActionTypeApproval,
		ActionTypeExecution,
		ActionTypeMandateChange,
		ActionTypeEmergencyStop,
		ActionTypeWatchCreation,
		ActionTypeWatchEvaluation,
		ActionTypeCallbackAcceptance,
		ActionTypeQFTrustReconstruction:
		return true
	default:
		return false
	}
}
