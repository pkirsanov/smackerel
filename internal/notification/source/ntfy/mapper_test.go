package ntfy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyMapperPreservesRawFieldsAndSeparatesLifecycleEvents(t *testing.T) {
	cfg := testConfig()
	raw := []byte(`{"id":"evt-map-1","time":1770000000,"event":"message","topic":"self-hosted-alerts","title":"Disk token=secret-token","message":"storage password=hunter2","priority":5,"tags":["disk","urgent"],"click":"https://example.invalid/path?token=secret-token","icon":"https://example.invalid/icon.png?api_key=secret-key","markdown":true,"attachment":{"name":"disk.txt","url":"https://example.invalid/disk.txt?secret=value"},"actions":[{"action":"view","url":"https://example.invalid/action?token=secret-token"}],"smackerel_loop_guard_key":"loop-key-1","smackerel_origin_id":"origin-1","unsafe_token":"must-not-preserve","safe_context":"rack-7"}`)
	event, err := ParseEvent(raw, cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse ntfy event: %v", err)
	}
	observedAt := time.Date(2026, 5, 24, 22, 0, 0, 0, time.UTC)
	envelope, err := MapEvent(context.Background(), cfg, event, observedAt)
	if err != nil {
		t.Fatalf("map ntfy event: %v", err)
	}
	if envelope.SourceType != SourceType || envelope.SourceInstanceID != cfg.SourceInstanceID || envelope.SourceForm != cfg.SourceForm {
		t.Fatalf("wrong source identity: %+v", envelope)
	}
	if string(envelope.RawPayload) != string(raw) || envelope.RawPayloadKind != notification.RawPayloadKindJSON {
		t.Fatalf("raw payload was not preserved as JSON: kind=%s raw=%s", envelope.RawPayloadKind, string(envelope.RawPayload))
	}
	if envelope.DeliveryMetadata["topic"] != "self-hosted-alerts" || envelope.SourceSpecificFields["ntfy.topic"] != "self-hosted-alerts" {
		t.Fatalf("topic provenance missing: delivery=%v fields=%v", envelope.DeliveryMetadata, envelope.SourceSpecificFields)
	}
	if envelope.MappingHints["domain"] != "ops" || envelope.MappingHints["service"] != "storage" || envelope.MappingHints["intent"] != "investigate" || envelope.MappingHints["severity"] != "critical" {
		t.Fatalf("mapping hints did not preserve normalized policy inputs: %+v", envelope.MappingHints)
	}
	serializedUnknown := envelope.SourceSpecificFields["ntfy.unknown_json"]
	if !strings.Contains(serializedUnknown, "safe_context") || strings.Contains(serializedUnknown, "unsafe_token") {
		t.Fatalf("unknown field safety mismatch: %s", serializedUnknown)
	}
	for key, value := range envelope.SourceSpecificFields {
		if strings.Contains(value, "secret-token") || strings.Contains(value, "hunter2") || strings.Contains(value, "secret-key") {
			t.Fatalf("source-specific field %s leaked sensitive material: %s", key, value)
		}
	}
	if envelope.LoopMetadata["loop_guard_key"] != "loop-key-1" || envelope.LoopMetadata["origin_id"] != "origin-1" {
		t.Fatalf("loop metadata not preserved: %+v", envelope.LoopMetadata)
	}

	lifecycle, err := ParseEvent([]byte(`{"event":"keepalive","topic":"self-hosted-alerts"}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse lifecycle event: %v", err)
	}
	if !lifecycle.IsLifecycle() || lifecycle.ShouldIngest() {
		t.Fatalf("lifecycle classification mismatch: %+v", lifecycle)
	}
	if _, err := MapEvent(context.Background(), cfg, lifecycle, observedAt); err == nil {
		t.Fatal("expected lifecycle event to stay out of normalized notification mapping")
	}
}
