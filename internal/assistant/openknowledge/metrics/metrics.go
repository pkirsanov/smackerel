// Package metrics — Spec 064 SCOPE-14 open-knowledge agent metrics.
//
// Closed-vocabulary Prometheus surface for the open-knowledge
// subsystem. All label values come from agent-owned enums (registered
// tools, contracts.AllRefusalCauses, the five-value BudgetScope set);
// unknown values are dropped at the increment site so an LLM that
// returns garbage cannot inflate Prometheus cardinality (G021).
//
// NO-DEFAULTS (G028): every histogram bucket is declared as a named
// var (no magic numbers at call sites); the Metrics struct is built
// against an explicit allowed-tools list passed in by the wiring
// layer; nothing registers itself into the default registry — the
// caller passes a prometheus.Registerer to Register().
package metrics

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// ----- Closed-vocabulary label values -----

// Tool-call outcome label values for openknowledge_tool_calls_total.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// AllToolOutcomes is the exhaustive list used by cardinality tests.
var AllToolOutcomes = []string{OutcomeSuccess, OutcomeError}

// Budget scope label values for openknowledge_budget_exhausted_total.
// The five scopes mirror the five cap sites in the agent loop:
// MaxIterations (iterations), PerQueryTokenBudget (tokens),
// PerQueryUSDBudget (usd), MonthlyBudgetUSDRemaining (monthly),
// PerUserMonthlyUSDRemaining (per_user_monthly).
const (
	BudgetScopeIterations     = "iterations"
	BudgetScopeTokens         = "tokens"
	BudgetScopeUSD            = "usd"
	BudgetScopeMonthly        = "monthly"
	BudgetScopePerUserMonthly = "per_user_monthly"
)

// AllBudgetScopes is the exhaustive list used by cardinality tests.
var AllBudgetScopes = []string{
	BudgetScopeIterations,
	BudgetScopeTokens,
	BudgetScopeUSD,
	BudgetScopeMonthly,
	BudgetScopePerUserMonthly,
}

// ----- Histogram buckets (named — no magic numbers at call sites) -----

// IterationBuckets covers v1's MaxIterations envelope; the upper
// bucket (13) is the Fibonacci step past the design's nominal cap of
// 8 so over-cap behaviour shows up in p99 without producing +Inf only.
var IterationBuckets = []float64{1, 2, 3, 5, 8, 13}

// TokenBuckets spans an order-of-magnitude ladder from 100 tokens
// (tiny calculator-only turns) to 100k tokens (the largest plausible
// per-query token cap before SST blocks the wiring).
var TokenBuckets = []float64{100, 500, 1000, 2500, 5000, 10000, 25000, 50000, 100000}

// USDCentsBuckets is denominated in cents so the histogram floor
// (0.1¢) is representable; the ceiling (1000¢ = $10) is well above
// any plausible per-query USD cap operators would configure.
var USDCentsBuckets = []float64{0.1, 0.5, 1, 5, 10, 50, 100, 500, 1000}

// ToolLatencyBuckets covers calculator-fast (5 ms) through long web
// searches (30 s). The provider per-call timeout is SST-bounded but
// can be tens of seconds for SearxNG.
var ToolLatencyBuckets = []float64{0.005, 0.025, 0.1, 0.5, 1, 2, 5, 10, 30}

// ----- Metric series names (exported so name-regression tests can pin them) -----

const (
	NameToolCalls          = "openknowledge_tool_calls_total"
	NameIterations         = "openknowledge_iterations_per_query"
	NameTokens             = "openknowledge_tokens_per_query"
	NameUSDCents           = "openknowledge_usd_cents_per_query"
	NameToolLatency        = "openknowledge_tool_latency_seconds"
	NameBudgetExhausted    = "openknowledge_budget_exhausted_total"
	NameFabricatedSource   = "openknowledge_fabricated_source_total"
	NameRefusal            = "openknowledge_refusal_total"
	NameCompactionSignaled = "openknowledge_compaction_signaled_total"
	// NameSuspiciousSnippet — SCOPE-15 security observability:
	// counts web-provider snippets that matched a prompt-injection
	// trigger pattern in SanitizeSnippet. Label `provider` is bounded
	// by the configured web provider name ("searxng" today; future
	// providers must be added on both the web/ and metrics/ sides).
	NameSuspiciousSnippet = "openknowledge_suspicious_snippet_total"
	// NameCircuitState — SCOPE-16 resilience observability: gauge
	// reporting the current state of each web-provider circuit
	// breaker. Values: 0=closed, 1=half_open, 2=open. Label
	// `provider` is bounded by the same allow-set as
	// NameSuspiciousSnippet.
	NameCircuitState = "openknowledge_circuit_state"
	// NameCircuitTrips — SCOPE-16: counter incremented every time a
	// web-provider circuit breaker transitions Closed→Open or
	// HalfOpen→Open. Bounded `provider` label.
	NameCircuitTrips = "openknowledge_circuit_trips_total"
)

// ----- Recorder contract -----

// Recorder is the narrow surface the agent loop uses to emit metrics.
// Implementations:
//   - *Metrics (live Prometheus)
//   - Nop (test / disabled subsystem)
type Recorder interface {
	RecordTurn(iterations int, tokens int, usdCents float64)
	IncToolCall(tool, outcome string)
	ObserveToolLatency(tool string, seconds float64)
	IncBudgetExhausted(scope string)
	IncFabricatedSource()
	IncRefusal(cause string)
	IncCompactionSignaled()
}

// Nop is a no-op Recorder used by tests that don't care about metrics
// and by the agent when wiring left the recorder unset.
type Nop struct{}

// RecordTurn — no-op.
func (Nop) RecordTurn(int, int, float64) {}

// IncToolCall — no-op.
func (Nop) IncToolCall(string, string) {}

// ObserveToolLatency — no-op.
func (Nop) ObserveToolLatency(string, float64) {}

// IncBudgetExhausted — no-op.
func (Nop) IncBudgetExhausted(string) {}

// IncFabricatedSource — no-op.
func (Nop) IncFabricatedSource() {}

// IncRefusal — no-op.
func (Nop) IncRefusal(string) {}

// IncCompactionSignaled — no-op.
func (Nop) IncCompactionSignaled() {}

// ----- Metrics struct (live Prometheus implementation) -----

// Metrics holds the live Prometheus collectors. Construct with New;
// register against the application Registerer with Register; pass as
// a Recorder into the agent loop.
type Metrics struct {
	toolCalls          *prometheus.CounterVec
	iterations         prometheus.Histogram
	tokens             prometheus.Histogram
	usdCents           prometheus.Histogram
	toolLatency        *prometheus.HistogramVec
	budgetExhausted    *prometheus.CounterVec
	fabricatedSource   prometheus.Counter
	refusal            *prometheus.CounterVec
	compactionSignaled prometheus.Counter
	suspiciousSnippet  *prometheus.CounterVec
	circuitState       *prometheus.GaugeVec
	circuitTrips       *prometheus.CounterVec

	allowedTools     map[string]struct{}
	allowedCauses    map[string]struct{}
	allowedScopes    map[string]struct{}
	allowedProviders map[string]struct{}
}

// ErrUnknownTool is returned by callers that want to fail loud on an
// unknown tool label rather than silently drop. Exposed for tests;
// the increment helpers themselves silently drop (G021 — no
// cardinality leak via uncontrolled labels).
var ErrUnknownTool = errors.New("openknowledge/metrics: unknown tool label")

// New constructs a Metrics with the supplied closed-vocabulary
// allow-set for tools. The cause allow-set comes from
// contracts.AllRefusalCauses; the scope allow-set comes from
// AllBudgetScopes — both are looked up internally so callers cannot
// widen them by accident.
//
// allowedProviders bounds the `provider` label on
// openknowledge_suspicious_snippet_total (SCOPE-15). Unknown
// providers silently drop the increment (G021 — no cardinality
// leak).
func New(allowedTools []string, allowedProviders ...string) *Metrics {
	toolSet := make(map[string]struct{}, len(allowedTools))
	for _, t := range allowedTools {
		toolSet[t] = struct{}{}
	}
	causeSet := make(map[string]struct{}, len(contracts.AllRefusalCauses))
	for _, c := range contracts.AllRefusalCauses {
		causeSet[string(c)] = struct{}{}
	}
	scopeSet := make(map[string]struct{}, len(AllBudgetScopes))
	for _, s := range AllBudgetScopes {
		scopeSet[s] = struct{}{}
	}
	providerSet := make(map[string]struct{}, len(allowedProviders))
	for _, p := range allowedProviders {
		providerSet[p] = struct{}{}
	}
	return &Metrics{
		toolCalls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameToolCalls,
			Help: "Open-knowledge tool invocations by tool and outcome (success|error).",
		}, []string{"tool", "outcome"}),
		iterations: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    NameIterations,
			Help:    "Open-knowledge agent loop iterations per user turn.",
			Buckets: IterationBuckets,
		}),
		tokens: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    NameTokens,
			Help:    "Open-knowledge LLM tokens (prompt+completion) per user turn.",
			Buckets: TokenBuckets,
		}),
		usdCents: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    NameUSDCents,
			Help:    "Open-knowledge USD spend per user turn, denominated in cents.",
			Buckets: USDCentsBuckets,
		}),
		toolLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    NameToolLatency,
			Help:    "Open-knowledge tool Execute latency in seconds, by tool.",
			Buckets: ToolLatencyBuckets,
		}, []string{"tool"}),
		budgetExhausted: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameBudgetExhausted,
			Help: "Open-knowledge budget-exhaustion events by scope (iterations|tokens|usd|monthly|per_user_monthly).",
		}, []string{"scope"}),
		fabricatedSource: prometheus.NewCounter(prometheus.CounterOpts{
			Name: NameFabricatedSource,
			Help: "Open-knowledge cite-back rejections (planner cited a source absent from the tool trace).",
		}),
		refusal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameRefusal,
			Help: "Open-knowledge refusals by cause (closed vocabulary from contracts.AllRefusalCauses).",
		}, []string{"cause"}),
		compactionSignaled: prometheus.NewCounter(prometheus.CounterOpts{
			Name: NameCompactionSignaled,
			Help: "Open-knowledge turns that crossed the compaction threshold (G083 signal).",
		}),
		suspiciousSnippet: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameSuspiciousSnippet,
			Help: "Open-knowledge web snippets that matched a prompt-injection trigger pattern (SCOPE-15 security observability; content is NOT stripped).",
		}, []string{"provider"}),
		circuitState: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: NameCircuitState,
			Help: "Open-knowledge web-provider circuit breaker state (0=closed, 1=half_open, 2=open). SCOPE-16 resilience observability.",
		}, []string{"provider"}),
		circuitTrips: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameCircuitTrips,
			Help: "Open-knowledge web-provider circuit breaker trip count (Closed→Open or HalfOpen→Open transitions). SCOPE-16 resilience observability.",
		}, []string{"provider"}),
		allowedTools:     toolSet,
		allowedCauses:    causeSet,
		allowedScopes:    scopeSet,
		allowedProviders: providerSet,
	}
}

// Register installs every collector into the supplied Registerer.
// Returns the joined error of any individual collector registration
// failure (typically duplicate-collector).
func (m *Metrics) Register(reg prometheus.Registerer) error {
	if reg == nil {
		return errors.New("openknowledge/metrics: Register: nil registerer")
	}
	collectors := []prometheus.Collector{
		m.toolCalls,
		m.iterations,
		m.tokens,
		m.usdCents,
		m.toolLatency,
		m.budgetExhausted,
		m.fabricatedSource,
		m.refusal,
		m.compactionSignaled,
		m.suspiciousSnippet,
		m.circuitState,
		m.circuitTrips,
	}
	var errs []error
	for _, c := range collectors {
		if err := reg.Register(c); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RecordTurn observes the per-turn histograms (iterations, tokens,
// usd cents). Called exactly once per Run() invocation regardless of
// status (success or refusal).
func (m *Metrics) RecordTurn(iterations int, tokens int, usdCents float64) {
	m.iterations.Observe(float64(iterations))
	m.tokens.Observe(float64(tokens))
	m.usdCents.Observe(usdCents)
}

// IncToolCall increments openknowledge_tool_calls_total. Unknown tool
// or outcome labels are dropped (G021 — no cardinality leak).
func (m *Metrics) IncToolCall(tool, outcome string) {
	if _, ok := m.allowedTools[tool]; !ok {
		return
	}
	if outcome != OutcomeSuccess && outcome != OutcomeError {
		return
	}
	m.toolCalls.WithLabelValues(tool, outcome).Inc()
}

// ObserveToolLatency observes the per-tool latency histogram. Unknown
// tool labels are dropped.
func (m *Metrics) ObserveToolLatency(tool string, seconds float64) {
	if _, ok := m.allowedTools[tool]; !ok {
		return
	}
	m.toolLatency.WithLabelValues(tool).Observe(seconds)
}

// IncBudgetExhausted increments by scope. Unknown scope labels are
// dropped (the five-value closed set is the only acceptable input).
func (m *Metrics) IncBudgetExhausted(scope string) {
	if _, ok := m.allowedScopes[scope]; !ok {
		return
	}
	m.budgetExhausted.WithLabelValues(scope).Inc()
}

// IncFabricatedSource increments the fabricated-source counter. No
// labels — the rate itself is the security signal.
func (m *Metrics) IncFabricatedSource() {
	m.fabricatedSource.Inc()
}

// IncRefusal increments by cause. Unknown causes are dropped (only
// contracts.AllRefusalCauses values are accepted — adversarial G021).
func (m *Metrics) IncRefusal(cause string) {
	if _, ok := m.allowedCauses[cause]; !ok {
		return
	}
	m.refusal.WithLabelValues(cause).Inc()
}

// IncCompactionSignaled increments the compaction-signal counter.
func (m *Metrics) IncCompactionSignaled() {
	m.compactionSignaled.Inc()
}

// IncSuspiciousSnippet increments the suspicious-snippet counter by
// provider. Unknown provider labels are dropped (G021 — no
// cardinality leak via uncontrolled labels). Satisfies the
// openknowledge/web.SuspiciousSnippetRecorder duck-typed interface.
func (m *Metrics) IncSuspiciousSnippet(provider string) {
	if _, ok := m.allowedProviders[provider]; !ok {
		return
	}
	m.suspiciousSnippet.WithLabelValues(provider).Inc()
}

// SetCircuitState updates openknowledge_circuit_state{provider} to
// stateCode. SCOPE-16 — satisfies the
// openknowledge/web.CircuitStateRecorder interface. Unknown provider
// labels are dropped (G021); stateCode is forwarded verbatim because
// the closed/half_open/open enum is owned by the web package.
func (m *Metrics) SetCircuitState(provider string, stateCode int) {
	if _, ok := m.allowedProviders[provider]; !ok {
		return
	}
	m.circuitState.WithLabelValues(provider).Set(float64(stateCode))
}

// IncCircuitTrip increments openknowledge_circuit_trips_total{provider}.
// SCOPE-16 — emitted on Closed→Open and HalfOpen→Open transitions.
// Unknown provider labels are dropped (G021).
func (m *Metrics) IncCircuitTrip(provider string) {
	if _, ok := m.allowedProviders[provider]; !ok {
		return
	}
	m.circuitTrips.WithLabelValues(provider).Inc()
}

// AllowedTools returns a defensive copy of the registered tool name
// set; exposed for tests that pin the closed vocabulary.
func (m *Metrics) AllowedTools() []string {
	out := make([]string, 0, len(m.allowedTools))
	for t := range m.allowedTools {
		out = append(out, t)
	}
	return out
}
