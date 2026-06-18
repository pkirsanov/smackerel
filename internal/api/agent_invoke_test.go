// Spec 088 SCOPE-03 — HTTP /v1/agent/invoke per-request model-override tests.
//
// These drive the REAL open_knowledge fast-path (agenttool.CurrentAgent() wired
// to a fake-LLM-backed agent + the allowlist singleton) through
// AgentInvokeHandlerFunc, proving the web/HTTP surface validates + applies the
// override IDENTICALLY to Telegram (SCN-088-A06):
//   - a `model` field threads Resolve→WithModelOverride→Run; the 200 envelope
//     carries the answering `model`;
//   - an off-allowlist `model` ⇒ HTTP 400 with the structured rejection
//     envelope (error_code + rejected_model/allowed_models/default_model +
//     the SAME message sentence Telegram renders), and NO agent call;
//   - no `model` ⇒ 200 with the baseline `model` in the envelope.
//
// They wire the agenttool package singletons and so do NOT run in parallel.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// spec088APIChat is a recording okagent.LLMChat for the api tests.
type spec088APIChat struct {
	t         *testing.T
	responses []llm.Result
	requests  []llm.ChatRequest
}

func (f *spec088APIChat) Chat(_ context.Context, req llm.ChatRequest) (llm.Result, error) {
	f.requests = append(f.requests, req)
	i := len(f.requests) - 1
	if i >= len(f.responses) {
		f.t.Fatalf("spec088APIChat: unexpected call #%d (queue exhausted)", i+1)
	}
	return f.responses[i], nil
}

type spec088APITool struct{}

func (spec088APITool) Name() string                  { return "fake_api_tool" }
func (spec088APITool) Description() string           { return "fake tool for spec-088 api tests" }
func (spec088APITool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (spec088APITool) Execute(_ context.Context, _ json.RawMessage) (*ok.ToolResult, error) {
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: "evidence", ContentHash: "h1", SourceRef: "https://ok.test/x"}},
		Sources: []ok.Source{{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: "https://ok.test/x", ContentHash: "h1", Provider: "fake", Snippet: "evidence"},
		}},
	}, nil
}

// stubInvokeRunner satisfies AgentInvokeRunner so the handler mounts; the
// open_knowledge fast-path bypasses it entirely.
type stubInvokeRunner struct{}

func (stubInvokeRunner) Invoke(context.Context, agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision) {
	return &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: []byte(`"ok"`)}, &agent.RoutingDecision{}
}
func (stubInvokeRunner) KnownIntents() []string { return []string{"open_knowledge"} }

func spec088WireAPIAgent(t *testing.T, chat okagent.LLMChat) func() {
	t.Helper()
	reg := ok.NewRegistry([]string{"fake_api_tool"})
	if err := reg.Register(spec088APITool{}); err != nil {
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
		// Spec 096 SCOPE-05 — CostFn is now the model-aware seam.
		CostFn:          func(string, int) (float64, error) { return 0, nil },
		EnforcementMode: string(citeback.EnforcementEnforce),
		SourcesMax:      5,
	})
	if err != nil {
		t.Fatalf("okagent.New: %v", err)
	}
	allow, err := modelswitch.NewAllowlist(
		[]string{"override-model"},
		map[string]int{"override-model": 4096, "gather-model": 4096, "synth-model": 4096},
		0,
		"gather-model",
		"synth-model",
		[]string{"gather-model"}, // spec 089 tool-capable gather set
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

func spec088Invoke(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := &AgentInvokeHandler{Runner: stubInvokeRunner{}}
	req := httptest.NewRequest(http.MethodPost, "/v1/agent/invoke", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	h.AgentInvokeHandlerFunc(rec, req)
	return rec
}

func TestAgentInvoke_ModelFieldApplied_EnvelopeCarriesModel_Spec088(t *testing.T) {
	chat := &spec088APIChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_api_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: "A grounded answer.", TokensUsed: 80},
	}}
	cleanup := spec088WireAPIAgent(t, chat)
	defer cleanup()

	rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns","model":"override-model"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, rec.Body.String())
	}
	if env["model"] != "override-model" {
		t.Fatalf("success envelope model = %v, want override-model", env["model"])
	}
	// The synthesis turn (request #2) ran the override; the gather turn kept the
	// baseline tool model.
	if len(chat.requests) < 2 || chat.requests[1].Model != "override-model" {
		t.Fatalf("synthesis turn must run the override model; requests=%d", len(chat.requests))
	}
}

func TestAgentInvoke_OffAllowlistModel_Returns400RejectionEnvelope_Spec088(t *testing.T) {
	chat := &spec088APIChat{t: t} // empty queue: ANY Chat call fails the test
	cleanup := spec088WireAPIAgent(t, chat)
	defer cleanup()

	rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns","model":"gpt-4o"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if len(chat.requests) != 0 {
		t.Fatalf("a rejected model MUST NOT reach the agent; got %d LLM call(s)", len(chat.requests))
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode rejection: %v; body=%s", err, rec.Body.String())
	}
	if env["status"] != "rejected" {
		t.Fatalf("status field = %v, want rejected", env["status"])
	}
	if env["error_code"] != modelswitch.ReasonNotAllowlisted {
		t.Fatalf("error_code = %v, want %q", env["error_code"], modelswitch.ReasonNotAllowlisted)
	}
	if env["rejected_model"] != "gpt-4o" {
		t.Fatalf("rejected_model = %v, want gpt-4o", env["rejected_model"])
	}
	if _, ok := env["allowed_models"]; !ok {
		t.Fatalf("rejection envelope MUST carry allowed_models, got %v", env)
	}
	if env["default_model"] != "synth-model" {
		t.Fatalf("default_model = %v, want synth-model (baseline synthesis model)", env["default_model"])
	}
	msg, _ := env["message"].(string)
	if msg == "" || !bytes.Contains([]byte(msg), []byte("is not a switchable model")) {
		t.Fatalf("message must be the verbatim rejection sentence, got %q", msg)
	}
}

func TestAgentInvoke_NoModel_EnvelopeModelPresent_Spec088(t *testing.T) {
	chat := &spec088APIChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_api_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: "A baseline grounded answer.", TokensUsed: 80},
	}}
	cleanup := spec088WireAPIAgent(t, chat)
	defer cleanup()

	rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, rec.Body.String())
	}
	// Baseline: the envelope still reports the resolved synthesis model.
	if env["model"] != "synth-model" {
		t.Fatalf("baseline envelope model = %v, want synth-model (structured metadata always present)", env["model"])
	}
	if chat.requests[1].Model != "synth-model" {
		t.Fatalf("baseline synthesis turn model = %q, want synth-model", chat.requests[1].Model)
	}
}

// TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088 — ADVERSARIAL
// (SCN-088-A06 parity, web half). The HTTP 400 rejection envelope is the SHARED
// modelswitch validator's output rendered VERBATIM: message, error_code,
// rejected_model, default_model, and allowed_models are byte-identical to
// modelswitch.Resolve(allow, raw) for the SAME allowlist the singleton is wired
// from. The Telegram half
// (internal/assistant: TestFacade_OffAllowlistOverride_ShortCircuits_NoAgentCall_NoCapture_Spec088)
// pins resp.Body to the SAME Resolve(...).Message, so an off-allowlist override
// yields a byte-identical rejection on BOTH surfaces from ONE validator. Fails if
// the handler ever reformats, truncates, or only substring-matches the validator
// sentence — the divergence the older substring-only assertion could not catch.
func TestAgentInvoke_RejectionEnvelopeByteIdenticalToValidator_Spec088(t *testing.T) {
	// Construct an allowlist with the SAME params spec088WireAPIAgent installs
	// into the singleton, so wantRej is exactly what the handler resolves.
	allow, err := modelswitch.NewAllowlist(
		[]string{"override-model"},
		map[string]int{"override-model": 4096, "gather-model": 4096, "synth-model": 4096},
		0,
		"gather-model",
		"synth-model",
		[]string{"gather-model"}, // spec 089 tool-capable gather set
	)
	if err != nil {
		t.Fatalf("NewAllowlist: %v", err)
	}
	_, wantRej := allow.Resolve("gpt-4o")
	if wantRej == nil {
		t.Fatalf("expected a rejection for the off-allowlist model gpt-4o")
	}

	chat := &spec088APIChat{t: t} // empty queue: ANY Chat call fails the test
	cleanup := spec088WireAPIAgent(t, chat)
	defer cleanup()

	rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns","model":"gpt-4o"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if len(chat.requests) != 0 {
		t.Fatalf("a rejected model MUST NOT reach the agent; got %d LLM call(s)", len(chat.requests))
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode rejection: %v; body=%s", err, rec.Body.String())
	}
	// Byte-identical to the shared validator output — NOT a substring, NOT a
	// reformatted sentence. This is the parity contract both surfaces honor.
	if env["message"] != wantRej.Message {
		t.Fatalf("HTTP message MUST be byte-identical to the validator.\n got: %v\nwant: %q", env["message"], wantRej.Message)
	}
	if env["error_code"] != wantRej.ReasonCode {
		t.Fatalf("error_code = %v, want %q", env["error_code"], wantRej.ReasonCode)
	}
	if env["rejected_model"] != wantRej.RejectedModel {
		t.Fatalf("rejected_model = %v, want %q", env["rejected_model"], wantRej.RejectedModel)
	}
	if env["default_model"] != wantRej.DefaultModel {
		t.Fatalf("default_model = %v, want %q", env["default_model"], wantRej.DefaultModel)
	}
	// allowed_models decodes as []any; assert order + content match the
	// validator's AllowedModels exactly (a reordered or trimmed set is a
	// parity break).
	gotAllowed, ok := env["allowed_models"].([]any)
	if !ok || len(gotAllowed) != len(wantRej.AllowedModels) {
		t.Fatalf("allowed_models = %v, want %v", env["allowed_models"], wantRej.AllowedModels)
	}
	for i, m := range wantRej.AllowedModels {
		if gotAllowed[i] != m {
			t.Fatalf("allowed_models[%d] = %v, want %q (order + content must match the validator)", i, gotAllowed[i], m)
		}
	}
}

// spec089WireAPIAgent installs a home-lab-shaped open-knowledge agent + the
// spec-089 allowlist (default=deepseek-r1:32b, gather=gemma4:26b, switchable +=
// 32b, tool-capable=[gemma4:26b, llama3.1:8b]) into the agenttool singletons.
// ModelPref is left UNSET (nil) so the sticky read is the default path for
// these unauthenticated test requests. Returns a cleanup that resets the
// singletons.
func spec089WireAPIAgent(t *testing.T, chat okagent.LLMChat) func() {
	t.Helper()
	reg := ok.NewRegistry([]string{"fake_api_tool"})
	if err := reg.Register(spec088APITool{}); err != nil {
		t.Fatalf("register fake tool: %v", err)
	}
	a, err := okagent.New(chat, reg, citeback.Verify, okagent.Config{
		SystemPrompt:               "test-system-prompt",
		Model:                      "gemma4:26b",
		SynthesisModel:             "deepseek-r1:32b",
		SynthesisRetryBudget:       0,
		MaxIterations:              2,
		PerQueryTokenBudget:        1_000_000,
		PerQueryUSDBudget:          1.0,
		MonthlyBudgetUSDRemaining:  100.0,
		PerUserMonthlyUSDRemaining: 100.0,
		CompactionThresholdRatio:   0.85,
		// Spec 096 SCOPE-05 — CostFn is now the model-aware seam.
		CostFn:          func(string, int) (float64, error) { return 0, nil },
		EnforcementMode: string(citeback.EnforcementEnforce),
		SourcesMax:      5,
	})
	if err != nil {
		t.Fatalf("okagent.New: %v", err)
	}
	allow, err := modelswitch.NewAllowlist(
		[]string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"},
		map[string]int{"gemma4:26b": 18432, "deepseek-r1:7b": 4864, "deepseek-r1:32b": 22528, "llama3.1:8b": 6144},
		0,                                     // dev envelope: skip the co-residence check
		"gemma4:26b",                          // gather (baseline)
		"deepseek-r1:32b",                     // standing default synthesis
		[]string{"gemma4:26b", "llama3.1:8b"}, // tool-capable gather set
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

// TestAgentInvoke_BareDefault_EnvelopeModel32bSourceDefault_Spec089 — a bare
// invoke (no model, no gather_model) ⇒ the 200 envelope reports the SST default
// synthesis model with source=default and the baseline gather with
// source=default. (The literal deepseek-r1:32b is the committed SST default
// proven by the SCOPE-01 config tests + the downstream live bubbles.devops
// re-verify; here the unit-level proof is the source classification.)
func TestAgentInvoke_BareDefault_EnvelopeModel32bSourceDefault_Spec089(t *testing.T) {
	chat := &spec088APIChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_api_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: "A grounded answer.", TokensUsed: 80},
	}}
	cleanup := spec089WireAPIAgent(t, chat)
	defer cleanup()

	rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, rec.Body.String())
	}
	if env["model"] != "deepseek-r1:32b" {
		t.Fatalf("bare-default model = %v, want deepseek-r1:32b (SST default)", env["model"])
	}
	if env["model_source"] != "default" {
		t.Fatalf("bare-default model_source = %v, want default", env["model_source"])
	}
	if env["gather_model"] != "gemma4:26b" {
		t.Fatalf("bare-default gather_model = %v, want gemma4:26b", env["gather_model"])
	}
	if env["gather_model_source"] != "default" {
		t.Fatalf("bare-default gather_model_source = %v, want default", env["gather_model_source"])
	}
}

// TestAgentInvoke_GatherModelField_EnvelopeCarriesGatherSource_AndNonCapableRejected_Spec089
// — ADVERSARIAL. A tool-capable gather_model threads through ResolveGather →
// clone → run and the 200 envelope carries gather_model + gather_model_source;
// a non-tool-capable gather_model ⇒ HTTP 400 with error_code
// model_not_tool_capable, rejected_turn gather, and NO agent call.
func TestAgentInvoke_GatherModelField_EnvelopeCarriesGatherSource_AndNonCapableRejected_Spec089(t *testing.T) {
	t.Run("tool_capable_gather_applied_and_attributed", func(t *testing.T) {
		chat := &spec088APIChat{t: t, responses: []llm.Result{
			{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "w0", Name: "fake_api_tool", Arguments: json.RawMessage(`{}`)}}, TokensUsed: 100},
			{StopReason: llm.StopEndTurn, FinalText: "A grounded answer.", TokensUsed: 80},
		}}
		cleanup := spec089WireAPIAgent(t, chat)
		defer cleanup()
		rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns","gather_model":"llama3.1:8b"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var env map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode envelope: %v; body=%s", err, rec.Body.String())
		}
		if env["gather_model"] != "llama3.1:8b" || env["gather_model_source"] != "per_request" {
			t.Fatalf("gather attribution = %v/%v, want llama3.1:8b/per_request", env["gather_model"], env["gather_model_source"])
		}
		if env["model"] != "deepseek-r1:32b" || env["model_source"] != "default" {
			t.Fatalf("synthesis attribution = %v/%v, want deepseek-r1:32b/default", env["model"], env["model_source"])
		}
		if len(chat.requests) < 1 || chat.requests[0].Model != "llama3.1:8b" {
			t.Fatalf("the gather/tool turn MUST run the overridden gather model llama3.1:8b; requests=%d", len(chat.requests))
		}
	})

	t.Run("non_tool_capable_gather_rejected_400", func(t *testing.T) {
		chat := &spec088APIChat{t: t} // empty queue: ANY Chat call fails the test
		cleanup := spec089WireAPIAgent(t, chat)
		defer cleanup()
		rec := spec088Invoke(t, `{"scenario_id":"open_knowledge","raw_input":"compare towns","gather_model":"deepseek-r1:7b"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
		}
		if len(chat.requests) != 0 {
			t.Fatalf("a non-tool-capable gather MUST NOT reach the agent; got %d LLM call(s)", len(chat.requests))
		}
		var env map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode rejection: %v; body=%s", err, rec.Body.String())
		}
		if env["error_code"] != modelswitch.ReasonNotToolCapable {
			t.Fatalf("error_code = %v, want %q", env["error_code"], modelswitch.ReasonNotToolCapable)
		}
		if env["rejected_turn"] != modelswitch.TurnGather {
			t.Fatalf("rejected_turn = %v, want %q", env["rejected_turn"], modelswitch.TurnGather)
		}
	})
}
