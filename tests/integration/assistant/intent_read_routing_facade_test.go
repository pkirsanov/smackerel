//go:build integration

// Spec 068 Scope 2 — Read Intent Routing (in-process Facade.Handle).
//
// HTTP-route e2e for SCN-068-A01/A02 is deferred to spec 069 wire-up.
// These tests drive the assistant.Facade directly with a stub
// intent.Transport so we can pin the CompiledIntent shape per turn
// without standing up the ML sidecar.

package assistant_integration

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
)

// --- shared stubs ---

// stubTransport returns a canned CompileResponse whose body is selected
// by inspecting the request's raw text. Each test installs a tiny
// router function so the same transport handles weather / retrieval /
// answer turns deterministically.
type stubTransport struct {
	mu      sync.Mutex
	resolve func(text string) string
	calls   int
}

func (s *stubTransport) Compile(_ context.Context, req intent.CompileRequest) (intent.CompileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	body := s.resolve(req.RawTurn.Text)
	return intent.CompileResponse{
		SchemaVersion:  "v1",
		CompiledIntent: json.RawMessage(body),
		Provider:       "stub",
		Model:          "stub",
		LatencyMS:      1,
	}, nil
}

// recordingRouter captures every envelope the facade hands to the
// router and returns a pre-configured RoutingDecision keyed by the
// envelope's ScenarioID (which the facade sets from the compiled
// scenario_hint).
type recordingRouter struct {
	mu        sync.Mutex
	envelopes []agent.IntentEnvelope
	byID      map[string]*agent.Scenario
}

func (r *recordingRouter) Route(_ context.Context, env agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.envelopes = append(r.envelopes, env)
	sc, ok := r.byID[env.ScenarioID]
	if !ok {
		return nil, agent.RoutingDecision{Reason: agent.ReasonUnknownIntent}, false
	}
	return sc, agent.RoutingDecision{
		Reason:    agent.ReasonExplicitScenarioID,
		Chosen:    env.ScenarioID,
		TopScore:  1.0,
		Threshold: 0.5,
	}, true
}

func (r *recordingRouter) lastEnvelope(t *testing.T) agent.IntentEnvelope {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.envelopes) == 0 {
		t.Fatalf("router was never invoked")
	}
	return r.envelopes[len(r.envelopes)-1]
}

// buildCompiler wires the spec 068 LLMCompiler against a stub
// transport using the same valid config shape as the Scope 1a unit
// tests.
func buildCompiler(t *testing.T, ft intent.Transport) intent.Compiler {
	t.Helper()
	cfg := intent.CompilerConfig{
		Enabled:               true,
		ModelRole:             "assistant_intent_compiler",
		PromptContractVersion: "intent-compiler-v1",
		SchemaVersion:         "v1",
		Timeout:               2 * time.Second,
		ConfidenceFloor:       0.5,
		MaxContextTurns:       4,
		MaxOutputBytes:        16384,
		RetryBudget:           1,
	}
	c, err := intent.NewLLMCompiler(cfg, ft)
	if err != nil {
		t.Fatalf("NewLLMCompiler: %v", err)
	}
	return c
}

func buildReadFacade(t *testing.T, compiler intent.Compiler, router *recordingRouter, registry *assistant.MapRegistry, enabled map[string]assistant.ManifestEntryForTest) *assistant.Facade {
	t.Helper()
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
	manifest, err := assistant.NewManifestForTest(enabled)
	if err != nil {
		t.Fatalf("NewManifestForTest: %v", err)
	}
	f, err := assistant.NewFacade(cfg, router, assistant.NewStubExecutor(), registry,
		manifest, assistant.NewInMemoryContextStore(), assistant.NewRecordingAudit())
	if err != nil {
		t.Fatalf("NewFacade: %v", err)
	}
	if compiler != nil {
		f.WithIntentCompiler(compiler)
	}
	return f
}

func weatherIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"check weather",
		"action_class":"external_lookup",
		"side_effect_class":"external_read",
		"scenario_hint":"weather_query",
		"tool_hints":["location_normalize","weather_lookup"],
		"normalized_request":{"query":"weather palm springs ca tomorrow"},
		"slots":{"location":{"raw":"palm springs ca"},"window":"tomorrow"},
		"missing_slots":[],
		"confidence":0.92,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":true,"allowed_source_kinds":["tool"]}
	}`
}

func retrievalIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"recall saved ACL artifacts",
		"action_class":"retrieve",
		"side_effect_class":"read",
		"scenario_hint":"retrieval_qa",
		"tool_hints":["vector_search"],
		"normalized_request":{"query":"what did I save about ACL tags last month"},
		"slots":{"topic":"ACL tags","window":"last month"},
		"missing_slots":[],
		"confidence":0.88,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":true,"allowed_source_kinds":["graph"]}
	}`
}

func answerIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"general question",
		"action_class":"answer",
		"side_effect_class":"none",
		"scenario_hint":"retrieval_qa",
		"tool_hints":[],
		"normalized_request":{"query":"what is the capital of france"},
		"slots":{},
		"missing_slots":[],
		"confidence":0.85,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}
	}`
}

// --- TestIntentReadRoutingFacade_WeatherCompilesBeforeRouteAndNormalizesLocation
// SCN-068-A01.
//
// Drive the facade with "weather in palm springs ca tomorrow". Assert:
//   - compiler is called exactly once
//   - compiler is called BEFORE the router
//   - router envelope ScenarioID == "weather_query" (from scenario_hint)
//   - router envelope StructuredContext carries compiled_intent with
//     action_class=external_lookup, scenario_hint=weather_query, and
//     slots.location.raw="palm springs ca"
func TestIntentReadRoutingFacade_WeatherCompilesBeforeRouteAndNormalizesLocation(t *testing.T) {
	ft := &stubTransport{resolve: func(_ string) string { return weatherIntentJSON(t) }}
	compiler := buildCompiler(t, ft)

	sc := &agent.Scenario{ID: "weather_query", Version: "v1"}
	router := &recordingRouter{byID: map[string]*agent.Scenario{"weather_query": sc}}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"weather_query": sc})
	f := buildReadFacade(t, compiler, router, registry, map[string]assistant.ManifestEntryForTest{
		"weather_query": {UserFacingLabel: "weather", EnableSSTKey: "assistant.skills.weather_query.enabled", Enabled: true},
	})

	_, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-weather",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "weather in palm springs ca tomorrow",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if ft.calls != 1 {
		t.Fatalf("compiler.calls = %d, want 1 (compiler MUST run before router)", ft.calls)
	}
	env := router.lastEnvelope(t)
	if env.ScenarioID != "weather_query" {
		t.Fatalf("router envelope ScenarioID = %q, want %q (scenario_hint not propagated)", env.ScenarioID, "weather_query")
	}
	if len(env.StructuredContext) == 0 {
		t.Fatal("router envelope StructuredContext is empty; compiled_intent was not attached")
	}
	var payload map[string]any
	if err := json.Unmarshal(env.StructuredContext, &payload); err != nil {
		t.Fatalf("StructuredContext JSON: %v", err)
	}
	ci, ok := payload["compiled_intent"].(map[string]any)
	if !ok {
		t.Fatalf("StructuredContext missing compiled_intent: %v", payload)
	}
	if ci["action_class"] != "external_lookup" {
		t.Fatalf("compiled_intent.action_class = %v, want external_lookup", ci["action_class"])
	}
	if ci["scenario_hint"] != "weather_query" {
		t.Fatalf("compiled_intent.scenario_hint = %v, want weather_query", ci["scenario_hint"])
	}
	slots, _ := ci["slots"].(map[string]any)
	loc, _ := slots["location"].(map[string]any)
	if loc["raw"] != "palm springs ca" {
		t.Fatalf("compiled_intent.slots.location.raw = %v, want %q", loc["raw"], "palm springs ca")
	}
	if win := slots["window"]; win != "tomorrow" {
		t.Fatalf("compiled_intent.slots.window = %v, want tomorrow", win)
	}
}

// --- TestIntentReadRoutingFacade_RetrievalReceivesStructuredContext
// SCN-068-A02.
//
// Drive the facade with the retrieval prompt. Assert the router
// envelope contains compiled_intent with action_class=retrieve and
// the normalized_request.query preserved (raw text alone does not
// drive behavior).
func TestIntentReadRoutingFacade_RetrievalReceivesStructuredContext(t *testing.T) {
	ft := &stubTransport{resolve: func(_ string) string { return retrievalIntentJSON(t) }}
	compiler := buildCompiler(t, ft)

	sc := &agent.Scenario{ID: "retrieval_qa", Version: "v1"}
	router := &recordingRouter{byID: map[string]*agent.Scenario{"retrieval_qa": sc}}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"retrieval_qa": sc})
	f := buildReadFacade(t, compiler, router, registry, map[string]assistant.ManifestEntryForTest{
		"retrieval_qa": {UserFacingLabel: "retrieval", EnableSSTKey: "assistant.skills.retrieval_qa.enabled", Enabled: true},
	})

	_, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-retrieval",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "what did I save about ACL tags last month?",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	env := router.lastEnvelope(t)
	if env.ScenarioID != "retrieval_qa" {
		t.Fatalf("router envelope ScenarioID = %q, want retrieval_qa", env.ScenarioID)
	}
	var payload map[string]any
	if err := json.Unmarshal(env.StructuredContext, &payload); err != nil {
		t.Fatalf("StructuredContext JSON: %v", err)
	}
	ci, ok := payload["compiled_intent"].(map[string]any)
	if !ok {
		t.Fatalf("StructuredContext missing compiled_intent: %v", payload)
	}
	if ci["action_class"] != "retrieve" {
		t.Fatalf("compiled_intent.action_class = %v, want retrieve", ci["action_class"])
	}
	nr, _ := ci["normalized_request"].(map[string]any)
	q, _ := nr["query"].(string)
	if !strings.Contains(q, "ACL tags") {
		t.Fatalf("normalized_request.query = %q, expected to contain 'ACL tags'", q)
	}
}

// --- TestIntentReadRoutingFacade_ReadIntentsNeverRouteFromRawTextOnly
// Regression: SCN-068-A01, SCN-068-A02.
//
// For every read action_class (retrieve, external_lookup, answer) the
// envelope handed to the router MUST contain compiled_intent in
// StructuredContext. Adversarial baseline: the SAME texts driven
// through a facade with NO compiler attached produce envelopes
// WITHOUT compiled_intent — proving the new wiring is actually doing
// the work and not merely inheriting a pre-existing field.
func TestIntentReadRoutingFacade_ReadIntentsNeverRouteFromRawTextOnly(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		body     string
		scenario string
	}{
		{name: "external_lookup", text: "weather in palm springs ca tomorrow", body: weatherIntentJSON(t), scenario: "weather_query"},
		{name: "retrieve", text: "what did I save about ACL tags last month?", body: retrievalIntentJSON(t), scenario: "retrieval_qa"},
		{name: "answer", text: "what is the capital of france?", body: answerIntentJSON(t), scenario: "retrieval_qa"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ft := &stubTransport{resolve: func(_ string) string { return tc.body }}
			compiler := buildCompiler(t, ft)
			sc := &agent.Scenario{ID: tc.scenario, Version: "v1"}
			router := &recordingRouter{byID: map[string]*agent.Scenario{tc.scenario: sc}}
			registry := assistant.NewMapRegistry(map[string]*agent.Scenario{tc.scenario: sc})
			f := buildReadFacade(t, compiler, router, registry, map[string]assistant.ManifestEntryForTest{
				tc.scenario: {UserFacingLabel: tc.scenario, EnableSSTKey: "assistant.skills." + tc.scenario + ".enabled", Enabled: true},
			})
			if _, err := f.Handle(context.Background(), contracts.AssistantMessage{
				UserID: "u-1", Transport: "telegram", Kind: contracts.KindText, Text: tc.text,
			}); err != nil {
				t.Fatalf("with-compiler Handle: %v", err)
			}
			env := router.lastEnvelope(t)
			if len(env.StructuredContext) == 0 {
				t.Fatalf("router envelope StructuredContext is empty for read intent %q", tc.name)
			}
			var payload map[string]any
			if err := json.Unmarshal(env.StructuredContext, &payload); err != nil {
				t.Fatalf("StructuredContext JSON: %v", err)
			}
			if _, ok := payload["compiled_intent"]; !ok {
				t.Fatalf("router envelope missing compiled_intent for read intent %q: %v", tc.name, payload)
			}

			// Adversarial baseline: no compiler attached → no
			// compiled_intent in StructuredContext for the same text.
			// This proves the with-compiler assertions above are not
			// passing because of some pre-existing payload.
			router2 := &recordingRouter{byID: map[string]*agent.Scenario{tc.scenario: sc}}
			fBase := buildReadFacade(t, nil, router2, registry, map[string]assistant.ManifestEntryForTest{
				tc.scenario: {UserFacingLabel: tc.scenario, EnableSSTKey: "assistant.skills." + tc.scenario + ".enabled", Enabled: true},
			})
			if _, err := fBase.Handle(context.Background(), contracts.AssistantMessage{
				UserID: "u-2", Transport: "telegram", Kind: contracts.KindText, Text: tc.text,
			}); err != nil {
				t.Fatalf("no-compiler Handle: %v", err)
			}
			envBase := router2.lastEnvelope(t)
			if len(envBase.StructuredContext) != 0 {
				var p map[string]any
				_ = json.Unmarshal(envBase.StructuredContext, &p)
				if _, leaked := p["compiled_intent"]; leaked {
					t.Fatalf("baseline (no compiler) envelope unexpectedly contains compiled_intent: %v", p)
				}
			}
		})
	}
}
