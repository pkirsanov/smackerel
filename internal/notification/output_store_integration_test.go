//go:build integration

package notification

import (
	"context"
	"testing"
	"time"
)

func TestOutputDeliveryAttemptsPersistWithoutIncidentPolicyMutation(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)
	result, err := service.Process(context.Background(), notificationIntegrationEnvelope(cfg, prefix+"-output", "checkout-api outage failed", "high"), time.Now().UTC())
	if err != nil {
		t.Fatalf("process notification: %v", err)
	}
	deliveries, err := store.ListDeliveries(context.Background(), 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	found := false
	for _, delivery := range deliveries {
		if delivery.DecisionID == result.Decision.ID {
			found = true
			if delivery.Status != "queued" || delivery.Channel != "dashboard" {
				t.Fatalf("unexpected delivery attempt: %+v", delivery)
			}
		}
	}
	if !found {
		t.Fatalf("delivery attempt for decision %s not found", result.Decision.ID)
	}
}
