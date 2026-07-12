package ntfy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyConfigValidationRequiresExplicitEnabledInstanceFieldsAndSecretReferences(t *testing.T) {
	valid := testConfig()
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "source instance id", mutate: func(cfg *Config) { cfg.SourceInstanceID = "" }, wantErr: "source instance id"},
		{name: "source form", mutate: func(cfg *Config) { cfg.SourceForm = "" }, wantErr: "source form"},
		{name: "transport mode", mutate: func(cfg *Config) { cfg.TransportMode = "" }, wantErr: "transport mode"},
		{name: "endpoint identity", mutate: func(cfg *Config) { cfg.EndpointURL = "" }, wantErr: "endpoint"},
		{name: "topic set", mutate: func(cfg *Config) { cfg.Topics = nil }, wantErr: "topic"},
		{name: "secret reference", mutate: func(cfg *Config) { cfg.Auth.SecretRefNames = nil }, wantErr: "secret reference"},
		{name: "config hash", mutate: func(cfg *Config) { cfg.ConfigHash = "" }, wantErr: "config hash"},
		{name: "redacted metadata", mutate: func(cfg *Config) { cfg.RedactedMetadata = nil }, wantErr: "redacted metadata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.mutate(&cfg)
			_, err := cfg.SourceInstanceConfig()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErr) {
				t.Fatalf("expected %q validation error, got %v", tc.wantErr, err)
			}
		})
	}

	noAuth := valid
	noAuth.Auth = AuthConfig{Mode: AuthModeNone}
	instance, err := noAuth.SourceInstanceConfig()
	if err != nil {
		t.Fatalf("auth_mode=none should explicitly allow zero secret refs: %v", err)
	}
	if len(instance.SecretRefNames) != 0 || instance.RedactedMetadata["auth_mode"] != string(AuthModeNone) {
		t.Fatalf("auth none instance did not preserve explicit auth metadata: %+v", instance)
	}
}

func TestNtfyAuthFailureReportsOnlyRedactedCredentialCategories(t *testing.T) {
	cfg := testConfig()
	now := time.Date(2026, 5, 23, 4, 0, 0, 0, time.UTC)
	report := AuthFailureHealth(cfg, now)
	if report.State != notification.SourceHealthDisconnected || report.LastErrorKind != ErrorAuthFailed {
		t.Fatalf("auth failure health = %+v", report)
	}
	if strings.Contains(report.LastErrorRedacted, "ntfy-secret-token") || strings.Contains(report.LastErrorRedacted, cfg.Auth.SecretRefNames[0]) {
		t.Fatalf("auth failure health leaked secret material or ref value: %+v", report)
	}
}

func TestNtfyRuntimeRejectsInvalidEnabledConfigWithoutFallbacks(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			name:    "missing source form",
			raw:     `[{"enabled":true,"source_instance_id":"ntfy-broken-source-form"}]`,
			wantErr: "source form",
		},
		{
			name:    "missing endpoint identity",
			raw:     `[{"enabled":true,"source_instance_id":"ntfy-broken-endpoint","source_form":"webhook","transport_mode":"webhook","topics":["self-hosted-alerts"],"auth_mode":"none","retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"broken","endpoint_label":"missing","config_hash":"sha256:broken"}]`,
			wantErr: "endpoint",
		},
		{
			name:    "missing topic set",
			raw:     `[{"enabled":true,"source_instance_id":"ntfy-broken-topics","source_form":"webhook","transport_mode":"webhook","endpoint_url":"http://smackerel-core:8080/api/notifications/sources/ntfy-broken-topics/ntfy/webhook","endpoint_ref_name":"NTFY_BROKEN_ENDPOINT_URL","auth_mode":"none","retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"broken","endpoint_label":"missing","config_hash":"sha256:broken"}]`,
			wantErr: "topic",
		},
		{
			name:    "credential backed source without secret references",
			raw:     `[{"enabled":true,"source_instance_id":"ntfy-broken-secret-ref","source_form":"webhook","transport_mode":"webhook","endpoint_url":"http://smackerel-core:8080/api/notifications/sources/ntfy-broken-secret-ref/ntfy/webhook","endpoint_ref_name":"NTFY_BROKEN_ENDPOINT_URL","topics":["self-hosted-alerts"],"auth_mode":"bearer_token","secret_ref_names":[],"retry_budget":3,"initial_delay_seconds":1,"max_delay_seconds":5,"keepalive_timeout_seconds":30,"lag_degraded_after_seconds":60,"lag_disconnected_after_seconds":300,"dead_letter_retry_budget":2,"max_payload_bytes":4096,"pressure_threshold_count":2,"display_name":"broken","endpoint_label":"missing","config_hash":"sha256:broken"}]`,
			wantErr: "secret reference",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &recordingSourceSink{}
			_, err := StartConfiguredAdapters(context.Background(), tc.raw, sink)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.wantErr) {
				t.Fatalf("expected fail-loud %q error, got %v", tc.wantErr, err)
			}
			if sink.envelopeCount() != 0 {
				t.Fatalf("invalid enabled config accepted %d source event(s)", sink.envelopeCount())
			}
		})
	}
}

func testConfig() Config {
	return Config{
		Enabled:          true,
		SourceInstanceID: "ntfy-self-hosted-alerts",
		SourceForm:       notification.SourceFormStream,
		TransportMode:    TransportModeStream,
		EndpointURL:      "https://ntfy.invalid",
		EndpointRefName:  "NTFY_SELF_HOSTED_ENDPOINT_URL",
		Topics:           []string{"self-hosted-alerts"},
		Auth:             AuthConfig{Mode: AuthModeBearerToken, SecretRefNames: []string{"NTFY_SELF_HOSTED_TOKEN"}},
		Mapping: MappingConfig{
			DefaultDomain: "ops",
			TopicSubjects: map[string]string{"self-hosted-alerts": "self-hosted"},
			TagServices:   map[string]string{"disk": "storage"},
			TagIntents:    map[string]string{"urgent": "investigate"},
		},
		Reconnect:        ReconnectConfig{RetryBudget: 3, InitialDelaySeconds: 1, MaxDelaySeconds: 5, KeepaliveTimeoutSeconds: 30},
		Lag:              LagConfig{DegradedAfterSeconds: 60, DisconnectedAfterSeconds: 300},
		DeadLetter:       DeadLetterConfig{RetryBudget: 2, MaxPayloadBytes: 4096, PressureThresholdCount: 2},
		RedactedMetadata: map[string]string{"display_name": "ntfy self-hosted environment alerts", "endpoint_label": "operator-managed ntfy endpoint"},
		ConfigHash:       "sha256:test-config",
	}
}
