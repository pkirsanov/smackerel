// Spec 064 SCOPE-14 — open-knowledge metrics unit tests.
package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestOpenKnowledgeMetrics_NamesPinned asserts the exact metric names
// the spec 049 dashboard / alert proposals will scrape. Renaming any
// of these is a coordinated cross-spec change.
func TestOpenKnowledgeMetrics_NamesPinned(t *testing.T) {
	want := map[string]string{
		"toolCalls":          "openknowledge_tool_calls_total",
		"iterations":         "openknowledge_iterations_per_query",
		"tokens":             "openknowledge_tokens_per_query",
		"usdCents":           "openknowledge_usd_cents_per_query",
		"toolLatency":        "openknowledge_tool_latency_seconds",
		"budgetExhausted":    "openknowledge_budget_exhausted_total",
		"fabricatedSource":   "openknowledge_fabricated_source_total",
		"refusal":            "openknowledge_refusal_total",
		"compactionSignaled": "openknowledge_compaction_signaled_total",
	}
	got := map[string]string{
		"toolCalls":          NameToolCalls,
		"iterations":         NameIterations,
		"tokens":             NameTokens,
		"usdCents":           NameUSDCents,
		"toolLatency":        NameToolLatency,
		"budgetExhausted":    NameBudgetExhausted,
		"fabricatedSource":   NameFabricatedSource,
		"refusal":            NameRefusal,
		"compactionSignaled": NameCompactionSignaled,
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("metric %s: got %q want %q", k, got[k], w)
		}
	}
}

func TestOpenKnowledgeMetrics_RefusalFamilyRegisteredBeforeFirstEvent(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() == NameRefusal {
			if got, want := len(family.Metric), len(contracts.AllRefusalCauses); got != want {
				t.Fatalf("refusal zero-series count=%d, want closed vocabulary size %d", got, want)
			}
			return
		}
	}
	t.Fatalf("fresh registry is missing %s", NameRefusal)
}

// TestOpenKnowledgeMetrics_RegisterAndScrape constructs Metrics
// against a fresh registry, drives every helper, and reads the
// values to prove the series materialise with the expected counts.
func TestOpenKnowledgeMetrics_RegisterAndScrape(t *testing.T) {
	m := New([]string{"calculator", "web_search"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	m.RecordTurn(3, 1200, 7.5)
	m.IncToolCall("calculator", OutcomeSuccess)
	m.IncToolCall("web_search", OutcomeError)
	m.ObserveToolLatency("web_search", 0.42)
	m.IncBudgetExhausted(BudgetScopeTokens)
	m.IncBudgetExhausted(BudgetScopeMonthly)
	m.IncFabricatedSource()
	m.IncFabricatedSource()
	m.IncRefusal(string(contracts.RefusalFabricatedSourceBlocked))
	m.IncRefusal(string(contracts.RefusalBudgetExhausted))
	m.IncCompactionSignaled()

	if got := testutil.ToFloat64(m.fabricatedSource); got != 2 {
		t.Errorf("fabricated_source counter = %v want 2", got)
	}
	if got := testutil.ToFloat64(m.compactionSignaled); got != 1 {
		t.Errorf("compaction_signaled counter = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.toolCalls.WithLabelValues("calculator", OutcomeSuccess)); got != 1 {
		t.Errorf("tool_calls{calculator,success} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.toolCalls.WithLabelValues("web_search", OutcomeError)); got != 1 {
		t.Errorf("tool_calls{web_search,error} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.budgetExhausted.WithLabelValues(BudgetScopeTokens)); got != 1 {
		t.Errorf("budget_exhausted{tokens} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.budgetExhausted.WithLabelValues(BudgetScopeMonthly)); got != 1 {
		t.Errorf("budget_exhausted{monthly} = %v want 1", got)
	}
	if got := testutil.ToFloat64(m.refusal.WithLabelValues(string(contracts.RefusalFabricatedSourceBlocked))); got != 1 {
		t.Errorf("refusal{fabricated_source_blocked} = %v want 1", got)
	}
}

// TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021 proves
// that an LLM (or buggy mapping) cannot inflate Prometheus
// cardinality by passing a fabricated cause / tool / scope label.
// The increments MUST be silently dropped — no panic, no new series.
func TestOpenKnowledgeMetrics_RejectsUnknownCause_AdversarialG021(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Baseline: one legitimate increment.
	m.IncRefusal(string(contracts.RefusalBudgetExhausted))

	// Adversarial: thousands of attempts at unbounded labels.
	for i := 0; i < 1000; i++ {
		m.IncRefusal("definitely_not_a_real_cause")
		m.IncRefusal("'; DROP TABLE refusals; --")
		m.IncRefusal("\x00\x01\x02")
		m.IncToolCall("not_a_tool", OutcomeSuccess)
		m.IncToolCall("calculator", "weird_outcome")
		m.IncBudgetExhausted("rogue_scope")
		m.ObserveToolLatency("not_a_tool", 1.0)
	}

	// Cardinality bounds: refusal vec retains exactly the preinitialized
	// closed vocabulary; adversarial labels add no new series.
	if got, want := testutil.CollectAndCount(m.refusal), len(contracts.AllRefusalCauses); got != want {
		t.Errorf("refusal series count = %d want %d (adversarial labels leaked)", got, want)
	}
	if got, want := testutil.CollectAndCount(m.toolCalls), 0; got != want {
		t.Errorf("toolCalls series count = %d want %d (adversarial labels leaked)", got, want)
	}
	if got, want := testutil.CollectAndCount(m.budgetExhausted), 0; got != want {
		t.Errorf("budgetExhausted series count = %d want %d (adversarial labels leaked)", got, want)
	}
	if got, want := testutil.CollectAndCount(m.toolLatency), 0; got != want {
		t.Errorf("toolLatency series count = %d want %d (adversarial labels leaked)", got, want)
	}
	if got := testutil.ToFloat64(m.refusal.WithLabelValues(string(contracts.RefusalBudgetExhausted))); got != 1 {
		t.Errorf("legitimate refusal{budget_exhausted} = %v want 1", got)
	}
}

// TestOpenKnowledgeMetrics_RegisterRejectsNil — Register(nil) returns
// a typed error rather than panicking.
func TestOpenKnowledgeMetrics_RegisterRejectsNil(t *testing.T) {
	m := New([]string{"calculator"})
	if err := m.Register(nil); err == nil {
		t.Fatalf("Register(nil) returned nil error")
	} else if !strings.Contains(err.Error(), "nil registerer") {
		t.Errorf("Register(nil) err = %q want 'nil registerer'", err.Error())
	}
}

// TestOpenKnowledgeMetrics_DuplicateRegisterFails — registering the
// same Metrics into a registry twice fails.
func TestOpenKnowledgeMetrics_DuplicateRegisterFails(t *testing.T) {
	m := New([]string{"calculator"})
	reg := prometheus.NewRegistry()
	if err := m.Register(reg); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := m.Register(reg); err == nil {
		t.Fatalf("second Register returned nil; expected duplicate-collector error")
	}
}

// TestOpenKnowledgeMetrics_NopSatisfiesRecorder — compile-time sanity.
func TestOpenKnowledgeMetrics_NopSatisfiesRecorder(t *testing.T) {
	var r Recorder = Nop{}
	r.RecordTurn(1, 100, 0.5)
	r.IncToolCall("anything", "anything")
	r.ObserveToolLatency("anything", 0)
	r.IncBudgetExhausted("anything")
	r.IncFabricatedSource()
	r.IncRefusal("anything")
	r.IncCompactionSignaled()
}

// TestOpenKnowledgeMetrics_LiveSatisfiesRecorder — compile-time sanity.
func TestOpenKnowledgeMetrics_LiveSatisfiesRecorder(t *testing.T) {
	var r Recorder = New([]string{"calculator"})
	_ = r
}

// TestOpenKnowledgeMetrics_AllowedToolsRoundTrips — defensive copy
// returns the same set the constructor received.
func TestOpenKnowledgeMetrics_AllowedToolsRoundTrips(t *testing.T) {
	in := []string{"calculator", "web_search", "internal_retrieval", "unit_convert"}
	m := New(in)
	got := m.AllowedTools()
	if len(got) != len(in) {
		t.Fatalf("AllowedTools len=%d want %d", len(got), len(in))
	}
	set := make(map[string]struct{}, len(got))
	for _, tn := range got {
		set[tn] = struct{}{}
	}
	for _, want := range in {
		if _, ok := set[want]; !ok {
			t.Errorf("AllowedTools missing %q", want)
		}
	}
}

// TestOpenKnowledgeMetrics_AllRefusalCausesAccepted proves every value
// in contracts.AllRefusalCauses is acceptable.
func TestOpenKnowledgeMetrics_AllRefusalCausesAccepted(t *testing.T) {
	m := New([]string{"calculator"})
	for _, c := range contracts.AllRefusalCauses {
		m.IncRefusal(string(c))
	}
	if got, want := testutil.CollectAndCount(m.refusal), len(contracts.AllRefusalCauses); got != want {
		t.Errorf("refusal series count = %d want %d", got, want)
	}
}

// TestOpenKnowledgeMetrics_AllBudgetScopesAccepted proves every value
// in AllBudgetScopes is acceptable.
func TestOpenKnowledgeMetrics_AllBudgetScopesAccepted(t *testing.T) {
	m := New([]string{"calculator"})
	for _, s := range AllBudgetScopes {
		m.IncBudgetExhausted(s)
	}
	if got, want := testutil.CollectAndCount(m.budgetExhausted), len(AllBudgetScopes); got != want {
		t.Errorf("budgetExhausted series count = %d want %d", got, want)
	}
}
