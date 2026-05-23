package notification

import (
	"testing"
	"time"
)

func TestLoopGuardSuppressesReentrantOutputEvents(t *testing.T) {
	guard := NewLoopGuard(10 * time.Minute)
	origin := LoopOrigin{DecisionID: "decision-a", OutputChannel: "dashboard", PayloadHash: "payload-a", EmittedAt: time.Date(2026, 5, 22, 7, 30, 0, 0, time.UTC)}
	reentrant := SourceEventEnvelope{SourceType: "webhook_fixture", SourceInstanceID: "source-a", SourceForm: SourceFormWebhook, SourceEventID: "event-loop", ObservedAt: origin.EmittedAt.Add(time.Minute), RawPayloadKind: RawPayloadKindText, RawPayload: []byte("payload"), DeliveryMetadata: map[string]string{"loop_guard_key": origin.Key()}, LoopMetadata: map[string]string{"loop_guard_key": origin.Key()}}
	suppression := guard.Evaluate(reentrant, []LoopOrigin{origin})
	if suppression == nil {
		t.Fatal("expected reentrant event suppression")
	}
	if suppression.Kind != SuppressionReactionLoop || suppression.Reason == "" {
		t.Fatalf("unexpected loop suppression: %+v", suppression)
	}
	if suppression.ID != "" {
		t.Fatalf("loop guard should let the store assign a unique audit id, got %q", suppression.ID)
	}
}
