// Spec 071 SCOPE-02 — Derived export fan-out (single source of truth).
//
// Hard Constraint (spec 071 design §"Observability"): exports must be
// derived from the persisted IntentTraceRow, not from a parallel
// telemetry path. The Exporter receives the row that was already
// validated + redacted + persisted and emits the three closed-vocab
// derived sinks:
//
//   structured_log → one slog.Info line per row
//   prometheus     → counter increment(s) with closed labels
//   otel           → attribute family stamped onto the active span (when present)
//
// The order is fixed so observers see logs in the same sequence as
// metric counters. Failures in one sink MUST NOT short-circuit the
// others — each derived sink is best-effort relative to persistence,
// which already succeeded by the time the exporter is invoked.

package intenttrace

import (
	"context"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	assistantintent "github.com/smackerel/smackerel/internal/assistant/intent"
)

// IntentTracesTotal counts persisted IntentTrace rows by transport,
// sampled bool, action class, and final response status. Cardinality
// is bound by the closed vocabularies pinned in types.go plus the
// finite action_class set declared by the spec 068 compiler.
var IntentTracesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_intent_traces_total",
		Help: "Persisted IntentTrace rows by transport, sampled flag, action_class, final_response_status (spec 071 SCOPE-02 export fan-out from validated row).",
	},
	[]string{"transport", "sampled", "action_class", "final_response_status"},
)

// IntentTraceRetentionSweepRowsTotal counts rows deleted by the
// retention sweep (SCN-071-A09). One increment per sweep call by the
// `Deleted` row count.
var IntentTraceRetentionSweepRowsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_assistant_intent_trace_retention_sweep_rows_total",
		Help: "Rows deleted by the IntentTrace retention sweep (spec 071 SCN-071-A09).",
	},
	[]string{"outcome"},
)

func init() {
	// Keep the registered family visible before the first compiled turn.
	// The labels are a valid closed-vocabulary refusal combination and
	// Add(0) records no synthetic event.
	IntentTracesTotal.WithLabelValues(
		string(TransportWeb),
		"true",
		string(assistantintent.ActionRefuse),
		string(StatusRefused),
	).Add(0)
	prometheus.MustRegister(IntentTracesTotal, IntentTraceRetentionSweepRowsTotal)
}

// Exporter fans the validated row out to the configured derived sinks.
// The recorder invokes Export AFTER a successful Put so the row that
// drives logs/metrics/OTel is provably the same row that was
// persisted.
type Exporter interface {
	Export(ctx context.Context, row IntentTraceRow)
}

// NopExporter is the zero exporter used in unit tests that do not
// care about fan-out.
type NopExporter struct{}

// Export implements Exporter.
func (NopExporter) Export(_ context.Context, _ IntentTraceRow) {}

// DefaultExporter implements the three-sink fan-out. Targets is the
// closed-vocab set ("structured_log", "prometheus", "otel"); each
// element gates the matching sink. An empty Targets disables every
// sink (useful in tests).
type DefaultExporter struct {
	Targets map[string]bool
}

// NewDefaultExporter constructs a DefaultExporter from the SST
// export_targets slice. Unknown targets are not silently dropped;
// callers are expected to validate the vocabulary via the config
// loader BEFORE constructing the exporter (loadIntentTraceConfig
// already enforces this).
func NewDefaultExporter(targets []string) *DefaultExporter {
	m := make(map[string]bool, len(targets))
	for _, t := range targets {
		m[t] = true
	}
	return &DefaultExporter{Targets: m}
}

// Export implements Exporter.
func (e *DefaultExporter) Export(ctx context.Context, row IntentTraceRow) {
	if e == nil {
		return
	}
	if e.Targets["structured_log"] {
		slog.InfoContext(ctx, "assistant_intent_trace",
			"schema_version", row.SchemaVersion,
			"trace_id", row.TraceID,
			"turn_id", row.TurnID,
			"user_id_hash", row.UserIDHash,
			"transport", string(row.Transport),
			"transport_message_id", row.TransportMessageID,
			"sampled", row.Sampled,
			"sampled_out_reason", row.SampledOutReason,
			"compiler_invoked", row.CompilerInvoked,
			"action_class", row.ActionClass,
			"side_effect_class", row.SideEffectClass,
			"route_decision", row.RouteDecision,
			"final_response_status", string(row.FinalResponseStatus),
			"refusal_cause", row.RefusalCause,
			"capture_cause", row.CaptureCause,
			"redacted_count", row.SlotsRedactionSummary.RedactedCount,
			"raw_text", row.SlotsRedactionSummary.RawText,
			"emitted_at", row.EmittedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		)
	}
	if e.Targets["prometheus"] {
		sampled := "false"
		if row.Sampled {
			sampled = "true"
		}
		actionClass := row.ActionClass
		if actionClass == "" {
			actionClass = "sampled_out"
		}
		status := string(row.FinalResponseStatus)
		if status == "" {
			status = "sampled_out"
		}
		IntentTracesTotal.WithLabelValues(
			string(row.Transport), sampled, actionClass, status,
		).Inc()
	}
	if e.Targets["otel"] {
		if span := trace.SpanFromContext(ctx); span != nil && span.SpanContext().IsValid() {
			span.SetAttributes(
				attribute.String("assistant.intent.trace_id", row.TraceID),
				attribute.String("assistant.intent.schema_version", row.SchemaVersion),
				attribute.Bool("assistant.intent.sampled", row.Sampled),
				attribute.String("assistant.intent.action_class", row.ActionClass),
				attribute.String("assistant.intent.side_effect_class", row.SideEffectClass),
				attribute.String("assistant.intent.route_decision", row.RouteDecision),
				attribute.String("assistant.intent.final_response_status", string(row.FinalResponseStatus)),
				attribute.Int("assistant.intent.redacted_count", row.SlotsRedactionSummary.RedactedCount),
			)
		}
	}
}
