package agent

import (
	"context"
	"testing"
)

// Below-floor input with NO fallback configured returns unknown-intent.
// The router MUST NOT pick the top-scored scenario when its score is
// below the configured floor — that would defeat the entire purpose of
// the threshold and reintroduce silent misrouting (BS-014).
func TestRouter_BelowFloor_NoFallback_UnknownIntent(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
		makeScenario("recipe_question", "what can I cook with chicken"),
	}
	// Input is orthogonal-ish to both examples; both cosines ~0.
	input := "what's the weather in Tokyo"
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0, 0},
		"what can I cook with chicken":      {0, 1, 0},
		input:                               {0, 0, 1},
	})
	cfg := defaultRoutingCfg() // ConfidenceFloor=0.65, no fallback
	r := newTestRouter(t, cfg, scenarios, emb)

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{
		Source:   "telegram",
		RawInput: input,
	})
	if ok {
		t.Fatalf("expected ok=false when top score < floor and no fallback; got chosen=%v decision=%+v", chosen, decision)
	}
	if chosen != nil {
		t.Fatalf("chosen MUST be nil on unknown-intent; got %+v", chosen)
	}
	if decision.Reason != ReasonUnknownIntent {
		t.Fatalf("wrong reason: got %q, want %q", decision.Reason, ReasonUnknownIntent)
	}
	if decision.Threshold != cfg.ConfidenceFloor {
		t.Fatalf("decision.Threshold MUST record the effective floor for the trace; got %v want %v", decision.Threshold, cfg.ConfidenceFloor)
	}
	if decision.TopScore >= cfg.ConfidenceFloor {
		t.Fatalf("top score %v should be below floor %v for this fixture", decision.TopScore, cfg.ConfidenceFloor)
	}
	// Considered MUST still be populated so the trace shows what was rejected.
	if len(decision.Considered) == 0 {
		t.Fatalf("Considered MUST be populated even on unknown-intent so the trace is auditable")
	}
}

// Per-envelope ConfidenceFloor override applies (e.g., a stricter
// surface). Setting it ABOVE the cfg default and watching a previously
// matching input now fall through to unknown-intent proves the override
// is actually consulted.
func TestRouter_EnvelopeFloorOverride(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
	}
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0},
		"input":                             {0.7, 0.7142}, // cosine ~0.7 with example
	})
	cfg := defaultRoutingCfg() // floor=0.65 → would match
	r := newTestRouter(t, cfg, scenarios, emb)

	// At default floor 0.65, the input matches.
	_, decision, ok := r.Route(context.Background(), IntentEnvelope{RawInput: "input"})
	if !ok || decision.Reason != ReasonSimilarityMatch {
		t.Fatalf("baseline at floor 0.65 should match: %+v", decision)
	}

	// At envelope floor 0.95, the input must NOT match.
	_, decision, ok = r.Route(context.Background(), IntentEnvelope{RawInput: "input", ConfidenceFloor: 0.95})
	if ok {
		t.Fatalf("envelope override floor 0.95 should reject; got %+v", decision)
	}
	if decision.Threshold != 0.95 {
		t.Fatalf("decision.Threshold MUST reflect envelope override; got %v", decision.Threshold)
	}
}
