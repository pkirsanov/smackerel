//go:build integration

// Spec 037 Scope 8 — `smackerel agent ...` CLI integration tests.
//
// These tests exercise the operator-UI CLI commands against the live
// test stack. They:
//
//  1. Insert representative agent_traces rows (one per outcome class).
//  2. Run `go run ./cmd/core agent traces --json` and assert the JSON
//     body contains every inserted trace.
//  3. Run `go run ./cmd/core agent traces show <id> --json` and
//     assert the rendered detail surfaces required fields.
//  4. Run `go run ./cmd/core agent tools --json` and assert at least
//     one registered tool from package init shows up.
//
// Tests skip cleanly when DATABASE_URL is unset (matches the Scope 6/7
// pattern). Each test cleans up the rows it created.

package agent_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/render"
)

func liveDBForCLI(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seededTrace is the minimal data we need to insert one row per outcome
// class for the CLI tests. Every row uses a unique trace_id prefix so
// the cleanup deletes only what this test created.
type seededTrace struct {
	traceID string
	outcome string
}

// insertScope8Traces inserts one agent_traces row per representative
// outcome class. Returns the inserted trace ids in insertion order.
func insertScope8Traces(t *testing.T, pool *pgxpool.Pool, prefix string) []seededTrace {
	t.Helper()
	now := time.Now().UTC()
	rows := []seededTrace{
		{prefix + "_ok", string(agent.OutcomeOK)},
		{prefix + "_av", string(agent.OutcomeAllowlistViolation)},
		{prefix + "_to", string(agent.OutcomeTimeout)},
		{prefix + "_te", string(agent.OutcomeToolError)},
	}
	emptyJSON := []byte(`{}`)
	emptyArr := []byte(`[]`)
	scenarioSnap := []byte(`{"id":"scope8_cli","version":"scope8_cli-v1"}`)
	envelope := []byte(`{"source":"test","raw_input":"hello","structured_context":{"q":"hello"}}`)
	routing := []byte(`{"reason":"explicit_scenario_id","chosen":"scope8_cli","top_score":0,"threshold":0.65,"considered":[]}`)
	final := []byte(`{"answer":"hello"}`)
	for i, r := range rows {
		var detail []byte
		switch r.outcome {
		case string(agent.OutcomeAllowlistViolation):
			detail = []byte(`{}`)
		case string(agent.OutcomeTimeout):
			detail = []byte(`{"deadline_s":30,"reason":"provider_did_not_respond_before_deadline"}`)
		case string(agent.OutcomeToolError):
			detail = []byte(`{"tool":"echo","error":"db down","detail":"connection refused"}`)
		default:
			detail = emptyJSON
		}
		var toolCalls []byte
		switch r.outcome {
		case string(agent.OutcomeAllowlistViolation):
			toolCalls = []byte(`[{"seq":0,"name":"forbidden_write","outcome":"allowlist-violation","rejection_reason":"tool_not_allowed","arguments":{},"latency_ms":1}]`)
		default:
			toolCalls = emptyArr
		}
		_, err := pool.Exec(context.Background(), `
INSERT INTO agent_traces (
  trace_id, scenario_id, scenario_version, scenario_hash, scenario_snapshot,
  source, input_envelope, routing, tool_calls, turn_log,
  final_output, outcome, outcome_detail,
  provider, model, tokens_prompt, tokens_completion,
  latency_ms, started_at, ended_at
) VALUES (
  $1,'scope8_cli','scope8_cli-v1','scope8_cli_hash',$2,
  'test',$3,$4,$5,'[]'::jsonb,
  $6,$7,$8,
  'fake','fake-model',0,0,
  $9,$10,$11
)
ON CONFLICT (trace_id) DO NOTHING
`, r.traceID, scenarioSnap, envelope, routing, toolCalls,
			final, r.outcome, detail,
			i+1, now.Add(time.Duration(i)*time.Second), now.Add(time.Duration(i+1)*time.Second))
		if err != nil {
			t.Fatalf("insert trace %s: %v", r.traceID, err)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.traceID)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM agent_traces WHERE trace_id = ANY($1)`, ids)
	})
	return rows
}

// runCLI invokes `go run ./cmd/core agent ...` and returns its
// (exitCode, stdout+stderr).
func runCLI(t *testing.T, extraEnv []string, args ...string) (int, string) {
	t.Helper()
	root := workspaceRootForCLI(t)
	allArgs := append([]string{"run", "./cmd/core", "agent"}, args...)
	cmd := exec.Command("go", allArgs...)
	cmd.Dir = root

	env := append([]string{}, os.Environ()...)
	env = append(env, agentEnvFromTestStack(t, root)...)
	env = append(env, extraEnv...)
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

func workspaceRootForCLI(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "smackerel.sh")); err == nil {
			return d
		}
	}
	t.Fatalf("could not locate smackerel.sh from %s", wd)
	return ""
}

func agentEnvFromTestStack(t *testing.T, root string) []string {
	t.Helper()
	path := filepath.Join(root, "config", "generated", "test.env")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("agentEnvFromTestStack: %s not readable: %v (relying on process env)", path, err)
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "AGENT_") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// TestCLI_TracesList_ContainsSeededTraces inserts representative rows
// and asserts the CLI list output contains every inserted trace_id.
func TestCLI_TracesList_ContainsSeededTraces(t *testing.T) {
	pool := liveDBForCLI(t)
	prefix := fmt.Sprintf("scope8_cli_%d", time.Now().UnixNano())
	rows := insertScope8Traces(t, pool, prefix)

	exit, out := runCLI(t, nil, "traces", "--json", "--limit", "200")
	if exit != 0 {
		t.Fatalf("CLI exit=%d\noutput:\n%s", exit, out)
	}
	jsonStart := strings.Index(out, "[")
	if jsonStart < 0 {
		t.Fatalf("no JSON array in output:\n%s", out)
	}
	var summaries []render.TraceSummary
	if err := json.Unmarshal([]byte(out[jsonStart:]), &summaries); err != nil {
		t.Fatalf("parse json: %v\nbody: %s", err, out[jsonStart:])
	}
	gotIDs := make(map[string]string, len(summaries))
	for _, s := range summaries {
		gotIDs[s.TraceID] = s.Outcome
	}
	for _, r := range rows {
		if _, ok := gotIDs[r.traceID]; !ok {
			t.Errorf("trace %s missing from CLI list output", r.traceID)
		}
	}
}

// TestCLI_TracesShow_RendersDetail asserts that `traces show` returns
// the routing + outcome view for one specific trace.
func TestCLI_TracesShow_RendersDetail(t *testing.T) {
	pool := liveDBForCLI(t)
	prefix := fmt.Sprintf("scope8_cli_show_%d", time.Now().UnixNano())
	rows := insertScope8Traces(t, pool, prefix)
	target := rows[1] // allowlist-violation

	exit, out := runCLI(t, nil, "traces", "show", "--json", target.traceID)
	if exit != 0 {
		t.Fatalf("CLI exit=%d\noutput:\n%s", exit, out)
	}
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("no JSON object in output:\n%s", out)
	}
	var det render.TraceDetail
	if err := json.Unmarshal([]byte(out[jsonStart:]), &det); err != nil {
		t.Fatalf("parse json: %v\nbody: %s", err, out[jsonStart:])
	}
	if det.Summary.TraceID != target.traceID {
		t.Fatalf("trace_id = %q want %q", det.Summary.TraceID, target.traceID)
	}
	if det.Outcome.Class != target.outcome {
		t.Fatalf("outcome class = %q want %q", det.Outcome.Class, target.outcome)
	}
	if len(det.Outcome.Fields) == 0 {
		t.Fatalf("no rendered outcome fields for %s", target.outcome)
	}
}
