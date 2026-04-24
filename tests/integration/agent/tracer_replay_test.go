//go:build integration

// Spec 037 Scope 6 — Trace Persistence & Replay integration tests.
//
// These tests exercise the PostgresTracer + ReplayTrace machinery
// against the live test stack (real PostgreSQL, real NATS). They cover
// the three Scope-6 DoD requirements that need a running runtime to
// prove:
//
//   1. An invocation produces a persisted agent_traces row + N
//      agent_tool_calls rows containing the denormalized scenario
//      snapshot, the input envelope, the routing decision, and the
//      tool-call audit array (BS-012).
//   2. Replaying that stored trace against the same in-memory scenario
//      returns Pass=true with zero diff entries (BS-013 happy).
//   3. Mutating the in-memory scenario's content_hash AFTER recording
//      causes ReplayTrace to return Pass=false with a structured
//      scenario_content_changed diff entry (BS-013 sad).
//   4. The tracer mirrors agent.tool_call.executed and agent.complete
//      onto the AGENT NATS stream (per-call + per-invocation) so
//      downstream consumers see terminal outcomes without polling
//      Postgres.
//
// All tests skip cleanly when DATABASE_URL or NATS_URL is unset so
// `go test ./...` (no live stack) does not fail.

package agent_integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/agent"
)

// --- live stack helpers ---------------------------------------------

func livePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set")
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

func liveNATS(t *testing.T) *nats.Conn {
	t.Helper()
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		t.Skip("integration: NATS_URL not set")
	}
	opts := []nats.Option{nats.Name("smackerel-scope6-integration")}
	if tok := os.Getenv("SMACKEREL_AUTH_TOKEN"); tok != "" {
		opts = append(opts, nats.Token(tok))
	}
	nc, err := nats.Connect(natsURL, opts...)
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(nc.Close)
	return nc
}

// natsPublisher adapts *nats.Conn to the agent.TracePublisher interface
// without dragging the internal/nats client into the test (which would
// require a full agent server config). For the purposes of the trace
// mirror, a core publish is sufficient — the AGENT stream subscribes to
// `agent.>` and will pick the message up regardless of whether the
// publisher used JetStream Publish or core Publish.
type natsPublisher struct{ nc *nats.Conn }

func (p natsPublisher) Publish(_ context.Context, subject string, data []byte) error {
	return p.nc.Publish(subject, data)
}

// scriptedDriver is a deterministic LLMDriver that returns canned
// turns. Local copy because internal/agent/executor_test_helpers_test.go
// is _test.go and not exported.
type liveScriptedDriver struct {
	mu    sync.Mutex
	turns []agent.TurnResponse
	idx   int
}

func (d *liveScriptedDriver) Turn(_ context.Context, _ agent.TurnRequest) (agent.TurnResponse, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.idx >= len(d.turns) {
		return agent.TurnResponse{}, errors.New("liveScriptedDriver: exhausted")
	}
	r := d.turns[d.idx]
	d.idx++
	return r, nil
}

// makeScopeSixScenario builds a minimal in-memory scenario suitable for
// driving one happy-path invocation. The schemas are intentionally
// minimal so the scripted driver's final answer trivially validates.
func makeScopeSixScenario(t *testing.T, idSuffix string) *agent.Scenario {
	t.Helper()
	id := "scope6_" + idSuffix
	inSchema := json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`)
	outSchema := json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`)
	inC, err := agent.CompileSchema(inSchema)
	if err != nil {
		t.Fatalf("compile input schema: %v", err)
	}
	outC, err := agent.CompileSchema(outSchema)
	if err != nil {
		t.Fatalf("compile output schema: %v", err)
	}
	return agent.NewScenarioForTest(agent.ScenarioForTest{
		ID:              id,
		Version:         id + "-v1",
		SystemPrompt:    "scope6 integration scenario",
		AllowedTools:    []agent.AllowedTool{{Name: "scope6_echo", SideEffectClass: agent.SideEffectRead}},
		InputSchema:     inSchema,
		OutputSchema:    outSchema,
		InputCompiled:   inC,
		OutputCompiled:  outC,
		Limits:          agent.ScenarioLimits{MaxLoopIterations: 4, TimeoutMs: 30000, SchemaRetryBudget: 2, PerToolTimeoutMs: 5000},
		TokenBudget:     1000,
		Temperature:     0.1,
		ModelPreference: "fast",
		SideEffectClass: agent.SideEffectRead,
		ContentHash:     "scope6_hash_" + idSuffix,
		SourcePath:      "test://scope6/" + idSuffix + ".yaml",
	})
}

// registerScopeSixEcho registers a deterministic read-only echo tool.
// Idempotent — re-registration would panic per registry contract, so
// use Has to short-circuit.
func registerScopeSixEcho(t *testing.T) {
	t.Helper()
	if agent.Has("scope6_echo") {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:            "scope6_echo",
		Description:     "echo q back",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "scope6_integration_test",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

// runOneInvocation drives the executor through one tool call + final.
// Returns the trace_id of the resulting persisted trace.
func runOneInvocation(t *testing.T, pool *pgxpool.Pool, pub agent.TracePublisher, sc *agent.Scenario) (string, *agent.InvocationResult) {
	t.Helper()
	tracer, err := agent.NewPostgresTracer(pool, pub, false)
	if err != nil {
		t.Fatalf("NewPostgresTracer: %v", err)
	}
	driver := &liveScriptedDriver{turns: []agent.TurnResponse{
		{
			ToolCalls: []agent.LLMToolCall{{
				Name:      "scope6_echo",
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
	env := agent.IntentEnvelope{
		Source:            "test",
		RawInput:          "hello",
		StructuredContext: json.RawMessage(`{"q":"hello"}`),
		Routing: agent.RoutingDecision{
			Reason:    agent.ReasonExplicitScenarioID,
			Chosen:    sc.ID,
			Threshold: 0,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res := exe.Run(ctx, sc, env)
	if res == nil {
		t.Fatal("executor returned nil result")
	}
	if res.Outcome != agent.OutcomeOK {
		t.Fatalf("expected outcome ok, got %s detail=%+v", res.Outcome, res.OutcomeDetail)
	}
	return res.TraceID, res
}

// cleanupTrace deletes the trace row (cascades to agent_tool_calls).
func cleanupTrace(t *testing.T, pool *pgxpool.Pool, traceID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(ctx, "DELETE FROM agent_traces WHERE trace_id = $1", traceID); err != nil {
			t.Logf("cleanup trace %s: %v", traceID, err)
		}
	})
}

// --- tests ----------------------------------------------------------

// TestTracerPersistsTraceAndReplayPasses is the BS-012 + BS-013 happy
// adversarial regression. Records an invocation, asserts the trace row
// + tool-call rows exist with the expected denormalized snapshot, then
// replays against the same scenario and asserts Pass=true.
//
// Adversarial gates:
//
//	G1: agent_traces row exists with the executor's trace_id
//	G2: scenario_id, scenario_version, scenario_hash match what the
//	    executor saw (frozen snapshot, BS-019 mechanism)
//	G3: scenario_snapshot JSONB contains the scenario.id (proves
//	    denormalized snapshot was written, not just the FK columns)
//	G4: agent_tool_calls has one row with seq=1, tool_name=scope6_echo,
//	    side_effect_class=read
//	G5: ReplayTrace against the same in-memory scenario returns
//	    Pass=true with zero diff entries
func TestTracerPersistsTraceAndReplayPasses(t *testing.T) {
	pool := livePool(t)
	nc := liveNATS(t)

	registerScopeSixEcho(t)
	sc := makeScopeSixScenario(t, "pass")

	traceID, _ := runOneInvocation(t, pool, natsPublisher{nc: nc}, sc)
	cleanupTrace(t, pool, traceID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// G1+G2: trace row landed with the right scenario identity.
	var (
		gotID, gotVersion, gotHash string
		snapshot                   []byte
	)
	err := pool.QueryRow(ctx, `
SELECT scenario_id, scenario_version, scenario_hash, scenario_snapshot
  FROM agent_traces WHERE trace_id = $1`, traceID).
		Scan(&gotID, &gotVersion, &gotHash, &snapshot)
	if err != nil {
		t.Fatalf("G1: select agent_traces: %v", err)
	}
	if gotID != sc.ID || gotVersion != sc.Version || gotHash != sc.ContentHash {
		t.Fatalf("G2: identity mismatch: id=%s version=%s hash=%s want id=%s version=%s hash=%s",
			gotID, gotVersion, gotHash, sc.ID, sc.Version, sc.ContentHash)
	}

	// G3: snapshot is non-empty JSONB and references the scenario id.
	var snap map[string]any
	if err := json.Unmarshal(snapshot, &snap); err != nil {
		t.Fatalf("G3: snapshot is not JSON: %v raw=%s", err, snapshot)
	}
	if snap["id"] != sc.ID {
		t.Fatalf("G3: snapshot.id=%v want %s", snap["id"], sc.ID)
	}

	// G4: per-call row materialized.
	var (
		seq     int
		name    string
		sideEff string
	)
	err = pool.QueryRow(ctx, `
SELECT seq, tool_name, side_effect_class
  FROM agent_tool_calls WHERE trace_id = $1 ORDER BY seq ASC LIMIT 1`, traceID).
		Scan(&seq, &name, &sideEff)
	if err != nil {
		t.Fatalf("G4: select agent_tool_calls: %v", err)
	}
	if seq != 1 || name != "scope6_echo" || sideEff != string(agent.SideEffectRead) {
		t.Fatalf("G4: tool call row wrong: seq=%d name=%s side=%s", seq, name, sideEff)
	}

	// G5: replay against the same in-memory scenario passes.
	tr, err := agent.LoadTrace(ctx, pool, traceID)
	if err != nil {
		t.Fatalf("G5: LoadTrace: %v", err)
	}
	res := agent.ReplayTrace(tr, agent.ScenarioLookupFromSlice([]*agent.Scenario{sc}), agent.ReplayOptions{})
	if !res.Pass {
		t.Fatalf("G5: replay Pass=false; diff=%+v", res.Diff)
	}
	if len(res.Diff) != 0 {
		t.Fatalf("G5: replay diff non-empty: %+v", res.Diff)
	}
}

// TestReplayDetectsMutatedScenarioSnapshot is the BS-013 sad
// adversarial regression. Records a trace, then mutates the in-memory
// scenario's content_hash, and asserts ReplayTrace returns a structured
// scenario_content_changed diff with Pass=false.
//
// This is the exact failure replay must catch: an operator edits the
// scenario YAML between recording and replay, content_hash changes,
// and replay surfaces the drift instead of silently passing.
//
// Adversarial gates:
//
//	G1: replay returns Pass=false
//	G2: diff contains exactly one scenario_content_changed entry
//	G3: that entry's recorded/current values are the original/mutated
//	    hash respectively
//	G4: --allow-content-drift suppresses the FAIL (proves the override
//	    is not vacuous)
func TestReplayDetectsMutatedScenarioSnapshot(t *testing.T) {
	pool := livePool(t)
	nc := liveNATS(t)

	registerScopeSixEcho(t)
	sc := makeScopeSixScenario(t, "fail")
	originalHash := sc.ContentHash

	traceID, _ := runOneInvocation(t, pool, natsPublisher{nc: nc}, sc)
	cleanupTrace(t, pool, traceID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, err := agent.LoadTrace(ctx, pool, traceID)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}

	// Mutate: simulate operator editing scenario YAML AFTER recording.
	mutated := makeScopeSixScenario(t, "fail")
	mutated.ContentHash = originalHash + "_MUTATED"

	res := agent.ReplayTrace(tr, agent.ScenarioLookupFromSlice([]*agent.Scenario{mutated}), agent.ReplayOptions{})

	// G1
	if res.Pass {
		t.Fatalf("G1: replay should FAIL on mutated content_hash; got Pass=true diff=%+v", res.Diff)
	}
	// G2 + G3
	var contentDiff *agent.DiffEntry
	for i, d := range res.Diff {
		if d.Kind == agent.DiffScenarioContentChange {
			contentDiff = &res.Diff[i]
			break
		}
	}
	if contentDiff == nil {
		t.Fatalf("G2: missing scenario_content_changed diff entry; got %+v", res.Diff)
	}
	if contentDiff.Recorded != originalHash || contentDiff.Current != mutated.ContentHash {
		t.Fatalf("G3: diff endpoints wrong: recorded=%q current=%q want recorded=%q current=%q",
			contentDiff.Recorded, contentDiff.Current, originalHash, mutated.ContentHash)
	}

	// G4: allow flag suppresses the FAIL (override is not theater).
	res2 := agent.ReplayTrace(tr, agent.ScenarioLookupFromSlice([]*agent.Scenario{mutated}),
		agent.ReplayOptions{AllowContentDrift: true})
	if !res2.Pass {
		t.Fatalf("G4: --allow-content-drift should suppress FAIL; got diff=%+v", res2.Diff)
	}
}

// TestTracerMirrorsNATSEvents asserts that an executor invocation
// causes the tracer to publish to both agent.tool_call.executed and
// agent.complete on the AGENT stream. This is the BS-012 NATS-mirror
// contract from design §6.1: downstream consumers (Operator UI, future
// analytics) must see terminal outcomes without polling Postgres.
//
// Adversarial gates:
//
//	G1: at least one agent.tool_call.executed message arrives carrying
//	    this invocation's trace_id
//	G2: exactly one agent.complete message arrives carrying this
//	    invocation's trace_id with outcome=ok
//	G3: the complete event includes scenario_id and scenario_version
//	    (matches the tracer publishComplete envelope contract)
func TestTracerMirrorsNATSEvents(t *testing.T) {
	pool := livePool(t)
	nc := liveNATS(t)

	registerScopeSixEcho(t)
	sc := makeScopeSixScenario(t, "natsmirror")

	// Subscribe BEFORE running the invocation so we don't miss events.
	type capture struct {
		mu       sync.Mutex
		toolCall []map[string]any
		complete []map[string]any
	}
	c := &capture{}
	subTool, err := nc.Subscribe(agent.SubjectToolCallExecuted, func(m *nats.Msg) {
		var body map[string]any
		if err := json.Unmarshal(m.Data, &body); err != nil {
			return
		}
		c.mu.Lock()
		c.toolCall = append(c.toolCall, body)
		c.mu.Unlock()
	})
	if err != nil {
		t.Fatalf("subscribe tool_call: %v", err)
	}
	defer subTool.Unsubscribe()
	subDone, err := nc.Subscribe(agent.SubjectAgentComplete, func(m *nats.Msg) {
		var body map[string]any
		if err := json.Unmarshal(m.Data, &body); err != nil {
			return
		}
		c.mu.Lock()
		c.complete = append(c.complete, body)
		c.mu.Unlock()
	})
	if err != nil {
		t.Fatalf("subscribe complete: %v", err)
	}
	defer subDone.Unsubscribe()
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush subscribe: %v", err)
	}

	traceID, _ := runOneInvocation(t, pool, natsPublisher{nc: nc}, sc)
	cleanupTrace(t, pool, traceID)

	// Wait for events. Core publish + receive on the same connection is
	// fast; budget 5s before we declare a missed mirror.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		gotTool := containsTraceID(c.toolCall, traceID)
		gotDone := containsTraceID(c.complete, traceID)
		c.mu.Unlock()
		if gotTool && gotDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// G1
	if !containsTraceID(c.toolCall, traceID) {
		t.Fatalf("G1: no agent.tool_call.executed event for trace_id=%s; got=%s",
			traceID, summarize(c.toolCall))
	}
	// G2
	matches := filterTraceID(c.complete, traceID)
	if len(matches) != 1 {
		t.Fatalf("G2: agent.complete count for trace_id=%s = %d, want 1; got=%s",
			traceID, len(matches), summarize(c.complete))
	}
	if matches[0]["outcome"] != string(agent.OutcomeOK) {
		t.Fatalf("G2: agent.complete outcome=%v want %s", matches[0]["outcome"], agent.OutcomeOK)
	}
	// G3
	if matches[0]["scenario_id"] != sc.ID {
		t.Fatalf("G3: agent.complete scenario_id=%v want %s", matches[0]["scenario_id"], sc.ID)
	}
	if matches[0]["scenario_version"] != sc.Version {
		t.Fatalf("G3: agent.complete scenario_version=%v want %s", matches[0]["scenario_version"], sc.Version)
	}
}

func containsTraceID(events []map[string]any, traceID string) bool {
	for _, e := range events {
		if e["trace_id"] == traceID {
			return true
		}
	}
	return false
}

func filterTraceID(events []map[string]any, traceID string) []map[string]any {
	var out []map[string]any
	for _, e := range events {
		if e["trace_id"] == traceID {
			out = append(out, e)
		}
	}
	return out
}

func summarize(events []map[string]any) string {
	if len(events) == 0 {
		return "<none>"
	}
	ids := make([]string, 0, len(events))
	for _, e := range events {
		if id, ok := e["trace_id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return fmt.Sprintf("%d events trace_ids=%v", len(events), ids)
}
