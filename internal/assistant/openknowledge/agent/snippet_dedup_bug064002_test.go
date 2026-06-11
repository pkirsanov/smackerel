// BUG-064-002 — adversarial regression tests for the open-knowledge
// answer-quality defects:
//
//	DEFECT 1 — snippet-dump instead of synthesis
//	DEFECT 2 — triplicate duplication (same snippet block 3x)
//	DEFECT 3b — source over-attach (32 sources)
//
// These tests assert on the agent's UN-REDACTED assembled TurnResult
// (FinalText + Sources), which is the proof the prod log hides
// (body_redacted=true). They are non-tautological: each FAILS against
// today's behavior and PASSES only after the fix.
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

// tideSnippet mirrors the live "wa-town-A tide times" search-result
// preview that the production salvage dumped three times.
const tideSnippet = "Tide Times · Home · United States; wa-town-A tides. " +
	"wa-town-A Tide Times, Washington. Tide Times Today & Tomorrow. " +
	"« Thu, June 11. wa-town-A tide today …"

// traceEntryWithSnippet builds a web_search trace entry whose single
// snippet+source is the given text/url/hash.
func traceEntryWithSnippet(text, url, hash string) ToolTraceEntry {
	return ToolTraceEntry{
		ToolName: "web_search",
		Result: &ok.ToolResult{
			Snippets: []ok.Snippet{{Text: text, ContentHash: hash, SourceRef: url}},
			Sources: []ok.Source{{
				Kind: ok.SourceWeb,
				Web:  &ok.WebSource{URL: url, ContentHash: hash, Provider: "fake", Snippet: text},
			}},
		},
	}
}

// TestSynthesizeFromSnippets_DedupsIdenticalLeadSnippets_BUG064002 is
// the headline DEFECT 2 reproduction at the pure-function layer. Three
// web_search calls that returned the SAME top snippet MUST collapse to
// ONE block in the salvage body, not appear 3x.
func TestSynthesizeFromSnippets_DedupsIdenticalLeadSnippets_BUG064002(t *testing.T) {
	trace := []ToolTraceEntry{
		traceEntryWithSnippet(tideSnippet, "https://tidetime.org/x", "h1"),
		traceEntryWithSnippet(tideSnippet, "https://surf-forecast.com/y", "h2"),
		traceEntryWithSnippet(tideSnippet, "https://surfline.com/z", "h3"),
	}
	body := synthesizeFromSnippets(trace)
	if c := strings.Count(body, tideSnippet); c != 1 {
		t.Fatalf("BUG-064-002 DEFECT 2: identical snippet appears %d times in the salvage body, want exactly 1.\nbody=%q", c, body)
	}
	triple := strings.Join([]string{tideSnippet, tideSnippet, tideSnippet}, "\n\n")
	if body == triple {
		t.Fatalf("BUG-064-002 DEFECT 2: salvage body is the verbatim 3x passthrough")
	}
}

// TestSynthesizeFromSnippets_KeepsDistinctSnippets_BUG064002 is the
// adversarial guard against an over-aggressive dedup that would drop
// genuinely distinct evidence. Distinct snippets MUST all survive.
func TestSynthesizeFromSnippets_KeepsDistinctSnippets_BUG064002(t *testing.T) {
	a := "High tide 7:42am at 8.9 ft."
	b := "Low tide 1:55pm at 0.3 ft."
	trace := []ToolTraceEntry{
		traceEntryWithSnippet(a, "https://tidetime.org/a", "h1"),
		traceEntryWithSnippet(b, "https://tidetime.org/b", "h2"),
	}
	body := synthesizeFromSnippets(trace)
	if !strings.Contains(body, a) || !strings.Contains(body, b) {
		t.Fatalf("dedup dropped a distinct snippet.\nbody=%q", body)
	}
}

// TestAgent_ForcedFinalEmptySalvage_NotTriplicated_BUG064002 is the
// end-to-end DEFECT 1+2 reproduction: 3 web_search calls return the
// same snippet, the model returns EMPTY text on the forced-final turn
// (gemma-class blank), the agent salvages from snippets. The assembled
// body MUST contain the snippet ONCE — not the 3x raw passthrough.
func TestAgent_ForcedFinalEmptySalvage_NotTriplicated_BUG064002(t *testing.T) {
	r := ok.NewRegistry([]string{"fake_web"})
	if err := r.Register(fakeWebTool{url: "https://tidetime.org/x", hash: "tide001", snippet: tideSnippet}); err != nil {
		t.Fatalf("register: %v", err)
	}
	// 3 tool-calling turns + 1 forced-final turn that returns "".
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w1", "fake_web", `{"query":"wa-town-A tide 06/11"}`, 100),
		toolUse("w2", "fake_web", `{"query":"wa-town-A tide times"}`, 100),
		toolUse("w3", "fake_web", `{"query":"wa-town-A highs lows ft"}`, 100),
		endTurn("", 50),
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(4, 100000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "wa-town-A tide schedule 06/11 highs lows ft")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success (salvage)", got.Status, got.RefusalReason)
	}
	if c := strings.Count(got.FinalText, tideSnippet); c != 1 {
		t.Fatalf("BUG-064-002 DEFECT 2 (e2e): snippet appears %d times in the assembled body, want 1.\nbody=%q", c, got.FinalText)
	}
}

// TestAgent_RealSynthesisIsPreserved_NotSnippetDump_BUG064002 is the
// DEFECT 1 positive: when the model DOES produce a cited synthesis, the
// assembled body is the SYNTHESIS, never a raw snippet passthrough.
func TestAgent_RealSynthesisIsPreserved_NotSnippetDump_BUG064002(t *testing.T) {
	synthesis := "wa-town-A 06/11: high 7:42am 8.9 ft, low 1:55pm 0.3 ft."
	// Cite the default fakeWebTool source (url/hash from newRegistry).
	final := synthesis + `<CITATIONS>[{"kind":"web","url":"https://example.test/x","content_hash":"deadbeef"}]</CITATIONS>`
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("w1", "fake_web", `{"query":"wa-town-A tide"}`, 100),
		endTurn(final, 80),
	}}
	r := newRegistry(t)
	a, err := New(fl, r, citeback.Verify, baseCfg(5, 100000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "wa-town-A tide schedule")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Status != StatusSuccess {
		t.Fatalf("Status=%q reason=%q want success", got.Status, got.RefusalReason)
	}
	if got.FinalText != synthesis {
		t.Fatalf("DEFECT 1: body must be the synthesis verbatim, got %q", got.FinalText)
	}
	// "hello" is the default fakeWebTool snippet — the body must NOT be
	// raw tool-snippet passthrough.
	if strings.Contains(got.FinalText, "hello") {
		t.Fatalf("DEFECT 1: body contains raw tool snippet, not a synthesis: %q", got.FinalText)
	}
}

// multiSourceWebTool returns k distinct web sources + snippets in one
// call, so collectTraceSources yields k>cap distinct sources.
type multiSourceWebTool struct{ k int }

func (multiSourceWebTool) Name() string                  { return "multi_web" }
func (multiSourceWebTool) Description() string           { return "fake multi-source web for tests" }
func (multiSourceWebTool) ParamsSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (m multiSourceWebTool) Execute(_ context.Context, _ json.RawMessage) (*ok.ToolResult, error) {
	res := &ok.ToolResult{}
	for i := 0; i < m.k; i++ {
		url := fmt.Sprintf("https://src%d.test/p", i)
		hash := fmt.Sprintf("hash%02d", i)
		text := fmt.Sprintf("distinct snippet %d", i)
		res.Snippets = append(res.Snippets, ok.Snippet{Text: text, ContentHash: hash, SourceRef: url})
		res.Sources = append(res.Sources, ok.Source{
			Kind: ok.SourceWeb,
			Web:  &ok.WebSource{URL: url, ContentHash: hash, Provider: "fake", Snippet: text},
		})
	}
	return res, nil
}

// TestAgent_SalvageSourcesCappedAndDeduped_BUG064002 — DEFECT 3b: when
// the trace carries far more distinct sources than the SST sources_max
// cap (baseCfg uses 5), the salvaged Sources MUST be capped to the cap
// and contain no duplicates. Today the agent attaches all 10.
func TestAgent_SalvageSourcesCappedAndDeduped_BUG064002(t *testing.T) {
	const cap5 = 5
	r := ok.NewRegistry([]string{"multi_web"})
	if err := r.Register(multiSourceWebTool{k: 10}); err != nil {
		t.Fatalf("register: %v", err)
	}
	fl := &fakeLLM{t: t, responses: []llm.Result{
		toolUse("m1", "multi_web", `{"query":"wa-town-A tide"}`, 100),
		endTurn("", 50), // forced-final empty -> snippet salvage
	}}
	a, err := New(fl, r, citeback.Verify, baseCfg(2, 100000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 }))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := a.Run(context.Background(), "wa-town-A tide schedule")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got.Sources) > cap5 {
		t.Fatalf("BUG-064-002 DEFECT 3b: attached %d sources, want <= %d (sources_max)", len(got.Sources), cap5)
	}
	seen := map[string]struct{}{}
	for _, s := range got.Sources {
		if s.Web == nil {
			continue
		}
		k := s.Web.URL + "|" + s.Web.ContentHash
		if _, dup := seen[k]; dup {
			t.Fatalf("BUG-064-002 DEFECT 3b: duplicate source %q in attached set", k)
		}
		seen[k] = struct{}{}
	}
}

// TestNew_RejectsNonPositiveSourcesMax_BUG064002 — FR-5 fail-loud: the
// salvage source cap is SST-sourced (assistant.sources_max) and MUST
// be > 0; New() rejects a non-positive value with no silent default.
func TestNew_RejectsNonPositiveSourcesMax_BUG064002(t *testing.T) {
	r := newRegistry(t)
	for _, bad := range []int{0, -1} {
		cfg := baseCfg(5, 1000, 1.0, 10.0, 10.0, 0.8, func(int) float64 { return 0 })
		cfg.SourcesMax = bad
		_, err := New(&fakeLLM{t: t}, r, citeback.Verify, cfg)
		if err == nil {
			t.Fatalf("New must reject SourcesMax=%d (fail-loud, no silent default)", bad)
		}
		if !strings.Contains(err.Error(), "SourcesMax") {
			t.Fatalf("New error must name SourcesMax, got %q", err.Error())
		}
	}
}
