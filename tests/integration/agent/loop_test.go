//go:build integration

// Spec 037 Scope 5 integration tests: full Go executor → NATS round
// trip against a fake LLM responder subscribed to the same agent
// subject the Python sidecar uses. The Python sidecar itself is unit-
// tested in ml/tests/test_agent.py; this layer exercises the wire
// contract end-to-end (marshalling, reply_subject delivery, and
// context-bounded blocking) without requiring litellm or a real LLM
// provider.
package agent_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/agent"
)

// fakeAgentResponder subscribes to a per-test subject and replies to
// the request's reply_subject with a scripted envelope. It mirrors
// the contract ml/app/agent.handle_invoke implements but uses a
// unique subject so the live ML sidecar's canonical
// `agent.invoke.request` subscriber does not race with the fake.
type fakeAgentResponder struct {
	nc       *nats.Conn
	t        *testing.T
	subject  string
	sub      *nats.Subscription
	calls    atomic.Int64
	respond  func(req map[string]any, calls int) (envelope map[string]any, delay time.Duration)
}

func startFakeResponder(t *testing.T, nc *nats.Conn, subject string, respond func(req map[string]any, calls int) (envelope map[string]any, delay time.Duration)) *fakeAgentResponder {
	t.Helper()
	r := &fakeAgentResponder{nc: nc, t: t, subject: subject, respond: respond}
	sub, err := nc.Subscribe(subject, func(m *nats.Msg) {
		idx := int(r.calls.Add(1))
		var req map[string]any
		if err := json.Unmarshal(m.Data, &req); err != nil {
			t.Logf("fakeResponder: bad request JSON: %v", err)
			return
		}
		// Dispatch each request in its own goroutine so the BS-021
		// concurrency test can prove the timeout is per-invocation
		// (the slow goroutine sleeps; the fast goroutine replies
		// immediately).
		go func() {
			env, delay := r.respond(req, idx)
			if delay > 0 {
				time.Sleep(delay)
			}
			reply, _ := req["reply_subject"].(string)
			if reply == "" {
				t.Logf("fakeResponder: missing reply_subject in request %d", idx)
				return
			}
			body, err := json.Marshal(env)
			if err != nil {
				t.Logf("fakeResponder: marshal envelope: %v", err)
				return
			}
			if err := nc.Publish(reply, body); err != nil {
				t.Logf("fakeResponder: publish reply: %v", err)
			}
		}()
	})
	if err != nil {
		t.Fatalf("fakeResponder: subscribe: %v", err)
	}
	r.sub = sub
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return r
}

// fakeNATSConn opens a core NATS connection for the integration test.
// Skips the test if NATS_URL is unset (live stack not available).
func fakeNATSConn(t *testing.T) *nats.Conn {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("integration: NATS_URL not set — live stack not available")
	}
	opts := []nats.Option{nats.Name("agent-loop-test")}
	if tok := os.Getenv("SMACKEREL_AUTH_TOKEN"); tok != "" {
		opts = append(opts, nats.Token(tok))
	}
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		t.Fatalf("connect NATS: %v", err)
	}
	t.Cleanup(func() { nc.Close() })
	return nc
}

// makeIntegrationScenario reuses the loader to compile a real Scenario
// for the executor. We avoid touching the file system by writing a
// scratch YAML — the loader covers this in isolation; here we just
// need a valid Scenario object.
func makeIntegrationScenario(t *testing.T, allowedTool string, maxIter, timeoutMs int) *agent.Scenario {
	t.Helper()
	dir := t.TempDir()
	yaml := fmt.Sprintf(`version: "loop_int-v1"
type: "scenario"
id: "loop_int"
description: "loop integration"
intent_examples:
  - "test"
system_prompt: |
  You are an integration test agent.
allowed_tools:
  - name: "%s"
    side_effect_class: "read"
input_schema:
  type: object
  required: [input]
  properties:
    input: { type: string }
output_schema:
  type: object
  required: [answer]
  properties:
    answer: { type: string }
limits:
  max_loop_iterations: %d
  timeout_ms: %d
  schema_retry_budget: 1
  per_tool_timeout_ms: 1000
token_budget: 500
temperature: 0.0
model_preference: "fast"
side_effect_class: "read"
`, allowedTool, maxIter, timeoutMs)
	path := dir + "/loop_int.yaml"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	registered, rejected, fatal := agent.DefaultLoader().Load(dir, "")
	if fatal != nil {
		t.Fatalf("loader fatal: %v", fatal)
	}
	if len(rejected) != 0 {
		t.Fatalf("loader rejected scenario: %+v", rejected)
	}
	if len(registered) != 1 {
		t.Fatalf("expected 1 scenario, got %d", len(registered))
	}
	return registered[0]
}

// registerEcho registers a one-shot read tool used by the loop tests.
// It panics on duplicate, so we name it per-test to avoid collisions
// across `go test -count=N` runs in the same process.
func registerEcho(t *testing.T, name string) {
	t.Helper()
	agent.RegisterTool(agent.Tool{
		Name:            name,
		Description:     "echo q back",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "agent_integration",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

// TestExecutor_LoopRoundTrip_ToolCallThenFinal exercises the full Go
// executor over a real NATS connection. The fake responder issues one
// tool call followed by a final answer; the executor MUST dispatch
// the tool, accept the final, and return outcome `ok`.
func TestExecutor_LoopRoundTrip_ToolCallThenFinal(t *testing.T) {
	nc := fakeNATSConn(t)
	registerEcho(t, "echo_loop_happy")
	sc := makeIntegrationScenario(t, "echo_loop_happy", 4, 30000)

	subject := uniqueSubject("loop_happy")
	startFakeResponder(t, nc, subject, func(req map[string]any, calls int) (map[string]any, time.Duration) {
		if calls == 1 {
			return map[string]any{
				"tool_calls": []map[string]any{{"name": "echo_loop_happy", "arguments": `{"q":"x"}`}},
				"final":      nil,
				"provider":   "fake",
				"model":      "fake-1",
				"tokens":     map[string]int{"prompt": 1, "completion": 1},
			}, 0
		}
		return map[string]any{
			"tool_calls": []map[string]any{},
			"final":      `{"answer":"ok"}`,
			"provider":   "fake",
			"model":      "fake-1",
			"tokens":     map[string]int{"prompt": 1, "completion": 1},
		}, 0
	})

	driver, err := agent.NewNATSLLMDriverOnSubject(nc, subject)
	if err != nil {
		t.Fatalf("NewNATSLLMDriver: %v", err)
	}
	exe, err := agent.NewExecutor(driver, agent.NopTracer{})
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	res := exe.Run(context.Background(), sc, agent.IntentEnvelope{
		Source:            "test",
		StructuredContext: json.RawMessage(`{"input":"hi"}`),
	})
	if res.Outcome != agent.OutcomeOK {
		t.Fatalf("outcome = %s, want ok; detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if string(res.Final) != `{"answer":"ok"}` {
		t.Fatalf("final = %s, want canonical answer", string(res.Final))
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Outcome != agent.OutcomeOK {
		t.Fatalf("expected one ok tool call, got %+v", res.ToolCalls)
	}
}

// TestExecutor_BS021_LLMTimeout is the adversarial regression for
// BS-021 mandated by Scope 5's DoD. The fake responder blocks past
// the scenario's timeout_ms. The executor MUST return outcome
// `timeout` with deadline_s populated. A parallel invocation MUST
// complete normally and prove the timeout did not lock global state.
//
// Failure modes this test catches:
//
//   - Executor never enforces the per-invocation timeout (test would
//     hang; we use a hard t.Fatal inside a watchdog goroutine).
//   - Executor returns provider-error instead of timeout (the
//     ctx.Err() branch must take precedence).
//   - Executor's timeout takes a global lock so the parallel
//     invocation also stalls.
//   - Outcome envelope omits deadline_s.
func TestExecutor_BS021_LLMTimeout(t *testing.T) {
	nc := fakeNATSConn(t)
	registerEcho(t, "echo_loop_timeout")

	// Slow scenario: 1000ms timeout (loader-min). Slow responder
	// blocks for 2500ms so the timeout MUST fire first.
	slowSc := makeIntegrationScenario(t, "echo_loop_timeout", 4, 1000)

	subject := uniqueSubject("bs021")
	// The responder distinguishes slow vs fast invocations by call
	// counter — first request blocks, all subsequent reply
	// immediately.
	startFakeResponder(t, nc, subject, func(req map[string]any, calls int) (map[string]any, time.Duration) {
		// Slow path: only the first request blocks past timeout.
		if calls == 1 {
			return map[string]any{
				"tool_calls": []map[string]any{},
				"final":      `{"answer":"too late"}`,
				"provider":   "fake",
				"model":      "fake-1",
				"tokens":     map[string]int{"prompt": 1, "completion": 0},
			}, 2500 * time.Millisecond // exceeds the 1000ms scenario timeout
		}
		return map[string]any{
			"tool_calls": []map[string]any{},
			"final":      `{"answer":"on-time"}`,
			"provider":   "fake",
			"model":      "fake-1",
			"tokens":     map[string]int{"prompt": 1, "completion": 0},
		}, 0
	})

	driver, err := agent.NewNATSLLMDriverOnSubject(nc, subject)
	if err != nil {
		t.Fatalf("NewNATSLLMDriver: %v", err)
	}
	exe, err := agent.NewExecutor(driver, agent.NopTracer{})
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	type runRes struct {
		label string
		res   *agent.InvocationResult
	}
	out := make(chan runRes, 2)

	// Watchdog so a regressed timeout does NOT hang CI forever.
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Errorf("BS-021 watchdog: invocations did not finish within 15s — timeout is not being enforced")
		}
	}()

	go func() {
		r := exe.Run(context.Background(), slowSc, agent.IntentEnvelope{
			Source:            "slow",
			StructuredContext: json.RawMessage(`{"input":"slow"}`),
		})
		out <- runRes{label: "slow", res: r}
	}()
	// Stagger the second call so the first occupies the responder's
	// slow path (calls==1 branch). 50ms is enough for NATS dispatch.
	time.Sleep(50 * time.Millisecond)
	go func() {
		r := exe.Run(context.Background(), slowSc, agent.IntentEnvelope{
			Source:            "fast",
			StructuredContext: json.RawMessage(`{"input":"fast"}`),
		})
		out <- runRes{label: "fast", res: r}
	}()

	got := map[string]*agent.InvocationResult{}
	for i := 0; i < 2; i++ {
		r := <-out
		got[r.label] = r.res
	}
	close(done)

	slow := got["slow"]
	fast := got["fast"]
	if slow == nil || fast == nil {
		t.Fatalf("missing results: %+v", got)
	}

	// Gate 1 — slow invocation MUST be `timeout`.
	if slow.Outcome != agent.OutcomeTimeout {
		t.Fatalf("Gate 1: slow outcome = %s, want %s; detail=%+v", slow.Outcome, agent.OutcomeTimeout, slow.OutcomeDetail)
	}
	// Gate 2 — outcome_detail.deadline_s populated.
	if _, ok := slow.OutcomeDetail["deadline_s"]; !ok {
		t.Fatalf("Gate 2: deadline_s missing from outcome_detail: %+v", slow.OutcomeDetail)
	}
	// Gate 3 — fast invocation MUST complete normally despite slow
	// running concurrently. No global lock.
	if fast.Outcome != agent.OutcomeOK {
		t.Fatalf("Gate 3 (no global lock): fast outcome = %s, want ok; detail=%+v", fast.Outcome, fast.OutcomeDetail)
	}
	if string(fast.Final) != `{"answer":"on-time"}` {
		t.Fatalf("Gate 3: fast final = %s, want canonical answer", string(fast.Final))
	}
}

// uniqueSubject builds a per-test NATS subject under the AGENT stream
// pattern (`agent.>`) that won't collide with the live Python
// sidecar's canonical agent.invoke.request subscriber.
func uniqueSubject(label string) string {
	return fmt.Sprintf("agent.invoke.request.%s.%d", label, time.Now().UnixNano())
}
