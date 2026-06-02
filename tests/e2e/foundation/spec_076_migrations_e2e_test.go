//go:build e2e

// Package foundation_e2e — TP-076-01-03R (SCN-076-F03 regression E2E).
//
// Adversarial regression: spec 076 SCOPE-1 migration
// `053_assistant_tool_traces.sql` MUST survive a fresh stack apply
// without disturbing migration 051's `artifact_capture_policy`
// invariants (provenance closed vocabulary + partial UNIQUE dedup
// index).
//
// Adversarial mechanism: the test creates a disposable schema
// (`spec076_f03_<nanos>`), points search_path at it, runs the full
// `db.Migrate` pipeline against that schema, then asserts every
// invariant. If migration 053 ever drops/alters the table, or if 051
// ever changes its provenance CHECK or dedup index shape, this test
// fails immediately on a truly fresh stack — not on an accumulated
// dev DB where the constraints might still exist from earlier runs.
package foundation_e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/db"
)

func TestSpec076MigrationsSurviveFreshStack(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	schema := fmt.Sprintf("spec076_f03_%d", time.Now().UnixNano())
	// Append an options parameter to redirect search_path to a fresh
	// disposable schema. The parent pool stays default; the per-test
	// pool gets a dedicated schema so the assertions never touch
	// production rows.
	sep := "?"
	if strings.Contains(dbURL, "?") {
		sep = "&"
	}
	scopedURL := dbURL + sep + "options=-c%20search_path=" + schema + ",public"

	admin, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect admin pool: %v", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create disposable schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		if _, err := admin.Exec(dropCtx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Logf("cleanup: drop schema %s: %v", schema, err)
		}
	})

	pool, err := pgxpool.New(ctx, scopedURL)
	if err != nil {
		t.Fatalf("connect scoped pool: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("db.Migrate against fresh schema %s: %v", schema, err)
	}

	// Spec 076 — assistant_tool_traces exists with all NOT NULL cols.
	requiredCols := []string{"turn_id", "tool_name", "payload_redacted", "lifecycle_state", "created_at"}
	for _, col := range requiredCols {
		var isNullable string
		err := pool.QueryRow(ctx, `
			SELECT is_nullable
			FROM information_schema.columns
			WHERE table_schema = $1
			  AND table_name   = 'assistant_tool_traces'
			  AND column_name  = $2`, schema, col).Scan(&isNullable)
		if err != nil {
			t.Fatalf("query %s.assistant_tool_traces.%s: %v", schema, col, err)
		}
		if isNullable != "NO" {
			t.Errorf("assistant_tool_traces.%s NOT NULL invariant violated: is_nullable=%q", col, isNullable)
		}
	}

	// Spec 074 invariants — provenance CHECK and partial UNIQUE dedup
	// index survive a fresh apply unchanged.
	var provCheck string
	err = pool.QueryRow(ctx, `
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE n.nspname = $1
		  AND t.relname = 'artifact_capture_policy'
		  AND c.contype = 'c'
		  AND pg_get_constraintdef(c.oid) ILIKE '%provenance%'
		LIMIT 1`, schema).Scan(&provCheck)
	if err != nil {
		t.Fatalf("query artifact_capture_policy provenance CHECK in %s: %v", schema, err)
	}
	for _, want := range []string{"capture-as-fallback", "capture-explicit"} {
		if !strings.Contains(provCheck, want) {
			t.Errorf("provenance CHECK missing %q in %s.artifact_capture_policy: %s", want, schema, provCheck)
		}
	}

	var idxDef string
	err = pool.QueryRow(ctx, `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = $1 AND indexname = 'idx_capture_fallback_dedup'`, schema).Scan(&idxDef)
	if err != nil {
		t.Fatalf("query idx_capture_fallback_dedup in %s: %v", schema, err)
	}
	for _, want := range []string{"UNIQUE", "capture-as-fallback", "normalized_text_hash", "dedup_bucket_start"} {
		if !strings.Contains(idxDef, want) {
			t.Errorf("idx_capture_fallback_dedup missing %q in %s: %s", want, schema, idxDef)
		}
	}
}
