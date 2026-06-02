//go:build integration

// Package db_integration — TP-076-01-03 (SCN-076-F03).
//
// Integration test: spec 076 SCOPE-1 foundation migration
// `053_assistant_tool_traces.sql` applies cleanly against the live
// test-stack Postgres without disturbing the shipped
// `artifact_capture_policy` CHECK constraint on `provenance` or the
// partial UNIQUE index `idx_capture_fallback_dedup` (migration 051).
//
// Requires DATABASE_URL pointing at the disposable test stack. The
// test boot path is db.Migrate, which is idempotent — running it
// after the test stack has already applied everything is a no-op
// against schema_migrations; the assertions below probe pg_catalog
// directly.
package db_integration

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/db"
)

func TestSpec076FoundationMigrationsApplyCleanly(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	// Idempotent re-run — must not error.
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate (idempotent re-run): %v", err)
	}

	// 1) assistant_tool_traces exists with the four required NOT NULL
	//    columns + CHECK on lifecycle_state.
	requiredCols := map[string]string{
		"turn_id":          "text",
		"tool_name":        "text",
		"payload_redacted": "jsonb",
		"lifecycle_state":  "text",
		"created_at":       "timestamp with time zone",
	}
	for col, wantType := range requiredCols {
		var (
			dataType   string
			isNullable string
		)
		err := pool.QueryRow(ctx, `
			SELECT data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'assistant_tool_traces'
			  AND column_name  = $1`, col).Scan(&dataType, &isNullable)
		if err != nil {
			t.Fatalf("query assistant_tool_traces.%s: %v", col, err)
		}
		if dataType != wantType {
			t.Errorf("assistant_tool_traces.%s: data_type=%q want %q", col, dataType, wantType)
		}
		if isNullable != "NO" {
			t.Errorf("assistant_tool_traces.%s: is_nullable=%q want %q", col, isNullable, "NO")
		}
	}

	// 2) lifecycle_state CHECK exists and contains the canonical tokens.
	var checkSrc string
	err = pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = 'assistant_tool_traces' AND c.contype = 'c'
		  AND pg_get_constraintdef(c.oid) ILIKE '%lifecycle_state%'
		LIMIT 1`).Scan(&checkSrc)
	if err != nil {
		t.Fatalf("query assistant_tool_traces lifecycle_state CHECK: %v", err)
	}
	for _, want := range []string{"active", "cooling", "pruned"} {
		if !strings.Contains(checkSrc, want) {
			t.Errorf("assistant_tool_traces lifecycle_state CHECK missing %q in: %s", want, checkSrc)
		}
	}

	// 3) Spec 074 invariants preserved — provenance CHECK still names
	//    capture-as-fallback and capture-explicit (closed vocabulary).
	var provCheck string
	err = pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = 'artifact_capture_policy' AND c.contype = 'c'
		  AND pg_get_constraintdef(c.oid) ILIKE '%provenance%'
		LIMIT 1`).Scan(&provCheck)
	if err != nil {
		t.Fatalf("query artifact_capture_policy provenance CHECK: %v", err)
	}
	for _, want := range []string{"capture-as-fallback", "capture-explicit"} {
		if !strings.Contains(provCheck, want) {
			t.Errorf("artifact_capture_policy provenance CHECK missing %q in: %s", want, provCheck)
		}
	}

	// 4) Spec 074 partial UNIQUE index idx_capture_fallback_dedup
	//    still exists with WHERE clause scoped to capture-as-fallback.
	var idxDef string
	err = pool.QueryRow(ctx, `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = 'public' AND indexname = 'idx_capture_fallback_dedup'`).Scan(&idxDef)
	if err != nil {
		t.Fatalf("query idx_capture_fallback_dedup definition: %v", err)
	}
	for _, want := range []string{"UNIQUE", "capture-as-fallback", "normalized_text_hash", "dedup_bucket_start"} {
		if !strings.Contains(idxDef, want) {
			t.Errorf("idx_capture_fallback_dedup definition missing %q in: %s", want, idxDef)
		}
	}
}

