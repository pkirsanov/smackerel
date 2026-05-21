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

// TestEnforceQFActionBoundaryFiresForForbiddenAndPassesForBenign documents the
// SCN-SM-041-020 shared-guard contract used by render, evidence export,
// callback-adjacent, and watch-adjacent call-sites. The guard MUST fire (return
// fired=true and an error) for every forbidden QF action type, recording the
// action-boundary-kick audit envelope and incrementing
// smackerel_qf_action_boundary_attempts_total{attempted_action_type}. The guard
// MUST be a no-op (return fired=false and nil) for the empty string and for
// non-action audit identifiers so it can be safely inlined into existing
// non-action emission paths without altering their behavior.
func TestEnforceQFActionBoundaryFiresForForbiddenAndPassesForBenign(t *testing.T) {
	metrics.QFActionBoundaryAttemptsTotal.Reset()
	now := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)

	t.Run("forbidden_fires_and_errors", func(t *testing.T) {
		before := testutil.ToFloat64(metrics.QFActionBoundaryAttemptsTotal.WithLabelValues(ActionTypeApproval))
		diagnostic, fired, err := EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: ActionTypeApproval,
			TraceID:             "trace-enforce-approval",
			PacketID:            "packet-enforce-approval",
			Surface:             SurfaceWeb,
			Reason:              "render_metadata_action_request",
			ObservedAt:          now,
		})
		if !fired {
			t.Fatal("expected fired=true for forbidden ActionTypeApproval")
		}
		if err == nil {
			t.Fatal("expected enforce error for forbidden ActionTypeApproval")
		}
		after := testutil.ToFloat64(metrics.QFActionBoundaryAttemptsTotal.WithLabelValues(ActionTypeApproval))
		if after-before != 1 {
			t.Fatalf("enforce metric delta for ActionTypeApproval = %v, want 1", after-before)
		}
		if diagnostic.AuditEnvelope.Action != AuditActionActionBoundaryKick || diagnostic.AuditEnvelope.Outcome != AuditOutcomeRejected {
			t.Fatalf("enforce audit envelope = %+v", diagnostic.AuditEnvelope)
		}
		if diagnostic.Reason != "render_metadata_action_request" {
			t.Fatalf("enforce diagnostic reason = %q, want %q", diagnostic.Reason, "render_metadata_action_request")
		}
	})

	t.Run("empty_action_type_is_noop", func(t *testing.T) {
		diagnostic, fired, err := EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: "",
			TraceID:             "trace-enforce-noop",
		})
		if fired {
			t.Fatal("expected fired=false for empty action type")
		}
		if err != nil {
			t.Fatalf("expected nil error for empty action type, got %v", err)
		}
		if diagnostic.AuditEnvelope.Action != "" {
			t.Fatalf("expected zero diagnostic for empty action type, got %+v", diagnostic.AuditEnvelope)
		}
	})

	t.Run("benign_action_type_is_noop", func(t *testing.T) {
		diagnostic, fired, err := EnforceQFActionBoundary(ActionBoundaryAttempt{
			AttemptedActionType: "surface_dismiss",
			TraceID:             "trace-enforce-benign",
		})
		if fired {
			t.Fatal("expected fired=false for benign action type")
		}
		if err != nil {
			t.Fatalf("expected nil error for benign action type, got %v", err)
		}
		if diagnostic.AuditEnvelope.Action != "" {
			t.Fatalf("expected zero diagnostic for benign action type, got %+v", diagnostic.AuditEnvelope)
		}
	})
}
