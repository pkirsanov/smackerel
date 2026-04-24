package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_BS004_ArgumentSchemaRejectionThenRecover: the LLM
// proposes echo(q=42) which violates the input schema (q must be
// string); the executor rejects without dispatch, the LLM retries with
// a valid argument shape, and the invocation finalises ok.
func TestExecutor_BS004_ArgumentSchemaRejectionThenRecover(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerEchoTool(t, "echo")

	driver := newScriptedDriver(
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: json.RawMessage(`{"q":42}`)}},
		}},
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: json.RawMessage(`{"q":"now-a-string"}`)}},
		}},
		turnReplyOrError{resp: TurnResponse{
			Final: json.RawMessage(`{"answer":"recovered after schema rejection"}`),
		}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "echo", SideEffectClass: SideEffectRead}}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))
	if res.Outcome != OutcomeOK {
		t.Fatalf("outcome = %s, want ok; detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("expected two tool-call records, got %d", len(res.ToolCalls))
	}
	first := res.ToolCalls[0]
	if first.Outcome != OutcomeToolError || first.RejectionReason != "argument_schema_violation" {
		t.Fatalf("first record should be argument_schema_violation, got %+v", first)
	}
	if res.ToolCalls[1].Outcome != OutcomeOK {
		t.Fatalf("recovery call should be ok, got %+v", res.ToolCalls[1])
	}
}
