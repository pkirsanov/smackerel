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

func TestFormatEmbedding_Empty(t *testing.T) {
	result := FormatEmbedding(nil)
	if result != "" {
		t.Errorf("expected empty string for nil, got %q", result)
	}
	result = FormatEmbedding([]float32{})
	if result != "" {
		t.Errorf("expected empty string for empty slice, got %q", result)
	}
}

func TestFormatEmbedding_SingleElement(t *testing.T) {
	result := FormatEmbedding([]float32{1.5})
	if !contains(result, "[") || !contains(result, "]") {
		t.Errorf("expected brackets, got %q", result)
	}
	if !contains(result, "1.5") {
		t.Errorf("expected 1.5 in output, got %q", result)
	}
}

func TestFormatEmbedding_MultipleElements(t *testing.T) {
	result := FormatEmbedding([]float32{0.1, 0.2, 0.3})
	if result[0] != '[' || result[len(result)-1] != ']' {
		t.Errorf("expected brackets wrapping, got %q", result)
	}
	// Should contain commas separating values
	parts := result[1 : len(result)-1] // strip brackets
	if len(parts) == 0 {
		t.Fatal("empty content inside brackets")
	}
	commaCount := 0
	for _, ch := range parts {
		if ch == ',' {
			commaCount++
		}
	}
	if commaCount != 2 {
		t.Errorf("expected 2 commas for 3 elements, got %d", commaCount)
	}
}

func TestFormatEmbedding_ZeroValues(t *testing.T) {
	result := FormatEmbedding([]float32{0.0, 0.0})
	if !contains(result, "0.") {
		t.Errorf("expected zero values, got %q", result)
	}
}

func TestFormatEmbedding_NegativeValues(t *testing.T) {
	result := FormatEmbedding([]float32{-1.5, 2.5})
	if !contains(result, "-1.5") {
		t.Errorf("expected negative value, got %q", result)
	}
}

func TestMigrationFiles_SortOrder(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Name() < entries[i-1].Name() {
			t.Errorf("migration files not sorted: %s before %s", entries[i-1].Name(), entries[i].Name())
		}
	}
}

func TestMigrationFiles_SQLNotEmpty(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			t.Errorf("failed to read %s: %v", entry.Name(), err)
			continue
		}
		if len(content) == 0 {
			t.Errorf("migration %s is empty", entry.Name())
		}
	}
}

func TestMigrationSQL_Constraints(t *testing.T) {
	content, err := migrationFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)

	// Should have primary key constraints
	if !contains(sql, "PRIMARY KEY") && !contains(sql, "primary key") {
		t.Error("migration missing PRIMARY KEY constraints")
	}

	// Should have NOT NULL on critical columns
	if !contains(sql, "NOT NULL") {
		t.Error("migration missing NOT NULL constraints")
	}
}

func TestMigrationSQL_VectorType(t *testing.T) {
	content, err := migrationFS.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(content)
	if !contains(sql, "vector") {
		t.Error("migration missing vector column type")
	}
}
