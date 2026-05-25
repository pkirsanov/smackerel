//go:build integration

// BUG-027-002 — F2 regression test (sweep-2026-05-25-r10 round 3 /
// stabilize-to-doc).
//
// Validates that updateRelevanceFromAnnotation is atomic under
// concurrent NATS callbacks for the same artifact. Before the fix the
// function performed a classic read-modify-write:
//
//     SELECT relevance_score FROM artifacts WHERE id = $1
//     <compute newScore = currentScore + delta>
//     UPDATE artifacts SET relevance_score = $1 WHERE id = $2
//
// Two concurrent callbacks for the same artifact would both observe
// the same prior score, compute their own newScore, and then race the
// UPDATE — last writer wins, losing one delta. The fix collapses the
// arithmetic into a single SQL statement so PostgreSQL's row-level
// write lock serializes concurrent UPDATEs and every delta is applied
// exactly once.
//
// Adversarial design: this test launches N=20 goroutines per artifact
// that each apply a +0.02 delta (TypeTagAdd) to the SAME artifact in
// burst. With the pre-fix code this test reliably fails under -race
// because the final relevance_score < 0.5 + 20*0.02 = 0.9. With the
// fix, the final relevance_score is exactly 0.9 (within float
// epsilon) because every delta lands atomically.
//
// No `t.Skip()` — when DATABASE_URL is unset the test fatals with an
// actionable message, mirroring the spec 043/044 no-skip precedent.

package integration

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// intelligenceRacePool opens a pgx pool against the live test stack
// DATABASE_URL and applies migrations. Mirrors the authTestPool helper
// in auth_bootstrap_test.go but uses a higher MaxConns budget so the
// race goroutines do not queue on the pool acquire path (we want them
// hitting the DB UPDATE concurrently, not serialized inside the
// client-side pool).
func intelligenceRacePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("intelligence annotation race integration test requires DATABASE_URL — run via `./smackerel.sh test integration` which brings up the live test stack and exports DATABASE_URL")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 32
	cfg.MinConns = 0

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// insertRaceArtifact inserts a fresh artifact row with a known
// relevance_score and returns the artifact id. The id is a uuid so
// concurrent test runs cannot collide.
func insertRaceArtifact(t *testing.T, pool *pgxpool.Pool, initialScore float64) string {
	t.Helper()
	id := "race-" + uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO artifacts (
			id, artifact_type, title, content_hash, source_id, relevance_score
		) VALUES (
			$1, 'test', 'BUG-027-002 race fixture', $2, 'race-test', $3
		)
	`, id, id+"-hash", initialScore)
	if err != nil {
		t.Fatalf("insert race fixture: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cleanupCtx, `DELETE FROM artifacts WHERE id = $1`, id); err != nil {
			t.Logf("cleanup race fixture %s: %v", id, err)
		}
	})

	return id
}

// readRelevanceScore returns the current relevance_score for the
// artifact row.
func readRelevanceScore(t *testing.T, pool *pgxpool.Pool, id string) float64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var score float64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(relevance_score, 0.5) FROM artifacts WHERE id = $1`, id).Scan(&score); err != nil {
		t.Fatalf("read relevance score: %v", err)
	}
	return score
}

// TestIntelligenceAnnotation_AtomicConcurrentDeltas is the canonical
// F2 regression guard. Spawning N goroutines that each apply a fixed
// +0.02 delta to the same artifact must yield final_score =
// initial + N*delta after the fix. Before the fix, lost updates would
// drop the final below the sum.
func TestIntelligenceAnnotation_AtomicConcurrentDeltas(t *testing.T) {
	pool := intelligenceRacePool(t)
	defer pool.Close()

	const (
		initialScore = 0.5
		delta        = 0.02 // TypeTagAdd delta from annotationRelevanceDelta
		numWriters   = 20
		expectedSum  = initialScore + delta*float64(numWriters) // 0.9
		epsilon      = 1e-6
	)

	artifactID := insertRaceArtifact(t, pool, initialScore)
	engine := intelligence.NewEngine(pool, nil /* nats not needed for direct call */)

	// Use the same exported entry point as the NATS subscriber.
	// updateRelevanceFromAnnotation is unexported, so go through the
	// exported HandleAnnotationCreated path which calls it internally.
	// If HandleAnnotationCreated does not exist, fall back to the
	// exported test-only wrapper added for this bug.
	//
	// (intelligence.Engine has no exported wrapper today; the
	// callback fans out from SubscribeAnnotations. For a focused
	// race test we exercise the package-internal helper via the
	// only-exported path that hits the same SQL: ApplyAnnotation.)
	//
	// If no exported wrapper exists in the package, we fall back to
	// publishing through NATS but for a focused race regression the
	// helper is added to intelligence.Engine in this fix.

	// Build a fixed-shape annotation that yields the +0.02 delta.
	mkAnn := func(i int) *annotation.Annotation {
		return &annotation.Annotation{
			ID:             fmt.Sprintf("race-ann-%d", i),
			ArtifactID:     artifactID,
			AnnotationType: annotation.TypeTagAdd,
			Tag:            "weeknight",
		}
	}

	// Fan out N concurrent callers. start gate ensures all goroutines
	// race the UPDATE at once instead of starting in registration
	// order.
	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make(chan error, numWriters)
	)
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		ann := mkAnn(i)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := engine.ApplyAnnotationRelevanceForTest(ctx, ann); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent annotation apply failed: %v", err)
	}

	finalScore := readRelevanceScore(t, pool, artifactID)
	if math.Abs(finalScore-expectedSum) > epsilon {
		t.Fatalf("BUG-027-002 F2 race regression: final relevance_score=%f, want %f (initial=%f + %d * %f); diff=%f exceeds epsilon=%f. The pre-fix read-modify-write race would manifest as final < %f due to lost updates.",
			finalScore, expectedSum, initialScore, numWriters, delta, math.Abs(finalScore-expectedSum), epsilon, expectedSum)
	}
}

// TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne verifies the
// SQL-level clamp [0, 1]. Spawning enough writers to push the score
// past 1.0 must clamp at exactly 1.0 instead of overflowing.
func TestIntelligenceAnnotation_AtomicConcurrentClampsAtOne(t *testing.T) {
	pool := intelligenceRacePool(t)
	defer pool.Close()

	const (
		initialScore = 0.5
		delta        = 0.15 // TypeRating rating=5 delta
		numWriters   = 20   // 0.5 + 20*0.15 = 3.5 → clamp 1.0
		epsilon      = 1e-6
	)

	artifactID := insertRaceArtifact(t, pool, initialScore)
	engine := intelligence.NewEngine(pool, nil)

	rating := 5
	mkAnn := func(i int) *annotation.Annotation {
		return &annotation.Annotation{
			ID:             fmt.Sprintf("clamp-ann-%d", i),
			ArtifactID:     artifactID,
			AnnotationType: annotation.TypeRating,
			Rating:         &rating,
		}
	}

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make(chan error, numWriters)
	)
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		ann := mkAnn(i)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := engine.ApplyAnnotationRelevanceForTest(ctx, ann); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent annotation apply failed: %v", err)
	}

	finalScore := readRelevanceScore(t, pool, artifactID)
	if math.Abs(finalScore-1.0) > epsilon {
		t.Fatalf("BUG-027-002 F2 clamp regression: final relevance_score=%f, want 1.0 (initial=%f + %d * %f = %f, clamped to 1.0); diff=%f exceeds epsilon=%f",
			finalScore, initialScore, numWriters, delta, initialScore+delta*float64(numWriters), math.Abs(finalScore-1.0), epsilon)
	}
}
