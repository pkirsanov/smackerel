//go:build stress

// Spec 037 Scope 7 — BS-018 concurrent invocation isolation stress test.
//
// Drives 200 parallel agent.Executor.Run invocations across 4
// distinct scenarios against the live test stack and asserts:
//
//   G1: every invocation returns OutcomeOK
//   G2: every persisted agent_traces row contains ONLY its own
//       trace_id's tool calls — no cross-trace leakage of (trace_id,
//       seq) pairs in agent_tool_calls.
//   G3: each per-invocation tracer pad held its own turn_messages
//       slice (proven implicitly by per-invocation distinct
//       tool_call.arguments which include the trace_id as a payload).
//   G4: per-invocation latency p50/p99 are reported in the test log
//       so a regression toward serialization is visible.
//
// The harness skips cleanly if DATABASE_URL or NATS_URL is unset so
// `go test -tags=stress ./...` outside the live stack does not fail.

package agent_stress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/agent"
)

const stressToolName = "scope7_stress_echo"

func livePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("stress: DATABASE_URL not set — live stack not available")
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

func liveNATS(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("stress: NATS_URL not set — live stack not available")
	}
	opts := []nats.Option{nats.Name("smackerel-scope7-stress")}
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

type natsPublisher struct{ nc *nats.Conn }

func (p natsPublisher) Publish(_ context.Context, subject string, data []byte) error {
	return p.nc.Publish(subject, data)
}

// stressDriver is a deterministic LLMDriver that always emits one
// echo tool call carrying the trace's marker as the tool argument,
// then a final answer. Per-invocation isolation is what we are
// proving; per-call payload uniqueness is the audit signal.
type stressDriver struct {
	marker string
}

func (d *stressDriver) Turn(_ context.Context, req agent.TurnRequest) (agent.TurnResponse, error) {
	// Determine which step we're on by counting prior assistant turns
	// in the conversation (each assistant turn counts as one previous
	// LLM response). Iter 1 → tool call. Iter 2 → final.
	var assistant int
	for _, m := range req.TurnMessages {
		if m.Role == agent.RoleAssistant {
			assistant++
		}
	}
	switch assistant {
	case 0:
		args, err := json.Marshal(map[string]string{"q": d.marker})
		if err != nil {
			return agent.TurnResponse{}, err
		}
		return agent.TurnResponse{
			ToolCalls: []agent.LLMToolCall{{Name: stressToolName, Arguments: args}},
			Provider:  "test", Model: "stress",
		}, nil
	case 1:
		final, _ := json.Marshal(map[string]string{"answer": d.marker})
		return agent.TurnResponse{
			Final:    final,
			Provider: "test", Model: "stress",
		}, nil
	default:
		return agent.TurnResponse{}, errors.New("stressDriver: unexpected turn")
	}
}

func registerStressEcho(t *testing.T) {
	t.Helper()
	if agent.Has(stressToolName) {
		return
	}
	agent.RegisterTool(agent.Tool{
		Name:            stressToolName,
		Description:     "stress echo q back",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "scope7_stress_test",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

func makeStressScenario(t *testing.T, idSuffix string) *agent.Scenario {
	t.Helper()
	id := "scope7_stress_" + idSuffix
	inSchema := json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`)
	outSchema := json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`)
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
		SystemPrompt:    "scope7 stress",
		AllowedTools:    []agent.AllowedTool{{Name: stressToolName, SideEffectClass: agent.SideEffectRead}},
		InputSchema:     inSchema,
		OutputSchema:    outSchema,
		InputCompiled:   inC,
		OutputCompiled:  outC,
		Limits:          agent.ScenarioLimits{MaxLoopIterations: 4, TimeoutMs: 30000, SchemaRetryBudget: 2, PerToolTimeoutMs: 5000},
		TokenBudget:     1000,
		Temperature:     0.1,
		ModelPreference: "fast",
		SideEffectClass: agent.SideEffectRead,
		ContentHash:     "scope7_stress_hash_" + idSuffix,
		SourcePath:      "test://scope7/stress/" + idSuffix + ".yaml",
	})
}

// TestConcurrentInvocationIsolation_BS018 is the BS-018 stress proof.
func TestConcurrentInvocationIsolation_BS018(t *testing.T) {
	const (
		totalInvocations = 200
		scenarioCount    = 4
	)

	pool := livePool(t)
	nc := liveNATS(t)
	registerStressEcho(t)

	scenarios := make([]*agent.Scenario, scenarioCount)
	for i := 0; i < scenarioCount; i++ {
		scenarios[i] = makeStressScenario(t, fmt.Sprintf("s%d", i))
	}

	tracer, err := agent.NewPostgresTracer(pool, natsPublisher{nc: nc}, false)
	if err != nil {
		t.Fatalf("NewPostgresTracer: %v", err)
	}

	type invocation struct {
		marker  string
		traceID string
		latency time.Duration
		err     error
	}
	results := make([]invocation, totalInvocations)
	var wg sync.WaitGroup
	wg.Add(totalInvocations)

	var failures atomic.Int32
	t0 := time.Now()
	for i := 0; i < totalInvocations; i++ {
		go func(idx int) {
			defer wg.Done()
			marker := fmt.Sprintf("inv-%04d-%d", idx, time.Now().UnixNano())
			driver := &stressDriver{marker: marker}
			exe, err := agent.NewExecutor(driver, tracer)
			if err != nil {
				results[idx] = invocation{marker: marker, err: err}
				failures.Add(1)
				return
			}
			sc := scenarios[idx%scenarioCount]
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			start := time.Now()
			res := exe.Run(ctx, sc, agent.IntentEnvelope{
				Source:            "stress",
				RawInput:          marker,
				StructuredContext: json.RawMessage(fmt.Sprintf(`{"q":%q}`, marker)),
				Routing:           agent.RoutingDecision{Reason: agent.ReasonExplicitScenarioID, Chosen: sc.ID},
			})
			latency := time.Since(start)
			if res == nil || res.Outcome != agent.OutcomeOK {
				failures.Add(1)
				results[idx] = invocation{marker: marker, err: fmt.Errorf("outcome=%v detail=%+v", outcomeOf(res), detailOf(res)), latency: latency}
				return
			}
			results[idx] = invocation{marker: marker, traceID: res.TraceID, latency: latency}
		}(i)
	}
	wg.Wait()
	t.Logf("BS-018: ran %d concurrent invocations in %s", totalInvocations, time.Since(t0))

	// Cleanup all persisted rows at the end of the test.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, r := range results {
			if r.traceID == "" {
				continue
			}
			_, _ = pool.Exec(ctx, "DELETE FROM agent_traces WHERE trace_id = $1", r.traceID)
		}
	})

	// G1: every invocation succeeded.
	if n := failures.Load(); n != 0 {
		var sample string
		for _, r := range results {
			if r.err != nil {
				sample = r.err.Error()
				break
			}
		}
		t.Fatalf("G1: %d/%d invocations failed; first error: %s", n, totalInvocations, sample)
	}

	// G2: per-trace tool-call isolation. For each invocation, SELECT
	// agent_tool_calls.arguments and verify the q payload matches that
	// invocation's marker — proving the per-row insert routed to the
	// correct (trace_id, seq) and no other invocation's args bled in.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, r := range results {
		var (
			rows  int
			argsB []byte
		)
		err := pool.QueryRow(ctx, `
SELECT count(*)::int, COALESCE(max(arguments::text), '{}')::bytea
  FROM agent_tool_calls WHERE trace_id = $1`, r.traceID).
			Scan(&rows, &argsB)
		if err != nil {
			t.Fatalf("G2: query trace %s: %v", r.traceID, err)
		}
		if rows != 1 {
			t.Fatalf("G2: trace %s has %d tool-call rows, want 1", r.traceID, rows)
		}
		var args map[string]any
		if err := json.Unmarshal(argsB, &args); err != nil {
			t.Fatalf("G2: trace %s args not JSON: %v (raw=%s)", r.traceID, err, argsB)
		}
		if args["q"] != r.marker {
			t.Fatalf("G2: trace %s args.q=%v want %s — cross-invocation leakage", r.traceID, args["q"], r.marker)
		}
	}

	// G4: latency report. Sort and print p50, p99.
	lats := make([]time.Duration, 0, len(results))
	for _, r := range results {
		lats = append(lats, r.latency)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	p50 := lats[len(lats)*50/100]
	p99 := lats[len(lats)*99/100]
	t.Logf("BS-018 latency p50=%s p99=%s max=%s", p50, p99, lats[len(lats)-1])
}

func outcomeOf(r *agent.InvocationResult) any {
	if r == nil {
		return "<nil>"
	}
	return r.Outcome
}
func detailOf(r *agent.InvocationResult) any {
	if r == nil {
		return nil
	}
	return r.OutcomeDetail
}
