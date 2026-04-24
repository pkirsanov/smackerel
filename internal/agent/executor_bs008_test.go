package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_BS008_LoopLimitTerminatesRunaway is the adversarial
// regression for BS-008. With max_loop_iterations=K, an LLM that emits
// an infinite stream of valid tool calls MUST terminate at iteration
// K+1 with outcome=loop-limit and the trace MUST contain exactly K
// recorded calls (no more, no fewer).
//
// Failure modes this test catches:
//
//   - Executor never enforces the cap and loops forever (the test
//     would hang; we use a small K and a hard-bounded driver).
//   - Executor terminates at K (off-by-one) so the LLM never gets the
//     K-th turn.
//   - Executor terminates at K+2 or later so the trace shows >K calls.
//   - Executor returns a non-loop-limit outcome class.
//   - Executor bails out earlier on any single call (early-return
//     would produce <K records).
func TestExecutor_BS008_LoopLimitTerminatesRunaway(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerEchoTool(t, "echo")

	const K = 3
	limits := defaultLimits()
	limits.MaxLoopIterations = K

	// Each scripted response is a fresh tool call. We seed K+5 to be
	// safe; the executor MUST NOT consume more than K of them.
	responses := make([]turnReplyOrError, 0, K+5)
	for i := 0; i < K+5; i++ {
		responses = append(responses, turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: jsonObj(t, map[string]string{"q": "spin"})}},
		}})
	}
	driver := newScriptedDriver(responses...)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "echo", SideEffectClass: SideEffectRead}}, limits)
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	// Gate 1 — outcome class.
	if res.Outcome != OutcomeLoopLimit {
		t.Fatalf("Gate 1: outcome = %s, want %s; detail=%v", res.Outcome, OutcomeLoopLimit, res.OutcomeDetail)
	}

	// Gate 2 — exactly K recorded tool calls (one per LLM turn that
	// dispatched a tool).
	if len(res.ToolCalls) != K {
		t.Fatalf("Gate 2: recorded tool calls = %d, want exactly K=%d", len(res.ToolCalls), K)
	}

	// Gate 3 — the driver must have been called K+1 times: K turns
	// that dispatched a tool plus the one extra turn that triggered
	// the loop-limit check. Per design §5.1 the iter++ at the top of
	// the loop fires AFTER iteration K, so on the (K+1)-th iteration
	// the executor returns immediately WITHOUT calling the driver
	// again.
	if driver.Calls() != K {
		t.Fatalf("Gate 3: driver called %d times, want K=%d (the K+1 iteration MUST NOT issue a turn)", driver.Calls(), K)
	}

	// Gate 4 — outcome_detail names the cap so operators can tell why.
	cap, ok := res.OutcomeDetail["max_loop_iterations"].(int)
	if !ok || cap != K {
		t.Fatalf("Gate 4: outcome_detail.max_loop_iterations = %v, want %d", res.OutcomeDetail["max_loop_iterations"], K)
	}

	// Gate 5 — every recorded call is a successful echo (no
	// rejections; this isolates the loop-limit cause).
	for i, c := range res.ToolCalls {
		if c.Outcome != OutcomeOK {
			t.Fatalf("Gate 5: recorded call %d had outcome %s, want ok (the cause MUST be loop-limit, not a per-call failure)", i, c.Outcome)
		}
	}

	// Gate 6 — Iterations equals K (capped by the rollback in
	// finalize). This guarantees the trace shows exactly K turns
	// rather than K+1.
	if res.Iterations != K {
		t.Fatalf("Gate 6: Iterations = %d, want K=%d", res.Iterations, K)
	}

	// Gate 7 — the final field must be empty (no successful answer).
	if len(res.Final) != 0 {
		t.Fatalf("Gate 7: Final must be empty on loop-limit, got %s", string(res.Final))
	}

	// Sentinel — also assert json.Valid is the only inspection happening
	// against the recorded args (deters accidental mutation).
	for _, c := range res.ToolCalls {
		if !json.Valid(c.Arguments) {
			t.Fatalf("Sentinel: recorded args are not valid JSON: %s", string(c.Arguments))
		}
	}
}
