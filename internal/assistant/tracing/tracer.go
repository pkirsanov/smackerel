// Package tracing wraps the OpenTelemetry SDK with a smackerel-shaped
// surface for the spec 061 conversational-assistant capability layer.
//
// Spec 061 SCOPE-09a (design §8.3.1 + §8.3.2 Step 1) ratified this
// package as the single seam through which every assistant span is
// emitted. SCOPE-09a wires ONE root span — `assistant.adapter.translate`
// — at the transport-adapter entry to prove the SDK → exporter →
// (no-op | sidecar) pipeline end-to-end; SCOPE-09b will instrument the
// remaining 8 mandatory + 1 conditional child spans on top of this
// substrate.
//
// Canonical attribute set per design §8.3.1.B:
//
//	transport          — closed-vocab transport name (e.g. "telegram")
//	user_id_hashed     — SHA-256 prefix of the canonical user_id
//	assistant_turn_id  — per-turn correlation id (ULID/UUID)
//	scenario_id        — closed-vocab scenario id ("" when not yet routed)
//	correlation_id     — transport-provided correlation token (e.g.
//	                     telegram_update_id) for slog ↔ trace join
//
// Outcome attributes set at span end:
//
//	status      — closed-vocab status token ("ok" | "error" | "noop")
//	error_cause — closed-vocab error cause; empty when status="ok"
package tracing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config is the subset of SST values NewTracer consumes. The caller
// (cmd/core/wiring.go) materializes this from config.AssistantObservabilityConfig
// so the tracing package never imports internal/config (avoids a
// dependency cycle if config ever wants tracing for its own paths).
type Config struct {
	// Enabled gates whether NewTracer returns a real OTLP/gRPC
	// TracerProvider (true) or the no-op TracerProvider (false).
	Enabled bool
	// Endpoint is the OTLP/gRPC exporter target. MUST be non-empty
	// when Enabled=true; ignored when Enabled=false.
	Endpoint string
	// ServiceName is the OTel resource service.name attribute. MUST
	// be non-empty regardless of Enabled state — the no-op tracer
	// still carries the resource for symmetric span shape.
	ServiceName string
}

// Tracer is the smackerel-shaped wrapper around otel.Tracer. Callers
// receive a non-nil Tracer in both enabled and disabled configurations;
// the no-op path returns spans that record nothing but are safe to
// call .End() on, so emission sites stay unconditional.
type Tracer struct {
	inner trace.Tracer
	// providerShutdown is the function the caller invokes during
	// graceful shutdown to flush any buffered spans. nil-safe.
	providerShutdown func(context.Context) error
}

// ShutdownFunc is the cleanup contract returned by NewTracer. It is
// always safe to invoke (returns nil immediately on the no-op path)
// and MUST be wired into the cmd/core/shutdown.go graceful-shutdown
// sequence.
type ShutdownFunc func(context.Context) error

// NewTracer constructs a *Tracer + ShutdownFunc pair from a Config.
//
// When cfg.Enabled is false, the returned tracer wraps a noop.NewTracerProvider
// — spans are no-ops but the API surface is identical, so emission
// sites never branch on enablement. The shutdown func is a no-op.
//
// When cfg.Enabled is true, the returned tracer wraps a real
// sdktrace.NewTracerProvider with:
//
//   - An OTLP/gRPC exporter pointing at cfg.Endpoint (insecure transport;
//     the sidecar is reached over the docker compose network).
//   - A BatchSpanProcessor for backpressure-friendly emission.
//   - A resource carrying service.name=cfg.ServiceName.
//
// Returns an error if cfg.Enabled=true AND the exporter cannot be
// constructed (per design §7.2 OTel validation rule — fail-loud at
// startup rather than silently dropping spans).
func NewTracer(ctx context.Context, cfg Config) (*Tracer, ShutdownFunc, error) {
	if cfg.ServiceName == "" {
		return nil, nil, fmt.Errorf("tracing.NewTracer: cfg.ServiceName is required (spec 061 SCOPE-09a design §8.3.2 Step 1)")
	}
	if !cfg.Enabled {
		// No-op path. We return a tracer whose inner is the
		// noop.NewTracerProvider tracer so StartSpan returns a
		// recording=false span. Shutdown is a no-op.
		return &Tracer{
			inner:            noop.NewTracerProvider().Tracer(cfg.ServiceName),
			providerShutdown: nil,
		}, func(context.Context) error { return nil }, nil
	}
	if cfg.Endpoint == "" {
		return nil, nil, fmt.Errorf("tracing.NewTracer: cfg.Endpoint is required when cfg.Enabled=true (spec 061 SCOPE-09a design §8.3.2 Step 1)")
	}
	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
		otlptracegrpc.WithInsecure(),
	))
	if err != nil {
		return nil, nil, fmt.Errorf("tracing.NewTracer: build OTLP/gRPC exporter at %q: %w", cfg.Endpoint, err)
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
	))
	if err != nil {
		return nil, nil, fmt.Errorf("tracing.NewTracer: build resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return &Tracer{
			inner:            tp.Tracer(cfg.ServiceName),
			providerShutdown: tp.Shutdown,
		}, func(ctx context.Context) error {
			if tp == nil {
				return nil
			}
			return tp.Shutdown(ctx)
		}, nil
}

// NewTracerFromProvider constructs a *Tracer from an externally-supplied
// trace.TracerProvider. Used by the in-memory exporter unit test to
// inject sdktrace.NewTracerProvider(WithSyncer(inmem)). The returned
// Tracer carries no shutdown closure — the test is responsible for
// lifecycle of the externally-supplied provider.
func NewTracerFromProvider(provider trace.TracerProvider, serviceName string) *Tracer {
	return &Tracer{
		inner: provider.Tracer(serviceName),
	}
}

// StartSpan begins a span and stamps the canonical attribute set from
// design §8.3.1.B. The caller MUST call .End() (via EndSpan) on the
// returned span. The returned context carries the span so descendant
// emissions (SCOPE-09b) attach as children automatically.
//
// transport / userIDHashed / assistantTurnID / scenarioID / correlationID
// are the 5 mandatory canonical attrs. scenarioID may legitimately be
// the empty string when called BEFORE routing has selected a scenario
// (e.g., adapter.translate runs pre-routing); empty is stamped
// unmodified so downstream observers can filter on "" to find
// pre-routing emissions.
func (t *Tracer) StartSpan(
	ctx context.Context,
	name string,
	transport, userIDHashed, assistantTurnID, scenarioID, correlationID string,
	extra ...attribute.KeyValue,
) (context.Context, trace.Span) {
	if t == nil {
		// Defensive: a nil tracer should not panic emission sites.
		// Return the input ctx + a noop span.
		nctx, nspan := noop.NewTracerProvider().Tracer("").Start(ctx, name)
		return nctx, nspan
	}
	attrs := append([]attribute.KeyValue{
		attribute.String("transport", transport),
		attribute.String("user_id_hashed", userIDHashed),
		attribute.String("assistant_turn_id", assistantTurnID),
		attribute.String("scenario_id", scenarioID),
		attribute.String("correlation_id", correlationID),
	}, extra...)
	return t.inner.Start(ctx, name, trace.WithAttributes(attrs...))
}

// EndSpan finalizes a span with the canonical outcome attributes from
// design §8.3.1.B. status is the closed-vocab token ("ok" | "error" |
// "noop"); errorCause is the closed-vocab cause and MUST be empty when
// status="ok". When status="error", the span status code is set to
// codes.Error so dashboards / Jaeger UI can filter by failure.
func EndSpan(span trace.Span, status, errorCause string) {
	if span == nil {
		return
	}
	span.SetAttributes(
		attribute.String("status", status),
		attribute.String("error_cause", errorCause),
	)
	if status == "error" {
		// errorCause is a closed-vocab token, not a free-form
		// message; use it verbatim as the span status description
		// for visibility in Jaeger UI.
		span.SetStatus(codes.Error, errorCause)
	}
	span.End()
}

// HashUserID returns the canonical 16-hex-character prefix of the
// SHA-256 of the supplied user_id. design §8.3.1.B requires that no
// raw user_id ever appears in span attributes; this helper is the
// single allowed stamping path. Empty input returns an empty string
// so callers do not need to pre-check.
func HashUserID(userID string) string {
	if userID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(userID))
	return hex.EncodeToString(sum[:8])
}
