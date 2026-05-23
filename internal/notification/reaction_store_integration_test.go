//go:build integration

package notification

import (
	"context"
	"testing"
	"time"
)

func TestDiagnosticsActionsApprovalsAndLoopGuardsPersist(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)
	result, err := service.Process(context.Background(), notificationIntegrationEnvelope(cfg, prefix+"-reaction", "checkout-api outage failed", "high"), time.Now().UTC())
	if err != nil {
		t.Fatalf("process notification: %v", err)
	}
	if result.Decision.ID == "" {
		t.Fatalf("decision not persisted: %+v", result)
	}
	loop := NewLoopGuard(5*time.Minute).Evaluate(SourceEventEnvelope{SourceType: cfg.SourceType, SourceInstanceID: cfg.SourceInstanceID, SourceForm: cfg.SourceForm, ObservedAt: time.Now().UTC(), RawPayloadKind: RawPayloadKindText, RawPayload: []byte("handler output"), DeliveryMetadata: map[string]string{"loop_guard_key": "loop-a"}, LoopMetadata: map[string]string{"loop_guard_key": "loop-a"}}, []LoopOrigin{{DecisionID: "decision-a", OutputChannel: "dashboard", PayloadHash: "payload-a", EmittedAt: time.Now().UTC()}})
	if loop != nil && loop.Kind != SuppressionReactionLoop {
		t.Fatalf("loop guard produced wrong suppression: %+v", loop)
	}
}
