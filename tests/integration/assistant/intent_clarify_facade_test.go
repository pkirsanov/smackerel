//go:build integration

// Spec 068 Scope 4 — Clarification gate (in-process Facade.Handle).
//
// SCN-068-A05: ambiguous turns (e.g. "springfield weather tomorrow")
// compile to action_class=clarify and the facade MUST emit a
// clarification response WITHOUT routing weather_lookup (or any
// scenario). HTTP-route e2e for this scenario is deferred to spec
// 069 wire-up.

package assistant_integration

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// springfieldClarifyIntentJSON pins SCN-068-A05: ambiguous location
// (Springfield) produces action_class=clarify with a missing-slot
// list identifying the location ambiguity and a clarification_prompt.
func springfieldClarifyIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"check weather",
		"action_class":"clarify",
		"side_effect_class":"none",
		"scenario_hint":"weather_query",
		"tool_hints":[],
		"normalized_request":{"query":"springfield weather tomorrow"},
		"slots":{"window":"tomorrow"},
		"missing_slots":["location"],
		"confidence":0.62,
		"clarification_prompt":"which Springfield did you mean (IL, MA, MO, OR)?",
		"safety_flags":[],
		"source_policy":{"requires_citations":true,"allowed_source_kinds":["tool"]}
	}`
}

// buildClarifyFacade reuses the Scope-3 helper so the same Facade
// shape (with a stub executor for assertions) is used here.
func buildClarifyFacade(t *testing.T, body string, executor *assistant.StubExecutor) (*assistant.Facade, *recordingRouter, *stubTransport) {
	t.Helper()
	ft := &stubTransport{resolve: func(_ string) string { return body }}
	compiler := buildCompiler(t, ft)
	sc := &agent.Scenario{ID: "weather_query", Version: "v1"}
	router := &recordingRouter{byID: map[string]*agent.Scenario{"weather_query": sc}}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"weather_query": sc})
	f := buildWriteFacade(t, compiler, router, registry, executor, map[string]assistant.ManifestEntryForTest{
		"weather_query": {UserFacingLabel: "weather", EnableSSTKey: "assistant.skills.weather_query.enabled", Enabled: true},
	})
	return f, router, ft
}

// TestIntentClarifyFacade_SpringfieldWeatherClarifiesLocation
// SCN-068-A05.
//
// Ambiguous Springfield weather turn compiles to action_class=clarify
// with missing_slots=["location"]. The facade MUST emit a clarification
// response (StatusUnavailable + ErrSlotMissing) carrying the compiler's
// clarification_prompt verbatim, MUST NOT invoke the router, and MUST
// NOT invoke the executor.
func TestIntentClarifyFacade_SpringfieldWeatherClarifiesLocation(t *testing.T) {
	executor := assistant.NewStubExecutor()
	f, router, ft := buildClarifyFacade(t, springfieldClarifyIntentJSON(t), executor)

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-clarify-1",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "springfield weather tomorrow",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if ft.calls != 1 {
		t.Fatalf("compiler.calls = %d, want 1 (compiler MUST run before clarify gate)", ft.calls)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Fatalf("response.Status = %q, want %q (clarify gate)", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrSlotMissing {
		t.Fatalf("response.ErrorCause = %q, want %q", resp.ErrorCause, contracts.ErrSlotMissing)
	}
	if !strings.Contains(strings.ToLower(resp.Body), "springfield") {
		t.Fatalf("response.Body = %q, want it to mention Springfield (from compiler clarification_prompt)", resp.Body)
	}
	router.mu.Lock()
	envCount := len(router.envelopes)
	router.mu.Unlock()
	if envCount != 0 {
		t.Fatalf("router was invoked %d times, want 0 (clarify MUST short-circuit before route)", envCount)
	}
	if executor.Invocations != 0 {
		t.Fatalf("executor was invoked %d times, want 0 (clarify MUST short-circuit before execute)", executor.Invocations)
	}
	_ = agent.IntentEnvelope{} // keep agent import alive for IDE
}

// TestIntentClarifyFacade_AmbiguousLocationNeverRoutesWeatherLookup
// Regression: SCN-068-A05.
//
// Adversarial baseline pair. First arm: clarify intent —
// router/executor MUST stay idle. Second arm: unambiguous weather
// intent (missing_slots=[], action_class=external_lookup) — router
// MUST be called and executor MUST execute. Without the baseline a
// regression that always-or-never clarifies would only fail in one
// direction.
func TestIntentClarifyFacade_AmbiguousLocationNeverRoutesWeatherLookup(t *testing.T) {
	// Clarify arm.
	exec1 := assistant.NewStubExecutor()
	f1, router1, _ := buildClarifyFacade(t, springfieldClarifyIntentJSON(t), exec1)
	if _, err := f1.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-clarify-2", Transport: "telegram", Kind: contracts.KindText,
		Text: "springfield weather tomorrow",
	}); err != nil {
		t.Fatalf("clarify Handle: %v", err)
	}
	router1.mu.Lock()
	clarifyRouted := len(router1.envelopes)
	router1.mu.Unlock()
	if clarifyRouted != 0 {
		t.Fatalf("clarify arm: router invoked %d times, want 0 (ambiguous turn must NOT route weather_lookup)", clarifyRouted)
	}
	if exec1.Invocations != 0 {
		t.Fatalf("clarify arm: executor invoked %d times, want 0", exec1.Invocations)
	}

	// Adversarial baseline: unambiguous weather. Router AND executor
	// MUST run.
	exec2 := assistant.NewStubExecutor()
	f2, router2, _ := buildClarifyFacade(t, weatherIntentJSON(t), exec2)
	if _, err := f2.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-clarify-3", Transport: "telegram", Kind: contracts.KindText,
		Text: "weather in palm springs ca tomorrow",
	}); err != nil {
		t.Fatalf("baseline Handle: %v", err)
	}
	router2.mu.Lock()
	baselineRouted := len(router2.envelopes)
	router2.mu.Unlock()
	if baselineRouted == 0 {
		t.Fatalf("adversarial baseline: router was never invoked for an unambiguous weather turn — clarify gate is over-firing (false positive)")
	}
	if exec2.Invocations == 0 {
		t.Fatalf("adversarial baseline: executor was never invoked for an unambiguous weather turn")
	}
}
