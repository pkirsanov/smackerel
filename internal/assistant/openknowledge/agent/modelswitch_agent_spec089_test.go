// Spec 089 SCOPE-03 — adversarial + guard tests for the four-axis runtime
// selection at the agent spine: the gather-turn override clone (Fork C;
// singleton never mutated, C6), the TurnResult.Model + TurnResult.GatherModel
// attribution stamped in the finalize chokepoint, the no-selection byte-for-
// byte baseline, the FR-13 <CITATIONS>/marker scaffolding strip on the salvage
// arms, the FR-14 forced-final escalated-retry-before-salvage regression, the
// trust contracts preserved under ANY selection, and the unchanged iteration
// envelope (the WriteTimeout inputs).
//
// Reuses the shared harness: fakeLLM/newRegistry/toolUse/endTurn (agent_test.go),
// recordingLLM (reasoning_loop_spec084_test.go), spec087Cfg/citationsBlock/
// webCiteEntry (synthesis_spec087_test.go). spec087Cfg sets Model="gather-model"
// (the gather/tool turns) and SynthesisModel="synth-model" (the forced-final
// turn) so the per-turn target of each override is unambiguous.
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// ── SCN-089-A01 — no selection ⇒ byte-for-byte the SST baseline ───────────────

// TestAgent_NoSelection_UsesSstDefaultSynthesis_ByteForByteBaseline_Spec089 —
// ADVERSARIAL. An all-default Effective yields a zero Override ⇒
// WithModelOverride returns the receiver; the recorded per-turn model sequence
// is exactly the spec-087/088 baseline (gather-model on tool turns, synth-model
// on the forced-final) and both attributions name the SST models. Fails if the
// no-selection path diverges in any turn (the regression NFR-4 forbids).
func TestAgent_NoSelection_UsesSstDefaultSynthesis_ByteForByteBaseline_Spec089(t *testing.T) {
	const maxIter = 2
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
	// An all-default Effective.Override() is the zero Override ⇒ receiver.
	zeroEff := modelswitch.Effective{
		SynthesisModel: "synth-model", SynthesisSource: modelswitch.SourceDefault,
		GatherModel: "gather-model", GatherSource: modelswitch.SourceDefault,
	}
	if !zeroEff.Override().IsZero() {
		t.Fatalf("an all-default Effective MUST produce a zero Override (baseline)")
	}
	if a := base.WithModelOverride(zeroEff.Override()); a != base {
		t.Fatalf("a zero Override MUST return the receiver pointer (no clone)")
	}
	got, err := base.Run(context.Background(), "q")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	wantSeq := []string{"gather-model", "synth-model"}
	for i, w := range wantSeq {
		if fl.requests[i].Model != w {
			t.Fatalf("baseline turn %d model=%q want %q (no-selection path diverged)", i, fl.requests[i].Model, w)
		}
	}
	if got.Model != "synth-model" {
		t.Fatalf("baseline answering attribution want synth-model, got %q", got.Model)
	}
	if got.GatherModel != "gather-model" {
		t.Fatalf("baseline gather attribution want gather-model, got %q", got.GatherModel)
	}
}

// ── SCN-089-A07 — a gather override re-points the gather turns only ───────────

// TestAgent_WithModelOverride_GatherClonePointsCfgModel_SingletonUnmutated_Spec089
// — ADVERSARIAL. Under a gather override the gather/tool turns run the
// OVERRIDDEN cfg.Model; the synthesis turn resolves by precedence (here the
// baseline synth-model, no synthesis override); the SST singleton's
// Model/SynthesisModel are byte-for-byte unchanged (C6). Fails if the override
// leaks onto the synthesis turn or mutates the singleton.
func TestAgent_WithModelOverride_GatherClonePointsCfgModel_SingletonUnmutated_Spec089(t *testing.T) {
	t.Run("gather_only_override", func(t *testing.T) {
		const maxIter = 2 // iter0 gather/tool, iter1 forced-final synthesis
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
		clone := base.WithModelOverride(modelswitch.Override{GatherModel: "gather-override"})
		if clone == base {
			t.Fatalf("a non-zero gather override MUST return a distinct clone")
		}
		got, err := clone.Run(context.Background(), "q")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != StatusSuccess {
			t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
		}
		if fl.requests[0].Model != "gather-override" {
			t.Fatalf("gather turn MUST use the OVERRIDDEN gather model, got %q", fl.requests[0].Model)
		}
		if fl.requests[maxIter-1].Model != "synth-model" {
			t.Fatalf("synthesis turn MUST keep the baseline synth model (no synthesis override), got %q (override leaked onto synthesis)", fl.requests[maxIter-1].Model)
		}
		if got.GatherModel != "gather-override" {
			t.Fatalf("TurnResult.GatherModel MUST attribute the overridden gather model, got %q", got.GatherModel)
		}
		// C6 — the SST singleton is never written.
		if base.cfg.Model != "gather-model" {
			t.Fatalf("singleton gather Model MUTATED to %q (C6 violation)", base.cfg.Model)
		}
		if base.cfg.SynthesisModel != "synth-model" {
			t.Fatalf("singleton SynthesisModel MUTATED to %q (C6 violation)", base.cfg.SynthesisModel)
		}
	})

	t.Run("both_turns_overridden_independently", func(t *testing.T) {
		const maxIter = 2
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
		clone := base.WithModelOverride(modelswitch.Override{SynthesisModel: "synth-override", GatherModel: "gather-override"})
		got, err := clone.Run(context.Background(), "q")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if fl.requests[0].Model != "gather-override" {
			t.Fatalf("gather turn want gather-override, got %q", fl.requests[0].Model)
		}
		if fl.requests[maxIter-1].Model != "synth-override" {
			t.Fatalf("synthesis turn want synth-override, got %q", fl.requests[maxIter-1].Model)
		}
		if got.Model != "synth-override" || got.GatherModel != "gather-override" {
			t.Fatalf("attribution want model=synth-override gather=gather-override, got model=%q gather=%q", got.Model, got.GatherModel)
		}
		if base.cfg.Model != "gather-model" || base.cfg.SynthesisModel != "synth-model" {
			t.Fatalf("singleton MUTATED (C6): Model=%q SynthesisModel=%q", base.cfg.Model, base.cfg.SynthesisModel)
		}
	})
}

// ── SCN-089-A12 — TurnResult.Model + GatherModel honest across every path ─────

// TestAgent_TurnResultModelAndGatherModelStamped_AllTerminalPaths_Spec089 — the
// finalize chokepoint stamps BOTH the answering Model and the GatherModel on
// success, honest-salvage, refuse, and early-StopEndTurn. GatherModel always
// reports the gather model that ran (a.cfg.Model); Model reports the model that
// actually produced the final text (honest per CT-4).
func TestAgent_TurnResultModelAndGatherModelStamped_AllTerminalPaths_Spec089(t *testing.T) {
	t.Run("success_forced_final", func(t *testing.T) {
		r := newRegistry(t)
		verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(verdict, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(2, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess || got.Model != "synth-model" || got.GatherModel != "gather-model" {
			t.Fatalf("success: status=%q model=%q gather=%q want success/synth-model/gather-model", got.Status, got.Model, got.GatherModel)
		}
	})

	t.Run("honest_salvage", func(t *testing.T) {
		r := newRegistry(t)
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn("", 50), // forced-final blank
			endTurn("", 50), // retry blank → salvage
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(2, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess || !strings.Contains(got.FinalText, "couldn't directly answer") {
			t.Fatalf("salvage path expected, got status=%q body=%q", got.Status, got.FinalText)
		}
		if got.GatherModel != "gather-model" {
			t.Fatalf("salvage MUST stamp the gather model that ran, got %q", got.GatherModel)
		}
	})

	t.Run("refuse_fabricated", func(t *testing.T) {
		r := newRegistry(t)
		final := "Ungrounded." + citationsBlock(webCiteEntry("https://fabricated.test/zzz", "nothash"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(final, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(2, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusRefused {
			t.Fatalf("refuse path expected, got status=%q", got.Status)
		}
		if got.GatherModel != "gather-model" {
			t.Fatalf("refusal MUST stamp the gather model that ran, got %q", got.GatherModel)
		}
	})

	t.Run("early_stop_reports_gather_model", func(t *testing.T) {
		const maxIter = 3 // iter0 tool, iter1 EARLY endTurn (< maxIter-1)
		r := newRegistry(t)
		verdict := "An early grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(verdict, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess {
			t.Fatalf("early-stop success expected, got status=%q reason=%q", got.Status, got.RefusalReason)
		}
		if got.Model != "gather-model" || got.GatherModel != "gather-model" {
			t.Fatalf("early-StopEndTurn MUST attribute the gather model on BOTH Model + GatherModel, got model=%q gather=%q", got.Model, got.GatherModel)
		}
	})
}

// ── SCN-089-A09 — no <CITATIONS>/marker scaffolding leaks into a salvage body ─

// TestAgent_StripContractScaffolding_NoCitationsLeakInSalvageBody_Spec089 —
// ADVERSARIAL (FR-13). A salvage-arm body carrying a residual/unterminated
// <CITATIONS> fragment or the "<one synthesized answer…>" contract marker is
// scrubbed before finalize; neither reaches the user body. Fails if the
// scaffolding survives into the body.
func TestAgent_StripContractScaffolding_NoCitationsLeakInSalvageBody_Spec089(t *testing.T) {
	t.Run("stray_unterminated_citations_block_stripped", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		// A real text answer + a MALFORMED, unterminated <CITATIONS> fragment
		// (no closing tag) ⇒ parseCitations fails ⇒ the missing-CITATIONS
		// salvage surfaces the trimmed text. stripContractScaffolding MUST
		// scrub the stray fragment first.
		leaky := "The answer is forty-two.\n<CITATIONS>\n[{\"kind\":\"web\""
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(leaky, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess {
			t.Fatalf("missing-CITATIONS salvage expected success, got status=%q reason=%q body=%q", got.Status, got.RefusalReason, got.FinalText)
		}
		if strings.Contains(got.FinalText, "<CITATIONS>") {
			t.Fatalf("FR-13 BREACH: <CITATIONS> scaffolding leaked into the body: %q", got.FinalText)
		}
		if got.FinalText != "The answer is forty-two." {
			t.Fatalf("body MUST be the clean answer after the scaffolding strip, got %q", got.FinalText)
		}
	})

	t.Run("contract_marker_stripped", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		leaky := "Here is the answer. <one synthesized answer that resolves the question>"
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(leaky, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess {
			t.Fatalf("salvage expected success, got status=%q body=%q", got.Status, got.FinalText)
		}
		if strings.Contains(got.FinalText, "<one synthesized answer") {
			t.Fatalf("FR-13 BREACH: the contract marker leaked into the body: %q", got.FinalText)
		}
		if got.FinalText != "Here is the answer." {
			t.Fatalf("body MUST be the clean answer after the marker strip, got %q", got.FinalText)
		}
	})
}

// ── SCN-089-A10 — empty forced-final ⇒ escalated retry, THEN honest salvage ───

// TestAgent_ForcedFinalEmpty_EscalatedRetryThenHonestSalvage_Spec089 —
// REGRESSION / ADVERSARIAL (FR-14; the 32b-Q6 shape). An empty forced-final
// fires EXACTLY one escalated retry (synthesis_retry_budget=1); a still-empty
// retry falls to the honest snippet salvage WITH sources. The user never
// receives a silently-empty body. Fails if the retry is skipped or a blank
// surfaces.
func TestAgent_ForcedFinalEmpty_EscalatedRetryThenHonestSalvage_Spec089(t *testing.T) {
	const maxIter = 2
	r := newRegistry(t)
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn("", 50), // forced-final blank
		endTurn("", 50), // escalated retry STILL blank → honest salvage
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1)) // retryBudget=1
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, _ := a.Run(context.Background(), "q")
	if len(fl.requests) != 3 {
		t.Fatalf("expected exactly 1 escalated retry (3 LLM calls: tool, forced-final, retry), got %d", len(fl.requests))
	}
	if got.Status != StatusSuccess || !strings.Contains(got.FinalText, "couldn't directly answer") {
		t.Fatalf("blank forced-final + blank retry MUST fall to honest salvage, got status=%q body=%q", got.Status, got.FinalText)
	}
	if len(got.Sources) == 0 {
		t.Fatalf("honest salvage MUST carry sources (never a silently-empty body)")
	}
}

// ── SCN-089-A05/A08 — trust contracts hold under ANY selection ────────────────

// TestAgent_TrustContractsHoldUnderAnySelection_Spec089 — ADVERSARIAL. The
// trust perimeter runs on the turn OUTPUT and is model-/selection-agnostic:
// under no selection, a synthesis override, and a gather override a fabricated
// citation is STILL refused (cite-back enforce), and a <think> chain is STILL
// stripped from the body. Fails if any selection weakens a trust contract.
func TestAgent_TrustContractsHoldUnderAnySelection_Spec089(t *testing.T) {
	selections := []struct {
		name string
		ov   modelswitch.Override
	}{
		{"default", modelswitch.Override{}},
		{"synthesis_override", modelswitch.Override{SynthesisModel: "override-synth"}},
		{"gather_override", modelswitch.Override{GatherModel: "gather-override"}},
	}
	for _, sel := range selections {
		t.Run("fabricated_citation_refused_"+sel.name, func(t *testing.T) {
			r := newRegistry(t)
			final := "Ungrounded claim." + citationsBlock(webCiteEntry("https://fabricated.test/zzz", "nothash"))
			fl := &fakeLLM{t: t, responses: []llm.Result{
				toolUse("w0", "fake_web", `{"query":"q"}`, 100),
				endTurn(final, 80),
			}}
			base, err := New(fl, r, citeback.Verify, spec087Cfg(2, 1))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got, _ := base.WithModelOverride(sel.ov).Run(context.Background(), "q")
			if got.Status != StatusRefused {
				t.Fatalf("[%s] a fabricated citation MUST be refused under any selection, got status=%q", sel.name, got.Status)
			}
		})
	}

	t.Run("think_stripped_under_override", func(t *testing.T) {
		r := newRegistry(t)
		final := "<think>secret reasoning</think>The grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(final, 80),
		}}
		base, err := New(fl, r, citeback.Verify, spec087Cfg(2, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := base.WithModelOverride(modelswitch.Override{SynthesisModel: "override-synth"}).Run(context.Background(), "q")
		if got.Status != StatusSuccess {
			t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
		}
		if strings.Contains(got.FinalText, "<think>") || strings.Contains(got.FinalText, "secret reasoning") {
			t.Fatalf("<think> MUST be stripped from the body under any selection, got %q", got.FinalText)
		}
	})
}

// ── NFR-2 — any selection preserves the iteration envelope (WriteTimeout) ─────

// TestAgent_AnySelection_PreservesIterationEnvelope_Spec089 — WithModelOverride
// changes ONLY Model/SynthesisModel; MaxIterations and SynthesisRetryBudget (the
// WriteTimeout = (max_iterations + synthesis_retry_budget) × llm_timeout_ms
// inputs) are unchanged under a synthesis, gather, or combined override ⇒ the
// documented (6+1)×600s = 4200s envelope is preserved.
func TestAgent_AnySelection_PreservesIterationEnvelope_Spec089(t *testing.T) {
	r := newRegistry(t)
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("unused", 10)}}
	base, err := New(fl, r, citeback.Verify, spec087Cfg(6, 1)) // the WriteTimeout inputs
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, ov := range []modelswitch.Override{
		{SynthesisModel: "override-synth"},
		{GatherModel: "gather-override"},
		{SynthesisModel: "override-synth", GatherModel: "gather-override"},
	} {
		clone := base.WithModelOverride(ov)
		if clone.cfg.MaxIterations != 6 {
			t.Fatalf("override %+v changed MaxIterations to %d (WriteTimeout input MUST be unchanged)", ov, clone.cfg.MaxIterations)
		}
		if clone.cfg.SynthesisRetryBudget != 1 {
			t.Fatalf("override %+v changed SynthesisRetryBudget to %d (WriteTimeout input MUST be unchanged)", ov, clone.cfg.SynthesisRetryBudget)
		}
	}
}
