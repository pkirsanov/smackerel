//go:build integration

// Spec 038 Scope 1 DoD-3 — Drive schema migrations apply cleanly on the
// disposable test database and preserve the existing artifact-identity
// boundary. The test runs against the live test stack brought up by
// ./smackerel.sh test integration: migrations have already been applied by
// the core service on startup. This test asserts:
//
//  1. Every drive_* table from migration 021_drive_schema.sql exists in the
//     test database with its expected columns.
//  2. The pre-existing artifacts table still has its prior columns
//     (migration 021 made no destructive changes to the artifacts table —
//     drive sensitivity lives on drive_files only per design.md §8.1 / F1).
//  3. Creating an artifact + linking a drive_files row, then deleting the
//     drive_files row, leaves the artifact intact (artifact identity
//     boundary preserved — the drive table holds provider identity, the
//     artifacts table holds canonical content identity).
//
// Naming: package drive (separate from package integration) keeps the new
// drive-specific helpers isolated from the general integration suite while
// running under the same DATABASE_URL the orchestrator wires for both.
package drive

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// driveTestPool returns a pgxpool connected to the live test database, or
// skips the test if DATABASE_URL is not set. Mirrors the pattern in
// tests/integration/helpers_test.go testPool but lives in this package so
// the drive integration suite is self-contained.
func driveTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func tableExists(t *testing.T, pool *pgxpool.Pool, ctx context.Context, table string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
		table,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check table %s: %v", table, err)
	}
	return exists
}

func columnExists(t *testing.T, pool *pgxpool.Pool, ctx context.Context, table, column string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2)",
		table, column,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	return exists
}

// TestDriveMigration021_TablesAndColumnsExist asserts that every drive_*
// table from migration 021 exists with its expected columns.
func TestDriveMigration021_TablesAndColumnsExist(t *testing.T) {
	pool := driveTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	expectedTables := []string{
		"drive_connections",
		"drive_files",
		"drive_folders",
		"drive_cursors",
		"drive_rules",
		"drive_save_requests",
		"drive_folder_resolutions",
		"drive_rule_audit",
	}
	for _, table := range expectedTables {
		if !tableExists(t, pool, ctx, table) {
			t.Errorf("expected table %q to exist after migration 021", table)
		}
	}

	// Spot-check the structurally important columns of each drive_*
	// table. The migration file is authoritative; this test pins the
	// shape so accidental column drops are caught.
	expectedColumns := map[string][]string{
		"drive_connections": {
			"id", "provider_id", "owner_user_id", "account_label",
			"access_mode", "status", "scope", "credentials_ref",
			"created_at", "updated_at",
		},
		"drive_files": {
			"id", "artifact_id", "connection_id", "provider_file_id",
			"provider_revision_id", "provider_url", "title", "mime_type",
			"size_bytes", "folder_path", "sharing_state", "sensitivity",
			"extraction_state", "version_chain",
		},
		"drive_folders": {
			"id", "connection_id", "provider_folder_id", "folder_path",
			"folder_summary", "summarized_at",
		},
		"drive_cursors": {
			"connection_id", "cursor", "valid_until",
			"last_rescan_started_at", "last_rescan_completed_at",
		},
		"drive_rules": {
			"id", "name", "enabled", "source_kinds", "classification",
			"sensitivity_in", "confidence_min", "provider_id",
			"target_folder_template", "on_missing_folder",
			"on_existing_file", "guardrails",
		},
		"drive_save_requests": {
			"id", "rule_id", "source_artifact_id", "target_path",
			"idempotency_key", "status", "attempts", "last_error",
		},
		"drive_folder_resolutions": {
			"id", "connection_id", "provider_id", "folder_path",
			"provider_folder_id", "created_by_request_id",
		},
		"drive_rule_audit": {
			"id", "rule_id", "source_artifact_id", "outcome", "reason",
		},
	}
	for table, cols := range expectedColumns {
		for _, col := range cols {
			if !columnExists(t, pool, ctx, table, col) {
				t.Errorf("expected column %s.%q to exist after migration 021", table, col)
			}
		}
	}
}

// TestDriveMigration021_ArtifactsTablePreservedColumns asserts that
// migration 021 made no destructive changes to the canonical artifacts
// table. The columns checked here are the cross-feature contract that
// downstream features (search, knowledge, domain extraction, lists, etc.)
// rely on. F1 in design.md §8.1 explicitly declares that drive sensitivity
// lives on drive_files only, NOT on artifacts.
func TestDriveMigration021_ArtifactsTablePreservedColumns(t *testing.T) {
	pool := driveTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	preserved := []string{
		"id", "artifact_type", "title", "summary", "content_raw",
		"content_hash", "key_ideas", "entities", "action_items",
		"topics", "sentiment", "source_id", "source_ref", "source_url",
		"embedding", "created_at", "updated_at",
		"processing_status",
		"domain_data", "domain_extraction_status", "domain_schema_version", "domain_extracted_at",
	}
	for _, col := range preserved {
		if !columnExists(t, pool, ctx, "artifacts", col) {
			t.Errorf("artifacts.%q is missing — migration 021 must not drop pre-existing artifacts columns", col)
		}
	}

	// Negative assertion — migration 021 must NOT have added a
	// sensitivity column to artifacts (F1 contract: drive sensitivity
	// lives on drive_files only).
	if columnExists(t, pool, ctx, "artifacts", "sensitivity") {
		t.Error("artifacts.sensitivity exists — F1 contract requires drive sensitivity on drive_files only")
	}
}

// TestDriveMigration021_ArtifactIdentityBoundaryPreserved asserts the F1
// boundary at runtime: the drive_files row holds provider identity; the
// artifacts row holds canonical content identity. Deleting the drive_files
// row MUST leave the artifact intact, so downstream features that already
// reference the artifact (search, knowledge, lists) continue to work even
// when the drive linkage is removed.
//
// This is the inverse of the ON DELETE CASCADE direction (deleting an
// artifact cascades to drive_files, but deleting drive_files does not
// touch artifacts). That direction is the contract Spec 038 Scope 1
// requires.
func TestDriveMigration021_ArtifactIdentityBoundaryPreserved(t *testing.T) {
	pool := driveTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	artifactID := "drive-mig-test-art-" + uuid.NewString()
	connID := uuid.New()
	driveFileID := uuid.New()
	ownerID := uuid.New()
	uniqueLabel := "drive-mig-test-" + uuid.NewString()

	// Cleanup is registered before insert so failures during the body
	// still tear the test fixtures down.
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		// Order matters: drive_files FKs to artifacts and to drive_connections.
		_, _ = pool.Exec(cctx, "DELETE FROM drive_files WHERE id = $1", driveFileID)
		_, _ = pool.Exec(cctx, "DELETE FROM drive_connections WHERE id = $1", connID)
		_, _ = pool.Exec(cctx, "DELETE FROM artifacts WHERE id = $1", artifactID)
	})

	// 1) Insert artifact.
	_, err := pool.Exec(ctx, `
        INSERT INTO artifacts (id, artifact_type, title, content_raw, content_hash, source_id)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, artifactID, "document", "drive boundary test", "raw content", "hash-"+uuid.NewString(), "drive-mig-test")
	if err != nil {
		t.Fatalf("insert artifact: %v", err)
	}

	// 2) Insert drive_connection.
	_, err = pool.Exec(ctx, `
        INSERT INTO drive_connections (id, provider_id, owner_user_id, account_label, access_mode, status)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, connID, "google", ownerID, uniqueLabel, "read_only", "healthy")
	if err != nil {
		t.Fatalf("insert drive_connections: %v", err)
	}

	// 3) Link drive_files row.
	_, err = pool.Exec(ctx, `
        INSERT INTO drive_files (
            id, artifact_id, connection_id, provider_file_id, provider_url,
            title, mime_type, size_bytes, sensitivity, extraction_state
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `, driveFileID, artifactID, connID, "g-file-001", "https://drive.google.com/file/d/g-file-001",
		"boundary.pdf", "application/pdf", int64(2048), "none", "pending")
	if err != nil {
		t.Fatalf("insert drive_files: %v", err)
	}

	// 4) Delete the drive_files row.
	tag, err := pool.Exec(ctx, "DELETE FROM drive_files WHERE id = $1", driveFileID)
	if err != nil {
		t.Fatalf("delete drive_files: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("expected 1 drive_files row deleted, got %d", tag.RowsAffected())
	}

	// 5) Adversarial assertion — artifact MUST still exist with its
	// canonical content. If a future migration added ON DELETE CASCADE
	// from drive_files -> artifacts (the wrong direction), this would
	// fail and catch the regression.
	var got string
	err = pool.QueryRow(ctx, "SELECT title FROM artifacts WHERE id = $1", artifactID).Scan(&got)
	if err != nil {
		t.Fatalf("artifact lookup after drive_files delete: %v (artifact identity boundary violated)", err)
	}
	if got != "drive boundary test" {
		t.Errorf("artifact title after drive_files delete = %q, want %q", got, "drive boundary test")
	}
}

// TestDriveMigration023_ExpiresAtAndOAuthStatesApplied asserts that the
// additive Round 6 migration 023 is present on the live test database:
//
//  1. drive_connections.expires_at TIMESTAMPTZ NULL exists (captures
//     OAuth access-token expiry returned by FinalizeConnect).
//  2. drive_oauth_states table exists with the bound-redirect columns
//     declared in design.md §3.4 (state_token PK, owner_user_id,
//     provider_id, access_mode, scope JSONB, created_at, expires_at).
//  3. Adversarial: a column not present in migration 023
//     (drive_oauth_states.refresh_token) MUST NOT exist — the migration
//     must not silently introduce extra fields.
//
// This test maps to design.md decision A1 (additive expires_at column)
// and the new drive_oauth_states table that backs the BeginConnect/
// FinalizeConnect split (decision B1). The migration is numbered 023
// rather than 022 because spec 039 already owns 022_recommendations.sql.
func TestDriveMigration023_ExpiresAtAndOAuthStatesApplied(t *testing.T) {
	pool := driveTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if !columnExists(t, pool, ctx, "drive_connections", "expires_at") {
		t.Error("drive_connections.expires_at is missing — migration 023 did not apply")
	}

	if !tableExists(t, pool, ctx, "drive_oauth_states") {
		t.Fatal("drive_oauth_states table is missing — migration 023 did not apply")
	}

	expectedCols := []string{
		"state_token",
		"owner_user_id",
		"provider_id",
		"access_mode",
		"scope",
		"created_at",
		"expires_at",
	}
	for _, col := range expectedCols {
		if !columnExists(t, pool, ctx, "drive_oauth_states", col) {
			t.Errorf("drive_oauth_states.%q is missing — migration 023 must declare it", col)
		}
	}

	// Adversarial: a column not in the migration MUST NOT exist. If a
	// future migration silently adds refresh_token to this table without
	// updating callers, this assertion forces an explicit migration +
	// test update.
	if columnExists(t, pool, ctx, "drive_oauth_states", "refresh_token") {
		t.Error("drive_oauth_states.refresh_token unexpectedly exists — migration 023 must not declare it")
	}
}
