//go:build integration

package ntfy

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyMultiTopicAndMultiInstanceEventsDoNotCollapseIdentity(t *testing.T) {
	_, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	primary := ntfyIntegrationConfig(prefix+"-a", notification.SourceFormWebhook, []string{"self-hosted-alerts"})
	secondary := ntfyIntegrationConfig(prefix+"-b", notification.SourceFormWebhook, []string{"shared-alerts"})
	seedNtfyIntegrationSource(t, notificationStore, primary)
	seedNtfyIntegrationSource(t, notificationStore, secondary)
	service := ntfyIntegrationService(t, notificationStore)
	for _, item := range []struct {
		cfg   Config
		topic string
	}{
		{cfg: primary, topic: "self-hosted-alerts"},
		{cfg: secondary, topic: "shared-alerts"},
	} {
		event, err := ParseEvent([]byte(`{"id":"same-upstream-id","event":"message","topic":"`+item.topic+`","message":"overlap"}`), item.cfg.DeadLetter.MaxPayloadBytes)
		if err != nil {
			t.Fatalf("parse event: %v", err)
		}
		envelope, err := MapEvent(context.Background(), item.cfg, event, nowUTCForIntegration())
		if err != nil {
			t.Fatalf("map event: %v", err)
		}
		if _, err := service.SubmitSourceEvent(context.Background(), envelope); err != nil {
			t.Fatalf("submit event: %v", err)
		}
	}
	var distinct int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(DISTINCT source_instance_id || ':' || source_event_id) FROM notification_raw_events WHERE source_event_id = 'same-upstream-id' AND source_instance_id LIKE $1", prefix+"%").Scan(&distinct); err != nil {
		t.Fatalf("count distinct provenance: %v", err)
	}
	if distinct != 2 {
		t.Fatalf("overlapping ntfy events collapsed provenance: distinct=%d", distinct)
	}
}

func nowUTCForIntegration() time.Time {
	return time.Now().UTC()
}
