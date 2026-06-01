//go:build integration

// Spec 075 SCOPE-4 — TP-075-13 / TP-075-14.
//
// Live-stack integration proof for:
//
//   - TP-075-13 (SCN-075-A05): the SQL PauseStateStore Pause() path
//     writes a row to assistant_legacy_retirement_state with
//     effective_state='paused', the supplied threshold_command, and
//     consecutive_days_over_threshold, AND the WindowStateResolver's
//     IsPaused() then reports true (the same row the resolver reads).
//
//   - TP-075-14 (SCN-075-A06): Resume() flips the row back to
//     effective_state='open', resets consecutive_days_over_threshold
//     to 0, and clears threshold_command. Residual telemetry rows in
//     assistant_legacy_retirement_residual are NOT touched (the test
//     pre-inserts a residual observation and asserts it survives).
//
// Adversarial sub-test (boundary): Pause() must reject empty
// command / windowID / updatedBy at the SQL boundary so a regression
// that loosens the writer cannot silently land an unaudited pause.

package assistant_integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openLegacyRetirementPoolForThreshold(t *testing.T) *pgxpool.Pool {
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

func TestSQLPauseStateStore_TP_075_13_PauseWritesAndResolverReads(t *testing.T) {
	pool := openLegacyRetirementPoolForThreshold(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	windowID := "spec075-tp13-" + time.Now().UTC().Format("20060102150405")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM assistant_legacy_retirement_state WHERE state_id = $1`, windowID)
	})

	store, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		t.Fatalf("NewSQLPauseStateStore: %v", err)
	}

	// Pre: resolver reports not-paused (no row).
	paused, err := store.IsPaused(ctx, windowID)
	if err != nil {
		t.Fatalf("IsPaused (pre): %v", err)
	}
	if paused {
		t.Fatalf("expected IsPaused=false before Pause, got true")
	}

	now := time.Now().UTC()
	if err := store.Pause(ctx, windowID, "/weather", 3, now, "threshold_evaluator"); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Row shape (effective_state, threshold_command, consecutive_days).
	var (
		effective string
		command   string
		days      int
		updatedBy string
	)
	if err := pool.QueryRow(ctx, `
		SELECT effective_state, threshold_command, consecutive_days_over_threshold, updated_by
		  FROM assistant_legacy_retirement_state
		 WHERE state_id = $1`, windowID).
		Scan(&effective, &command, &days, &updatedBy); err != nil {
		t.Fatalf("select pause row: %v", err)
	}
	if effective != "paused" || command != "/weather" || days != 3 || updatedBy != "threshold_evaluator" {
		t.Errorf("pause row mismatch: effective=%q command=%q days=%d updatedBy=%q",
			effective, command, days, updatedBy)
	}

	// Resolver reads the same row.
	paused, err = store.IsPaused(ctx, windowID)
	if err != nil {
		t.Fatalf("IsPaused (post-pause): %v", err)
	}
	if !paused {
		t.Fatalf("expected IsPaused=true after Pause")
	}
}

func TestSQLPauseStateStore_TP_075_14_ResumeResetsAndPreservesTelemetry(t *testing.T) {
	pool := openLegacyRetirementPoolForThreshold(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	windowID := "spec075-tp14-" + time.Now().UTC().Format("20060102150405")
	bucket := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	day := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(), time.Now().UTC().Day(), 0, 0, 0, 0, time.UTC)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM assistant_legacy_retirement_state WHERE state_id = $1`, windowID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	store, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		t.Fatalf("NewSQLPauseStateStore: %v", err)
	}
	now := time.Now().UTC()
	if err := store.Pause(ctx, windowID, "/weather", 3, now, "threshold_evaluator"); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	// Pre-insert a residual row that Resume must NOT touch.
	if _, err := pool.Exec(ctx, `
		INSERT INTO assistant_legacy_retirement_residual
		    (window_id, command, user_bucket, day, count, last_seen_at)
		VALUES ($1, '/weather', $2, $3, 5, $4)`,
		windowID, bucket, day, now); err != nil {
		t.Fatalf("seed residual: %v", err)
	}

	if err := store.Resume(ctx, windowID, now.Add(time.Minute), "operator:test"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	var (
		effective string
		days      int
		updatedBy string
	)
	if err := pool.QueryRow(ctx, `
		SELECT effective_state, consecutive_days_over_threshold, updated_by
		  FROM assistant_legacy_retirement_state
		 WHERE state_id = $1`, windowID).
		Scan(&effective, &days, &updatedBy); err != nil {
		t.Fatalf("select post-resume row: %v", err)
	}
	if effective != "open" || days != 0 || updatedBy != "operator:test" {
		t.Errorf("post-resume row mismatch: effective=%q days=%d updatedBy=%q",
			effective, days, updatedBy)
	}

	// Residual row must survive.
	var residualCount int
	if err := pool.QueryRow(ctx, `
		SELECT count FROM assistant_legacy_retirement_residual
		 WHERE window_id = $1 AND command = '/weather' AND user_bucket = $2 AND day = $3`,
		windowID, bucket, day).Scan(&residualCount); err != nil {
		t.Fatalf("residual survival check: %v", err)
	}
	if residualCount != 5 {
		t.Errorf("Resume must not touch residual telemetry; expected count=5, got %d", residualCount)
	}
}

func TestSQLPauseStateStore_RejectsEmptyAuditInputs(t *testing.T) {
	pool := openLegacyRetirementPoolForThreshold(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		t.Fatalf("NewSQLPauseStateStore: %v", err)
	}
	now := time.Now().UTC()

	if err := store.Pause(ctx, "", "/weather", 3, now, "u"); err == nil {
		t.Errorf("Pause with empty windowID must fail")
	}
	if err := store.Pause(ctx, "w", "", 3, now, "u"); err == nil {
		t.Errorf("Pause with empty command must fail")
	}
	if err := store.Pause(ctx, "w", "/c", 0, now, "u"); err == nil {
		t.Errorf("Pause with zero consecutiveDays must fail")
	}
	if err := store.Pause(ctx, "w", "/c", 3, now, ""); err == nil {
		t.Errorf("Pause with empty updatedBy must fail")
	}
	if err := store.Resume(ctx, "", now, "u"); err == nil {
		t.Errorf("Resume with empty windowID must fail")
	}
	if err := store.Resume(ctx, "w", now, ""); err == nil {
		t.Errorf("Resume with empty updatedBy must fail")
	}
}
