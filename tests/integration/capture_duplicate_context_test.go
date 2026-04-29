//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/pipeline"
)

// TestBUG001_MergeUserContext_AppendsTwoContexts is the live-stack regression
// for BUG-001 (Scope 1 of spec 008): re-sharing the same URL with new context
// must append the new context to artifacts.metadata.user_contexts.
//
// Before the fix, no MergeUserContext helper existed and metadata stayed NULL
// after duplicate POSTs. After the fix, two sequential merges produce a
// JSONB array containing both context strings in submission order.
func TestBUG001_MergeUserContext_AppendsTwoContexts(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	artifactID := ulid.Make().String()
	cleanupArtifact(t, pool, artifactID)

	// Seed a stub artifact with NULL metadata to mirror the production
	// "first capture, no contexts merged yet" state.
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, source_url, processing_status)
		VALUES ($1, 'article', 'BUG-001 test artifact', $2, 'api', $3, 'pending')
	`, artifactID, "bug001-hash-"+artifactID, "https://example.com/bug001-"+artifactID); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	if err := pipeline.MergeUserContext(ctx, pool, artifactID, "first context"); err != nil {
		t.Fatalf("first MergeUserContext failed: %v", err)
	}
	if err := pipeline.MergeUserContext(ctx, pool, artifactID, "second context"); err != nil {
		t.Fatalf("second MergeUserContext failed: %v", err)
	}

	var raw []byte
	if err := pool.QueryRow(ctx,
		`SELECT metadata->'user_contexts' FROM artifacts WHERE id = $1`,
		artifactID,
	).Scan(&raw); err != nil {
		t.Fatalf("read user_contexts: %v", err)
	}

	var got []string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal user_contexts %q: %v", string(raw), err)
	}

	want := []string{"first context", "second context"}
	if len(got) != len(want) {
		t.Fatalf("user_contexts length = %d (%v); want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("user_contexts[%d] = %q; want %q", i, got[i], want[i])
		}
	}

	// Empty-context merge must not modify the array.
	if err := pipeline.MergeUserContext(ctx, pool, artifactID, ""); err != nil {
		t.Fatalf("no-op MergeUserContext failed: %v", err)
	}
	var afterRaw []byte
	if err := pool.QueryRow(ctx,
		`SELECT metadata->'user_contexts' FROM artifacts WHERE id = $1`,
		artifactID,
	).Scan(&afterRaw); err != nil {
		t.Fatalf("re-read user_contexts: %v", err)
	}
	if string(afterRaw) != string(raw) {
		t.Errorf("empty-context merge mutated metadata: before=%s after=%s", raw, afterRaw)
	}
}
