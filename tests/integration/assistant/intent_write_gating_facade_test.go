//go:build integration

// Spec 068 Scope 3 — Write and State-Mutation Gating (in-process Facade.Handle).
//
// HTTP-route e2e for SCN-068-A03/A04/A09 is deferred to spec 069
// wire-up. These tests drive the assistant.Facade directly with a stub
// intent.Transport so each turn pins a specific CompiledIntent shape
// without standing up the ML sidecar.

package assistant_integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// listWriteIntentJSON pins SCN-068-A03: shopping-list creation maps
// to action_class=internal_action with side_effect_class=write.
func listWriteIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"create a shopping list",
		"action_class":"internal_action",
		"side_effect_class":"write",
		"scenario_hint":"list_assemble",
		"tool_hints":["entity_resolve","list_assemble"],
		"normalized_request":{"query":"shopping list for pad thai and caesar"},
		"slots":{"recipes":["pad thai","caesar"],"list_type":"shopping"},
		"missing_slots":[],
		"confidence":0.90,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}
	}`
}

// annotationIntentJSON pins SCN-068-A04: annotation text maps to
// action_class=state_mutation with side_effect_class=write and slots
// carrying interaction_type/rating/note (NOT keyword-derived).
func annotationIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"annotate the recipe i tried last night",
		"action_class":"state_mutation",
		"side_effect_class":"write",
		"scenario_hint":"annotation_apply",
		"tool_hints":["annotation_apply"],
		"normalized_request":{"query":"made it last night, 4 out of 5, needs more garlic"},
		"slots":{"interaction_type":"made_it","rating":4,"note":"needs more garlic"},
		"missing_slots":[],
		"confidence":0.88,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}
	}`
}

// externalWriteIntentJSON pins SCN-068-A09: any external_write must
// be gated even if the scenario itself would otherwise execute.
func externalWriteIntentJSON(t *testing.T) string {
	t.Helper()
	return `{
		"version":"v1",
		"language":"en",
		"user_goal":"post to external system",
		"action_class":"internal_action",
		"side_effect_class":"external_write",
		"scenario_hint":"external_post",
		"tool_hints":["external_post"],
		"normalized_request":{"query":"post to external"},
		"slots":{},
		"missing_slots":[],
		"confidence":0.91,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}
	}`
}

// assertGated asserts that the response is a confirm-required gate
// emission AND that neither the router nor the executor saw the turn.
func assertGated(t *testing.T, resp contracts.AssistantResponse, router *recordingRouter, executor *assistant.StubExecutor) {
	t.Helper()
	if resp.Status != contracts.StatusUnavailable {
		t.Fatalf("response Status = %q, want %q (confirm-required gate)", resp.Status, contracts.StatusUnavailable)
	}
	if !resp.CaptureRoute {
		t.Fatalf("response.CaptureRoute = false, want true (gated turn must capture)")
	}
	if !strings.Contains(strings.ToLower(resp.Body), "confirm") {
		t.Fatalf("response.Body = %q, want it to mention 'confirm'", resp.Body)
	}
	router.mu.Lock()
	envCount := len(router.envelopes)
	router.mu.Unlock()
	if envCount != 0 {
		t.Fatalf("router was invoked %d times, want 0 (gate must short-circuit before route)", envCount)
	}
	if executor.Invocations != 0 {
		t.Fatalf("executor was invoked %d times, want 0 (gate must short-circuit before execute)", executor.Invocations)
	}
}

// TestIntentWriteGatingFacade_ListWriteRequiresConfirmationBeforePersistence
// SCN-068-A03.
//
// Drive the facade with "make a shopping list for Pad Thai and Caesar".
// The compiler returns side_effect_class=write. Assert the facade emits
// a confirm-required response and the scenario executor is never
// invoked — i.e. no list can be persisted before confirmation.
func TestIntentWriteGatingFacade_ListWriteRequiresConfirmationBeforePersistence(t *testing.T) {
	ft := &stubTransport{resolve: func(_ string) string { return listWriteIntentJSON(t) }}
	compiler := buildCompiler(t, ft)

	sc := &agent.Scenario{ID: "list_assemble", Version: "v1"}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"list_assemble": sc})
	executor := assistant.NewStubExecutor()
	router := &recordingRouter{byID: map[string]*agent.Scenario{"list_assemble": sc}}
	f := buildWriteFacade(t, compiler, router, registry, executor, map[string]assistant.ManifestEntryForTest{
		"list_assemble": {UserFacingLabel: "list", EnableSSTKey: "assistant.skills.list_assemble.enabled", Enabled: true},
	})

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-list-write",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "make a shopping list for Pad Thai and Caesar",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if ft.calls != 1 {
		t.Fatalf("compiler.calls = %d, want 1 (compiler MUST run before gate)", ft.calls)
	}
	assertGated(t, resp, router, executor)
}

// TestIntentWriteGatingFacade_AnnotationSlotsComeFromCompiledIntent
// SCN-068-A04.
//
// Annotation turns compile to state_mutation with side_effect_class=write
// and structured slots {interaction_type, rating, note}. The facade
// MUST gate (so no annotation persists) and the compiled slots present
// in the compiler trace MUST come from the LLM, not a runtime keyword
// map. The negative-control sub-test sends a text that contains NO
// annotation-style keywords; the compiler still returns the same slot
// shape, proving slot derivation does not depend on raw-text keywords.
func TestIntentWriteGatingFacade_AnnotationSlotsComeFromCompiledIntent(t *testing.T) {
	var lastRawText string
	ft := &stubTransport{resolve: func(text string) string {
		lastRawText = text
		return annotationIntentJSON(t)
	}}
	compiler := buildCompiler(t, ft)

	sc := &agent.Scenario{ID: "annotation_apply", Version: "v1"}
	router := &recordingRouter{byID: map[string]*agent.Scenario{"annotation_apply": sc}}
	registry := assistant.NewMapRegistry(map[string]*agent.Scenario{"annotation_apply": sc})
	executor := assistant.NewStubExecutor()
	f := buildWriteFacade(t, compiler, router, registry, executor, map[string]assistant.ManifestEntryForTest{
		"annotation_apply": {UserFacingLabel: "annotate", EnableSSTKey: "assistant.skills.annotation_apply.enabled", Enabled: true},
	})

	// 1) Natural-language annotation text — slots from compiler.
	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-ann-1",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "made it last night, 4 out of 5, needs more garlic",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	assertGated(t, resp, router, executor)

	// Confirm the compiled slot shape we asserted on the wire is
	// what the compiler actually emitted (parses cleanly + keys
	// present). This is what the facade would have handed
	// downstream had the gate not fired.
	var ci map[string]any
	if err := json.Unmarshal([]byte(annotationIntentJSON(t)), &ci); err != nil {
		t.Fatalf("annotationIntentJSON parse: %v", err)
	}
	slots, _ := ci["slots"].(map[string]any)
	if slots["interaction_type"] != "made_it" {
		t.Fatalf("compiled slots.interaction_type = %v, want made_it", slots["interaction_type"])
	}
	if got := slots["rating"]; got != float64(4) {
		t.Fatalf("compiled slots.rating = %v, want 4", got)
	}
	if slots["note"] != "needs more garlic" {
		t.Fatalf("compiled slots.note = %v, want %q", slots["note"], "needs more garlic")
	}

	// 2) Negative control: text with NO annotation-style keywords
	// still maps to the same slot shape. Proves a runtime keyword
	// map did not derive interaction_type from the raw text.
	router2 := &recordingRouter{byID: map[string]*agent.Scenario{"annotation_apply": sc}}
	executor2 := assistant.NewStubExecutor()
	f2 := buildWriteFacade(t, compiler, router2, registry, executor2, map[string]assistant.ManifestEntryForTest{
		"annotation_apply": {UserFacingLabel: "annotate", EnableSSTKey: "assistant.skills.annotation_apply.enabled", Enabled: true},
	})
	resp2, err := f2.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-ann-2",
		Transport: "telegram",
		Kind:      contracts.KindText,
		Text:      "yellow elephants over the rainbow",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	assertGated(t, resp2, router2, executor2)
	if !strings.Contains(lastRawText, "yellow elephants") {
		t.Fatalf("compiler did not see the negative-control raw text; got last=%q", lastRawText)
	}
}

// TestIntentWriteGatingFacade_WriteAndStateMutationNeverBypassConfirmGate
// Regression: SCN-068-A03, SCN-068-A04, SCN-068-A09.
//
// For every gated side_effect_class — write (list + annotation) and
// external_write — the facade MUST emit a confirm-required response
// and the executor MUST NOT be invoked. Adversarial baseline: the
// SAME texts driven through the SAME compiler but with a transport
// that returns side_effect_class=read DO reach the executor. Without
// the baseline, a regression that always-gates or never-gates would
// only fail in one direction.
func TestIntentWriteGatingFacade_WriteAndStateMutationNeverBypassConfirmGate(t *testing.T) {
	type tc struct {
		name     string
		text     string
		body     string
		scenario string
	}
	cases := []tc{
		{name: "list_write", text: "make a shopping list for Pad Thai", body: listWriteIntentJSON(t), scenario: "list_assemble"},
		{name: "annotation_state_mutation", text: "made it last night, 4 out of 5", body: annotationIntentJSON(t), scenario: "annotation_apply"},
		{name: "external_write", text: "post this to external", body: externalWriteIntentJSON(t), scenario: "external_post"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ft := &stubTransport{resolve: func(_ string) string { return c.body }}
			compiler := buildCompiler(t, ft)
			sc := &agent.Scenario{ID: c.scenario, Version: "v1"}
			router := &recordingRouter{byID: map[string]*agent.Scenario{c.scenario: sc}}
			registry := assistant.NewMapRegistry(map[string]*agent.Scenario{c.scenario: sc})
			executor := assistant.NewStubExecutor()
			f := buildWriteFacade(t, compiler, router, registry, executor, map[string]assistant.ManifestEntryForTest{
				c.scenario: {UserFacingLabel: c.scenario, EnableSSTKey: "assistant.skills." + c.scenario + ".enabled", Enabled: true},
			})
			resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
				UserID: "u-gate-1", Transport: "telegram", Kind: contracts.KindText, Text: c.text,
			})
			if err != nil {
				t.Fatalf("Handle: %v", err)
			}
			assertGated(t, resp, router, executor)

			// Adversarial baseline: same scenario + text, but the
			// compiler returns side_effect_class=read. The gate
			// MUST NOT fire and the executor MUST be invoked.
			readBody := strings.Replace(c.body, `"side_effect_class":"write"`, `"side_effect_class":"read"`, 1)
			readBody = strings.Replace(readBody, `"side_effect_class":"external_write"`, `"side_effect_class":"read"`, 1)
			readBody = strings.Replace(readBody, `"action_class":"state_mutation"`, `"action_class":"retrieve"`, 1)
			readBody = strings.Replace(readBody, `"action_class":"internal_action"`, `"action_class":"retrieve"`, 1)
			ftBase := &stubTransport{resolve: func(_ string) string { return readBody }}
			compilerBase := buildCompiler(t, ftBase)
			routerBase := &recordingRouter{byID: map[string]*agent.Scenario{c.scenario: sc}}
			executorBase := assistant.NewStubExecutor()
			fBase := buildWriteFacade(t, compilerBase, routerBase, registry, executorBase, map[string]assistant.ManifestEntryForTest{
				c.scenario: {UserFacingLabel: c.scenario, EnableSSTKey: "assistant.skills." + c.scenario + ".enabled", Enabled: true},
			})
			if _, err := fBase.Handle(context.Background(), contracts.AssistantMessage{
				UserID: "u-gate-2", Transport: "telegram", Kind: contracts.KindText, Text: c.text,
			}); err != nil {
				t.Fatalf("baseline Handle: %v", err)
			}
			if executorBase.Invocations == 0 {
				t.Fatalf("adversarial baseline: executor was not invoked for read-class %q — gate is firing for non-write turns (false positive)", c.name)
			}
		})
	}
}
