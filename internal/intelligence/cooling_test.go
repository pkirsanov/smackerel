// Spec 021 BUG-021-005 — tests for the LLM-driven relationship-cooling
// evaluator and its pure decision helpers. The evaluator is exercised with a
// scripted bridge runner (no live LLM); the producer's DB query is covered by
// the live-stack integration tier.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// scriptedCoolingRunner is a coolingBridgeRunner that returns a canned
// InvocationResult and records the envelope it received.
type scriptedCoolingRunner struct {
	result   *agent.InvocationResult
	gotEnv   agent.IntentEnvelope
	invoked  bool
	routing  *agent.RoutingDecision
	returnNl bool
}

func (s *scriptedCoolingRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.invoked = true
	s.gotEnv = env
	if s.returnNl {
		return nil, nil
	}
	return s.result, s.routing
}

func okCoolingResult(t *testing.T, decision CoolingDecision) *agent.InvocationResult {
	t.Helper()
	final, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: final}
}

func sampleCandidate() CoolingCandidate {
	return CoolingCandidate{
		PersonID:                 "person-123",
		Name:                     "Alex",
		DaysSinceLastInteraction: 45,
		TotalInteractions:        30,
		RelationshipSpanDays:     210,
		TypicalGapDays:           7,
	}
}

func TestBridgeCoolingEvaluator_ParsesCoolingDecision(t *testing.T) {
	want := CoolingDecision{IsCooling: true, Confidence: 0.86, Rationale: "weekly contact gone silent 6x their cadence"}
	runner := &scriptedCoolingRunner{result: okCoolingResult(t, want)}
	ev := &BridgeCoolingEvaluator{Runner: runner}

	got, err := ev.EvaluateCooling(context.Background(), sampleCandidate())
	if err != nil {
		t.Fatalf("EvaluateCooling: %v", err)
	}
	if got != want {
		t.Errorf("decision = %+v, want %+v", got, want)
	}

	// The evaluator MUST route to the explicit cooling scenario and carry the
	// candidate signals in the structured context.
	if runner.gotEnv.ScenarioID != "relationship_cooling_evaluate" {
		t.Errorf("ScenarioID = %q, want relationship_cooling_evaluate", runner.gotEnv.ScenarioID)
	}
	var sent CoolingCandidate
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not the candidate JSON: %v", err)
	}
	if sent.Name != "Alex" || sent.DaysSinceLastInteraction != 45 || sent.TypicalGapDays != 7 {
		t.Errorf("candidate signals not forwarded: %+v", sent)
	}
	// PersonID is internal-only (json:"-") and MUST NOT leak into the prompt.
	if sent.PersonID != "" {
		t.Errorf("PersonID leaked into the LLM envelope: %q", sent.PersonID)
	}
}

func TestBridgeCoolingEvaluator_NotCooling(t *testing.T) {
	want := CoolingDecision{IsCooling: false, Confidence: 0.9, Rationale: "quarterly contact, silence within normal rhythm"}
	runner := &scriptedCoolingRunner{result: okCoolingResult(t, want)}
	ev := &BridgeCoolingEvaluator{Runner: runner}

	got, err := ev.EvaluateCooling(context.Background(), sampleCandidate())
	if err != nil {
		t.Fatalf("EvaluateCooling: %v", err)
	}
	if got.IsCooling {
		t.Errorf("expected is_cooling=false, got %+v", got)
	}
}

func TestBridgeCoolingEvaluator_ErrorPaths(t *testing.T) {
	cases := []struct {
		name   string
		runner *scriptedCoolingRunner
		nilEv  bool
	}{
		{name: "nil_runner", runner: nil},
		{name: "nil_result", runner: &scriptedCoolingRunner{returnNl: true}},
		{name: "non_ok_outcome", runner: &scriptedCoolingRunner{result: &agent.InvocationResult{Outcome: agent.Outcome("schema-failure")}}},
		{name: "empty_final", runner: &scriptedCoolingRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil}}},
		{name: "bad_json", runner: &scriptedCoolingRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: json.RawMessage(`{not json`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ev *BridgeCoolingEvaluator
			if tc.name == "nil_runner" {
				ev = &BridgeCoolingEvaluator{Runner: nil}
			} else {
				ev = &BridgeCoolingEvaluator{Runner: tc.runner}
			}
			_, err := ev.EvaluateCooling(context.Background(), sampleCandidate())
			if err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

func TestBridgeCoolingEvaluator_NilReceiver(t *testing.T) {
	var ev *BridgeCoolingEvaluator
	_, err := ev.EvaluateCooling(context.Background(), sampleCandidate())
	if !errors.Is(err, ErrCoolingEvaluatorUnavailable) {
		t.Errorf("nil receiver should return ErrCoolingEvaluatorUnavailable, got %v", err)
	}
}

func TestCoolingTypicalGapDays(t *testing.T) {
	cases := []struct {
		span, total int
		want        float64
	}{
		{span: 210, total: 30, want: 210.0 / 29.0}, // weekly-ish cadence
		{span: 300, total: 4, want: 100},           // quarterly cadence
		{span: 0, total: 1, want: 0},               // single interaction → no gap
		{span: 50, total: 0, want: 0},              // defensive: no interactions
	}
	for _, tc := range cases {
		if got := coolingTypicalGapDays(tc.span, tc.total); got != tc.want {
			t.Errorf("coolingTypicalGapDays(%d,%d) = %v, want %v", tc.span, tc.total, got, tc.want)
		}
	}
}

func TestCoolingShouldSurface(t *testing.T) {
	floor := 0.7
	cases := []struct {
		name     string
		decision CoolingDecision
		want     bool
	}{
		{"cooling_above_floor", CoolingDecision{IsCooling: true, Confidence: 0.8}, true},
		{"cooling_at_floor", CoolingDecision{IsCooling: true, Confidence: 0.7}, true},
		{"cooling_below_floor", CoolingDecision{IsCooling: true, Confidence: 0.69}, false},
		{"not_cooling_high_conf", CoolingDecision{IsCooling: false, Confidence: 0.99}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coolingShouldSurface(tc.decision, floor); got != tc.want {
				t.Errorf("coolingShouldSurface(%+v, %v) = %v, want %v", tc.decision, floor, got, tc.want)
			}
		})
	}
}
