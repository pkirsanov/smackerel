package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// TestExecutor_BS015_ToolErrorSurfacedToLLM: a tool returns an error.
// The executor MUST record it, surface a structured tool_error message
// to the LLM, and CONTINUE the loop so the LLM can recover (BS-015).
// The LLM finalises ok in the next turn.
func TestExecutor_BS015_ToolErrorSurfacedToLLM(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerErroringTool(t, "flaky", errors.New("database timeout"))
	registerEchoTool(t, "echo")

	driver := newScriptedDriver(
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "flaky", Arguments: json.RawMessage(`{}`)}},
		}},
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: jsonObj(t, map[string]string{"q": "fallback"})}},
		}},
		turnReplyOrError{resp: TurnResponse{
			Final: json.RawMessage(`{"answer":"recovered after tool error"}`),
		}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{
		{Name: "flaky", SideEffectClass: SideEffectRead},
		{Name: "echo", SideEffectClass: SideEffectRead},
	}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))
	if res.Outcome != OutcomeOK {
		t.Fatalf("outcome = %s, want ok; detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("expected two tool-call records, got %d", len(res.ToolCalls))
	}
	if res.ToolCalls[0].Outcome != OutcomeToolError || res.ToolCalls[0].RejectionReason != "tool_error" {
		t.Fatalf("first call should be tool_error, got %+v", res.ToolCalls[0])
	}
	if res.ToolCalls[0].Error == "" {
		t.Fatal("tool error must be recorded in trace, was empty")
	}
	if res.ToolCalls[1].Outcome != OutcomeOK {
		t.Fatalf("recovery call should be ok, got %+v", res.ToolCalls[1])
	}
}
