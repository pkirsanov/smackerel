//go:build integration

// ingest_test.go — spec 104 SCOPE-03 integration test.
//
// Proves the Ingestor's orchestration against REAL PostgreSQL:
//   - first ingest publishes one smackerel_self artifact per derived entry;
//   - re-ingest is idempotent (all dedup by content_hash: 0 published, 0 swept);
//   - a stale smackerel_self row (a body no longer in the corpus) is swept.
//
// The fakePublisher faithfully mirrors pipeline.RawArtifactPublisher's insert +
// content_hash dedup (ON CONFLICT DO NOTHING) MINUS the NATS embedding publish,
// so this test exercises the NEW code (Ingestor orchestration + stale sweep)
// without a NATS dependency. The real publisher + embedding path is exercised
// at boot (SCOPE-04 wiring) and end-to-end (SCOPE-08).

package selfknowledge_integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/assistant"
	"github.com/smackerel/smackerel/internal/assistant/selfknowledge"
	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/extract"
)

// fakePublisher mirrors RawArtifactPublisher.PublishRawArtifact's DB behaviour
// (insert + content_hash dedup) without the NATS embedding publish.
type fakePublisher struct{ pool *pgxpool.Pool }

func (f fakePublisher) PublishRawArtifact(ctx context.Context, a connector.RawArtifact) (string, error) {
	id := ulid.Make().String()
	hash := extract.HashContent(a.RawContent)
	ct, err := f.pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, source_url, processing_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending')
		ON CONFLICT (content_hash) WHERE content_hash IS NOT NULL DO NOTHING
	`, id, a.ContentType, a.Title, a.RawContent, hash, a.SourceID, a.URL)
	if err != nil {
		return "", err
	}
	if ct.RowsAffected() == 0 {
		return "", nil
	}
	return id, nil
}

func openSelfKnowledgePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func loadRealManifest(t *testing.T) *assistant.SkillsManifest {
	t.Helper()
	path := filepath.Join("..", "..", "..", "config", "assistant", "scenarios.yaml")
	m, err := assistant.LoadSkillsManifest(path, func(string) (bool, bool) { return true, true })
	if err != nil {
		t.Fatalf("LoadSkillsManifest(%s): %v", path, err)
	}
	return m
}

func cleanupSelfKnowledge(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `DELETE FROM artifacts WHERE source_id = $1`, selfknowledge.SelfKnowledgeNamespace); err != nil {
		t.Logf("cleanup smackerel_self: %v", err)
	}
}

func countSelfKnowledge(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM artifacts WHERE source_id = $1`, selfknowledge.SelfKnowledgeNamespace).Scan(&n); err != nil {
		t.Fatalf("count smackerel_self: %v", err)
	}
	return n
}

func insertRawSelfKnowledge(t *testing.T, pool *pgxpool.Pool, title, body string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id, source_url, processing_status)
		VALUES ($1, 'capability', $2, $3, $4, $5, $6, 'pending')
	`, ulid.Make().String(), title, body, extract.HashContent(body), selfknowledge.SelfKnowledgeNamespace, "stale:"+title)
	if err != nil {
		t.Fatalf("insert stale self-knowledge: %v", err)
	}
}

func TestIngestor_IdempotentWithStaleSweep(t *testing.T) {
	pool := openSelfKnowledgePool(t)
	cleanupSelfKnowledge(t, pool)
	t.Cleanup(func() { cleanupSelfKnowledge(t, pool) })

	ing := selfknowledge.NewIngestor(loadRealManifest(t), fakePublisher{pool: pool}, pool)
	ctx := context.Background()

	// First ingest: one artifact per derived entry.
	r1, err := ing.Ingest(ctx)
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if r1.Entries == 0 {
		t.Fatal("Ingest derived 0 entries")
	}
	if r1.Published != r1.Entries {
		t.Fatalf("first ingest published %d, want %d (one per entry)", r1.Published, r1.Entries)
	}
	if got := countSelfKnowledge(t, pool); got != r1.Entries {
		t.Fatalf("smackerel_self count = %d, want %d after first ingest", got, r1.Entries)
	}

	// Re-ingest: idempotent (all dedup, nothing published, nothing swept).
	r2, err := ing.Ingest(ctx)
	if err != nil {
		t.Fatalf("re-Ingest: %v", err)
	}
	if r2.Published != 0 {
		t.Errorf("re-ingest published %d, want 0 (idempotent)", r2.Published)
	}
	if r2.Swept != 0 {
		t.Errorf("re-ingest swept %d, want 0", r2.Swept)
	}
	if got := countSelfKnowledge(t, pool); got != r1.Entries {
		t.Fatalf("smackerel_self count = %d, want %d after re-ingest", got, r1.Entries)
	}

	// Inject a stale row (a body no longer in the corpus), then ingest: swept.
	insertRawSelfKnowledge(t, pool, "removed-capability", "an old removed capability description "+time.Now().Format(time.RFC3339Nano))
	if got := countSelfKnowledge(t, pool); got != r1.Entries+1 {
		t.Fatalf("smackerel_self count = %d, want %d after injecting stale row", got, r1.Entries+1)
	}
	r3, err := ing.Ingest(ctx)
	if err != nil {
		t.Fatalf("Ingest after stale: %v", err)
	}
	if r3.Swept != 1 {
		t.Errorf("swept %d, want 1 (the stale row)", r3.Swept)
	}
	if got := countSelfKnowledge(t, pool); got != r1.Entries {
		t.Fatalf("smackerel_self count = %d, want %d after stale sweep", got, r1.Entries)
	}
}
