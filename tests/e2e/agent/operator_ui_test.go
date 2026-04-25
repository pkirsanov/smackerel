//go:build e2e

// Spec 037 Scope 8 — operator UI HTTP-level navigation e2e test.
//
// Stands up the real chi router + AgentAdminHandler against the live
// test Postgres pool, seeds one row per representative outcome class,
// then drives an HTTP client through:
//
//   GET /admin/agent/traces                  → assert table includes seeded rows
//   GET /admin/agent/traces?outcome=timeout  → assert filter narrows the result
//   GET /admin/agent/traces/<id>             → assert outcome banner present
//   GET /admin/agent/scenarios               → assert page renders
//   GET /admin/agent/scenarios/<id>          → assert page renders for a known scenario
//
// Honest e2e per Scope 8 spec: no headless browser is required for
// this scope's DoD ("navigate Trace List → Detail → Scenario Detail"),
// so we drive the real router with net/http/httptest.NewServer. The
// router, middleware, templates, queries, and DB are all real.

package agent_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/render"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/web"
)

func seedOpUITraces(t *testing.T, pool *pgxpool.Pool, prefix string) []string {
	t.Helper()
	now := time.Now().UTC()
	type seed struct {
		id      string
		outcome string
	}
	seeds := []seed{
		{prefix + "_ok", string(agent.OutcomeOK)},
		{prefix + "_to", string(agent.OutcomeTimeout)},
		{prefix + "_av", string(agent.OutcomeAllowlistViolation)},
	}
	scenarioSnap := []byte(`{"id":"scope8_e2e_ui","version":"scope8_e2e_ui-v1"}`)
	envelope := []byte(`{"source":"test","raw_input":"hi","structured_context":{"q":"hi"}}`)
	routing := []byte(`{"reason":"explicit_scenario_id","chosen":"scope8_e2e_ui"}`)
	final := []byte(`{"answer":"hi"}`)
	ids := make([]string, 0, len(seeds))
	for i, s := range seeds {
		var detail []byte
		switch s.outcome {
		case string(agent.OutcomeTimeout):
			detail = []byte(`{"deadline_s":30,"reason":"provider_did_not_respond_before_deadline"}`)
		default:
			detail = []byte(`{}`)
		}
		toolCalls := []byte(`[]`)
		if s.outcome == string(agent.OutcomeAllowlistViolation) {
			toolCalls = []byte(`[{"seq":0,"name":"forbidden_write","outcome":"allowlist-violation","rejection_reason":"tool_not_allowed","arguments":{},"latency_ms":1}]`)
		}
		_, err := pool.Exec(context.Background(), `
INSERT INTO agent_traces (
  trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
  source, input_envelope, routing, tool_calls, turn_log,
  final_output, outcome, outcome_detail,
  provider, model, tokens_prompt, tokens_completion,
  latency_ms, started_at, ended_at
) VALUES (
  $1,'scope8_e2e_ui','scope8_e2e_ui-v1','seu_hash',$2,
  'test',$3,$4,$5,'[]'::jsonb,
  $6,$7,$8,
  'fake','fake-model',0,0,
  $9,$10,$11
) ON CONFLICT (trace_id) DO NOTHING
`, s.id, scenarioSnap, envelope, routing, toolCalls,
			final, s.outcome, detail,
			i+1, now.Add(time.Duration(i)*time.Second), now.Add(time.Duration(i+1)*time.Second))
		if err != nil {
			t.Fatalf("insert %s: %v", s.id, err)
		}
		ids = append(ids, s.id)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM agent_traces WHERE trace_id = ANY($1)`, ids)
	})
	return ids
}

// stubScenario is a minimal scenario the admin handler's loader stub
// returns so /admin/agent/scenarios/<id> has something to render.
// Built by populating the exported fields directly — the render layer
// does not require compiled schemas.
func stubScenario() *agent.Scenario {
	return &agent.Scenario{
		ID:              "scope8_e2e_ui",
		Version:         "scope8_e2e_ui-v1",
		Description:     "operator-ui e2e fixture",
		SystemPrompt:    "fixture",
		IntentExamples:  []string{"hi", "hello"},
		AllowedTools:    []agent.AllowedTool{{Name: "scope8_op_ui_echo", SideEffectClass: agent.SideEffectRead}},
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`),
		Limits:          agent.ScenarioLimits{MaxLoopIterations: 4, TimeoutMs: 30000, SchemaRetryBudget: 2, PerToolTimeoutMs: 5000},
		TokenBudget:     1000,
		Temperature:     0.1,
		ModelPreference: "fast",
		SideEffectClass: agent.SideEffectRead,
		ContentHash:     "scope8_e2e_ui_hash",
		SourcePath:      "test://scope8_e2e_ui.yaml",
	}
}

// TestOperatorUI_NavigateTraceListToDetailToScenarioDetail asserts the
// real admin web served via the real chi router renders the trace
// list, the trace detail (with outcome banner), and the scenario
// detail page.
func TestOperatorUI_NavigateTraceListToDetailToScenarioDetail(t *testing.T) {
	pool := liveDB(t)
	prefix := fmt.Sprintf("scope8_opui_%d", time.Now().UnixNano())
	ids := seedOpUITraces(t, pool, prefix)

	scn := stubScenario()
	handler := web.NewAgentAdminHandler(pool)
	handler.LoadScenarios = func() ([]*agent.Scenario, []agent.LoadError, error) {
		return []*agent.Scenario{scn}, nil, nil
	}

	deps := &api.Dependencies{
		AgentAdminHandler: handler,
	}
	router := api.NewRouter(deps)
	srv := httptest.NewServer(router)
	defer srv.Close()
	client := srv.Client()

	// 1) Trace list
	body := mustGet(t, client, srv.URL+"/admin/agent/traces?limit=200")
	for _, id := range ids {
		if !strings.Contains(body, id) {
			t.Errorf("trace list missing seeded id %s", id)
		}
	}
	for _, oc := range []string{"ok", "timeout", "allowlist-violation"} {
		if !strings.Contains(body, oc) {
			t.Errorf("trace list missing outcome class %q", oc)
		}
	}

	// 2) Outcome filter
	filtered := mustGet(t, client, srv.URL+"/admin/agent/traces?outcome=timeout&limit=200")
	timeoutID := prefix + "_to"
	okID := prefix + "_ok"
	if !strings.Contains(filtered, timeoutID) {
		t.Errorf("filtered list missing timeout trace %s", timeoutID)
	}
	if strings.Contains(filtered, okID) {
		t.Errorf("filtered list leaked ok trace %s", okID)
	}

	// 3) Trace detail — assert outcome banner present + Required fields
	detailBody := mustGet(t, client, srv.URL+"/admin/agent/traces/"+timeoutID)
	if !strings.Contains(detailBody, `outcome-banner`) {
		t.Errorf("trace detail missing outcome-banner CSS class")
	}
	if !strings.Contains(detailBody, "Outcome: Timeout") {
		t.Errorf("trace detail missing Timeout label; body excerpt:\n%s", excerpt(detailBody, 400))
	}
	for _, key := range render.RequiredFields(string(agent.OutcomeTimeout)) {
		if !strings.Contains(detailBody, key) {
			t.Errorf("trace detail missing required field %q for outcome timeout", key)
		}
	}

	// 4) Scenario detail
	scnBody := mustGet(t, client, srv.URL+"/admin/agent/scenarios/scope8_e2e_ui")
	if !strings.Contains(scnBody, "scope8_e2e_ui-v1") {
		t.Errorf("scenario detail missing version; body excerpt:\n%s", excerpt(scnBody, 400))
	}
	if !strings.Contains(scnBody, "side-effect-read") {
		t.Errorf("scenario detail missing read side-effect badge css class")
	}
}

// TestOperatorUI_ScenarioCatalogShowsRejections asserts the rejected
// section renders when the loader produced rejections.
func TestOperatorUI_ScenarioCatalogShowsRejections(t *testing.T) {
	pool := liveDB(t)
	handler := web.NewAgentAdminHandler(pool)
	handler.LoadScenarios = func() ([]*agent.Scenario, []agent.LoadError, error) {
		return nil, []agent.LoadError{
			{Path: "/tmp/bad.yaml", Message: "missing required field id"},
		}, nil
	}
	deps := &api.Dependencies{AgentAdminHandler: handler}
	router := api.NewRouter(deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	body := mustGet(t, srv.Client(), srv.URL+"/admin/agent/scenarios")
	if !strings.Contains(body, "/tmp/bad.yaml") {
		t.Errorf("rejected scenario path missing from catalog")
	}
	if !strings.Contains(body, "missing required field id") {
		t.Errorf("rejected scenario reason missing from catalog")
	}
}

// TestOperatorUI_ToolDetailShowsSideEffectBadge asserts the tool
// detail view renders the side-effect badge with both text + a CSS
// color class.
func TestOperatorUI_ToolDetailShowsSideEffectBadge(t *testing.T) {
	pool := liveDB(t)
	// Register a fixture tool only once per process (repeat
	// registration would panic).
	const toolName = "scope8_op_ui_demo_tool"
	if !agent.Has(toolName) {
		agent.RegisterTool(agent.Tool{
			Name:            toolName,
			Description:     "Scope 8 operator UI fixture tool",
			InputSchema:     json.RawMessage(`{"type":"object"}`),
			OutputSchema:    json.RawMessage(`{"type":"object"}`),
			SideEffectClass: agent.SideEffectExternal,
			OwningPackage:   "scope8_e2e_ui",
			Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
				return args, nil
			},
		})
	}
	handler := web.NewAgentAdminHandler(pool)
	handler.LoadScenarios = func() ([]*agent.Scenario, []agent.LoadError, error) {
		return nil, nil, nil
	}
	deps := &api.Dependencies{AgentAdminHandler: handler}
	router := api.NewRouter(deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	body := mustGet(t, srv.Client(), srv.URL+"/admin/agent/tools/"+toolName)
	if !strings.Contains(body, "side-effect-external") {
		t.Errorf("tool detail missing side-effect-external CSS class")
	}
	if !strings.Contains(body, "external") {
		t.Errorf("tool detail missing 'external' text label")
	}
}

func mustGet(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d\nbody: %s", url, resp.StatusCode, body)
	}
	return string(body)
}

func excerpt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
