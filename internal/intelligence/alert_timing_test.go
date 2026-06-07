// Spec 021 BUG-021-006 — tests for the LLM-driven alert-timing evaluator and
// its pure decision helper. The evaluator is exercised with a scripted bridge
// runner (no live LLM); the producers' DB queries are covered by the
// live-stack integration tier.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

type scriptedTimingRunner struct {
	result   *agent.InvocationResult
	gotEnv   agent.IntentEnvelope
	returnNl bool
}

func (s *scriptedTimingRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.gotEnv = env
	if s.returnNl {
		return nil, nil
	}
	return s.result, nil
}

func okTimingResult(t *testing.T, decision AlertTimingDecision) *agent.InvocationResult {
	t.Helper()
	final, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: final}
}

func sampleTimingCandidate() AlertTimingCandidate {
	return AlertTimingCandidate{
		ArtifactID:     "sub-123",
		AlertType:      AlertBill,
		Priority:       2,
		AlertKind:      AlertKindBill,
		Subject:        "Streaming Plus",
		DaysUntilEvent: 6,
		Detail:         "annual USD 120.00 subscription",
	}
}

func TestBridgeAlertTimingEvaluator_ParsesDecision(t *testing.T) {
	want := AlertTimingDecision{ShouldAlert: true, Confidence: 0.84, Rationale: "large annual charge in 6 days warrants notice"}
	runner := &scriptedTimingRunner{result: okTimingResult(t, want)}
	ev := &BridgeAlertTimingEvaluator{Runner: runner}

	got, err := ev.EvaluateAlertTiming(context.Background(), sampleTimingCandidate())
	if err != nil {
		t.Fatalf("EvaluateAlertTiming: %v", err)
	}
	if got != want {
		t.Errorf("decision = %+v, want %+v", got, want)
	}
	if runner.gotEnv.ScenarioID != "alert_timing_evaluate" {
		t.Errorf("ScenarioID = %q, want alert_timing_evaluate", runner.gotEnv.ScenarioID)
	}

	// The structured context must carry the public signals but NOT the internal
	// ArtifactID / AlertType / Priority (json:"-").
	var sent map[string]any
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not JSON: %v", err)
	}
	if sent["alert_kind"] != "bill" || sent["subject"] != "Streaming Plus" {
		t.Errorf("public signals not forwarded: %v", sent)
	}
	for _, leaked := range []string{"ArtifactID", "artifact_id", "AlertType", "Priority"} {
		if _, ok := sent[leaked]; ok {
			t.Errorf("internal field %q leaked into the LLM envelope: %v", leaked, sent)
		}
	}
}

func TestBridgeAlertTimingEvaluator_ErrorPaths(t *testing.T) {
	cases := []struct {
		name   string
		runner *scriptedTimingRunner
	}{
		{name: "nil_result", runner: &scriptedTimingRunner{returnNl: true}},
		{name: "non_ok_outcome", runner: &scriptedTimingRunner{result: &agent.InvocationResult{Outcome: agent.Outcome("schema-failure")}}},
		{name: "empty_final", runner: &scriptedTimingRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil}}},
		{name: "bad_json", runner: &scriptedTimingRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: json.RawMessage(`{nope`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &BridgeAlertTimingEvaluator{Runner: tc.runner}
			if _, err := ev.EvaluateAlertTiming(context.Background(), sampleTimingCandidate()); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

func TestBridgeAlertTimingEvaluator_NilReceiverAndRunner(t *testing.T) {
	var nilEv *BridgeAlertTimingEvaluator
	if _, err := nilEv.EvaluateAlertTiming(context.Background(), sampleTimingCandidate()); !errors.Is(err, ErrAlertTimingEvaluatorUnavailable) {
		t.Errorf("nil receiver should return ErrAlertTimingEvaluatorUnavailable, got %v", err)
	}
	ev := &BridgeAlertTimingEvaluator{Runner: nil}
	if _, err := ev.EvaluateAlertTiming(context.Background(), sampleTimingCandidate()); !errors.Is(err, ErrAlertTimingEvaluatorUnavailable) {
		t.Errorf("nil runner should return ErrAlertTimingEvaluatorUnavailable, got %v", err)
	}
}

func TestAlertTimingShouldSurface(t *testing.T) {
	floor := 0.7
	cases := []struct {
		name     string
		decision AlertTimingDecision
		want     bool
	}{
		{"alert_above_floor", AlertTimingDecision{ShouldAlert: true, Confidence: 0.8}, true},
		{"alert_at_floor", AlertTimingDecision{ShouldAlert: true, Confidence: 0.7}, true},
		{"alert_below_floor", AlertTimingDecision{ShouldAlert: true, Confidence: 0.5}, false},
		{"no_alert_high_conf", AlertTimingDecision{ShouldAlert: false, Confidence: 0.99}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := alertTimingShouldSurface(tc.decision, floor); got != tc.want {
				t.Errorf("alertTimingShouldSurface(%+v, %v) = %v, want %v", tc.decision, floor, got, tc.want)
			}
		})
	}
}
