package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/api"
)

// fakeSearcher records the last Search request so tests can assert on
// the cap behavior and on plumbing.
type fakeSearcher struct {
	lastReq api.SearchRequest
	results []api.SearchResult
	mode    string
	err     error
}

func (f *fakeSearcher) Search(_ context.Context, req api.SearchRequest) ([]api.SearchResult, int, string, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, 0, "", f.err
	}
	return f.results, len(f.results), f.mode, nil
}

func TestRetrievalSearch_Registered(t *testing.T) {
	// init() must have populated the registry; the tool must exist.
	if !agent.Has(ToolName) {
		t.Fatalf("expected %q to be registered after init()", ToolName)
	}
	tool, ok := agent.ByName(ToolName)
	if !ok {
		t.Fatalf("ByName(%q) returned !ok", ToolName)
	}
	if tool.SideEffectClass != agent.SideEffectRead {
		t.Errorf("side_effect_class: got %q, want read", tool.SideEffectClass)
	}
	if tool.OwningPackage != "internal/agent/tools/retrieval" {
		t.Errorf("owning_package: got %q", tool.OwningPackage)
	}
	if tool.Handler == nil {
		t.Fatal("handler is nil")
	}
}

func TestRetrievalSearch_NotConfigured(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	args := json.RawMessage(`{"query":"x","user_id":"u"}`)
	_, err := tool.Handler(context.Background(), args)
	if err == nil {
		t.Fatal("expected error when services are not wired")
	}
	if !strings.Contains(err.Error(), "retrieval_tools_not_configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRetrievalSearch_HappyPath(t *testing.T) {
	fs := &fakeSearcher{
		results: []api.SearchResult{
			{ArtifactID: "A1", Title: "Tailscale notes", Snippet: "ACL tag …", CreatedAt: "2026-05-01T12:00:00Z"},
			{ArtifactID: "A2", Title: "ACL tags primer", Snippet: "tag:bridge …", CreatedAt: "2026-05-02T12:00:00Z"},
		},
		mode: "semantic",
	}
	SetServices(&Services{Engine: fs, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	args := json.RawMessage(`{"query":"tailscale acl","user_id":"u1"}`)
	out, err := tool.Handler(context.Background(), args)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	var got retrievalOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(got.Hits) != 2 {
		t.Fatalf("hits: got %d, want 2", len(got.Hits))
	}
	if got.Hits[0].ArtifactID != "A1" || got.Hits[1].ArtifactID != "A2" {
		t.Errorf("hits: %+v", got.Hits)
	}
}

func TestRetrievalSearch_TopKCap(t *testing.T) {
	fs := &fakeSearcher{}
	SetServices(&Services{Engine: fs, MaxTopK: 4})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	// Request top_k=20; the SST cap is 4 so the engine MUST see Limit=4.
	args := json.RawMessage(`{"query":"x","user_id":"u","top_k":20}`)
	if _, err := tool.Handler(context.Background(), args); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fs.lastReq.Limit != 4 {
		t.Errorf("expected engine limit clamped to MaxTopK=4, got %d", fs.lastReq.Limit)
	}
}

func TestRetrievalSearch_TopKZeroUsesCap(t *testing.T) {
	fs := &fakeSearcher{}
	SetServices(&Services{Engine: fs, MaxTopK: 7})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	args := json.RawMessage(`{"query":"x","user_id":"u"}`)
	if _, err := tool.Handler(context.Background(), args); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if fs.lastReq.Limit != 7 {
		t.Errorf("expected engine limit defaulted to MaxTopK=7, got %d", fs.lastReq.Limit)
	}
}

func TestRetrievalSearch_BadInput(t *testing.T) {
	SetServices(&Services{Engine: &fakeSearcher{}, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	cases := []struct {
		name string
		args string
		want string
	}{
		{"empty body", `{}`, "missing_user_id"},
		{"empty query", `{"query":"","user_id":"u"}`, "empty_query"},
		{"empty user", `{"query":"x","user_id":""}`, "missing_user_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Handler(context.Background(), json.RawMessage(tc.args))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestRetrievalSearch_EngineError(t *testing.T) {
	SetServices(&Services{Engine: &fakeSearcher{err: errors.New("kaboom")}, MaxTopK: 5})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"x","user_id":"u"}`))
	if err == nil || !strings.Contains(err.Error(), "retrieval_search_engine_error") {
		t.Errorf("got %v, want retrieval_search_engine_error", err)
	}
}

func TestRetrievalSearch_OutputSchemaCompiles(t *testing.T) {
	in, out, ok := agent.SchemasFor(ToolName)
	if !ok {
		t.Fatal("schemas missing for retrieval_search")
	}
	if in == nil || out == nil {
		t.Fatal("nil compiled schema")
	}
	// Adversarial: valid sample passes; missing required field fails.
	if err := out.ValidateBytes(json.RawMessage(`{"hits":[]}`)); err != nil {
		t.Errorf("valid sample rejected: %v", err)
	}
	if err := out.ValidateBytes(json.RawMessage(`{}`)); err == nil {
		t.Error("expected required-field violation for {}")
	}
}
