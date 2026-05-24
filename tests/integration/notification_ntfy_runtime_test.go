//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
)

func TestNtfyRuntimeStartsConfiguredWebhookAdapterAndSubmitsObservedMessages(t *testing.T) {
	cfg := ntfyIntegrationConfig()
	rawConfig := `[{
		"source_instance_id":"ntfy-integration-webhook",
		"enabled":true,
		"source_form":"webhook",
		"transport_mode":"webhook",
		"endpoint_url":"http://smackerel-core:8080/api/notifications/sources/ntfy-integration-webhook/ntfy/webhook",
		"endpoint_ref_name":"NTFY_INTEGRATION_WEBHOOK_ENDPOINT_URL",
		"topics":["home-lab-alerts"],
		"auth_mode":"none",
		"secret_ref_names":[],
		"default_domain":"ops",
		"retry_budget":3,
		"initial_delay_seconds":1,
		"max_delay_seconds":5,
		"keepalive_timeout_seconds":30,
		"lag_degraded_after_seconds":60,
		"lag_disconnected_after_seconds":300,
		"dead_letter_retry_budget":2,
		"max_payload_bytes":4096,
		"pressure_threshold_count":2,
		"display_name":"ntfy integration webhook",
		"endpoint_label":"integration webhook endpoint",
		"config_hash":"sha256:ntfy-integration-webhook"
	}]`
	registry := ntfysource.NewWebhookReceiverRegistry()
	sink := &integrationRecordingSink{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runtime, err := ntfysource.StartConfiguredAdapters(ctx, rawConfig, sink, ntfysource.WithRuntimeWebhookReceiver(registry))
	if err != nil {
		t.Fatalf("start configured adapters: %v", err)
	}
	defer func() {
		if err := runtime.Stop(context.Background()); err != nil {
			t.Fatalf("stop runtime: %v", err)
		}
	}()
	waitForIntegrationWebhookRegistration(t, registry, cfg.SourceInstanceID)
	if err := registry.ReceiveRaw(context.Background(), cfg.SourceInstanceID, []byte(`{"id":"evt-integration-webhook","event":"message","topic":"home-lab-alerts","title":"Integration","message":"webhook runtime"}`)); err != nil {
		t.Fatalf("receive webhook: %v", err)
	}
	if got := sink.waitForEnvelopeCount(t, 1); got[0].SourceEventID != "evt-integration-webhook" {
		t.Fatalf("unexpected integration envelope: %+v", got)
	}
	if err := registry.ReceiveRaw(context.Background(), cfg.SourceInstanceID, []byte(`{"event":"message"`)); !ntfysource.IsWebhookPayloadInvalid(err) {
		t.Fatalf("expected malformed webhook payload error, got %v", err)
	}
}

func waitForIntegrationWebhookRegistration(t *testing.T, registry *ntfysource.WebhookReceiverRegistry, sourceInstanceID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registry.IsRegistered(sourceInstanceID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for webhook source %s registration", sourceInstanceID)
}

func ntfyIntegrationConfig() ntfysource.Config {
	return ntfysource.Config{Enabled: true, SourceInstanceID: "ntfy-integration-webhook", SourceForm: notification.SourceFormWebhook, TransportMode: ntfysource.TransportModeWebhook, EndpointURL: "http://smackerel-core:8080/api/notifications/sources/ntfy-integration-webhook/ntfy/webhook", EndpointRefName: "NTFY_INTEGRATION_WEBHOOK_ENDPOINT_URL", Topics: []string{"home-lab-alerts"}, Auth: ntfysource.AuthConfig{Mode: ntfysource.AuthModeNone}, Reconnect: ntfysource.ReconnectConfig{RetryBudget: 3, InitialDelaySeconds: 1, MaxDelaySeconds: 5, KeepaliveTimeoutSeconds: 30}, Lag: ntfysource.LagConfig{DegradedAfterSeconds: 60, DisconnectedAfterSeconds: 300}, DeadLetter: ntfysource.DeadLetterConfig{RetryBudget: 2, MaxPayloadBytes: 4096, PressureThresholdCount: 2}, RedactedMetadata: map[string]string{"display_name": "ntfy integration webhook", "endpoint_label": "integration webhook endpoint"}, ConfigHash: "sha256:ntfy-integration-webhook"}
}

type integrationRecordingSink struct {
	envelopes []notification.SourceEventEnvelope
	health    []notification.SourceHealthReport
}

func (sink *integrationRecordingSink) SubmitSourceEvent(ctx context.Context, envelope notification.SourceEventEnvelope) (notification.IngestReceipt, error) {
	sink.envelopes = append(sink.envelopes, envelope)
	return notification.IngestReceipt{SourceType: envelope.SourceType, SourceInstanceID: envelope.SourceInstanceID, SourceForm: envelope.SourceForm, RawEventID: "raw-" + envelope.SourceEventID, Accepted: true, Status: "accepted"}, nil
}

func (sink *integrationRecordingSink) ReportSourceHealth(ctx context.Context, report notification.SourceHealthReport) error {
	sink.health = append(sink.health, report)
	return nil
}

func (sink *integrationRecordingSink) waitForEnvelopeCount(t *testing.T, want int) []notification.SourceEventEnvelope {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(sink.envelopes) >= want {
			return append([]notification.SourceEventEnvelope(nil), sink.envelopes...)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d envelopes; got %d", want, len(sink.envelopes))
	return nil
}
