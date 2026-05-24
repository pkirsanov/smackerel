package ntfy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func MapEvent(ctx context.Context, cfg Config, event Event, observedAt time.Time) (notification.SourceEventEnvelope, error) {
	if observedAt.IsZero() {
		return notification.SourceEventEnvelope{}, fmt.Errorf("ntfy mapper: observed_at is required")
	}
	if _, err := cfg.SourceInstanceConfig(); err != nil {
		return notification.SourceEventEnvelope{}, err
	}
	if !topicConfigured(cfg, event.Topic) {
		return notification.SourceEventEnvelope{}, fmt.Errorf("ntfy mapper: topic %q is not configured", event.Topic)
	}
	if !event.ShouldIngest() {
		return notification.SourceEventEnvelope{}, fmt.Errorf("ntfy mapper: event type %q is not message-like", event.EventType)
	}
	title, _ := notification.RedactText(event.Title)
	body, _ := notification.RedactText(event.Message)
	delivery := map[string]string{"topic": event.Topic, "transport_mode": cfg.TransportMode, "source_form": string(cfg.SourceForm), "endpoint_ref_name": cfg.EndpointRefName, "event_type": event.EventType}
	fields := map[string]string{"ntfy.event": event.EventType, "ntfy.topic": event.Topic, "ntfy.title": title, "title": title, "ntfy.message": body, "body": body}
	if event.ID != "" {
		fields["ntfy.id"] = event.ID
	}
	if event.RawTime != "" {
		fields["ntfy.time"] = event.RawTime
	}
	if event.Priority != "" {
		fields["ntfy.priority"] = event.Priority
		fields["severity"] = severityHint(event.Priority)
	}
	if len(event.Tags) > 0 {
		fields["ntfy.tags_json"] = mustJSON(event.Tags)
	}
	if event.Click != "" {
		fields["ntfy.click"] = redactURLLike(event.Click)
	}
	if event.Icon != "" {
		fields["ntfy.icon"] = redactURLLike(event.Icon)
	}
	if event.Attach != "" {
		fields["ntfy.attach"] = redactURLLike(event.Attach)
	}
	if event.Markdown != nil {
		fields["ntfy.markdown"] = fmt.Sprintf("%t", *event.Markdown)
	}
	if len(event.Attachment) > 0 {
		fields["ntfy.attachment_json"] = redactJSONString(string(event.Attachment))
	}
	if len(event.Actions) > 0 {
		fields["ntfy.actions_json"] = redactJSONString(string(event.Actions))
	}
	if unknown := sortedUnknownJSON(event.Unknown); unknown != "" {
		fields["ntfy.unknown_json"] = redactJSONString(unknown)
	}
	hints := map[string]string{"title": title, "body": body}
	if hint := severityHint(event.Priority); hint != "" {
		hints["severity"] = hint
	}
	if subject := cfg.Mapping.TopicSubjects[event.Topic]; subject != "" {
		hints["subject"] = subject
	}
	if cfg.Mapping.DefaultDomain != "" {
		hints["domain"] = cfg.Mapping.DefaultDomain
	}
	for _, tag := range event.Tags {
		if service := cfg.Mapping.TagServices[tag]; service != "" {
			hints["service"] = service
		}
		if intent := cfg.Mapping.TagIntents[tag]; intent != "" {
			hints["intent"] = intent
		}
	}
	loop := loopMetadata(event)
	if key := loop["loop_guard_key"]; key != "" {
		delivery["loop_guard_key"] = key
	}
	return notification.SourceEventEnvelope{SourceType: SourceType, SourceInstanceID: cfg.SourceInstanceID, SourceForm: cfg.SourceForm, SourceEventID: event.ID, ObservedAt: observedAt, EventTimestamp: event.Time, RawPayloadKind: notification.RawPayloadKindJSON, RawPayload: append([]byte(nil), event.Raw...), DeliveryMetadata: delivery, SourceSpecificFields: fields, MappingHints: hints, LoopMetadata: loop}, nil
}

func topicConfigured(cfg Config, topic string) bool {
	for _, configured := range cfg.Topics {
		if configured == topic {
			return true
		}
	}
	return false
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func redactURLLike(value string) string {
	redacted, _ := notification.RedactText(value)
	return redacted
}

func redactJSONString(value string) string {
	redacted, _ := notification.RedactText(value)
	return redacted
}

func loopMetadata(event Event) map[string]string {
	metadata := map[string]string{}
	knownKeys := map[string]string{"smackerel_loop_guard_key": "loop_guard_key", "smackerel_origin_id": "origin_id", "smackerel_incident_id": "incident_id", "smackerel_decision_id": "decision_id", "smackerel_output_trace_ref": "output_trace_ref"}
	for sourceKey, targetKey := range knownKeys {
		if value := unknownString(event.Unknown, sourceKey); value != "" {
			metadata[targetKey] = value
		}
	}
	return metadata
}

func unknownString(values map[string]json.RawMessage, key string) string {
	var value string
	if err := json.Unmarshal(values[key], &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}
