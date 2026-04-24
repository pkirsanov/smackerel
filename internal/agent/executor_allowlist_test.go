package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_BS003_AllowlistRejectionThenRetrySucceeds: the LLM first
// proposes a tool that is registered globally but NOT in the scenario
// allowlist; the executor rejects without dispatch, the LLM receives a
// structured tool_not_allowed error, retries with an allowed tool, and
// finalises ok.
func TestExecutor_BS003_AllowlistRejectionThenRetrySucceeds(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerEchoTool(t, "echo")
	// Register a write-class tool that is NOT in the scenario's allowlist.
	dispatchedDelete := false
	RegisterTool(Tool{
		Name:            "delete_all_expenses",
		Description:     "DESTRUCTIVE — purges all expense rows",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		SideEffectClass: SideEffectWrite,
		OwningPackage:   "executor_test",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			dispatchedDelete = true
			return json.RawMessage(`{}`), nil
		},
	})

	driver := newScriptedDriver(
		// Turn 1 — propose a disallowed tool.
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "delete_all_expenses", Arguments: json.RawMessage(`{}`)}},
		}},
		// Turn 2 — recover with the allowed tool.
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: jsonObj(t, map[string]string{"q": "ok"})}},
		}},
		// Turn 3 — finalise.
		turnReplyOrError{resp: TurnResponse{Final: json.RawMessage(`{"answer":"recovered"}`)}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "echo", SideEffectClass: SideEffectRead}}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	if res.Outcome != OutcomeOK {
		t.Fatalf("outcome = %s, want ok; detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if dispatchedDelete {
		t.Fatal("BS-003 violated: disallowed write tool was dispatched")
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("expected two tool-call records (rejection + success), got %d", len(res.ToolCalls))
	}
	rej := res.ToolCalls[0]
	if rej.Name != "delete_all_expenses" || rej.Outcome != OutcomeAllowlistViolation || rej.RejectionReason != "not_in_allowlist" {
		t.Fatalf("first tool-call record wrong: %+v", rej)
	}
	if res.ToolCalls[1].Outcome != OutcomeOK || res.ToolCalls[1].Name != "echo" {
		t.Fatalf("recovery tool-call record wrong: %+v", res.ToolCalls[1])
	}
}
