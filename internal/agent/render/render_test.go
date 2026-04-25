package render

// Spec 037 Scope 8 — render layer unit tests.
// Asserts that every outcome class from design §8 produces an
// OutcomeView whose Fields list contains every key declared in the
// outcomeRegistry.required slice, and that the values are non-empty
// strings (the Field renderer substitutes "(unset)" rather than
// returning an empty string, so a present-but-blank value is also a
// rendering bug worth catching).

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// representativeTrace returns a TraceRow shaped to make every required
// field for the named outcome class non-empty. The shape mirrors what
// the executor + tracer actually persist in production.
func representativeTrace(t *testing.T, outcome string) *agent.TraceRow {
	t.Helper()
	tr := &agent.TraceRow{
		TraceID:         "trace_test_" + outcome,
		ScenarioID:      "scope8_render",
		ScenarioVersion: "scope8_render-v1",
		ScenarioHash:    "deadbeef",
		Source:          "test",
		Outcome:         outcome,
		Provider:        "fake",
		Model:           "fake-model",
		LatencyMs:       42,
		StartedAt:       time.Unix(1700000000, 0),
		EndedAt:         time.Unix(1700000001, 0),
	}
	tr.Routing = mustMarshal(t, agent.RoutingDecision{
		Reason:    agent.ReasonSimilarityMatch,
		Chosen:    "scope8_render",
		TopScore:  0.87,
		Threshold: 0.65,
		Considered: []agent.CandidateScore{
			{ScenarioID: "scope8_render", Score: 0.87},
			{ScenarioID: "other", Score: 0.41},
		},
	})
	tr.InputEnvelope = mustMarshal(t, agent.IntentEnvelope{
		Source:            "test",
		RawInput:          "render fixture",
		StructuredContext: json.RawMessage(`{"q":"hello"}`),
	})

	switch outcome {
	case string(agent.OutcomeOK):
		tr.ToolCalls = mustMarshal(t, []agent.ExecutedToolCall{
			{Seq: 0, Name: "echo", Outcome: agent.OutcomeOK, LatencyMs: 5,
				Arguments: json.RawMessage(`{"q":"hello"}`),
				Result:    json.RawMessage(`{"q":"hello"}`)},
		})
		tr.FinalOutput = json.RawMessage(`{"answer":"hello"}`)
	case string(agent.OutcomeUnknownIntent):
		tr.Routing = mustMarshal(t, agent.RoutingDecision{
			Reason: agent.ReasonUnknownIntent, TopScore: 0.10, Threshold: 0.65,
			Considered: []agent.CandidateScore{{ScenarioID: "other", Score: 0.10}},
		})
	case string(agent.OutcomeAllowlistViolation):
		tr.ToolCalls = mustMarshal(t, []agent.ExecutedToolCall{
			{Seq: 0, Name: "evil_write", Outcome: agent.OutcomeAllowlistViolation,
				RejectionReason: "tool_not_allowed", LatencyMs: 1,
				Arguments: json.RawMessage(`{}`)},
		})
		tr.OutcomeDetail = json.RawMessage(`{}`)
	case string(agent.OutcomeHallucinatedTool):
		tr.ToolCalls = mustMarshal(t, []agent.ExecutedToolCall{
			{Seq: 0, Name: "made_up_tool", Outcome: agent.OutcomeHallucinatedTool,
				RejectionReason: "unknown_tool", LatencyMs: 1,
				Arguments: json.RawMessage(`{}`)},
		})
	case string(agent.OutcomeToolError):
		tr.OutcomeDetail = json.RawMessage(`{"tool":"echo","error":"db down","detail":"connection refused"}`)
	case string(agent.OutcomeToolReturnInvalid):
		tr.OutcomeDetail = json.RawMessage(`{"tool":"echo","error":"return_schema_violation","detail":"missing field q"}`)
	case string(agent.OutcomeSchemaFailure):
		tr.OutcomeDetail = json.RawMessage(`{"attempts":2,"last_error":"missing field answer"}`)
	case string(agent.OutcomeLoopLimit):
		tr.OutcomeDetail = json.RawMessage(`{"reason":"max_iterations_exceeded","max_loop_iterations":4,"iterations":4}`)
	case string(agent.OutcomeTimeout):
		tr.OutcomeDetail = json.RawMessage(`{"deadline_s":30,"reason":"provider_did_not_respond_before_deadline"}`)
	case string(agent.OutcomeProviderError):
		tr.OutcomeDetail = json.RawMessage(`{"error":"llm_driver_error","detail":"http 500"}`)
	case string(agent.OutcomeInputSchemaViolation):
		tr.OutcomeDetail = json.RawMessage(`{"error":"input_schema_violation","detail":"required field q"}`)
	}
	return tr
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestBuildOutcomeView_AllClassesRenderRequiredFields(t *testing.T) {
	classes := AllOutcomeClasses()
	if len(classes) < 9 {
		t.Fatalf("AllOutcomeClasses must cover at least the nine design §8 outcomes; got %d", len(classes))
	}
	for _, class := range classes {
		t.Run(class, func(t *testing.T) {
			tr := representativeTrace(t, class)
			view := buildOutcomeView(tr)

			if view.Class != class {
				t.Fatalf("Class = %q want %q", view.Class, class)
			}
			if view.Label == "" {
				t.Fatalf("Label is empty for outcome %q", class)
			}
			if view.Severity == "" {
				t.Fatalf("Severity is empty for outcome %q", class)
			}
			required := RequiredFields(class)
			if len(required) == 0 {
				t.Fatalf("RequiredFields(%q) returned no required keys", class)
			}
			present := map[string]string{}
			for _, f := range view.Fields {
				present[f.Key] = f.Value
			}
			for _, key := range required {
				val, ok := present[key]
				if !ok {
					t.Errorf("outcome %q missing required field %q (have %v)", class, key, fieldKeys(view.Fields))
				}
				if val == "" {
					t.Errorf("outcome %q field %q rendered empty string (must be at least \"(unset)\")", class, key)
				}
			}
		})
	}
}

func fieldKeys(fs []Field) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Key)
	}
	return out
}

func TestIsValidOutcomeClass(t *testing.T) {
	for _, c := range AllOutcomeClasses() {
		if !IsValidOutcomeClass(c) {
			t.Errorf("IsValidOutcomeClass(%q) = false; want true", c)
		}
	}
	if IsValidOutcomeClass("not-a-real-class") {
		t.Error("IsValidOutcomeClass accepted bogus value")
	}
}

func TestBuildTraceDetail_PopulatesEnvelopeRoutingAndCalls(t *testing.T) {
	tr := representativeTrace(t, string(agent.OutcomeOK))
	det := BuildTraceDetail(tr)

	if det.Summary.TraceID != tr.TraceID {
		t.Fatalf("summary.TraceID = %q want %q", det.Summary.TraceID, tr.TraceID)
	}
	if det.Routing.Chosen != "scope8_render" {
		t.Fatalf("routing.Chosen = %q", det.Routing.Chosen)
	}
	if len(det.Routing.Considered) != 2 {
		t.Fatalf("routing.Considered len = %d want 2", len(det.Routing.Considered))
	}
	if det.Envelope.Source != "test" || det.Envelope.RawInput != "render fixture" {
		t.Fatalf("envelope view incorrect: %+v", det.Envelope)
	}
	if len(det.ToolCalls) != 1 || det.ToolCalls[0].Name != "echo" {
		t.Fatalf("tool calls not surfaced: %+v", det.ToolCalls)
	}
	if !strings.Contains(det.FinalOutput, "answer") {
		t.Fatalf("FinalOutput pretty-print missing key: %q", det.FinalOutput)
	}
}

func TestBuildToolSummary_BadgeAndAllowlistedBy(t *testing.T) {
	tool := agent.Tool{
		Name:            "search_x",
		Description:     "search x",
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "pkg",
	}
	got := BuildToolSummary(tool, []string{"b_scenario", "a_scenario"})
	if got.SideEffectBadge != "read" {
		t.Fatalf("badge = %q", got.SideEffectBadge)
	}
	if len(got.AllowlistedByIDs) != 2 || got.AllowlistedByIDs[0] != "a_scenario" {
		t.Fatalf("allowlistedBy not sorted: %v", got.AllowlistedByIDs)
	}
}
