//go:build integration

package notification

import (
	"context"
	"testing"
	"time"
)

func TestRawEventIsCommittedBeforeNormalizedNotification(t *testing.T) {
	store, pool := notificationIntegrationStore(t)
	prefix := notificationIntegrationPrefix(t)
	cfg := seedNotificationIntegrationSource(t, store, prefix)
	envelope := notificationIntegrationEnvelope(cfg, prefix+"-raw-first", "checkout-api latency threshold", "high")
	raw, err := store.CreateRawEvent(context.Background(), envelope, time.Now().UTC())
	if err != nil {
		t.Fatalf("create raw event: %v", err)
	}
	var rawCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM notification_raw_events WHERE id = $1", raw.ID).Scan(&rawCount); err != nil {
		t.Fatalf("count raw event: %v", err)
	}
	if rawCount != 1 {
		t.Fatalf("raw event count before normalization = %d", rawCount)
	}
	normalized, err := NewNormalizer().Normalize(raw, envelope)
	if err != nil {
		t.Fatalf("normalize event: %v", err)
	}
	if err := store.CreateNormalizedNotification(context.Background(), normalized); err != nil {
		t.Fatalf("create normalized notification: %v", err)
	}
}
