package citeback

import (
	"encoding/json"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

func TestVerifyVerdict_HappyOK(t *testing.T) {
	art := artifactTrace("art-1", "T", "S")
	trace := ToolTrace{{ToolName: "internal_retrieval", RecordedSources: []ok.Source{art}}}
	cites := []Citation{{Kind: ok.SourceArtifact, ArtifactID: "art-1"}}
	v := VerifyVerdict(cites, trace)
	if !v.OK || len(v.MissingCites) != 0 || len(v.FabricatedCites) != 0 {
		t.Fatalf("expected clean verdict, got %+v", v)
	}
}

func TestVerifyVerdict_FabricatedBucket_NotInTrace(t *testing.T) {
	ws := webTrace("https://example.com/real", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	cites := []Citation{
		{Kind: ok.SourceWeb, URL: "https://example.com/fake", ContentHash: "deadbeef"},
	}
	v := VerifyVerdict(cites, trace)
	if v.OK {
		t.Fatalf("expected NOT OK")
	}
	if len(v.FabricatedCites) != 1 || len(v.MissingCites) != 0 {
		t.Fatalf("expected one fabricated, zero missing; got %+v", v)
	}
}

func TestVerifyVerdict_MissingBucket_HashMismatch(t *testing.T) {
	ws := webTrace("https://example.com/x", "Real", "Real")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	cites := []Citation{
		{Kind: ok.SourceWeb, URL: "https://example.com/x", ContentHash: "0000000000000000000000000000000000000000000000000000000000000000"},
	}
	v := VerifyVerdict(cites, trace)
	if v.OK {
		t.Fatalf("expected NOT OK")
	}
	if len(v.MissingCites) != 1 || len(v.FabricatedCites) != 0 {
		t.Fatalf("expected one missing, zero fabricated; got %+v", v)
	}
}

func TestVerifyVerdict_MissingBucket_Malformed(t *testing.T) {
	trace := ToolTrace{}
	cites := []Citation{
		{Kind: ok.SourceToolComputation, Tool: "", Input: json.RawMessage(`{}`), Output: json.RawMessage(`{}`)},
	}
	v := VerifyVerdict(cites, trace)
	if v.OK || len(v.MissingCites) != 1 || len(v.FabricatedCites) != 0 {
		t.Fatalf("expected one missing (malformed), got %+v", v)
	}
}

func TestVerifyVerdict_MixedBuckets(t *testing.T) {
	ws := webTrace("https://example.com/x", "T", "S")
	trace := ToolTrace{{ToolName: "web_search", RecordedSources: []ok.Source{ws}}}
	cites := []Citation{
		{Kind: ok.SourceWeb, URL: "https://example.com/x", ContentHash: ws.Web.ContentHash}, // ok
		{Kind: ok.SourceWeb, URL: "https://example.com/x", ContentHash: "ffff"},             // missing (hash mismatch)
		{Kind: ok.SourceWeb, URL: "https://example.com/fake", ContentHash: "abcd"},          // fabricated
	}
	v := VerifyVerdict(cites, trace)
	if v.OK {
		t.Fatalf("expected NOT OK")
	}
	if len(v.MissingCites) != 1 || len(v.FabricatedCites) != 1 {
		t.Fatalf("expected 1 missing + 1 fabricated, got %+v", v)
	}
}
