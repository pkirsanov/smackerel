package db

import (
	"context"
	"testing"
)

func TestMigrationsEmbed(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("failed to read embedded migrations: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no migration files found in embed")
	}

	found := false
	for _, e := range entries {
		if e.Name() == "001_initial_schema.sql" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 001_initial_schema.sql in embedded migrations")
	}
}

func TestMigrationSQL_Parseable(t *testing.T) {
	content, err := migrationFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		t.Fatalf("failed to read migration: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("migration file is empty")
	}

	sql := string(content)
	requiredTables := []string{
		"CREATE TABLE IF NOT EXISTS artifacts",
		"CREATE TABLE IF NOT EXISTS people",
		"CREATE TABLE IF NOT EXISTS topics",
		"CREATE TABLE IF NOT EXISTS edges",
		"CREATE TABLE IF NOT EXISTS sync_state",
		"CREATE TABLE IF NOT EXISTS action_items",
		"CREATE TABLE IF NOT EXISTS digests",
	}
	for _, table := range requiredTables {
		if !contains(sql, table) {
			t.Errorf("migration missing: %s", table)
		}
	}
}

func TestMigrationSQL_Extensions(t *testing.T) {
	content, err := migrationFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		t.Fatalf("failed to read migration: %v", err)
	}
	sql := string(content)
	if !contains(sql, "CREATE EXTENSION IF NOT EXISTS vector") {
		t.Error("migration missing pgvector extension")
	}
	if !contains(sql, "CREATE EXTENSION IF NOT EXISTS pg_trgm") {
		t.Error("migration missing pg_trgm extension")
	}
}

func TestMigrationSQL_Indexes(t *testing.T) {
	content, err := migrationFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		t.Fatalf("failed to read migration: %v", err)
	}
	sql := string(content)
	requiredIndexes := []string{
		"idx_artifacts_type",
		"idx_artifacts_source",
		"idx_artifacts_created",
		"idx_artifacts_relevance",
		"idx_artifacts_hash",
		"idx_artifacts_embedding",
		"idx_artifacts_title_trgm",
		"idx_edges_src",
		"idx_edges_dst",
		"idx_edges_type",
		"idx_action_items_status",
	}
	for _, idx := range requiredIndexes {
		if !contains(sql, idx) {
			t.Errorf("migration missing index: %s", idx)
		}
	}
}

// contains is a simple substring check.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure Migrate function signature is correct (compile-time check).
var _ = func() {
	_ = Migrate(context.Background(), nil)
}
