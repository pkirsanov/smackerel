// Package metrics provides trace context helpers for NATS header propagation.
package metrics

import (
	"github.com/nats-io/nats.go"
)

// TraceHeaders returns NATS headers with W3C traceparent if traceID is non-empty.
// This provides NATS-level trace context propagation without a hard dependency
// on the OpenTelemetry SDK. When OTEL_ENABLED=false, callers pass empty traceID
// and no headers are added (zero overhead).
func TraceHeaders(traceID string) nats.Header {
	h := nats.Header{}
	if traceID != "" {
		// W3C traceparent format: version-traceid-parentid-flags
		// Using a placeholder parent span ID and sampled flag
		h.Set("traceparent", "00-"+traceID+"-0000000000000001-01")
	}
	return h
}

// ExtractTraceID extracts the trace ID from W3C traceparent NATS header.
// Returns empty string if header is missing or malformed.
func ExtractTraceID(headers nats.Header) string {
	tp := headers.Get("traceparent")
	if tp == "" {
		return ""
	}
	// W3C traceparent format: version-traceid-parentid-flags
	// e.g., "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	parts := splitTraceparent(tp)
	if len(parts) != 4 {
		return ""
	}
	return parts[1]
}

// splitTraceparent splits a traceparent header into its components.
func splitTraceparent(tp string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(tp); i++ {
		if tp[i] == '-' {
			parts = append(parts, tp[start:i])
			start = i + 1
		}
	}
	parts = append(parts, tp[start:])
	return parts
}
