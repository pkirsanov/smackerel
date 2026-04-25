//go:build e2e

// Spec 037 Scope 8 — `smackerel agent traces --outcome=...` filter
// e2e test.
//
// Seeds a known trace set spanning multiple outcome classes and runs
// the CLI with --outcome=allowlist-violation --json. Asserts the
// returned set contains every allowlist-violation row we inserted and
// no trace of any other class.

package agent_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/render"
)

func seedFilterTraces(t *testing.T, pool *pgxpool.Pool, prefix string) (allowlistIDs map[string]struct{}, allIDs map[string]struct{}) {
	t.Helper()
	now := time.Now().UTC()
	type seed struct {
		id      string
		outcome string
	}
	rows := []seed{
		{prefix + "_av_a", string(agent.OutcomeAllowlistViolation)},
		{prefix + "_av_b", string(agent.OutcomeAllowlistViolation)},
		{prefix + "_ok_a", string(agent.OutcomeOK)},
		{prefix + "_to_a", string(agent.OutcomeTimeout)},
	}
	scenarioSnap := []byte(`{"id":"scope8_e2e_filter","version":"scope8_e2e_filter-v1"}`)
	envelope := []byte(`{"source":"test","raw_input":"hi"}`)
	routing := []byte(`{"reason":"explicit_scenario_id","chosen":"scope8_e2e_filter"}`)
	final := []byte(`{"answer":"hi"}`)
	allowlistIDs = map[string]struct{}{}
	allIDs = map[string]struct{}{}
	for i, r := range rows {
		toolCalls := []byte(`[]`)
		if r.outcome == string(agent.OutcomeAllowlistViolation) {
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
  $1,'scope8_e2e_filter','scope8_e2e_filter-v1','sef_hash',$2,
  'test',$3,$4,$5,'[]'::jsonb,
  $6,$7,'{}'::jsonb,
  'fake','fake-model',0,0,
  $8,$9,$10
)
ON CONFLICT (trace_id) DO NOTHING
`, r.id, scenarioSnap, envelope, routing, toolCalls,
			final, r.outcome,
			i+1, now.Add(time.Duration(i)*time.Second), now.Add(time.Duration(i+1)*time.Second))
		if err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
		allIDs[r.id] = struct{}{}
		if r.outcome == string(agent.OutcomeAllowlistViolation) {
			allowlistIDs[r.id] = struct{}{}
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ids := make([]string, 0, len(rows))
		for id := range allIDs {
			ids = append(ids, id)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM agent_traces WHERE trace_id = ANY($1)`, ids)
	})
	return allowlistIDs, allIDs
}

// TestCLI_TracesOutcomeFilter_AllowlistViolation seeds 4 traces (2
// allowlist-violation + 2 other classes) and asserts that the CLI
// outcome filter returns ONLY the allowlist-violation rows from the
// seeded set, and at least the count we inserted.
func TestCLI_TracesOutcomeFilter_AllowlistViolation(t *testing.T) {
	pool := liveDB(t) // helpers_test.go
	prefix := fmt.Sprintf("scope8_e2e_filter_%d", time.Now().UnixNano())
	allowIDs, allIDs := seedFilterTraces(t, pool, prefix)

	exit, out := runAgentCLI(t, "traces", "--outcome=allowlist-violation", "--json", "--limit", "200")
	if exit != 0 {
		t.Fatalf("CLI exit=%d\noutput:\n%s", exit, out)
	}
	body := out[strings.Index(out, "["):]
	var rows []render.TraceSummary
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("parse json: %v\nbody: %s", err, body)
	}

	got := map[string]string{}
	for _, r := range rows {
		got[r.TraceID] = r.Outcome
		// G028: every returned row in this filtered set MUST be
		// allowlist-violation (no leakage). We allow rows we did not
		// seed (other tests may share the table) but they MUST also
		// be of class allowlist-violation.
		if r.Outcome != string(agent.OutcomeAllowlistViolation) {
			t.Errorf("filter leaked: trace %s has outcome %s", r.TraceID, r.Outcome)
		}
	}
	for id := range allowIDs {
		if _, ok := got[id]; !ok {
			t.Errorf("expected allowlist-violation trace %s missing from filtered output", id)
		}
	}
	for id := range allIDs {
		if _, isAllow := allowIDs[id]; isAllow {
			continue
		}
		if _, ok := got[id]; ok {
			t.Errorf("non-allowlist trace %s appeared in filtered output", id)
		}
	}
}

// runAgentCLI runs `go run ./cmd/core agent ...` reusing the same
// AGENT_* env loading the helpers_test.go runReplayCLI uses.
func runAgentCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	root := workspaceRoot(t)
	allArgs := append([]string{"run", "./cmd/core", "agent"}, args...)
	cmd := exec.Command("go", allArgs...)
	cmd.Dir = root
	env := append([]string{}, os.Environ()...)
	env = append(env, agentEnvFromTestStack(t)...)
	cmd.Env = env

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err == nil {
		return 0, out
	}
	if ex, ok := err.(*exec.ExitError); ok {
		return ex.ExitCode(), out
	}
	t.Fatalf("go run failed (not an exit error): %v\noutput:\n%s", err, out)
	return -1, out
}
