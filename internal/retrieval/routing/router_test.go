// Spec 095 SCOPE-03 — RetrievalStrategyRouter behavioural tests.
package routing

import (
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/intent"
)

func newTestRouter(t *testing.T) *Router {
	t.Helper()
	cfg := testRoutingConfig()
	reg, err := NewContractRegistry(cfg)
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	return NewRouter(cfg, reg)
}

// intentWith builds a CompiledIntent with the given action class, confidence,
// and slots.
func intentWith(class intent.ActionClass, confidence float64, slots map[string]any) intent.CompiledIntent {
	return intent.CompiledIntent{
		ActionClass: class,
		Confidence:  confidence,
		Slots:       slots,
	}
}

// TestSelectEmitsTracedSelection — SCN-095-A01: a confident shaped intent
// selects exactly one strategy and the selection carries the intent class,
// confidence, and matched contract type (Principle 8).
func TestSelectEmitsTracedSelection(t *testing.T) {
	r := newTestRouter(t)
	reg, _ := NewContractRegistry(testRoutingConfig())
	contract := reg.ContractFor("subscription")
	in := intentWith(intent.ActionRetrieve, 0.9, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "subscription",
	})
	sel := r.Select(in, contract)
	if sel.Strategy != StrategyStructuredAggregate {
		t.Errorf("strategy = %s, want structured_aggregate", sel.Strategy)
	}
	if sel.Reason != ReasonIntentMatch {
		t.Errorf("reason = %s, want intent_match", sel.Reason)
	}
	if sel.FellBack {
		t.Error("should not be a fallback for a confident admitted shape")
	}
	if sel.IntentClass != string(intent.ActionRetrieve) {
		t.Errorf("intent class = %q, want retrieve", sel.IntentClass)
	}
	if sel.Confidence != 0.9 {
		t.Errorf("confidence = %g, want 0.9", sel.Confidence)
	}
	if sel.ArtifactType != "subscription" {
		t.Errorf("artifact type = %q, want subscription", sel.ArtifactType)
	}
	if sel.TraceToken() != "strategy_selected" {
		t.Errorf("trace token = %q, want strategy_selected", sel.TraceToken())
	}
}

// TestSelectWholeDocument — a confident whole-document intent against a
// transcript contract selects the whole_document strategy.
func TestSelectWholeDocument(t *testing.T) {
	r := newTestRouter(t)
	reg, _ := NewContractRegistry(testRoutingConfig())
	in := intentWith(intent.ActionRetrieve, 0.8, map[string]any{
		"retrieval_shape": "whole_document_summary",
		"target_type":     "transcript",
	})
	sel := r.Select(in, reg.ContractFor("transcript"))
	if sel.Strategy != StrategyWholeDocument || sel.Reason != ReasonIntentMatch {
		t.Errorf("got strategy=%s reason=%s, want whole_document/intent_match", sel.Strategy, sel.Reason)
	}
}

// TestContractGatesStrategy — SCN-095-C02: an aggregate-intent query against a
// type whose contract does NOT admit aggregate_spend resolves to vague_recall
// with the resolution reason recorded.
func TestContractGatesStrategy(t *testing.T) {
	r := newTestRouter(t)
	reg, _ := NewContractRegistry(testRoutingConfig())
	// transcript admits [whole_document_summary, vague_recall] — NOT aggregate_spend.
	in := intentWith(intent.ActionRetrieve, 0.95, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "transcript",
	})
	sel := r.Select(in, reg.ContractFor("transcript"))
	if sel.Strategy != StrategyVagueRecall {
		t.Errorf("strategy = %s, want vague_recall (contract gating)", sel.Strategy)
	}
	if sel.Reason != ReasonShapeNotAdmitted {
		t.Errorf("reason = %s, want shape_not_admitted_by_contract", sel.Reason)
	}
	if !sel.FellBack {
		t.Error("FellBack should be true when the contract gates the desired strategy")
	}
}

// TestLowConfidenceFallback — SCN-095-A05 boundary: just-below the threshold
// falls back to vague_recall; just-above routes to the specialized strategy
// (no tautology — both sides asserted).
func TestLowConfidenceFallback(t *testing.T) {
	r := newTestRouter(t) // threshold 0.65
	reg, _ := NewContractRegistry(testRoutingConfig())
	contract := reg.ContractFor("subscription")
	mk := func(conf float64) intent.CompiledIntent {
		return intentWith(intent.ActionRetrieve, conf, map[string]any{
			"retrieval_shape": "aggregate_spend",
			"target_type":     "subscription",
		})
	}
	below := r.Select(mk(0.64), contract)
	if below.Strategy != StrategyVagueRecall || below.Reason != ReasonLowConfidence || !below.FellBack {
		t.Errorf("just-below threshold: got strategy=%s reason=%s fellback=%t, want vague_recall/low_confidence_fallback/true",
			below.Strategy, below.Reason, below.FellBack)
	}
	above := r.Select(mk(0.66), contract)
	if above.Strategy != StrategyStructuredAggregate || above.Reason != ReasonIntentMatch {
		t.Errorf("just-above threshold: got strategy=%s reason=%s, want structured_aggregate/intent_match",
			above.Strategy, above.Reason)
	}
	// Exactly at the threshold is admitted (>= threshold).
	at := r.Select(mk(0.65), contract)
	if at.Strategy != StrategyStructuredAggregate {
		t.Errorf("at threshold: got %s, want structured_aggregate (>= is admitted)", at.Strategy)
	}
}

// TestRoute_UnknownTypeFailsSafe — SCN-095-C03 routing: an unknown queried
// type routes to vague_recall with ContractKnown=false (observable) and the
// missing-contract reason.
func TestRoute_UnknownTypeFailsSafe(t *testing.T) {
	r := newTestRouter(t)
	in := intentWith(intent.ActionRetrieve, 0.95, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "totally_unknown_type",
	})
	sel := r.Route(in)
	if sel.Strategy != StrategyVagueRecall {
		t.Errorf("strategy = %s, want vague_recall", sel.Strategy)
	}
	if sel.ContractKnown {
		t.Error("ContractKnown should be false for an unregistered type (observable missing contract)")
	}
	if sel.Reason != ReasonMissingContract {
		t.Errorf("reason = %s, want missing_contract_fallback", sel.Reason)
	}
}

// TestDefaultVagueRecall — a vague content-recall intent keeps vague_recall and
// is NOT a fallback.
func TestDefaultVagueRecall(t *testing.T) {
	r := newTestRouter(t)
	reg, _ := NewContractRegistry(testRoutingConfig())
	in := intentWith(intent.ActionRetrieve, 0.9, map[string]any{"target_type": "subscription"})
	sel := r.Select(in, reg.ContractFor("subscription"))
	if sel.Strategy != StrategyVagueRecall || sel.Reason != ReasonDefaultVagueRecall || sel.FellBack {
		t.Errorf("got strategy=%s reason=%s fellback=%t, want vague_recall/default_vague_recall/false",
			sel.Strategy, sel.Reason, sel.FellBack)
	}
}

// TestDossierResolvesVagueRecall — dossier has no v1 overlay; it resolves to
// vague_recall with the no_specialized_strategy reason.
func TestDossierResolvesVagueRecall(t *testing.T) {
	r := newTestRouter(t)
	reg, _ := NewContractRegistry(testRoutingConfig())
	in := intentWith(intent.ActionRetrieve, 0.9, map[string]any{
		"retrieval_shape": "dossier",
		"target_type":     "place",
	})
	sel := r.Select(in, reg.ContractFor("place"))
	if sel.Strategy != StrategyVagueRecall || sel.Reason != ReasonNoSpecializedStrategy {
		t.Errorf("got strategy=%s reason=%s, want vague_recall/no_specialized_strategy", sel.Strategy, sel.Reason)
	}
}

// TestStrategyDisabledFallback — a desired strategy disabled in SST falls back
// to vague_recall with the strategy_disabled reason.
func TestStrategyDisabledFallback(t *testing.T) {
	cfg := testRoutingConfig()
	cfg.StructuredAggregateEnabled = false
	reg, _ := NewContractRegistry(cfg)
	r := NewRouter(cfg, reg)
	in := intentWith(intent.ActionRetrieve, 0.95, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "subscription",
	})
	sel := r.Select(in, reg.ContractFor("subscription"))
	if sel.Strategy != StrategyVagueRecall || sel.Reason != ReasonStrategyDisabled || !sel.FellBack {
		t.Errorf("got strategy=%s reason=%s fellback=%t, want vague_recall/strategy_disabled_fallback/true",
			sel.Strategy, sel.Reason, sel.FellBack)
	}
}

// TestRoutingDisabled — routing.enabled=false keeps everything on vague_recall.
func TestRoutingDisabled(t *testing.T) {
	cfg := testRoutingConfig()
	cfg.Enabled = false
	reg, _ := NewContractRegistry(cfg)
	r := NewRouter(cfg, reg)
	in := intentWith(intent.ActionRetrieve, 0.99, map[string]any{
		"retrieval_shape": "aggregate_spend",
		"target_type":     "subscription",
	})
	sel := r.Select(in, reg.ContractFor("subscription"))
	if sel.Strategy != StrategyVagueRecall || sel.Reason != ReasonRoutingDisabled {
		t.Errorf("got strategy=%s reason=%s, want vague_recall/routing_disabled", sel.Strategy, sel.Reason)
	}
}

// TestDesiredShape_BooleanMarkers — DesiredShape derives the shape from
// boolean markers when no explicit retrieval_shape slot is present, and never
// calls out (deterministic; NFR-1).
func TestDesiredShape_BooleanMarkers(t *testing.T) {
	if got := DesiredShape(intentWith(intent.ActionRetrieve, 0.9, map[string]any{"aggregate": true})); got != ShapeAggregateSpend {
		t.Errorf("aggregate marker → %s, want aggregate_spend", got)
	}
	if got := DesiredShape(intentWith(intent.ActionRetrieve, 0.9, map[string]any{"whole_document": "true"})); got != ShapeWholeDocumentSummary {
		t.Errorf("whole_document marker → %s, want whole_document_summary", got)
	}
	if got := DesiredShape(intentWith(intent.ActionRetrieve, 0.9, nil)); got != ShapeVagueRecall {
		t.Errorf("no markers → %s, want vague_recall (safe default)", got)
	}
}
