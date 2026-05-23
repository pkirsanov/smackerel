//go:build integration

package notification

import (
	"context"
	"testing"
	"time"
)

func TestEnrichmentAndDecisionRecordsPersistWithRationale(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)
	result, err := service.Process(context.Background(), notificationIntegrationEnvelope(cfg, prefix+"-decision", "checkout-api outage failed", "high"), time.Now().UTC())
	if err != nil {
		t.Fatalf("process notification: %v", err)
	}
	if result.Decision.ID == "" || result.Decision.Rationale == "" || len(result.Decision.ReasonCodes) == 0 {
		t.Fatalf("decision record missing rationale: %+v", result.Decision)
	}
	detail, err := store.GetEventDetail(context.Background(), result.Notification.ID)
	if err != nil {
		t.Fatalf("get detail: %v", err)
	}
	if detail.Decision == nil || detail.Decision.DecisionType == "" {
		t.Fatalf("decision detail missing: %+v", detail.Decision)
	}
}
