package agent

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
)

// BUG-064-001 DEFECT A — the open-knowledge agent must not refuse every query
// at the per-user-monthly USD pre-flight gate on a zero-cost (local Ollama +
// searxng) deployment.
//
// Live evidence (home-lab, 2026-06-11): every /ask routed to open_knowledge
// and returned `termination_reason=cap_usd, iterations:1, tokens_used:0,
// tool_calls:[]` — refused BEFORE any LLM/tool call — because the SST budget
// per_user_monthly_budget_usd was 0 while the production CostFn charges $0.
//
// The agent gate itself is correct (a genuinely-exhausted positive budget
// MUST still refuse — SCN-064-A08). These tests pin BOTH halves of that
// contract so the BUG-064-001 fix (a positive SST budget) is behaviourally
// guarded and the gate is not accidentally weakened.

// TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight asserts that
// with the BUG-064-001 fixed budget shape (monthly 100, per-user 25) and the
// production zero-cost CostFn, the agent proceeds past the pre-flight gate and
// actually grounds an answer instead of refusing cap_usd.
func TestAgent_BUG064001_PositivePerUserBudget_ProceedsPastPreflight(t *testing.T) {
	final := "The answer is 2.\n<CITATIONS>[{\"kind\":\"tool_computation\",\"tool\":\"calculator\",\"input\":{\"expression\":\"1+1\"},\"output\":{\"result\":2}}]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("c1", "calculator", `{"expression":"1+1"}`, 10),
		endTurn(final, 20),
	}}
	r := newRegistry(t)
	// monthlyUSD=100, perUserUSD=25 mirror the BUG-064-001 fixed SST values;
	// the zero-cost CostFn mirrors cmd/core/wiring_assistant_openknowledge.go.
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 100.0, 25.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is 1+1")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.TerminationReason == TerminationCapUSD {
		t.Fatalf("DEFECT A regression: pre-flight cap_usd refusal with a positive per-user budget (reason=%q, tokens=%d)", got.TerminationReason, got.TokensUsed)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q; want success (agent should ground via tools); refusal=%q", got.Status, got.RefusalReason)
	}
	if fl.calls == 0 {
		t.Fatalf("LLM was never called; agent did not proceed past the pre-flight gate")
	}
}

// TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight preserves the
// SCN-064-A08 contract: a genuinely-exhausted (zero remaining) per-user
// monthly budget MUST refuse cap_usd BEFORE any LLM call. This guards the
// fix from over-correcting (e.g. disabling the gate for paid providers).
func TestAgent_BUG064001_ZeroPerUserBudget_StillRefusesPreflight(t *testing.T) {
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("MUST-NOT-BE-REACHED", 1)}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000, 1.0, 100.0, 0.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.TerminationReason != TerminationCapUSD {
		t.Fatalf("SCN-064-A08: want cap_usd pre-flight refusal with zero per-user budget, got %q", got.TerminationReason)
	}
	if got.TokensUsed != 0 {
		t.Fatalf("pre-flight refusal must not call the LLM; TokensUsed=%d want 0", got.TokensUsed)
	}
	if fl.calls != 0 {
		t.Fatalf("pre-flight refusal must not call the LLM; fakeLLM.calls=%d want 0", fl.calls)
	}
}
