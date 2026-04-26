//go:build integration

// Spec 037 Scope 10 — scheduler bridge integration test.
//
// Proves that the internal/scheduler.FireScenario entry point routes a
// scheduler-initiated invocation through agent.Bridge → Executor.Run
// against the live test stack and that the persisted agent_traces row
// records source="scheduler" (the canonical signal pipeline+scheduler
// future triggers will rely on for filtered telemetry queries).
//
// This is a live-stack integration test: it requires DATABASE_URL and
// NATS_URL pointing at a running smackerel-test stack (real Postgres
// for trace persistence, real NATS for tracer event mirroring).
package agent_integration

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/scheduler"
)

// scriptedDriverScope10 is a tiny LLMDriver that returns canned turns
// in order. We do not use the loop_test.go fakeAgentResponder here
// because the scheduler/pipeline call sites are about *which surface*
// fires the bridge, not about the LLM transport — a direct in-process
// driver isolates the source attribution we are testing.
type scriptedDriverScope10 struct {
	mu    sync.Mutex
	turns []agent.TurnResponse
	idx   int
}

func (d *scriptedDriverScope10) Turn(_ context.Context, _ agent.TurnRequest) (agent.TurnResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx >= len(d.turns) {
		return agent.TurnResponse{}, nil
	}
	r := d.turns[d.idx]
	d.idx++
	return r, nil
}

// natsPublisherScope10 adapts a *nats.Conn to agent.TracePublisher.
type natsPublisherScope10 struct{ nc *nats.Conn }

func (p natsPublisherScope10) Publish(_ context.Context, subject string, data []byte) error {
	return p.nc.Publish(subject, data)
}

// liveStackForScope10 returns a (pool, nc) pair against the live test
// stack, or skips the test when the env contract is not satisfied.
func liveStackForScope10(t *testing.T) (*pgxpool.Pool, *nats.Conn) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	natsURL := os.Getenv("NATS_URL")
	if dbURL == "" || natsURL == "" {
		t.Skip("integration: DATABASE_URL and NATS_URL must be set")
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

	opts := []nats.Option{nats.Name("scope10-bridge-test")}
	if tok := os.Getenv("SMACKEREL_AUTH_TOKEN"); tok != "" {
		opts = append(opts, nats.Token(tok))
	}
	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(nc.Close)
	return pool, nc
}

// scope10Tool registers a per-test read-only tool exactly once. We
// guard with agent.Has because RegisterTool panics on duplicate (the
// global registry is process-wide, so a `go test -count=2` run would
// otherwise crash on the second invocation).
func scope10Tool(t *testing.T, name string) {
	t.Helper()
	if agent.Has(name) {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:            name,
		Description:     "scope10 echo tool for bridge tests",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "scope10_bridge_test",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

// scope10Scenario builds a minimal scenario in a temp dir and loads it
// via the real loader so the test exercises the production scenario
// schema end-to-end (no struct shortcuts).
func scope10Scenario(t *testing.T, scenarioID, toolName string) *agent.Scenario {
	t.Helper()
	dir := t.TempDir()
	yaml := "" +
		"version: \"" + scenarioID + "-v1\"\n" +
		"type: \"scenario\"\n" +
		"id: \"" + scenarioID + "\"\n" +
		"description: \"scope 10 bridge surface test\"\n" +
		"intent_examples:\n" +
		"  - \"echo q please\"\n" +
		"system_prompt: |\n" +
		"  scope 10 bridge integration\n" +
		"allowed_tools:\n" +
		"  - name: \"" + toolName + "\"\n" +
		"    side_effect_class: \"read\"\n" +
		"input_schema:\n" +
		"  type: object\n" +
		"  required: [q]\n" +
		"  properties:\n" +
		"    q: { type: string }\n" +
		"output_schema:\n" +
		"  type: object\n" +
		"  required: [answer]\n" +
		"  properties:\n" +
		"    answer: { type: string }\n" +
		"limits:\n" +
		"  max_loop_iterations: 4\n" +
		"  timeout_ms: 30000\n" +
		"  schema_retry_budget: 1\n" +
		"  per_tool_timeout_ms: 1000\n" +
		"token_budget: 500\n" +
		"temperature: 0.0\n" +
		"model_preference: \"fast\"\n" +
		"side_effect_class: \"read\"\n"
	path := dir + "/" + scenarioID + ".yaml"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	if len(rejected) != 0 {
		t.Fatalf("loader rejected: %+v", rejected)
	}
	if len(registered) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(registered))
	}
	return registered[0]
}

// buildBridgeScope10 wires a Bridge backed by a scripted driver +
// PostgresTracer against the live stack. Returns the bridge and the
// trace_id-cleanup hook the test should call after each invocation.
func buildBridgeScope10(t *testing.T, pool *pgxpool.Pool, nc *nats.Conn, sc *agent.Scenario, turns []agent.TurnResponse) *agent.Bridge {
	t.Helper()
	tracer, err := agent.NewPostgresTracer(pool, natsPublisherScope10{nc: nc}, false)
	if err != nil {
		t.Fatalf("NewPostgresTracer: %v", err)
	}
	driver := &scriptedDriverScope10{turns: turns}
	exe, err := agent.NewExecutor(driver, tracer)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	cfg := &agent.Config{
		ScenarioDir:  "/dev/null", // unused — we hand-load via scope10Scenario
		ScenarioGlob: "*.yaml",
		Routing: agent.RoutingConfig{
			ConfidenceFloor: 0.0,
			ConsiderTopN:    5,
		},
	}
	// We can't use the real loader path (the temp dir is per-test) so
	// we synthesize a Bridge that already has the scenario installed
	// by going through the public NewBridge with a stub loader.
	stub := stubLoader{scenarios: []*agent.Scenario{sc}}
	bridge, _, err := agent.NewBridge(context.Background(), agent.BridgeOptions{
		Config:   cfg,
		Loader:   stub,
		Executor: exe,
	})
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}
	return bridge
}

// stubLoader returns a fixed slice; used to bypass disk scanning.
type stubLoader struct{ scenarios []*agent.Scenario }

func (s stubLoader) Load(_, _ string) ([]*agent.Scenario, []agent.LoadError, error) {
	return s.scenarios, nil, nil
}

// fetchTraceSource reads the source column of the persisted trace row.
func fetchTraceSource(t *testing.T, pool *pgxpool.Pool, traceID string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var src string
	err := pool.QueryRow(ctx,
		`SELECT source FROM agent_traces WHERE trace_id = $1`, traceID,
	).Scan(&src)
	if err != nil {
		t.Fatalf("fetch trace source: %v", err)
	}
	return src
}

// TestScope10_SchedulerBridge_FiresExecutorWithSchedulerSource is the
// DoD gate for the scheduler call-site contract. It proves:
//
//	G1: scheduler.FireScenario routes through agent.Bridge → Executor.Run
//	    (no separate dispatch path).
//	G2: the invocation completes with outcome=ok.
//	G3: the persisted agent_traces row's source column is exactly
//	    "scheduler" — proving env.Source flowed through unchanged.
//	G4: the routing decision recorded ReasonExplicitScenarioID — the
//	    call site uses the explicit-id fast path (BS-002), so no
//	    embedding call is required to fire scheduler-driven scenarios.
func TestScope10_SchedulerBridge_FiresExecutorWithSchedulerSource(t *testing.T) {
	pool, nc := liveStackForScope10(t)

	scope10Tool(t, "scope10_sched_echo")
	sc := scope10Scenario(t, "scope10_sched", "scope10_sched_echo")
	turns := []agent.TurnResponse{
		{
			ToolCalls: []agent.LLMToolCall{{
				Name:      "scope10_sched_echo",
				Arguments: json.RawMessage(`{"q":"sched"}`),
			}},
			Provider: "scope10", Model: "scope10-fake",
		},
		{
			Final:    json.RawMessage(`{"answer":"sched-ok"}`),
			Provider: "scope10", Model: "scope10-fake",
		},
	}
	bridge := buildBridgeScope10(t, pool, nc, sc, turns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// G1: invoke via the scheduler call site (NOT directly via Bridge).
	res, decision := scheduler.FireScenario(ctx, bridge, sc.ID, []byte(`{"q":"sched"}`))
	if res == nil {
		t.Fatal("G1: scheduler.FireScenario returned nil result")
	}
	cleanupTrace(t, pool, res.TraceID)

	// G2: outcome is ok.
	if res.Outcome != agent.OutcomeOK {
		t.Fatalf("G2: outcome=%s want=ok detail=%+v", res.Outcome, res.OutcomeDetail)
	}

	// G3: persisted source column is "scheduler".
	src := fetchTraceSource(t, pool, res.TraceID)
	if src != "scheduler" {
		t.Fatalf("G3: agent_traces.source=%q want=%q", src, "scheduler")
	}

	// G4: explicit-id fast path was used (no similarity work).
	if decision == nil || decision.Reason != agent.ReasonExplicitScenarioID {
		t.Fatalf("G4: reason=%v want=%s", decision, agent.ReasonExplicitScenarioID)
	}
}
