//go:build integration

// Spec 068 Scope 2 — trace sequence for read flows.
//
// Asserts the canonical sequence design §"Observability" specifies for
// a routed read turn:
//
//   raw_turn_received -> intent_compiled -> intent_validated ->
//   route_selected -> tool_or_action_executed -> response_synthesized
//
// We verify the in-process order by recording call timestamps from
// the stub Transport (compile), stub Router (route_selected), and
// stub Executor (tool_or_action_executed) and asserting strict
// ordering. raw_turn_received and response_synthesized are implicit
// in the facade entry/exit and are bracketed by the assertion that
// Handle returned a non-error response.

package assistant_integration

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
)

type traceRecorder struct {
	mu  sync.Mutex
	seq []string
}

func (r *traceRecorder) record(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq = append(r.seq, name)
}

func (r *traceRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.seq))
	copy(out, r.seq)
	return out
}

type traceTransport struct {
	rec  *traceRecorder
	body string
}

func (t *traceTransport) Compile(_ context.Context, _ intent.CompileRequest) (intent.CompileResponse, error) {
	t.rec.record("intent_compiled")
	return intent.CompileResponse{
		SchemaVersion:  "v1",
		CompiledIntent: json.RawMessage(t.body),
		Provider:       "stub",
		Model:          "stub",
	}, nil
}

type traceRouter struct {
	rec      *traceRecorder
	chosen   *agent.Scenario
	envelope atomic.Pointer[agent.IntentEnvelope]
}

func (r *traceRouter) Route(_ context.Context, env agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	r.rec.record("route_selected")
	envCopy := env
	r.envelope.Store(&envCopy)
	return r.chosen, agent.RoutingDecision{
		Reason:    agent.ReasonExplicitScenarioID,
		Chosen:    r.chosen.ID,
		TopScore:  1.0,
		Threshold: 0.5,
	}, true
}

type traceExecutor struct {
	rec *traceRecorder
}

func (e *traceExecutor) Run(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
	e.rec.record("tool_or_action_executed")
	return &agent.InvocationResult{
		TraceID:    "trace-068-scope2",
		ScenarioID: sc.ID,
		Outcome:    agent.OutcomeOK,
		Final:      []byte(`"ok"`),
		StartedAt:  time.Unix(0, 0),
		EndedAt:    time.Unix(0, 0),
	}
}

// TestIntentTraceRecordsCompileValidateRouteToolResponseSequence drives
// a weather read turn through the in-process facade and asserts the
// recorded call order is: intent_compiled, route_selected,
// tool_or_action_executed. raw_turn_received is implicit at Handle
// entry; response_synthesized is implicit at Handle exit (verified by
// the non-error return + non-empty response body).
func TestIntentTraceRecordsCompileValidateRouteToolResponseSequence(t *testing.T) {
	rec := &traceRecorder{}
	transport := &traceTransport{rec: rec, body: weatherIntentJSON(t)}
	compiler := buildCompiler(t, transport)

	sc := &agent.Scenario{ID: "weather_query", Version: "v1"}
	router := &traceRouter{rec: rec, chosen: sc}
	executor := &traceExecutor{rec: rec}

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	cfg := assistant.FacadeConfig{
		BorderlineFloor:      0.75,
		AgentConfidenceFloor: 0.50,
		SourcesMax:           5,
		BodyMaxChars:         1000,
		WindowTurns:          5,
		DisambigMaxChoices:   3,
		DisambigTimeout:      30 * time.Second,
		Now:                  func() time.Time { return now },
	}
	manifest, err := assistant.NewManifestForTest(map[string]assistant.ManifestEntryForTest{
		"weather_query": {UserFacingLabel: "weather", EnableSSTKey: "assistant.skills.weather_query.enabled", Enabled: true},
	})
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"weather_query": sc})
	f, err := assistant.NewFacade(cfg, router, executor, registry, manifest,
		assistant.NewInMemoryContextStore(), assistant.NewRecordingAudit())
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	f.WithIntentCompiler(compiler)

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-trace",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "weather in palm springs ca tomorrow",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Body == "" {
		t.Fatal("Handle returned empty body — response_synthesized step did not produce output")
	}

	got := rec.snapshot()
	want := []string{"intent_compiled", "route_selected", "tool_or_action_executed"}
	if len(got) != len(want) {
		t.Fatalf("trace sequence length = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("trace sequence[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
		}
	}

	// Sanity: the envelope the router received MUST carry the
	// compiled_intent payload (intent_validated happened before
	// route_selected — schema validation runs inside Compiler.Compile
	// and the resulting CompiledIntent is what we marshal into
	// StructuredContext).
	envPtr := router.envelope.Load()
	if envPtr == nil || len(envPtr.StructuredContext) == 0 {
		t.Fatal("router envelope StructuredContext is empty; intent_validated did not feed route_selected")
	}
	var payload map[string]any
	if err := json.Unmarshal(envPtr.StructuredContext, &payload); err != nil {
		t.Fatalf("StructuredContext JSON: %v", err)
	}
	if _, ok := payload["compiled_intent"]; !ok {
		t.Fatalf("router envelope missing compiled_intent: %v", payload)
	}
}

// TestIntentTraceDistinguishesClarifyFailureAndOperationalBypass
// SCN-068-A05, SCN-068-A08 (trace shape).
//
// The CompilerTrace contract must distinguish four observable states
// so the trace inspector (and the bypass-guard fixtures) can tell
// them apart:
//
//   1. Compiled success with action_class=clarify  → OutcomeCompiled
//      AND Compiled.ActionClass == ActionClarify.
//   2. Compiler failure (provider error)           → OutcomeProviderError
//      AND non-empty ErrorCause; no Compiled set.
//   3. Operational-command bypass                  → OutcomeBypass
//      AND Bypass.Label == BypassTraceLabel.
//   4. (Implicit baseline) compiled actionable     → OutcomeCompiled
//      AND ActionClass != ActionClarify; covered by the read-flow
//      test above; we include a single positive assert here as the
//      adversarial baseline so the four states cannot collapse.
func TestIntentTraceDistinguishesClarifyFailureAndOperationalBypass(t *testing.T) {
	// 1. Clarify.
	clarifyTransport := &stubTransport{resolve: func(_ string) string { return springfieldClarifyIntentJSON(t) }}
	clarifyCompiler := buildCompiler(t, clarifyTransport)
	clarifyCI, clarifyTrace, err := clarifyCompiler.Compile(context.Background(), intent.RawTurn{
		UserID: "u-trace-clarify", Transport: "telegram", Text: "springfield weather tomorrow",
	})
	if err != nil {
		t.Fatalf("clarify Compile err: %v", err)
	}
	if clarifyTrace.Outcome != intent.OutcomeCompiled {
		t.Fatalf("clarify trace.Outcome = %q, want %q", clarifyTrace.Outcome, intent.OutcomeCompiled)
	}
	if clarifyCI.ActionClass != intent.ActionClarify {
		t.Fatalf("clarify compiled.ActionClass = %q, want %q", clarifyCI.ActionClass, intent.ActionClarify)
	}
	if clarifyTrace.Compiled == nil || clarifyTrace.Compiled.ActionClass != intent.ActionClarify {
		t.Fatalf("clarify trace.Compiled missing or wrong ActionClass: %+v", clarifyTrace.Compiled)
	}
	if clarifyTrace.Bypass != nil {
		t.Fatalf("clarify trace.Bypass = %+v, want nil", clarifyTrace.Bypass)
	}

	// 2. Compiler failure (transport error).
	errTransport := &errorTransport{}
	errCompiler := buildCompiler(t, errTransport)
	_, errTrace, errErr := errCompiler.Compile(context.Background(), intent.RawTurn{
		UserID: "u-trace-err", Transport: "telegram", Text: "anything",
	})
	if errErr == nil {
		t.Fatalf("expected compiler error, got nil")
	}
	if errTrace.Outcome != intent.OutcomeProviderError {
		t.Fatalf("error trace.Outcome = %q, want %q", errTrace.Outcome, intent.OutcomeProviderError)
	}
	if errTrace.ErrorCause == "" {
		t.Fatalf("error trace.ErrorCause is empty")
	}
	if errTrace.Compiled != nil {
		t.Fatalf("error trace.Compiled = %+v, want nil", errTrace.Compiled)
	}

	// 3. Operational-command bypass — produced by intent.BypassTrace,
	// not by Compile (the facade detects the carve-out first and
	// records the bypass trace directly per spec 068 SCOPE-1).
	bypassTrace := intent.BypassTrace("/status now", "/status")
	if bypassTrace.Outcome != intent.OutcomeBypass {
		t.Fatalf("bypass trace.Outcome = %q, want %q", bypassTrace.Outcome, intent.OutcomeBypass)
	}
	if bypassTrace.Bypass == nil || bypassTrace.Bypass.Label != intent.BypassTraceLabel {
		t.Fatalf("bypass trace.Bypass = %+v, want Label=%q", bypassTrace.Bypass, intent.BypassTraceLabel)
	}
	if bypassTrace.Compiled != nil {
		t.Fatalf("bypass trace.Compiled = %+v, want nil", bypassTrace.Compiled)
	}

	// 4. Adversarial baseline: actionable read still produces
	// OutcomeCompiled with ActionClass != clarify, so the clarify
	// distinction in (1) cannot be a tautology.
	readTransport := &stubTransport{resolve: func(_ string) string { return weatherIntentJSON(t) }}
	readCompiler := buildCompiler(t, readTransport)
	readCI, readTrace, err := readCompiler.Compile(context.Background(), intent.RawTurn{
		UserID: "u-trace-read", Transport: "telegram", Text: "weather palm springs ca tomorrow",
	})
	if err != nil {
		t.Fatalf("read Compile err: %v", err)
	}
	if readTrace.Outcome != intent.OutcomeCompiled {
		t.Fatalf("read trace.Outcome = %q, want %q", readTrace.Outcome, intent.OutcomeCompiled)
	}
	if readCI.ActionClass == intent.ActionClarify {
		t.Fatalf("read compiled.ActionClass = %q, must NOT equal clarify (adversarial baseline)", readCI.ActionClass)
	}
}

// errorTransport always returns a transport error so the compiler
// emits OutcomeProviderError.
type errorTransport struct{}

func (e *errorTransport) Compile(_ context.Context, _ intent.CompileRequest) (intent.CompileResponse, error) {
	return intent.CompileResponse{}, errStub
}

var errStub = errStubError("simulated provider failure")

type errStubError string

func (e errStubError) Error() string { return string(e) }
