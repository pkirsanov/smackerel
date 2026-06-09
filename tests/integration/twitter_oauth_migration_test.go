//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BUG-056-002 Scope A DoD item 4 — prove migration 056_twitter_oauth_pkce.sql
// applies cleanly against a real disposable Postgres: both tables and the
// expires_at index exist after migrate, and the CREATE TABLE IF NOT EXISTS form
// is idempotent on re-apply. Mirrors recommendations_migration_test.go and
// reuses the shared testPool + tableExists helpers.

var twitterOAuthTables = []string{
	"twitter_oauth_states",
	"twitter_oauth_tokens",
}

const twitterOAuthDownSQL = `
DROP TABLE IF EXISTS twitter_oauth_tokens CASCADE;
DROP TABLE IF EXISTS twitter_oauth_states CASCADE;
`

func TestTwitterOAuthMigration_AppliesCleanly(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "internal", "db", "migrations", "056_twitter_oauth_pkce.sql"))
	if err != nil {
		t.Fatalf("read twitter oauth migration: %v", err)
	}

	// Clean slate (the live stack applied 056 at startup; drop to prove the SQL
	// applies from scratch, not merely that it ran once before).
	if _, err := pool.Exec(ctx, twitterOAuthDownSQL); err != nil {
		t.Fatalf("drop twitter_oauth_* before test: %v", err)
	}
	assertTwitterOAuthTablesAbsent(t, ctx, pool)

	// Apply → both tables + the expires_at index present.
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply twitter oauth migration: %v", err)
	}
	assertTwitterOAuthTablesPresent(t, ctx, pool)
	assertTwitterOAuthIndexPresent(t, ctx, pool)

	// Idempotent: CREATE TABLE IF NOT EXISTS re-applies without error.
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("re-apply twitter oauth migration (idempotency): %v", err)
	}
	assertTwitterOAuthTablesPresent(t, ctx, pool)
}

func assertTwitterOAuthTablesPresent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, tbl := range twitterOAuthTables {
		if !tableExists(t, ctx, pool, tbl) {
			t.Fatalf("expected table %s to exist after migrate", tbl)
		}
	}
}

func assertTwitterOAuthTablesAbsent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, tbl := range twitterOAuthTables {
		if tableExists(t, ctx, pool, tbl) {
			t.Fatalf("expected table %s to be absent before migrate", tbl)
		}
	}
}

func assertTwitterOAuthIndexPresent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)",
		"idx_twitter_oauth_states_expires_at").Scan(&exists); err != nil {
		t.Fatalf("check twitter oauth index: %v", err)
	}
	if !exists {
		t.Fatal("expected index idx_twitter_oauth_states_expires_at to exist after migrate")
	}
}
