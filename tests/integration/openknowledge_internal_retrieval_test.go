//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
)

// TestOpenKnowledgeInternalRetrieval_LiveSearch drives the
// internal_retrieval tool against the disposable test compose (live
// PostgreSQL via DATABASE_URL). Fixtures are inserted with t.Cleanup
// removal so the test leaves zero residue (G028 test isolation).
func TestOpenKnowledgeInternalRetrieval_LiveSearch(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	seed := []struct {
		id, title, summary string
	}{
		{testID(t) + "-1", "Sourdough fermentation notes", "Bulk ferment timing and starter ratios"},
		{testID(t) + "-2", "Pasta dough hydration", "Egg vs water pasta ratios for sourdough comparison"},
		{testID(t) + "-3", "Garden compost log", "Carbon to nitrogen balance for fall leaves"},
	}
	for _, s := range seed {
		cleanupArtifact(t, pool, s.id)
		_, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, summary, content_hash, source_id, created_at, updated_at)
			VALUES ($1, 'note', $2, $3, $4, 'test-source', NOW(), NOW())
		`, s.id, s.title, s.summary, fmt.Sprintf("hash-%s", s.id))
		if err != nil {
			t.Fatalf("seed artifact %s: %v", s.id, err)
		}
	}

	tool := tools.NewInternalRetrieval(tools.NewPgxGraphSearcher(pool))

	// Happy path: matches the two sourdough-related rows.
	res, err := tool.Execute(ctx, json.RawMessage(`{"query":"sourdough","k":5}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Error != nil {
		t.Fatalf("tool error: %v", res.Error)
	}
	if len(res.Snippets) < 2 {
		t.Fatalf("expected >=2 matches for sourdough, got %d", len(res.Snippets))
	}
	if len(res.Snippets) != len(res.Sources) {
		t.Fatalf("snippet/source length mismatch: %d vs %d", len(res.Snippets), len(res.Sources))
	}
	wantIDs := map[string]bool{seed[0].id: false, seed[1].id: false}
	for i, s := range res.Sources {
		if s.Kind != ok.SourceArtifact {
			t.Errorf("source[%d].Kind = %v want SourceArtifact", i, s.Kind)
		}
		if s.Artifact == nil || s.Artifact.ID == "" {
			t.Fatalf("source[%d] missing artifact id", i)
		}
		if _, ok := wantIDs[s.Artifact.ID]; ok {
			wantIDs[s.Artifact.ID] = true
		}
		if res.Snippets[i].SourceRef != s.Artifact.ID {
			t.Errorf("snippet[%d].SourceRef=%q != source artifact id %q", i, res.Snippets[i].SourceRef, s.Artifact.ID)
		}
		if res.Snippets[i].ContentHash == "" {
			t.Errorf("snippet[%d].ContentHash empty", i)
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Errorf("expected seeded artifact %s in results", id)
		}
	}

	// Adversarial: query for text absent from every seeded row must
	// return zero snippets without error. A regression that widens the
	// ILIKE pattern or drops the WHERE clause would surface here.
	resEmpty, err := tool.Execute(ctx, json.RawMessage(`{"query":"zzzzzz-no-match-`+testID(t)+`","k":5}`))
	if err != nil {
		t.Fatalf("execute empty: %v", err)
	}
	if resEmpty.Error != nil {
		t.Fatalf("unexpected tool error on empty match: %v", resEmpty.Error)
	}
	if len(resEmpty.Snippets) != 0 || len(resEmpty.Sources) != 0 {
		t.Errorf("expected empty results, got snippets=%d sources=%d", len(resEmpty.Snippets), len(resEmpty.Sources))
	}
}
