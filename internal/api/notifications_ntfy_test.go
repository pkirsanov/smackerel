package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
)

func TestNtfyProductionWebhookRouteDispatchesReceiverAndRejectsMalformedCases(t *testing.T) {
	status := validNtfyWebhookSourceStatusForMetadataTest()
	registry := ntfysource.NewWebhookReceiverRegistry()
	cfg, err := ntfyConfigFromStatus(status)
	if err != nil {
		t.Fatalf("reconstruct ntfy webhook config: %v", err)
	}
	sink := &apiRecordingSourceSink{}
	adapter, err := ntfysource.NewAdapter(cfg, ntfysource.WithWebhookReceiver(registry))
	if err != nil {
		t.Fatalf("new webhook adapter: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := adapter.Start(ctx, sink); err != nil {
		t.Fatalf("start webhook adapter: %v", err)
	}
	waitForAPIWebhookRegistration(t, registry, cfg.SourceInstanceID)
	defer func() {
		if err := adapter.Stop(context.Background()); err != nil {
			t.Fatalf("stop webhook adapter: %v", err)
		}
	}()
	handler := &NotificationHandlers{sources: staticNotificationSourceStatusProvider{statuses: []notification.SourceStatus{status}}, ntfyWebhooks: registry}

	validReq := ntfyWebhookRequest(t, status.Config.SourceInstanceID, `{"id":"evt-route","event":"message","topic":"home-lab-alerts","title":"Route","message":"production webhook"}`)
	validRec := httptest.NewRecorder()
	handler.ReceiveNtfyWebhook(validRec, validReq)
	if validRec.Code != http.StatusAccepted {
		t.Fatalf("valid webhook status = %d, want 202; body=%s", validRec.Code, validRec.Body.String())
	}
	if sink.envelopeCount() != 1 {
		t.Fatalf("webhook route did not submit through source sink; envelopes=%d", sink.envelopeCount())
	}

	malformedReq := ntfyWebhookRequest(t, status.Config.SourceInstanceID, `{"event":"message"`)
	malformedRec := httptest.NewRecorder()
	handler.ReceiveNtfyWebhook(malformedRec, malformedReq)
	if malformedRec.Code != http.StatusBadRequest || !strings.Contains(malformedRec.Body.String(), "invalid_ntfy_webhook_payload") {
		t.Fatalf("malformed webhook status/body = %d %s", malformedRec.Code, malformedRec.Body.String())
	}

	wrongTopicReq := ntfyWebhookRequest(t, status.Config.SourceInstanceID, `{"id":"evt-wrong-topic","event":"message","topic":"not-configured","message":"wrong topic"}`)
	wrongTopicRec := httptest.NewRecorder()
	handler.ReceiveNtfyWebhook(wrongTopicRec, wrongTopicReq)
	if wrongTopicRec.Code != http.StatusBadRequest || !strings.Contains(wrongTopicRec.Body.String(), "ntfy_webhook_rejected") {
		t.Fatalf("wrong-topic webhook status/body = %d %s", wrongTopicRec.Code, wrongTopicRec.Body.String())
	}

	notRunningHandler := &NotificationHandlers{sources: staticNotificationSourceStatusProvider{statuses: []notification.SourceStatus{status}}, ntfyWebhooks: ntfysource.NewWebhookReceiverRegistry()}
	notRunningReq := ntfyWebhookRequest(t, status.Config.SourceInstanceID, `{"id":"evt-not-running","event":"message","topic":"home-lab-alerts","message":"not running"}`)
	notRunningRec := httptest.NewRecorder()
	notRunningHandler.ReceiveNtfyWebhook(notRunningRec, notRunningReq)
	if notRunningRec.Code != http.StatusServiceUnavailable || !strings.Contains(notRunningRec.Body.String(), "ntfy_webhook_receiver_unavailable") {
		t.Fatalf("not-running webhook status/body = %d %s", notRunningRec.Code, notRunningRec.Body.String())
	}

	streamStatus := validNtfySourceStatusForMetadataTest()
	streamHandler := &NotificationHandlers{sources: staticNotificationSourceStatusProvider{statuses: []notification.SourceStatus{streamStatus}}, ntfyWebhooks: registry}
	streamReq := ntfyWebhookRequest(t, streamStatus.Config.SourceInstanceID, `{"id":"evt-stream","event":"message","topic":"home-lab-alerts","message":"stream"}`)
	streamRec := httptest.NewRecorder()
	streamHandler.ReceiveNtfyWebhook(streamRec, streamReq)
	if streamRec.Code != http.StatusBadRequest || !strings.Contains(streamRec.Body.String(), "invalid_ntfy_webhook_source") {
		t.Fatalf("stream-source webhook status/body = %d %s", streamRec.Code, streamRec.Body.String())
	}
}

func TestNtfyDeadLetterAPIResponseRedactsReplayEligibleRawPayload(t *testing.T) {
	status := validNtfyWebhookSourceStatusForMetadataTest()
	cfg, err := ntfyConfigFromStatus(status)
	if err != nil {
		t.Fatalf("reconstruct ntfy webhook config: %v", err)
	}
	rawPayload := []byte(`{"id":"evt-api-redaction","event":"message","topic":"home-lab-alerts","message":"operator-visible metadata remains","api_key":"sk_live_055","token":"ntfy-token-055","password":"hunter2","Authorization":"Bearer secret-token-055"}`)
	event, err := ntfysource.ParseEvent(rawPayload, cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse replay-eligible event: %v", err)
	}
	record := ntfysource.NewDeadLetterRecord(cfg, event, rawPayload, ntfysource.DeadLetterSinkUnavailable, "sink failed Authorization: Bearer secret-token-055 password=hunter2", true, time.Date(2026, 5, 24, 23, 55, 0, 0, time.UTC))
	if len(record.RawPayload) == 0 || !strings.Contains(string(record.RawPayload), "sk_live_055") {
		t.Fatalf("test record must retain internal replay bytes before API serialization: %+v", record)
	}

	encodedResponse, err := json.Marshal(map[string]any{"dead_letter": redactedNtfyDeadLetterResponse(record)})
	if err != nil {
		t.Fatalf("marshal redacted API response: %v", err)
	}
	body := string(encodedResponse)
	lowerBody := strings.ToLower(body)
	rawPayloadBase64 := strings.ToLower(base64.StdEncoding.EncodeToString(record.RawPayload))
	for _, forbidden := range []string{"rawpayload", "raw_payload", "raw_payload_bytes", rawPayloadBase64, "sk_live_055", "ntfy-token-055", "secret-token-055", "hunter2", "\"api_key\":", "\"token\":", "\"password\":", "\"authorization\":"} {
		if strings.Contains(lowerBody, forbidden) {
			t.Fatalf("dead-letter API response leaked forbidden replay payload content %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{"\"id\":\"", "\"payload_hash\":\"", "\"payload_size_bytes\":", "\"replay_eligible\":true", "\"replay_status\":\"pending\"", "operator-visible metadata remains"} {
		if !strings.Contains(body, required) {
			t.Fatalf("dead-letter API response lost safe metadata/status %q: %s", required, body)
		}
	}
}

func validNtfySourceStatusForMetadataTest() notification.SourceStatus {
	now := time.Date(2026, 5, 24, 21, 0, 0, 0, time.UTC)
	return notification.SourceStatus{Config: notification.SourceInstanceRecord{SourceType: ntfysource.SourceType, SourceInstanceID: "ntfy-test-source", SourceForm: notification.SourceFormStream, Enabled: true, ConfigHash: "sha256:ntfy-test-source", SecretRefNames: []string{"NTFY_HOME_LAB_TOKEN"}, RedactedMetadata: map[string]string{"display_name": "ntfy test source", "endpoint_label": "operator-managed ntfy endpoint", "endpoint_ref_name": "NTFY_HOME_LAB_ENDPOINT_URL", "transport_mode": ntfysource.TransportModeStream, "auth_mode": ntfysource.AuthModeBearerToken, "topic_count": "1", "topics": "home-lab-alerts", "retry_budget": "3", "reconnect_initial_delay_seconds": "1", "reconnect_max_delay_seconds": "5", "keepalive_timeout_seconds": "30", "lag_degraded_after_seconds": "60", "lag_disconnected_after_seconds": "300", "dead_letter_retry_budget": "2", "max_payload_bytes": "4096", "pressure_threshold_count": "2"}, CreatedAt: now, UpdatedAt: now}, Health: notification.SourceHealthReport{SourceType: ntfysource.SourceType, SourceInstanceID: "ntfy-test-source", SourceForm: notification.SourceFormStream, State: notification.SourceHealthDegraded, LastErrorKind: ntfysource.ErrorConnectivityFailed, ObservedAt: now}}
}

func validNtfyWebhookSourceStatusForMetadataTest() notification.SourceStatus {
	status := validNtfySourceStatusForMetadataTest()
	status.Config.SourceInstanceID = "ntfy-webhook-test-source"
	status.Config.SourceForm = notification.SourceFormWebhook
	status.Config.SecretRefNames = nil
	status.Config.RedactedMetadata["transport_mode"] = ntfysource.TransportModeWebhook
	status.Config.RedactedMetadata["auth_mode"] = ntfysource.AuthModeNone
	status.Health.SourceInstanceID = status.Config.SourceInstanceID
	status.Health.SourceForm = notification.SourceFormWebhook
	return status
}

type staticNotificationSourceStatusProvider struct {
	statuses []notification.SourceStatus
}

func (provider staticNotificationSourceStatusProvider) ListSourceStatuses(ctx context.Context) ([]notification.SourceStatus, error) {
	return append([]notification.SourceStatus(nil), provider.statuses...), nil
}

type apiRecordingSourceSink struct {
	envelopes []notification.SourceEventEnvelope
}

func (sink *apiRecordingSourceSink) SubmitSourceEvent(ctx context.Context, envelope notification.SourceEventEnvelope) (notification.IngestReceipt, error) {
	sink.envelopes = append(sink.envelopes, envelope)
	return notification.IngestReceipt{SourceType: envelope.SourceType, SourceInstanceID: envelope.SourceInstanceID, SourceForm: envelope.SourceForm, RawEventID: "raw-" + envelope.SourceEventID, Accepted: true, Status: "accepted"}, nil
}

func (sink *apiRecordingSourceSink) ReportSourceHealth(ctx context.Context, report notification.SourceHealthReport) error {
	return nil
}

func (sink *apiRecordingSourceSink) envelopeCount() int {
	return len(sink.envelopes)
}

func ntfyWebhookRequest(t *testing.T, sourceInstanceID string, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/notifications/sources/"+sourceInstanceID+"/ntfy/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(chiRouteContext(t, map[string]string{"source_instance_id": sourceInstanceID}))
}

func waitForAPIWebhookRegistration(t *testing.T, registry *ntfysource.WebhookReceiverRegistry, sourceInstanceID string) {
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
