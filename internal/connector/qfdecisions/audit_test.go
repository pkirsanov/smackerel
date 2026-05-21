package qfdecisions

import (
	"testing"
	"time"
)

func TestCrossProductAuditEnvelopeV1Shape(t *testing.T) {
	observedAt := time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC)
	envelope := BuildCrossProductAuditEnvelopeV1(AuditEnvelopeInput{
		TraceID:         "trace-audit-001",
		PacketID:        "packet-audit-001",
		ExportID:        "export-audit-001",
		SignalID:        "signal-audit-001",
		ActorRef:        "operator:scope5-test",
		Surface:         SurfaceDigest,
		Action:          AuditActionEvidenceExportAttempt,
		Outcome:         AuditOutcomeOK,
		BundleID:        "bundle-audit-001",
		TargetContext:   TargetContextPacketContext,
		SensitivityTier: "personal",
		ObservedAt:      observedAt,
	})

	if envelope.TraceID != "trace-audit-001" || envelope.PacketID != "packet-audit-001" || envelope.ExportID != "export-audit-001" || envelope.SignalID != "signal-audit-001" {
		t.Fatalf("audit identity fields = %+v", envelope)
	}
	if envelope.ActorRef != "operator:scope5-test" || envelope.Surface != SurfaceDigest {
		t.Fatalf("audit actor/surface = %+v", envelope)
	}
	if envelope.Action != AuditActionEvidenceExportAttempt || envelope.Outcome != AuditOutcomeOK {
		t.Fatalf("audit action/outcome = %+v", envelope)
	}
	if envelope.TS != observedAt.Format(time.RFC3339) || envelope.RecordedAt != envelope.TS {
		t.Fatalf("audit timestamps TS=%q RecordedAt=%q want %q", envelope.TS, envelope.RecordedAt, observedAt.Format(time.RFC3339))
	}
	if envelope.AuditEnvelopeVersion != AuditEnvelopeVersionV1 {
		t.Fatalf("audit envelope version = %q, want %q", envelope.AuditEnvelopeVersion, AuditEnvelopeVersionV1)
	}
	if envelope.BundleID != "bundle-audit-001" || envelope.TargetContextType != TargetContextPacketContext || envelope.SensitivityTier != "personal" {
		t.Fatalf("audit evidence extension fields = %+v", envelope)
	}
}

func TestScope5AuditEmissionPointConstantsAreStable(t *testing.T) {
	wantActions := []string{
		AuditActionPacketIngest,
		AuditActionEvidenceExportAttempt,
		AuditActionEvidenceRevocation,
		AuditActionEngagementSignalFlush,
		AuditActionCallbackAttempt,
		AuditActionDeepLinkRender,
		AuditActionCapabilityHandshake,
		AuditActionActionBoundaryKick,
	}
	for _, action := range wantActions {
		if action == "" {
			t.Fatalf("scope 5 audit action constant must not be empty: %v", wantActions)
		}
	}
}

func TestEmitEngagementSignalFlushAuditPopulatesEnvelopeShape(t *testing.T) {
	observedAt := time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
	envelope := EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
		SignalID:   "signal-001",
		TraceID:    "trace-001",
		PacketID:   "packet-001",
		ActorRef:   "operator:scope5-test",
		Surface:    SurfaceDigest,
		Event:      "packet_marked_seen",
		Status:     "ok",
		Reason:     "user_marked_seen_in_digest",
		ObservedAt: observedAt,
	})

	if envelope.Action != AuditActionEngagementSignalFlush {
		t.Fatalf("engagement audit action = %q, want %q", envelope.Action, AuditActionEngagementSignalFlush)
	}
	if envelope.Outcome != AuditOutcomeOK {
		t.Fatalf("engagement audit outcome = %q, want %q", envelope.Outcome, AuditOutcomeOK)
	}
	if envelope.SignalID != "signal-001" || envelope.TraceID != "trace-001" || envelope.PacketID != "packet-001" {
		t.Fatalf("engagement audit identity fields = %+v", envelope)
	}
	if envelope.ActorRef != "operator:scope5-test" || envelope.Surface != SurfaceDigest {
		t.Fatalf("engagement audit actor/surface = %+v", envelope)
	}
	if envelope.AuditEnvelopeVersion != AuditEnvelopeVersionV1 {
		t.Fatalf("engagement audit envelope version = %q", envelope.AuditEnvelopeVersion)
	}
	if envelope.TS != observedAt.Format(time.RFC3339) || envelope.RecordedAt != envelope.TS {
		t.Fatalf("engagement audit timestamps TS=%q RecordedAt=%q", envelope.TS, envelope.RecordedAt)
	}
}

func TestEmitEngagementSignalFlushAuditMapsRejectedStatus(t *testing.T) {
	envelope := EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
		SignalID:   "signal-rej",
		Surface:    SurfaceDigest,
		Status:     "rejected",
		Reason:     "rate_limit",
		ObservedAt: time.Date(2026, 5, 22, 9, 5, 0, 0, time.UTC),
	})
	if envelope.Outcome != AuditOutcomeRejected {
		t.Fatalf("rejected engagement outcome = %q, want %q", envelope.Outcome, AuditOutcomeRejected)
	}
}

func TestEmitCallbackAttemptAuditPopulatesEnvelopeShape(t *testing.T) {
	observedAt := time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC)
	envelope := EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
		TraceID:    "trace-cb-001",
		PacketID:   "packet-cb-001",
		ActorRef:   "operator:scope5-test",
		Surface:    SurfaceDigest,
		Action:     "surface_dismiss",
		Status:     "ok",
		ObservedAt: observedAt,
	})

	if envelope.Action != AuditActionCallbackAttempt {
		t.Fatalf("callback audit action = %q, want %q", envelope.Action, AuditActionCallbackAttempt)
	}
	if envelope.Outcome != AuditOutcomeOK {
		t.Fatalf("callback audit outcome = %q, want %q", envelope.Outcome, AuditOutcomeOK)
	}
	if envelope.Reason != "surface_dismiss" {
		t.Fatalf("callback audit reason = %q, want fallback to Action %q", envelope.Reason, "surface_dismiss")
	}
	if envelope.AuditEnvelopeVersion != AuditEnvelopeVersionV1 {
		t.Fatalf("callback audit envelope version = %q", envelope.AuditEnvelopeVersion)
	}
	if envelope.TS != observedAt.Format(time.RFC3339) {
		t.Fatalf("callback audit TS = %q", envelope.TS)
	}
}

func TestEmitCallbackAttemptAuditMapsErrorStatus(t *testing.T) {
	envelope := EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
		TraceID:    "trace-cb-err",
		Surface:    SurfaceDigest,
		Action:     "surface_engage",
		Status:     "error",
		Reason:     "signature_rejected",
		ObservedAt: time.Date(2026, 5, 22, 10, 35, 0, 0, time.UTC),
	})
	if envelope.Outcome != AuditOutcomeError {
		t.Fatalf("error callback outcome = %q, want %q", envelope.Outcome, AuditOutcomeError)
	}
	if envelope.Reason != "signature_rejected" {
		t.Fatalf("error callback reason = %q, want %q", envelope.Reason, "signature_rejected")
	}
}
