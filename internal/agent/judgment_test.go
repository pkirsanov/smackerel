// Spec 021 BUG-021-010 — tests for the reusable LLM-judgment primitive.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type scriptedJudgmentRunner struct {
	result *InvocationResult
	gotEnv IntentEnvelope
	nilRes bool
}

func (s *scriptedJudgmentRunner) Invoke(_ context.Context, env IntentEnvelope) (*InvocationResult, *RoutingDecision) {
	s.gotEnv = env
	if s.nilRes {
		return nil, nil
	}
	return s.result, nil
}

type sampleDecision struct {
	Verdict string  `json:"verdict"`
	Score   float64 `json:"score"`
}

type sampleSignals struct {
	Subject  string `json:"subject"`
	Count    int    `json:"count"`
	Internal string `json:"-"`
}

func okJudgmentResult(t *testing.T, d sampleDecision) *InvocationResult {
	t.Helper()
	final, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return &InvocationResult{Outcome: OutcomeOK, Final: final}
}

func TestInvokeJudgment_ParsesRoutesAndForwardsSignals(t *testing.T) {
	want := sampleDecision{Verdict: "surface", Score: 0.91}
	runner := &scriptedJudgmentRunner{result: okJudgmentResult(t, want)}

	got, err := InvokeJudgment[sampleDecision](context.Background(), runner, "scheduler", "demo_scenario", sampleSignals{Subject: "x", Count: 3, Internal: "secret"})
	if err != nil {
		t.Fatalf("InvokeJudgment: %v", err)
	}
	if got != want {
		t.Errorf("decision = %+v, want %+v", got, want)
	}
	if runner.gotEnv.ScenarioID != "demo_scenario" {
		t.Errorf("ScenarioID = %q, want demo_scenario", runner.gotEnv.ScenarioID)
	}
	if runner.gotEnv.Source != "scheduler" {
		t.Errorf("Source = %q, want scheduler", runner.gotEnv.Source)
	}

	var sent map[string]any
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not JSON: %v", err)
	}
	if sent["subject"] != "x" || sent["count"] == nil {
		t.Errorf("public signals not forwarded: %v", sent)
	}
	if _, leaked := sent["Internal"]; leaked {
		t.Errorf("json:\"-\" field leaked into the envelope: %v", sent)
	}
}

func TestInvokeJudgment_NilRunner(t *testing.T) {
	_, err := InvokeJudgment[sampleDecision](context.Background(), nil, "scheduler", "demo_scenario", sampleSignals{})
	if !errors.Is(err, ErrJudgmentUnavailable) {
		t.Errorf("nil runner should return ErrJudgmentUnavailable, got %v", err)
	}
}

func TestInvokeJudgment_ErrorPaths(t *testing.T) {
	cases := []struct {
		name            string
		runner          *scriptedJudgmentRunner
		wantUnavailable bool
	}{
		{"nil_result", &scriptedJudgmentRunner{nilRes: true}, true},
		{"non_ok_outcome", &scriptedJudgmentRunner{result: &InvocationResult{Outcome: Outcome("schema-failure")}}, false},
		{"empty_final", &scriptedJudgmentRunner{result: &InvocationResult{Outcome: OutcomeOK, Final: nil}}, false},
		{"bad_json", &scriptedJudgmentRunner{result: &InvocationResult{Outcome: OutcomeOK, Final: json.RawMessage(`{nope`)}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := InvokeJudgment[sampleDecision](context.Background(), tc.runner, "scheduler", "demo_scenario", sampleSignals{})
			if err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
			if tc.wantUnavailable && !errors.Is(err, ErrJudgmentUnavailable) {
				t.Errorf("%s: want ErrJudgmentUnavailable, got %v", tc.name, err)
			}
		})
	}
}
