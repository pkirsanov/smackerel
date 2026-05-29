package assistant_adapter

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/smackerel/smackerel/internal/assistant/tracing"
)

// TestAdapter_Translate_EmitsRootSpan is the spec 061 SCOPE-09a
// DoD #3f acceptance: the adapter's Translate method MUST emit
// exactly ONE span named `assistant.adapter.translate` carrying the
// 5 mandatory canonical attrs from design §8.3.1.B + the 2 outcome
// attrs (status + error_cause) stamped by tracing.EndSpan.
func TestAdapter_Translate_EmitsRootSpan(t *testing.T) {
	t.Parallel()
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	tr := tracing.NewTracerFromProvider(provider, "smackerel-core")

	a, err := NewAdapter(Options{
		Sender:          &recordingSender{},
		Capture:         (&recordingCapture{}).fn(),
		ResolveUser:     fixedResolver("user-061-test"),
		MarkdownMode:    MarkdownV2,
		MaxMessageChars: 4096,
		Tracer:          tr,
	})
	if err != nil {
		t.Fatalf("NewAdapter err = %v", err)
	}

	update := updateWithText(123, 4242, "hello there")
	update.UpdateID = 9999
	msg, err := a.Translate(context.Background(), update)
	if err != nil {
		t.Fatalf("Translate err = %v; want nil", err)
	}
	if msg.UserID != "user-061-test" {
		t.Errorf("msg.UserID = %q; want user-061-test", msg.UserID)
	}

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected exactly 1 span; got %d", len(spans))
	}
	got := spans[0]
	if got.Name != "assistant.adapter.translate" {
		t.Errorf("span name = %q; want assistant.adapter.translate", got.Name)
	}
	attrs := map[string]string{}
	for _, kv := range got.Attributes {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	// 5 mandatory canonical attrs from design §8.3.1.B.
	required := []string{"transport", "user_id_hashed", "assistant_turn_id", "scenario_id", "correlation_id"}
	for _, key := range required {
		if _, ok := attrs[key]; !ok {
			t.Errorf("required attr %q missing; attrs=%v", key, attrs)
		}
	}
	if attrs["transport"] != "telegram" {
		t.Errorf("transport = %q; want telegram", attrs["transport"])
	}
	// adapter-translate is the pre-routing site, so scenario_id MUST
	// be empty (downstream SCOPE-09b spans will stamp it).
	if attrs["scenario_id"] != "" {
		t.Errorf("scenario_id = %q; want empty at adapter-translate (pre-routing)", attrs["scenario_id"])
	}
	// correlation_id MUST be the telegram_update_id (9999) per §18.6.
	if attrs["correlation_id"] != "9999" {
		t.Errorf("correlation_id = %q; want 9999", attrs["correlation_id"])
	}
	// user_id_hashed MUST be the hashed form, not the raw "user-061-test".
	if attrs["user_id_hashed"] == "user-061-test" {
		t.Error("user_id_hashed leaked the raw user_id; HashUserID() did not run")
	}
	if attrs["user_id_hashed"] == "" {
		t.Error("user_id_hashed empty; expected SHA-256 prefix after resolve")
	}
	// Outcome attrs from EndSpan.
	if attrs["status"] != "ok" {
		t.Errorf("status = %q; want ok", attrs["status"])
	}
	if attrs["error_cause"] != "" {
		t.Errorf("error_cause = %q; want empty for ok", attrs["error_cause"])
	}
}

// TestAdapter_Translate_InvalidPayloadEmitsErrorSpan asserts the
// adversarial path: a non-Update payload still produces ONE span,
// but with status="error" + error_cause="invalid_payload" so
// dashboards can count transport-misuse incidents.
func TestAdapter_Translate_InvalidPayloadEmitsErrorSpan(t *testing.T) {
	t.Parallel()
	exp := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	tr := tracing.NewTracerFromProvider(provider, "smackerel-core")

	a, err := NewAdapter(Options{
		Sender:          &recordingSender{},
		Capture:         (&recordingCapture{}).fn(),
		ResolveUser:     fixedResolver("u1"),
		MarkdownMode:    PlainText,
		MaxMessageChars: 4096,
		Tracer:          tr,
	})
	if err != nil {
		t.Fatalf("NewAdapter err = %v", err)
	}

	// Pass a non-Update payload — should fail-loud and still emit one span.
	_, err = a.Translate(context.Background(), (*tgbotapi.Update)(nil))
	if err == nil {
		t.Fatal("Translate(nil) err = nil; want non-nil")
	}
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected exactly 1 span on error path; got %d", len(spans))
	}
	attrs := map[string]string{}
	for _, kv := range spans[0].Attributes {
		attrs[string(kv.Key)] = kv.Value.AsString()
	}
	if attrs["status"] != "error" {
		t.Errorf("status = %q; want error", attrs["status"])
	}
	if attrs["error_cause"] != "invalid_payload" {
		t.Errorf("error_cause = %q; want invalid_payload", attrs["error_cause"])
	}
}
