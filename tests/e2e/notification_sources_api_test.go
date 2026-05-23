//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNotificationSourcesStatusShowsConnectedDisconnectedAndDegradedSources(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	store, pool, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	now := time.Date(2026, 5, 22, 6, 30, 0, 0, time.UTC)
	prefix := notificationE2EPrefix()
	seedNotificationSourceStatus(t, store, prefix+"-connected", "stream_fixture", notification.SourceFormStream, notification.SourceHealthConnected, 0, "", now)
	seedNotificationSourceStatus(t, store, prefix+"-disconnected", "webhook_fixture", notification.SourceFormWebhook, notification.SourceHealthDisconnected, 1, "invalid_credentials", now)
	seedNotificationSourceStatus(t, store, prefix+"-degraded", "polling_fixture", notification.SourceFormPolling, notification.SourceHealthDegraded, 4, "transient_failure", now)
	t.Cleanup(func() { cleanupNotificationE2ESources(t, pool, prefix) })

	resp, err := apiGet(cfg, "/api/notifications/sources")
	if err != nil {
		t.Fatalf("notification sources request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if strings.Contains(string(body), "password=hunter2") || strings.Contains(string(body), "secret-token-123") {
		t.Fatalf("source status response leaked sensitive raw error data: %s", string(body))
	}
	var parsed struct {
		Sources []struct {
			SourceInstanceID      string `json:"source_instance_id"`
			SourceType            string `json:"source_type"`
			SourceForm            string `json:"source_form"`
			HealthState           string `json:"health_state"`
			RetryCount            int    `json:"retry_count"`
			LastErrorKind         string `json:"last_error_kind"`
			LastErrorRedacted     string `json:"last_error_redacted"`
			LastEventAt           string `json:"last_event_at"`
			LastSuccessfulCheckAt string `json:"last_successful_check_at"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse notification sources: %v; body=%s", err, string(body))
	}
	seen := map[string]struct {
		State      string
		RetryCount int
		Error      string
		Form       string
		LastEvent  string
		LastCheck  string
	}{}
	for _, source := range parsed.Sources {
		if strings.HasPrefix(source.SourceInstanceID, prefix) {
			seen[source.SourceInstanceID] = struct {
				State      string
				RetryCount int
				Error      string
				Form       string
				LastEvent  string
				LastCheck  string
			}{State: source.HealthState, RetryCount: source.RetryCount, Error: source.LastErrorRedacted, Form: source.SourceForm, LastEvent: source.LastEventAt, LastCheck: source.LastSuccessfulCheckAt}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 notification test sources, got %d in %s", len(seen), string(body))
	}
	if seen[prefix+"-connected"].State != "connected" || seen[prefix+"-connected"].LastEvent == "" || seen[prefix+"-connected"].LastCheck == "" {
		t.Fatalf("connected source missing expected timestamps: %+v", seen[prefix+"-connected"])
	}
	if seen[prefix+"-disconnected"].State != "disconnected" || seen[prefix+"-disconnected"].Error != "source authentication failed" {
		t.Fatalf("disconnected source not redacted as expected: %+v", seen[prefix+"-disconnected"])
	}
	if seen[prefix+"-degraded"].State != "degraded" || seen[prefix+"-degraded"].RetryCount != 4 || seen[prefix+"-degraded"].Error != "transient source check failed" {
		t.Fatalf("degraded source not returned as expected: %+v", seen[prefix+"-degraded"])
	}
}

func TestNotificationSourcesRejectDuplicateInstanceIdsBeforeProcessing(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	store, pool, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	now := time.Date(2026, 5, 22, 6, 40, 0, 0, time.UTC)
	prefix := notificationE2EPrefix()
	instanceID := prefix + "-duplicate"
	t.Cleanup(func() { cleanupNotificationE2ESources(t, pool, prefix) })
	enabled := true
	first := notification.SourceInstanceConfig{SourceType: "queue_fixture", SourceInstanceID: instanceID, SourceForm: notification.SourceFormQueue, Enabled: &enabled, ConfigHash: "sha256:dup-a", SecretRefNames: []string{"QUEUE_TOKEN_REF"}, RedactedMetadata: map[string]string{"queue": "redacted"}}
	if _, err := store.CreateSourceInstance(context.Background(), first, now); err != nil {
		t.Fatalf("create first duplicate guard source: %v", err)
	}
	second := notification.SourceInstanceConfig{SourceType: "manual_fixture", SourceInstanceID: instanceID, SourceForm: notification.SourceFormManual, Enabled: &enabled, ConfigHash: "sha256:dup-b", SecretRefNames: []string{"MANUAL_TOKEN_REF"}, RedactedMetadata: map[string]string{"actor": "redacted"}}
	if _, err := store.CreateSourceInstance(context.Background(), second, now); err == nil {
		t.Fatal("expected duplicate source instance id to be rejected before event processing")
	}
	resp, err := apiGet(cfg, "/api/notifications/sources")
	if err != nil {
		t.Fatalf("notification sources request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if strings.Count(string(body), instanceID) != 1 {
		t.Fatalf("duplicate source instance appeared more than once in status response: %s", string(body))
	}
}

func notificationE2EStore(t *testing.T) (*notification.Store, *pgxpool.Pool, func()) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("e2e: DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	return notification.NewStore(pool), pool, pool.Close
}

func notificationE2EPrefix() string {
	return "notif-e2e-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "-")
}

func seedNotificationSourceStatus(t *testing.T, store *notification.Store, instanceID, sourceType string, form notification.SourceForm, state notification.SourceHealthState, retries int, errorKind string, observedAt time.Time) {
	t.Helper()
	enabled := true
	_, err := store.CreateSourceInstance(context.Background(), notification.SourceInstanceConfig{SourceType: sourceType, SourceInstanceID: instanceID, SourceForm: form, Enabled: &enabled, ConfigHash: "sha256:" + instanceID, SecretRefNames: []string{"SOURCE_TOKEN_REF"}, RedactedMetadata: map[string]string{"endpoint": "redacted"}}, observedAt)
	if err != nil {
		t.Fatalf("create source instance %s: %v", instanceID, err)
	}
	lastEvent := observedAt.Add(-2 * time.Minute)
	lastCheck := observedAt.Add(-time.Minute)
	report := notification.SourceHealthReport{SourceType: sourceType, SourceInstanceID: instanceID, SourceForm: form, State: state, LastEventAt: &lastEvent, LastSuccessfulCheckAt: &lastCheck, RetryCount: retries, LastErrorKind: errorKind, LastErrorRedacted: "password=hunter2 secret", ObservedAt: observedAt}
	if state == notification.SourceHealthDisconnected {
		report.LastEventAt = nil
		report.LastSuccessfulCheckAt = nil
	}
	if err := store.RecordSourceHealth(context.Background(), report); err != nil {
		t.Fatalf("record source health %s: %v", instanceID, err)
	}
}

func cleanupNotificationE2ESources(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "DELETE FROM notification_source_instances WHERE source_instance_id LIKE $1", prefix+"%")
	if err != nil {
		t.Logf("cleanup notification sources failed: %v", err)
	}
}
