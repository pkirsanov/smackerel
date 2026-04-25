//go:build e2e

// Spec 037 Scope 6 — live-stack e2e helpers for the
// `smackerel agent replay` CLI.
//
// These helpers stand up the minimum needed to drive the CLI against
// the live test stack: a real Postgres connection (DATABASE_URL), a
// real NATS connection (NATS_URL — used here only to build the
// PostgresTracer's publisher; replay itself doesn't need NATS), a
// scenario-on-disk written into a temp directory the CLI's loader can
// read, and a `go run ./cmd/core agent replay <trace_id>` subprocess
// invocation that returns the structured exit code.
//
// All tests in this package skip cleanly when DATABASE_URL or NATS_URL
// is unset so a pure `go test ./tests/e2e/...` (no live stack) does
// not fail.

package agent_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/agent"
)

const echoToolName = "scope6_e2e_echo"

// liveDB returns a pgx pool against DATABASE_URL or skips.
func liveDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
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

// liveNATS returns a NATS connection against NATS_URL or skips.
func liveNATS(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("e2e: NATS_URL not set — live stack not available")
	}
	opts := []nats.Option{nats.Name("smackerel-scope6-e2e")}
	if tok := os.Getenv("SMACKEREL_AUTH_TOKEN"); tok != "" {
		opts = append(opts, nats.Token(tok))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

// natsPublisher adapts *nats.Conn to agent.TracePublisher.
type natsPublisher struct{ nc *nats.Conn }

func (p natsPublisher) Publish(_ context.Context, subject string, data []byte) error {
	return p.nc.Publish(subject, data)
}

// scriptedDriver returns canned LLM responses in order.
type scriptedDriver struct {
	mu    sync.Mutex
	turns []agent.TurnResponse
	idx   int
}

func (d *scriptedDriver) Turn(_ context.Context, _ agent.TurnRequest) (agent.TurnResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx >= len(d.turns) {
		return agent.TurnResponse{}, errors.New("scriptedDriver: exhausted")
	}
	r := d.turns[d.idx]
	d.idx++
	return r, nil
}

// registerEchoTool registers the e2e echo tool once per process.
// Re-registration would panic per registry contract.
func registerEchoTool(t *testing.T) {
	t.Helper()
	if agent.Has(echoToolName) {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:            echoToolName,
		Description:     "echo q back",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "scope6_e2e",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

// scenarioYAML is a minimal scenario definition the loader will accept.
// We parameterise the system_prompt so the FAIL test can mutate it
// between recording and replay (which changes content_hash and
// triggers the scenario_content_changed diff).
const scenarioYAML = `type: scenario
id: %s
version: %s-v1
description: scope 6 e2e replay scenario
intent_examples:
  - "echo this please"
  - "say hello"
allowed_tools:
  - name: %s
    side_effect_class: read
input_schema:
  type: object
  required: [q]
  properties:
    q:
      type: string
output_schema:
  type: object
  required: [answer]
  properties:
    answer:
      type: string
limits:
  max_loop_iterations: 4
  timeout_ms: 30000
  schema_retry_budget: 2
  per_tool_timeout_ms: 5000
token_budget: 1000
temperature: 0.1
model_preference: fast
side_effect_class: read
system_prompt: |
  %s
`

// writeScenarioDir writes one scenario YAML file into a fresh tempdir
// the CLI's loader can scan, returning the dir path.
func writeScenarioDir(t *testing.T, scenarioID, systemPrompt string) string {
	t.Helper()
	dir := t.TempDir()
	body := fmt.Sprintf(scenarioYAML, scenarioID, scenarioID, echoToolName, systemPrompt)
	path := filepath.Join(dir, scenarioID+".yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write scenario yaml: %v", err)
	}
	return dir
}

// loadScenarioFromDir runs the agent loader the same way the CLI does
// and returns the single registered scenario (or fails the test).
func loadScenarioFromDir(t *testing.T, dir, scenarioID string) *agent.Scenario {
	t.Helper()
	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "*.yaml")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	if len(rejected) > 0 {
		t.Fatalf("loader rejected: %+v", rejected)
	}
	for _, sc := range registered {
		if sc.ID == scenarioID {
			return sc
		}
	}
	t.Fatalf("scenario %s not loaded; registered=%d", scenarioID, len(registered))
	return nil
}

// recordOneTrace runs a happy-path invocation against the loaded scenario
// and returns the persisted trace_id.
func recordOneTrace(t *testing.T, pool *pgxpool.Pool, nc *nats.Conn, sc *agent.Scenario) string {
	t.Helper()
	tracer, err := agent.NewPostgresTracer(pool, natsPublisher{nc: nc}, false)
	if err != nil {
		t.Fatalf("NewPostgresTracer: %v", err)
	}
	driver := &scriptedDriver{turns: []agent.TurnResponse{
		{
			ToolCalls: []agent.LLMToolCall{{
				Name:      echoToolName,
				Arguments: json.RawMessage(`{"q":"hello"}`),
			}},
			Provider: "test", Model: "test-model",
		},
		{
			Final:    json.RawMessage(`{"answer":"hello"}`),
			Provider: "test", Model: "test-model",
		},
	}}
	exe, err := agent.NewExecutor(driver, tracer)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res := exe.Run(ctx, sc, agent.IntentEnvelope{
		Source:            "test",
		RawInput:          "hello",
		StructuredContext: json.RawMessage(`{"q":"hello"}`),
		Routing:           agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, Chosen: sc.ID},
	})
	if res == nil || res.Outcome != agent.OutcomeOK {
		t.Fatalf("invocation failed: outcome=%v detail=%+v", outcomeOf(res), detailOf(res))
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM agent_traces WHERE trace_id = $1", res.TraceID)
	})
	return res.TraceID
}

func outcomeOf(r *agent.InvocationResult) any {
	if r == nil {
		return "<nil result>"
	}
	return r.Outcome
}
func detailOf(r *agent.InvocationResult) any {
	if r == nil {
		return nil
	}
	return r.OutcomeDetail
}

// runReplayCLI invokes `go run ./cmd/core agent replay <args...>` against
// the workspace root and returns (exitCode, combinedOutput).
func runReplayCLI(t *testing.T, scenarioDir string, args ...string) (int, string) {
	t.Helper()
	root := workspaceRoot(t)
	allArgs := append([]string{"run", "-tags=e2e_agent_tools", "./cmd/core", "agent", "replay"}, args...)
	cmd := exec.Command("go", allArgs...)
	cmd.Dir = root

	// The CLI loads scenarios via agent.LoadConfig() which requires
	// the full AGENT_* env contract (~24 keys) plus DATABASE_URL.
	// Inherit the current process env (which the e2e harness or a
	// developer-shell `source config/generated/test.env` populates),
	// then layer test-specific overrides on top.
	env := append([]string{}, os.Environ()...)
	env = append(env, agentEnvFromTestStack(t)...)
	env = append(env,
		"AGENT_SCENARIO_DIR="+scenarioDir,
		"AGENT_SCENARIO_GLOB=*.yaml",
	)
	cmd.Env = env

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err == nil {
		return 0, out
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), out
	}
	t.Fatalf("go run failed (not an exit error): %v\noutput:\n%s", err, out)
	return -1, out
}

// agentEnvFromTestStack reads config/generated/test.env from the
// workspace root and returns every AGENT_* assignment as KEY=VALUE
// lines suitable for passing into exec.Cmd.Env. Returns nil and skips
// no test if the file is missing — the caller will fall back to the
// process environment, and any missing AGENT_* var will surface as a
// loud LoadConfig failure when the CLI runs.
func agentEnvFromTestStack(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(workspaceRoot(t), "config", "generated", "test.env")
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

// workspaceRoot finds the smackerel.sh root by walking up from CWD.
// The e2e harness `cd`s into /workspace inside docker, but a local
// `go test -tags=e2e ./tests/e2e/...` runs from the package dir.
func workspaceRoot(t *testing.T) string {
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

// envHas asserts that the substring appears in the CLI output (used to
// audit human-readable verdict strings without overspecifying format).
func envHas(t *testing.T, out, want string) {
	t.Helper()
	if !strings.Contains(out, want) {
		t.Fatalf("CLI output missing %q\noutput:\n%s", want, out)
	}
}
