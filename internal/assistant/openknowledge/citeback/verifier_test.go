package citeback

import (
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	web "github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
)

func artifactTrace(id, title, summary string) ok.Source {
	return ok.Source{
		Kind:     ok.SourceArtifact,
		Artifact: &ok.ArtifactRef{ID: id, Kind: "artifact", Title: title},
	}
}

func webTrace(url, title, snippet string) ok.Source {
	return ok.Source{
		Kind: ok.SourceWeb,
		Web: &ok.WebSource{
			URL:         url,
			Title:       title,
			Snippet:     snippet,
			Provider:    "searxng",
			ContentHash: web.CanonicalContentHash(url, title, snippet),
		},
	}
}

func compTrace(tool string, in, out json.RawMessage) ok.Source {
	return ok.Source{
		Kind: ok.SourceToolComputation,
		Computation: &ok.ComputationSource{
			Tool:   tool,
			Input:  in,
			Output: out,
		},
	}
}

func TestCiteBackHappy(t *testing.T) {
	art := artifactTrace("art-1", "Title", "Summary")
	ws := webTrace("https://example.com/a", "T", "S")
	cp := compTrace("calculator", json.RawMessage(`{"expression":"1+1"}`), json.RawMessage(`{"result":2}`))
	trace := ToolTrace{
		{ToolName: "internal_retrieval", RecordedSources: []ok.Source{art}},
		{ToolName: "web_search", RecordedSources: []ok.Source{ws}},
		{ToolName: "calculator", RecordedSources: []ok.Source{cp}},
	}
	cites := []Citation{
		{Kind: ok.SourceArtifact, ArtifactID: "art-1"},
		{Kind: ok.SourceWeb, URL: "https://example.com/a", ContentHash: ws.Web.ContentHash},
		{Kind: ok.SourceToolComputation, Tool: "calculator",
			Input:  json.RawMessage(`{"expression":"1+1"}`),
			Output: json.RawMessage(`{"result":2}`)},
	}
	res := Verify(cites, trace)
	if !res.OK {
		t.Fatalf("expected OK, got rejects=%+v", res.Rejected)
	}
	if len(res.Verified) != 3 {
		t.Fatalf("expected 3 verified, got %d", len(res.Verified))
	}
}

func TestCiteBackFabricatedURL(t *testing.T) {
	ws := webTrace("https://example.com/real", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	cites := []Citation{
		{Kind: ok.SourceWeb, URL: "https://example.com/fake", ContentHash: "deadbeef"},
	}
	res := Verify(cites, trace)
	if res.OK || len(res.Rejected) != 1 {
		t.Fatalf("expected rejection, got %+v", res)
	}
	if !errors.Is(res.Rejected[0].Reason, ReasonNotInTrace) {
		t.Fatalf("expected ReasonNotInTrace, got %v", res.Rejected[0].Reason)
	}
}

func TestCiteBackHashMismatch(t *testing.T) {
	ws := webTrace("https://example.com/x", "Real Title", "Real Snippet")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	cites := []Citation{
		{Kind: ok.SourceWeb, URL: "https://example.com/x", ContentHash: "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	res := Verify(cites, trace)
	if res.OK || !errors.Is(res.Rejected[0].Reason, ReasonHashMismatch) {
		t.Fatalf("expected ReasonHashMismatch, got %+v", res)
	}
}

func TestCiteBackPartialCitation(t *testing.T) {
	cases := []Citation{
		{Kind: ok.SourceArtifact, ArtifactID: "  "},
		{Kind: ok.SourceWeb, URL: "https://x", ContentHash: ""},
		{Kind: ok.SourceWeb, URL: "", ContentHash: "abc"},
		{Kind: ok.SourceToolComputation, Tool: "calculator"},
		{Kind: ok.SourceToolComputation, Tool: "calculator",
			Input: json.RawMessage(`not json`), Output: json.RawMessage(`{"r":1}`)},
		{Kind: ok.SourceKind(99), ArtifactID: "x"},
	}
	for i, c := range cases {
		res := Verify([]Citation{c}, ToolTrace{})
		if res.OK || !errors.Is(res.Rejected[0].Reason, ReasonMalformedCitation) {
			t.Fatalf("case %d: expected ReasonMalformedCitation, got %+v", i, res)
		}
	}
}

func TestCiteBackKindMismatch(t *testing.T) {
	// Recorded entry has Computation populated but Kind=SourceArtifact:
	// locator (tool name) matches but Kind disagrees.
	bad := ok.Source{
		Kind: ok.SourceArtifact,
		Computation: &ok.ComputationSource{
			Tool:   "calculator",
			Input:  json.RawMessage(`{"expression":"1+1"}`),
			Output: json.RawMessage(`{"result":2}`),
		},
	}
	trace := ToolTrace{{ToolName: "calculator", RecordedSources: []ok.Source{bad}}}
	c := Citation{Kind: ok.SourceToolComputation, Tool: "calculator",
		Input:  json.RawMessage(`{"expression":"1+1"}`),
		Output: json.RawMessage(`{"result":2}`)}
	res := Verify([]Citation{c}, trace)
	if res.OK || !errors.Is(res.Rejected[0].Reason, ReasonKindMismatch) {
		t.Fatalf("expected ReasonKindMismatch, got %+v", res)
	}
}

func TestCiteBackEmptyCitationsEmptyTrace(t *testing.T) {
	res := Verify(nil, nil)
	if !res.OK || len(res.Verified) != 0 || len(res.Rejected) != 0 {
		t.Fatalf("expected OK with empty slices, got %+v", res)
	}
}

func TestCiteBackEmptyCitationsNonEmptyTrace(t *testing.T) {
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{webTrace("https://x", "T", "S")}}}
	res := Verify(nil, trace)
	if !res.OK || len(res.Verified) != 0 {
		t.Fatalf("no citation = no fabrication; got %+v", res)
	}
}

func TestCiteBackNonEmptyCitationsEmptyTrace(t *testing.T) {
	cites := []Citation{
		{Kind: ok.SourceArtifact, ArtifactID: "art-1"},
		{Kind: ok.SourceWeb, URL: "https://x", ContentHash: "abc"},
	}
	res := Verify(cites, ToolTrace{})
	if res.OK || len(res.Rejected) != 2 {
		t.Fatalf("expected all rejected, got %+v", res)
	}
	for _, r := range res.Rejected {
		if !errors.Is(r.Reason, ReasonNotInTrace) {
			t.Fatalf("expected ReasonNotInTrace, got %v", r.Reason)
		}
	}
}

func TestCiteBackAdversarial_URLCaseSensitivity(t *testing.T) {
	// Recorded with lowercase host, cited with mixed-case host + scheme
	// → must match. Path case must be preserved (case-sensitive) so a
	// path-case difference must NOT match.
	ws := webTrace("https://example.com/Path", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}

	matchCite := Citation{Kind: ok.SourceWeb, URL: "HTTPS://Example.COM/Path", ContentHash: ws.Web.ContentHash}
	r := Verify([]Citation{matchCite}, trace)
	if !r.OK {
		t.Fatalf("expected host case-insensitive match, got %+v", r)
	}

	nopeCite := Citation{Kind: ok.SourceWeb, URL: "https://example.com/path", ContentHash: ws.Web.ContentHash}
	r = Verify([]Citation{nopeCite}, trace)
	if r.OK {
		t.Fatalf("expected path-case mismatch to NOT match")
	}
}

func TestCiteBackAdversarial_TrailingSlashEquivalence(t *testing.T) {
	ws := webTrace("https://example.com/path/", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	// Even though the recorded hash was computed from the trailing-slash
	// form, normalised URL equality lets the citation use either form;
	// the ContentHash field is still compared verbatim against what the
	// model claims it cited. The model is expected to cite the recorded
	// hash; the test confirms URL form does not block the match.
	cite := Citation{Kind: ok.SourceWeb, URL: "https://example.com/path", ContentHash: ws.Web.ContentHash}
	r := Verify([]Citation{cite}, trace)
	if !r.OK {
		t.Fatalf("trailing slash should be equivalent, got %+v", r)
	}
	// Root path must be preserved.
	if normalizeURL("https://example.com/") != "https://example.com/" {
		t.Fatalf("root slash must be preserved")
	}
}

func TestCiteBackAdversarial_DuplicateCitationCountsOnce(t *testing.T) {
	ws := webTrace("https://example.com/a", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	cites := []Citation{
		{Kind: ok.SourceWeb, URL: "https://example.com/a", ContentHash: ws.Web.ContentHash},
		{Kind: ok.SourceWeb, URL: "https://example.com/a", ContentHash: ws.Web.ContentHash},
		{Kind: ok.SourceWeb, URL: "HTTPS://EXAMPLE.COM/a", ContentHash: ws.Web.ContentHash},
	}
	r := Verify(cites, trace)
	if !r.OK {
		t.Fatalf("expected OK, got %+v", r)
	}
	if len(r.Verified) != 1 {
		t.Fatalf("expected 1 deduped verified, got %d", len(r.Verified))
	}
}

func TestCiteBackAdversarial_ComputationInputDiffers(t *testing.T) {
	in := json.RawMessage(`{"expression":"3+1"}`)
	out := json.RawMessage(`{"result":4}`)
	trace := ToolTrace{{ToolName: "calculator", RecordedSources: []ok.Source{compTrace("calculator", in, out)}}}
	// Tool exists in trace but citation claims a different input
	// (and a fabricated output to go with it).
	cite := Citation{
		Kind:   ok.SourceToolComputation,
		Tool:   "calculator",
		Input:  json.RawMessage(`{"expression":"1+1"}`),
		Output: json.RawMessage(`{"result":5}`),
	}
	r := Verify([]Citation{cite}, trace)
	if r.OK || !errors.Is(r.Rejected[0].Reason, ReasonHashMismatch) {
		t.Fatalf("expected ReasonHashMismatch, got %+v", r)
	}

	// Reordered-key parity: same logical input/output → match.
	in2 := json.RawMessage(`{"a":1,"b":2}`)
	out2 := json.RawMessage(`{"x":1,"y":2}`)
	trace2 := ToolTrace{{ToolName: "calc", RecordedSources: []ok.Source{compTrace("calc", in2, out2)}}}
	cite2 := Citation{
		Kind:   ok.SourceToolComputation,
		Tool:   "calc",
		Input:  json.RawMessage(`{"b":2,"a":1}`),
		Output: json.RawMessage(`{"y":2,"x":1}`),
	}
	r = Verify([]Citation{cite2}, trace2)
	if !r.OK {
		t.Fatalf("expected canonical-JSON parity match, got %+v", r)
	}
}

func TestCiteBackAdversarial_PureStdlibOnly(t *testing.T) {
	// The verifier MUST NOT depend on LLM, HTTP, DB, or other I/O
	// packages. Parse verifier.go's imports and reject anything
	// suspicious. The openknowledge type-only import is allowed.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "verifier.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	forbidden := []string{"http", "sql", "pgx", "nats", "llm", "openai", "anthropic", "ollama", "ml"}
	allowedExternal := map[string]bool{
		"github.com/smackerel/smackerel/internal/assistant/openknowledge": true,
	}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		for _, bad := range forbidden {
			if strings.Contains(path, bad) {
				t.Fatalf("verifier.go imports forbidden package %q (matched %q)", path, bad)
			}
		}
		if strings.Contains(path, ".") { // non-stdlib
			if !allowedExternal[path] {
				t.Fatalf("verifier.go imports non-stdlib package %q (only openknowledge types are allowed)", path)
			}
		}
	}
}

func TestComputationCanonicalHash_Deterministic(t *testing.T) {
	in1 := json.RawMessage(`{"a":1,"b":[2,3]}`)
	in2 := json.RawMessage(`{"b":[2,3],"a":1}`)
	out := json.RawMessage(`{"r":42}`)
	h1, err := ComputationCanonicalHash("calc", in1, out)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := ComputationCanonicalHash("calc", in2, out)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("key order must not change hash: %s vs %s", h1, h2)
	}
	// Different tool name must change the hash.
	h3, err := ComputationCanonicalHash("other", in1, out)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h3 {
		t.Fatal("tool name must contribute to hash")
	}
}
