// Spec 095 SCOPE-04 — whole_document strategy tests.
package wholedocument

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// fakeFetcher returns a multi-chunk artifact whose FULL content includes a
// sentinel that lives ONLY in a late chunk — so a top-k(1) subset would miss
// it. This is the no-tautology fixture: the whole-document answer is genuinely
// different from a chunk-subset answer.
type fakeFetcher struct {
	chunks    []string
	lastErr   error
	gotFullID string
}

func (f *fakeFetcher) FetchFullArtifact(_ context.Context, id string) (routing.FullArtifact, error) {
	if f.lastErr != nil {
		return routing.FullArtifact{}, f.lastErr
	}
	f.gotFullID = id
	return routing.FullArtifact{
		ID:        id,
		Type:      "transcript",
		Title:     "March 5th meeting",
		Content:   strings.Join(f.chunks, "\n"),
		NumChunks: len(f.chunks),
	}, nil
}

// topKSubset models the legacy §9.2 behaviour: only the first k chunks.
func topKSubset(chunks []string, k int) string {
	if k > len(chunks) {
		k = len(chunks)
	}
	return strings.Join(chunks[:k], "\n")
}

// TestFetchesFullArtifact — SCN-095-A02: the strategy fetches the FULL artifact
// (all chunks), proven by a sentinel that only a complete fetch surfaces.
func TestFetchesFullArtifact(t *testing.T) {
	const sentinel = "DECISION: ship on the 12th" // lives only in the last chunk
	chunks := []string{
		"intro and agenda for the meeting",
		"discussion about pricing summary",
		"action items review",
		sentinel,
	}
	f := &fakeFetcher{chunks: chunks}
	s := New(f)

	res, err := s.Execute(context.Background(), routing.RetrievalRequest{ArtifactID: "art-1", ArtifactType: "transcript"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.FullArtifact {
		t.Error("result.FullArtifact should be true (whole document fetched)")
	}
	if res.Strategy != routing.StrategyWholeDocument {
		t.Errorf("strategy = %s, want whole_document", res.Strategy)
	}
	if !strings.Contains(res.Answer, sentinel) {
		t.Errorf("answer should contain the late-chunk sentinel %q (full doc), got: %q", sentinel, res.Answer)
	}
	if f.gotFullID != "art-1" {
		t.Errorf("fetcher should be asked for the full artifact id art-1, got %q", f.gotFullID)
	}
	if len(res.Sources) != 1 || res.Sources[0].Kind != routing.SourceFullArtifact || res.Sources[0].ArtifactID != "art-1" {
		t.Errorf("result should cite the full artifact, got %+v", res.Sources)
	}

	// No-tautology guard: a top-k(1) subset would MISS the sentinel, so the
	// whole-document answer is genuinely different from a chunk-subset answer.
	if strings.Contains(topKSubset(chunks, 1), sentinel) {
		t.Fatal("fixture invalid: sentinel must NOT be in the top-1 chunk (otherwise the test is tautological)")
	}
}

// TestExecute_EmptyArtifactID_Errors — the strategy cannot fetch a whole
// document without a target id (it must NOT silently fall back to chunks).
func TestExecute_EmptyArtifactID_Errors(t *testing.T) {
	s := New(&fakeFetcher{chunks: []string{"x"}})
	if _, err := s.Execute(context.Background(), routing.RetrievalRequest{ArtifactID: ""}); err == nil {
		t.Fatal("Execute should error on an empty ArtifactID")
	}
}
