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
	"github.com/smackerel/smackerel/internal/config"
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

// Spec 096 §13 — HOSTED provider-dispatch label values for
// openknowledge_provider_dispatch_total{provider}. The closed set mirrors the
// non-ollama subset of config.AllModelConnectionKinds: a bare/ollama dispatch
// stays the spec 089 path and emits NO provider metric. Unknown providers drop
// silently at the increment site (G021 — no cardinality leak).
var AllDispatchProviders = []string{
	config.ModelConnectionKindAnthropic,
	config.ModelConnectionKindOpenAI,
	config.ModelConnectionKindAzureFoundry,
	config.ModelConnectionKindGoogle,
	config.ModelConnectionKindBedrock,
}

// Spec 096 §13 — typed vault/credential failure reasons for
// openknowledge_vault_decrypt_failures_total{reason}. These mirror the
// credential/vault-class subset of llm.RejectReason (the cases where the
// SecretVault credential path is implicated). A reason token NEVER carries
// secret material; unknown/non-vault reasons drop silently (G021).
const (
	VaultDecryptReasonCredentialMissing  = "credential_missing"
	VaultDecryptReasonVaultNotConfigured = "vault_not_configured"
	VaultDecryptReasonDecryptFailed      = "decrypt_failed"
)

// AllVaultDecryptReasons is the exhaustive list used by cardinality tests.
var AllVaultDecryptReasons = []string{
	VaultDecryptReasonCredentialMissing,
	VaultDecryptReasonVaultNotConfigured,
	VaultDecryptReasonDecryptFailed,
}

// Spec 096 §13 — provider DISCOVERY provider vocab for
// openknowledge_provider_discovery_*{provider}. It is the dispatch set PLUS
// "ollama": Ollama IS discovered (its live /api/tags probe is a first-class
// discovery provider), unlike dispatch where a bare/ollama turn is the spec 089
// path and emits NO provider metric. Unknown providers drop at the increment
// site (G021 — no cardinality leak).
var AllDiscoveryProviders = append([]string{config.ModelConnectionKindOllama}, AllDispatchProviders...)

// Spec 096 §13 — provider DISCOVERY reachability states for
// openknowledge_provider_discovery_total{state}. The closed set mirrors the
// catalog build() outcomes (catalog.StateOK / StateTimeout / StateUnreachable),
// duplicated as plain literals so this package does NOT import catalog (which
// imports THIS package for its Recorder — a back-import would form a cycle). A
// discovery probe NEVER touches a credential, so a state token carries no secret.
const (
	DiscoveryStateOK          = "ok"
	DiscoveryStateTimeout     = "timeout"
	DiscoveryStateUnreachable = "unreachable"
)

// AllDiscoveryStates is the exhaustive list used by cardinality tests.
var AllDiscoveryStates = []string{
	DiscoveryStateOK,
	DiscoveryStateTimeout,
	DiscoveryStateUnreachable,
}

// Spec 096 §13 — operator connection-TEST kind vocab for
// openknowledge_model_connection_test_total{kind,outcome}. Unlike dispatch
// (where a bare/ollama turn emits NO provider metric), the operator test surface
// probes EVERY declared db-mode connection kind, so the full closed
// connection-kind set (ollama + the hosted providers) is admissible. A kind
// outside this set drops at the increment site (G021 — no cardinality leak).
var AllConnectionTestKinds = []string{
	config.ModelConnectionKindOllama,
	config.ModelConnectionKindAnthropic,
	config.ModelConnectionKindOpenAI,
	config.ModelConnectionKindAzureFoundry,
	config.ModelConnectionKindGoogle,
	config.ModelConnectionKindBedrock,
}

// Spec 096 §13 — operator connection-TEST outcome vocab for
// openknowledge_model_connection_test_total{outcome}. Mirrors the connstore
// TestOutcome tokens (ok|failed), duplicated as plain literals so this package
// does NOT import connstore. The probe's truthful typed Detail
// (auth_failed|unreachable|timeout) is DELIBERATELY excluded — it can carry
// endpoint specifics, so ONLY the bounded ok|failed outcome ever becomes a label.
const (
	ConnectionTestOutcomeOK     = "ok"
	ConnectionTestOutcomeFailed = "failed"
)

// AllConnectionTestOutcomes is the exhaustive list used by cardinality tests.
var AllConnectionTestOutcomes = []string{ConnectionTestOutcomeOK, ConnectionTestOutcomeFailed}

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

// DiscoveryLatencyBuckets covers the per-provider discovery probe latency: a
// fast in-memory curated-list build (single-digit ms) through a live Ollama
// /api/tags probe bounded by the SST per_provider_timeout_ms (seconds). Same
// second-denominated ladder shape as ToolLatencyBuckets.
var DiscoveryLatencyBuckets = []float64{0.005, 0.025, 0.1, 0.5, 1, 2, 5, 10, 30}

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
	// NameProviderDispatch — spec 096 §13: counter incremented once per HOSTED
	// provider dispatch in the /ask loop (ollama/bare dispatches are NOT counted
	// — they remain the spec 089 path). Bounded `provider` label.
	NameProviderDispatch = "openknowledge_provider_dispatch_total"
	// NameProviderDispatchTokens / NameProviderDispatchUSDCents — spec 096 §13:
	// per-HOSTED-dispatch token + USD-cents histograms, labelled by provider,
	// observed AFTER the Chat result is priced.
	NameProviderDispatchTokens   = "openknowledge_provider_dispatch_tokens"
	NameProviderDispatchUSDCents = "openknowledge_provider_dispatch_usd_cents"
	// NameVaultDecryptFailures — spec 096 §13: counter of provider-credential
	// vault decrypt / credential-resolution failures by typed reason. NEVER
	// carries secret material.
	NameVaultDecryptFailures = "openknowledge_vault_decrypt_failures_total"
	// NameProviderDiscovery — spec 096 §13: per-provider discovery reachability
	// counter, labelled by provider kind + closed state (ok|timeout|unreachable).
	// Ollama IS a discovery provider (its /api/tags probe), unlike dispatch.
	NameProviderDiscovery = "openknowledge_provider_discovery_total"
	// NameProviderDiscoveryLatency — spec 096 §13: per-provider discovery latency
	// histogram (seconds), labelled by provider kind. Discovery touches NO
	// credential, so neither this metric nor its labels can carry a secret.
	NameProviderDiscoveryLatency = "openknowledge_provider_discovery_latency_seconds"
	// NameConnectionTest — spec 096 §13: counter of operator connection-test
	// outcomes by connection kind + closed outcome (ok|failed), emitted by the
	// /v1/admin/model-connections/{id}/test handler AFTER the truthful probe. The
	// probe decrypts a credential, but NEITHER label can carry a secret: the kind
	// is the closed connection-kind vocab and the outcome is the closed ok|failed
	// set (the typed failure Detail is DELIBERATELY excluded).
	NameConnectionTest = "openknowledge_model_connection_test_total"
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
	// Spec 096 §13 — per-HOSTED-provider (cost-bearing) dispatch observability.
	// IncProviderDispatch counts one hosted dispatch; ObserveProviderDispatch
	// records that dispatch's tokens + USD cents AFTER the Chat result is priced;
	// IncVaultDecryptFailure counts a typed, secret-free credential/vault failure.
	IncProviderDispatch(provider string)
	ObserveProviderDispatch(provider string, tokens int, usdCents float64)
	IncVaultDecryptFailure(reason string)
	// Spec 096 §13 — per-provider DISCOVERY observability (catalog aggregator).
	// IncProviderDiscovery counts one discovery outcome by provider + closed
	// reachability state (ok|timeout|unreachable); ObserveProviderDiscoveryLatency
	// records that probe's latency. Discovery touches NO credential, so neither a
	// label nor any caller value can carry a secret.
	IncProviderDiscovery(provider, state string)
	ObserveProviderDiscoveryLatency(provider string, seconds float64)
	// Spec 096 §13 — operator connection-TEST observability (admin surface). One
	// IncConnectionTest per /v1/admin/model-connections/{id}/test, labelled by the
	// closed connection kind + the closed ok|failed outcome. The test handler
	// decrypts a credential to probe, but NEITHER label can carry a secret (the
	// kind is closed and the outcome is ok|failed — the typed failure Detail is
	// excluded).
	IncConnectionTest(kind, outcome string)
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

// IncProviderDispatch — no-op.
func (Nop) IncProviderDispatch(string) {}

// ObserveProviderDispatch — no-op.
func (Nop) ObserveProviderDispatch(string, int, float64) {}

// IncVaultDecryptFailure — no-op.
func (Nop) IncVaultDecryptFailure(string) {}

// IncProviderDiscovery — no-op.
func (Nop) IncProviderDiscovery(string, string) {}

// ObserveProviderDiscoveryLatency — no-op.
func (Nop) ObserveProviderDiscoveryLatency(string, float64) {}

// IncConnectionTest — no-op.
func (Nop) IncConnectionTest(string, string) {}

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
	// Spec 096 §13 — provider-aware (cost-bearing) dispatch observability.
	providerDispatch       *prometheus.CounterVec
	providerDispatchTokens *prometheus.HistogramVec
	providerDispatchUSD    *prometheus.HistogramVec
	vaultDecryptFailures   *prometheus.CounterVec
	// Spec 096 §13 — provider DISCOVERY observability (catalog aggregator).
	providerDiscovery        *prometheus.CounterVec
	providerDiscoveryLatency *prometheus.HistogramVec
	// Spec 096 §13 — operator connection-TEST observability (admin surface).
	connectionTest *prometheus.CounterVec

	allowedTools     map[string]struct{}
	allowedCauses    map[string]struct{}
	allowedScopes    map[string]struct{}
	allowedProviders map[string]struct{}
	// Spec 096 §13 — closed vocab for the dispatch-provider + vault-reason labels.
	allowedDispatchProviders map[string]struct{}
	allowedVaultReasons      map[string]struct{}
	// Spec 096 §13 — closed vocab for the discovery provider + state labels.
	allowedDiscoveryProviders map[string]struct{}
	allowedDiscoveryStates    map[string]struct{}
	// Spec 096 §13 — closed vocab for the connection-test kind + outcome labels.
	allowedConnectionTestKinds    map[string]struct{}
	allowedConnectionTestOutcomes map[string]struct{}
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
	dispatchProviderSet := make(map[string]struct{}, len(AllDispatchProviders))
	for _, p := range AllDispatchProviders {
		dispatchProviderSet[p] = struct{}{}
	}
	vaultReasonSet := make(map[string]struct{}, len(AllVaultDecryptReasons))
	for _, r := range AllVaultDecryptReasons {
		vaultReasonSet[r] = struct{}{}
	}
	discoveryProviderSet := make(map[string]struct{}, len(AllDiscoveryProviders))
	for _, p := range AllDiscoveryProviders {
		discoveryProviderSet[p] = struct{}{}
	}
	discoveryStateSet := make(map[string]struct{}, len(AllDiscoveryStates))
	for _, s := range AllDiscoveryStates {
		discoveryStateSet[s] = struct{}{}
	}
	connectionTestKindSet := make(map[string]struct{}, len(AllConnectionTestKinds))
	for _, k := range AllConnectionTestKinds {
		connectionTestKindSet[k] = struct{}{}
	}
	connectionTestOutcomeSet := make(map[string]struct{}, len(AllConnectionTestOutcomes))
	for _, o := range AllConnectionTestOutcomes {
		connectionTestOutcomeSet[o] = struct{}{}
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
		providerDispatch: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameProviderDispatch,
			Help: "Open-knowledge HOSTED provider dispatches by provider kind (spec 096 §13; ollama/bare dispatches are the spec 089 path and are NOT counted).",
		}, []string{"provider"}),
		providerDispatchTokens: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    NameProviderDispatchTokens,
			Help:    "Open-knowledge tokens per HOSTED provider dispatch, by provider (spec 096 §13).",
			Buckets: TokenBuckets,
		}, []string{"provider"}),
		providerDispatchUSD: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    NameProviderDispatchUSDCents,
			Help:    "Open-knowledge USD spend (cents) per HOSTED provider dispatch, by provider (spec 096 §13).",
			Buckets: USDCentsBuckets,
		}, []string{"provider"}),
		vaultDecryptFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameVaultDecryptFailures,
			Help: "Open-knowledge provider-credential vault decrypt/credential-resolution failures by typed reason (spec 096 §13). NEVER carries secret material.",
		}, []string{"reason"}),
		providerDiscovery: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameProviderDiscovery,
			Help: "Open-knowledge per-provider discovery outcomes by provider kind + reachability state (ok|timeout|unreachable) (spec 096 §13). Ollama IS a discovery provider.",
		}, []string{"provider", "state"}),
		providerDiscoveryLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    NameProviderDiscoveryLatency,
			Help:    "Open-knowledge per-provider discovery probe latency in seconds, by provider kind (spec 096 §13). NEVER carries secret material.",
			Buckets: DiscoveryLatencyBuckets,
		}, []string{"provider"}),
		connectionTest: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameConnectionTest,
			Help: "Open-knowledge operator connection-test outcomes by connection kind + outcome (ok|failed) (spec 096 §13). NEVER carries secret material.",
		}, []string{"kind", "outcome"}),
		allowedTools:             toolSet,
		allowedCauses:            causeSet,
		allowedScopes:            scopeSet,
		allowedProviders:         providerSet,
		allowedDispatchProviders: dispatchProviderSet,
		allowedVaultReasons:      vaultReasonSet,
		// Spec 096 §13 — discovery provider + state closed vocab.
		allowedDiscoveryProviders: discoveryProviderSet,
		allowedDiscoveryStates:    discoveryStateSet,
		// Spec 096 §13 — connection-test kind + outcome closed vocab.
		allowedConnectionTestKinds:    connectionTestKindSet,
		allowedConnectionTestOutcomes: connectionTestOutcomeSet,
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
		m.providerDispatch,
		m.providerDispatchTokens,
		m.providerDispatchUSD,
		m.vaultDecryptFailures,
		m.providerDiscovery,
		m.providerDiscoveryLatency,
		m.connectionTest,
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

// IncProviderDispatch increments openknowledge_provider_dispatch_total{provider}
// once per HOSTED provider dispatch (spec 096 §13). Unknown providers are
// dropped (G021); an ollama/bare dispatch never reaches here — it is the spec
// 089 path.
func (m *Metrics) IncProviderDispatch(provider string) {
	if _, ok := m.allowedDispatchProviders[provider]; !ok {
		return
	}
	m.providerDispatch.WithLabelValues(provider).Inc()
}

// ObserveProviderDispatch observes the per-HOSTED-dispatch token + USD-cents
// histograms for provider (spec 096 §13), recorded AFTER the Chat result is
// priced. Unknown providers are dropped (G021).
func (m *Metrics) ObserveProviderDispatch(provider string, tokens int, usdCents float64) {
	if _, ok := m.allowedDispatchProviders[provider]; !ok {
		return
	}
	m.providerDispatchTokens.WithLabelValues(provider).Observe(float64(tokens))
	m.providerDispatchUSD.WithLabelValues(provider).Observe(usdCents)
}

// IncVaultDecryptFailure increments
// openknowledge_vault_decrypt_failures_total{reason} (spec 096 §13). reason is
// the TYPED, secret-free failure token; unknown/non-vault reasons are dropped
// (G021 — no cardinality leak, and no secret material can become a label).
func (m *Metrics) IncVaultDecryptFailure(reason string) {
	if _, ok := m.allowedVaultReasons[reason]; !ok {
		return
	}
	m.vaultDecryptFailures.WithLabelValues(reason).Inc()
}

// IncProviderDiscovery increments
// openknowledge_provider_discovery_total{provider,state} once per discovery
// outcome (spec 096 §13). provider is the closed discovery-provider kind (the
// dispatch set PLUS ollama); state is the closed reachability token
// (ok|timeout|unreachable). An unknown provider OR an unknown state drops the
// increment (G021 — no cardinality leak). Discovery touches NO credential, so
// no label can ever carry a secret.
func (m *Metrics) IncProviderDiscovery(provider, state string) {
	if _, ok := m.allowedDiscoveryProviders[provider]; !ok {
		return
	}
	if _, ok := m.allowedDiscoveryStates[state]; !ok {
		return
	}
	m.providerDiscovery.WithLabelValues(provider, state).Inc()
}

// ObserveProviderDiscoveryLatency observes the per-provider discovery latency
// histogram in seconds (spec 096 §13). Unknown providers drop the observation
// (G021 — no cardinality leak).
func (m *Metrics) ObserveProviderDiscoveryLatency(provider string, seconds float64) {
	if _, ok := m.allowedDiscoveryProviders[provider]; !ok {
		return
	}
	m.providerDiscoveryLatency.WithLabelValues(provider).Observe(seconds)
}

// IncConnectionTest increments
// openknowledge_model_connection_test_total{kind,outcome} once per operator
// connection-test (spec 096 §13). kind is the closed connection-kind vocab;
// outcome is the closed ok|failed set. An unknown kind OR an unknown outcome
// drops the increment (G021 — no cardinality leak). The /v1/admin test handler
// decrypts a credential to probe, but NEITHER label can carry a secret — the
// typed failure Detail is DELIBERATELY excluded from this surface.
func (m *Metrics) IncConnectionTest(kind, outcome string) {
	if _, ok := m.allowedConnectionTestKinds[kind]; !ok {
		return
	}
	if _, ok := m.allowedConnectionTestOutcomes[outcome]; !ok {
		return
	}
	m.connectionTest.WithLabelValues(kind, outcome).Inc()
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
