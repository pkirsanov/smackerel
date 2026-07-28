package graphsynthetic

// telemetry.go — the concrete metrics + trace + log adapter behind the
// generic Observer seam.
//
// TRACE-WORKFLOW BOUNDARY (BUG-080-001 SCOPE-03 Observability Evidence
// Contract): this repository registers exactly ONE trace workflow,
// `core.health`, and that workflow is UNRELATED to the Knowledge Graph.
// This adapter therefore:
//
//   - does NOT attach an `observabilityWorkflow` of any kind,
//   - does NOT declare or claim a graph-specific G080/G100 trace or SLO
//     contract, and
//   - does NOT emit into, reuse, or otherwise misuse `core.health`.
//
// It emits PLAIN spans through the existing generic tracer with a
// graph-owned span name and value-safe attributes only. A graph trace
// workflow, if one is ever wanted, is a separate operator-owned
// registration and is deliberately not invented here.
//
// VALUE SAFETY (SCN-080-001-07): every metric label, span attribute,
// and log attribute is drawn from a closed vocabulary — a canonical
// family name, a closed state, a closed diagnostic code, a bounded
// duration, and a family-derived evidence reference. There is no path
// in this file that can emit a label, an id, a query value, a cursor
// body, a credential, secret material, or a target host.

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/attribute"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/assistant/tracing"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Span names owned by the graph read synthetic. They are plain span
// names, NOT registered trace-workflow identifiers, and they are
// deliberately disjoint from `core.health`.
const (
	// SpanActivation covers the fail-soft activation resolution.
	SpanActivation = "graph.activation"
	// SpanFamilyRead covers one canonical family read.
	SpanFamilyRead = "graph.family_read"
	// SpanAggregate covers the aggregate reduction and publication.
	SpanAggregate = "graph.synthetic_aggregate"
)

// TelemetryObserver emits closed metrics, plain spans, and structured
// logs for activation and family reads. Construct it with
// NewTelemetryObserver.
type TelemetryObserver struct {
	tracer *tracing.Tracer
	logger *slog.Logger
}

// NewTelemetryObserver returns the concrete telemetry adapter.
//
// tracer MAY be nil: span emission is then skipped while metrics and
// logs continue, matching the existing nil-tracer convention in
// internal/api (a metrics-only deployment is a supported posture, not a
// silent degradation of a required signal).
//
// logger MAY be nil: the process default logger is used.
func NewTelemetryObserver(tracer *tracing.Tracer, logger *slog.Logger) *TelemetryObserver {
	if logger == nil {
		logger = slog.Default()
	}
	return &TelemetryObserver{tracer: tracer, logger: logger}
}

// ObserveActivation emits the activation counter, a plain activation
// span, and a value-safe activation log. The cursor secret, its length,
// its hash, and every derivative are structurally absent: Activation
// carries only the state, the PRESENCE CLASS, and a closed code.
func (t *TelemetryObserver) ObserveActivation(activation graphapi.Activation) {
	mode := string(activation.State)
	outcome := "enabled"
	if activation.Disabled() {
		outcome = "disabled"
	}
	metrics.GraphActivationTotal.WithLabelValues(mode, outcome, activation.Code).Inc()

	t.emitSpan(SpanActivation,
		attribute.String("graph.activation.mode", mode),
		attribute.String("graph.activation.outcome", outcome),
		attribute.String("graph.activation.code", activation.Code),
		attribute.String("graph.activation.secret_presence", string(activation.SecretPresence)),
	)

	t.logger.Info("graph activation resolved",
		"mode", mode,
		"outcome", outcome,
		"code", activation.Code,
		"secret_presence", string(activation.SecretPresence),
	)
}

// ObserveFamilyRead emits the per-family counter and latency histogram,
// a plain family-read span, and a value-safe log line.
func (t *TelemetryObserver) ObserveFamilyRead(result GraphFamilyResult) {
	family := string(result.Family)
	outcome := string(result.State)

	metrics.GraphReadRequestsTotal.WithLabelValues(family, outcome).Inc()
	metrics.GraphReadDurationSeconds.WithLabelValues(family, outcome).
		Observe(float64(result.DurationMs) / 1000.0)

	t.emitSpan(SpanFamilyRead,
		attribute.String("graph.read.family", family),
		attribute.String("graph.read.outcome", outcome),
		attribute.String("graph.read.code", result.Code),
		attribute.String("graph.read.evidence_ref", result.EvidenceRef),
		attribute.Int64("graph.read.duration_ms", result.DurationMs),
	)

	level := slog.LevelInfo
	if result.State == StateFailed {
		level = slog.LevelWarn
	}
	t.logger.Log(context.Background(), level, "graph family read observed",
		"family", family,
		"state", outcome,
		"code", result.Code,
		"evidence_ref", result.EvidenceRef,
		"duration_ms", durationMillis(result.DurationMs),
	)
}

// ObserveAggregate publishes the ONE-HOT aggregate gauge, a plain
// aggregate span, and a value-safe log line. Every declared aggregate
// state is written on each observation — the current state to 1 and all
// others to 0 — so a stale series can never be mistaken for the current
// truth.
func (t *TelemetryObserver) ObserveAggregate(result AggregateResult) {
	for _, state := range aggregateStates {
		value := 0.0
		if state == result.State {
			value = 1.0
		}
		metrics.GraphSyntheticResult.WithLabelValues(string(state)).Set(value)
	}

	t.emitSpan(SpanAggregate,
		attribute.String("graph.synthetic.activation", string(result.Activation)),
		attribute.String("graph.synthetic.state", string(result.State)),
		attribute.String("graph.synthetic.code", result.Code),
		attribute.String("graph.synthetic.evidence_ref", result.EvidenceRef),
		attribute.Int64("graph.synthetic.duration_ms", result.DurationMs),
		attribute.Int("graph.synthetic.family_count", len(result.Families)),
	)

	level := slog.LevelInfo
	if result.State == AggregateUnavailable {
		level = slog.LevelWarn
	}
	t.logger.Log(context.Background(), level, "graph read synthetic aggregate observed",
		"activation", string(result.Activation),
		"state", string(result.State),
		"code", result.Code,
		"evidence_ref", result.EvidenceRef,
		"duration_ms", durationMillis(result.DurationMs),
		"family_count", len(result.Families),
	)
}

// emitSpan starts and immediately ends a plain span carrying only the
// supplied value-safe attributes. The generic tracer's identity
// parameters (transport, hashed user, turn, scenario, correlation) are
// intentionally EMPTY: a synthetic observation belongs to no user
// session, no assistant turn, and no registered trace workflow.
func (t *TelemetryObserver) emitSpan(name string, attrs ...attribute.KeyValue) {
	if t.tracer == nil {
		return
	}
	_, span := t.tracer.StartSpan(context.Background(), name, "", "", "", "", "", attrs...)
	tracing.EndSpan(span, "ok", "")
}
