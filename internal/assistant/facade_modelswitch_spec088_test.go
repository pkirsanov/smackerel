// Spec 088 SCOPE-02 — facade resolve→reject→thread→attribute spine tests.
//
// These drive the REAL open_knowledge fast-path (okagenttool.CurrentAgent()
// wired to a fake-LLM-backed agent) through Facade.Handle, proving:
//   - an off-allowlist override SHORT-CIRCUITS the fast-path: the agent is
//     NEVER called and capture-as-fallback is NOT invoked (pre-agent request
//     validation, not an agent run; design "Rejection ≠ capture-skip"), and
//     the reply carries the verbatim modelswitch.Rejection.Message under the
//     typed ErrModelNotSwitchable cause; and
//   - an applied override threads attribution to resp.ModelAttribution
//     {ModelID: turn.Model, OverrideApplied: true} via runOpenKnowledgeDirect.
//
// They wire the agenttool package singletons (SetAgent / SetSwitchableModels)
// and therefore do NOT run in parallel; both are reset on exit.
package assistant

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// fakeOKChat is a recording okagent.LLMChat. It records every request's Model
// and returns scripted results; tests assert on calls/requests.
type fakeOKChat struct {
	t         *testing.T
	responses []llm.Result
	requests  []llm.ChatRequest
}

func (f *fakeOKChat) Chat(_ context.Context, req llm.ChatRequest) (llm.Result, error) {
	f.requests = append(f.requests, req)
	i := len(f.requests) - 1
	if i >= len(f.responses) {
		f.t.Fatalf("fakeOKChat: unexpected call #%d (queue exhausted)", i+1)
	}
	return f.responses[i], nil
}

// fakeOKTool is a minimal ok.Tool returning one web snippet + source so the
// agent loop has trace sources to salvage from.
type fakeOKTool struct{}

func (fakeOKTool) Name() string                  { return "fake_ok_tool" }
func (fakeOKTool) Description() string           { return "fake tool for spec-088 facade tests" }
func (fakeOKTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (fakeOKTool) Execute(_ context.Context, _ json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: "evidence", ContentHash: "h1", SourceRef: "https://ok.test/x"}},
		Sources: []ok.Source{{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: "https://ok.test/x", ContentHash: "h1", Provider: "fake", Snippet: "evidence"},
		}},
	}, nil
}

// recordingCapturePolicy records whether the capture-as-fallback hook fired.
type recordingCapturePolicy struct {
	decideCalls  int
	captureCalls int
}

func (p *recordingCapturePolicy) Decide(_ context.Context, _ capturefallback.Request) (capturefallback.Decision, error) {
	p.decideCalls++
	return capturefallback.Decision{}, nil
}
func (p *recordingCapturePolicy) Capture(_ context.Context, dec capturefallback.Decision) (capturefallback.CaptureResult, error) {
	p.captureCalls++
	return capturefallback.CaptureResult{Decision: dec}, nil
}
func (p *recordingCapturePolicy) CaptureForUser(_ context.Context, _ string, dec capturefallback.Decision) (capturefallback.CaptureResult, error) {
	p.captureCalls++
	return capturefallback.CaptureResult{Decision: dec}, nil
}

// spec088WireAgent builds an okagent backed by the supplied fake LLM (Model=
// gather-model, SynthesisModel=synth-model), installs it + the allowlist into
// the agenttool singletons, and returns a cleanup. The allowlist offers
// override-model; defaultModel=synth-model (the baseline synthesis model).
func spec088WireAgent(t *testing.T, chat okagent.LLMChat) func() {
	t.Helper()
	reg := ok.NewRegistry([]string{"fake_ok_tool"})
	if err := reg.Register(fakeOKTool{}); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}
	a, err := okagent.New(chat, reg, citeback.Verify, okagent.Config{
		SystemPrompt:               "test-system-prompt",
		Model:                      "gather-model",
		SynthesisModel:             "synth-model",
		SynthesisRetryBudget:       0,
		MaxIterations:              2,
		PerQueryTokenBudget:        1_000_000,
		PerQueryUSDBudget:          1.0,
		MonthlyBudgetUSDRemaining:  100.0,
		PerUserMonthlyUSDRemaining: 100.0,
		CompactionThresholdRatio:   0.85,
		CostFn:                     func(int) float64 { return 0 },
		EnforcementMode:            string(citeback.EnforcementEnforce),
		SourcesMax:                 5,
	})
	if err != nil {
		t.Fatalf("okagent.New: %v", err)
	}
	allow, err := modelswitch.NewAllowlist(
		[]string{"override-model"},
		map[string]int{"override-model": 4096, "gather-model": 4096, "synth-model": 4096},
		0, // dev envelope: skip the co-residence check
		"gather-model",
		"synth-model",
		[]string{"gather-model"}, // spec 089 tool-capable gather set (baseline gather-model is a member)
	)
	if err != nil {
		t.Fatalf("NewAllowlist: %v", err)
	}
	agenttool.SetAgent(a)
	agenttool.SetSwitchableModels(allow)
	return func() {
		agenttool.SetAgent(nil)
		agenttool.SetSwitchableModels(nil)
	}
}

// spec088Facade builds a Facade routed straight to open_knowledge (high band),
// open_knowledge enabled + provenance-gate-off (so the attribution survives),
// with the supplied capture policy wired.
func spec088Facade(t *testing.T, policy capturefallback.Policy) *Facade {
	t.Helper()
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	sc := &agent.Scenario{ID: "open_knowledge"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{"open_knowledge": sc}}
	manifest := newTestManifest(map[string]manifestEntry{
		"open_knowledge": {
			UserFacingLabel:    "ask anything",
			SlashShortcut:      "/ask",
			EnableSSTKey:       "assistant.skill.open_knowledge.enabled",
			Enabled:            true,
			RequiresProvenance: false,
		},
	})
	router := &stubRouter{
		chosen: sc,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "open_knowledge", TopScore: 0.95,
			Considered: []agent.CandidateScore{{ScenarioID: "open_knowledge", Score: 0.95}},
		},
		ok: true,
	}
	f := mustFacade(cfg, router, &stubExecutor{}, registry, manifest, newMemContextStore(), &recordingAudit{})
	if policy != nil {
		f = f.WithCaptureFallbackPolicy(policy)
	}
	return f
}

// SCN-088-A05 (ADVERSARIAL) — a rejected override short-circuits the fast-path:
// the agent is NEVER called and capture-as-fallback is NOT invoked for the
// rejected request; the reply is the verbatim rejection under the typed cause.
func TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088(t *testing.T) {
	chat := &fakeOKChat{t: t} // empty queue: ANY Chat call fails the test
	cleanup := spec088WireAgent(t, chat)
	defer cleanup()
	policy := &recordingCapturePolicy{}
	f := spec088Facade(t, policy)

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-rej", Transport: "telegram",
		Text:          "compare these towns for pomegranates",
		Kind:          contracts.KindText,
		ModelOverride: "gpt-4o", // off-allowlist
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if len(chat.requests) != 0 {
		t.Fatalf("rejected override MUST NOT call the agent; got %d LLM call(s)", len(chat.requests))
	}
	if policy.decideCalls != 0 || policy.captureCalls != 0 {
		t.Fatalf("rejected override MUST NOT invoke capture-as-fallback; decide=%d capture=%d", policy.decideCalls, policy.captureCalls)
	}
	if resp.ErrorCause != contracts.ErrModelNotSwitchable {
		t.Fatalf("rejection ErrorCause = %q, want %q", resp.ErrorCause, contracts.ErrModelNotSwitchable)
	}
	// The reply body is the verbatim modelswitch rejection sentence.
	if wantSub := "is not a switchable model"; !strings.Contains(resp.Body, wantSub) {
		t.Fatalf("rejection body must be the fail-loud rejection message, got %q", resp.Body)
	}
	if !strings.Contains(resp.Body, "gpt-4o") {
		t.Fatalf("rejection body must name the rejected model, got %q", resp.Body)
	}
	// SCN-088-A06 parity (Telegram half) — the body is BYTE-IDENTICAL to the
	// shared validator's Resolve(...).Message, not merely a superstring. The
	// HTTP half (internal/api: TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088)
	// pins the SAME Resolve(...).Message on the web envelope, so both surfaces
	// emit one validator's exact rejection. Reconstruct the allowlist
	// spec088WireAgent installed (BodyMaxChars=1000 > the ~230-char sentence, so
	// truncateBody is a no-op here and a future cap below the message length
	// would correctly trip this parity guard).
	parityAllow, perr := modelswitch.NewAllowlist(
		[]string{"override-model"},
		map[string]int{"override-model": 4096, "gather-model": 4096, "synth-model": 4096},
		0,
		"gather-model",
		"synth-model",
		[]string{"gather-model"}, // spec 089 tool-capable gather set
	)
	if perr != nil {
		t.Fatalf("parity allowlist build: %v", perr)
	}
	if _, wantRej := parityAllow.Resolve("gpt-4o"); wantRej == nil {
		t.Fatalf("parity allowlist must reject gpt-4o")
	} else if resp.Body != wantRej.Message {
		t.Fatalf("Telegram rejection body MUST be byte-identical to the shared validator.\n got: %q\nwant: %q", resp.Body, wantRej.Message)
	}
	if resp.ModelAttribution != nil {
		t.Fatalf("a rejection MUST NOT carry a model attribution footer, got %+v", resp.ModelAttribution)
	}
}

// SCN-088-A01 — an applied (allowlisted) override threads through
// runOpenKnowledgeDirect and stamps resp.ModelAttribution with the model that
// produced the final text and OverrideApplied=true.
func TestFacade_AppliedOverride_ThreadsAttribution_Spec088(t *testing.T) {
	// iter0 tool call (gather-model), iter1 forced-final synthesis (override).
	chat := &fakeOKChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_ok_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: "A grounded comparison verdict.", TokensUsed: 80},
	}}
	cleanup := spec088WireAgent(t, chat)
	defer cleanup()
	f := spec088Facade(t, nil)

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-ok", Transport: "telegram",
		Text:          "compare these towns for pomegranates",
		Kind:          contracts.KindText,
		ModelOverride: "override-model", // on-allowlist
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.ModelAttribution == nil {
		t.Fatalf("an applied override MUST stamp resp.ModelAttribution")
	}
	if !resp.ModelAttribution.OverrideApplied {
		t.Fatalf("OverrideApplied MUST be true for an applied override")
	}
	// The forced-final synthesis turn produced the text under the override, so
	// the attributed model is the overridden synthesis model.
	if resp.ModelAttribution.ModelID != "override-model" {
		t.Fatalf("ModelAttribution.ModelID = %q, want override-model (synthesis turn ran the override)", resp.ModelAttribution.ModelID)
	}
	// The synthesis turn (request #2) used the override; the gather turn kept
	// the baseline tool model.
	if len(chat.requests) < 2 {
		t.Fatalf("expected ≥2 LLM calls (tool + forced-final), got %d", len(chat.requests))
	}
	if chat.requests[0].Model != "gather-model" {
		t.Fatalf("gather turn model = %q, want gather-model", chat.requests[0].Model)
	}
	if chat.requests[1].Model != "override-model" {
		t.Fatalf("forced-final synthesis model = %q, want override-model", chat.requests[1].Model)
	}
}

// SCN-088-A03 — a baseline /ask (no override) is NOT rejected and carries an
// attribution with OverrideApplied=false (no Telegram footer; the renderer
// shows the footer only-on-override).
func TestFacade_NoOverride_BaselineAttributionNotApplied_Spec088(t *testing.T) {
	chat := &fakeOKChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_ok_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: "A baseline grounded verdict.", TokensUsed: 80},
	}}
	cleanup := spec088WireAgent(t, chat)
	defer cleanup()
	f := spec088Facade(t, nil)

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-base", Transport: "telegram",
		Text: "compare these towns for pomegranates",
		Kind: contracts.KindText,
		// no ModelOverride
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.ErrorCause == contracts.ErrModelNotSwitchable {
		t.Fatalf("a baseline /ask MUST NOT be rejected")
	}
	if resp.ModelAttribution == nil {
		t.Fatalf("open_knowledge answer should carry an attribution (HTTP-always; footer gated on OverrideApplied)")
	}
	if resp.ModelAttribution.OverrideApplied {
		t.Fatalf("baseline (no override) MUST have OverrideApplied=false (no Telegram footer, NFR-4)")
	}
	// Baseline synthesis turn used the SST synthesis model.
	if chat.requests[1].Model != "synth-model" {
		t.Fatalf("baseline forced-final model = %q, want synth-model", chat.requests[1].Model)
	}
}

// D02-T2-6 — the facade override-resolve is nil-safe: with the agent wired but
// the allowlist singleton NOT installed (SCOPE-02 before SCOPE-03 wiring, or
// open_knowledge disabled), an override yields baseline passthrough — never a
// panic, never a rejection. This proves SCOPE-02 is independently non-breaking.
func TestFacade_NilAllowlist_BaselinePassthrough_NoPanic_Spec088(t *testing.T) {
	chat := &fakeOKChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_ok_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: "A baseline verdict.", TokensUsed: 80},
	}}
	// Wire ONLY the agent; leave the allowlist singleton nil.
	cleanup := spec088WireAgent(t, chat)
	defer cleanup()
	agenttool.SetSwitchableModels(nil) // override the helper's allowlist install

	f := spec088Facade(t, nil)
	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-nil", Transport: "telegram",
		Text:          "compare these towns for pomegranates",
		Kind:          contracts.KindText,
		ModelOverride: "override-model", // ignored: no allowlist wired
	})
	if err != nil {
		t.Fatalf("Handle err = %v (nil allowlist must not error)", err)
	}
	if resp.ErrorCause == contracts.ErrModelNotSwitchable {
		t.Fatalf("nil allowlist MUST NOT reject; got a model-switch rejection")
	}
	if len(chat.requests) == 0 {
		t.Fatalf("nil allowlist MUST fall through to the baseline agent run; agent was not called")
	}
	// Baseline: even though an override string was supplied, with no allowlist
	// the synthesis turn keeps the SST synthesis model.
	if chat.requests[1].Model != "synth-model" {
		t.Fatalf("nil-allowlist baseline forced-final model = %q, want synth-model", chat.requests[1].Model)
	}
}
