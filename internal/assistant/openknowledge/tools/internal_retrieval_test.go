package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

type stubSearcher struct {
	out []GraphArtifact
	err error
	got struct {
		query string
		k     int
	}
}

func (s *stubSearcher) Search(_ context.Context, q string, k int) ([]GraphArtifact, error) {
	s.got.query = q
	s.got.k = k
	return s.out, s.err
}

func TestInternalRetrieval_HappyPath(t *testing.T) {
	stub := &stubSearcher{out: []GraphArtifact{
		{ID: "a1", Title: "Pasta", Summary: "Italian noodles"},
		{ID: "a2", Title: "Risotto", Summary: ""},
	}}
	tool := NewInternalRetrieval(stub)

	res, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"food","k":5}`))
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %v", res.Error)
	}
	if stub.got.query != "food" || stub.got.k != 5 {
		t.Errorf("searcher got query=%q k=%d", stub.got.query, stub.got.k)
	}
	if len(res.Snippets) != 2 || len(res.Sources) != 2 {
		t.Fatalf("expected 2 snippets and 2 sources, got %d/%d", len(res.Snippets), len(res.Sources))
	}
	// Verify deterministic hash matches canonical form.
	expected := "Pasta\n\nItalian noodles"
	sum := sha256.Sum256([]byte(expected))
	wantHash := hex.EncodeToString(sum[:])
	if res.Snippets[0].Text != expected {
		t.Errorf("snippet[0].Text = %q want %q", res.Snippets[0].Text, expected)
	}
	if res.Snippets[0].ContentHash != wantHash {
		t.Errorf("snippet[0].ContentHash = %q want %q", res.Snippets[0].ContentHash, wantHash)
	}
	if res.Snippets[0].SourceRef != "a1" {
		t.Errorf("snippet[0].SourceRef = %q want a1", res.Snippets[0].SourceRef)
	}
	// Empty summary must collapse to title-only canonical form.
	if res.Snippets[1].Text != "Risotto" {
		t.Errorf("snippet[1].Text = %q want %q", res.Snippets[1].Text, "Risotto")
	}
	if res.Sources[0].Kind != ok.SourceArtifact {
		t.Errorf("source[0].Kind = %v want SourceArtifact", res.Sources[0].Kind)
	}
	if res.Sources[0].Artifact == nil || res.Sources[0].Artifact.ID != "a1" {
		t.Errorf("source[0].Artifact not populated: %+v", res.Sources[0].Artifact)
	}
}

func TestInternalRetrieval_ZeroResultsIsNotError(t *testing.T) {
	tool := NewInternalRetrieval(&stubSearcher{out: nil})
	res, err := tool.Execute(context.Background(), json.RawMessage(`{"query":"nothing","k":3}`))
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("unexpected tool error: %v", res.Error)
	}
	if len(res.Snippets) != 0 || len(res.Sources) != 0 {
		t.Errorf("expected empty snippets/sources, got %d/%d", len(res.Snippets), len(res.Sources))
	}
}

func TestInternalRetrieval_BackendErrorWrapped(t *testing.T) {
	stub := &stubSearcher{err: errors.New("pgx: boom")}
	tool := NewInternalRetrieval(stub)
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"x","k":1}`))
	if res.Error == nil || res.Error.Code != ErrInternalRetrievalBackend.Code {
		t.Fatalf("expected backend_failure error, got %v", res.Error)
	}
	if !strings.Contains(res.Error.Message, "pgx: boom") {
		t.Errorf("expected wrapped backend message, got %q", res.Error.Message)
	}
}

func TestInternalRetrieval_MalformedParams(t *testing.T) {
	tool := NewInternalRetrieval(&stubSearcher{})
	cases := []struct {
		name   string
		params string
	}{
		{"not_json", `not json`},
		{"missing_query", `{"k":1}`},
		{"missing_k", `{"query":"x"}`},
		{"unknown_field", `{"query":"x","k":1,"extra":true}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, _ := tool.Execute(context.Background(), json.RawMessage(c.params))
			if res.Error == nil || res.Error.Code != ErrInternalRetrievalMalformed.Code {
				t.Fatalf("expected malformed_params, got %v", res.Error)
			}
		})
	}
}

func TestInternalRetrieval_KOutOfRange(t *testing.T) {
	tool := NewInternalRetrieval(&stubSearcher{})
	cases := []string{
		`{"query":"x","k":0}`,
		`{"query":"x","k":-1}`,
		`{"query":"x","k":26}`, // MaxInternalRetrievalK + 1
	}
	for _, p := range cases {
		res, _ := tool.Execute(context.Background(), json.RawMessage(p))
		if res.Error == nil || res.Error.Code != ErrInternalRetrievalK.Code {
			t.Errorf("params %s: expected invalid_k, got %v", p, res.Error)
		}
	}
}

func TestInternalRetrieval_EmptyQueryRejected(t *testing.T) {
	tool := NewInternalRetrieval(&stubSearcher{})
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"   ","k":1}`))
	if res.Error == nil || res.Error.Code != ErrInternalRetrievalQuery.Code {
		t.Fatalf("expected invalid_query, got %v", res.Error)
	}
}

// Adversarial: would catch a regression where the graph adapter starts
// returning rows with empty IDs (e.g. NULL → empty string scan drift).
// Without this guard the cite-back verifier in SCOPE-08 would silently
// accept ungrounded citations.
func TestInternalRetrieval_RejectsEmptyArtifactID(t *testing.T) {
	stub := &stubSearcher{out: []GraphArtifact{
		{ID: "good", Title: "T", Summary: "S"},
		{ID: "", Title: "leaked", Summary: ""},
	}}
	tool := NewInternalRetrieval(stub)
	res, _ := tool.Execute(context.Background(), json.RawMessage(`{"query":"x","k":5}`))
	if res.Error == nil || res.Error.Code != ErrInvalidArtifact.Code {
		t.Fatalf("expected invalid_artifact, got %v", res.Error)
	}
}

func TestInternalRetrieval_NilSearcherPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil searcher")
		}
	}()
	NewInternalRetrieval(nil)
}
