package agent

import (
	"context"
	"testing"
)

// Scenario: Similarity routing picks the right scenario (BS-002).
//
// We pin deterministic 2-D unit vectors so cosine similarities are
// trivially computable: input "how much did I spend on groceries
// last week?" has vector {1,0}, expense_question's example has {1,0}
// (cosine=1), recipe_question's example has {0,1} (cosine=0). The
// router must rank expense_question first AND record both candidates
// in the Considered list with their actual scores so the trace shows
// what was rejected.
func TestRouter_Similarity_PicksTopScored(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
		makeScenario("recipe_question", "what can I cook with chicken"),
	}
	input := "how much did I spend on groceries last week?"
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0},
		"what can I cook with chicken":      {0, 1},
		input:                               {1, 0},
	})
	r := newTestRouter(t, defaultRoutingCfg(), scenarios, emb)

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{
		Source:   "telegram",
		RawInput: input,
	})
	if !ok || chosen == nil {
		t.Fatalf("expected ok=true; got decision=%+v", decision)
	}
	if chosen.ID != "expense_question" {
		t.Fatalf("wrong scenario: got %q, want expense_question", chosen.ID)
	}
	if decision.Reason != ReasonSimilarityMatch {
		t.Fatalf("wrong reason: got %q, want %q", decision.Reason, ReasonSimilarityMatch)
	}
	if decision.TopScore < 0.999 {
		t.Fatalf("expected TopScore ~1.0, got %v", decision.TopScore)
	}

	// Considered MUST list both scenarios in descending score order.
	if len(decision.Considered) != 2 {
		t.Fatalf("Considered length: got %d, want 2 (both scenarios)", len(decision.Considered))
	}
	if decision.Considered[0].ScenarioID != "expense_question" {
		t.Fatalf("Considered[0] should be top-ranked expense_question, got %q", decision.Considered[0].ScenarioID)
	}
	if decision.Considered[1].ScenarioID != "recipe_question" {
		t.Fatalf("Considered[1] should be recipe_question, got %q", decision.Considered[1].ScenarioID)
	}
	if decision.Considered[0].Score <= decision.Considered[1].Score {
		t.Fatalf("Considered must be sorted desc by score: %+v", decision.Considered)
	}
}

// Multiple intent_examples per scenario: max(cosine) wins. This catches
// a regression where the router averages instead of taking the max,
// which would dilute scenarios whose examples cover diverse phrasings.
func TestRouter_Similarity_MaxOverExamples(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question",
			"how much did I spend on groceries", // close to input
			"weekly food cost summary",          // distant from input
		),
		makeScenario("recipe_question", "what can I cook with chicken"),
	}
	input := "how much did I spend on groceries last week?"
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0},
		"weekly food cost summary":          {0, 1}, // intentionally orthogonal
		"what can I cook with chicken":      {0, 1},
		input:                               {1, 0},
	})
	r := newTestRouter(t, defaultRoutingCfg(), scenarios, emb)

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{RawInput: input})
	if !ok || chosen == nil || chosen.ID != "expense_question" {
		t.Fatalf("expected expense_question via max-over-examples; got %+v", decision)
	}
	// Find expense_question in Considered and assert it carries the
	// MAX score (1.0), not an average (~0.5).
	var found bool
	for _, c := range decision.Considered {
		if c.ScenarioID == "expense_question" {
			found = true
			if c.Score < 0.999 {
				t.Fatalf("expense_question score %v indicates averaging; must be max-over-examples (~1.0)", c.Score)
			}
		}
	}
	if !found {
		t.Fatalf("expense_question missing from Considered: %+v", decision.Considered)
	}
}

// Embedder is invoked exactly once for the input on the similarity
// path — not once per scenario, not once per example. Example vectors
// are precomputed at NewRouter time.
func TestRouter_Similarity_OneEmbedCallPerRoute(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("a", "ex_a"),
		makeScenario("b", "ex_b"),
		makeScenario("c", "ex_c"),
	}
	emb := newRecordingEmbedder(map[string][]float32{
		"ex_a":  {1, 0, 0},
		"ex_b":  {0, 1, 0},
		"ex_c":  {0, 0, 1},
		"input": {1, 0, 0},
	})
	r := newTestRouter(t, defaultRoutingCfg(), scenarios, emb)
	before := emb.Calls()

	_, _, _ = r.Route(context.Background(), IntentEnvelope{RawInput: "input"})
	if got := emb.Calls() - before; got != 1 {
		t.Fatalf("similarity route should call Embed exactly once; got %d", got)
	}
}
