// Spec 021 BUG-021-010 — tests for the LLM-driven hospitality concern evaluator.
// The evaluator is exercised with a scripted bridge runner (no live LLM); the
// candidate DB queries are covered by the live-stack integration tier.
package digest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

type scriptedHospitalityRunner struct {
	result *agent.InvocationResult
	gotEnv agent.IntentEnvelope
	nilRes bool
}

func (s *scriptedHospitalityRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.gotEnv = env
	if s.nilRes {
		return nil, nil
	}
	return s.result, nil
}

func okHospitalityResult(t *testing.T, d HospitalityDecision) *agent.InvocationResult {
	t.Helper()
	final, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: final}
}

func ratingPtr(v float64) *float64 { return &v }

func sampleGuests() []GuestSignal {
	return []GuestSignal{
		{Ref: 0, Name: "Alice", Email: "alice@example.com", TotalStays: 4, Sentiment: ratingPtr(0.28), TotalSpend: 2400},
		{Ref: 1, Name: "Bob", Email: "bob@example.com", TotalStays: 1, Sentiment: ratingPtr(0.9), TotalSpend: 300},
	}
}

func sampleProperties() []PropertySignal {
	return []PropertySignal{
		{Ref: 0, Name: "Beach House", IssueCount: 6, AvgRating: ratingPtr(3.4)},
	}
}

func TestBridgeHospitalityEvaluator_ParsesBatch(t *testing.T) {
	want := HospitalityDecision{
		GuestAlerts:    []ConcernJudgment{{Ref: 0, AlertType: "low_sentiment", Description: "High-value repeat guest with low recent sentiment."}},
		PropertyAlerts: []ConcernJudgment{{Ref: 0, AlertType: "open_issue_backlog", Description: "Six open issues with a soft average rating."}},
	}
	runner := &scriptedHospitalityRunner{result: okHospitalityResult(t, want)}
	ev := &BridgeHospitalityEvaluator{Runner: runner}

	got, err := ev.EvaluateConcerns(context.Background(), sampleGuests(), sampleProperties())
	if err != nil {
		t.Fatalf("EvaluateConcerns: %v", err)
	}
	if len(got.GuestAlerts) != 1 || got.GuestAlerts[0] != want.GuestAlerts[0] {
		t.Errorf("guest alerts = %+v, want %+v", got.GuestAlerts, want.GuestAlerts)
	}
	if len(got.PropertyAlerts) != 1 || got.PropertyAlerts[0] != want.PropertyAlerts[0] {
		t.Errorf("property alerts = %+v, want %+v", got.PropertyAlerts, want.PropertyAlerts)
	}
	if runner.gotEnv.ScenarioID != "hospitality_concern_evaluate" {
		t.Errorf("ScenarioID = %q, want hospitality_concern_evaluate", runner.gotEnv.ScenarioID)
	}

	// The envelope must carry the public signals (incl. ref) but NOT the
	// internal guest Email (json:"-").
	var sent struct {
		Guests     []map[string]any `json:"guests"`
		Properties []map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not JSON: %v", err)
	}
	if len(sent.Guests) != 2 || sent.Guests[0]["name"] != "Alice" || sent.Guests[0]["ref"] == nil {
		t.Errorf("guest signals not forwarded: %v", sent.Guests)
	}
	for _, leaked := range []string{"Email", "email"} {
		if _, ok := sent.Guests[0][leaked]; ok {
			t.Errorf("internal field %q leaked into the LLM envelope: %v", leaked, sent.Guests[0])
		}
	}
	if len(sent.Properties) != 1 || sent.Properties[0]["name"] != "Beach House" {
		t.Errorf("property signals not forwarded: %v", sent.Properties)
	}
}

func TestBridgeHospitalityEvaluator_EmptyInput(t *testing.T) {
	runner := &scriptedHospitalityRunner{nilRes: true} // would error if invoked
	ev := &BridgeHospitalityEvaluator{Runner: runner}

	got, err := ev.EvaluateConcerns(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("empty input should not error, got %v", err)
	}
	if len(got.GuestAlerts) != 0 || len(got.PropertyAlerts) != 0 {
		t.Errorf("expected empty decision for empty input, got %+v", got)
	}
	if runner.gotEnv.ScenarioID != "" {
		t.Errorf("runner must not be invoked for empty input, got scenario %q", runner.gotEnv.ScenarioID)
	}
}

func TestBridgeHospitalityEvaluator_ErrorPaths(t *testing.T) {
	cases := []struct {
		name            string
		runner          *scriptedHospitalityRunner
		wantUnavailable bool
	}{
		{"nil_result", &scriptedHospitalityRunner{nilRes: true}, true},
		{"non_ok_outcome", &scriptedHospitalityRunner{result: &agent.InvocationResult{Outcome: agent.Outcome("schema-failure")}}, false},
		{"empty_final", &scriptedHospitalityRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil}}, false},
		{"bad_json", &scriptedHospitalityRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: json.RawMessage(`{nope`)}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &BridgeHospitalityEvaluator{Runner: tc.runner}
			_, err := ev.EvaluateConcerns(context.Background(), sampleGuests(), sampleProperties())
			if err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
			if tc.wantUnavailable && !errors.Is(err, agent.ErrJudgmentUnavailable) {
				t.Errorf("%s: want ErrJudgmentUnavailable, got %v", tc.name, err)
			}
		})
	}
}

func TestBridgeHospitalityEvaluator_NilReceiverAndRunner(t *testing.T) {
	var nilEv *BridgeHospitalityEvaluator
	if _, err := nilEv.EvaluateConcerns(context.Background(), sampleGuests(), nil); !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil receiver should return ErrJudgmentUnavailable, got %v", err)
	}
	ev := &BridgeHospitalityEvaluator{Runner: nil}
	if _, err := ev.EvaluateConcerns(context.Background(), sampleGuests(), nil); !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil runner should return ErrJudgmentUnavailable, got %v", err)
	}
}
