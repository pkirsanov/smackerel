package agent

import (
	"context"
	"testing"
)

// Scenario: Explicit scenario id bypasses similarity (BS-002 fast path).
//
// The router MUST NOT call the embedder when the envelope names a
// scenario id. We assert that by counting Embed calls on the recording
// embedder — even though the embedder also has scenario-example
// vectors precomputed at NewRouter time, the Route call itself should
// add zero further calls.
func TestRouter_ExplicitScenarioID_ShortCircuits(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
		makeScenario("recipe_question", "what can I cook with chicken"),
	}
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0},
		"what can I cook with chicken":      {0, 1},
	})
	r := newTestRouter(t, defaultRoutingCfg(), scenarios, emb)

	// Reset call counter AFTER NewRouter so we measure Route() only.
	callsBefore := emb.Calls()

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{
		Source:     "telegram",
		RawInput:   "anything at all — even a contradictory query",
		ScenarioID: "expense_question",
	})
	if !ok {
		t.Fatalf("expected ok=true on explicit id route, got decision=%+v", decision)
	}
	if chosen == nil || chosen.ID != "expense_question" {
		t.Fatalf("wrong scenario chosen: got %v, want expense_question", chosen)
	}
	if decision.Reason != ReasonExplicitScenarioID {
		t.Fatalf("wrong reason: got %q, want %q", decision.Reason, ReasonExplicitScenarioID)
	}
	if decision.Chosen != "expense_question" {
		t.Fatalf("decision.Chosen mismatch: got %q", decision.Chosen)
	}
	if got := emb.Calls() - callsBefore; got != 0 {
		t.Fatalf("explicit-id route MUST NOT call embedder; got %d Embed call(s)", got)
	}
	if len(decision.Considered) != 0 {
		t.Fatalf("explicit-id route MUST NOT populate Considered (no scoring ran); got %+v", decision.Considered)
	}
}

// Explicit id that is not registered → unknown-intent (no embedder call).
func TestRouter_ExplicitScenarioID_Unknown(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
	}
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0},
	})
	r := newTestRouter(t, defaultRoutingCfg(), scenarios, emb)
	callsBefore := emb.Calls()

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{
		Source:     "api",
		RawInput:   "irrelevant",
		ScenarioID: "no_such_scenario",
	})
	if ok {
		t.Fatalf("expected ok=false for unknown explicit id; got chosen=%v", chosen)
	}
	if decision.Reason != ReasonUnknownIntent {
		t.Fatalf("wrong reason: got %q, want %q", decision.Reason, ReasonUnknownIntent)
	}
	if got := emb.Calls() - callsBefore; got != 0 {
		t.Fatalf("unknown explicit id route MUST NOT call embedder; got %d", got)
	}
}
