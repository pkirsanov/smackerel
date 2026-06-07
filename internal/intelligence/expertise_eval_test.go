// Spec 021 BUG-021-008 — tests for the LLM-driven expertise classifier. The
// evaluator is exercised with a scripted bridge runner (no live LLM); the
// dormant DB query is covered by the live-stack integration tier.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

type scriptedExpertiseRunner struct {
	result   *agent.InvocationResult
	gotEnv   agent.IntentEnvelope
	returnNl bool
}

func (s *scriptedExpertiseRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.gotEnv = env
	if s.returnNl {
		return nil, nil
	}
	return s.result, nil
}

func okExpertiseResult(t *testing.T, classifications []ExpertiseClassification) *agent.InvocationResult {
	t.Helper()
	final, err := json.Marshal(expertiseResponse{Classifications: classifications})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: final}
}

func sampleExpertiseSignals() []ExpertiseSignals {
	return []ExpertiseSignals{
		{TopicID: "topic-a", Ref: 0, TopicName: "Vector databases", CaptureCount: 40, SourceDiversity: 6, DepthRatio: 0.7, Engagement: 55, ConnectionDensity: 2.1, RecentCaptures: 12, AvgMonthly: 6},
		{TopicID: "topic-b", Ref: 1, TopicName: "Sourdough", CaptureCount: 3, SourceDiversity: 1, DepthRatio: 0.0, Engagement: 1, ConnectionDensity: 0, RecentCaptures: 0, AvgMonthly: 0.5},
	}
}

func TestBridgeExpertiseEvaluator_ParsesBatch(t *testing.T) {
	want := []ExpertiseClassification{
		{Ref: 0, Tier: "deep", Growth: "accelerating", Confidence: 0.83},
		{Ref: 1, Tier: "novice", Growth: "stopped", Confidence: 0.9},
	}
	runner := &scriptedExpertiseRunner{result: okExpertiseResult(t, want)}
	ev := &BridgeExpertiseEvaluator{Runner: runner}

	got, err := ev.ClassifyExpertise(context.Background(), 200, sampleExpertiseSignals())
	if err != nil {
		t.Fatalf("ClassifyExpertise: %v", err)
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("classifications = %+v, want %+v", got, want)
	}
	if runner.gotEnv.ScenarioID != "expertise_classify" {
		t.Errorf("ScenarioID = %q, want expertise_classify", runner.gotEnv.ScenarioID)
	}

	// The structured context must carry data_days + the public per-topic signals
	// (incl. ref) but NOT the internal TopicID (json:"-").
	var sent struct {
		DataDays int              `json:"data_days"`
		Topics   []map[string]any `json:"topics"`
	}
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not JSON: %v", err)
	}
	if sent.DataDays != 200 {
		t.Errorf("data_days = %d, want 200", sent.DataDays)
	}
	if len(sent.Topics) != 2 {
		t.Fatalf("expected 2 topics in envelope, got %d", len(sent.Topics))
	}
	if sent.Topics[0]["topic_name"] != "Vector databases" || sent.Topics[0]["ref"] == nil {
		t.Errorf("public signals not forwarded: %v", sent.Topics[0])
	}
	for _, leaked := range []string{"TopicID", "topic_id"} {
		if _, ok := sent.Topics[0][leaked]; ok {
			t.Errorf("internal field %q leaked into the LLM envelope: %v", leaked, sent.Topics[0])
		}
	}
}

func TestBridgeExpertiseEvaluator_EmptyTopics(t *testing.T) {
	runner := &scriptedExpertiseRunner{returnNl: true} // would error if invoked
	ev := &BridgeExpertiseEvaluator{Runner: runner}

	got, err := ev.ClassifyExpertise(context.Background(), 10, nil)
	if err != nil {
		t.Fatalf("empty topics should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil classifications for empty input, got %+v", got)
	}
	if runner.gotEnv.ScenarioID != "" {
		t.Errorf("runner must not be invoked for empty input, got scenario %q", runner.gotEnv.ScenarioID)
	}
}

func TestBridgeExpertiseEvaluator_ErrorPaths(t *testing.T) {
	cases := []struct {
		name   string
		runner *scriptedExpertiseRunner
	}{
		{name: "nil_result", runner: &scriptedExpertiseRunner{returnNl: true}},
		{name: "non_ok_outcome", runner: &scriptedExpertiseRunner{result: &agent.InvocationResult{Outcome: agent.Outcome("schema-failure")}}},
		{name: "empty_final", runner: &scriptedExpertiseRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil}}},
		{name: "bad_json", runner: &scriptedExpertiseRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: json.RawMessage(`{nope`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &BridgeExpertiseEvaluator{Runner: tc.runner}
			if _, err := ev.ClassifyExpertise(context.Background(), 200, sampleExpertiseSignals()); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

func TestBridgeExpertiseEvaluator_NilReceiverAndRunner(t *testing.T) {
	var nilEv *BridgeExpertiseEvaluator
	if _, err := nilEv.ClassifyExpertise(context.Background(), 1, sampleExpertiseSignals()); !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil receiver should return agent.ErrJudgmentUnavailable, got %v", err)
	}
	ev := &BridgeExpertiseEvaluator{Runner: nil}
	if _, err := ev.ClassifyExpertise(context.Background(), 1, sampleExpertiseSignals()); !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil runner should return agent.ErrJudgmentUnavailable, got %v", err)
	}
}
