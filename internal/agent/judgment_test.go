// Spec 021 BUG-021-010 — tests for the reusable LLM-judgment primitive.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/auth"
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

// ctxCapturingJudgmentRunner records the context InvokeJudgment built, which is
// the context every tool in the judgment turn would receive.
type ctxCapturingJudgmentRunner struct {
	gotCtx context.Context
	result *InvocationResult
}

func (s *ctxCapturingJudgmentRunner) Invoke(ctx context.Context, _ IntentEnvelope) (*InvocationResult, *RoutingDecision) {
	s.gotCtx = ctx
	return s.result, nil
}

// T-07 / SCN-07. A server-initiated invocation runs as an explicit system
// principal that holds no corpus grant.
//
// The value of asserting this — rather than asserting that no session is
// present — is that absence and intent look identical from the outside. Before
// BUG-061-012 these surfaces failed closed only because nothing had injected
// anything yet; the first upstream default session would have handed them
// whatever it granted, silently. A declared principal with a declared-empty
// grant set is a fact a test can hold onto.
func TestSystemSurfaces_InjectPrincipalWithoutCorpusGrant(t *testing.T) {
	runner := &ctxCapturingJudgmentRunner{result: okJudgmentResult(t, sampleDecision{Verdict: "surface", Score: 0.5})}

	if _, err := InvokeJudgment[sampleDecision](context.Background(), runner, "scheduler", "demo_scenario", sampleSignals{Subject: "x"}); err != nil {
		t.Fatalf("InvokeJudgment: %v", err)
	}

	sess, ok := auth.SessionFromContext(runner.gotCtx)
	if !ok {
		t.Fatal("judgment invoked with no session at all; 'no corpus authority' is then an accident rather than a decision")
	}
	if !auth.IsSystem(sess) {
		t.Errorf("session source = %q, want the system source; a server trigger must be identifiable as one", sess.Source)
	}
	if auth.GateGlobalCorpusRead(sess).Allowed {
		t.Errorf("the system principal is authorized to read the corpus (scopes=%v); a scheduler tick can read a user's knowledge base", sess.Scopes)
	}
	if sess.UserID == "" {
		t.Error("system principal has an empty UserID; the audit trail cannot name which surface acted")
	}

	// The judgment principal REPLACES any inbound session. A judgment invoked
	// while serving a user's HTTP request is still the server deciding, and
	// inheriting the user's grants there would be a privilege the caller never
	// asked to delegate.
	inbound := auth.WithSession(context.Background(),
		auth.SessionWithRole("u-caller", "tok-1", auth.RoleOperator, auth.GrantGlobalCorpusRead))
	if _, err := InvokeJudgment[sampleDecision](inbound, runner, "api", "demo_scenario", sampleSignals{Subject: "x"}); err != nil {
		t.Fatalf("InvokeJudgment(inbound): %v", err)
	}
	got, ok := auth.SessionFromContext(runner.gotCtx)
	if !ok {
		t.Fatal("no session after an inbound-session invocation")
	}
	if !auth.IsSystem(got) {
		t.Errorf("judgment inherited the caller's session (%q); a user's grants leaked into a server decision", got.UserID)
	}
	if auth.GateGlobalCorpusRead(got).Allowed {
		t.Errorf("judgment inherited corpus:read from the inbound caller (scopes=%v)", got.Scopes)
	}
}

