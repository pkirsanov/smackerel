//go:build stress

// Spec 095 SCOPE-06 — routing-overhead stress test (NFR-1).
//
// The retrieval-strategy router reuses the ALREADY-COMPUTED CompiledIntent and
// adds NO second LLM round-trip to route (NFR-1). This in-process burst proves
// the added decision overhead is a negligible fraction of the reactive p95
// budget (< 5s, matching spec 061/062/063 §14.G). Because the router is pure
// (no I/O, no store, no model call), the test needs NO live stack and runs on
// any tier — it is the cpu-tier-runnable proof of the NFR-1 latency contract.
//
// Asserts:
//   - G1: every Select returns a valid strategy (closed vocabulary).
//   - G2: per-call p95 routing overhead is far below the 5s reactive budget
//     (a regression toward a per-call model round-trip or lock contention
//     would blow far past the asserted micro-budget).
//   - G3: p50/p95/p99/max are logged for operator drift visibility.
package stress

import (
	"sort"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/intent"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

const (
	routingStressIterations = 200000
	// The router is a pure decision; its overhead must stay a tiny fraction
	// of the 5s reactive budget. 5ms p95 is already ~1000x headroom and trips
	// loudly on any accidental per-call I/O or model round-trip (NFR-1).
	routingStressP95Budget = 5 * time.Millisecond
	routingReactiveBudget  = 5 * time.Second // §14.G reactive ceiling (for context)
)

func routingStressConfig() config.RetrievalRoutingConfig {
	return config.RetrievalRoutingConfig{
		Enabled:                    true,
		IntentConfidenceThreshold:  0.65,
		WholeDocumentEnabled:       true,
		StructuredAggregateEnabled: true,
		VagueRecallEnabled:         true,
		Contracts: map[string][]string{
			"transcript":   {"whole_document_summary", "vague_recall"},
			"subscription": {"aggregate_spend", "vague_recall"},
			"place":        {"dossier", "vague_recall"},
		},
	}
}

func TestRetrievalRoutingOverheadStressP95(t *testing.T) {
	cfg := routingStressConfig()
	reg, err := routing.NewContractRegistry(cfg)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	router := routing.NewRouter(cfg, reg)

	// A representative mix of the four routing windows.
	intents := []intent.CompiledIntent{
		{ActionClass: intent.ActionRetrieve, Confidence: 0.9, Slots: map[string]any{"retrieval_shape": "aggregate_spend", "target_type": "subscription"}},
		{ActionClass: intent.ActionRetrieve, Confidence: 0.9, Slots: map[string]any{"retrieval_shape": "whole_document_summary", "target_type": "transcript"}},
		{ActionClass: intent.ActionRetrieve, Confidence: 0.5, Slots: map[string]any{"retrieval_shape": "aggregate_spend", "target_type": "subscription"}},
		{ActionClass: intent.ActionRetrieve, Confidence: 0.9, Slots: map[string]any{"target_type": "subscription"}},
	}

	latencies := make([]time.Duration, 0, routingStressIterations)
	for i := 0; i < routingStressIterations; i++ {
		in := intents[i%len(intents)]
		start := time.Now()
		sel := router.Route(in)
		latencies = append(latencies, time.Since(start))
		if !routing.IsValidStrategyKind(sel.Strategy) {
			t.Fatalf("iteration %d: router returned invalid strategy %q", i, sel.Strategy)
		}
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p := func(q float64) time.Duration {
		idx := int(q * float64(len(latencies)))
		if idx >= len(latencies) {
			idx = len(latencies) - 1
		}
		return latencies[idx]
	}
	p50, p95, p99, max := p(0.50), p(0.95), p(0.99), latencies[len(latencies)-1]
	t.Logf("routing overhead over %d iterations: p50=%s p95=%s p99=%s max=%s (reactive budget %s)",
		routingStressIterations, p50, p95, p99, max, routingReactiveBudget)

	if p95 > routingStressP95Budget {
		t.Fatalf("routing p95 overhead %s exceeds the %s micro-budget — a per-call round-trip or contention regressed NFR-1", p95, routingStressP95Budget)
	}
}
