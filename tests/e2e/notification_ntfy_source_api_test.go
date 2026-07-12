//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
)

func TestNtfyProductionWebhookRouteAcceptsConfiguredSourceAndRejectsMalformedPayload(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	validResp, err := apiPostRaw(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(`{"id":"evt-e2e-ntfy-webhook","event":"message","topic":"self-hosted-alerts","title":"E2E ntfy","message":"webhook delivery enters source sink"}`))
	if err != nil {
		t.Fatalf("valid ntfy webhook request failed: %v", err)
	}
	validBody, err := readBody(validResp)
	if err != nil {
		t.Fatalf("read valid webhook response: %v", err)
	}
	if validResp.StatusCode != http.StatusAccepted || !strings.Contains(string(validBody), `"accepted":true`) {
		t.Fatalf("valid webhook status/body = %d %s", validResp.StatusCode, string(validBody))
	}

	malformedResp, err := apiPostRaw(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(`{"event":"message"`))
	if err != nil {
		t.Fatalf("malformed ntfy webhook request failed: %v", err)
	}
	malformedBody, err := readBody(malformedResp)
	if err != nil {
		t.Fatalf("read malformed webhook response: %v", err)
	}
	if malformedResp.StatusCode != http.StatusBadRequest || !strings.Contains(string(malformedBody), "invalid_ntfy_webhook_payload") {
		t.Fatalf("malformed webhook status/body = %d %s", malformedResp.StatusCode, string(malformedBody))
	}

	detailBody := ntfyAPIGetBody(t, cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy")
	if !strings.Contains(detailBody, "source_output_boundary") || !strings.Contains(detailBody, "SourceEventSink") || !strings.Contains(detailBody, "evt-e2e-ntfy-webhook") {
		t.Fatalf("ntfy detail did not expose source-sink boundary and accepted event proof: %s", detailBody)
	}

	reconnectResp, err := apiPostJSON(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/reconnect", map[string]any{})
	if err != nil {
		t.Fatalf("ntfy reconnect request failed: %v", err)
	}
	reconnectBody, err := readBody(reconnectResp)
	if err != nil {
		t.Fatalf("read reconnect body: %v", err)
	}
	if reconnectResp.StatusCode != http.StatusAccepted || !strings.Contains(string(reconnectBody), `"created_notification":false`) {
		t.Fatalf("reconnect status/body = %d %s", reconnectResp.StatusCode, string(reconnectBody))
	}

	dlqBody := ntfyAPIGetBody(t, cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/dead-letters?limit=1")
	if !strings.Contains(dlqBody, "malformed_json") || strings.Contains(dlqBody, "secret-token") {
		t.Fatalf("dead-letter list missing malformed redacted record or leaked secret: %s", dlqBody)
	}
	var dlqList struct {
		DeadLetters []struct {
			ID             string `json:"id"`
			ReplayEligible bool   `json:"replay_eligible"`
		} `json:"dead_letters"`
	}
	if err := json.Unmarshal([]byte(dlqBody), &dlqList); err != nil {
		t.Fatalf("parse dead-letter list: %v; body=%s", err, dlqBody)
	}
	if len(dlqList.DeadLetters) == 0 || dlqList.DeadLetters[0].ID == "" {
		t.Fatalf("dead-letter list did not include a stable id: %+v", dlqList)
	}
	detailDLQBody := ntfyAPIGetBody(t, cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/dead-letters/"+dlqList.DeadLetters[0].ID)
	if !strings.Contains(detailDLQBody, dlqList.DeadLetters[0].ID) || strings.Contains(detailDLQBody, "secret-token") {
		t.Fatalf("dead-letter detail mismatch or leaked secret: %s", detailDLQBody)
	}
	replayResp, err := apiPostJSON(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/dead-letters/"+dlqList.DeadLetters[0].ID+"/replay", map[string]any{"confirmation": "replay_through_source_sink"})
	if err != nil {
		t.Fatalf("ntfy replay request failed: %v", err)
	}
	replayBody, err := readBody(replayResp)
	if err != nil {
		t.Fatalf("read replay body: %v", err)
	}
	if replayResp.StatusCode != http.StatusAccepted || !strings.Contains(string(replayBody), "SourceEventSink") || strings.Contains(string(replayBody), "telegram") {
		t.Fatalf("replay status/body = %d %s", replayResp.StatusCode, string(replayBody))
	}
	invalidLimitResp, err := apiGet(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/dead-letters?limit=0")
	if err != nil {
		t.Fatalf("invalid limit request failed: %v", err)
	}
	invalidLimitBody, err := readBody(invalidLimitResp)
	if err != nil {
		t.Fatalf("read invalid limit body: %v", err)
	}
	if invalidLimitResp.StatusCode != http.StatusBadRequest || !strings.Contains(string(invalidLimitBody), "invalid_ntfy_dead_letter_limit") {
		t.Fatalf("invalid limit status/body = %d %s", invalidLimitResp.StatusCode, string(invalidLimitBody))
	}
}

func TestNtfyDeadLetterAPIRedactsReplayEligibleRawPayload(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	store, pool, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	prefix := notificationE2EPrefix()
	sourceID := prefix + "-ntfy-sensitive-dlq"
	ntfyCfg := ntfyE2EConfig(sourceID)
	seedNtfyE2ESource(t, store, ntfyCfg)
	t.Cleanup(func() { cleanupNtfyE2EArtifacts(t, pool, prefix) })
	ntfyStore := ntfysource.NewStore(pool)
	rawPayload := []byte(`{"id":"evt-sensitive-dlq","event":"message","topic":"self-hosted-alerts","message":"safe-visible status remains","api_key":"raw-api-key-456","token":"secret-token-123","password":"hunter2","Authorization":"Bearer raw-bearer-789"}`)
	event, err := ntfysource.ParseEvent(rawPayload, ntfyCfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse sensitive dead-letter event: %v", err)
	}
	now := time.Date(2026, 5, 24, 23, 58, 0, 0, time.UTC)
	record, err := ntfyStore.CreateDeadLetter(context.Background(), ntfysource.NewDeadLetterRecord(ntfyCfg, event, event.Raw, ntfysource.DeadLetterSinkUnavailable, "sink failed with token=secret-token-123 password=hunter2", true, now))
	if err != nil {
		t.Fatalf("create replay-eligible sensitive dead letter: %v", err)
	}
	if !record.ReplayEligible || len(record.RawPayload) == 0 || !strings.Contains(string(record.RawPayload), "raw-api-key-456") {
		t.Fatalf("test dead letter must retain internal replay bytes: %+v", record)
	}

	listBody := ntfyAPIGetBody(t, cfg, "/api/notifications/sources/"+sourceID+"/ntfy/dead-letters?limit=5")
	assertNtfyDeadLetterAPIRedactsReplayPayload(t, listBody, record)
	var parsed struct {
		DeadLetters []struct {
			ID             string `json:"id"`
			ReplayEligible bool   `json:"replay_eligible"`
			ReplayStatus   string `json:"replay_status"`
			PayloadHash    string `json:"payload_hash"`
		} `json:"dead_letters"`
	}
	if err := json.Unmarshal([]byte(listBody), &parsed); err != nil {
		t.Fatalf("parse sensitive dead-letter list: %v; body=%s", err, listBody)
	}
	if len(parsed.DeadLetters) != 1 || parsed.DeadLetters[0].ID != record.ID || !parsed.DeadLetters[0].ReplayEligible || parsed.DeadLetters[0].ReplayStatus != ntfysource.ReplayStatusPending || parsed.DeadLetters[0].PayloadHash != record.PayloadHash {
		t.Fatalf("sensitive dead-letter list lost safe replay metadata: %+v", parsed)
	}

	detailBody := ntfyAPIGetBody(t, cfg, "/api/notifications/sources/"+sourceID+"/ntfy/dead-letters/"+record.ID)
	assertNtfyDeadLetterAPIRedactsReplayPayload(t, detailBody, record)
}

func TestNtfyDeadLetterReplayAPIIsIdempotent(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	store, pool, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	prefix := notificationE2EPrefix()
	sourceID := prefix + "-ntfy-replay-api"
	ntfyCfg := ntfyE2EConfig(sourceID)
	seedNtfyE2ESource(t, store, ntfyCfg)
	t.Cleanup(func() { cleanupNtfyE2EArtifacts(t, pool, prefix) })
	ntfyStore := ntfysource.NewStore(pool)
	rawPayload := []byte(`{"id":"evt-e2e-replay-burst","event":"message","topic":"self-hosted-alerts","title":"Replay API","message":"operator double submit"}`)
	event, err := ntfysource.ParseEvent(rawPayload, ntfyCfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse replay API event: %v", err)
	}
	now := time.Date(2026, 5, 24, 23, 59, 0, 0, time.UTC)
	record, err := ntfyStore.CreateDeadLetter(context.Background(), ntfysource.NewDeadLetterRecord(ntfyCfg, event, event.Raw, ntfysource.DeadLetterSinkUnavailable, "source sink unavailable", true, now))
	if err != nil {
		t.Fatalf("create replay API dead letter: %v", err)
	}
	replays := make([]ntfyReplayAPIResponse, 0, 3)
	for i := 0; i < 3; i++ {
		resp, err := apiPostJSON(cfg, "/api/notifications/sources/"+sourceID+"/ntfy/dead-letters/"+record.ID+"/replay", map[string]any{"confirmation": "replay_through_source_sink"})
		if err != nil {
			t.Fatalf("ntfy replay API request %d failed: %v", i+1, err)
		}
		body, err := readBody(resp)
		if err != nil {
			t.Fatalf("read replay API response %d: %v", i+1, err)
		}
		if resp.StatusCode != http.StatusAccepted || strings.Contains(string(body), "telegram") || strings.Contains(string(body), string(rawPayload)) {
			t.Fatalf("replay API response %d status/body = %d %s", i+1, resp.StatusCode, string(body))
		}
		var parsed ntfyReplayAPIResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatalf("parse replay API response %d: %v; body=%s", i+1, err, string(body))
		}
		replays = append(replays, parsed)
	}
	if replays[0].Attempt.AlreadyReplayed || !replays[1].Attempt.AlreadyReplayed || !replays[2].Attempt.AlreadyReplayed {
		t.Fatalf("replay API did not expose already-replayed state: %+v", replays)
	}
	if replays[0].Attempt.RawEventID == "" || replays[1].Attempt.RawEventID != replays[0].Attempt.RawEventID || replays[2].Attempt.RawEventID != replays[0].Attempt.RawEventID {
		t.Fatalf("replay API returned mismatched raw event IDs: %+v", replays)
	}
	attemptCount := ntfyE2ECount(t, pool, "SELECT COUNT(*) FROM notification_ntfy_replay_attempts WHERE dead_letter_id = $1", record.ID)
	rawCount := ntfyE2ECount(t, pool, "SELECT COUNT(*) FROM notification_raw_events WHERE source_instance_id = $1 AND source_event_id = $2", sourceID, event.ID)
	normalizedCount := ntfyE2ECount(t, pool, "SELECT COUNT(*) FROM normalized_notifications WHERE source_instance_id = $1 AND source_event_id = $2", sourceID, event.ID)
	if attemptCount != 1 || rawCount != 1 || normalizedCount != 1 {
		t.Fatalf("replay API repeated side effects: attempts=%d raw=%d normalized=%d", attemptCount, rawCount, normalizedCount)
	}
}

type ntfyReplayAPIResponse struct {
	Attempt struct {
		Status          string `json:"Status"`
		RawEventID      string `json:"RawEventID"`
		SinkStatus      string `json:"SinkStatus"`
		AlreadyReplayed bool   `json:"already_replayed"`
	} `json:"attempt"`
}

func assertNtfyDeadLetterAPIRedactsReplayPayload(t *testing.T, body string, record ntfysource.DeadLetterRecord) {
	t.Helper()
	lowerBody := strings.ToLower(body)
	encodedPayload := strings.ToLower(base64.StdEncoding.EncodeToString(record.RawPayload))
	for _, forbidden := range []string{"rawpayload", "raw_payload", "raw_payload_bytes", encodedPayload, "secret-token-123", "hunter2", "raw-api-key-456", "raw-bearer-789", "\"api_key\":", "\"token\":", "\"password\":", "\"authorization\":"} {
		if strings.Contains(lowerBody, forbidden) {
			t.Fatalf("ntfy dead-letter API leaked replay payload content %q: %s", forbidden, body)
		}
	}
	for _, required := range []string{record.ID, record.PayloadHash, `"replay_eligible":true`, `"replay_status":"pending"`, "safe-visible status remains"} {
		if !strings.Contains(body, required) {
			t.Fatalf("ntfy dead-letter API response lost safe metadata/status %q: %s", required, body)
		}
	}
}

func TestNtfySourceStatusAPIRedactsInvalidConfigAndAuthFailures(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	store, pool, cleanup := notificationE2EStore(t)
	t.Cleanup(cleanup)
	prefix := notificationE2EPrefix()
	sourceID := prefix + "-ntfy-auth-failed"
	ntfyCfg := ntfyE2EConfig(sourceID)
	seedNtfyE2ESource(t, store, ntfyCfg)
	t.Cleanup(func() { cleanupNtfyE2EArtifacts(t, pool, prefix) })
	now := time.Date(2026, 5, 24, 20, 0, 0, 0, time.UTC)
	report := notification.SourceHealthReport{SourceType: ntfysource.SourceType, SourceInstanceID: sourceID, SourceForm: ntfyCfg.SourceForm, State: notification.SourceHealthDisconnected, RetryCount: 2, LastErrorKind: ntfysource.ErrorAuthFailed, LastErrorRedacted: "token=secret-token-123 password=hunter2", ObservedAt: now}
	if err := store.RecordSourceHealth(context.Background(), report); err != nil {
		t.Fatalf("record auth failure health: %v", err)
	}
	ntfyStore := ntfysource.NewStore(pool)
	event, err := ntfysource.ParseEvent([]byte(`{"id":"evt-auth-dlq","event":"message","topic":"self-hosted-alerts","message":"token=secret-token-123"}`), ntfyCfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse auth dead-letter event: %v", err)
	}
	if _, err := ntfyStore.CreateDeadLetter(context.Background(), ntfysource.NewDeadLetterRecord(ntfyCfg, event, event.Raw, ntfysource.DeadLetterAuthFailed, "auth failed with password=hunter2 token=secret-token-123", false, now)); err != nil {
		t.Fatalf("create auth dead letter: %v", err)
	}

	status := ntfySourceStatusFromAPI(t, cfg, sourceID)
	if status.HealthState != "disconnected" || status.LastErrorKind != ntfysource.ErrorAuthFailed || status.LastErrorRedacted != "source authentication failed" {
		t.Fatalf("auth-failed ntfy status mismatch: %+v", status)
	}
	if len(status.SecretRefNames) != 1 || status.SecretRefNames[0] != "NTFY_E2E_TOKEN_REF" || status.RedactedMetadata["auth_mode"] != ntfysource.AuthModeBearerToken {
		t.Fatalf("auth-failed ntfy status lost reference-only identity: %+v", status)
	}
	for _, path := range []string{"/api/notifications/sources", "/api/notifications/sources/" + sourceID + "/ntfy", "/api/notifications/sources/" + sourceID + "/ntfy/dead-letters?limit=1"} {
		body := ntfyAPIGetBody(t, cfg, path)
		if strings.Contains(body, "secret-token-123") || strings.Contains(body, "hunter2") {
			t.Fatalf("%s leaked credential material: %s", path, body)
		}
	}
}

func TestNtfyRecoveredHealthRequiresAcceptedEventAndNoFallbackConnectedState(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	reconnectResp, err := apiPostJSON(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/reconnect", map[string]any{})
	if err != nil {
		t.Fatalf("ntfy reconnect request failed: %v", err)
	}
	reconnectBody, err := readBody(reconnectResp)
	if err != nil {
		t.Fatalf("read reconnect body: %v", err)
	}
	if reconnectResp.StatusCode != http.StatusAccepted || !strings.Contains(string(reconnectBody), `"created_notification":false`) {
		t.Fatalf("reconnect status/body = %d %s", reconnectResp.StatusCode, string(reconnectBody))
	}
	reconnecting := ntfySourceStatusFromAPI(t, cfg, "ntfy-local-webhook")
	if reconnecting.HealthState == "connected" || reconnecting.RetryCount == 0 || reconnecting.LastErrorRedacted == "" {
		t.Fatalf("reconnect fabricated connected health instead of degraded observable retry state: %+v", reconnecting)
	}
	eventID := "evt-e2e-ntfy-recovered-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "-")
	validResp, err := apiPostRaw(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", []byte(`{"id":"`+eventID+`","event":"message","topic":"self-hosted-alerts","title":"Recovered ntfy","message":"accepted source event restores health"}`))
	if err != nil {
		t.Fatalf("valid ntfy recovery webhook failed: %v", err)
	}
	validBody, err := readBody(validResp)
	if err != nil {
		t.Fatalf("read recovery webhook response: %v", err)
	}
	if validResp.StatusCode != http.StatusAccepted || !strings.Contains(string(validBody), `"accepted":true`) {
		t.Fatalf("valid recovery webhook status/body = %d %s", validResp.StatusCode, string(validBody))
	}
	recovered := ntfySourceStatusFromAPI(t, cfg, "ntfy-local-webhook")
	if recovered.HealthState != "connected" || recovered.LastEventAt == "" || recovered.LastSuccessfulCheckAt == "" {
		t.Fatalf("accepted ntfy event did not restore connected health with real event/check timestamps: %+v", recovered)
	}
	detail := ntfyAPIDetail(t, cfg, "ntfy-local-webhook")
	if detail.LastAcceptedEvent.SourceEventID != eventID || !detail.LastAcceptedEvent.RawStored || !detail.LastAcceptedEvent.Normalized || detail.LastAcceptedEvent.Topic != "self-hosted-alerts" {
		t.Fatalf("accepted ntfy event proof mismatch after recovery: %+v", detail.LastAcceptedEvent)
	}
}

func TestNtfyMessagePipelineAPIRoundTripShowsRawNormalizedAndSourceFields(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)
	eventID := "evt-e2e-ntfy-pipeline-" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "-")
	payload := []byte(`{"id":"` + eventID + `","event":"message","topic":"self-hosted-alerts","title":"Pipeline token=secret-token-123","message":"storage failure password=hunter2","priority":5,"tags":["disk","urgent"],"safe_context":"rack-7","unsafe_token":"must-not-preserve","smackerel_loop_guard_key":"loop-e2e","smackerel_decision_id":"decision-e2e"}`)
	resp, err := apiPostRaw(cfg, "/api/notifications/sources/ntfy-local-webhook/ntfy/webhook", payload)
	if err != nil {
		t.Fatalf("pipeline ntfy webhook failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read pipeline webhook body: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted || !strings.Contains(string(body), `"accepted":true`) {
		t.Fatalf("pipeline webhook status/body = %d %s", resp.StatusCode, string(body))
	}
	detail := ntfyAPIDetail(t, cfg, "ntfy-local-webhook")
	if detail.LastAcceptedEvent.SourceEventID != eventID || detail.LastAcceptedEvent.NotificationID == "" || detail.LastAcceptedEvent.RawEventID == "" {
		t.Fatalf("ntfy detail did not expose raw/normalized event chain: %+v", detail.LastAcceptedEvent)
	}
	eventDetail := notificationEventDetail(t, cfg, detail.LastAcceptedEvent.NotificationID)
	if eventDetail.Notification.SourceInstanceID != "ntfy-local-webhook" || eventDetail.Notification.SourceEventID != eventID || eventDetail.Notification.DeliveryMetadata["topic"] != "self-hosted-alerts" {
		t.Fatalf("normalized ntfy notification lost source/topic provenance: %+v", eventDetail.Notification)
	}
	if eventDetail.RawEvent.SourceSpecific["ntfy.priority"] != "urgent" || eventDetail.RawEvent.SourceSpecific["ntfy.topic"] != "self-hosted-alerts" || !strings.Contains(eventDetail.RawEvent.SourceSpecific["ntfy.unknown_json"], "safe_context") {
		t.Fatalf("raw ntfy source-specific fields missing expected provenance: %+v", eventDetail.RawEvent.SourceSpecific)
	}
	if strings.Contains(eventDetail.RawEvent.SourceSpecific["ntfy.unknown_json"], "unsafe_token") || strings.Contains(eventDetail.Notification.Body, "hunter2") || strings.Contains(eventDetail.RawEvent.SourceSpecific["body"], "hunter2") {
		t.Fatalf("ntfy raw/normalized pipeline leaked unsafe source content: %+v", eventDetail)
	}
	if eventDetail.Classification == nil || eventDetail.Classification.Severity == "" || eventDetail.Classification.Domain != "ops" || eventDetail.Classification.Rationale == "" {
		t.Fatalf("ntfy classification did not produce final core-owned rationale: %+v", eventDetail.Classification)
	}
}

func apiPostRaw(cfg e2eConfig, path string, payload []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, cfg.CoreURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	return client.Do(req)
}

func ntfyAPIGetBody(t *testing.T, cfg e2eConfig, path string) string {
	t.Helper()
	resp, err := apiGet(cfg, path)
	if err != nil {
		t.Fatalf("GET %s failed: %v", path, err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read %s body: %v", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return string(body)
}

type ntfySourceStatus struct {
	SourceInstanceID      string            `json:"source_instance_id"`
	SourceType            string            `json:"source_type"`
	SourceForm            string            `json:"source_form"`
	SecretRefNames        []string          `json:"secret_ref_names"`
	RedactedMetadata      map[string]string `json:"redacted_metadata"`
	HealthState           string            `json:"health_state"`
	RetryCount            int               `json:"retry_count"`
	LastErrorKind         string            `json:"last_error_kind"`
	LastErrorRedacted     string            `json:"last_error_redacted"`
	LastEventAt           string            `json:"last_event_at"`
	LastSuccessfulCheckAt string            `json:"last_successful_check_at"`
}

func ntfySourceStatusFromAPI(t *testing.T, cfg e2eConfig, sourceID string) ntfySourceStatus {
	t.Helper()
	body := ntfyAPIGetBody(t, cfg, "/api/notifications/sources")
	var parsed struct {
		Sources []ntfySourceStatus `json:"sources"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("parse notification sources: %v; body=%s", err, body)
	}
	for _, source := range parsed.Sources {
		if source.SourceInstanceID == sourceID {
			return source
		}
	}
	t.Fatalf("ntfy source %s not found in source status: %s", sourceID, body)
	return ntfySourceStatus{}
}

type ntfyDetailResponse struct {
	LastAcceptedEvent struct {
		NotificationID string `json:"notification_id"`
		RawEventID     string `json:"raw_event_id"`
		SourceEventID  string `json:"source_event_id"`
		Topic          string `json:"topic"`
		RawStored      bool   `json:"raw_stored"`
		Normalized     bool   `json:"normalized"`
	} `json:"last_accepted_event"`
}

func ntfyAPIDetail(t *testing.T, cfg e2eConfig, sourceID string) ntfyDetailResponse {
	t.Helper()
	body := ntfyAPIGetBody(t, cfg, "/api/notifications/sources/"+sourceID+"/ntfy")
	var detail ntfyDetailResponse
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatalf("parse ntfy detail: %v; body=%s", err, body)
	}
	return detail
}

func ntfyE2EConfig(sourceID string) ntfysource.Config {
	return ntfysource.Config{Enabled: true, SourceInstanceID: sourceID, SourceForm: notification.SourceFormWebhook, TransportMode: ntfysource.TransportModeWebhook, EndpointURL: "http://smackerel-core:8080/api/notifications/sources/" + sourceID + "/ntfy/webhook", EndpointRefName: "NTFY_E2E_ENDPOINT_URL", Topics: []string{"self-hosted-alerts"}, Auth: ntfysource.AuthConfig{Mode: ntfysource.AuthModeBearerToken, SecretRefNames: []string{"NTFY_E2E_TOKEN_REF"}}, Reconnect: ntfysource.ReconnectConfig{RetryBudget: 3, InitialDelaySeconds: 1, MaxDelaySeconds: 5, KeepaliveTimeoutSeconds: 30}, Lag: ntfysource.LagConfig{DegradedAfterSeconds: 60, DisconnectedAfterSeconds: 300}, DeadLetter: ntfysource.DeadLetterConfig{RetryBudget: 2, MaxPayloadBytes: 4096, PressureThresholdCount: 2}, RedactedMetadata: map[string]string{"display_name": "ntfy e2e redaction source", "endpoint_label": "redacted endpoint"}, ConfigHash: "sha256:" + sourceID}
}

func seedNtfyE2ESource(t *testing.T, store *notification.Store, cfg ntfysource.Config) {
	t.Helper()
	instance, err := cfg.SourceInstanceConfig()
	if err != nil {
		t.Fatalf("build ntfy source config: %v", err)
	}
	if _, err := store.CreateSourceInstance(context.Background(), instance, time.Now().UTC()); err != nil {
		t.Fatalf("create ntfy source instance: %v", err)
	}
}

func ntfyE2ECount(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&count); err != nil {
		t.Fatalf("count ntfy E2E rows: %v", err)
	}
	return count
}

func cleanupNtfyE2EArtifacts(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	ctx := context.Background()
	for _, statement := range []string{
		"DELETE FROM notification_ntfy_replay_attempts WHERE source_instance_id LIKE $1",
		"DELETE FROM notification_ntfy_dead_letters WHERE source_instance_id LIKE $1",
		"DELETE FROM notification_ntfy_subscription_states WHERE source_instance_id LIKE $1",
		"DELETE FROM notification_delivery_attempts WHERE decision_id IN (SELECT d.id FROM notification_processing_decisions d JOIN normalized_notifications n ON n.id = d.notification_id WHERE n.source_instance_id LIKE $1) OR incident_id IN (SELECT ie.incident_id FROM notification_incident_events ie JOIN normalized_notifications n ON n.id = ie.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_action_results WHERE action_attempt_id IN (SELECT aa.id FROM notification_action_attempts aa JOIN notification_processing_decisions d ON d.id = aa.decision_id JOIN normalized_notifications n ON n.id = d.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_action_attempts WHERE decision_id IN (SELECT d.id FROM notification_processing_decisions d JOIN normalized_notifications n ON n.id = d.notification_id WHERE n.source_instance_id LIKE $1) OR incident_id IN (SELECT ie.incident_id FROM notification_incident_events ie JOIN normalized_notifications n ON n.id = ie.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_approval_decisions WHERE approval_request_id IN (SELECT ar.id FROM notification_approval_requests ar JOIN notification_processing_decisions d ON d.id = ar.decision_id JOIN normalized_notifications n ON n.id = d.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_approval_requests WHERE decision_id IN (SELECT d.id FROM notification_processing_decisions d JOIN normalized_notifications n ON n.id = d.notification_id WHERE n.source_instance_id LIKE $1) OR incident_id IN (SELECT ie.incident_id FROM notification_incident_events ie JOIN normalized_notifications n ON n.id = ie.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_diagnostics WHERE notification_id IN (SELECT id FROM normalized_notifications WHERE source_instance_id LIKE $1) OR incident_id IN (SELECT ie.incident_id FROM notification_incident_events ie JOIN normalized_notifications n ON n.id = ie.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_suppressions WHERE source_instance_id LIKE $1 OR notification_id IN (SELECT id FROM normalized_notifications WHERE source_instance_id LIKE $1) OR incident_id IN (SELECT ie.incident_id FROM notification_incident_events ie JOIN normalized_notifications n ON n.id = ie.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_processing_decisions WHERE notification_id IN (SELECT id FROM normalized_notifications WHERE source_instance_id LIKE $1) OR incident_id IN (SELECT ie.incident_id FROM notification_incident_events ie JOIN normalized_notifications n ON n.id = ie.notification_id WHERE n.source_instance_id LIKE $1)",
		"DELETE FROM notification_incident_events WHERE notification_id IN (SELECT id FROM normalized_notifications WHERE source_instance_id LIKE $1)",
		"DELETE FROM notification_classifications WHERE notification_id IN (SELECT id FROM normalized_notifications WHERE source_instance_id LIKE $1)",
		"DELETE FROM normalized_notifications WHERE source_instance_id LIKE $1",
		"DELETE FROM notification_raw_events WHERE source_instance_id LIKE $1",
		"DELETE FROM notification_incidents WHERE EXISTS (SELECT 1 FROM unnest(source_instance_ids) source_instance_id WHERE source_instance_id LIKE $1)",
		"DELETE FROM notification_source_instances WHERE source_instance_id LIKE $1",
	} {
		if _, err := pool.Exec(ctx, statement, prefix+"%"); err != nil {
			t.Logf("cleanup ntfy e2e artifacts failed for %q: %v", statement, err)
		}
	}
}
