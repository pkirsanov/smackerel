//go:build integration

package notification

import (
	"context"
	"testing"
	"time"
)

func TestIncidentCorrelationSuppressionAndTransitionsPersist(t *testing.T) {
	store, _ := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	service := notificationIntegrationService(t, store)
	first, err := service.Process(context.Background(), notificationIntegrationEnvelope(cfg, prefix+"-incident-a", "checkout-api outage failed", "high"), time.Now().UTC())
	if err != nil {
		t.Fatalf("process first notification: %v", err)
	}
	second, err := service.Process(context.Background(), notificationIntegrationEnvelope(cfg, prefix+"-incident-b", "checkout-api outage failed again", "high"), time.Now().UTC().Add(time.Second))
	if err != nil {
		t.Fatalf("process second notification: %v", err)
	}
	if first.Incident.ID != second.Incident.ID {
		t.Fatalf("related notifications did not correlate into one incident: first=%s second=%s", first.Incident.ID, second.Incident.ID)
	}
	incident, err := store.GetIncident(context.Background(), first.Incident.ID)
	if err != nil {
		t.Fatalf("get incident: %v", err)
	}
	if incident.PersistenceCount < 2 {
		t.Fatalf("incident persistence count = %d, want >=2", incident.PersistenceCount)
	}
}
