//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/bookmarks"
)

// T-2-06: FilterNew returns only artifacts whose URLs are not already in the database.
func TestFilterNew_ReturnsOnlyNewURLs(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)

	// Insert a known bookmark artifact
	knownURL := "https://example.com/known-" + tid
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, source_id, source_ref, artifact_type, title, created_at, updated_at)
		VALUES ($1, 'bookmarks', $2, 'bookmark', 'Known Bookmark', NOW(), NOW())
	`, "test-"+tid, knownURL)
	if err != nil {
		t.Fatalf("insert known artifact: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM artifacts WHERE id = $1`, "test-"+tid)

	dedup := bookmarks.NewURLDeduplicator(pool)

	artifacts := []connector.RawArtifact{
		{URL: knownURL, SourceRef: knownURL, Title: "Known"},
		{URL: "https://example.com/new-" + tid, SourceRef: "https://example.com/new-" + tid, Title: "New"},
	}

	result, dupes, err := dedup.FilterNew(ctx, artifacts)
	if err != nil {
		t.Fatalf("FilterNew() error: %v", err)
	}

	if dupes != 1 {
		t.Errorf("dupes = %d, want 1", dupes)
	}
	if len(result) != 1 {
		t.Errorf("result len = %d, want 1", len(result))
	}
	if len(result) > 0 && result[0].Title != "New" {
		t.Errorf("result[0].Title = %q, want %q", result[0].Title, "New")
	}
}

// T-2-07: FilterNew returns all artifacts when none are known.
func TestFilterNew_AllNewWhenNoneKnown(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	dedup := bookmarks.NewURLDeduplicator(pool)

	artifacts := []connector.RawArtifact{
		{URL: "https://unique-a-" + tid + ".example.com", SourceRef: "a", Title: "A"},
		{URL: "https://unique-b-" + tid + ".example.com", SourceRef: "b", Title: "B"},
	}

	result, dupes, err := dedup.FilterNew(ctx, artifacts)
	if err != nil {
		t.Fatalf("FilterNew() error: %v", err)
	}
	if dupes != 0 {
		t.Errorf("dupes = %d, want 0", dupes)
	}
	if len(result) != 2 {
		t.Errorf("result len = %d, want 2", len(result))
	}
}

// T-2-08: IsKnown returns true for an existing bookmark URL.
func TestIsKnown_ReturnsTrueForExistingURL(t *testing.T) {
	pool := testPool(t)
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tid := testID(t)
	knownURL := "https://example.com/isknown-" + tid

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, source_id, source_ref, artifact_type, title, created_at, updated_at)
		VALUES ($1, 'bookmarks', $2, 'bookmark', 'Known', NOW(), NOW())
	`, "test-ik-"+tid, knownURL)
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	defer pool.Exec(ctx, `DELETE FROM artifacts WHERE id = $1`, "test-ik-"+tid)

	dedup := bookmarks.NewURLDeduplicator(pool)

	known, err := dedup.IsKnown(ctx, knownURL)
	if err != nil {
		t.Fatalf("IsKnown() error: %v", err)
	}
	if !known {
		t.Error("IsKnown() = false, want true for existing URL")
	}

	unknown, err := dedup.IsKnown(ctx, "https://example.com/unknown-"+tid)
	if err != nil {
		t.Fatalf("IsKnown() error: %v", err)
	}
	if unknown {
		t.Error("IsKnown() = true, want false for unknown URL")
	}
}
