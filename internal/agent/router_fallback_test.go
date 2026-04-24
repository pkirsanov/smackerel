package agent

import (
	"context"
	"testing"
)

// Below-floor input WITH fallback configured returns the fallback
// scenario and reason "fallback_clarify" (per design §4.1). The
// fallback scenario is the operator's escape hatch for asking a
// clarifying question rather than failing the user with unknown-intent.
func TestRouter_BelowFloor_WithFallback_ReturnsFallback(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
		makeScenario("recipe_question", "what can I cook with chicken"),
		makeScenario("clarify_intent"), // fallback has NO intent_examples on purpose
	}
	input := "what's the weather in Tokyo"
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0, 0},
		"what can I cook with chicken":      {0, 1, 0},
		input:                               {0, 0, 1},
	})
	cfg := defaultRoutingCfg()
	cfg.FallbackScenarioID = "clarify_intent"
	r := newTestRouter(t, cfg, scenarios, emb)

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{RawInput: input})
	if !ok {
		t.Fatalf("expected ok=true on fallback; got %+v", decision)
	}
	if chosen == nil || chosen.ID != "clarify_intent" {
		t.Fatalf("wrong scenario chosen: got %v, want clarify_intent", chosen)
	}
	if decision.Reason != ReasonFallbackClarify {
		t.Fatalf("wrong reason: got %q, want %q", decision.Reason, ReasonFallbackClarify)
	}
	if decision.Chosen != "clarify_intent" {
		t.Fatalf("decision.Chosen mismatch: got %q", decision.Chosen)
	}
	if len(decision.Considered) == 0 {
		t.Fatalf("Considered MUST be populated on fallback so the trace shows the rejected candidates")
	}
}

// Fallback scenario id configured but NOT registered → unknown-intent.
// The router MUST NOT silently degrade by, say, picking the second-best
// similarity match. Misconfiguration must be visible in the trace.
func TestRouter_FallbackMisconfigured_UnknownIntent(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
	}
	input := "weather in Tokyo"
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0},
		input:                               {0, 1}, // cosine ~0
	})
	cfg := defaultRoutingCfg()
	cfg.FallbackScenarioID = "scenario_does_not_exist"
	r := newTestRouter(t, cfg, scenarios, emb)

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{RawInput: input})
	if ok {
		t.Fatalf("misconfigured fallback id MUST yield unknown-intent; got chosen=%v", chosen)
	}
	if decision.Reason != ReasonUnknownIntent {
		t.Fatalf("wrong reason: got %q, want %q", decision.Reason, ReasonUnknownIntent)
	}
}
