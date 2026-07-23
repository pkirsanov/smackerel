//go:build integration

// semantic_searcher_test.go — spec 104 SCOPE-01 integration test.
//
// Proves the general PgxSemanticSearcher against REAL pgvector:
//   - namespace scoping: a row in a DIFFERENT source_id namespace that is the
//     closest overall match is NOT returned (isolation, FR-5);
//   - cosine ordering: within the namespace, the nearer embedding ranks first.
//
// Deterministic: artifacts are seeded with explicit embeddings and the query
// is embedded by a fixed fake embedder, so no ML sidecar is required and the
// ordering is exact.

package openknowledge_integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/tools"
	"github.com/smackerel/smackerel/internal/db"
)

type fixedEmbedder struct{ vec []float32 }

func (f fixedEmbedder) Embed(_ context.Context, _ string) ([]float32, error) { return f.vec, nil }

func openSemanticPool(t *testing.T) *pgxpool.Pool {
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

// vec384 builds a 384-dim embedding (matching artifacts.embedding vector(384))
// with the given leading values, zero-padded.
func vec384(lead ...float32) []float32 {
	v := make([]float32, 384)
	copy(v, lead)
	return v
}

func insertEmbeddedArtifact(t *testing.T, pool *pgxpool.Pool, id, sourceID, title string, emb []float32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id, embedding)
		VALUES ($1, 'capability', $2, $3, $4, $5::vector)
	`, id, title, "h-"+id, sourceID, db.FormatEmbedding(emb))
	if err != nil {
		t.Fatalf("insert artifact %s: %v", id, err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, derr := pool.Exec(cctx, `DELETE FROM artifacts WHERE id = $1`, id); derr != nil {
			t.Logf("cleanup artifact %s: %v", id, derr)
		}
	})
}

func TestPgxSemanticSearcher_NamespaceScopedCosine(t *testing.T) {
	pool := openSemanticPool(t)
	pfx := "sk-sem-" + time.Now().Format("150405.000000")
	userNS := "user:" + pfx

	// Query vector points along dim 0.
	query := vec384(1, 0, 0)
	// smackerel_self: A near the query, B orthogonal (far).
	insertEmbeddedArtifact(t, pool, pfx+"-A", "smackerel_self", "capabilities overview", vec384(0.9, 0.1, 0))
	insertEmbeddedArtifact(t, pool, pfx+"-B", "smackerel_self", "unrelated", vec384(0, 1, 0))
	// A different namespace row that is the CLOSEST overall (identical to query)
	// but MUST NOT appear in a smackerel_self search (isolation, FR-5).
	insertEmbeddedArtifact(t, pool, pfx+"-Cuser", userNS, "private personal note", vec384(1, 0, 0))

	searcher := tools.NewPgxSemanticSearcher(pool, fixedEmbedder{vec: query})
	got, err := searcher.Search(context.Background(), "smackerel_self", "what can smackerel do", 25)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// The user-namespace row (closest overall) must be absent — isolation.
	for _, a := range got {
		if a.ID == pfx+"-Cuser" {
			t.Fatalf("user-namespace artifact %q leaked into a smackerel_self search", a.ID)
		}
	}

	// Filter to this run's rows (the shared test DB may hold other
	// smackerel_self rows once the ingestor lands). Order is preserved.
	var ids []string
	for _, a := range got {
		if len(a.ID) >= len(pfx) && a.ID[:len(pfx)] == pfx {
			ids = append(ids, a.ID)
		}
	}
	if len(ids) != 2 {
		t.Fatalf("got %d in-run smackerel_self rows, want 2 (ids=%v)", len(ids), ids)
	}
	// Cosine ordering: A (near) ranks before B (orthogonal).
	if ids[0] != pfx+"-A" {
		t.Fatalf("closest in-run row = %q, want %q-A (cosine ordering)", ids[0], pfx)
	}
}
