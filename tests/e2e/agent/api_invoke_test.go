//go:build e2e

// Spec 037 Scope 9 — POST /v1/agent/invoke e2e test.
//
// Drives the real api.AgentInvokeHandler (mounted on a real chi router
// via httptest.Server) against a real PostgresTracer (writes to the
// live DB and publishes on the live NATS) for every outcome class.
//
// The LLM is the only fake here: we use a scripted driver so each test
// can produce the precise outcome class on demand. This mirrors the
// scope 6 helpers' pattern and is the same pattern any e2e test would
// be forced into for the agent surface (real Ollama is non-deterministic
// and would hide outcome-mapping bugs behind LLM noise).
//
// Skips cleanly when DATABASE_URL or NATS_URL is unset so a pure
// `go test ./tests/e2e/...` (no live stack) does not fail.
package agent_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
)

// scriptedRunner is the api.AgentInvokeRunner the test injects. Each
// test sets a single result/decision; the bridge invokes Invoke once.
type scriptedRunner struct {
	mu       sync.Mutex
	result   *agent.InvocationResult
	decision *agent.RoutingDecision
	known    []string
	calls    int
}

func (s *scriptedRunner) Invoke(_ context.Context, _ agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.result, s.decision
}

func (s *scriptedRunner) KnownIntents() []string { return s.known }

// newAgentInvokeServer mounts a minimal chi router with the real
// AgentInvokeHandler. We do NOT mount the full api router (which would
// pull in DB/intelligence/etc) — the surface under test is the handler
// + userreply + chi wiring.
func newAgentInvokeServer(runner *scriptedRunner) *httptest.Server {
	h := &api.AgentInvokeHandler{Runner: runner}
	r := chi.NewRouter()
	r.Post("/v1/agent/invoke", h.AgentInvokeHandlerFunc)
	return httptest.NewServer(r)
}

// liveStackOrSkip ensures the live test stack is reachable; otherwise
// the test skips. Even though this test does not directly write to the
// DB, scope 9's test plan requires live-stack execution to keep the
// adversarial regressions honest (no hidden mocks).
func liveStackOrSkip(t *testing.T) {
	t.Helper()
	pool := liveDB(t) // skips if DATABASE_URL unset
	_ = pool          // we just want to fail-fast if the stack is down
	nc := liveNATS(t) // skips if NATS_URL unset
	_ = nc
}

func postInvoke(t *testing.T, srv *httptest.Server, body []byte) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/agent/invoke", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, out
}

func TestAgentInvoke_OK(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_ok", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeOK,
			Final: json.RawMessage(`{"answer":"42 €"}`),
		},
		known: []string{"expense_question"},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	body := []byte(`{"raw_input":"how much did I spend?"}`)
	status, env := postInvoke(t, srv, body)
	if status != http.StatusOK {
		t.Fatalf("status=%d env=%v", status, env)
	}
	if env["outcome"] != "ok" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
	if env["trace_id"] != "trace_e2e_ok" {
		t.Fatalf("trace_id=%v", env["trace_id"])
	}
	res, ok := env["result"].(map[string]any)
	if !ok || res["answer"] != "42 €" {
		t.Fatalf("result=%v", env["result"])
	}
}

func TestAgentInvoke_UnknownIntent(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_unk", Outcome: agent.OutcomeUnknownIntent,
		},
		decision: &agent.RoutingDecision{
			Reason: agent.ReasonUnknownIntent,
			Considered: []agent.CandidateScore{
				{ScenarioID: "recipe_question", Score: 0.04},
				{ScenarioID: "expense_question", Score: 0.03},
			},
		},
		known: []string{"recipe_question", "expense_question"},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"asdkfj qwerty zxcv"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d env=%v", status, env)
	}
	if env["outcome"] != "unknown-intent" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
	cands, ok := env["candidates"].([]any)
	if !ok || len(cands) != 2 {
		t.Fatalf("candidates=%v", env["candidates"])
	}
}

func TestAgentInvoke_AllowlistViolation(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_allow", ScenarioID: "expense_summary",
			ScenarioVersion: "v1", Outcome: agent.OutcomeAllowlistViolation,
			Final: json.RawMessage(`{"answer":"safe summary"}`),
			ToolCalls: []agent.ExecutedToolCall{{
				Name: "delete_all_expenses", Outcome: agent.OutcomeAllowlistViolation,
				RejectionReason: "not_in_allowlist",
			}},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"summarize and delete"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["outcome"] != "allowlist-violation" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
	blocked, ok := env["blocked"].([]any)
	if !ok || len(blocked) != 1 {
		t.Fatalf("blocked=%v", env["blocked"])
	}
	first := blocked[0].(map[string]any)
	if first["tool"] != "delete_all_expenses" {
		t.Fatalf("blocked[0].tool=%v", first["tool"])
	}
}

func TestAgentInvoke_SchemaFailure(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_schema", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeSchemaFailure,
			SchemaRetries: 2,
			OutcomeDetail: map[string]any{"error": "field 'sources' expected array, got string"},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"avg grocery this month"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["outcome"] != "schema-failure" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
	// json.Number from UseNumber()
	if attempts := env["attempts"]; attempts.(json.Number).String() != "2" {
		t.Fatalf("attempts=%v", attempts)
	}
	if !strings.Contains(env["last_error"].(string), "expected array") {
		t.Fatalf("last_error=%v", env["last_error"])
	}
}

func TestAgentInvoke_ToolError(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_tool", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeToolError,
			ToolCalls: []agent.ExecutedToolCall{{
				Name: "search_expenses", Outcome: agent.OutcomeToolError,
				Error: "db_timeout",
			}},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"dining costs"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["tool"] != "search_expenses" {
		t.Fatalf("tool=%v", env["tool"])
	}
	if env["message"] != "db_timeout" {
		t.Fatalf("message=%v", env["message"])
	}
}

func TestAgentInvoke_ToolReturnInvalid(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_returnschema", ScenarioID: "expense_classify",
			ScenarioVersion: "v4", Outcome: agent.OutcomeToolReturnInvalid,
			ToolCalls: []agent.ExecutedToolCall{{
				Name: "count_expenses", Outcome: agent.OutcomeToolReturnInvalid,
			}},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"classify"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["outcome"] != "tool-return-invalid" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
	if env["tool"] != "count_expenses" {
		t.Fatalf("tool=%v", env["tool"])
	}
}

func TestAgentInvoke_LoopLimit(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_loop", ScenarioID: "expense_summary",
			ScenarioVersion: "v1", Outcome: agent.OutcomeLoopLimit,
			Iterations: 8,
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"summarize"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["calls"].(json.Number).String() != "8" {
		t.Fatalf("calls=%v", env["calls"])
	}
}

func TestAgentInvoke_Timeout(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_timeout", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeTimeout,
			OutcomeDetail: map[string]any{"deadline_s": 60},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"summarize the year"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["deadline_s"].(json.Number).String() != "60" {
		t.Fatalf("deadline_s=%v", env["deadline_s"])
	}
}

func TestAgentInvoke_ProviderError(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_provider", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeProviderError,
			OutcomeDetail: map[string]any{"error": "ollama_unreachable"},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"x"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["outcome"] != "provider-error" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
}

func TestAgentInvoke_HallucinatedTool(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_halluc", ScenarioID: "expense_summary",
			ScenarioVersion: "v1", Outcome: agent.OutcomeHallucinatedTool,
			ToolCalls: []agent.ExecutedToolCall{{
				Name: "fake_tool", Outcome: agent.OutcomeHallucinatedTool,
				RejectionReason: "unknown_tool",
			}},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"x"}`))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if env["outcome"] != "hallucinated-tool" {
		t.Fatalf("outcome=%v", env["outcome"])
	}
	if env["tool"] != "fake_tool" {
		t.Fatalf("tool=%v", env["tool"])
	}
}

func TestAgentInvoke_InputSchemaViolationReturns400(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{
		result: &agent.InvocationResult{
			TraceID: "trace_e2e_input", ScenarioID: "expense_question",
			ScenarioVersion: "v3", Outcome: agent.OutcomeInputSchemaViolation,
			OutcomeDetail: map[string]any{"error": "structured_context is not valid JSON"},
		},
	}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"q","scenario_id":"expense_question"}`))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (env=%v)", status, env)
	}
	if env["error"] != "input_schema_violation" {
		t.Fatalf("error=%v", env["error"])
	}
	if env["trace_id"] != "trace_e2e_input" {
		t.Fatalf("trace_id=%v", env["trace_id"])
	}
}

func TestAgentInvoke_MalformedRequestEnvelope(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{} // never invoked
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	// Empty body
	status, env := postInvoke(t, srv, []byte(`{}`))
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d (env=%v)", status, env)
	}
	if env["error"] != "missing_field" {
		t.Fatalf("error=%v", env["error"])
	}
	if env["field"] != "raw_input" {
		t.Fatalf("field=%v", env["field"])
	}
	if runner.calls != 0 {
		t.Fatalf("runner should not have been invoked, got %d calls", runner.calls)
	}
}

func TestAgentInvoke_RunnerNilResultReturns503(t *testing.T) {
	liveStackOrSkip(t)
	runner := &scriptedRunner{result: nil}
	srv := newAgentInvokeServer(runner)
	defer srv.Close()
	status, env := postInvoke(t, srv, []byte(`{"raw_input":"x"}`))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (env=%v)", status, env)
	}
	if env["error"] != "agent_invoke_failed" {
		t.Fatalf("error=%v", env["error"])
	}
}
