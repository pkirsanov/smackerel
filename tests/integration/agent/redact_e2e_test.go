//go:build integration

// Spec 037 Scope 7 — end-to-end x-redact persistence-boundary test
// (BS-022).
//
// Drives a real invocation through the executor against the live test
// stack, with a scenario whose input_schema and a tool whose
// input/output schemas mark fields x-redact:true. Then SELECTs the
// persisted agent_traces row and asserts:
//
//   G1: input_envelope.structured_context.contact == "***"
//   G2: input_envelope.structured_context.q       == "hello"  (untagged)
//   G3: tool_calls[0].arguments.password          == "***"
//   G4: tool_calls[0].result.token                == "***"
//   G5: agent_tool_calls.arguments.password       == "***"   (per-row)
//   G6: agent_tool_calls.result.token             == "***"   (per-row)
//   G7: the InvocationResult returned to the caller still contains
//       the REAL secret (handler-visible contract: persistence-only
//       redaction).

package agent_integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

const redactToolName = "scope7_redact_tool"

// registerRedactTool registers a tool whose input has a "password"
// field marked x-redact and whose output has a "token" field marked
// x-redact. Idempotent.
func registerRedactTool(t *testing.T) {
	t.Helper()
	if agent.Has(redactToolName) {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:        redactToolName,
		Description: "scope7 redact e2e tool",
		InputSchema: json.RawMessage(`{
            "type":"object",
            "required":["user"],
            "properties":{
              "user":{"type":"string"},
              "password":{"type":"string","x-redact":true}
            }
        }`),
		OutputSchema: json.RawMessage(`{
            "type":"object",
            "required":["status"],
            "properties":{
              "status":{"type":"string"},
              "token":{"type":"string","x-redact":true}
            }
        }`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "scope7_integration_test",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			// Echo args + a synthetic token so we can assert it is
			// stored redacted but returned to the caller raw.
			var in map[string]any
			_ = json.Unmarshal(args, &in)
			out := map[string]any{
				"status": "ok",
				"token":  "live-token-" + asString(in["password"]),
			}
			return json.Marshal(out)
		},
	})
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// makeRedactScenario returns a scenario whose input_schema marks
// `contact` x-redact:true and allows the redact tool.
func makeRedactScenario(t *testing.T, idSuffix string) *agent.Scenario {
	t.Helper()
	id := "scope7_redact_" + idSuffix
	inSchema := json.RawMessage(`{
        "type":"object",
        "required":["q"],
        "properties":{
          "q":{"type":"string"},
          "contact":{"type":"string","x-redact":true}
        }
    }`)
	outSchema := json.RawMessage(`{
        "type":"object",
        "required":["answer"],
        "properties":{
          "answer":{"type":"string"}
        }
    }`)
	inC, err := agent.CompileSchema(inSchema)
	if err != nil {
		t.Fatalf("compile in: %v", err)
	}
	outC, err := agent.CompileSchema(outSchema)
	if err != nil {
		t.Fatalf("compile out: %v", err)
	}
	return agent.NewScenarioForTest(agent.ScenarioForTest{
		ID:              id,
		Version:         id + "-v1",
		SystemPrompt:    "scope7 redact scenario",
		AllowedTools:    []agent.AllowedTool{{Name: redactToolName, SideEffectClass: agent.SideEffectRead}},
		InputSchema:     inSchema,
		OutputSchema:    outSchema,
		InputCompiled:   inC,
		OutputCompiled:  outC,
		Limits:          agent.ScenarioLimits{MaxLoopIterations: 4, TimeoutMs: 30000, SchemaRetryBudget: 2, PerToolTimeoutMs: 5000},
		TokenBudget:     1000,
		Temperature:     0.1,
		ModelPreference: "fast",
		SideEffectClass: agent.SideEffectRead,
		ContentHash:     "scope7_redact_hash_" + idSuffix,
		SourcePath:      "test://scope7/" + idSuffix + ".yaml",
	})
}

// TestRedactionAtPersistenceBoundary runs the BS-022 end-to-end
// proof: real DB, real executor, real handler call. Asserts the
// persisted JSON shows the marker and the in-memory result keeps the
// real value.
func TestRedactionAtPersistenceBoundary(t *testing.T) {
	pool := livePool(t)
	nc := liveNATS(t)

	registerRedactTool(t)
	sc := makeRedactScenario(t, "e2e")

	tracer, err := agent.NewPostgresTracer(pool, natsPublisher{nc: nc}, true)
	if err != nil {
		t.Fatalf("NewPostgresTracer: %v", err)
	}
	tracer.WithRedactMarker("***")

	driver := &liveScriptedDriver{turns: []agent.TurnResponse{
		{
			ToolCalls: []agent.LLMToolCall{{
				Name:      redactToolName,
				Arguments: json.RawMessage(`{"user":"alice","password":"hunter2"}`),
			}},
			Provider: "test", Model: "test-model",
		},
		{
			Final:    json.RawMessage(`{"answer":"done"}`),
			Provider: "test", Model: "test-model",
		},
	}}
	exe, err := agent.NewExecutor(driver, tracer)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res := exe.Run(ctx, sc, agent.IntentEnvelope{
		Source:            "test",
		RawInput:          "hello",
		StructuredContext: json.RawMessage(`{"q":"hello","contact":"alice@example.com"}`),
		Routing:           agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, Chosen: sc.ID},
	})
	if res == nil || res.Outcome != agent.OutcomeOK {
		t.Fatalf("invocation failed: %+v", res)
	}
	cleanupTrace(t, pool, res.TraceID)

	// G7: handler-visible contract — the live result returned to the
	// caller still has the real secret. (Tool's call arguments came
	// from the LLM; the handler returned a token derived from the real
	// password. We assert the executor's ToolCalls slice was NOT
	// mutated by the persistence-boundary redaction.)
	if len(res.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(res.ToolCalls))
	}
	if !strings.Contains(string(res.ToolCalls[0].Arguments), "hunter2") {
		t.Errorf("G7: in-memory tool call args mutated: %s", res.ToolCalls[0].Arguments)
	}
	if !strings.Contains(string(res.ToolCalls[0].Result), "live-token-hunter2") {
		t.Errorf("G7: in-memory tool call result mutated: %s", res.ToolCalls[0].Result)
	}

	// Now SELECT the persisted row.
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	var (
		envelopeJSON  []byte
		toolCallsJSON []byte
	)
	err = pool.QueryRow(rctx, `
SELECT input_envelope, tool_calls FROM agent_traces WHERE trace_id = $1`, res.TraceID).
		Scan(&envelopeJSON, &toolCallsJSON)
	if err != nil {
		t.Fatalf("select trace: %v", err)
	}

	// G1 + G2: input_envelope.structured_context redaction
	var env map[string]any
	if err := json.Unmarshal(envelopeJSON, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	sctx, _ := env["structured_context"].(map[string]any)
	if sctx["contact"] != "***" {
		t.Errorf("G1: persisted contact=%v want '***'  envelope=%s", sctx["contact"], envelopeJSON)
	}
	if sctx["q"] != "hello" {
		t.Errorf("G2: untagged q mutated: %v", sctx["q"])
	}

	// G3 + G4: denormalized tool_calls JSON redaction
	var calls []map[string]any
	if err := json.Unmarshal(toolCallsJSON, &calls); err != nil {
		t.Fatalf("unmarshal tool_calls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("denorm tool_calls len=%d want 1: %s", len(calls), toolCallsJSON)
	}
	args, _ := calls[0]["arguments"].(map[string]any)
	result, _ := calls[0]["result"].(map[string]any)
	if args["password"] != "***" {
		t.Errorf("G3: denorm args.password=%v want '***'", args["password"])
	}
	if args["user"] != "alice" {
		t.Errorf("G3: untagged user mutated: %v", args["user"])
	}
	if result["token"] != "***" {
		t.Errorf("G4: denorm result.token=%v want '***'", result["token"])
	}
	if result["status"] != "ok" {
		t.Errorf("G4: untagged status mutated: %v", result["status"])
	}

	// G5 + G6: per-row agent_tool_calls redaction
	var (
		argsRow   []byte
		resultRow []byte
	)
	err = pool.QueryRow(rctx, `
SELECT arguments, result FROM agent_tool_calls WHERE trace_id = $1 AND seq = 1`, res.TraceID).
		Scan(&argsRow, &resultRow)
	if err != nil {
		t.Fatalf("select tool_calls row: %v", err)
	}
	if !strings.Contains(string(argsRow), `"password"`) {
		t.Errorf("G5: per-row args missing password key: %s", argsRow)
	}
	if strings.Contains(string(argsRow), "hunter2") {
		t.Errorf("G5: per-row args leaked secret: %s", argsRow)
	}
	var argsParsed map[string]any
	if err := json.Unmarshal(argsRow, &argsParsed); err != nil {
		t.Fatalf("G5: per-row args not JSON: %v", err)
	}
	if argsParsed["password"] != "***" {
		t.Errorf("G5: per-row password=%v want '***'", argsParsed["password"])
	}
	if !strings.Contains(string(resultRow), `"token"`) {
		t.Errorf("G6: per-row result missing token key: %s", resultRow)
	}
	if strings.Contains(string(resultRow), "live-token-hunter2") {
		t.Errorf("G6: per-row result leaked secret: %s", resultRow)
	}
	var resultParsed map[string]any
	if err := json.Unmarshal(resultRow, &resultParsed); err != nil {
		t.Fatalf("G6: per-row result not JSON: %v", err)
	}
	if resultParsed["token"] != "***" {
		t.Errorf("G6: per-row token=%v want '***'", resultParsed["token"])
	}
}
