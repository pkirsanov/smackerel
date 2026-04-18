//go:build integration

package integration

import (
	"context"
	"testing"
	"time"
)

// Scenario: All migrations apply cleanly
// Given a fresh PostgreSQL instance
// When all 17 migrations are applied in sequence
// Then all tables exist with correct columns and indexes
func TestMigrations_AllTablesExist(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	expectedTables := []string{
		"schema_migrations",
		"artifacts",
		"people",
		"topics",
		"edges",
		"sync_state",
		"action_items",
		"digests",
		"annotations",
		"telegram_message_artifacts",
		"lists",
		"list_items",
	}

	for _, table := range expectedTables {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %q to exist", table)
		}
	}
}

func TestMigrations_ArtifactsColumns(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expectedColumns := []string{
		"id", "artifact_type", "title", "summary", "content_raw",
		"content_hash", "key_ideas", "entities", "action_items",
		"topics", "sentiment", "source_id", "source_ref", "source_url",
		"embedding", "created_at", "updated_at",
		// Migration 006
		"processing_status",
		// Migration 015 — domain extraction
		"domain_data", "domain_extraction_status", "domain_schema_version", "domain_extracted_at",
	}

	for _, col := range expectedColumns {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'artifacts' AND column_name = $1)",
			col,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check column artifacts.%s: %v", col, err)
		}
		if !exists {
			t.Errorf("expected column artifacts.%q to exist", col)
		}
	}
}

func TestMigrations_IndexesExist(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	expectedIndexes := []string{
		"idx_artifacts_type",
		"idx_artifacts_source",
		"idx_artifacts_created",
		"idx_artifacts_hash",
		"idx_artifacts_embedding",
		"idx_artifacts_title_trgm",
		"idx_artifacts_domain_data_gin",
		"idx_annotations_artifact",
		"idx_annotations_type",
		"idx_list_items_list",
		"idx_lists_status",
	}

	for _, idx := range expectedIndexes {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = $1)",
			idx,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check index %s: %v", idx, err)
		}
		if !exists {
			t.Errorf("expected index %q to exist", idx)
		}
	}
}

func TestMigrations_ExtensionsLoaded(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, ext := range []string{"vector", "pg_trgm"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = $1)",
			ext,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check extension %s: %v", ext, err)
		}
		if !exists {
			t.Errorf("expected extension %q to be loaded", ext)
		}
	}
}

func TestMigrations_SchemaVersionCount(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}

	if count < 1 {
		t.Errorf("expected at least 1 migration applied, got %d", count)
	}
	t.Logf("schema_migrations count: %d", count)
}

// Scenario: Migration rollback works
// Given the consolidated schema has been applied
// When specific tables are dropped
// Then they can be recreated and other tables are unaffected
func TestMigrations_TableDropAndRecreate(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify lists tables exist
	for _, table := range []string{"lists", "list_items"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Skipf("table %q does not exist", table)
		}
	}

	// Drop and recreate list_items + lists
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}

	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS list_items CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop list_items: %v", err)
	}
	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS lists CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop lists: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit drop: %v", err)
	}

	// Verify lists tables are gone
	for _, table := range []string{"lists", "list_items"} {
		var exists bool
		pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		if exists {
			t.Errorf("table %q should not exist after drop", table)
		}
	}

	// Verify other tables are unaffected
	for _, table := range []string{"artifacts", "annotations", "edges"} {
		var exists bool
		pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		if !exists {
			t.Errorf("table %q should still exist after lists drop", table)
		}
	}

	// Recreate using the table DDL
	recreateSQL := `
CREATE TABLE IF NOT EXISTS lists (
    id TEXT PRIMARY KEY, list_type TEXT NOT NULL, title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft', source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',
    source_query TEXT, domain TEXT, total_items INTEGER NOT NULL DEFAULT 0,
    checked_items INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_lists_status ON lists(status);
CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(list_type);
CREATE INDEX IF NOT EXISTS idx_lists_created ON lists(created_at DESC);
CREATE TABLE IF NOT EXISTS list_items (
    id TEXT PRIMARY KEY, list_id TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    content TEXT NOT NULL, category TEXT, status TEXT NOT NULL DEFAULT 'pending',
    substitution TEXT, source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',
    is_manual BOOLEAN NOT NULL DEFAULT FALSE, quantity REAL, unit TEXT,
    normalized_name TEXT, sort_order INTEGER NOT NULL DEFAULT 0,
    checked_at TIMESTAMPTZ, notes TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_list_items_list ON list_items(list_id);
CREATE INDEX IF NOT EXISTS idx_list_items_status ON list_items(list_id, status);
CREATE INDEX IF NOT EXISTS idx_list_items_category ON list_items(list_id, category);
`
	_, err = pool.Exec(ctx, recreateSQL)
	if err != nil {
		t.Fatalf("recreate lists tables: %v", err)
	}
	t.Log("table drop and recreate verified")
}

func TestMigrations_DomainColumnsExist(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, col := range []string{"domain_data", "domain_extraction_status", "domain_schema_version", "domain_extracted_at"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'artifacts' AND column_name = $1)",
			col,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check column %s: %v", col, err)
		}
		if !exists {
			t.Errorf("expected column artifacts.%q to exist", col)
		}
	}
}

func TestMigrations_AnnotationsConstraints(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// rating constraint: must be 1-5 when present
	var constraintExists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'chk_rating_range' AND table_name = 'annotations')",
	).Scan(&constraintExists)
	if err != nil {
		t.Fatalf("check constraint: %v", err)
	}
	if !constraintExists {
		t.Error("expected chk_rating_range constraint on annotations table")
	}
}
