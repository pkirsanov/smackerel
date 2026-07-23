package tools

// self_knowledge_test.go — spec 104 SCOPE-04 unit tests.
//
// A recording SemanticSearcher proves the tool searches the configured
// namespace and maps results to cited Source{Kind:SourceArtifact} entries,
// plus the validation + backend error paths. The tool over real pgvector is
// integration-tested (semantic_searcher_test.go build-tagged integration).

import (
	"context"
	"encoding/json"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

type recordingSearcher struct {
	gotNS    string
	gotQuery string
	gotK     int
	arts     []GraphArtifact
	err      error
}

func (r *recordingSearcher) Search(_ context.Context, ns, q string, k int) ([]GraphArtifact, error) {
	r.gotNS, r.gotQuery, r.gotK = ns, q, k
	return r.arts, r.err
}

func TestSelfKnowledge_Contract(t *testing.T) {
	t.Parallel()
	tool := NewSelfKnowledge(&recordingSearcher{}, "smackerel_self")
	if tool.Name() != "self_knowledge" {
		t.Errorf("Name = %q, want self_knowledge", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description is empty")
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.ParamsSchema(), &schema); err != nil {
		t.Errorf("ParamsSchema is not valid JSON: %v", err)
	}
}

func TestSelfKnowledge_ExecuteMapsCitedSources(t *testing.T) {
	t.Parallel()
	rs := &recordingSearcher{arts: []GraphArtifact{
		{ID: "cap-1", Title: "capabilities overview", Summary: "what smackerel does"},
		{ID: "cap-2", Title: "/ask command", Summary: ""},
	}}
	tool := NewSelfKnowledge(rs, "smackerel_self")

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"what can smackerel do","k":5}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %v", res.Error)
	}
	if rs.gotNS != "smackerel_self" {
		t.Errorf("searched namespace %q, want smackerel_self (isolation)", rs.gotNS)
	}
	if rs.gotQuery != "what can smackerel do" || rs.gotK != 5 {
		t.Errorf("searcher got query=%q k=%d, want (what can smackerel do, 5)", rs.gotQuery, rs.gotK)
	}
	if len(res.Sources) != 2 || len(res.Snippets) != 2 {
		t.Fatalf("got %d sources / %d snippets, want 2 / 2", len(res.Sources), len(res.Snippets))
	}
	for i, s := range res.Sources {
		if s.Kind != ok.SourceArtifact || s.Artifact == nil {
			t.Fatalf("source %d Kind=%q artifact=%v, want SourceArtifact + non-nil", i, s.Kind, s.Artifact)
		}
	}
	if res.Sources[0].Artifact.ID != "cap-1" {
		t.Errorf("first source id = %q, want cap-1", res.Sources[0].Artifact.ID)
	}
}

func TestSelfKnowledge_ExecuteErrorPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		params   string
		searcher SemanticSearcher
		wantCode string
	}{
		{"malformed json", `{`, &recordingSearcher{}, "malformed_params"},
		{"unknown field", `{"query":"q","k":1,"x":1}`, &recordingSearcher{}, "malformed_params"},
		{"missing query", `{"k":1}`, &recordingSearcher{}, "malformed_params"},
		{"missing k", `{"query":"q"}`, &recordingSearcher{}, "malformed_params"},
		{"blank query", `{"query":"   ","k":1}`, &recordingSearcher{}, "invalid_query"},
		{"k zero", `{"query":"q","k":0}`, &recordingSearcher{}, "invalid_k"},
		{"k over max", `{"query":"q","k":26}`, &recordingSearcher{}, "invalid_k"},
		{"backend error", `{"query":"q","k":1}`, &recordingSearcher{err: context.DeadlineExceeded}, "backend_failure"},
		{"empty artifact id", `{"query":"q","k":1}`, &recordingSearcher{arts: []GraphArtifact{{ID: "  ", Title: "t"}}}, "invalid_artifact"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tool := NewSelfKnowledge(tc.searcher, "smackerel_self")
			res, err := tool.Execute(context.Background(), json.RawMessage(tc.params))
			if err != nil {
				t.Fatalf("Execute returned a Go error: %v", err)
			}
			if res.Error == nil {
				t.Fatalf("want ToolResult.Error code %q, got success", tc.wantCode)
			}
			if res.Error.Code != tc.wantCode {
				t.Errorf("error code = %q, want %q", res.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestNewSelfKnowledge_NilArgsPanic(t *testing.T) {
	t.Parallel()
	assertSemanticSearcherPanics(t, "nil searcher", func() { NewSelfKnowledge(nil, "smackerel_self") })
	assertSemanticSearcherPanics(t, "empty namespace", func() { NewSelfKnowledge(&recordingSearcher{}, "  ") })
}
