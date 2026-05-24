//go:build integration

package ntfy

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyMessageAcceptedThroughSourceSinkCreatesRawAndNormalizedRecords(t *testing.T) {
	ntfyStore, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	cfg := ntfyIntegrationConfig(prefix, notification.SourceFormWebhook, []string{"home-lab-alerts"})
	seedNtfyIntegrationSource(t, notificationStore, cfg)
	service := ntfyIntegrationService(t, notificationStore)
	adapter, err := NewAdapter(cfg, WithStore(ntfyStore))
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	event, err := ParseEvent([]byte(`{"id":"evt-int-ingest","event":"message","topic":"home-lab-alerts","title":"Integration raw","message":"normalized through core","priority":4,"tags":["disk"]}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse event: %v", err)
	}
	if err := adapter.handleTransportEvent(context.Background(), service, event); err != nil {
		t.Fatalf("handle event: %v", err)
	}
	var rawCount, normalizedCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1 AND source_event_id = $2", cfg.SourceInstanceID, event.ID).Scan(&rawCount); err != nil {
		t.Fatalf("count raw events: %v", err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM normalized_notifications WHERE source_instance_id = $1 AND source_event_id = $2", cfg.SourceInstanceID, event.ID).Scan(&normalizedCount); err != nil {
		t.Fatalf("count normalized notifications: %v", err)
	}
	if rawCount != 1 || normalizedCount != 1 {
		t.Fatalf("expected one raw and normalized record, got raw=%d normalized=%d", rawCount, normalizedCount)
	}
	states, err := ntfyStore.ListSubscriptionStates(context.Background(), cfg.SourceInstanceID)
	if err != nil {
		t.Fatalf("list topic states: %v", err)
	}
	if len(states) != 1 || states[0].SubscriptionState != SubscriptionConnected || states[0].LastNtfyEventID != event.ID {
		t.Fatalf("connected topic state not persisted: %+v", states)
	}
	lifecycle, err := ParseEvent([]byte(`{"event":"keepalive","topic":"home-lab-alerts"}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse lifecycle event: %v", err)
	}
	if err := adapter.handleTransportEvent(context.Background(), service, lifecycle); err != nil {
		t.Fatalf("handle lifecycle event: %v", err)
	}
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM normalized_notifications WHERE source_instance_id = $1 AND source_event_id = ''", cfg.SourceInstanceID).Scan(&normalizedCount); err != nil {
		t.Fatalf("count lifecycle normalized notifications: %v", err)
	}
	if normalizedCount != 0 {
		t.Fatalf("lifecycle event created normalized notifications: %d", normalizedCount)
	}
}
