// Spec 084 — adversarial + guard tests for the open-knowledge REASONING LOOP.
//
// These tests assert on the agent's UN-REDACTED assembled TurnResult and on
// the per-iteration request stream. They are non-tautological: the three
// behavioral-change tests (reflect-before-final nudge, comparison-salvage
// honesty, honest-salvage framing) FAIL against the pre-spec-084 agent.go and
// PASS only after the loop/honesty changes land. The guard tests
// (multi-hop drill-in, genuine-synthesis verbatim, fabricated-citation
// rejection) protect the preserved behavior + trust contracts.
//
// They reuse the shared harness in agent_test.go (fakeLLM, fakeWebTool,
// toolUse, endTurn, baseCfg, newRegistry) plus a small recordingLLM and a
// per-call sequential web tool defined here.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/citeback"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
)

// recordingLLM is fakeLLM plus a captured request stream, so a test can
// assert what the agent sent the model on each iteration (needed to prove the
// reflect-before-final nudge is injected on the second-to-last iteration).
type recordingLLM struct {
	responses []llm.Result
	requests  []llm.ChatRequest
	calls     int
	t         *testing.T
}

func (f *recordingLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.Result, error) {
	f.requests = append(f.requests, req)
	if f.calls >= len(f.responses) {
		f.t.Fatalf("recordingLLM: unexpected call #%d (queue exhausted)", f.calls+1)
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

// requestText concatenates every message body of a recorded request so a test
// can search for an injected nudge regardless of which message carries it.
func requestText(req llm.ChatRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

// seqWebTool returns a DISTINCT snippet+source per Execute call, modelling a
// multi-hop / comparison turn where each side's evidence comes from a separate
// tool call. The counter is a pointer so the value-receiver Execute mutates it
// across registry-dispatched calls.
type seqWebTool struct {
	snips  []string
	urls   []string
	hashes []string
	i      *int
}

func (seqWebTool) Name() string {
	return "seq_web"
}

func (seqWebTool) Description() string {
	return "sequential distinct-per-call web search for tests"
}

func (seqWebTool) ParamsSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (s seqWebTool) Execute(_ context.Context, _ json.RawMessage) (*ok.ToolResult, error) {
	idx := *s.i
	if idx >= len(s.snips) {
		idx = len(s.snips) - 1
	}
	*s.i++
	return &ok.ToolResult{
		Snippets: []ok.Snippet{{Text: s.snips[idx], ContentHash: s.hashes[idx], SourceRef: s.urls[idx]}},
		Sources: []ok.Source{{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: s.urls[idx], ContentHash: s.hashes[idx], Provider: "fake", Snippet: s.snips[idx]},
		}},
	}, nil
}

// ── SCN-084-A02 — loop drills in (reflect-before-final + multi-hop) ──────────

// TestAgent_ReflectBeforeFinal_NudgeOnSecondToLastIteration_Spec084 is the
// headline reflect-nudge reproduction. ADVERSARIAL: the pre-spec-084 loop
// injects NO reflect nudge, so the assertion that the second-to-last request
// carries the "Before you give your final answer" coverage check FAILS on the
// old code and PASSES only after the nudge lands. It also proves the
// forced-final tool-stripping message is preserved on the last iteration.
func TestAgent_ReflectBeforeFinal_NudgeOnSecondToLastIteration_Spec084(t *testing.T) {
	const maxIter = 6 // iters 0..5; reflect at iter 4 (maxIter-2); forced-final at iter 5
	counter := 0
	r := ok.NewRegistry([]string{"seq_web"})
	if err := r.Register(seqWebTool{
		snips:  []string{"s0", "s1", "s2", "s3", "s4"},
		urls:   []string{"https://e.test/0", "https://e.test/1", "https://e.test/2", "https://e.test/3", "https://e.test/4"},
		hashes: []string{"h0", "h1", "h2", "h3", "h4"},
		i:      &counter,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	final := "Synthesized answer.\n<CITATIONS>[{\"kind\":\"web\",\"url\":\"https://e.test/0\",\"content_hash\":\"h0\"}]</CITATIONS>"
	fl := &recordingLLM{t: t, responses: []llm.Result{
		toolUse("w0", "seq_web", `{"query":"q0"}`, 100),
		toolUse("w1", "seq_web", `{"query":"q1"}`, 100),
		toolUse("w2", "seq_web", `{"query":"q2"}`, 100),
		toolUse("w3", "seq_web", `{"query":"q3"}`, 100),
		toolUse("w4", "seq_web", `{"query":"q4"}`, 100),
		endTurn(final, 80),
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(maxIter, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "multi-hop reasoning question")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	if len(fl.requests) != maxIter {
		t.Fatalf("recorded %d requests, want %d", len(fl.requests), maxIter)
	}
	const reflectMarker = "Before you give your final answer"
	const forcedFinalMarker = "write your final answer NOW"

	// Reflect nudge MUST appear on the second-to-last iteration (index maxIter-2)
	// and NOWHERE before it.
	secondToLast := requestText(fl.requests[maxIter-2])
	if !strings.Contains(secondToLast, reflectMarker) {
		t.Fatalf("SCN-084-A02: reflect-before-final nudge missing on the second-to-last request (index %d).\nrequest text=%q", maxIter-2, secondToLast)
	}
	for i := 0; i < maxIter-2; i++ {
		if strings.Contains(requestText(fl.requests[i]), reflectMarker) {
			t.Fatalf("SCN-084-A02: reflect nudge leaked onto request index %d (should only fire on the second-to-last)", i)
		}
	}
	// Forced-final tool-stripping message MUST still be present on the last
	// iteration, and the reflect nudge MUST NOT replace it.
	last := requestText(fl.requests[maxIter-1])
	if !strings.Contains(last, forcedFinalMarker) {
		t.Fatalf("SCN-084-A02: forced-final synthesis message missing on the last request.\nrequest text=%q", last)
	}
	if fl.requests[maxIter-1].Tools != nil {
		t.Fatalf("SCN-084-A02: tools must be stripped on the forced-final turn, got %d tools", len(fl.requests[maxIter-1].Tools))
	}
}

// TestAgent_MultiHop_AllowsDistinctToolCallsBeforeForcedFinal_Spec084 is the
// drill-in capability guard: at max_iterations=6 the loop processes FIVE
// distinct tool calls through to a genuine cited synthesis without forcing a
// premature stop or collapsing the calls. This is the "the loop is ALLOWED to
// issue >=2 distinct tool calls before the forced final" guarantee.
func TestAgent_MultiHop_AllowsDistinctToolCallsBeforeForcedFinal_Spec084(t *testing.T) {
	const maxIter = 6
	counter := 0
	r := ok.NewRegistry([]string{"seq_web"})
	if err := r.Register(seqWebTool{
		snips:  []string{"a", "b", "c", "d", "e"},
		urls:   []string{"https://m.test/0", "https://m.test/1", "https://m.test/2", "https://m.test/3", "https://m.test/4"},
		hashes: []string{"m0", "m1", "m2", "m3", "m4"},
		i:      &counter,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	final := "Reasoned synthesis across all hops.\n<CITATIONS>[{\"kind\":\"web\",\"url\":\"https://m.test/0\",\"content_hash\":\"m0\"}]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "seq_web", `{"query":"hop-0"}`, 100),
		toolUse("w1", "seq_web", `{"query":"hop-1"}`, 100),
		toolUse("w2", "seq_web", `{"query":"hop-2"}`, 100),
		toolUse("w3", "seq_web", `{"query":"hop-3"}`, 100),
		toolUse("w4", "seq_web", `{"query":"hop-4"}`, 100),
		endTurn(final, 80),
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(maxIter, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "why does X cause Y across multiple hops")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess || got.TerminationReason != TerminationFinal {
		t.Fatalf("Status=%q termination=%q reason=%q want success/final", got.Status, got.TerminationReason, got.RefusalReason)
	}
	if len(got.ToolTrace) != 5 {
		t.Fatalf("SCN-084-A02: want 5 distinct tool calls before the forced final, got %d", len(got.ToolTrace))
	}
	seenArgs := map[string]struct{}{}
	for _, e := range got.ToolTrace {
		seenArgs[string(e.Args)] = struct{}{}
	}
	if len(seenArgs) != 5 {
		t.Fatalf("SCN-084-A02: tool calls must be distinct (not paraphrase collapse), got %d unique of 5", len(seenArgs))
	}
}

// ── SCN-084-A03 — comparison salvage is honest ──────────────────────────────

// TestAgent_ComparisonSalvage_HonestlyFramed_BothSides_Spec084 — ADVERSARIAL.
// Two DISTINCT tool calls gather side X and side Y; the model fails to
// synthesize on the forced-final turn. The pre-spec-084 salvage stitches the
// two snippets and presents them as a confident answer (no framing). The new
// behavior frames the body as raw findings AND still carries both sides. The
// "honest frame present" assertion FAILS on the old code.
func TestAgent_ComparisonSalvage_HonestlyFramed_BothSides_Spec084(t *testing.T) {
	const maxIter = 3 // iters 0,1,2; reflect at iter1; forced-final at iter2
	sideX := "wa-town-A: mild maritime climate, rarely below freezing."
	sideY := "wa-town-B: cooler inland nights with greater frost risk."
	counter := 0
	r := ok.NewRegistry([]string{"seq_web"})
	if err := r.Register(seqWebTool{
		snips:  []string{sideX, sideY},
		urls:   []string{"https://cmp.test/x", "https://cmp.test/y"},
		hashes: []string{"hx", "hy"},
		i:      &counter,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("wx", "seq_web", `{"query":"wa-town-A pomegranate climate"}`, 100),
		toolUse("wy", "seq_web", `{"query":"wa-town-B pomegranate climate"}`, 100),
		endTurn("", 50), // forced-final blank -> snippet salvage
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(maxIter, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "what is a better place to grow pomegranate, wa-town-A or wa-town-B?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success (salvage)", got.Status, got.RefusalReason)
	}
	body := got.FinalText
	if !strings.Contains(body, "couldn't directly answer") {
		t.Fatalf("SCN-084-A03: comparison salvage must be honestly framed (couldn't directly answer), not a confident verdict.\nbody=%q", body)
	}
	if !strings.Contains(body, sideX) || !strings.Contains(body, sideY) {
		t.Fatalf("SCN-084-A03: salvage must carry BOTH sides' distinct evidence.\nbody=%q", body)
	}
	if strings.HasPrefix(strings.TrimSpace(body), sideX) || strings.HasPrefix(strings.TrimSpace(body), sideY) {
		t.Fatalf("SCN-084-A03: salvage body must NOT lead with a raw snippet as if it were the verdict.\nbody=%q", body)
	}
}

// ── SCN-084-A04 — honest salvage on empty / ungrounded forced-final ─────────

// TestAgent_HonestSalvage_EmptyForcedFinal_FramedWithSources_Spec084 —
// ADVERSARIAL. Forced-final empty text triggers snippet salvage; the body MUST
// be framed as raw findings (not a confident answer) and MUST still carry
// capped sources (not a zero-source refusal).
func TestAgent_HonestSalvage_EmptyForcedFinal_FramedWithSources_Spec084(t *testing.T) {
	const maxIter = 2   // iter0 tool call, iter1 forced-final empty
	r := newRegistry(t) // fakeWebTool snippet="hello", url=example.test/x, hash=deadbeef
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"some topic"}`, 100),
		endTurn("", 50),
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(maxIter, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
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
		t.Fatalf("SCN-084-A04: empty-forced-final salvage must be honestly framed.\nbody=%q", got.FinalText)
	}
	if !strings.Contains(got.FinalText, "hello") {
		t.Fatalf("SCN-084-A04: framed salvage must still include the retrieved finding.\nbody=%q", got.FinalText)
	}
	if len(got.Sources) == 0 {
		t.Fatalf("SCN-084-A04: framed salvage must still carry sources (not a zero-source refusal)")
	}
}

// TestAgent_HonestSalvage_UngroundedExcuse_ReplacedWithFramedFindings_Spec084 —
// ADVERSARIAL. The model writes an "I was unable to find" excuse with empty
// citations even though a tool DID return content. The empty-citations
// body-quality salvage replaces the lie. The replacement MUST be the honest
// frame + the real finding (the old code replaced it with a bare snippet).
func TestAgent_HonestSalvage_UngroundedExcuse_ReplacedWithFramedFindings_Spec084(t *testing.T) {
	const maxIter = 5 // iter1 end-turn is NOT the forced-final; empty-citations salvage path
	r := newRegistry(t)
	excuse := "I was unable to find any relevant information.\n<CITATIONS>[]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"some topic"}`, 100),
		endTurn(excuse, 50),
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(maxIter, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
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
		t.Fatalf("SCN-084-A04: ungrounded-excuse salvage must be replaced with the honest frame.\nbody=%q", got.FinalText)
	}
	if !strings.Contains(got.FinalText, "hello") {
		t.Fatalf("SCN-084-A04: framed salvage must include the real finding, not the excuse.\nbody=%q", got.FinalText)
	}
	if strings.Contains(got.FinalText, "I was unable to find") {
		t.Fatalf("SCN-084-A04: the ungrounded excuse must be replaced, not kept.\nbody=%q", got.FinalText)
	}
	if len(got.Sources) == 0 {
		t.Fatalf("SCN-084-A04: framed salvage must still carry sources")
	}
}

// ── SCN-084-A05 — trust contracts preserved (guards) ────────────────────────

// TestAgent_GenuineSynthesis_ReturnedVerbatim_NoSalvageFrame_Spec084 — GUARD.
// When the model produces a real cited synthesis, the body is returned
// verbatim with NO honest-salvage frame (the framing must never leak onto the
// happy path).
func TestAgent_GenuineSynthesis_ReturnedVerbatim_NoSalvageFrame_Spec084(t *testing.T) {
	synthesis := "wa-town-B edges out wa-town-A for pomegranates due to warmer summer days."
	final := synthesis + "\n<CITATIONS>[{\"kind\":\"web\",\"url\":\"https://example.test/x\",\"content_hash\":\"deadbeef\"}]</CITATIONS>"
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"pomegranate climate"}`, 100),
		endTurn(final, 80),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "pomegranate comparison")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	if got.FinalText != synthesis {
		t.Fatalf("SCN-084-A05: genuine synthesis must be returned verbatim, got %q", got.FinalText)
	}
	if strings.Contains(got.FinalText, "couldn't directly answer") {
		t.Fatalf("SCN-084-A05: honest-salvage frame must NOT leak onto a genuine synthesis.\nbody=%q", got.FinalText)
	}
}

// TestAgent_FabricatedCitation_StillRejected_Spec084 — GUARD. A citation that
// does not hash-match any recorded tool result is still rejected by the
// cite-back verifier in enforce mode (the trust contract is untouched by the
// reasoning-loop changes).
func TestAgent_FabricatedCitation_StillRejected_Spec084(t *testing.T) {
	final := fmt.Sprintf("Confident-sounding answer.\n<CITATIONS>[{%q:%q,%q:%q,%q:%q}]</CITATIONS>",
		"kind", "web", "url", "https://fabricated.test/nope", "content_hash", "nomatch")
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w0", "fake_web", `{"query":"real query"}`, 100),
		endTurn(final, 80),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 1000000, 1.0, 100.0, 100.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "real query")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusRefused {
		t.Fatalf("SCN-084-A05: fabricated citation must be refused, got status=%q body=%q", got.Status, got.FinalText)
	}
	if got.TerminationReason != TerminationFabricatedSource {
		t.Fatalf("SCN-084-A05: want termination=fabricated_source, got %q", got.TerminationReason)
	}
}
