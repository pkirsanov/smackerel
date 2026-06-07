// Spec 021 BUG-021-007 — tests for the LLM-driven resurfacing-worthiness
// evaluator and its pure decision helper. The evaluator is exercised with a
// scripted bridge runner (no live LLM); the dormancy DB query is covered by the
// live-stack integration tier.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

type scriptedResurfaceRunner struct {
	result   *agent.InvocationResult
	gotEnv   agent.IntentEnvelope
	returnNl bool
}

func (s *scriptedResurfaceRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.gotEnv = env
	if s.returnNl {
		return nil, nil
	}
	return s.result, nil
}

func okResurfaceResult(t *testing.T, decision ResurfaceDecision) *agent.InvocationResult {
	t.Helper()
	final, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: final}
}

func sampleResurfaceSignals() ResurfaceSignals {
	return ResurfaceSignals{
		ArtifactID:  "art-9",
		Title:       "Deep dive on vector indexing",
		DaysDormant: 95,
		Relevance:   0.82,
		AccessCount: 7,
	}
}

func TestBridgeResurfaceEvaluator_ParsesDecision(t *testing.T) {
	want := ResurfaceDecision{WorthResurfacing: true, Confidence: 0.88, Reason: "You returned to this reference often before it went quiet."}
	runner := &scriptedResurfaceRunner{result: okResurfaceResult(t, want)}
	ev := &BridgeResurfaceEvaluator{Runner: runner}

	got, err := ev.EvaluateResurface(context.Background(), sampleResurfaceSignals())
	if err != nil {
		t.Fatalf("EvaluateResurface: %v", err)
	}
	if got != want {
		t.Errorf("decision = %+v, want %+v", got, want)
	}
	if runner.gotEnv.ScenarioID != "resurface_evaluate" {
		t.Errorf("ScenarioID = %q, want resurface_evaluate", runner.gotEnv.ScenarioID)
	}

	// The structured context must carry the public signals but NOT the internal
	// ArtifactID (json:"-").
	var sent map[string]any
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not JSON: %v", err)
	}
	if sent["title"] != "Deep dive on vector indexing" || sent["access_count"] == nil {
		t.Errorf("public signals not forwarded: %v", sent)
	}
	for _, leaked := range []string{"ArtifactID", "artifact_id"} {
		if _, ok := sent[leaked]; ok {
			t.Errorf("internal field %q leaked into the LLM envelope: %v", leaked, sent)
		}
	}
}

func TestBridgeResurfaceEvaluator_NotWorth(t *testing.T) {
	want := ResurfaceDecision{WorthResurfacing: false, Confidence: 0.9, Reason: "Never really engaged with; resurfacing would be noise."}
	runner := &scriptedResurfaceRunner{result: okResurfaceResult(t, want)}
	ev := &BridgeResurfaceEvaluator{Runner: runner}

	got, err := ev.EvaluateResurface(context.Background(), sampleResurfaceSignals())
	if err != nil {
		t.Fatalf("EvaluateResurface: %v", err)
	}
	if got.WorthResurfacing {
		t.Errorf("expected worth_resurfacing=false, got %+v", got)
	}
}

func TestBridgeResurfaceEvaluator_ErrorPaths(t *testing.T) {
	cases := []struct {
		name   string
		runner *scriptedResurfaceRunner
	}{
		{name: "nil_result", runner: &scriptedResurfaceRunner{returnNl: true}},
		{name: "non_ok_outcome", runner: &scriptedResurfaceRunner{result: &agent.InvocationResult{Outcome: agent.Outcome("schema-failure")}}},
		{name: "empty_final", runner: &scriptedResurfaceRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil}}},
		{name: "bad_json", runner: &scriptedResurfaceRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: json.RawMessage(`{nope`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &BridgeResurfaceEvaluator{Runner: tc.runner}
			if _, err := ev.EvaluateResurface(context.Background(), sampleResurfaceSignals()); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

func TestBridgeResurfaceEvaluator_NilReceiverAndRunner(t *testing.T) {
	var nilEv *BridgeResurfaceEvaluator
	if _, err := nilEv.EvaluateResurface(context.Background(), sampleResurfaceSignals()); !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil receiver should return agent.ErrJudgmentUnavailable, got %v", err)
	}
	ev := &BridgeResurfaceEvaluator{Runner: nil}
	if _, err := ev.EvaluateResurface(context.Background(), sampleResurfaceSignals()); !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil runner should return agent.ErrJudgmentUnavailable, got %v", err)
	}
}

func TestResurfaceShouldSurface(t *testing.T) {
	floor := 0.7
	cases := []struct {
		name     string
		decision ResurfaceDecision
		want     bool
	}{
		{"worth_above_floor", ResurfaceDecision{WorthResurfacing: true, Confidence: 0.8}, true},
		{"worth_at_floor", ResurfaceDecision{WorthResurfacing: true, Confidence: 0.7}, true},
		{"worth_below_floor", ResurfaceDecision{WorthResurfacing: true, Confidence: 0.5}, false},
		{"not_worth_high_conf", ResurfaceDecision{WorthResurfacing: false, Confidence: 0.99}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resurfaceShouldSurface(tc.decision, floor); got != tc.want {
				t.Errorf("resurfaceShouldSurface(%+v, %v) = %v, want %v", tc.decision, floor, got, tc.want)
			}
		})
	}
}
