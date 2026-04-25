// Every outcome class produced by the executor MUST map to:
//
//   - a Telegram reply that is ≤ 4 lines AND ends with the trace ref;
//   - an API envelope with the documented status and shape per spec
//     §UX "End-User Failure Surface — API".
//
// These tests are pure (no I/O); the live-stack equivalents in
// tests/e2e/agent/api_invoke_test.go and telegram_replies_test.go
// drive the same mappings end-to-end with a real executor.
package userreply

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

// allOutcomes is the closed set the executor may return. If a future
// outcome is added to internal/agent/executor.go without being added
// here, the cover-every-outcome test below will fail.
var allOutcomes = []agent.Outcome{
	agent.OutcomeOK,
	agent.OutcomeUnknownIntent,
	agent.OutcomeAllowlistViolation,
	agent.OutcomeHallucinatedTool,
	agent.OutcomeToolError,
	agent.OutcomeToolReturnInvalid,
	agent.OutcomeSchemaFailure,
	agent.OutcomeLoopLimit,
	agent.OutcomeTimeout,
	agent.OutcomeProviderError,
	agent.OutcomeInputSchemaViolation,
}

func baseResult(o agent.Outcome) *agent.InvocationResult {
	return &agent.InvocationResult{
		TraceID:         "trace_test_001",
		ScenarioID:      "expense_question",
		ScenarioVersion: "v3",
		Outcome:         o,
		Iterations:      2,
		SchemaRetries:   1,
		ToolCalls:       []agent.ExecutedToolCall{},
	}
}

// TestRenderTelegramRespectsLineCap asserts the ≤4-line UX rule for
// every outcome class.
func TestRenderTelegramRespectsLineCap(t *testing.T) {
	for _, o := range allOutcomes {
		o := o
		t.Run(string(o), func(t *testing.T) {
			r := baseResult(o)
			// Outcome-specific data so the renderer has something to work with.
			switch o {
			case agent.OutcomeOK:
				r.Final = json.RawMessage(`{"answer":"42"}`)
			case agent.OutcomeAllowlistViolation:
				r.ToolCalls = []agent.ExecutedToolCall{{
					Name: "delete_all_expenses", Outcome: agent.OutcomeAllowlistViolation,
					RejectionReason: "not_in_allowlist",
				}}
			case agent.OutcomeToolError:
				r.ToolCalls = []agent.ExecutedToolCall{{
					Name: "search_expenses", Outcome: agent.OutcomeToolError,
					Error: "db_timeout",
				}}
			case agent.OutcomeToolReturnInvalid:
				r.ToolCalls = []agent.ExecutedToolCall{{
					Name: "count_expenses", Outcome: agent.OutcomeToolReturnInvalid,
				}}
			case agent.OutcomeHallucinatedTool:
				r.ToolCalls = []agent.ExecutedToolCall{{
					Name: "fake_tool", Outcome: agent.OutcomeHallucinatedTool,
					RejectionReason: "unknown_tool",
				}}
			case agent.OutcomeTimeout:
				r.OutcomeDetail = map[string]any{"deadline_s": 60}
			case agent.OutcomeSchemaFailure:
				r.OutcomeDetail = map[string]any{"error": "field 'sources' expected array"}
			}
			reply := RenderTelegram(Inputs{Result: r, KnownIntents: []string{"expenses", "recipes"}})
			lines := reply.Lines()
			if len(lines) == 0 {
				t.Fatalf("outcome %s: empty reply", o)
			}
			if len(lines) > MaxTelegramLines {
				t.Fatalf("outcome %s: reply has %d lines (>4):\n%s", o, len(lines), reply.Text)
			}
			if !reply.HasTraceRef() {
				t.Fatalf("outcome %s: missing trace ref:\n%s", o, reply.Text)
			}
		})
	}
}

func TestRenderTelegramOK(t *testing.T) {
	r := baseResult(agent.OutcomeOK)
	r.Final = json.RawMessage(`{"answer":"You spent 87,42 € on groceries last week."}`)
	reply := RenderTelegram(Inputs{Result: r})
	if !strings.Contains(reply.Text, "87,42 €") {
		t.Fatalf("expected answer text in reply, got:\n%s", reply.Text)
	}
}

// TestRenderTelegramUnknownIntentMarker is the unit-level twin of the
// BS-014 e2e regression: the structural marker MUST appear and the
// known-intents list MUST be present.
func TestRenderTelegramUnknownIntentMarker(t *testing.T) {
	r := baseResult(agent.OutcomeUnknownIntent)
	r.ScenarioID = ""
	r.ScenarioVersion = ""
	known := []string{"expenses", "recipes", "meal_plans"}
	reply := RenderTelegram(Inputs{Result: r, KnownIntents: known})
	if !strings.Contains(reply.Text, UnknownIntentMarker) {
		t.Fatalf("expected marker %q in reply, got:\n%s", UnknownIntentMarker, reply.Text)
	}
	for _, intent := range known {
		if !strings.Contains(reply.Text, intent) {
			t.Fatalf("expected intent %q listed, got:\n%s", intent, reply.Text)
		}
	}
	if !strings.Contains(reply.Text, "trace_test_001") {
		t.Fatalf("expected trace id in reply, got:\n%s", reply.Text)
	}
}

func TestRenderTelegramTimeoutEmitsDeadline(t *testing.T) {
	r := baseResult(agent.OutcomeTimeout)
	r.OutcomeDetail = map[string]any{"deadline_s": 60}
	reply := RenderTelegram(Inputs{Result: r})
	if !strings.Contains(reply.Text, "(60s)") {
		t.Fatalf("expected (60s) in reply, got:\n%s", reply.Text)
	}
	if !strings.Contains(reply.Text, TimeoutMarker) {
		t.Fatalf("expected timeout marker, got:\n%s", reply.Text)
	}
}

func TestRenderTelegramTimeoutFallsBackToTimeoutMs(t *testing.T) {
	r := baseResult(agent.OutcomeTimeout)
	r.OutcomeDetail = map[string]any{"timeout_ms": 30000}
	reply := RenderTelegram(Inputs{Result: r})
	if !strings.Contains(reply.Text, "(30s)") {
		t.Fatalf("expected (30s) derived from timeout_ms, got:\n%s", reply.Text)
	}
}

func TestRenderTelegramLoopLimitNamesIterationCount(t *testing.T) {
	r := baseResult(agent.OutcomeLoopLimit)
	r.Iterations = 8
	reply := RenderTelegram(Inputs{Result: r})
	if !strings.Contains(reply.Text, "8 things") {
		t.Fatalf("expected '8 things' in reply, got:\n%s", reply.Text)
	}
}

// --- API envelopes ---

func TestRenderAPIOK(t *testing.T) {
	r := baseResult(agent.OutcomeOK)
	r.Final = json.RawMessage(`{"answer":"hello","sources":["a"]}`)
	resp := RenderAPI(Inputs{Result: r})
	if resp.Status != 200 {
		t.Fatalf("ok: expected 200, got %d", resp.Status)
	}
	for _, k := range []string{"outcome", "scenario", "version", "trace_id", "result"} {
		if _, ok := resp.Body[k]; !ok {
			t.Fatalf("ok body missing %q: %+v", k, resp.Body)
		}
	}
	if resp.Body["outcome"] != "ok" {
		t.Fatalf("ok body outcome=%v", resp.Body["outcome"])
	}
	res, ok := resp.Body["result"].(map[string]any)
	if !ok {
		t.Fatalf("ok body result not map: %T", resp.Body["result"])
	}
	if res["answer"] != "hello" {
		t.Fatalf("ok body result.answer=%v", res["answer"])
	}
}

func TestRenderAPIUnknownIntent(t *testing.T) {
	r := baseResult(agent.OutcomeUnknownIntent)
	r.ScenarioID = ""
	d := &agent.RoutingDecision{
		Considered: []agent.CandidateScore{
			{ScenarioID: "recipe_question", Score: 0.04},
			{ScenarioID: "expense_question", Score: 0.03},
		},
	}
	resp := RenderAPI(Inputs{Result: r, Routing: d})
	if resp.Status != 200 {
		t.Fatalf("unknown-intent: expected 200, got %d", resp.Status)
	}
	cands, ok := resp.Body["candidates"].([]map[string]any)
	if !ok {
		t.Fatalf("candidates type %T", resp.Body["candidates"])
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(cands))
	}
	if cands[0]["scenario"] != "recipe_question" {
		t.Fatalf("expected first candidate recipe_question, got %v", cands[0])
	}
}

func TestRenderAPIAllowlistViolation(t *testing.T) {
	r := baseResult(agent.OutcomeAllowlistViolation)
	r.Final = json.RawMessage(`{"answer":"partial"}`)
	r.ToolCalls = []agent.ExecutedToolCall{{
		Name: "delete_all_expenses", Outcome: agent.OutcomeAllowlistViolation,
		RejectionReason: "not_in_allowlist",
	}}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Status != 200 {
		t.Fatalf("allowlist: expected 200, got %d", resp.Status)
	}
	blocked, ok := resp.Body["blocked"].([]map[string]any)
	if !ok || len(blocked) != 1 {
		t.Fatalf("expected one blocked entry, got %v", resp.Body["blocked"])
	}
	if blocked[0]["tool"] != "delete_all_expenses" {
		t.Fatalf("blocked tool=%v", blocked[0]["tool"])
	}
	if _, ok := resp.Body["result"]; !ok {
		t.Fatalf("expected result present when partial answer available")
	}
}

func TestRenderAPISchemaFailure(t *testing.T) {
	r := baseResult(agent.OutcomeSchemaFailure)
	r.SchemaRetries = 2
	r.OutcomeDetail = map[string]any{"error": "field 'sources' expected array, got string"}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Status != 200 || resp.Body["outcome"] != "schema-failure" {
		t.Fatalf("schema-failure body=%+v status=%d", resp.Body, resp.Status)
	}
	if resp.Body["attempts"] != 2 {
		t.Fatalf("attempts=%v", resp.Body["attempts"])
	}
	if !strings.Contains(resp.Body["last_error"].(string), "expected array") {
		t.Fatalf("last_error=%v", resp.Body["last_error"])
	}
}

func TestRenderAPILoopLimit(t *testing.T) {
	r := baseResult(agent.OutcomeLoopLimit)
	r.Iterations = 8
	resp := RenderAPI(Inputs{Result: r})
	if resp.Body["calls"] != 8 {
		t.Fatalf("calls=%v", resp.Body["calls"])
	}
}

func TestRenderAPITimeout(t *testing.T) {
	r := baseResult(agent.OutcomeTimeout)
	r.OutcomeDetail = map[string]any{"deadline_s": 60}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Body["deadline_s"] != 60 {
		t.Fatalf("deadline_s=%v", resp.Body["deadline_s"])
	}
}

func TestRenderAPIInputSchemaViolationReturns400(t *testing.T) {
	r := baseResult(agent.OutcomeInputSchemaViolation)
	r.OutcomeDetail = map[string]any{"error": "structured_context is not valid JSON"}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Status != 400 {
		t.Fatalf("expected 400, got %d", resp.Status)
	}
	if resp.Body["error"] != "input_schema_violation" {
		t.Fatalf("error=%v", resp.Body["error"])
	}
	if resp.Body["trace_id"] != "trace_test_001" {
		t.Fatalf("trace_id=%v", resp.Body["trace_id"])
	}
}

func TestRenderAPIToolError(t *testing.T) {
	r := baseResult(agent.OutcomeToolError)
	r.ToolCalls = []agent.ExecutedToolCall{{
		Name: "search_expenses", Outcome: agent.OutcomeToolError, Error: "db_timeout",
	}}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Body["tool"] != "search_expenses" {
		t.Fatalf("tool=%v", resp.Body["tool"])
	}
	if resp.Body["message"] != "db_timeout" {
		t.Fatalf("message=%v", resp.Body["message"])
	}
}

func TestRenderAPIProviderError(t *testing.T) {
	r := baseResult(agent.OutcomeProviderError)
	r.OutcomeDetail = map[string]any{"error": "ollama_unreachable"}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Body["outcome"] != "provider-error" {
		t.Fatalf("outcome=%v", resp.Body["outcome"])
	}
	if resp.Body["message"] != "ollama_unreachable" {
		t.Fatalf("message=%v", resp.Body["message"])
	}
}

func TestRenderAPIToolReturnInvalid(t *testing.T) {
	r := baseResult(agent.OutcomeToolReturnInvalid)
	r.ToolCalls = []agent.ExecutedToolCall{{
		Name: "count_expenses", Outcome: agent.OutcomeToolReturnInvalid,
	}}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Body["tool"] != "count_expenses" {
		t.Fatalf("tool=%v", resp.Body["tool"])
	}
	if resp.Body["outcome"] != "tool-return-invalid" {
		t.Fatalf("outcome=%v", resp.Body["outcome"])
	}
}

func TestRenderAPIHallucinatedTool(t *testing.T) {
	r := baseResult(agent.OutcomeHallucinatedTool)
	r.ToolCalls = []agent.ExecutedToolCall{{
		Name: "fake_tool", Outcome: agent.OutcomeHallucinatedTool,
		RejectionReason: "unknown_tool",
	}}
	resp := RenderAPI(Inputs{Result: r})
	if resp.Body["outcome"] != "hallucinated-tool" {
		t.Fatalf("outcome=%v", resp.Body["outcome"])
	}
	if resp.Body["tool"] != "fake_tool" {
		t.Fatalf("tool=%v", resp.Body["tool"])
	}
}

func TestMalformedRequestResponse(t *testing.T) {
	resp := MalformedRequestResponse("raw_input")
	if resp.Status != 400 {
		t.Fatalf("status=%d", resp.Status)
	}
	if resp.Body["error"] != "missing_field" {
		t.Fatalf("error=%v", resp.Body["error"])
	}
	if resp.Body["field"] != "raw_input" {
		t.Fatalf("field=%v", resp.Body["field"])
	}
}

func TestInfrastructureFailureResponse(t *testing.T) {
	resp := InfrastructureFailureResponse("trace_store_unreachable")
	if resp.Status != 503 {
		t.Fatalf("status=%d", resp.Status)
	}
	if resp.Body["error"] != "trace_store_unreachable" {
		t.Fatalf("error=%v", resp.Body["error"])
	}
}

// TestNilResultIsHandledFailLoud guarantees programmer errors do not
// produce free-form text (would violate BS-014 even at the surface).
func TestNilResultIsHandledFailLoud(t *testing.T) {
	rep := RenderTelegram(Inputs{Result: nil})
	if !strings.Contains(rep.Text, "Internal error") {
		t.Fatalf("expected internal-error reply, got %q", rep.Text)
	}
	resp := RenderAPI(Inputs{Result: nil})
	if resp.Status != 500 {
		t.Fatalf("expected 500, got %d", resp.Status)
	}
}
