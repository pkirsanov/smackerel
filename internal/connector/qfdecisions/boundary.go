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

// EnforceQFActionBoundary is the shared no-action defense-in-depth guard used
// by sync, render, evidence export, callback-adjacent, and watch-adjacent
// call-sites. It pre-checks the supplied attempted action type against
// IsForbiddenQFActionType and, when the type is forbidden, dispatches to
// RejectQFActionBoundary so the action-boundary-kick audit envelope is
// emitted and smackerel_qf_action_boundary_attempts_total{attempted_action_type}
// is incremented BEFORE any side effect (artifact emission, card render, HTTP
// export, callback acceptance, or watch proposal) reaches the surface.
//
// Returns (diagnostic, fired=true, err) when the action type is forbidden so
// callers can short-circuit. Returns (zero, false, nil) when the action type
// is empty or not forbidden so non-action paths remain unaffected. This is the
// only function call-site wirings should use; calling RejectQFActionBoundary
// directly is reserved for the Sync-loop's QF-bridge-emitted
// packet_action_boundary_attempted event (SCN-SM-041-020).
func EnforceQFActionBoundary(attempt ActionBoundaryAttempt) (ActionBoundaryDiagnostic, bool, error) {
	if !IsForbiddenQFActionType(attempt.AttemptedActionType) {
		return ActionBoundaryDiagnostic{}, false, nil
	}
	diagnostic, err := RejectQFActionBoundary(attempt)
	return diagnostic, true, err
}
