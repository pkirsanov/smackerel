// Spec 087 — adversarial + guard tests for the open-knowledge GENUINE
// SYNTHESIS turn: split synthesis model, <think> stripping, structured
// forced-final + retry-before-salvage, and the preserved trust contracts.
//
// These tests assert on the agent's UN-REDACTED assembled TurnResult and on
// the per-iteration request stream. They are non-tautological: the four
// behavioral-change tests (think-strip, model-swap, retry-before-salvage,
// think-no-leak) FAIL against the pre-spec-087 agent.go and PASS only after
// the synthesis changes land. The guard tests (genuine comparison verdict,
// fabricated-citation rejection, retry-exhausted honest salvage) protect the
// preserved behavior + trust contracts.
//
// They reuse the shared harness in agent_test.go (fakeLLM, newRegistry,
// baseCfg, toolUse, endTurn) and recordingLLM/seqWebTool/requestText from
// reasoning_loop_spec084_test.go.
package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
)

// spec087Cfg returns a baseCfg with a DISTINCT synthesis model and the given
// retry budget so the spec-087 split-model + retry paths are exercised.
func spec087Cfg(maxIter, retryBudget int) Config {
	cfg := baseCfg(maxIter, 1_000_000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 })
	cfg.Model = "gather-model"
	cfg.SynthesisModel = "synth-model"
	cfg.SynthesisRetryBudget = retryBudget
	return cfg
}

func webCiteEntry(url, hash string) string {
	return fmt.Sprintf(`{"kind":"web","url":%q,"content_hash":%q}`, url, hash)
}

func citationsBlock(entries ...string) string {
	return "\n<CITATIONS>[" + strings.Join(entries, ",") + "]</CITATIONS>"
}

// ── SCN-087-A01 — reasoning-model <think> stripped, verdict returned ─────────

// TestAgent_SynthesisThinkBlockStripped_VerdictReturned_Spec087 — ADVERSARIAL.
// The forced-final synthesis turn returns a deepseek-r1-style
// <think>...</think> chain-of-thought followed by a genuine cited verdict.
// The pre-spec-087 agent does NOT strip <think>, so the think text leaks into
// the user body; the "body has no <think>" assertion FAILS on the old code and
// PASSES only after stripThinkBlocks lands. The verdict is returned verbatim
// (no honest-salvage frame) and cite-back accepted the citation post-strip.
func TestAgent_SynthesisThinkBlockStripped_VerdictReturned_Spec087(t *testing.T) {
	const maxIter = 2   // iter0 tool call, iter1 forced-final synthesis
	r := newRegistry(t) // fakeWebTool: url=https://example.test/x hash=deadbeef snippet="hello"
	think := "<think>Weighing wa-town-A' maritime winters against wa-town-B's inland frost risk for pomegranates.</think>"
	verdict := "wa-town-A is the better choice: its milder maritime winters rarely reach the lows that kill pomegranates, while wa-town-B's inland frost risk is higher."
	final := think + verdict + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"pomegranate climate wa-town-A wa-town-B"}`, 100),
		endTurn(final, 80),
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is a better place to grow pomegranate, wa-town-A or wa-town-B?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess || got.TerminationReason != TerminationFinal {
		t.Fatalf("Status=%q termination=%q reason=%q want success/final", got.Status, got.TerminationReason, got.RefusalReason)
	}
	if strings.Contains(got.FinalText, "<think>") || strings.Contains(got.FinalText, "Weighing wa-town-A") {
		t.Fatalf("SCN-087-A01: <think> chain-of-thought leaked into the user body.\nbody=%q", got.FinalText)
	}
	if got.FinalText != verdict {
		t.Fatalf("SCN-087-A01: verdict must be returned verbatim (post-strip, citations removed).\ngot=%q\nwant=%q", got.FinalText, verdict)
	}
	if strings.Contains(got.FinalText, "couldn't directly answer") {
		t.Fatalf("SCN-087-A01: a genuine cited synthesis must NOT be wrapped in the honest-salvage frame.\nbody=%q", got.FinalText)
	}
	if len(got.Sources) != 1 || got.Sources[0].Kind != ok.SourceWeb {
		t.Fatalf("SCN-087-A01: expected exactly 1 verified web source, got %+v", got.Sources)
	}
}

// ── SCN-087-A02 — forced-final uses the synthesis model, tool turns the tool model ─

// TestAgent_ForcedFinalUsesSynthesisModel_ToolTurnsUseToolModel_Spec087 —
// ADVERSARIAL. The pre-spec-087 loop issues EVERY request with cfg.Model, so
// the forced-final request's Model == the gather model; the assertion that the
// forced-final used the distinct synthesis model FAILS on the old code and
// PASSES only after the model swap lands. Also proves tools are stripped on the
// forced-final turn.
func TestAgent_ForcedFinalUsesSynthesisModel_ToolTurnsUseToolModel_Spec087(t *testing.T) {
	const maxIter = 2
	r := newRegistry(t)
	verdict := "A grounded answer." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(verdict, 80),
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	if len(fl.requests) != maxIter {
		t.Fatalf("recorded %d requests, want %d", len(fl.requests), maxIter)
	}
	if fl.requests[0].Model != "gather-model" {
		t.Fatalf("SCN-087-A02: tool-calling turn must use the tool model (gather-model), got %q", fl.requests[0].Model)
	}
	if fl.requests[maxIter-1].Model != "synth-model" {
		t.Fatalf("SCN-087-A02: forced-final synthesis turn must use the synthesis model (synth-model), got %q", fl.requests[maxIter-1].Model)
	}
	if fl.requests[maxIter-1].Tools != nil {
		t.Fatalf("SCN-087-A02: tools must be stripped on the forced-final turn, got %d tools", len(fl.requests[maxIter-1].Tools))
	}
}

// ── SCN-087-A03 — comparison synthesis verdict returned, NOT salvage ─────────

// TestAgent_ComparisonSynthesisVerdict_NotSalvage_Spec087 — GUARD. Two DISTINCT
// tool calls gather side X and side Y; the forced-final synthesis produces a
// real cited verdict. The verdict is returned verbatim with the verified
// citations — NOT the honest-salvage snippet wall. This is the headline happy
// path the whole spec exists to make the default outcome.
func TestAgent_ComparisonSynthesisVerdict_NotSalvage_Spec087(t *testing.T) {
	const maxIter = 3 // iter0 toolX, iter1 toolY (+reflect nudge), iter2 forced-final
	counter := 0
	r := ok.NewRegistry([]string{"seq_web"})
	if err := r.Register(seqWebTool{
		snips:  []string{"wa-town-A: mild maritime winters, rarely below freezing.", "wa-town-B: cooler inland nights, higher frost risk."},
		urls:   []string{"https://cmp.test/x", "https://cmp.test/y"},
		hashes: []string{"hx", "hy"},
		i:      &counter,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	verdict := "wa-town-A is better for pomegranates: its maritime winters avoid the hard frosts that wa-town-B's inland nights bring."
	final := verdict + citationsBlock(webCiteEntry("https://cmp.test/x", "hx"), webCiteEntry("https://cmp.test/y", "hy"))
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("wx", "seq_web", `{"query":"wa-town-A pomegranate climate"}`, 100),
		toolUse("wy", "seq_web", `{"query":"wa-town-B pomegranate climate"}`, 100),
		endTurn(final, 80),
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is a better place to grow pomegranate, wa-town-A or wa-town-B?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess || got.TerminationReason != TerminationFinal {
		t.Fatalf("Status=%q termination=%q reason=%q want success/final", got.Status, got.TerminationReason, got.RefusalReason)
	}
	if got.FinalText != verdict {
		t.Fatalf("SCN-087-A03: the cited comparison verdict must be returned verbatim.\ngot=%q\nwant=%q", got.FinalText, verdict)
	}
	if strings.Contains(got.FinalText, "couldn't directly answer") {
		t.Fatalf("SCN-087-A03: a genuine verdict must NOT be the honest-salvage wall.\nbody=%q", got.FinalText)
	}
	if len(got.Sources) != 2 {
		t.Fatalf("SCN-087-A03: expected 2 verified sources (both sides), got %d", len(got.Sources))
	}
}

// ── SCN-087-A04 — retry-before-salvage rescues an empty forced-final ─────────

// TestAgent_RetryBeforeSalvage_RescuesEmptyForcedFinal_Spec087 — ADVERSARIAL.
// The first forced-final synthesis returns EMPTY; with synthesis_retry_budget=1
// the agent re-issues the synthesis turn with an escalated prompt, which now
// produces a real cited verdict. The pre-spec-087 agent has NO retry — the empty
// forced-final fires the honest snippet salvage immediately, so the assertion
// that the verdict (not salvage) is returned FAILS on the old code.
func TestAgent_RetryBeforeSalvage_RescuesEmptyForcedFinal_Spec087(t *testing.T) {
	const maxIter = 2
	r := newRegistry(t)
	verdict := "wa-town-A edges out wa-town-B for pomegranates on winter-low grounds." + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"pomegranate climate comparison"}`, 100),
		endTurn("", 50),      // forced-final blank → eligible for retry
		endTurn(verdict, 80), // retry produces the real cited verdict
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is a better place to grow pomegranate, wa-town-A or wa-town-B?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess || got.TerminationReason != TerminationFinal {
		t.Fatalf("Status=%q termination=%q reason=%q want success/final", got.Status, got.TerminationReason, got.RefusalReason)
	}
	if len(fl.requests) != 3 {
		t.Fatalf("SCN-087-A04: expected 3 LLM calls (tool, forced-final, retry), got %d", len(fl.requests))
	}
	// The retry (3rd request) MUST carry the escalated prompt and use the
	// synthesis model; the first forced-final (2nd request) MUST NOT carry it.
	const retryMarker = "Output ONLY your final answer now"
	if strings.Contains(requestText(fl.requests[1]), retryMarker) {
		t.Fatalf("SCN-087-A04: the escalated retry prompt leaked onto the first forced-final request")
	}
	if !strings.Contains(requestText(fl.requests[2]), retryMarker) {
		t.Fatalf("SCN-087-A04: the synthesis retry request must carry the escalated prompt.\nreq=%q", requestText(fl.requests[2]))
	}
	if fl.requests[2].Model != "synth-model" {
		t.Fatalf("SCN-087-A04: the synthesis retry must use the synthesis model, got %q", fl.requests[2].Model)
	}
	if strings.Contains(got.FinalText, "couldn't directly answer") {
		t.Fatalf("SCN-087-A04: the retry succeeded, so honest salvage must NOT fire.\nbody=%q", got.FinalText)
	}
	if !strings.Contains(got.FinalText, "wa-town-A edges out wa-town-B") {
		t.Fatalf("SCN-087-A04: the retry's verdict must be returned.\nbody=%q", got.FinalText)
	}
}

// ── SCN-087-A05 — trust contracts preserved (guards) ─────────────────────────

// TestAgent_FabricatedCitationInSynthesis_StillRefused_Spec087 — GUARD. The
// synthesis turn emits a citation that does not hash-match any recorded tool
// result; the cite-back verifier (enforce mode, running on the post-<think>-strip
// text) MUST replace the answer with the canonical refusal.
func TestAgent_FabricatedCitationInSynthesis_StillRefused_Spec087(t *testing.T) {
	const maxIter = 2
	r := newRegistry(t)
	// Fabricated citation: URL + hash that no tool result recorded.
	final := "A confident-looking but ungrounded verdict." + citationsBlock(webCiteEntry("https://fabricated.test/zzz", "nothash"))
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(final, 80),
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused || got.TerminationReason != TerminationFabricatedSource {
		t.Fatalf("SCN-087-A05: a fabricated synthesis citation must be refused, got status=%q termination=%q", got.Status, got.TerminationReason)
	}
}

// TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087 — GUARD. When every
// synthesis attempt (original + retries) comes back empty, the spec-084 honest
// snippet salvage MUST still fire (honest prefix + capped sources, not a
// zero-source refusal). Proves the genuine-failure fallback is preserved.
func TestAgent_RetryBudgetExhausted_HonestSalvage_Spec087(t *testing.T) {
	const maxIter = 2
	r := newRegistry(t) // fakeWebTool snippet="hello"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"some topic"}`, 100),
		endTurn("", 50), // forced-final blank
		endTurn("", 50), // retry also blank → salvage
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "tell me about some topic")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success (salvage)", got.Status, got.RefusalReason)
	}
	if !strings.Contains(got.FinalText, "couldn't directly answer") {
		t.Fatalf("SCN-087-A05: retry-exhausted synthesis must fall back to the honest salvage frame.\nbody=%q", got.FinalText)
	}
	if !strings.Contains(got.FinalText, "hello") {
		t.Fatalf("SCN-087-A05: honest salvage must still carry the retrieved finding.\nbody=%q", got.FinalText)
	}
	if len(got.Sources) == 0 {
		t.Fatalf("SCN-087-A05: honest salvage must still carry sources (not a zero-source refusal)")
	}
}

// TestAgent_ThinkBlockNeverLeaksNeverCited_Spec087 — ADVERSARIAL. The synthesis
// turn's <think> block contains a fabricated URL. After stripping, that URL must
// NOT appear in the user body and must NOT be treated as a citation; only the
// real cited source is attached. The pre-spec-087 agent does not strip <think>,
// so the fabricated URL leaks into the body — the assertion FAILS on the old code.
func TestAgent_ThinkBlockNeverLeaksNeverCited_Spec087(t *testing.T) {
	const maxIter = 2
	r := newRegistry(t)
	think := "<think>I could pad this with https://fabricated.test/zzz (hash badhash) but I won't.</think>"
	verdict := "wa-town-A is better on winter-low grounds."
	final := think + verdict + citationsBlock(webCiteEntry("https://example.test/x", "deadbeef"))
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"q"}`, 100),
		endTurn(final, 80),
	}}
	a, err := New(fl, r, citeback.Verify, spec087Cfg(maxIter, 1))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "a question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	if strings.Contains(got.FinalText, "fabricated.test") || strings.Contains(got.FinalText, "<think>") {
		t.Fatalf("SCN-087-A05: <think> content (fabricated URL) leaked into the user body.\nbody=%q", got.FinalText)
	}
	if len(got.Sources) != 1 {
		t.Fatalf("SCN-087-A05: only the real cited source must attach, got %d", len(got.Sources))
	}
	if got.Sources[0].Web == nil || got.Sources[0].Web.URL != "https://example.test/x" {
		t.Fatalf("SCN-087-A05: the attached source must be the real one, got %+v", got.Sources[0])
	}
}
