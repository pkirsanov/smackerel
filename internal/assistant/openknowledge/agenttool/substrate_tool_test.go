// Substrate-bridge tests for Spec 064 SCOPE-12. Verify the
// TerminationReason → RefusalCause mapping, the success envelope
// shape, and the adversarial G021 path (fabricated-source termination
// MUST produce RefusalFabricatedSourceBlocked with zero sources, even
// if the upstream TurnResult somehow carried a populated Sources
// slice).
package agenttool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	okagent "github.com/smackerel/smackerel/internal/assistant/openknowledge/agent"
)

// TestInit_RegistersSubstrateTool verifies the package init() hook
// registered ToolName with the spec 037 registry. Blank-importing
// this package MUST make agent.Has return true so the open_knowledge
// scenario can list it in allowed_tools.
func TestInit_RegistersSubstrateTool(t *testing.T) {
	if !agent.Has(ToolName) {
		t.Fatalf("agent.Has(%q)=false; init() registration regression", ToolName)
	}
	tool, ok := agent.ByName(ToolName)
	if !ok {
		t.Fatalf("agent.ByName(%q)=false", ToolName)
	}
	if tool.SideEffectClass != agent.SideEffectExternal {
		t.Errorf("SideEffectClass=%q want external", tool.SideEffectClass)
	}
	if tool.OwningPackage != owningPackage {
		t.Errorf("OwningPackage=%q want %q", tool.OwningPackage, owningPackage)
	}
	if tool.Handler == nil {
		t.Errorf("Handler is nil; init() registration regression")
	}
}

// TestMapTerminationToRefusalCause is the exhaustive closed-vocab
// mapping. Every defined TerminationReason MUST map deterministically;
// any new TerminationReason added without updating this table is a
// silent fallback (G028 violation) and this test fails loud.
func TestMapTerminationToRefusalCause(t *testing.T) {
	cases := []struct {
		name   string
		reason okagent.TerminationReason
		want   contracts.RefusalCause
	}{
		{"cap_iterations", okagent.TerminationCapIterations, contracts.RefusalBudgetExhausted},
		{"cap_tokens", okagent.TerminationCapTokens, contracts.RefusalBudgetExhausted},
		{"cap_usd", okagent.TerminationCapUSD, contracts.RefusalBudgetExhausted},
		{"tool_error", okagent.TerminationToolError, contracts.RefusalToolUnavailable},
		{"tool_unavailable", okagent.TerminationToolUnavailable, contracts.RefusalToolUnavailable},
		{"fabricated_source", okagent.TerminationFabricatedSource, contracts.RefusalFabricatedSourceBlocked},
		{"refused", okagent.TerminationRefused, contracts.RefusalDefault},
		{"final_on_refused", okagent.TerminationFinal, contracts.RefusalDefault},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapTerminationToRefusalCause(tc.reason)
			if got != tc.want {
				t.Errorf("MapTerminationToRefusalCause(%q)=%q want %q", tc.reason, got, tc.want)
			}
		})
	}
}

// TestMapTurnResult_Refused verifies every refused TerminationReason
// produces the right envelope: status="refused", canonical body for
// the mapped cause, zero sources.
func TestMapTurnResult_Refused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason okagent.TerminationReason
		cause  contracts.RefusalCause
	}{
		{"cap_iterations", okagent.TerminationCapIterations, contracts.RefusalBudgetExhausted},
		{"cap_tokens", okagent.TerminationCapTokens, contracts.RefusalBudgetExhausted},
		{"cap_usd", okagent.TerminationCapUSD, contracts.RefusalBudgetExhausted},
		{"tool_error", okagent.TerminationToolError, contracts.RefusalToolUnavailable},
		{"tool_unavailable", okagent.TerminationToolUnavailable, contracts.RefusalToolUnavailable},
		{"fabricated_source", okagent.TerminationFabricatedSource, contracts.RefusalFabricatedSourceBlocked},
		{"refused", okagent.TerminationRefused, contracts.RefusalDefault},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := okagent.TurnResult{
				Status:            okagent.StatusRefused,
				TerminationReason: tc.reason,
				RefusalReason:     "irrelevant — body comes from canonical mapping",
			}
			env := MapTurnResult(turn)
			if env.Status != "refused" {
				t.Errorf("Status=%q want refused", env.Status)
			}
			if env.RefusalCause != string(tc.cause) {
				t.Errorf("RefusalCause=%q want %q", env.RefusalCause, tc.cause)
			}
			wantBody := contracts.CanonicalRefusalBodyFor(tc.cause)
			if env.Body != wantBody {
				t.Errorf("Body=%q want %q", env.Body, wantBody)
			}
			if env.Termination != string(tc.reason) {
				t.Errorf("Termination=%q want %q", env.Termination, tc.reason)
			}
			if len(env.Sources) != 0 {
				t.Errorf("Sources len=%d want 0 (refused MUST drop sources)", len(env.Sources))
			}
		})
	}
}

// TestMapTurnResult_Success verifies a successful turn surfaces the
// planner's FinalText plus the verified sources.
func TestMapTurnResult_Success(t *testing.T) {
	turn := okagent.TurnResult{
		Status:            okagent.StatusSuccess,
		FinalText:         "Paris is the capital of France.",
		TerminationReason: okagent.TerminationFinal,
		Sources: []ok.Source{
			{
				Kind: ok.SourceWeb,
				Web: &ok.WebSource{
					URL:         "https://example.test/paris",
					Title:       "Paris",
					Provider:    "searxng",
					ContentHash: "abc",
					Snippet:     "Paris is the capital of France.",
				},
			},
		},
	}
	env := MapTurnResult(turn)
	if env.Status != "success" {
		t.Fatalf("Status=%q want success", env.Status)
	}
	if env.Body != "Paris is the capital of France." {
		t.Errorf("Body=%q", env.Body)
	}
	if env.RefusalCause != "" {
		t.Errorf("RefusalCause=%q want empty (success path)", env.RefusalCause)
	}
	if len(env.Sources) != 1 {
		t.Fatalf("Sources len=%d want 1", len(env.Sources))
	}
	got := env.Sources[0]
	if got["kind"] != "web" || got["url"] != "https://example.test/paris" {
		t.Errorf("source[0]=%+v", got)
	}
}

// TestMapTurnResult_ModelCarried_BothArms_Spec088 — the spec 088 model
// attribution rides the substrate envelope on BOTH the success and the
// refusal arms (structured HTTP metadata, always present when a model ran);
// an empty Model (pre-loop refusal that ran no LLM round) is omitted from the
// JSON envelope via omitempty.
func TestMapTurnResult_ModelCarried_BothArms_Spec088(t *testing.T) {
	t.Run("success_arm", func(t *testing.T) {
		turn := okagent.TurnResult{
			Status:            okagent.StatusSuccess,
			FinalText:         "An answer.",
			TerminationReason: okagent.TerminationFinal,
			Model:             "deepseek-r1:7b",
		}
		env := MapTurnResult(turn)
		if env.Status != "success" {
			t.Fatalf("Status=%q want success", env.Status)
		}
		if env.Model != "deepseek-r1:7b" {
			t.Fatalf("success envelope MUST carry the model, got %q", env.Model)
		}
	})
	t.Run("refusal_arm", func(t *testing.T) {
		turn := okagent.TurnResult{
			Status:            okagent.StatusRefused,
			TerminationReason: okagent.TerminationFabricatedSource,
			Model:             "deepseek-r1:7b",
		}
		env := MapTurnResult(turn)
		if env.Status != "refused" {
			t.Fatalf("Status=%q want refused", env.Status)
		}
		if env.Model != "deepseek-r1:7b" {
			t.Fatalf("refusal envelope MUST carry the model, got %q", env.Model)
		}
	})
	t.Run("empty_model_omitted_in_json", func(t *testing.T) {
		turn := okagent.TurnResult{Status: okagent.StatusRefused, TerminationReason: okagent.TerminationCapUSD}
		env := MapTurnResult(turn)
		if env.Model != "" {
			t.Fatalf("empty model expected, got %q", env.Model)
		}
		b, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(b), `"model"`) {
			t.Fatalf("omitempty MUST drop an empty model key, got %s", string(b))
		}
	})
}

// TestMapTurnResult_FabricatedSource_StripsSources is the adversarial
// G021 case: if the planner emitted a citation list but the cite-back
// verifier rejected it (Status=refused, TerminationFabricatedSource),
// the envelope MUST report zero sources EVEN IF the upstream TurnResult
// accidentally carried Sources. This guard prevents a regression where
// a future TurnResult mutation surfaces unverified sources to the user.
func TestMapTurnResult_FabricatedSource_StripsSources(t *testing.T) {
	turn := okagent.TurnResult{
		Status:            okagent.StatusRefused,
		TerminationReason: okagent.TerminationFabricatedSource,
		FinalText:         "untrusted text (should NOT surface)",
		// Adversarial: populate Sources to verify the mapping drops them.
		Sources: []ok.Source{
			{Kind: ok.SourceWeb, Web: &ok.WebSource{URL: "https://example.test/hallucinated"}},
		},
	}
	env := MapTurnResult(turn)
	if env.Status != "refused" {
		t.Fatalf("Status=%q want refused", env.Status)
	}
	if env.RefusalCause != string(contracts.RefusalFabricatedSourceBlocked) {
		t.Fatalf("RefusalCause=%q want %q", env.RefusalCause, contracts.RefusalFabricatedSourceBlocked)
	}
	if env.Body != contracts.CanonicalRefusalBodyFor(contracts.RefusalFabricatedSourceBlocked) {
		t.Errorf("Body=%q (must be canonical fabricated-source refusal)", env.Body)
	}
	if len(env.Sources) != 0 {
		t.Fatalf("Sources len=%d want 0 (G021: fabricated-source MUST NOT surface unverified citations)", len(env.Sources))
	}
	if strings.Contains(env.Body, "untrusted text") {
		t.Errorf("Body leaked planner FinalText on fabricated-source path: %q", env.Body)
	}
}

// TestHandler_NotWired_ReturnsToolUnavailable verifies the
// Handler-without-SetAgent path returns a structured refusal envelope
// (NOT a Go error) so the executor records a normal scenario outcome.
func TestHandler_NotWired_ReturnsToolUnavailable(t *testing.T) {
	// Ensure no agent is bound for this test; restore at end.
	prior := CurrentAgent()
	SetAgent(nil)
	defer SetAgent(prior)

	args := json.RawMessage(`{"prompt":"hello"}`)
	out, err := Handler(context.Background(), args)
	if err != nil {
		t.Fatalf("Handler err=%v want nil (not-wired surfaces as structured refusal)", err)
	}
	var env outputEnvelope
	if uerr := json.Unmarshal(out, &env); uerr != nil {
		t.Fatalf("unmarshal envelope: %v\nraw=%s", uerr, string(out))
	}
	if env.Status != "refused" {
		t.Errorf("Status=%q want refused", env.Status)
	}
	if env.RefusalCause != string(contracts.RefusalToolUnavailable) {
		t.Errorf("RefusalCause=%q want %q", env.RefusalCause, contracts.RefusalToolUnavailable)
	}
	if env.Termination != "not_wired" {
		t.Errorf("Termination=%q want not_wired", env.Termination)
	}
	if len(env.Sources) != 0 {
		t.Errorf("Sources len=%d want 0", len(env.Sources))
	}
}

// TestHandler_RejectsEmptyPrompt — defensive guard against an empty
// prompt slipping through the substrate input-schema validator.
func TestHandler_RejectsEmptyPrompt(t *testing.T) {
	prior := CurrentAgent()
	SetAgent(nil)
	defer SetAgent(prior)

	cases := []string{
		`{"prompt":""}`,
		`{"prompt":"   "}`,
		`{}`,
	}
	for _, c := range cases {
		_, err := Handler(context.Background(), json.RawMessage(c))
		if err == nil {
			t.Errorf("Handler(%s) err=nil want error", c)
		}
	}
}

// TestHandler_RejectsMalformedJSON
func TestHandler_RejectsMalformedJSON(t *testing.T) {
	_, err := Handler(context.Background(), json.RawMessage(`not json`))
	if err == nil {
		t.Fatalf("Handler malformed err=nil want error")
	}
	if !strings.Contains(err.Error(), "parse args") {
		t.Errorf("err=%q want substring %q", err.Error(), "parse args")
	}
}
