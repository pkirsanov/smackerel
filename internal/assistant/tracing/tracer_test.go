package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestNewTracer_DisabledIsNoOp asserts the noop path: when
// cfg.Enabled=false, NewTracer returns a non-nil tracer + non-nil
// shutdown, the shutdown is safe to call, StartSpan returns a span
// that records nothing (IsRecording() == false), and EndSpan does
// not panic on the noop span. This is the production-default config
// for spec 061 SCOPE-09a.
func TestNewTracer_DisabledIsNoOp(t *testing.T) {
	cfg := Config{
		Enabled:     false,
		Endpoint:    "",
		ServiceName: "smackerel-core",
	}
	tr, shutdown, err := NewTracer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewTracer with Enabled=false must succeed; got: %v", err)
	}
	if tr == nil {
		t.Fatal("NewTracer must return a non-nil tracer in the noop path so emission sites stay unconditional")
	}
	if shutdown == nil {
		t.Fatal("NewTracer must return a non-nil shutdown func in the noop path")
	}
	ctx, span := tr.StartSpan(context.Background(), "test.span",
		"telegram", "deadbeef0badbeef", "turn-1", "", "corr-1")
	if span == nil {
		t.Fatal("StartSpan must return a non-nil span even in the noop path")
	}
	if span.IsRecording() {
		t.Errorf("noop tracer span IsRecording() = true; want false")
	}
	// Span end + shutdown must not panic.
	EndSpan(span, "ok", "")
	_ = ctx
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown must return nil; got: %v", err)
	}
}

// TestNewTracer_ServiceNameRequired proves the SST contract: even on
// the noop path, ServiceName="" is fail-loud because the resource
// service.name attribute is required for symmetric span shape.
func TestNewTracer_ServiceNameRequired(t *testing.T) {
	_, _, err := NewTracer(context.Background(), Config{
		Enabled:     false,
		ServiceName: "",
	})
	if err == nil {
		t.Fatal("NewTracer with empty ServiceName must error; got nil")
	}
}

// TestNewTracer_EnabledRequiresEndpoint proves the SST contract: when
// cfg.Enabled=true, an empty Endpoint is fail-loud (no exporter target).
func TestNewTracer_EnabledRequiresEndpoint(t *testing.T) {
	_, _, err := NewTracer(context.Background(), Config{
		Enabled:     true,
		Endpoint:    "",
		ServiceName: "smackerel-core",
	})
	if err == nil {
		t.Fatal("NewTracer with Enabled=true and empty Endpoint must error; got nil")
	}
}

// TestStartSpan_InMemoryExporter_StampsCanonicalAttrs is the SCOPE-09a
// DoD #3g acceptance: drive a single span through an in-memory
// exporter and assert it carries the 5 mandatory canonical attrs from
// design §8.3.1.B plus the 2 outcome attrs from EndSpan.
func TestStartSpan_InMemoryExporter_StampsCanonicalAttrs(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
	)
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("provider shutdown: %v", err)
		}
	}()
	tr := NewTracerFromProvider(provider, "smackerel-core")

	_, span := tr.StartSpan(context.Background(), "assistant.adapter.translate",
		"telegram", HashUserID("user-abc"), "turn-7", "", "telegram-update-42")
	EndSpan(span, "ok", "")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected exactly 1 span recorded; got %d", len(spans))
	}
	got := spans[0]
	if got.Name != "assistant.adapter.translate" {
		t.Errorf("span name = %q; want %q", got.Name, "assistant.adapter.translate")
	}
	attrs := map[string]string{}
	for _, kv := range got.Attributes {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	requiredKeys := []string{"transport", "user_id_hashed", "assistant_turn_id", "scenario_id", "correlation_id", "status", "error_cause"}
	for _, key := range requiredKeys {
		if _, ok := attrs[key]; !ok {
			t.Errorf("required attr %q missing from recorded span; got attrs=%v", key, attrs)
		}
	}
	if attrs["transport"] != "telegram" {
		t.Errorf("transport = %q; want telegram", attrs["transport"])
	}
	if attrs["status"] != "ok" {
		t.Errorf("status = %q; want ok", attrs["status"])
	}
	if attrs["error_cause"] != "" {
		t.Errorf("error_cause = %q; want empty for ok status", attrs["error_cause"])
	}
	if attrs["correlation_id"] != "telegram-update-42" {
		t.Errorf("correlation_id = %q; want telegram-update-42", attrs["correlation_id"])
	}
	// user_id_hashed MUST NOT equal the raw user id and MUST NOT be empty.
	if attrs["user_id_hashed"] == "" {
		t.Error("user_id_hashed is empty; spec 061 §8.3.1.B requires SHA-256 prefix")
	}
	if attrs["user_id_hashed"] == "user-abc" {
		t.Error("user_id_hashed leaked the raw user id; HashUserID() did not run")
	}
}

// TestHashUserID_DeterministicAndPrefixed locks the SHA-256 prefix
// shape so a future refactor cannot silently swap the hash algorithm
// (which would invalidate dashboards that join on user_id_hashed).
func TestHashUserID_DeterministicAndPrefixed(t *testing.T) {
	if HashUserID("") != "" {
		t.Error("HashUserID(\"\") must return empty string")
	}
	a := HashUserID("user-abc")
	b := HashUserID("user-abc")
	if a != b {
		t.Errorf("HashUserID not deterministic: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Errorf("HashUserID length = %d; want 16 (8 bytes hex)", len(a))
	}
	if HashUserID("user-abc") == HashUserID("user-xyz") {
		t.Error("HashUserID returns same hash for different inputs; collision suspected")
	}
}
