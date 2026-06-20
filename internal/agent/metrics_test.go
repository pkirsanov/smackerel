package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/metrics"
)

// Round 22 stochastic-quality-sweep devops finding F-037-DEVOPS-001 —
// proves the PostgresTracer emits the agent Prometheus metrics that back
// the smackerel-agent alert group in config/prometheus/alerts.yml. These
// tests exercise ONLY the metric path: the tracer is built with a nil
// pool (writeTrace early-returns) and a NopPublisher, so no Postgres or
// NATS is required.

// metricsToolSeq makes registered-tool names unique across repeated runs
// (-count>1) so RegisterTool's duplicate-name panic cannot trip the test.
var metricsToolSeq atomic.Int64

// newMetricsOnlyTracer builds a PostgresTracer with no DB pool and a
// NopPublisher so the Record* methods exercise only Prometheus emission.
func newMetricsOnlyTracer() *PostgresTracer {
	return &PostgresTracer{
		publisher: NopPublisher{},
		pads:      make(map[string]*tracePad),
		publishCtxFn: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), time.Second)
		},
	}
}

// durationSampleCount returns the histogram sample count for one scenario
// label of AgentInvocationDuration via the dto projection.
func durationSampleCount(t *testing.T, scenario string) uint64 {
	t.Helper()
	o, err := metrics.AgentInvocationDuration.GetMetricWithLabelValues(scenario)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues(%q): %v", scenario, err)
	}
	m, ok := o.(prometheus.Metric)
	if !ok {
		t.Fatalf("histogram child is not a prometheus.Metric")
	}
	var dm dto.Metric
	if err := m.Write(&dm); err != nil {
		t.Fatalf("write histogram dto: %v", err)
	}
	return dm.GetHistogram().GetSampleCount()
}

// TestPostgresTracer_RecordOutcome_EmitsInvocationMetrics proves a single
// RecordOutcome increments the per-(scenario,outcome) counter exactly once
// and records exactly one latency-histogram sample.
func TestPostgresTracer_RecordOutcome_EmitsInvocationMetrics(t *testing.T) {
	tr := newMetricsOnlyTracer()
	const scenario = "metricstest_invocation"
	const traceID = "metricstest-inv-1"
	tr.Begin(TraceContext{TraceID: traceID, Scenario: &Scenario{ID: scenario}, StartedAt: time.Now()})

	beforeCtr := testutil.ToFloat64(metrics.AgentInvocations.WithLabelValues(scenario, string(OutcomeTimeout)))
	beforeHist := durationSampleCount(t, scenario)

	start := time.Date(2026, time.June, 17, 0, 0, 0, 0, time.UTC)
	tr.RecordOutcome(traceID, &InvocationResult{
		TraceID:    traceID,
		ScenarioID: scenario,
		Outcome:    OutcomeTimeout,
		StartedAt:  start,
		EndedAt:    start.Add(1500 * time.Millisecond),
	})

	if got := testutil.ToFloat64(metrics.AgentInvocations.WithLabelValues(scenario, string(OutcomeTimeout))) - beforeCtr; got != 1 {
		t.Fatalf("AgentInvocations{%s,timeout} delta = %v, want 1", scenario, got)
	}
	if got := durationSampleCount(t, scenario) - beforeHist; got != 1 {
		t.Fatalf("AgentInvocationDuration{%s} sample delta = %d, want 1", scenario, got)
	}
}

// TestPostgresTracer_RecordOutcome_UnroutedScenarioBucket proves an
// unknown-intent invocation (empty scenario id) is bucketed under the
// fixed "unrouted" label rather than an empty label value.
func TestPostgresTracer_RecordOutcome_UnroutedScenarioBucket(t *testing.T) {
	tr := newMetricsOnlyTracer()
	const traceID = "metricstest-unrouted-1"
	tr.Begin(TraceContext{TraceID: traceID, Scenario: &Scenario{ID: ""}, StartedAt: time.Now()})

	before := testutil.ToFloat64(metrics.AgentInvocations.WithLabelValues("unrouted", string(OutcomeUnknownIntent)))
	start := time.Now()
	tr.RecordOutcome(traceID, &InvocationResult{
		TraceID:    traceID,
		ScenarioID: "",
		Outcome:    OutcomeUnknownIntent,
		StartedAt:  start,
		EndedAt:    start.Add(10 * time.Millisecond),
	})
	if got := testutil.ToFloat64(metrics.AgentInvocations.WithLabelValues("unrouted", string(OutcomeUnknownIntent))) - before; got != 1 {
		t.Fatalf("AgentInvocations{unrouted,unknown-intent} delta = %v, want 1", got)
	}
}

// TestPostgresTracer_ToolCall_HallucinatedNameBucketedUnregistered is the
// cardinality-guard proof: a tool name the LLM invented (NOT in the
// registry) MUST be counted under the fixed "unregistered" tool label and
// MUST NOT leak the invented name into a metric label. Without this guard
// a hallucinating or adversarial model could mint unbounded label values.
func TestPostgresTracer_ToolCall_HallucinatedNameBucketedUnregistered(t *testing.T) {
	tr := newMetricsOnlyTracer()
	const scenario = "metricstest_tool_hallucinated"
	const traceID = "metricstest-tool-1"
	tr.Begin(TraceContext{TraceID: traceID, Scenario: &Scenario{ID: scenario}, StartedAt: time.Now()})

	const invented = "delete_everything_99f3c1a7_invented_by_llm"
	before := testutil.ToFloat64(metrics.AgentToolCalls.WithLabelValues(scenario, "unregistered", string(OutcomeHallucinatedTool)))
	// Negative control: the invented-name series must NEVER receive a sample.
	beforeInvented := testutil.ToFloat64(metrics.AgentToolCalls.WithLabelValues(scenario, invented, string(OutcomeHallucinatedTool)))

	tr.RecordRejection(traceID, ExecutedToolCall{
		Seq:             0,
		Name:            invented,
		Outcome:         OutcomeHallucinatedTool,
		RejectionReason: "unknown_tool",
	})

	if got := testutil.ToFloat64(metrics.AgentToolCalls.WithLabelValues(scenario, "unregistered", string(OutcomeHallucinatedTool))) - before; got != 1 {
		t.Fatalf("AgentToolCalls{%s,unregistered,hallucinated-tool} delta = %v, want 1", scenario, got)
	}
	if got := testutil.ToFloat64(metrics.AgentToolCalls.WithLabelValues(scenario, invented, string(OutcomeHallucinatedTool))) - beforeInvented; got != 0 {
		t.Fatalf("invented tool name leaked into a metric label (delta=%v) — cardinality guard failed", got)
	}
}

// TestPostgresTracer_ToolCall_RegisteredAllowlistViolation proves the
// registered-tool path keeps the real tool name AND that the result label
// carries the security-relevant allowlist-violation per-call outcome that
// the SmackerelAgentAllowlistViolations alert watches.
func TestPostgresTracer_ToolCall_RegisteredAllowlistViolation(t *testing.T) {
	tr := newMetricsOnlyTracer()
	const scenario = "metricstest_tool_allowlist"
	const traceID = "metricstest-tool-2"
	tr.Begin(TraceContext{TraceID: traceID, Scenario: &Scenario{ID: scenario}, StartedAt: time.Now()})

	tool := fmt.Sprintf("metricstest_read_tool_%d", metricsToolSeq.Add(1))
	registerEchoTool(t, tool)

	before := testutil.ToFloat64(metrics.AgentToolCalls.WithLabelValues(scenario, tool, string(OutcomeAllowlistViolation)))
	tr.RecordRejection(traceID, ExecutedToolCall{
		Seq:             0,
		Name:            tool,
		Outcome:         OutcomeAllowlistViolation,
		RejectionReason: "not_in_allowlist",
	})
	if got := testutil.ToFloat64(metrics.AgentToolCalls.WithLabelValues(scenario, tool, string(OutcomeAllowlistViolation))) - before; got != 1 {
		t.Fatalf("AgentToolCalls{%s,%s,allowlist-violation} delta = %v, want 1", scenario, tool, got)
	}
}
