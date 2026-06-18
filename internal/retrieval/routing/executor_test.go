// Spec 095 SCOPE-06 — retrieval executor tests.
package routing

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent"
)

// stubStrategy is a minimal RetrievalStrategy for executor wiring tests.
type stubStrategy struct {
	kind   StrategyKind
	answer string
	calls  int
}

func (s *stubStrategy) Kind() StrategyKind { return s.kind }
func (s *stubStrategy) Execute(_ context.Context, req RetrievalRequest) (RetrievalResult, error) {
	s.calls++
	return RetrievalResult{Strategy: s.kind, Answer: s.answer}, nil
}

func newExecutorForTest(t *testing.T, strategies ...RetrievalStrategy) *Executor {
	t.Helper()
	cfg := testRoutingConfig()
	reg, err := NewContractRegistry(cfg)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	ex, err := NewExecutor(NewRouter(cfg, reg), strategies...)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	return ex
}

// TestNewExecutor_RequiresVagueRecall — the safe fallback must always be
// registered or construction fails loud.
func TestNewExecutor_RequiresVagueRecall(t *testing.T) {
	cfg := testRoutingConfig()
	reg, _ := NewContractRegistry(cfg)
	_, err := NewExecutor(NewRouter(cfg, reg), &stubStrategy{kind: StrategyWholeDocument})
	if err == nil {
		t.Fatal("NewExecutor must require a vague_recall strategy")
	}
}

// TestRetrieve_RoutesToSelectedStrategy — a confident aggregate intent dispatches
// to the structured_aggregate overlay.
func TestRetrieve_RoutesToSelectedStrategy(t *testing.T) {
	agg := &stubStrategy{kind: StrategyStructuredAggregate, answer: "AGG"}
	vague := &stubStrategy{kind: StrategyVagueRecall, answer: "VAGUE"}
	ex := newExecutorForTest(t, agg, vague)

	in := intentWith(intent.ActionRetrieve, 0.9, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "subscription",
	})
	res, sel, err := ex.Retrieve(context.Background(), in, RetrievalRequest{})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if res.Answer != "AGG" || sel.Strategy != StrategyStructuredAggregate {
		t.Errorf("got answer=%q strategy=%s, want AGG/structured_aggregate", res.Answer, sel.Strategy)
	}
	if agg.calls != 1 || vague.calls != 0 {
		t.Errorf("aggregate overlay should run once, vague zero; got agg=%d vague=%d", agg.calls, vague.calls)
	}
}

// TestRetrieve_VagueRecallDefault — a vague intent dispatches to the vague_recall
// overlay.
func TestRetrieve_VagueRecallDefault(t *testing.T) {
	vague := &stubStrategy{kind: StrategyVagueRecall, answer: "VAGUE"}
	ex := newExecutorForTest(t, vague)
	in := intentWith(intent.ActionRetrieve, 0.9, map[string]any{"target_type": "subscription"})
	res, sel, err := ex.Retrieve(context.Background(), in, RetrievalRequest{})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if res.Answer != "VAGUE" || sel.Strategy != StrategyVagueRecall {
		t.Errorf("got answer=%q strategy=%s, want VAGUE/vague_recall", res.Answer, sel.Strategy)
	}
}

// TestRetrieve_DegradesWhenOverlayMissing — when the selected specialized
// overlay is not wired, the executor degrades to the always-present vague_recall
// rather than erroring.
func TestRetrieve_DegradesWhenOverlayMissing(t *testing.T) {
	vague := &stubStrategy{kind: StrategyVagueRecall, answer: "VAGUE"}
	ex := newExecutorForTest(t, vague) // no structured_aggregate overlay wired
	in := intentWith(intent.ActionRetrieve, 0.95, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "subscription",
	})
	res, sel, err := ex.Retrieve(context.Background(), in, RetrievalRequest{})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if res.Answer != "VAGUE" || sel.Strategy != StrategyVagueRecall || !sel.FellBack {
		t.Errorf("missing overlay should degrade to vague_recall fallback, got answer=%q strategy=%s fellback=%t",
			res.Answer, sel.Strategy, sel.FellBack)
	}
}
