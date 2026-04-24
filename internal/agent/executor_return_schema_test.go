package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_BS005_ReturnSchemaViolationTerminates: a tool returns a
// value that fails its declared output schema. The executor MUST
// terminate the invocation with `tool-return-invalid` and MUST NOT
// continue the LLM loop with the malformed result.
func TestExecutor_BS005_ReturnSchemaViolationTerminates(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerBadReturnTool(t, "bad_count")

	driver := newScriptedDriver(
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "bad_count", Arguments: json.RawMessage(`{}`)}},
		}},
		// If the loop incorrectly continues, this would be the next
		// turn — the test asserts driver was called exactly once so
		// we never reach this.
		turnReplyOrError{resp: TurnResponse{Final: json.RawMessage(`{"answer":"should-not-reach"}`)}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "bad_count", SideEffectClass: SideEffectRead}}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))
	if res.Outcome != OutcomeToolReturnInvalid {
		t.Fatalf("outcome = %s, want %s; detail=%v", res.Outcome, OutcomeToolReturnInvalid, res.OutcomeDetail)
	}
	if driver.Calls() != 1 {
		t.Fatalf("driver called %d times after return-schema violation; loop must terminate", driver.Calls())
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Outcome != OutcomeToolReturnInvalid {
		t.Fatalf("expected one tool call recorded as return-invalid, got %+v", res.ToolCalls)
	}
}
