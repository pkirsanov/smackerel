package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedDriver is a deterministic LLMDriver for executor unit tests.
// Each call to Turn returns the next response from `responses`. Once
// exhausted, it returns errExhausted so a faulty test fails loud
// instead of looping forever. A test may inject a per-call sleep or
// error via the `gate` closure to simulate provider hangs (BS-021)
// without needing real time to elapse.
type scriptedDriver struct {
	mu        sync.Mutex
	responses []turnReplyOrError
	calls     atomic.Int64
	gate      func(ctx context.Context, idx int) error // optional per-call hook
	seenReqs  []TurnRequest
}

// turnReplyOrError wraps a TurnResponse plus an optional error so the
// scripted driver can model "Turn returned (zero, err)".
type turnReplyOrError struct {
	resp TurnResponse
	err  error
}

var errExhausted = errors.New("scripted driver: out of canned responses")

func newScriptedDriver(responses ...turnReplyOrError) *scriptedDriver {
	return &scriptedDriver{responses: responses}
}

func (d *scriptedDriver) Turn(ctx context.Context, req TurnRequest) (TurnResponse, error) {
	idx := int(d.calls.Add(1)) - 1
	if d.gate != nil {
		if err := d.gate(ctx, idx); err != nil {
			d.mu.Lock()
			d.seenReqs = append(d.seenReqs, req)
			d.mu.Unlock()
			return TurnResponse{}, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seenReqs = append(d.seenReqs, req)
	if idx >= len(d.responses) {
		return TurnResponse{}, errExhausted
	}
	r := d.responses[idx]
	return r.resp, r.err
}

func (d *scriptedDriver) Calls() int { return int(d.calls.Load()) }
func (d *scriptedDriver) Requests() []TurnRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]TurnRequest, len(d.seenReqs))
	copy(out, d.seenReqs)
	return out
}

// makeExecutorScenario builds an in-memory Scenario configured for
// executor tests: input requires an `input` string, output requires an
// `answer` string, and limits permit several iterations and retries so
// the test scripts the precise budget consumed.
func makeExecutorScenario(t *testing.T, allowed []AllowedTool, limits ScenarioLimits) *Scenario {
	t.Helper()
	inSchema := json.RawMessage(`{"type":"object","required":["input"],"properties":{"input":{"type":"string"}}}`)
	outSchema := json.RawMessage(`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`)
	inC, err := CompileSchema(inSchema)
	if err != nil {
		t.Fatalf("compile input schema: %v", err)
	}
	outC, err := CompileSchema(outSchema)
	if err != nil {
		t.Fatalf("compile output schema: %v", err)
	}
	return &Scenario{
		ID:              "exec_test",
		Version:         "exec_test-v1",
		SystemPrompt:    "You are a test agent.",
		AllowedTools:    allowed,
		InputSchema:     inSchema,
		OutputSchema:    outSchema,
		Limits:          limits,
		TokenBudget:     1000,
		Temperature:     0.1,
		ModelPreference: "fast",
		SideEffectClass: SideEffectRead,
		ContentHash:     "00",
		SourcePath:      "test://exec_test.yaml",
		inputSchema:     inC,
		outputSchema:    outC,
	}
}

// defaultLimits is a generous limits block that makes single-iteration
// happy paths easy to test without bumping into edges.
func defaultLimits() ScenarioLimits {
	return ScenarioLimits{
		MaxLoopIterations: 4,
		TimeoutMs:         30000,
		SchemaRetryBudget: 2,
		PerToolTimeoutMs:  5000,
	}
}

// validInput returns a structured context that satisfies the input
// schema in makeExecutorScenario.
func validInput() json.RawMessage {
	return json.RawMessage(`{"input":"hi"}`)
}

// registerEchoTool registers a deterministic read-only tool used by
// most executor tests. The handler echoes args["q"] back as result["q"].
func registerEchoTool(t *testing.T, name string) {
	t.Helper()
	RegisterTool(Tool{
		Name:            name,
		Description:     "echo q back",
		InputSchema:     json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["q"],"properties":{"q":{"type":"string"}}}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "executor_test",
		Handler: func(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
			return args, nil
		},
	})
}

// registerErroringTool registers a read-only tool whose handler always
// returns the supplied error. Used by the BS-015 tool-error test.
func registerErroringTool(t *testing.T, name string, handlerErr error) {
	t.Helper()
	RegisterTool(Tool{
		Name:            name,
		Description:     "always errors",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "executor_test",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, handlerErr
		},
	})
}

// registerBadReturnTool registers a tool whose handler returns a value
// that violates its declared output schema.
func registerBadReturnTool(t *testing.T, name string) {
	t.Helper()
	RegisterTool(Tool{
		Name:            name,
		Description:     "returns malformed result",
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object","required":["count"],"properties":{"count":{"type":"integer"}}}`),
		SideEffectClass: SideEffectRead,
		OwningPackage:   "executor_test",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return json.RawMessage(`{"count":"many"}`), nil
		},
	})
}

// newTestExecutor builds an Executor over the scripted driver with a
// stable monotonic clock so trace_ids are stable in tests.
func newTestExecutor(t *testing.T, driver LLMDriver) *Executor {
	t.Helper()
	exe, err := NewExecutor(driver, NopTracer{})
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}
	tick := atomic.Int64{}
	base := time.Date(2026, time.April, 23, 0, 0, 0, 0, time.UTC)
	exe.SetClock(func() time.Time {
		n := tick.Add(1)
		return base.Add(time.Duration(n) * time.Millisecond)
	})
	return exe
}

// envFromInput wraps a JSON input as an IntentEnvelope.
func envFromInput(input json.RawMessage) IntentEnvelope {
	return IntentEnvelope{Source: "test", RawInput: "raw", StructuredContext: input}
}

// jsonObj returns a tool-call arguments helper.
func jsonObj(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
