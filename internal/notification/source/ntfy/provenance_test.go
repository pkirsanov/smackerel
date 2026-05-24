package ntfy

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyProvenanceAndLoopMetadataArePreservedInEnvelope(t *testing.T) {
	base := testConfig()
	other := base
	other.SourceInstanceID = "ntfy-overlap-secondary"
	other.Topics = []string{"home-lab-alerts", "shared-alerts"}
	other.SourceForm = notification.SourceFormWebhook
	other.TransportMode = TransportModeWebhook
	other.Auth = AuthConfig{Mode: AuthModeNone}
	eventA, err := ParseEvent([]byte(`{"id":"duplicate-id","event":"message","topic":"home-lab-alerts","message":"primary","smackerel_loop_guard_key":"loop-a","smackerel_decision_id":"decision-a"}`), base.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse primary event: %v", err)
	}
	eventB, err := ParseEvent([]byte(`{"id":"duplicate-id","event":"message","topic":"shared-alerts","message":"secondary","smackerel_loop_guard_key":"loop-b","smackerel_output_trace_ref":"trace-b"}`), other.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse secondary event: %v", err)
	}
	now := time.Date(2026, 5, 24, 22, 45, 0, 0, time.UTC)
	envelopeA, err := MapEvent(context.Background(), base, eventA, now)
	if err != nil {
		t.Fatalf("map primary event: %v", err)
	}
	envelopeB, err := MapEvent(context.Background(), other, eventB, now)
	if err != nil {
		t.Fatalf("map secondary event: %v", err)
	}
	if envelopeA.SourceEventID != envelopeB.SourceEventID {
		t.Fatalf("fixture expected duplicate upstream IDs: %q vs %q", envelopeA.SourceEventID, envelopeB.SourceEventID)
	}
	if envelopeA.SourceInstanceID == envelopeB.SourceInstanceID || envelopeA.DeliveryMetadata["topic"] == envelopeB.DeliveryMetadata["topic"] {
		t.Fatalf("multi-instance/topic provenance collapsed: A=%+v B=%+v", envelopeA, envelopeB)
	}
	if envelopeA.LoopMetadata["decision_id"] != "decision-a" || envelopeB.LoopMetadata["output_trace_ref"] != "trace-b" {
		t.Fatalf("loop metadata did not survive mapping: A=%+v B=%+v", envelopeA.LoopMetadata, envelopeB.LoopMetadata)
	}
}
