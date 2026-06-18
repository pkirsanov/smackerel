// Spec 095 SCOPE-07 / PKT-095-B — tests for the production agent-bridge
// EvergreenJudge. The judge is exercised with a scripted bridge runner (no live
// LLM); the transport primitive (agent.InvokeJudgment) is covered in
// internal/agent. These tests pin the scenario id, the envelope source, the
// signal forwarding, and the ArtifactID-stays-internal contract.
package evergreen

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// scriptedEvergreenRunner is an agent.JudgmentRunner that returns a canned
// InvocationResult and records the envelope it received.
type scriptedEvergreenRunner struct {
	result   *agent.InvocationResult
	gotEnv   agent.IntentEnvelope
	returnNl bool
}

func (s *scriptedEvergreenRunner) Invoke(_ context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.gotEnv = env
	if s.returnNl {
		return nil, nil
	}
	return s.result, nil
}

func okEvergreenResult(t *testing.T, decision EvergreenDecision) *agent.InvocationResult {
	t.Helper()
	final, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: final}
}

func TestBridgeEvergreenJudge_ParsesDecision(t *testing.T) {
	want := EvergreenDecision{IsEvergreen: true, Confidence: 0.88, Rationale: "user attached context to a durable reference note"}
	runner := &scriptedEvergreenRunner{result: okEvergreenResult(t, want)}
	j := &BridgeEvergreenJudge{Runner: runner}

	cand := EvergreenCandidate{ArtifactID: "art-123", SourceKind: "gmail", ContentLen: 500, UserStarred: true, HasContext: true}
	got, err := j.JudgeEvergreen(context.Background(), cand)
	if err != nil {
		t.Fatalf("JudgeEvergreen: %v", err)
	}
	if got != want {
		t.Errorf("decision = %+v, want %+v", got, want)
	}

	// MUST route to the explicit scenario, from the pipeline source.
	if runner.gotEnv.ScenarioID != EvergreenScenarioID {
		t.Errorf("ScenarioID = %q, want %q", runner.gotEnv.ScenarioID, EvergreenScenarioID)
	}
	if runner.gotEnv.Source != "pipeline" {
		t.Errorf("Source = %q, want pipeline", runner.gotEnv.Source)
	}

	// The candidate signals MUST be forwarded in the structured context.
	var sent EvergreenCandidate
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &sent); err != nil {
		t.Fatalf("structured context not the candidate JSON: %v", err)
	}
	if sent.SourceKind != "gmail" || sent.ContentLen != 500 || !sent.UserStarred || !sent.HasContext {
		t.Errorf("candidate signals not forwarded: %+v", sent)
	}

	// ArtifactID is internal-only (json:"-") and MUST NOT leak into the prompt.
	if sent.ArtifactID != "" {
		t.Errorf("ArtifactID leaked into the unmarshaled candidate: %q", sent.ArtifactID)
	}
	var raw map[string]any
	if err := json.Unmarshal(runner.gotEnv.StructuredContext, &raw); err != nil {
		t.Fatalf("structured context not an object: %v", err)
	}
	if _, ok := raw["ArtifactID"]; ok {
		t.Error("ArtifactID key present in the LLM envelope JSON — correlation key leaked")
	}
	if _, ok := raw["source_kind"]; !ok {
		t.Error("source_kind key missing from the LLM envelope JSON — signals not forwarded")
	}
}

func TestBridgeEvergreenJudge_ErrorPaths(t *testing.T) {
	cases := []struct {
		name   string
		runner *scriptedEvergreenRunner
	}{
		{name: "nil_result", runner: &scriptedEvergreenRunner{returnNl: true}},
		{name: "non_ok_outcome", runner: &scriptedEvergreenRunner{result: &agent.InvocationResult{Outcome: agent.Outcome("schema-failure")}}},
		{name: "empty_final", runner: &scriptedEvergreenRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil}}},
		{name: "bad_json", runner: &scriptedEvergreenRunner{result: &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: json.RawMessage(`{not json`)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := &BridgeEvergreenJudge{Runner: tc.runner}
			if _, err := j.JudgeEvergreen(context.Background(), EvergreenCandidate{ArtifactID: "a"}); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}

func TestBridgeEvergreenJudge_NilReceiver(t *testing.T) {
	var j *BridgeEvergreenJudge
	_, err := j.JudgeEvergreen(context.Background(), EvergreenCandidate{ArtifactID: "a"})
	if !errors.Is(err, agent.ErrJudgmentUnavailable) {
		t.Errorf("nil receiver should return agent.ErrJudgmentUnavailable, got %v", err)
	}
}

// TestNoopRetrievalEvergreenRegistered proves the package init() registered the
// loader-contract no-op tool, so config/prompt_contracts/retrieval-evergreen-v1.yaml
// passes the spec 037 allowed_tools validation wherever this package is imported
// (cmd/core transitively, cmd/scenario-lint explicitly).
func TestNoopRetrievalEvergreenRegistered(t *testing.T) {
	if !agent.Has("noop_retrieval_evergreen") {
		t.Fatal("noop_retrieval_evergreen is NOT registered — the retrieval_evergreen scenario would be rejected by the loader (init() regressed)")
	}
}
