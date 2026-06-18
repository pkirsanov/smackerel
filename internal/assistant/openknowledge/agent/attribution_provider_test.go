// Spec 096 SCOPE-03 — SCN-096-G04: provider-qualified attribution. The spec 089
// TurnResult.Model contract is carried through ADDITIVELY — the SHAPE is
// unchanged (still the single attribution string stamped once in finalize);
// only the VALUE is now provider-qualified ("<kind>/<backend>", e.g.
// "anthropic/claude-3-5-sonnet"). An answer produced by a selected hosted model
// is attributed to that provider+model and is NEVER coerced to the bare backend
// id or an Ollama name. Two providers' answers are distinguishable by their
// attribution.
//
// Reuses the agent_test.go / synthesis_spec087_test.go harness (recordingLLM,
// spec087Cfg, toolUse, endTurn, webCiteEntry, citationsBlock, newRegistry).
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// TestAttribution_ProviderQualified_Spec096 — the forced-final synthesis turn,
// re-pointed by a per-request override to a PROVIDER-QUALIFIED model id, stamps
// that exact provider-qualified id onto TurnResult.Model. Adversarial: the
// attribution must NOT be the bare backend suffix (qualifier stripped) and must
// NOT be coerced to an Ollama name; and two providers must be distinguishable.
func TestAttribution_ProviderQualified_Spec096(t *testing.T) {
	runProviderQualified := func(t *testing.T, providerModel string) TurnResult {
		t.Helper()
		const maxIter = 2 // iter0 tool turn, iter1 forced-final synthesis turn
		r := newRegistry(t)
		verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &recordingLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(verdict, 80),
		}}
		base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		// The selected hosted model is provider-qualified (the spec 096 catalog
		// id). It re-points the synthesis turn (Fork B); the gather turn keeps
		// the SST baseline.
		a := base.WithModelOverride(modelswitch.Override{SynthesisModel: providerModel})
		got, err := a.Run(context.Background(), "a question")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != StatusSuccess {
			t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
		}
		return got
	}

	const anthropicModel = "anthropic/claude-3-5-sonnet"
	const openaiModel = "openai/gpt-4o"

	anthropic := runProviderQualified(t, anthropicModel)
	openai := runProviderQualified(t, openaiModel)

	// Provider-qualified, verbatim — not coerced to a bare or Ollama name.
	for _, tc := range []struct {
		name, want string
		got        TurnResult
	}{
		{"anthropic", anthropicModel, anthropic},
		{"openai", openaiModel, openai},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Model != tc.want {
				t.Fatalf("TurnResult.Model = %q, want the provider-qualified id %q", tc.got.Model, tc.want)
			}
			// ADVERSARIAL — qualifier must survive: the attribution is NOT the
			// bare backend suffix.
			_, backend, _ := strings.Cut(tc.want, "/")
			if tc.got.Model == backend {
				t.Fatalf("attribution coerced to the BARE backend id %q (provider qualifier stripped)", backend)
			}
			// ADVERSARIAL — never coerced to Ollama.
			if !strings.Contains(tc.got.Model, "/") || strings.HasPrefix(tc.got.Model, "ollama/") || tc.got.Model == "ollama" {
				t.Fatalf("attribution %q is not provider-qualified or was coerced to an Ollama name", tc.got.Model)
			}
		})
	}

	// Two providers' answers are distinguishable by their attribution.
	if anthropic.Model == openai.Model {
		t.Fatalf("two providers MUST be distinguishable by attribution; both stamped %q", anthropic.Model)
	}
}
