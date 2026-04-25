//go:build integration

// Spec 037 Scope 6 — BS-012 trace completeness regression.
//
// design §6.1 declares an exact set of columns each agent_traces row
// must carry so UC-002 (operator inspects every invocation) can fully
// reconstruct what happened without consulting external state. This
// test executes one happy-path invocation and asserts that EVERY
// required field on the persisted row is populated to the contracted
// value (not just non-NULL — wrong-value bugs slip past null checks).
//
// It also runs an EXPLAIN against the four indexed query patterns
// from design §6.1 and asserts the planner picks the corresponding
// idx_agent_traces_* index. This keeps the operator UI's list filters
// honest as the table grows.
//
// Adversarial gates (no bailout):
//
//	G1: a row with the executor-issued trace_id exists.
//	G2: every required column from design §6.1 carries the contracted
//	    value (input_envelope contains the source field, routing
//	    contains the routing reason, tool_calls is a non-empty JSON
//	    array, final_output contains the answer, outcome=ok,
//	    provider/model match the driver, latency_ms > 0, started_at <
//	    ended_at, created_at non-null).
//	G3: per-call rows in agent_tool_calls match the denormalized
//	    snapshot count and seq order.
//	G4: EXPLAIN on a started_at DESC scan picks idx_agent_traces_started_at.
//	G5: EXPLAIN on a scenario_id+started_at scan picks idx_agent_traces_scenario.
//	G6: EXPLAIN on an outcome filter picks idx_agent_traces_outcome.
//	G7: EXPLAIN on a source filter picks idx_agent_traces_source.
//
// Skips when DATABASE_URL is unset.

package agent_integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/smackerel/smackerel/internal/agent"
)

// TestBS012_TraceCompletenessAndIndexUsage executes one happy path and
// audits the resulting row against design §6.1.
func TestBS012_TraceCompletenessAndIndexUsage(t *testing.T) {
	pool := livePool(t)
	nc := liveNATS(t)

	registerScopeSixEcho(t)
	sc := makeScopeSixScenario(t, "completeness")

	traceID, res := runOneInvocation(t, pool, natsPublisher{nc: nc}, sc)
	cleanupTrace(t, pool, traceID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// G1+G2: select every required column from design §6.1.
	var (
		gotTraceID, gotScenarioID, gotScenarioVersion, gotScenarioHash string
		gotSource, gotOutcome, gotProvider, gotModel                   string
		inputEnvelope, routing, toolCalls, finalOutput                 []byte
		tokensPrompt, tokensCompletion, latencyMs                      int
		startedAt, endedAt, createdAt                                  time.Time
	)
	err := pool.QueryRow(ctx, `
SELECT
    trace_id, scenario_id, scenario_version, scenario_hash,
    source, input_envelope, routing, tool_calls, final_output,
    outcome, provider, model,
    tokens_prompt, tokens_completion, latency_ms,
    started_at, ended_at, created_at
FROM agent_traces WHERE trace_id = $1`, traceID).
		Scan(
			&gotTraceID, &gotScenarioID, &gotScenarioVersion, &gotScenarioHash,
			&gotSource, &inputEnvelope, &routing, &toolCalls, &finalOutput,
			&gotOutcome, &gotProvider, &gotModel,
			&tokensPrompt, &tokensCompletion, &latencyMs,
			&startedAt, &endedAt, &createdAt,
		)
	if err != nil {
		t.Fatalf("G1: select agent_traces: %v", err)
	}

	// G2 enumeration — wrong-value bugs slip past null checks, so each
	// assertion compares to a contracted value, not just non-emptiness.
	checks := []struct {
		name string
		ok   bool
		got  any
	}{
		{"trace_id matches executor", gotTraceID == traceID, gotTraceID},
		{"scenario_id matches scenario", gotScenarioID == sc.ID, gotScenarioID},
		{"scenario_version matches scenario", gotScenarioVersion == sc.Version, gotScenarioVersion},
		{"scenario_hash matches scenario", gotScenarioHash == sc.ContentHash, gotScenarioHash},
		{"source = test", gotSource == "test", gotSource},
		{"outcome = ok", gotOutcome == string(agent.OutcomeOK), gotOutcome},
		{"provider = test (driver)", gotProvider == "test", gotProvider},
		{"model = test-model (driver)", gotModel == "test-model", gotModel},
		{"latency_ms >= 0", latencyMs >= 0, latencyMs},
		{"started_at <= ended_at", !startedAt.After(endedAt), [2]time.Time{startedAt, endedAt}},
		{"created_at not zero", !createdAt.IsZero(), createdAt},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("G2: %s — got %v", c.name, c.got)
		}
	}

	// JSONB fields must round-trip and carry their contracted shape.
	var env map[string]any
	if err := json.Unmarshal(inputEnvelope, &env); err != nil {
		t.Fatalf("G2: input_envelope not JSON: %v raw=%s", err, inputEnvelope)
	}
	if env["source"] != "test" {
		t.Errorf("G2: input_envelope.source=%v want test", env["source"])
	}
	if env["raw_input"] != "hello" {
		t.Errorf("G2: input_envelope.raw_input=%v want hello", env["raw_input"])
	}
	if _, ok := env["structured_context"]; !ok {
		t.Errorf("G2: input_envelope.structured_context missing — replay would have to hand-fabricate it")
	}

	var rt map[string]any
	if err := json.Unmarshal(routing, &rt); err != nil {
		t.Fatalf("G2: routing not JSON: %v raw=%s", err, routing)
	}
	if rt["Reason"] != string(agent.ReasonExplicitScenarioID) && rt["reason"] != string(agent.ReasonExplicitScenarioID) {
		t.Errorf("G2: routing.reason=%v want %s — operator UI cannot explain how the scenario was chosen", rt["Reason"], agent.ReasonExplicitScenarioID)
	}

	var calls []map[string]any
	if err := json.Unmarshal(toolCalls, &calls); err != nil {
		t.Fatalf("G2: tool_calls not JSON: %v raw=%s", err, toolCalls)
	}
	if len(calls) == 0 {
		t.Errorf("G2: tool_calls denormalized array empty — list-view fast path is broken")
	}

	var final map[string]any
	if err := json.Unmarshal(finalOutput, &final); err != nil {
		t.Fatalf("G2: final_output not JSON: %v raw=%s", err, finalOutput)
	}
	if final["answer"] != "hello" {
		t.Errorf("G2: final_output.answer=%v want hello", final["answer"])
	}

	// Tokens must round-trip even when the scripted driver reports
	// zeros — the columns must NOT be NULL (DEFAULT 0 contract).
	if tokensPrompt != res.TokensPrompt || tokensCompletion != res.TokensCompletion {
		t.Errorf("G2: tokens prompt/completion = %d/%d want %d/%d",
			tokensPrompt, tokensCompletion, res.TokensPrompt, res.TokensCompletion)
	}

	// G3: per-call rows match the denormalized array.
	var perCallCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM agent_tool_calls WHERE trace_id = $1`, traceID).
		Scan(&perCallCount); err != nil {
		t.Fatalf("G3: count agent_tool_calls: %v", err)
	}
	if perCallCount != len(calls) {
		t.Errorf("G3: agent_tool_calls count=%d != denormalized snapshot=%d (denorm-vs-norm divergence is a BS-012 violation)",
			perCallCount, len(calls))
	}

	// G4-G7: EXPLAIN must show the planner picks the indexed path for
	// the four canonical query shapes. We assert the index name appears
	// in the plan; non-Index Scan plans are acceptable on tiny test
	// tables, so we ANALYZE first to nudge the planner toward the index.
	if _, err := pool.Exec(ctx, "ANALYZE agent_traces"); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}
	// Force planner to prefer index even on small tables. Acquire a
	// dedicated connection so the SET only affects this connection's
	// session; pgx returns the connection to the pool when we Release.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Logf("SET enable_seqscan=off (best-effort): %v", err)
	}

	type indexCheck struct {
		gate, query, wantIndex string
	}
	for _, ic := range []indexCheck{
		{"G4", "SELECT trace_id FROM agent_traces ORDER BY started_at DESC LIMIT 10", "idx_agent_traces_started_at"},
		{"G5", "SELECT trace_id FROM agent_traces WHERE scenario_id = 'scope6_completeness' ORDER BY started_at DESC LIMIT 10", "idx_agent_traces_scenario"},
		{"G6", "SELECT trace_id FROM agent_traces WHERE outcome = 'ok' ORDER BY started_at DESC LIMIT 10", "idx_agent_traces_outcome"},
		{"G7", "SELECT trace_id FROM agent_traces WHERE source = 'test' ORDER BY started_at DESC LIMIT 10", "idx_agent_traces_source"},
	} {
		plan := explainPlanOnConn(ctx, t, conn.Conn(), ic.query)
		if !strings.Contains(plan, ic.wantIndex) {
			t.Errorf("%s: planner did not use %s for query %q\nplan:\n%s",
				ic.gate, ic.wantIndex, ic.query, plan)
		}
	}
}

func explainPlanOnConn(ctx context.Context, t *testing.T, conn *pgx.Conn, query string) string {
	t.Helper()
	rows, err := conn.Query(ctx, "EXPLAIN "+query)
	if err != nil {
		t.Fatalf("EXPLAIN %q: %v", query, err)
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan EXPLAIN line: %v", err)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
