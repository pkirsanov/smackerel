// Spec 088 SCOPE-02 — adversarial + guard tests for the runtime-switchable
// model override at the agent spine: the per-request WithModelOverride clone
// (singleton never mutated, C6), the TurnResult.Model attribution stamped in
// the finalize chokepoint, the synthesis-only re-point (Fork B), the preserved
// trust contracts under an overridden model, and the unchanged iteration
// envelope (the WriteTimeout inputs).
//
// They reuse the shared harness in agent_test.go (fakeLLM, newRegistry,
// baseCfg, toolUse, endTurn) + recordingLLM/requestText from
// reasoning_loop_spec084_test.go + spec087Cfg/webCiteEntry/citationsBlock from
// synthesis_spec087_test.go. spec087Cfg sets Model="gather-model",
// SynthesisModel="synth-model" so the override target is unambiguous.
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

const spec088OverrideModel = "override-synth"

// ── SCN-088-A01 — a valid override re-points the SYNTHESIS turn only ──────────

// TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088 —
// ADVERSARIAL. Under an override the forced-final SYNTHESIS turn (and its
// retry) runs the OVERRIDDEN model; every gather/tool turn keeps the baseline
// tool model. Fails if the override leaks onto a gather turn or never reaches
// synthesis.
func TestAgent_SynthesisOverrideApplied_SynthesisTurnUsesOverriddenModel_Spec088(t *testing.T) {
	t.Run("forced_final_uses_override", func(t *testing.T) {
		const maxIter = 2 // iter0 tool, iter1 forced-final synthesis
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
		a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec088OverrideModel})
		got, err := a.Run(context.Background(), "a question")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got.Status != StatusSuccess {
			t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
		}
		if fl.requests[0].Model != "gather-model" {
			t.Fatalf("gather/tool turn MUST keep the baseline tool model, got %q (override leaked onto gather)", fl.requests[0].Model)
		}
		if fl.requests[maxIter-1].Model != spec088OverrideModel {
			t.Fatalf("forced-final synthesis turn MUST use the OVERRIDDEN model, got %q", fl.requests[maxIter-1].Model)
		}
		if got.Model != spec088OverrideModel {
			t.Fatalf("TurnResult.Model MUST attribute the overridden synthesis model, got %q", got.Model)
		}
	})

	t.Run("synthesis_retry_uses_override", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		verdict := "A grounded answer on retry." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &recordingLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn("", 50),      // forced-final blank → retry
			endTurn(verdict, 80), // retry produces the verdict
		}}
		base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec088OverrideModel})
		got, err := a.Run(context.Background(), "a question")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if len(fl.requests) != 3 {
			t.Fatalf("expected 3 LLM calls (tool, forced-final, retry), got %d", len(fl.requests))
		}
		if fl.requests[2].Model != spec088OverrideModel {
			t.Fatalf("the synthesis RETRY MUST use the overridden model, got %q", fl.requests[2].Model)
		}
		if got.Model != spec088OverrideModel {
			t.Fatalf("attribution after retry MUST be the overridden model, got %q", got.Model)
		}
	})
}

// TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088 — the clone
// contract (C6 / NFR-4): a zero override returns the receiver pointer; a
// non-zero override returns a distinct clone whose SynthesisModel is the
// override and whose gather Model is unchanged; the receiver (SST singleton)
// is NEVER mutated.
func TestAgent_WithModelOverride_ClonesSingletonNeverMutated_Spec088(t *testing.T) {
	r := newRegistry(t)
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("unused", 10)}}
	base, err := New(fl, r, citeback.Verify, spec087Cfg(2, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if same := base.WithModelOverride(modelswitch.Override{}); same != base {
		t.Fatalf("zero override MUST return the receiver pointer (no clone)")
	}

	clone := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec088OverrideModel})
	if clone == base {
		t.Fatalf("non-zero override MUST return a distinct clone")
	}
	if clone.cfg.SynthesisModel != spec088OverrideModel {
		t.Fatalf("clone SynthesisModel = %q, want %q", clone.cfg.SynthesisModel, spec088OverrideModel)
	}
	if clone.cfg.Model != "gather-model" {
		t.Fatalf("clone gather Model leaked the override: %q (Fork B: synthesis-only)", clone.cfg.Model)
	}
	// C6 — the SST singleton is never written.
	if base.cfg.SynthesisModel != "synth-model" {
		t.Fatalf("singleton SynthesisModel MUTATED to %q (C6 violation)", base.cfg.SynthesisModel)
	}
	if base.cfg.Model != "gather-model" {
		t.Fatalf("singleton Model MUTATED to %q (C6 violation)", base.cfg.Model)
	}
}

// ── SCN-088-A03 — no override ⇒ byte-for-byte spec-087 baseline ───────────────

// TestAgent_NoOverride_ByteForByteBaseline_Spec088 — ADVERSARIAL. A zero
// override returns the receiver; the recorded per-turn model sequence is
// exactly the spec-087 baseline (gather-model on tool turns, synth-model on the
// forced-final) and the attribution is the SST synthesis model. Fails if the
// no-override path diverges in any turn.
func TestAgent_NoOverride_ByteForByteBaseline_Spec088(t *testing.T) {
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
	a := base.WithModelOverride(modelswitch.Override{}) // baseline
	if a != base {
		t.Fatalf("zero override MUST return the receiver (baseline path)")
	}
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	wantSeq := []string{"gather-model", "synth-model"}
	for i, w := range wantSeq {
		if fl.requests[i].Model != w {
			t.Fatalf("baseline turn %d model = %q, want %q (no-override path diverged)", i, fl.requests[i].Model, w)
		}
	}
	if got.Model != "synth-model" {
		t.Fatalf("baseline attribution MUST be the SST synthesis model, got %q", got.Model)
	}
}

// ── SCN-088-A04 — TurnResult.Model honest across every terminal path ──────────

// TestAgent_TurnResultModelStamped_AllTerminalPaths_Spec088 — finalize stamps
// TurnResult.Model on success, honest-salvage, refuse, AND early-StopEndTurn.
// Early-StopEndTurn under a SYNTHESIS override still reports the gather model
// (the synthesis seat was never reached) — honest per CT-3.
func TestAgent_TurnResultModelStamped_AllTerminalPaths_Spec088(t *testing.T) {
	t.Run("success_forced_final", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(verdict, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess || got.Model != "synth-model" {
			t.Fatalf("success path: status=%q model=%q want success/synth-model", got.Status, got.Model)
		}
	})

	t.Run("honest_salvage", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn("", 50), // forced-final blank
			endTurn("", 50), // retry blank → salvage
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess || !strings.Contains(got.FinalText, "couldn't directly answer") {
			t.Fatalf("salvage path expected, got status=%q body=%q", got.Status, got.FinalText)
		}
		if got.Model != "synth-model" {
			t.Fatalf("salvage attribution MUST be the synthesis model that ran (then failed), got %q", got.Model)
		}
	})

	t.Run("refuse_fabricated", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		final := "Ungrounded." + citationsBlock(webCiteEntry("https://fabricated.test/zzz", "nothash"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(final, 80),
		}}
		a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusRefused {
			t.Fatalf("refuse path expected, got status=%q", got.Status)
		}
		if got.Model != "synth-model" {
			t.Fatalf("refusal attribution MUST be the synthesis model that produced the fabricated citation, got %q", got.Model)
		}
	})

	t.Run("early_stop_under_override_reports_gather_model", func(t *testing.T) {
		const maxIter = 3 // iter0 tool, iter1 EARLY endTurn (not forced-final)
		r := newRegistry(t)
		verdict := "An early grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(verdict, 80), // iter1 < maxIter-1=2 ⇒ early stop, tool model produced the text
		}}
		base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec088OverrideModel})
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess {
			t.Fatalf("early-stop success expected, got status=%q reason=%q", got.Status, got.RefusalReason)
		}
		if got.Model != "gather-model" {
			t.Fatalf("early-StopEndTurn under a synthesis override MUST attribute the gather model that actually produced the text (synthesis seat never reached), got %q", got.Model)
		}
	})
}

// ── SCN-088-A05 — trust contracts hold under an overridden model ──────────────

// TestAgent_TrustContractsHoldUnderOverride_Spec088 — ADVERSARIAL. Under an
// allowlisted SYNTHESIS override: (a) a fabricated citation is still refused by
// the post-<think>-strip cite-back verifier (enforce mode); (b) a reasoning-
// model <think> chain-of-thought never leaks into the body and is never cited.
// The trust perimeter runs on the turn OUTPUT and is inherently model-agnostic;
// these prove the override does not weaken it. (The provenance gate + capture-
// as-fallback are facade-level, model-agnostic hooks proven in the facade
// tests; the override never reaches them.)
func TestAgent_TrustContractsHoldUnderOverride_Spec088(t *testing.T) {
	t.Run("fabricated_citation_still_refused", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		final := "Confident but ungrounded." + citationsBlock(webCiteEntry("https://fabricated.test/zzz", "nothash"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(final, 80),
		}}
		base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec088OverrideModel})
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusRefused || got.TerminationReason != TerminationFabricatedSource {
			t.Fatalf("a fabricated citation under an override MUST be refused, got status=%q termination=%q", got.Status, got.TerminationReason)
		}
		if got.Model != spec088OverrideModel {
			t.Fatalf("the refusal MUST still attribute the overridden synthesis model, got %q", got.Model)
		}
	})

	t.Run("think_never_leaks_never_cited", func(t *testing.T) {
		const maxIter = 2
		r := newRegistry(t)
		think := "<think>I could pad this with https://fabricated.test/zzz (hash badhash) but I won't.</think>"
		verdict := "wa-town-A wins on winter-low grounds."
		final := think + verdict + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
		fl := &fakeLLM{t: t, responses: []llm.Result{
			toolUse("w0", "fake_web", `{"query":"q"}`, 100),
			endTurn(final, 80),
		}}
		base, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		a := base.WithModelOverride(modelswitch.Override{SynthesisModel: spec088OverrideModel})
		got, _ := a.Run(context.Background(), "q")
		if got.Status != StatusSuccess {
			t.Fatalf("status=%q reason=%q want success", got.Status, got.RefusalReason)
		}
		if strings.Contains(got.FinalText, "fabricated.test") || strings.Contains(got.FinalText, "<think>") {
			t.Fatalf("<think> content leaked into the body under an override.\nbody=%q", got.FinalText)
		}
		if len(got.Sources) != 1 {
			t.Fatalf("only the real cited source must attach under an override, got %d", len(got.Sources))
		}
	})
}

// ── SCN-088-A08 — synthesis-only switch adds no turns (WriteTimeout inputs) ───

// TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088 —
// WithModelOverride changes ONLY SynthesisModel; MaxIterations and
// SynthesisRetryBudget (the (max_iterations + synthesis_retry_budget) ×
// llm_timeout_ms = 4200s WriteTimeout formula inputs) are unchanged, so a
// switched (even slower) synthesis model adds no turns.
func TestAgent_SynthesisOverride_PreservesIterationEnvelope_Spec088(t *testing.T) {
	r := newRegistry(t)
	fl := &fakeLLM{t: t, responses: []llm.Result{endTurn("unused", 10)}}
	base, err := New(fl, r, citeback.Verify, spec087Cfg(6, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clone := base.WithModelOverride(modelswitch.Override{SynthesisModel: "slow-reasoner:70b"})
	if clone.cfg.MaxIterations != base.cfg.MaxIterations {
		t.Fatalf("override changed MaxIterations %d -> %d (a WriteTimeout input)", base.cfg.MaxIterations, clone.cfg.MaxIterations)
	}
	if clone.cfg.SynthesisRetryBudget != base.cfg.SynthesisRetryBudget {
		t.Fatalf("override changed SynthesisRetryBudget %d -> %d (a WriteTimeout input)", base.cfg.SynthesisRetryBudget, clone.cfg.SynthesisRetryBudget)
	}
	if clone.cfg.SynthesisModel != "slow-reasoner:70b" {
		t.Fatalf("override should re-point SynthesisModel, got %q", clone.cfg.SynthesisModel)
	}
	if base.cfg.MaxIterations != 6 || base.cfg.SynthesisRetryBudget != 1 {
		t.Fatalf("baseline (6+1)×600s=4200s envelope inputs changed: maxIter=%d retry=%d", base.cfg.MaxIterations, base.cfg.SynthesisRetryBudget)
	}
}
