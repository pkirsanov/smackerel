package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/notification"
	ntfysource "github.com/smackerel/smackerel/internal/notification/source/ntfy"
)

const ntfyReplayConfirmationValue = "replay_through_source_sink"

func (h *NotificationHandlers) ReceiveNtfyWebhook(w http.ResponseWriter, r *http.Request) {
	status, ok := h.ntfySourceStatus(w, r)
	if !ok {
		return
	}
	cfg, err := ntfyConfigFromStatus(status)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_source_metadata", err.Error())
		return
	}
	if cfg.TransportMode != ntfysource.TransportModeWebhook || cfg.SourceForm != notification.SourceFormWebhook {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_webhook_source", "ntfy webhook delivery requires a webhook source instance")
		return
	}
	if h.ntfyWebhooks == nil {
		writeError(w, http.StatusServiceUnavailable, "ntfy_webhook_receiver_unavailable", "ntfy webhook receiver is not running")
		return
	}
	payload, ok := readNtfyWebhookPayload(w, r, cfg.DeadLetter.MaxPayloadBytes)
	if !ok {
		return
	}
	if err := h.ntfyWebhooks.ReceiveRaw(r.Context(), cfg.SourceInstanceID, payload); err != nil {
		switch {
		case ntfysource.IsWebhookSourceNotRunning(err):
			writeError(w, http.StatusServiceUnavailable, "ntfy_webhook_receiver_unavailable", "ntfy webhook receiver is not running")
		case ntfysource.IsWebhookPayloadInvalid(err):
			writeError(w, http.StatusBadRequest, "invalid_ntfy_webhook_payload", "ntfy webhook payload must be valid configured ntfy JSON")
		default:
			writeError(w, http.StatusBadRequest, "ntfy_webhook_rejected", "ntfy webhook payload was rejected by the source adapter")
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"source_instance_id": cfg.SourceInstanceID, "accepted": true, "transport_mode": cfg.TransportMode})
}

func (h *NotificationHandlers) GetNtfySourceDetail(w http.ResponseWriter, r *http.Request) {
	status, ok := h.ntfySourceStatus(w, r)
	if !ok {
		return
	}
	cfg, err := ntfyConfigFromStatus(status)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_source_metadata", err.Error())
		return
	}
	topics := []ntfysource.SubscriptionState{}
	if h.ntfyStore != nil {
		topics, err = h.ntfyStore.ListSubscriptionStates(r.Context(), cfg.SourceInstanceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ntfy_source_detail_unavailable", "failed to load ntfy topic states")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": status, "topics": topics, "last_accepted_event": h.ntfyLastAcceptedEvent(r, cfg.SourceInstanceID), "source_output_boundary": "ntfy source events enter only through SourceEventSink; output dispatch remains core-owned"})
}

func (h *NotificationHandlers) ReconnectNtfySource(w http.ResponseWriter, r *http.Request) {
	status, ok := h.ntfySourceStatus(w, r)
	if !ok {
		return
	}
	cfg, err := ntfyConfigFromStatus(status)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_source_metadata", err.Error())
		return
	}
	if h.ntfyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ntfy_operational_store_unavailable", "ntfy operational store is unavailable")
		return
	}
	now := time.Now().UTC()
	states := make([]ntfysource.SubscriptionState, 0, len(cfg.Topics))
	for _, topic := range cfg.Topics {
		state := ntfysource.FinalizeSubscriptionState(cfg, ntfysource.SubscriptionState{SourceInstanceID: cfg.SourceInstanceID, Topic: topic, SourceForm: cfg.SourceForm, TransportMode: cfg.TransportMode, SubscriptionState: ntfysource.SubscriptionReconnecting, PossibleGap: true, RetryCount: 1, RetryBudget: cfg.Reconnect.RetryBudget, LastErrorKind: ntfysource.ErrorConnectivityFailed, LastErrorRedacted: "operator requested reconnect", RedactionState: map[string]any{"status": "redacted", "categories": []string{}}, CreatedAt: now, UpdatedAt: now}, now)
		if err := h.ntfyStore.UpsertSubscriptionState(r.Context(), state); err != nil {
			writeError(w, http.StatusInternalServerError, "ntfy_reconnect_failed", "failed to record ntfy reconnect state")
			return
		}
		states = append(states, state)
	}
	if err := h.store.RecordSourceHealth(r.Context(), ntfysource.HealthFromTopics(cfg, states, now)); err != nil {
		writeError(w, http.StatusInternalServerError, "ntfy_reconnect_failed", "failed to record ntfy source health")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"source_instance_id": cfg.SourceInstanceID, "state": "reconnecting", "created_notification": false})
}

func (h *NotificationHandlers) ListNtfyDeadLetters(w http.ResponseWriter, r *http.Request) {
	status, ok := h.ntfySourceStatus(w, r)
	if !ok {
		return
	}
	if h.ntfyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ntfy_operational_store_unavailable", "ntfy operational store is unavailable")
		return
	}
	limit, ok := ntfyPositiveLimit(w, r)
	if !ok {
		return
	}
	page, err := h.ntfyStore.ListDeadLetters(r.Context(), status.Config.SourceInstanceID, limit, strings.TrimSpace(r.URL.Query().Get("cursor")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ntfy_dead_letters_unavailable", "failed to load ntfy dead letters")
		return
	}
	writeJSON(w, http.StatusOK, ntfyDeadLetterPageResponse{DeadLetters: redactedNtfyDeadLetterResponses(page.Records), NextCursor: page.NextCursor})
}

func (h *NotificationHandlers) GetNtfyDeadLetter(w http.ResponseWriter, r *http.Request) {
	status, ok := h.ntfySourceStatus(w, r)
	if !ok {
		return
	}
	if h.ntfyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ntfy_operational_store_unavailable", "ntfy operational store is unavailable")
		return
	}
	record, err := h.ntfyStore.GetDeadLetter(r.Context(), status.Config.SourceInstanceID, chi.URLParam(r, "dead_letter_id"))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ntfy_dead_letter_not_found", "ntfy dead letter not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "ntfy_dead_letter_unavailable", "failed to load ntfy dead letter")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dead_letter": redactedNtfyDeadLetterResponse(record)})
}

type ntfyDeadLetterPageResponse struct {
	DeadLetters []ntfyDeadLetterResponse `json:"dead_letters"`
	NextCursor  string                   `json:"next_cursor"`
}

type ntfyDeadLetterResponse struct {
	ID                 string         `json:"id"`
	SourceInstanceID   string         `json:"source_instance_id"`
	Topic              string         `json:"topic,omitempty"`
	SourceEventID      string         `json:"source_event_id,omitempty"`
	EventType          string         `json:"event_type,omitempty"`
	ObservedAt         time.Time      `json:"observed_at"`
	PayloadHash        string         `json:"payload_hash"`
	PayloadSizeBytes   int            `json:"payload_size_bytes"`
	SourceRawEventID   string         `json:"source_raw_event_id,omitempty"`
	SafePayloadPreview string         `json:"safe_payload_preview,omitempty"`
	CauseKind          string         `json:"cause_kind"`
	CauseRedacted      string         `json:"cause_redacted"`
	ReplayEligible     bool           `json:"replay_eligible"`
	ReplayStatus       string         `json:"replay_status"`
	AttemptCount       int            `json:"attempt_count"`
	LastAttemptAt      *time.Time     `json:"last_attempt_at,omitempty"`
	RedactionState     map[string]any `json:"redaction_state"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

func redactedNtfyDeadLetterResponses(records []ntfysource.DeadLetterRecord) []ntfyDeadLetterResponse {
	responses := make([]ntfyDeadLetterResponse, 0, len(records))
	for _, record := range records {
		responses = append(responses, redactedNtfyDeadLetterResponse(record))
	}
	return responses
}

func redactedNtfyDeadLetterResponse(record ntfysource.DeadLetterRecord) ntfyDeadLetterResponse {
	return ntfyDeadLetterResponse{
		ID:                 record.ID,
		SourceInstanceID:   record.SourceInstanceID,
		Topic:              record.Topic,
		SourceEventID:      record.SourceEventID,
		EventType:          record.EventType,
		ObservedAt:         record.ObservedAt,
		PayloadHash:        record.PayloadHash,
		PayloadSizeBytes:   record.PayloadSizeBytes,
		SourceRawEventID:   record.SourceRawEventID,
		SafePayloadPreview: redactedNtfyDeadLetterPreview(record),
		CauseKind:          record.CauseKind,
		CauseRedacted:      record.CauseRedacted,
		ReplayEligible:     record.ReplayEligible,
		ReplayStatus:       record.ReplayStatus,
		AttemptCount:       record.AttemptCount,
		LastAttemptAt:      record.LastAttemptAt,
		RedactionState:     cloneNtfyRedactionState(record.RedactionState),
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
	}
}

func redactedNtfyDeadLetterPreview(record ntfysource.DeadLetterRecord) string {
	preview := strings.TrimSpace(record.SafePayloadPreview)
	if preview != "" && !containsNtfyCredentialMarker(preview) {
		return preview
	}
	if len(record.RawPayload) == 0 {
		if preview == "" {
			return ""
		}
		return "[redacted:payload_preview]"
	}
	var payload map[string]any
	if err := json.Unmarshal(record.RawPayload, &payload); err != nil {
		return "[redacted:payload_preview]"
	}
	safePayload := make(map[string]any)
	for _, key := range []string{"id", "event", "topic", "title", "message", "priority", "tags"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		safePayload[key] = redactedNtfyPreviewValue(value)
	}
	if len(safePayload) == 0 {
		return "[redacted:payload_preview]"
	}
	encoded, err := json.Marshal(safePayload)
	if err != nil {
		return "[redacted:payload_preview]"
	}
	preview = string(encoded)
	if len(preview) > 240 {
		preview = preview[:240]
	}
	return preview
}

func redactedNtfyPreviewValue(value any) any {
	switch typed := value.(type) {
	case string:
		redacted, _ := notification.RedactText(typed)
		if containsNtfyCredentialMarker(redacted) {
			return "[redacted:payload_field]"
		}
		return redacted
	case []any:
		items := make([]any, 0, len(typed))
		for _, item := range typed {
			items = append(items, redactedNtfyPreviewValue(item))
		}
		return items
	default:
		return value
	}
}

func containsNtfyCredentialMarker(value string) bool {
	lowerValue := strings.ToLower(value)
	for _, marker := range []string{"\"api_key\"", "\"apikey\"", "\"authorization\"", "\"password\"", "\"token\"", "\"secret\"", "authorization:", "bearer ", "token=", "password=", "api_key=", "apikey=", "secret="} {
		if strings.Contains(lowerValue, marker) {
			return true
		}
	}
	return false
}

func cloneNtfyRedactionState(values map[string]any) map[string]any {
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

type ntfyReplayRequest struct {
	Confirmation string `json:"confirmation"`
}

func (h *NotificationHandlers) ReplayNtfyDeadLetter(w http.ResponseWriter, r *http.Request) {
	status, ok := h.ntfySourceStatus(w, r)
	if !ok {
		return
	}
	cfg, err := ntfyConfigFromStatus(status)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_source_metadata", err.Error())
		return
	}
	if h.ntfyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ntfy_operational_store_unavailable", "ntfy operational store is unavailable")
		return
	}
	var req ntfyReplayRequest
	if !decodeJSONBody(w, r, &req, "invalid_ntfy_replay_request", "request body must include replay confirmation") {
		return
	}
	if req.Confirmation != ntfyReplayConfirmationValue {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_replay_confirmation", "confirmation must be replay_through_source_sink")
		return
	}
	attempt, err := h.ntfyStore.ReplayDeadLetter(r.Context(), cfg, chi.URLParam(r, "dead_letter_id"), h.service, "authenticated_operator", time.Now().UTC())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ntfy_dead_letter_not_found", "ntfy dead letter not found")
			return
		}
		writeError(w, http.StatusBadRequest, "ntfy_replay_failed", "ntfy dead letter replay failed")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"attempt": attempt, "source_output_boundary": "replay submitted only through SourceEventSink"})
}

func readNtfyWebhookPayload(w http.ResponseWriter, r *http.Request, maxPayloadBytes int) ([]byte, bool) {
	if maxPayloadBytes <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_source_metadata", "ntfy webhook payload limit must be positive")
		return nil, false
	}
	reader := http.MaxBytesReader(w, r.Body, int64(maxPayloadBytes))
	payload, err := io.ReadAll(reader)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "invalid_ntfy_webhook_payload", "ntfy webhook payload exceeds configured size limit")
		return nil, false
	}
	if strings.TrimSpace(string(payload)) == "" {
		writeError(w, http.StatusBadRequest, "invalid_ntfy_webhook_payload", "ntfy webhook payload is required")
		return nil, false
	}
	return payload, true
}

func (h *NotificationHandlers) ntfySourceStatus(w http.ResponseWriter, r *http.Request) (notification.SourceStatus, bool) {
	instanceID := chi.URLParam(r, "source_instance_id")
	statuses, err := h.sources.ListSourceStatuses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "notification_sources_unavailable", "failed to load notification sources")
		return notification.SourceStatus{}, false
	}
	for _, status := range statuses {
		if status.Config.SourceInstanceID == instanceID {
			if status.Config.SourceType != ntfysource.SourceType {
				writeError(w, http.StatusNotFound, "ntfy_source_not_found", "notification source is not an ntfy source")
				return notification.SourceStatus{}, false
			}
			return status, true
		}
	}
	writeError(w, http.StatusNotFound, "source_not_found", "notification source not found")
	return notification.SourceStatus{}, false
}

func ntfyConfigFromStatus(status notification.SourceStatus) (ntfysource.Config, error) {
	metadata := status.Config.RedactedMetadata
	topics, err := requiredNtfyStatusTopics(metadata)
	if err != nil {
		return ntfysource.Config{}, err
	}
	transportMode, err := requiredNtfyStatusTransportMode(metadata, status.Config.SourceForm)
	if err != nil {
		return ntfysource.Config{}, err
	}
	endpointRefName, err := requiredNtfyStatusMetadata(metadata, "endpoint_ref_name", "endpoint identity")
	if err != nil {
		return ntfysource.Config{}, err
	}
	authMode, err := requiredNtfyStatusAuthMode(metadata)
	if err != nil {
		return ntfysource.Config{}, err
	}
	policy, err := requiredNtfyStatusPolicyValues(metadata)
	if err != nil {
		return ntfysource.Config{}, err
	}
	return ntfysource.Config{Enabled: status.Config.Enabled, SourceInstanceID: status.Config.SourceInstanceID, SourceForm: status.Config.SourceForm, TransportMode: transportMode, EndpointURL: endpointRefName, EndpointRefName: endpointRefName, Topics: topics, Auth: ntfysource.AuthConfig{Mode: authMode, SecretRefNames: append([]string(nil), status.Config.SecretRefNames...)}, Reconnect: ntfysource.ReconnectConfig{RetryBudget: policy.reconnectRetryBudget, InitialDelaySeconds: policy.reconnectInitialDelaySeconds, MaxDelaySeconds: policy.reconnectMaxDelaySeconds, KeepaliveTimeoutSeconds: policy.keepaliveTimeoutSeconds}, Lag: ntfysource.LagConfig{DegradedAfterSeconds: policy.lagDegradedAfterSeconds, DisconnectedAfterSeconds: policy.lagDisconnectedAfterSeconds}, DeadLetter: ntfysource.DeadLetterConfig{RetryBudget: policy.deadLetterRetryBudget, MaxPayloadBytes: policy.maxPayloadBytes, PressureThresholdCount: policy.pressureThresholdCount}, RedactedMetadata: cloneNotificationMetadata(metadata), ConfigHash: status.Config.ConfigHash}, nil
}

type ntfyStatusPolicyValues struct {
	reconnectRetryBudget         int
	reconnectInitialDelaySeconds int
	reconnectMaxDelaySeconds     int
	keepaliveTimeoutSeconds      int
	lagDegradedAfterSeconds      int
	lagDisconnectedAfterSeconds  int
	deadLetterRetryBudget        int
	maxPayloadBytes              int
	pressureThresholdCount       int
}

func requiredNtfyStatusTopics(metadata map[string]string) ([]string, error) {
	raw, err := requiredNtfyStatusMetadata(metadata, "topics", "topics")
	if err != nil {
		return nil, err
	}
	parts := strings.Split(raw, ",")
	topics := make([]string, 0, len(parts))
	for _, part := range parts {
		if topic := strings.TrimSpace(part); topic != "" {
			topics = append(topics, topic)
		}
	}
	if len(topics) == 0 {
		return nil, redactedNtfyStatusMetadataError("topics are required")
	}
	return topics, nil
}

func requiredNtfyStatusTransportMode(metadata map[string]string, form notification.SourceForm) (string, error) {
	mode, err := requiredNtfyStatusMetadata(metadata, "transport_mode", "transport mode")
	if err != nil {
		return "", err
	}
	if mode != ntfysource.TransportModeStream && mode != ntfysource.TransportModeWebhook {
		return "", redactedNtfyStatusMetadataError("transport mode must be stream or webhook")
	}
	if string(form) != mode {
		return "", redactedNtfyStatusMetadataError("source form must match transport mode")
	}
	return mode, nil
}

func requiredNtfyStatusAuthMode(metadata map[string]string) (string, error) {
	mode, err := requiredNtfyStatusMetadata(metadata, "auth_mode", "auth mode")
	if err != nil {
		return "", err
	}
	if mode != ntfysource.AuthModeBearerToken && mode != ntfysource.AuthModeBasic && mode != ntfysource.AuthModeNone {
		return "", redactedNtfyStatusMetadataError("auth_mode must be bearer_token, basic, or none")
	}
	return mode, nil
}

func requiredNtfyStatusPolicyValues(metadata map[string]string) (ntfyStatusPolicyValues, error) {
	values := ntfyStatusPolicyValues{}
	keys := []struct {
		name   string
		assign func(int)
	}{
		{name: "retry_budget", assign: func(v int) { values.reconnectRetryBudget = v }},
		{name: "reconnect_initial_delay_seconds", assign: func(v int) { values.reconnectInitialDelaySeconds = v }},
		{name: "reconnect_max_delay_seconds", assign: func(v int) { values.reconnectMaxDelaySeconds = v }},
		{name: "keepalive_timeout_seconds", assign: func(v int) { values.keepaliveTimeoutSeconds = v }},
		{name: "lag_degraded_after_seconds", assign: func(v int) { values.lagDegradedAfterSeconds = v }},
		{name: "lag_disconnected_after_seconds", assign: func(v int) { values.lagDisconnectedAfterSeconds = v }},
		{name: "dead_letter_retry_budget", assign: func(v int) { values.deadLetterRetryBudget = v }},
		{name: "max_payload_bytes", assign: func(v int) { values.maxPayloadBytes = v }},
		{name: "pressure_threshold_count", assign: func(v int) { values.pressureThresholdCount = v }},
	}
	for _, key := range keys {
		raw, err := requiredNtfyStatusMetadata(metadata, key.name, key.name)
		if err != nil {
			return ntfyStatusPolicyValues{}, err
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return ntfyStatusPolicyValues{}, redactedNtfyStatusMetadataError(fmt.Sprintf("%s must be a positive integer", key.name))
		}
		key.assign(value)
	}
	return values, nil
}

func requiredNtfyStatusMetadata(metadata map[string]string, key string, label string) (string, error) {
	value := strings.TrimSpace(metadata[key])
	if value == "" {
		return "", redactedNtfyStatusMetadataError(label + " is required")
	}
	return value, nil
}

func redactedNtfyStatusMetadataError(message string) error {
	return fmt.Errorf("ntfy source metadata is invalid: %s", message)
}

func ntfyPositiveLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 200 {
			writeError(w, http.StatusBadRequest, "invalid_ntfy_dead_letter_limit", "limit must be a positive integer up to 200")
			return 0, false
		}
		limit = value
	}
	return limit, true
}

func (h *NotificationHandlers) ntfyLastAcceptedEvent(r *http.Request, sourceInstanceID string) map[string]any {
	if h.store == nil {
		return nil
	}
	events, err := h.store.ListNotifications(r.Context(), 100)
	if err != nil {
		return nil
	}
	for _, event := range events {
		if event.SourceInstanceID == sourceInstanceID {
			return map[string]any{"notification_id": event.ID, "raw_event_id": event.RawEventID, "source_event_id": event.SourceEventID, "topic": event.DeliveryMetadata["topic"], "raw_stored": true, "normalized": true, "title_preview": event.Title}
		}
	}
	return nil
}
