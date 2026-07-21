//go:build e2e

package assistant_e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func isolateRequiredAssistantConversation(t *testing.T, stack httpTurnLiveStack) {
	t.Helper()
	reset := func(label string) {
		req := httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "test-bug069005-reset-" + label + "-" + timestamp(),
			Kind:               string(contracts.KindReset),
			TransportHint:      "web",
		}
		resp, raw := postAssistantTurn(t, stack, req)
		if resp.StatusCode != 200 {
			t.Fatalf("%s reset status = %d, want 200; body=%s", label, resp.StatusCode, string(raw))
		}
	}
	reset("before")
	t.Cleanup(func() { reset("after") })
}

func openRequiredAssistantPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("required assistant E2E needs DATABASE_URL from ./smackerel.sh test e2e")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open required assistant database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedAnnotationArtifact(t *testing.T, pool *pgxpool.Pool, artifactID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, processing_status, capture_method)
VALUES ($1, 'note', $2, $3, 'test:bug069005', 'completed', 'test')
`, artifactID, "BUG-069-005 annotation target", artifactID+"-hash")
	if err != nil {
		t.Fatalf("seed annotation artifact: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM annotations WHERE artifact_id = $1`, artifactID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM artifacts WHERE id = $1`, artifactID)
	})
}

func annotationCount(t *testing.T, pool *pgxpool.Pool, artifactID string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM annotations WHERE artifact_id = $1`, artifactID).Scan(&count); err != nil {
		t.Fatalf("count annotations: %v", err)
	}
	return count
}

func listCountBySourceQuery(t *testing.T, pool *pgxpool.Pool, sourceQuery string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM lists WHERE source_query = $1`, sourceQuery).Scan(&count); err != nil {
		t.Fatalf("count lists: %v", err)
	}
	return count
}

func assertSingleListItem(t *testing.T, pool *pgxpool.Pool, sourceQuery, expectedItem string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var listID, item string
	err := pool.QueryRow(ctx, `
SELECT l.id, li.content
  FROM lists l
  JOIN list_items li ON li.list_id = l.id
 WHERE l.source_query = $1
`, sourceQuery).Scan(&listID, &item)
	if err != nil {
		t.Fatalf("load confirmed list item: %v", err)
	}
	if item != expectedItem {
		t.Fatalf("confirmed list item = %q, want compiled slot %q (list=%s)", item, expectedItem, listID)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM lists WHERE id = $1`, listID)
	})
}

func pendingDisambiguationChoiceCount(t *testing.T, pool *pgxpool.Pool, ref string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	err := pool.QueryRow(ctx, `
SELECT COALESCE(jsonb_array_length(pending_disambig->'choices'), 0)
  FROM assistant_conversations
 WHERE pending_disambig->>'disambiguation_ref' = $1
`, ref).Scan(&count)
	if err != nil {
		t.Fatalf("load pending disambiguation %s: %v", ref, err)
	}
	return count
}

func pendingDisambiguationRows(t *testing.T, pool *pgxpool.Pool, ref string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	err := pool.QueryRow(ctx, `
SELECT COUNT(*)
  FROM assistant_conversations
 WHERE pending_disambig->>'disambiguation_ref' = $1
`, ref).Scan(&count)
	if err != nil {
		t.Fatalf("count pending disambiguation %s: %v", ref, err)
	}
	return count
}

func assertAnnotationSlots(t *testing.T, pool *pgxpool.Pool, artifactID, actorID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `
SELECT annotation_type, COALESCE(rating, 0), COALESCE(note, ''), COALESCE(interaction_type, '')
  FROM annotations
 WHERE artifact_id = $1 AND actor_id = $2
 ORDER BY annotation_type
`, artifactID, actorID)
	if err != nil {
		t.Fatalf("query compiled annotations: %v", err)
	}
	defer rows.Close()
	found := map[string]string{}
	for rows.Next() {
		var annotationType, note, interaction string
		var rating int
		if err := rows.Scan(&annotationType, &rating, &note, &interaction); err != nil {
			t.Fatalf("scan compiled annotation: %v", err)
		}
		found[annotationType] = fmt.Sprintf("%d|%s|%s", rating, note, interaction)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate compiled annotations: %v", err)
	}
	if found["interaction"] != "0||made_it" {
		t.Fatalf("interaction annotation = %q, want compiled made_it slot", found["interaction"])
	}
	if found["rating"] != "4||" {
		t.Fatalf("rating annotation = %q, want compiled rating 4", found["rating"])
	}
	if found["note"] != "0|needs more garlic|" {
		t.Fatalf("note annotation = %q, want compiled note slot", found["note"])
	}
}
