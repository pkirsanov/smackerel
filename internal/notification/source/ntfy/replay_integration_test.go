//go:build integration

package ntfy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfySinkFailureRetriesDeadLettersAndReplaysThroughSourceSink(t *testing.T) {
	ntfyStore, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	cfg := ntfyIntegrationConfig(prefix, notification.SourceFormWebhook, []string{"home-lab-alerts"})
	seedNtfyIntegrationSource(t, notificationStore, cfg)
	now := time.Date(2026, 5, 24, 23, 30, 0, 0, time.UTC)
	event, err := ParseEvent([]byte(`{"id":"evt-replay-int","event":"message","topic":"home-lab-alerts","title":"Replay","message":"sink recovery"}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse event: %v", err)
	}
	record, err := ntfyStore.CreateDeadLetter(context.Background(), NewDeadLetterRecord(cfg, event, event.Raw, DeadLetterSinkUnavailable, "source sink unavailable", true, now))
	if err != nil {
		t.Fatalf("create dead letter: %v", err)
	}
	attempt, err := ntfyStore.ReplayDeadLetter(context.Background(), cfg, record.ID, ntfyIntegrationService(t, notificationStore), "integration-operator", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("replay dead letter: %v", err)
	}
	if attempt.Status != "accepted" || attempt.RawEventID == "" || attempt.SinkStatus != "accepted" {
		t.Fatalf("replay attempt did not return accepted source sink receipt: %+v", attempt)
	}
	attemptCount := ntfyIntegrationCount(t, pool, "SELECT COUNT(*) FROM notification_ntfy_replay_attempts WHERE dead_letter_id = $1", record.ID)
	rawCount := ntfyIntegrationCount(t, pool, "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1 AND source_event_id = $2", cfg.SourceInstanceID, event.ID)
	if attemptCount != 1 || rawCount != 1 {
		t.Fatalf("replay did not persist attempt/raw event: attempts=%d raw=%d", attemptCount, rawCount)
	}
	page, err := ntfyStore.ListDeadLetters(context.Background(), cfg.SourceInstanceID, 1, "")
	if err != nil {
		t.Fatalf("list dead letters page 1: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].ReplayStatus != ReplayStatusReplayed {
		t.Fatalf("dead letter replay status not updated: %+v", page)
	}
}

func TestNtfyDeadLetterReplayBurstIsIdempotent(t *testing.T) {
	ntfyStore, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	cfg := ntfyIntegrationConfig(prefix, notification.SourceFormWebhook, []string{"home-lab-alerts"})
	seedNtfyIntegrationSource(t, notificationStore, cfg)
	now := time.Date(2026, 5, 24, 23, 45, 0, 0, time.UTC)
	event, err := ParseEvent([]byte(`{"id":"evt-replay-burst","event":"message","topic":"home-lab-alerts","title":"Replay burst","message":"operator retry burst"}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse event: %v", err)
	}
	record, err := ntfyStore.CreateDeadLetter(context.Background(), NewDeadLetterRecord(cfg, event, event.Raw, DeadLetterSinkUnavailable, "source sink unavailable", true, now))
	if err != nil {
		t.Fatalf("create dead letter: %v", err)
	}
	service := ntfyIntegrationService(t, notificationStore)
	first, err := ntfyStore.ReplayDeadLetter(context.Background(), cfg, record.ID, service, "integration-operator", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("first replay dead letter: %v", err)
	}
	second, err := ntfyStore.ReplayDeadLetter(context.Background(), cfg, record.ID, service, "integration-operator", now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("second replay dead letter: %v", err)
	}
	third, err := ntfyStore.ReplayDeadLetter(context.Background(), cfg, record.ID, service, "integration-operator", now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("third replay dead letter: %v", err)
	}
	for label, attempt := range map[string]ReplayAttemptRecord{"first": first, "second": second, "third": third} {
		if attempt.Status != "accepted" || attempt.RawEventID == "" || attempt.SinkStatus != "accepted" {
			t.Fatalf("%s replay attempt did not return accepted source sink receipt: %+v", label, attempt)
		}
	}
	if second.RawEventID != first.RawEventID || third.RawEventID != first.RawEventID {
		t.Fatalf("replayed attempts returned different raw events: first=%+v second=%+v third=%+v", first, second, third)
	}
	if first.AlreadyReplayed || !second.AlreadyReplayed || !third.AlreadyReplayed {
		t.Fatalf("already-replayed semantics mismatch: first=%+v second=%+v third=%+v", first, second, third)
	}
	attemptCount := ntfyIntegrationCount(t, pool, "SELECT COUNT(*) FROM notification_ntfy_replay_attempts WHERE dead_letter_id = $1", record.ID)
	rawCount := ntfyIntegrationCount(t, pool, "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1 AND source_event_id = $2", cfg.SourceInstanceID, event.ID)
	normalizedCount := ntfyIntegrationCount(t, pool, "SELECT COUNT(*) FROM normalized_notifications WHERE source_instance_id = $1 AND source_event_id = $2", cfg.SourceInstanceID, event.ID)
	if attemptCount != 1 || rawCount != 1 || normalizedCount != 1 {
		t.Fatalf("replay burst repeated side effects: attempts=%d raw=%d normalized=%d", attemptCount, rawCount, normalizedCount)
	}
}

func TestNtfyDeadLetterPressureThresholdReportsDegradedSourceHealth(t *testing.T) {
	ntfyStore, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	cfg := ntfyIntegrationConfig(prefix, notification.SourceFormWebhook, []string{"home-lab-alerts"})
	cfg.DeadLetter.PressureThresholdCount = 2
	seedNtfyIntegrationSource(t, notificationStore, cfg)
	adapter, err := NewAdapter(cfg, WithStore(ntfyStore))
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	sink := ntfyIntegrationService(t, notificationStore)
	handler := adapterWebhookHandler{adapter: adapter, sink: sink}
	nowErr := errors.New("malformed json token=secret-token")
	if err := handler.HandleNtfyPayloadError(context.Background(), []byte(`{"event":"message"`), nowErr); err != nil {
		t.Fatalf("record first malformed payload: %v", err)
	}
	if err := handler.HandleNtfyPayloadError(context.Background(), []byte(`{"event":"message","message":"second"`), nowErr); err != nil {
		t.Fatalf("record second malformed payload: %v", err)
	}
	statuses, err := notificationStore.ListSourceStatuses(context.Background())
	if err != nil {
		t.Fatalf("list source statuses: %v", err)
	}
	found := false
	for _, status := range statuses {
		if status.Config.SourceInstanceID != cfg.SourceInstanceID {
			continue
		}
		found = true
		if status.Health.State != notification.SourceHealthDegraded || status.Health.LastErrorKind != ErrorDeadLetterPressure {
			t.Fatalf("dead-letter pressure did not degrade source health: %+v", status.Health)
		}
		if status.Health.LastErrorRedacted != "source dead-letter pressure threshold exceeded" {
			t.Fatalf("dead-letter pressure health was not redacted with the pressure category: %+v", status.Health)
		}
	}
	if !found {
		t.Fatalf("ntfy source %s not listed after dead-letter pressure", cfg.SourceInstanceID)
	}
	rawCount := ntfyIntegrationCount(t, pool, "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1", cfg.SourceInstanceID)
	if rawCount != 0 {
		t.Fatalf("malformed dead-letter pressure accepted raw events: %d", rawCount)
	}
}

func ntfyIntegrationCount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count ntfy integration rows: %v", err)
	}
	return count
}
