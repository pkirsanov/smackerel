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
