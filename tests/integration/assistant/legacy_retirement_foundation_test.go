//go:build integration

// Spec 075 SCOPE-1 — TP-075-02 / SCN-075-A10.
//
// Live-stack integration proof that the migration-046 contract holds:
//
//   1. The legacy_retirement_notices JSONB column exists on
//      assistant_conversations, is NOT NULL, and carries NO default
//      value. Design (§"Configuration And Migrations") says
//      "Implementation must explicitly populate
//      `legacy_retirement_notices` for existing rows during migration
//      or startup migration; it must not rely on a runtime fallback
//      value." A regression that adds DEFAULT '{}'::jsonb would be a
//      silent fallback — this test trips when that happens.
//
//   2. An INSERT into assistant_conversations that omits the new
//      column fails loud with a NOT NULL violation. This is the
//      adversarial assertion: if the migration silently re-defaulted
//      the column, the INSERT would succeed and the regression would
//      be invisible.
//
//   3. assistant_legacy_retirement_state and
//      assistant_legacy_retirement_observations exist with the
//      expected primary keys and check constraints.

package assistant_integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openLegacyRetirementPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestLegacyRetirementForundation_LedgerColumnIsNotNullNoDefault asserts
// the column shape against information_schema.columns and proves the
// NO-DEFAULT invariant via an adversarial INSERT.
func TestLegacyRetirementFoundation_LedgerColumnIsNotNullNoDefault(t *testing.T) {
	pool := openLegacyRetirementPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var (
		isNullable    string
		columnDefault *string
		dataType      string
	)
	err := pool.QueryRow(ctx, `
		SELECT is_nullable, column_default, data_type
		FROM information_schema.columns
		WHERE table_name = 'assistant_conversations'
		  AND column_name = 'legacy_retirement_notices'
	`).Scan(&isNullable, &columnDefault, &dataType)
	if errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("legacy_retirement_notices column missing from assistant_conversations; migration 046 did not apply")
	}
	if err != nil {
		t.Fatalf("information_schema query: %v", err)
	}
	if isNullable != "NO" {
		t.Errorf("legacy_retirement_notices.is_nullable = %q, want %q (NOT NULL)", isNullable, "NO")
	}
	if columnDefault != nil {
		t.Errorf("legacy_retirement_notices.column_default = %q, want NULL (NO DEFAULT — design forbids runtime fallback)", *columnDefault)
	}
	if dataType != "jsonb" {
		t.Errorf("legacy_retirement_notices.data_type = %q, want %q", dataType, "jsonb")
	}

	// Adversarial INSERT — must fail loud with NOT NULL violation.
	userID := fmt.Sprintf("spec-075-foundation-test-%d", time.Now().UnixNano())
	const transport = "test-no-fallback"
	_, insertErr := pool.Exec(ctx, `
		INSERT INTO assistant_conversations (user_id, transport, last_activity_at)
		VALUES ($1, $2, now())
	`, userID, transport)
	t.Cleanup(func() {
		// Defensive cleanup if the INSERT unexpectedly succeeded.
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	})
	if insertErr == nil {
		t.Fatal("INSERT into assistant_conversations without legacy_retirement_notices succeeded; the NOT NULL / NO DEFAULT contract is silently being satisfied by a fallback — regression")
	}
	// Postgres surfaces NOT NULL violations as 23502; surface the
	// error text in the failure log so a regression that changes
	// the SQLSTATE is easy to triage.
	if !strings.Contains(insertErr.Error(), "23502") && !strings.Contains(strings.ToLower(insertErr.Error()), "not-null") && !strings.Contains(strings.ToLower(insertErr.Error()), "not null") {
		t.Errorf("INSERT failed but not with NOT NULL violation: %v", insertErr)
	}

	// Positive control — explicit population must succeed.
	const ledgerJSON = `{"schema_version":1,"window_id":"2026-05-retirement","commands":{}}`
	_, posErr := pool.Exec(ctx, `
		INSERT INTO assistant_conversations (user_id, transport, last_activity_at, legacy_retirement_notices)
		VALUES ($1, $2, now(), $3::jsonb)
	`, userID, transport, ledgerJSON)
	if posErr != nil {
		t.Fatalf("explicit-population INSERT failed: %v", posErr)
	}
}

// TestLegacyRetirementFoundation_RuntimeStateTableShape asserts the
// pause-state table exists with its CHECK constraint enforcing the
// closed-set of allowed effective_state values.
func TestLegacyRetirementFoundation_RuntimeStateTableShape(t *testing.T) {
	pool := openLegacyRetirementPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var pkCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.table_constraints
		WHERE table_name = 'assistant_legacy_retirement_state'
		  AND constraint_type = 'PRIMARY KEY'
	`).Scan(&pkCount); err != nil {
		t.Fatalf("information_schema constraints query: %v", err)
	}
	if pkCount != 1 {
		t.Fatalf("assistant_legacy_retirement_state expected exactly 1 primary key; got %d", pkCount)
	}

	// Adversarial: insert with effective_state="closed" must fail —
	// CHECK constraint pins the closed-set to {open, paused} because
	// "closed" is SST-only and never written here.
	rowID := fmt.Sprintf("spec-075-state-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_state WHERE state_id = $1`, rowID)
	})
	_, err := pool.Exec(ctx, `
		INSERT INTO assistant_legacy_retirement_state
		    (state_id, window_id, effective_state, consecutive_days_over_threshold, updated_at, updated_by, schema_version)
		VALUES ($1, 'test-window', 'closed', 0, now(), 'test', 1)
	`, rowID)
	if err == nil {
		t.Fatal("INSERT with effective_state='closed' must fail (CHECK constraint should restrict to {open, paused}); succeeded — regression")
	}
}

// TestLegacyRetirementFoundation_ObservationsTableShape proves the
// observation report table exists and accepts the documented shape.
func TestLegacyRetirementFoundation_ObservationsTableShape(t *testing.T) {
	pool := openLegacyRetirementPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reportID := fmt.Sprintf("spec-075-obs-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_observations WHERE report_id = $1`, reportID)
	})
	_, err := pool.Exec(ctx, `
		INSERT INTO assistant_legacy_retirement_observations
		    (report_id, window_id, observation_started_at, observation_ended_at,
		     retired_handler_invocations, generated_at, schema_version)
		VALUES ($1, 'test-window', now() - interval '14 days', now(), 0, now(), 1)
	`, reportID)
	if err != nil {
		t.Fatalf("observations INSERT failed: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM assistant_legacy_retirement_observations WHERE report_id = $1`,
		reportID,
	).Scan(&count); err != nil {
		t.Fatalf("observations SELECT failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("observations row not persisted; got %d rows", count)
	}
}
