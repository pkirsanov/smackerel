//go:build integration

package notification

import (
	"context"
	"testing"
	"time"
)

func TestClassificationPersistenceAndAuditRetrieval(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)
	result, err := service.Process(context.Background(), notificationIntegrationEnvelope(cfg, prefix+"-classify", "checkout-api outage failed", "critical"), time.Now().UTC())
	if err != nil {
		t.Fatalf("process notification: %v", err)
	}
	detail, err := store.GetEventDetail(context.Background(), result.Notification.ID)
	if err != nil {
		t.Fatalf("get event detail: %v", err)
	}
	if detail.Classification == nil || detail.Classification.Rationale == "" || detail.Classification.Confidence <= 0 {
		t.Fatalf("classification audit missing from detail: %+v", detail.Classification)
	}
}
