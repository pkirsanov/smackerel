package agent

import (
	"context"
	"testing"
)

// Adversarial regression for BS-014: silent top-pick despite threshold.
//
// THE BUG THIS TEST PREVENTS:
// A router that ranks candidates by similarity but forgets to enforce
// the ConfidenceFloor — i.e. one that returns the top-scored scenario
// regardless of whether its score actually clears the threshold.
//
// HOW THIS TEST WOULD FAIL IF THE BUG WERE REINTRODUCED:
// The fixture is constructed so EVERY candidate scores well below the
// floor (max ~0.31 < floor 0.65) AND there is a clear "top" candidate
// (expense_question at ~0.31 > recipe_question at ~0.10). A buggy
// router that picks the top would return chosen=expense_question with
// reason=similarity_match. This test asserts:
//
//  1. ok == false (no chosen scenario returned at all),
//  2. decision.Reason == "unknown_intent" exactly,
//  3. decision.Chosen == "" (no scenario name leaked into the trace),
//  4. decision.TopScore < decision.Threshold (the rejection is real),
//  5. decision.Considered enumerates BOTH candidates with their actual
//     scores so an operator can see what was rejected and by how much,
//  6. decision.Threshold equals the configured floor (so the trace
//     records the rule that fired, not just the score).
//
// NO BAILOUT IS PERMITTED. The test does not skip on missing fixtures
// or short-circuit if a scenario is "close to" the floor — every check
// is a hard assertion. If the router silently top-picks, every
// assertion above fires together, making the failure mode unmistakable
// in the test report.
func TestRouter_BS014_BelowFloor_NeverSilentlyTopPicks(t *testing.T) {
	scenarios := []*Scenario{
		makeScenario("expense_question", "how much did I spend on groceries"),
		makeScenario("recipe_question", "what can I cook with chicken"),
	}
	input := "weather forecast for tokyo this weekend"
	// Vectors chosen so that expense_question scores higher than
	// recipe_question (creating a clear "top" the buggy router could
	// silently pick) but BOTH are well below the 0.65 floor.
	//   cos(input, expense_example) ~= 0.31
	//   cos(input, recipe_example)  ~= 0.10
	emb := newRecordingEmbedder(map[string][]float32{
		"how much did I spend on groceries": {1, 0, 0, 0},
		"what can I cook with chicken":      {0, 1, 0, 0},
		// Input has a small-but-nonzero projection on each example
		// axis; designed so neither cosine clears the floor.
		input: {0.31, 0.10, 0.95, 0},
	})
	cfg := defaultRoutingCfg() // floor=0.65, no fallback
	r := newTestRouter(t, cfg, scenarios, emb)

	chosen, decision, ok := r.Route(context.Background(), IntentEnvelope{
		Source:   "telegram",
		RawInput: input,
	})

	// (1) ok must be false — the router must NOT report success.
	if ok {
		t.Fatalf("BS-014 regression: router returned ok=true despite top score below floor; chosen=%v decision=%+v", chosen, decision)
	}
	// (2) Reason must be exactly unknown_intent.
	if decision.Reason != ReasonUnknownIntent {
		t.Fatalf("BS-014 regression: reason %q must be %q (router silently picked despite floor)", decision.Reason, ReasonUnknownIntent)
	}
	// (3) No scenario name may leak into the chosen field.
	if decision.Chosen != "" {
		t.Fatalf("BS-014 regression: decision.Chosen must be empty on unknown-intent; got %q (router exposed a top pick that did not pass the floor)", decision.Chosen)
	}
	if chosen != nil {
		t.Fatalf("BS-014 regression: chosen *Scenario must be nil; got %+v", chosen)
	}
	// (4) The top score must be strictly below the threshold for this
	// fixture to make sense as a regression. If this assertion ever
	// fails, the FIXTURE is broken (not the router); fix the vectors
	// so the top score genuinely does not clear the floor.
	if decision.TopScore >= decision.Threshold {
		t.Fatalf("BS-014 fixture invalid: top score %v must be < threshold %v for the regression to be meaningful", decision.TopScore, decision.Threshold)
	}
	// (5) Considered must enumerate BOTH candidates with their scores,
	// in descending order, so the trace lets an operator audit the
	// rejection.
	if len(decision.Considered) != 2 {
		t.Fatalf("BS-014 regression: Considered must list both rejected candidates; got %+v", decision.Considered)
	}
	if decision.Considered[0].ScenarioID != "expense_question" {
		t.Fatalf("BS-014 regression: Considered[0] must be the (rejected) top candidate expense_question; got %q score=%v", decision.Considered[0].ScenarioID, decision.Considered[0].Score)
	}
	if decision.Considered[1].ScenarioID != "recipe_question" {
		t.Fatalf("BS-014 regression: Considered[1] must be recipe_question; got %q", decision.Considered[1].ScenarioID)
	}
	if decision.Considered[0].Score <= decision.Considered[1].Score {
		t.Fatalf("BS-014 regression: Considered must be sorted by descending score; got %+v", decision.Considered)
	}
	// (6) Threshold must equal the configured floor for this run so
	// the trace records the rule that fired.
	if decision.Threshold != cfg.ConfidenceFloor {
		t.Fatalf("BS-014 regression: decision.Threshold must equal configured ConfidenceFloor; got %v want %v", decision.Threshold, cfg.ConfidenceFloor)
	}
}
