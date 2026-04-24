package agent

import (
	"context"
	"encoding/json"
	"testing"
)

// TestExecutor_HappyPath_ToolCallThenFinal exercises the full §5.1
// success path: the LLM proposes one valid tool call, the executor
// dispatches it, the LLM produces a schema-valid final answer, and the
// outcome is `ok`.
func TestExecutor_HappyPath_ToolCallThenFinal(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerEchoTool(t, "echo")

	driver := newScriptedDriver(
		// Turn 1 — propose echo({q:"hello"}).
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: jsonObj(t, map[string]string{"q": "hello"})}},
			Provider:  "ollama",
			Model:     "test",
			Tokens:    Tokens{Prompt: 10, Completion: 5},
		}},
		// Turn 2 — finalize with a schema-valid answer.
		turnReplyOrError{resp: TurnResponse{
			Final:    json.RawMessage(`{"answer":"echoed: hello"}`),
			Provider: "ollama",
			Model:    "test",
			Tokens:   Tokens{Prompt: 12, Completion: 7},
		}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{{Name: "echo", SideEffectClass: SideEffectRead}}, defaultLimits())
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))
	if res.Outcome != OutcomeOK {
		t.Fatalf("outcome = %s, want %s; detail=%v", res.Outcome, OutcomeOK, res.OutcomeDetail)
	}
	if string(res.Final) != `{"answer":"echoed: hello"}` {
		t.Fatalf("final = %s, want canonical schema-valid answer", string(res.Final))
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "echo" || res.ToolCalls[0].Outcome != OutcomeOK {
		t.Fatalf("expected one ok tool call, got %+v", res.ToolCalls)
	}
	if res.Iterations != 2 {
		t.Fatalf("iterations = %d, want 2 (one tool turn + one final turn)", res.Iterations)
	}
	if res.TokensPrompt != 22 || res.TokensCompletion != 12 {
		t.Fatalf("token totals wrong: prompt=%d completion=%d", res.TokensPrompt, res.TokensCompletion)
	}
	if driver.Calls() != 2 {
		t.Fatalf("driver called %d times, want 2", driver.Calls())
	}
}
