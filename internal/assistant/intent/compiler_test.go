// Spec 068 SCN-068-A06 — compiler rejects malformed JSON without routing.

package intent_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/intent"
)

type fakeTransport struct {
	resp    intent.CompileResponse
	err     error
	calls   int
	lastReq intent.CompileRequest
}

func (f *fakeTransport) Compile(_ context.Context, req intent.CompileRequest) (intent.CompileResponse, error) {
	f.calls++
	f.lastReq = req
	return f.resp, f.err
}

func validConfig() intent.CompilerConfig {
	return intent.CompilerConfig{
		Enabled:               true,
		ModelRole:             "assistant_intent_compiler",
		PromptContractVersion: "intent-compiler-v1",
		SchemaVersion:         "v1",
		Timeout:               2 * time.Second,
		ConfidenceFloor:       0.6,
		MaxContextTurns:       4,
		MaxOutputBytes:        16384,
		RetryBudget:           1,
	}
}

func validRawTurn() intent.RawTurn {
	return intent.RawTurn{
		UserID:    "u1",
		Transport: "test",
		Text:      "hello",
	}
}

// TestCompilerRejectsMalformedJSONWithoutRouting — SCN-068-A06.
//
// Drive the compiler with malformed LLM output. Assert:
//   - returned error is a SchemaError with cause="json_invalid"
//   - the returned CompiledIntent is the zero value (no routing material)
//   - the trace Outcome is OutcomeSchemaInvalid and Compiled is nil
//   - the transport WAS called (compiler is not short-circuiting on
//     pre-call validation)
func TestCompilerRejectsMalformedJSONWithoutRouting(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "truncated_json", body: `{"version":"v1"`},
		{name: "garbage", body: `not json at all`},
		{name: "missing_required_action_class", body: `{"version":"v1","language":"en","user_goal":"x","side_effect_class":"none","confidence":0.5,"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}}`},
		{name: "unknown_action_class", body: `{"version":"v1","language":"en","user_goal":"x","action_class":"fly","side_effect_class":"none","confidence":0.5,"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}}`},
		{name: "confidence_out_of_range", body: `{"version":"v1","language":"en","user_goal":"x","action_class":"answer","side_effect_class":"none","confidence":1.5,"source_policy":{"requires_citations":false,"allowed_source_kinds":["graph"]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ft := &fakeTransport{resp: intent.CompileResponse{
				SchemaVersion:  "v1",
				CompiledIntent: []byte(tc.body),
			}}
			c, err := intent.NewLLMCompiler(validConfig(), ft)
			if err != nil {
				t.Fatalf("NewLLMCompiler: %v", err)
			}
			ci, trace, err := c.Compile(context.Background(), validRawTurn())
			if err == nil {
				t.Fatalf("expected error for malformed body %q, got nil", tc.body)
			}
			se, ok := intent.IsSchemaError(err)
			if !ok {
				t.Fatalf("expected SchemaError, got %T: %v", err, err)
			}
			if se.Cause != "schema_invalid" && se.Cause != "json_invalid" {
				t.Fatalf("expected cause schema_invalid|json_invalid, got %q", se.Cause)
			}
			if ci.ActionClass != "" {
				t.Fatalf("expected zero CompiledIntent on failure, got action_class=%q", ci.ActionClass)
			}
			if trace.Outcome != intent.OutcomeSchemaInvalid {
				t.Fatalf("expected trace outcome %q, got %q", intent.OutcomeSchemaInvalid, trace.Outcome)
			}
			if trace.Compiled != nil {
				t.Fatalf("expected nil trace.Compiled on failure")
			}
			if ft.calls != 1 {
				t.Fatalf("expected transport.Compile to be called exactly once, got %d", ft.calls)
			}
		})
	}
}

// Sanity: a valid CompiledIntent round-trips and emits OutcomeCompiled.
func TestCompilerAcceptsValidIntent(t *testing.T) {
	body := `{
		"version":"v1",
		"language":"en",
		"user_goal":"check the weather",
		"action_class":"external_lookup",
		"side_effect_class":"external_read",
		"scenario_hint":"weather_query",
		"tool_hints":["location_normalize","weather_lookup"],
		"normalized_request":{"query":"weather tomorrow"},
		"slots":{"window":"tomorrow"},
		"missing_slots":[],
		"confidence":0.92,
		"clarification_prompt":null,
		"safety_flags":[],
		"source_policy":{"requires_citations":true,"allowed_source_kinds":["tool"]}
	}`
	ft := &fakeTransport{resp: intent.CompileResponse{
		SchemaVersion:  "v1",
		CompiledIntent: []byte(body),
	}}
	c, err := intent.NewLLMCompiler(validConfig(), ft)
	if err != nil {
		t.Fatalf("NewLLMCompiler: %v", err)
	}
	ci, trace, err := c.Compile(context.Background(), validRawTurn())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if ci.ActionClass != intent.ActionExternalLookup {
		t.Fatalf("expected action_class external_lookup, got %q", ci.ActionClass)
	}
	if trace.Outcome != intent.OutcomeCompiled {
		t.Fatalf("expected trace outcome %q, got %q", intent.OutcomeCompiled, trace.Outcome)
	}
	if trace.Compiled == nil {
		t.Fatalf("expected trace.Compiled to be populated")
	}
}

// Provider transport errors map to OutcomeProviderError without
// touching the schema validator.
func TestCompilerSurfacesProviderError(t *testing.T) {
	wantErr := errors.New("sidecar down")
	ft := &fakeTransport{err: wantErr}
	c, err := intent.NewLLMCompiler(validConfig(), ft)
	if err != nil {
		t.Fatalf("NewLLMCompiler: %v", err)
	}
	_, trace, err := c.Compile(context.Background(), validRawTurn())
	if err == nil || !strings.Contains(err.Error(), "sidecar down") {
		t.Fatalf("expected wrapped provider error, got %v", err)
	}
	if trace.Outcome != intent.OutcomeProviderError {
		t.Fatalf("expected provider_error outcome, got %q", trace.Outcome)
	}
}
