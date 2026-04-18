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

	if count < 17 {
		t.Errorf("expected at least 17 migrations applied, got %d", count)
	}
	t.Logf("schema_migrations count: %d", count)
}

// Scenario: Migration rollback works
// Given migration 017 has been applied
// When the rollback SQL is executed
// Then the list_items and lists tables are dropped
// And other tables are unaffected
func TestMigrations_Rollback017(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify lists tables exist before rollback
	for _, table := range []string{"lists", "list_items"} {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s before rollback: %v", table, err)
		}
		if !exists {
			t.Skipf("table %q does not exist — migration 017 not applied", table)
		}
	}

	// Execute rollback for 017
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}

	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS list_items CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("rollback list_items: %v", err)
	}
	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS lists CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("rollback lists: %v", err)
	}
	_, err = tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = '017_actionable_lists.sql'")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("remove migration record: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit rollback: %v", err)
	}

	// Verify lists tables are gone
	for _, table := range []string{"lists", "list_items"} {
		var exists bool
		pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		if exists {
			t.Errorf("table %q should not exist after rollback", table)
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
			t.Errorf("table %q should still exist after 017 rollback", table)
		}
	}

	// Re-apply migration 017 to restore state for subsequent tests
	migration017 := `
CREATE TABLE IF NOT EXISTS lists (
    id                  TEXT PRIMARY KEY,
    list_type           TEXT NOT NULL,
    title               TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft',
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',
    source_query        TEXT,
    domain              TEXT,
    total_items         INTEGER NOT NULL DEFAULT 0,
    checked_items       INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_lists_status ON lists(status);
CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(list_type);
CREATE INDEX IF NOT EXISTS idx_lists_created ON lists(created_at DESC);
CREATE TABLE IF NOT EXISTS list_items (
    id                  TEXT PRIMARY KEY,
    list_id             TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    content             TEXT NOT NULL,
    category            TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    substitution        TEXT,
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',
    is_manual           BOOLEAN NOT NULL DEFAULT FALSE,
    quantity            REAL,
    unit                TEXT,
    normalized_name     TEXT,
    sort_order          INTEGER NOT NULL DEFAULT 0,
    checked_at          TIMESTAMPTZ,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_list_items_list ON list_items(list_id);
CREATE INDEX IF NOT EXISTS idx_list_items_status ON list_items(list_id, status);
CREATE INDEX IF NOT EXISTS idx_list_items_category ON list_items(list_id, category);
INSERT INTO schema_migrations (version) VALUES ('017_actionable_lists.sql');
`
	_, err = pool.Exec(ctx, migration017)
	if err != nil {
		t.Fatalf("re-apply migration 017: %v", err)
	}
	t.Log("migration 017 rollback and re-apply verified")
}

func TestMigrations_Rollback016(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Verify annotations table exists
	var exists bool
	pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'annotations')",
	).Scan(&exists)
	if !exists {
		t.Skip("annotations table does not exist — migration 016 not applied")
	}

	// Execute rollback for 016
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}

	// Must drop 017 first due to FK dependencies
	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS list_items CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop list_items for 016 rollback: %v", err)
	}
	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS lists CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop lists for 016 rollback: %v", err)
	}
	_, err = tx.Exec(ctx, "DROP MATERIALIZED VIEW IF EXISTS artifact_annotation_summary")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop mat view: %v", err)
	}
	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS telegram_message_artifacts CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop telegram_message_artifacts: %v", err)
	}
	_, err = tx.Exec(ctx, "DROP TABLE IF EXISTS annotations CASCADE")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("drop annotations: %v", err)
	}
	_, err = tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version IN ('016_user_annotations.sql', '017_actionable_lists.sql')")
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("remove migration records: %v", err)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit rollback: %v", err)
	}

	// Verify annotations gone, artifacts still present
	pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'annotations')",
	).Scan(&exists)
	if exists {
		t.Error("annotations table should not exist after 016 rollback")
	}

	pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'artifacts')",
	).Scan(&exists)
	if !exists {
		t.Error("artifacts table should still exist after 016 rollback")
	}

	// Re-apply 016 and 017 to restore state
	// Use the internal migrate package to re-apply properly
	reapplySQL := `
CREATE TABLE IF NOT EXISTS annotations (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    annotation_type TEXT NOT NULL,
    rating          INTEGER,
    note            TEXT,
    tag             TEXT,
    interaction_type TEXT,
    source_channel  TEXT NOT NULL DEFAULT 'api',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_annotations_artifact ON annotations(artifact_id);
CREATE INDEX IF NOT EXISTS idx_annotations_type ON annotations(annotation_type);
CREATE INDEX IF NOT EXISTS idx_annotations_created ON annotations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_annotations_tag ON annotations(tag) WHERE tag IS NOT NULL;
ALTER TABLE annotations ADD CONSTRAINT chk_rating_range CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5));
CREATE TABLE IF NOT EXISTS telegram_message_artifacts (
    message_id  BIGINT NOT NULL,
    chat_id     BIGINT NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, chat_id)
);
CREATE MATERIALIZED VIEW IF NOT EXISTS artifact_annotation_summary AS
SELECT a.artifact_id,
    (SELECT rating FROM annotations WHERE artifact_id = a.artifact_id AND annotation_type = 'rating' ORDER BY created_at DESC LIMIT 1) AS current_rating,
    AVG(CASE WHEN a2.annotation_type = 'rating' THEN a2.rating END)::REAL AS average_rating,
    COUNT(CASE WHEN a2.annotation_type = 'rating' THEN 1 END)::INTEGER AS rating_count,
    COUNT(CASE WHEN a2.annotation_type = 'interaction' THEN 1 END)::INTEGER AS times_used,
    MAX(CASE WHEN a2.annotation_type = 'interaction' THEN a2.created_at END) AS last_used,
    ARRAY_AGG(DISTINCT a2.tag) FILTER (WHERE a2.annotation_type = 'tag_add' AND a2.tag IS NOT NULL) AS tags,
    COUNT(CASE WHEN a2.annotation_type = 'note' THEN 1 END)::INTEGER AS notes_count
FROM (SELECT DISTINCT artifact_id FROM annotations) a
LEFT JOIN annotations a2 ON a2.artifact_id = a.artifact_id
GROUP BY a.artifact_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_annotation_summary_artifact ON artifact_annotation_summary(artifact_id);
INSERT INTO schema_migrations (version) VALUES ('016_user_annotations.sql');
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
INSERT INTO schema_migrations (version) VALUES ('017_actionable_lists.sql');
`
	_, err = pool.Exec(ctx, reapplySQL)
	if err != nil {
		t.Fatalf("re-apply migrations 016+017: %v", err)
	}
	t.Log("migration 016 rollback and re-apply verified")
}

func TestMigrations_Rollback015(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check domain_data column exists
	var exists bool
	pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'artifacts' AND column_name = 'domain_data')",
	).Scan(&exists)
	if !exists {
		t.Skip("domain_data column does not exist — migration 015 not applied")
	}

	// Execute rollback for 015 (only drops columns, no table drops)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rollback: %v", err)
	}

	rollback015 := `
DROP INDEX IF EXISTS idx_artifacts_domain_data_gin;
DROP INDEX IF EXISTS idx_artifacts_domain_extraction_status;
DROP INDEX IF EXISTS idx_artifacts_domain_schema_version;
ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_data;
ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_extraction_status;
ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_schema_version;
ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_extracted_at;
DELETE FROM schema_migrations WHERE version = '015_domain_extraction.sql';
`
	_, err = tx.Exec(ctx, rollback015)
	if err != nil {
		tx.Rollback(ctx)
		t.Fatalf("execute 015 rollback: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit 015 rollback: %v", err)
	}

	// Verify column gone
	pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name = 'artifacts' AND column_name = 'domain_data')",
	).Scan(&exists)
	if exists {
		t.Error("domain_data column should not exist after 015 rollback")
	}

	// Verify artifacts table still exists
	pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'artifacts')",
	).Scan(&exists)
	if !exists {
		t.Error("artifacts table should still exist after 015 rollback")
	}

	// Re-apply 015
	reapply015 := `
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_data JSONB;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_extraction_status TEXT;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_schema_version TEXT;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_extracted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_data_gin ON artifacts USING gin (domain_data jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_extraction_status ON artifacts (domain_extraction_status) WHERE domain_extraction_status IN ('pending', 'failed');
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_schema_version ON artifacts (domain_schema_version) WHERE domain_schema_version IS NOT NULL;
INSERT INTO schema_migrations (version) VALUES ('015_domain_extraction.sql');
`
	_, err = pool.Exec(ctx, reapply015)
	if err != nil {
		t.Fatalf("re-apply 015: %v", err)
	}
	t.Log("migration 015 rollback and re-apply verified")
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
