package qfdecisions

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/metrics"
)

func TestRejectQFActionBoundaryBlocksAllFinancialActionTypes(t *testing.T) {
	metrics.QFActionBoundaryAttemptsTotal.Reset()
	now := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC)
	for _, actionType := range []string{
		ActionTypeApproval,
		ActionTypeExecution,
		ActionTypeMandateChange,
		ActionTypeEmergencyStop,
		ActionTypeWatchCreation,
		ActionTypeWatchEvaluation,
		ActionTypeCallbackAcceptance,
		ActionTypeQFTrustReconstruction,
	} {
		t.Run(actionType, func(t *testing.T) {
			if !IsForbiddenQFActionType(actionType) {
				t.Fatalf("%s must be classified as a forbidden QF action type", actionType)
			}
			before := testutil.ToFloat64(metrics.QFActionBoundaryAttemptsTotal.WithLabelValues(actionType))
			diagnostic, err := RejectQFActionBoundary(ActionBoundaryAttempt{
				AttemptedActionType: actionType,
				TraceID:             "trace-boundary-001",
				PacketID:            "packet-boundary-001",
				Surface:             SurfaceWeb,
				ObservedAt:          now,
			})
			if err == nil {
				t.Fatal("expected boundary rejection error")
			}
			after := testutil.ToFloat64(metrics.QFActionBoundaryAttemptsTotal.WithLabelValues(actionType))
			if after-before != 1 {
				t.Fatalf("action boundary metric delta for %s = %v, want 1", actionType, after-before)
			}
			if diagnostic.AuditEnvelope.Action != AuditActionActionBoundaryKick || diagnostic.AuditEnvelope.Outcome != AuditOutcomeRejected {
				t.Fatalf("boundary audit envelope = %+v", diagnostic.AuditEnvelope)
			}
			if diagnostic.AuditEnvelope.TraceID != "trace-boundary-001" || diagnostic.AuditEnvelope.PacketID != "packet-boundary-001" || diagnostic.AuditEnvelope.Surface != SurfaceWeb {
				t.Fatalf("boundary audit identity fields = %+v", diagnostic.AuditEnvelope)
			}
		})
	}
}
