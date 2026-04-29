//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

var recommendationTables = []string{
	"recommendation_provider_runtime_state",
	"recommendation_preference_corrections",
	"recommendation_seen_state",
	"recommendation_suppression_state",
	"recommendation_feedback",
	"recommendation_delivery_attempts",
	"recommendations",
	"recommendation_candidate_provider_facts",
	"recommendation_candidates",
	"recommendation_provider_facts",
	"recommendation_requests",
	"recommendation_watch_rate_windows",
	"recommendation_watch_runs",
	"recommendation_watches",
}

const recommendationDownSQL = `
DROP TABLE IF EXISTS recommendation_provider_runtime_state CASCADE;
DROP TABLE IF EXISTS recommendation_preference_corrections CASCADE;
DROP TABLE IF EXISTS recommendation_seen_state CASCADE;
DROP TABLE IF EXISTS recommendation_suppression_state CASCADE;
DROP TABLE IF EXISTS recommendation_feedback CASCADE;
DROP TABLE IF EXISTS recommendation_delivery_attempts CASCADE;
DROP TABLE IF EXISTS recommendations CASCADE;
DROP TABLE IF EXISTS recommendation_candidate_provider_facts CASCADE;
DROP TABLE IF EXISTS recommendation_candidates CASCADE;
DROP TABLE IF EXISTS recommendation_provider_facts CASCADE;
DROP TABLE IF EXISTS recommendation_requests CASCADE;
DROP TABLE IF EXISTS recommendation_watch_rate_windows CASCADE;
DROP TABLE IF EXISTS recommendation_watch_runs CASCADE;
DROP TABLE IF EXISTS recommendation_watches CASCADE;
`

func TestRecommendationMigration_UpDownRoundTripIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "internal", "db", "migrations", "022_recommendations.sql"))
	if err != nil {
		t.Fatalf("read recommendation migration: %v", err)
	}

	if _, err := pool.Exec(ctx, recommendationDownSQL); err != nil {
		t.Fatalf("drop recommendation tables before test: %v", err)
	}
	assertRecommendationTablesAbsent(t, ctx, pool)

	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply recommendation migration first time: %v", err)
	}
	assertRecommendationTablesPresent(t, ctx, pool)
	assertRecommendationIndexesPresent(t, ctx, pool)

	if _, err := pool.Exec(ctx, recommendationDownSQL); err != nil {
		t.Fatalf("down migration: %v", err)
	}
	assertRecommendationTablesAbsent(t, ctx, pool)

	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("re-apply recommendation migration: %v", err)
	}
	assertRecommendationTablesPresent(t, ctx, pool)
}

func assertRecommendationTablesPresent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, table := range recommendationTables {
		if !tableExists(t, ctx, pool, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
}

func assertRecommendationTablesAbsent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, table := range recommendationTables {
		if tableExists(t, ctx, pool, table) {
			t.Fatalf("expected table %s to be absent", table)
		}
	}
}

func assertRecommendationIndexesPresent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, index := range []string{
		"idx_recommendation_watches_actor",
		"idx_recommendation_requests_trace",
		"idx_recommendation_candidates_title_trgm",
		"idx_recommendations_trace",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)", index).Scan(&exists); err != nil {
			t.Fatalf("check index %s: %v", index, err)
		}
		if !exists {
			t.Fatalf("expected index %s to exist", index)
		}
	}
}

type queryPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func tableExists(t *testing.T, ctx context.Context, pool queryPool, table string) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)", table).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	return exists
}