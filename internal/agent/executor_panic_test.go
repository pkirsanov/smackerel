package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// panicSpyTracer embeds NopTracer and records only the two hooks the
// panic-containment regression asserts on: RecordToolError (the contained
// panic must be reported per-call) and RecordOutcome (the trace must
// still be flushed so a tool panic cannot orphan a half-published trace).
type panicSpyTracer struct {
	NopTracer
	mu             sync.Mutex
	outcomeCount   int
	toolErrorCount int
	lastOutcome    *InvocationResult
}

func (s *panicSpyTracer) RecordToolError(_ string, _ ExecutedToolCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolErrorCount++
}

func (s *panicSpyTracer) RecordOutcome(_ string, result *InvocationResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomeCount++
	s.lastOutcome = result
}

func (s *panicSpyTracer) snapshot() (outcomes, toolErrors int, last *InvocationResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcomeCount, s.toolErrorCount, s.lastOutcome
}

// registerPanickingTool registers a read-only tool whose handler panics
// on every call — the chaos analogue of a foreseeable runtime panic in a
// first-party tool (e.g. the notification tool's crypto/rand.Read
// failure path, which panics on entropy exhaustion).
func registerPanickingTool(t *testing.T, name string, panicVal any) {
	t.Helper()
	RegisterTool(Tool{
		Name:            name,
		Description:     "panics on call",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "executor_test",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			panic(panicVal)
		},
	})
}

// TestExecutor_ToolPanicContained_LoopRecovers is the Round 19 chaos
// regression for the tool-handler panic-containment gap (resilience
// dimension; NOT covered by the Round 8 security pass).
//
// A tool handler that PANICS (not merely returns an error) MUST NOT
// escape Executor.Run. Before containment the panic unwound through
// Bridge.Invoke into the scheduler/telegram/NATS goroutine and crashed
// the entire Go core process (those call sites have no net/http recover);
// on the API path it leaked the tracer pad and orphaned the
// half-published trace. The executor MUST instead record a per-call
// tool-error with reason "tool_panic" and CONTINUE the §5.1 loop so the
// LLM can recover — identical recovery semantics to BS-015.
//
// Adversarial property: with the recover removed from invokeToolHandler,
// this test does not assert a merely-wrong value — the test goroutine
// itself panics and FAILS. The pinned assertions additionally reject a
// silent swallow (treating the panic as outcome ok with no record).
func TestExecutor_ToolPanicContained_LoopRecovers(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerPanickingTool(t, "boom", "crypto/rand.Read: EOF (entropy exhausted)")
	registerEchoTool(t, "echo")

	tracer := &panicSpyTracer{}

	driver := newScriptedDriver(
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "boom", Arguments: json.RawMessage(`{}`)}},
		}},
		turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "echo", Arguments: jsonObj(t, map[string]string{"q": "after panic"})}},
		}},
		turnReplyOrError{resp: TurnResponse{
			Final: json.RawMessage(`{"answer":"recovered after tool panic"}`),
		}},
	)

	sc := makeExecutorScenario(t, []AllowedTool{
		{Name: "boom", SideEffectClass: SideEffectRead},
		{Name: "echo", SideEffectClass: SideEffectRead},
	}, defaultLimits())

	exe, err := NewExecutor(driver, tracer)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	// Must RETURN — a propagated panic fails the test here (the goroutine
	// unwinds) before any assertion runs. This is the core regression.
	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	if res.Outcome != OutcomeOK {
		t.Fatalf("outcome = %s, want ok (loop recovers after contained panic); detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if len(res.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool-call records (panic + recovery), got %d: %+v", len(res.ToolCalls), res.ToolCalls)
	}

	first := res.ToolCalls[0]
	if first.Outcome != OutcomeToolError {
		t.Fatalf("panicking call outcome = %s, want tool-error", first.Outcome)
	}
	if first.RejectionReason != "tool_panic" {
		t.Fatalf("panicking call rejection_reason = %q, want tool_panic (distinct from a returned tool_error)", first.RejectionReason)
	}
	if !strings.Contains(first.Error, "crypto/rand.Read: EOF") {
		t.Fatalf("panic value not captured in trace Error: %q", first.Error)
	}
	if res.ToolCalls[1].Outcome != OutcomeOK {
		t.Fatalf("recovery call should be ok, got %+v", res.ToolCalls[1])
	}

	// Auditability under panic: finalize MUST run exactly once so the
	// trace is flushed (PostgresTracer would delete the pad + INSERT the
	// row). A half-published trace with no terminal flush is exactly the
	// corruption this gate prevents.
	outcomes, toolErrors, last := tracer.snapshot()
	if outcomes != 1 {
		t.Fatalf("RecordOutcome fired %d times, want exactly 1 (trace must flush even after a tool panic)", outcomes)
	}
	if toolErrors != 1 {
		t.Fatalf("RecordToolError fired %d times, want exactly 1 (the contained panic)", toolErrors)
	}
	if last == nil || last.Outcome != OutcomeOK {
		t.Fatalf("flushed outcome = %+v, want terminal ok", last)
	}
}

// TestExecutor_ToolPanicDeterministic_HitsLoopLimit proves the bounded
// loop caps a deterministically-panicking tool: every turn the LLM
// re-calls the panicking tool, every call is contained as tool_panic,
// and the invocation terminates at the loop limit instead of crashing
// the process or spinning forever.
func TestExecutor_ToolPanicDeterministic_HitsLoopLimit(t *testing.T) {
	resetRegistryForTest()
	defer resetRegistryForTest()
	registerPanickingTool(t, "boom", "always down")

	limits := defaultLimits()
	limits.MaxLoopIterations = 3

	// The LLM keeps calling the panicking tool on every turn.
	replies := make([]turnReplyOrError, 0, limits.MaxLoopIterations)
	for i := 0; i < limits.MaxLoopIterations; i++ {
		replies = append(replies, turnReplyOrError{resp: TurnResponse{
			ToolCalls: []LLMToolCall{{Name: "boom", Arguments: json.RawMessage(`{}`)}},
		}})
	}
	driver := newScriptedDriver(replies...)

	sc := makeExecutorScenario(t, []AllowedTool{
		{Name: "boom", SideEffectClass: SideEffectRead},
	}, limits)
	exe := newTestExecutor(t, driver)

	res := exe.Run(context.Background(), sc, envFromInput(validInput()))

	if res.Outcome != OutcomeLoopLimit {
		t.Fatalf("outcome = %s, want loop-limit for a deterministically-panicking tool; detail=%v", res.Outcome, res.OutcomeDetail)
	}
	if len(res.ToolCalls) != limits.MaxLoopIterations {
		t.Fatalf("expected %d contained tool-panic records, got %d", limits.MaxLoopIterations, len(res.ToolCalls))
	}
	for i, c := range res.ToolCalls {
		if c.Outcome != OutcomeToolError || c.RejectionReason != "tool_panic" {
			t.Fatalf("call %d = %+v, want tool-error/tool_panic", i, c)
		}
	}
}
