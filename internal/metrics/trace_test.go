package metrics

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestTraceHeaders_EmptyTraceID(t *testing.T) {
	h := TraceHeaders("")
	if h.Get("traceparent") != "" {
		t.Error("expected no traceparent header for empty traceID")
	}
}

func TestTraceHeaders_WithTraceID(t *testing.T) {
	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	h := TraceHeaders(traceID)
	tp := h.Get("traceparent")
	if tp == "" {
		t.Fatal("expected traceparent header")
	}
	expected := "00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000001-01"
	if tp != expected {
		t.Errorf("expected %q, got %q", expected, tp)
	}
}

func TestExtractTraceID(t *testing.T) {
	h := nats.Header{}
	h.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	got := ExtractTraceID(h)
	if got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace ID, got %q", got)
	}
}

func TestExtractTraceID_Missing(t *testing.T) {
	h := nats.Header{}
	got := ExtractTraceID(h)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestExtractTraceID_Malformed(t *testing.T) {
	h := nats.Header{}
	h.Set("traceparent", "invalid")
	got := ExtractTraceID(h)
	if got != "" {
		t.Errorf("expected empty for malformed, got %q", got)
	}
}

func TestTraceRoundTrip(t *testing.T) {
	traceID := "abcdef1234567890abcdef1234567890"
	headers := TraceHeaders(traceID)
	extracted := ExtractTraceID(headers)
	if extracted != traceID {
		t.Errorf("round-trip failed: injected %q, extracted %q", traceID, extracted)
	}
}

func TestExtractTraceID_TooFewParts(t *testing.T) {
	h := nats.Header{}
	h.Set("traceparent", "00-traceid-parentid")
	got := ExtractTraceID(h)
	if got != "" {
		t.Errorf("expected empty for 3-part traceparent, got %q", got)
	}
}

func TestExtractTraceID_TooManyParts(t *testing.T) {
	h := nats.Header{}
	h.Set("traceparent", "00-traceid-parentid-01-extra")
	got := ExtractTraceID(h)
	if got != "" {
		t.Errorf("expected empty for 5-part traceparent, got %q", got)
	}
}
