//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Spec 083 Card Rewards Companion (Scope 01) — T-01-04 / SCN-083-A05, A06.
// Prove migration 057_card_rewards.sql applies cleanly against a real
// disposable Postgres: all 10 card-rewards tables exist after migrate, the
// lifecycle CHECK constraint + the (card_catalog_id, period_label) UNIQUE
// index + the needs_verification NOT NULL default are present, and the
// CREATE ... IF NOT EXISTS form is idempotent on re-apply. Mirrors
// twitter_oauth_migration_test.go and reuses the shared testPool + tableExists
// helpers.

var cardRewardsTables = []string{
	"card_catalog",
	"card_runs",
	"user_cards",
	"card_offers",
	"card_selections",
	"signup_bonuses",
	"rotating_category_observations",
	"rotating_categories",
	"category_aliases",
	"card_recommendations",
}

// Drop in reverse dependency order; CASCADE clears FKs/indexes/constraints.
const cardRewardsDownSQL = `
DROP TABLE IF EXISTS card_recommendations CASCADE;
DROP TABLE IF EXISTS category_aliases CASCADE;
DROP TABLE IF EXISTS rotating_categories CASCADE;
DROP TABLE IF EXISTS rotating_category_observations CASCADE;
DROP TABLE IF EXISTS signup_bonuses CASCADE;
DROP TABLE IF EXISTS card_selections CASCADE;
DROP TABLE IF EXISTS card_offers CASCADE;
DROP TABLE IF EXISTS user_cards CASCADE;
DROP TABLE IF EXISTS card_runs CASCADE;
DROP TABLE IF EXISTS card_catalog CASCADE;
`

func TestCardRewardsMigration_AppliesCleanly(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "internal", "db", "migrations", "057_card_rewards.sql"))
	if err != nil {
		t.Fatalf("read card rewards migration: %v", err)
	}

	// Clean slate (the live stack applied 057 at startup; drop to prove the SQL
	// applies from scratch, not merely that it ran once before).
	if _, err := pool.Exec(ctx, cardRewardsDownSQL); err != nil {
		t.Fatalf("drop card_rewards tables before test: %v", err)
	}
	assertCardRewardsTablesAbsent(t, ctx, pool)

	// Apply → all 10 tables + key constraints/indexes present (SCN-083-A05/A06).
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply card rewards migration: %v", err)
	}
	assertCardRewardsTablesPresent(t, ctx, pool)

	// SCN-083-A06 — the lifecycle CHECK actually REJECTS an out-of-range value.
	// These Exec calls live in the main test (pool is the concrete pool type);
	// the queryPool helper interface is QueryRow-only, mirroring the twitter
	// migration test's split.
	seedSQL := `INSERT INTO card_catalog (id, name, issuer, card_type, source)
		VALUES ('test-catalog-card', 'Test Card', 'TestIssuer', 'rotating', 'seed')
		ON CONFLICT (id) DO NOTHING;`
	if _, err := pool.Exec(ctx, seedSQL); err != nil {
		t.Fatalf("seed card_catalog for constraint test: %v", err)
	}
	badInsert := `INSERT INTO rotating_categories
		(id, card_catalog_id, period_label, categories, lifecycle_state, confidence)
		VALUES (gen_random_uuid(), 'test-catalog-card', 'Q3_2026', ARRAY['Restaurants'], 'bogus', 0.9);`
	if _, err := pool.Exec(ctx, badInsert); err == nil {
		t.Fatal("expected lifecycle_state CHECK to reject 'bogus', but insert succeeded")
	}

	assertCardRewardsConstraints(t, ctx, pool)

	// Idempotent: CREATE ... IF NOT EXISTS re-applies without error.
	if _, err := pool.Exec(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("re-apply card rewards migration (idempotency): %v", err)
	}
	assertCardRewardsTablesPresent(t, ctx, pool)
}

func assertCardRewardsTablesPresent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, tbl := range cardRewardsTables {
		if !tableExists(t, ctx, pool, tbl) {
			t.Fatalf("expected table %s to exist after migrate", tbl)
		}
	}
}

func assertCardRewardsTablesAbsent(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()
	for _, tbl := range cardRewardsTables {
		if tableExists(t, ctx, pool, tbl) {
			t.Fatalf("expected table %s to be absent before migrate", tbl)
		}
	}
}

// SCN-083-A06 — lifecycle CHECK, (card_catalog_id, period_label) UNIQUE index,
// and needs_verification NOT NULL default false.
func assertCardRewardsConstraints(t *testing.T, ctx context.Context, pool queryPool) {
	t.Helper()

	// Lifecycle CHECK constraint exists on rotating_categories.
	var checkExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conname = $1 AND contype = 'c')`,
		"rotating_categories_lifecycle_check").Scan(&checkExists); err != nil {
		t.Fatalf("check lifecycle constraint: %v", err)
	}
	if !checkExists {
		t.Fatal("expected CHECK constraint rotating_categories_lifecycle_check to exist")
	}

	// UNIQUE (card_catalog_id, period_label) index present (named idx_rotating_card_period).
	var uniqueExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`,
		"idx_rotating_card_period").Scan(&uniqueExists); err != nil {
		t.Fatalf("check unique index: %v", err)
	}
	if !uniqueExists {
		t.Fatal("expected UNIQUE index idx_rotating_card_period to exist")
	}

	// needs_verification: NOT NULL with default false.
	var isNullable, colDefault string
	if err := pool.QueryRow(ctx,
		`SELECT is_nullable, COALESCE(column_default, '') FROM information_schema.columns
		 WHERE table_name = 'rotating_categories' AND column_name = 'needs_verification'`).
		Scan(&isNullable, &colDefault); err != nil {
		t.Fatalf("inspect needs_verification column: %v", err)
	}
	if isNullable != "NO" {
		t.Errorf("needs_verification is_nullable = %q, want NO", isNullable)
	}
	if colDefault != "false" {
		t.Errorf("needs_verification default = %q, want false", colDefault)
	}

	// A couple of the summary indexes from design §2.
	for _, idx := range []string{"idx_user_cards_active", "idx_observations_card_period", "idx_recommendations_period", "idx_runs_type_time"} {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)`, idx).Scan(&exists); err != nil {
			t.Fatalf("check index %s: %v", idx, err)
		}
		if !exists {
			t.Errorf("expected index %s to exist after migrate", idx)
		}
	}
}
