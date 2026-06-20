// Spec 037 LLM Scenario Agent observability — round 22 stochastic-quality-sweep
// devops finding F-037-DEVOPS-001.
//
// The LLM scenario agent (internal/agent) is a live runtime surface: it
// routes intents to scenarios, drives an LLM over NATS, executes
// allowlisted tools, and persists a trace per invocation. Before this
// finding it emitted DB trace rows (agent_traces) and two NATS events
// (agent.tool_call.executed, agent.complete) but ZERO Prometheus metrics —
// so an operator running the monitoring stack could not be ALERTED when
// the agent degraded (LLM provider unreachable, scenario timeout budget
// structurally unreachable, a prompt-injection burst tripping tool
// allowlists). Every comparable subsystem (ingestion, search, connectors,
// NATS, DB, ML embedding, backup) already had Prometheus alerting; the
// agent was the conspicuous gap. spec.md "### Observability" explicitly
// required "Aggregate metrics by scenario: invocation count, success /
// unknown-intent / schema-failure / loop-limit / tool-error / timeout
// counts, p50/p95 latency, average tool calls per invocation" plus
// "Allowlist violations and hallucinated tool calls counted per scenario".
// These three metrics satisfy that NFR and back the smackerel-agent alert
// group in config/prometheus/alerts.yml.
//
// Cardinality is bounded by construction (see the label-cardinality
// contract at the top of metrics.go):
//   - `scenario` is the repo-controlled scenario id, or "unrouted" when
//     intent routing produced no scenario (unknown-intent).
//   - `outcome` is the closed agent.Outcome enum (11 terminal values).
//   - `tool` is the REGISTERED tool name, or "unregistered" for a name the
//     LLM invented (hallucinated-tool). Bucketing unregistered names is a
//     hard cardinality guard: without it a hallucinating or adversarial
//     model could mint unbounded label values and blow up Prometheus
//     head-block memory. The emission site (internal/agent/tracer.go)
//     looks the name up in the tool registry before labeling.
//   - `result` is the closed per-call ExecutedToolCall.Outcome enum
//     (ok | hallucinated-tool | allowlist-violation | tool-error |
//     tool-return-invalid).
package metrics

import "github.com/prometheus/client_golang/prometheus"

// AgentInvocations counts scenario-agent invocations by scenario and
// terminal outcome. Backs SmackerelAgentProviderErrors and
// SmackerelAgentInvocationTimeouts; satisfies the per-scenario
// invocation-count + outcome-class NFR.
var AgentInvocations = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_agent_invocations_total",
		Help: "LLM scenario-agent invocations by scenario and terminal outcome",
	},
	[]string{"scenario", "outcome"},
)

// AgentInvocationDuration records end-to-end invocation wall-clock per
// scenario. Backs the p50/p95 latency NFR
// (histogram_quantile over the _bucket series) and the LLM-agent latency
// dashboard panel. Buckets span sub-second deterministic scenarios through
// the multi-second-to-minutes LLM calls the timeout ceiling permits.
var AgentInvocationDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "smackerel_agent_invocation_duration_seconds",
		Help:    "LLM scenario-agent end-to-end invocation latency in seconds by scenario",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120},
	},
	[]string{"scenario"},
)

// AgentToolCalls counts tool-call attempts by scenario, tool, and the
// per-call result. Backs SmackerelAgentAllowlistViolations and the
// "average tool calls per invocation" NFR
// (rate(tool_calls)/rate(invocations)). The result label carries the
// security-relevant per-call outcomes (allowlist-violation,
// hallucinated-tool) the spec.md Observability NFR demands be counted.
var AgentToolCalls = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "smackerel_agent_tool_calls_total",
		Help: "LLM scenario-agent tool-call attempts by scenario, registered tool, and per-call result",
	},
	[]string{"scenario", "tool", "result"},
)
