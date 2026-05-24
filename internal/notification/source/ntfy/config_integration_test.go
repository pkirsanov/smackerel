//go:build integration

package ntfy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyInvalidEnabledInstanceRegistersDisconnectedHealthAndAcceptsNoEvents(t *testing.T) {
	_, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	cfg := ntfyIntegrationConfig(prefix, notification.SourceFormWebhook, []string{"home-lab-alerts"})
	seedNtfyIntegrationSource(t, notificationStore, cfg)
	now := time.Date(2026, 5, 24, 23, 0, 0, 0, time.UTC)
	if err := notificationStore.RecordSourceHealth(context.Background(), AuthFailureHealth(cfg, now)); err != nil {
		t.Fatalf("record auth failure health: %v", err)
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
		if status.Health.State != notification.SourceHealthDisconnected || status.Health.LastErrorKind != ErrorAuthFailed {
			t.Fatalf("auth failure status mismatch: %+v", status)
		}
		if len(status.Config.SecretRefNames) != 0 || status.Config.RedactedMetadata["auth_mode"] != AuthModeNone {
			t.Fatalf("auth_mode=none source identity not preserved: %+v", status.Config)
		}
	}
	if !found {
		t.Fatalf("ntfy source %s not listed", cfg.SourceInstanceID)
	}
	var rawCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1", cfg.SourceInstanceID).Scan(&rawCount); err != nil {
		t.Fatalf("count raw events: %v", err)
	}
	if rawCount != 0 {
		t.Fatalf("invalid/auth-failed source accepted raw events: %d", rawCount)
	}
}

func TestNtfyBootstrapInvalidEnabledMissingFieldConfigsRegisterDisconnectedHealthRows(t *testing.T) {
	_, notificationStore, pool := ntfyIntegrationStores(t)
	prefix := ntfyIntegrationPrefix()
	now := time.Date(2026, 5, 24, 23, 45, 0, 0, time.UTC)
	items := []string{
		`{"enabled":true,"source_form":"webhook","transport_mode":"webhook","endpoint_url":"http://smackerel-core:8080/api/notifications/sources/missing-id/ntfy/webhook","endpoint_ref_name":"NTFY_MISSING_ID_ENDPOINT_URL","topics":["home-lab-alerts"],"auth_mode":"none","retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"missing id","endpoint_label":"invalid endpoint","config_hash":"sha256:missing-id"}`,
		`{"enabled":true,"source_instance_id":"` + prefix + `-missing-form","transport_mode":"webhook","endpoint_url":"http://smackerel-core:8080/api/notifications/sources/missing-form/ntfy/webhook","endpoint_ref_name":"NTFY_MISSING_FORM_ENDPOINT_URL","topics":["home-lab-alerts"],"auth_mode":"none","retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"missing form","endpoint_label":"invalid endpoint","config_hash":"sha256:missing-form"}`,
		`{"enabled":true,"source_instance_id":"` + prefix + `-missing-endpoint","source_form":"webhook","transport_mode":"webhook","topics":["home-lab-alerts"],"auth_mode":"none","retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"missing endpoint","endpoint_label":"invalid endpoint","config_hash":"sha256:missing-endpoint"}`,
		`{"enabled":true,"source_instance_id":"` + prefix + `-missing-topics","source_form":"webhook","transport_mode":"webhook","endpoint_url":"http://smackerel-core:8080/api/notifications/sources/missing-topics/ntfy/webhook","endpoint_ref_name":"NTFY_MISSING_TOPICS_ENDPOINT_URL","auth_mode":"none","retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"missing topics","endpoint_label":"invalid endpoint","config_hash":"sha256:missing-topics"}`,
		`{"enabled":true,"source_instance_id":"` + prefix + `-missing-secret-ref","source_form":"webhook","transport_mode":"webhook","endpoint_url":"http://smackerel-core:8080/api/notifications/sources/missing-secret-ref/ntfy/webhook","endpoint_ref_name":"NTFY_MISSING_SECRET_ENDPOINT_URL","topics":["home-lab-alerts"],"auth_mode":"bearer_token","secret_ref_names":[],"retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"missing secret","endpoint_label":"invalid endpoint","config_hash":"sha256:missing-secret"}`,
	}
	err := BootstrapConfiguredSources(context.Background(), "["+strings.Join(items, ",")+"]", notificationStore, now)
	if err == nil {
		t.Fatal("expected invalid enabled ntfy configs to fail loud after recording health")
	}
	expected := map[string]string{
		diagnosticSourceInstanceID(Config{}, 0, []byte(items[0])): ErrorMissingSourceInstanceID,
		prefix + "-missing-form":                                  ErrorMissingSourceForm,
		prefix + "-missing-endpoint":                              ErrorMissingEndpoint,
		prefix + "-missing-topics":                                ErrorMissingTopics,
		prefix + "-missing-secret-ref":                            ErrorCredentialRefMissing,
	}
	statuses, err := notificationStore.ListSourceStatuses(context.Background())
	if err != nil {
		t.Fatalf("list source statuses: %v", err)
	}
	found := map[string]notification.SourceStatus{}
	for _, status := range statuses {
		if _, ok := expected[status.Config.SourceInstanceID]; ok {
			found[status.Config.SourceInstanceID] = status
		}
	}
	for sourceID, errorKind := range expected {
		status, ok := found[sourceID]
		if !ok {
			t.Fatalf("invalid enabled config %s did not register a source status row; statuses=%+v", sourceID, statuses)
		}
		if status.Health.State != notification.SourceHealthDisconnected || status.Health.LastErrorKind != errorKind {
			t.Fatalf("invalid config health mismatch for %s: %+v", sourceID, status.Health)
		}
		if status.Config.RedactedMetadata["config_status"] != "invalid" || status.Config.RedactedMetadata["config_error_kind"] != errorKind {
			t.Fatalf("invalid config metadata missing redacted error category for %s: %+v", sourceID, status.Config.RedactedMetadata)
		}
	}
	var rawCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = ANY($1)", []string{prefix + "-missing-form", prefix + "-missing-endpoint", prefix + "-missing-topics", prefix + "-missing-secret-ref"}).Scan(&rawCount); err != nil {
		t.Fatalf("count raw events for invalid configs: %v", err)
	}
	if rawCount != 0 {
		t.Fatalf("invalid enabled config accepted raw events: %d", rawCount)
	}
}
