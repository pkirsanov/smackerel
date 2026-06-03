//go:build integration

// Spec 076 SCOPE-6d — TP-076-06-06 / SCN-075-A06.
//
// Live-stack integration proof: Resume on the SQL pause store flips
// the row back to effective_state='open' and resets
// consecutive_days_over_threshold to 0 — proving operator resume
// resets the auto-pause counter without touching residual telemetry.

package legacyretirement_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// TestRetirement_ResumeResetsConsecutiveDayCounter covers
// SCN-075-A06 against the live test-stack Postgres.
func TestRetirement_ResumeResetsConsecutiveDayCounter(t *testing.T) {
	pool := openPool(t)
	windowID := fmt.Sprintf("tp-076-06-06-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_state WHERE state_id = $1`, windowID)
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	store, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		t.Fatalf("NewSQLPauseStateStore: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	// Pre-pause to set consecutive_days_over_threshold = 3.
	if err := store.Pause(ctx, windowID, "/weather", 3, now, "tp-076-06-06-evaluator"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused, err := store.IsPaused(ctx, windowID); err != nil || !paused {
		t.Fatalf("IsPaused post-pause: paused=%v err=%v", paused, err)
	}

	// Pre-insert a residual row that Resume MUST NOT touch.
	bucket := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO assistant_legacy_retirement_residual
		    (window_id, command, user_bucket, day, count, last_seen_at)
		VALUES ($1, '/weather', $2, $3, 7, $4)`,
		windowID, bucket, day, now); err != nil {
		t.Fatalf("seed residual: %v", err)
	}

	if err := store.Resume(ctx, windowID, now.Add(time.Hour), "tp-076-06-06-operator"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	var effective, updatedBy string
	var days int
	if err := pool.QueryRow(ctx, `
		SELECT effective_state, consecutive_days_over_threshold, updated_by
		  FROM assistant_legacy_retirement_state
		 WHERE state_id = $1`, windowID).
		Scan(&effective, &days, &updatedBy); err != nil {
		t.Fatalf("read pause row post-resume: %v", err)
	}
	if effective != "open" {
		t.Errorf("effective_state=%q, want open", effective)
	}
	if days != 0 {
		t.Errorf("consecutive_days_over_threshold=%d, want 0 (Resume MUST reset counter)", days)
	}
	if updatedBy != "tp-076-06-06-operator" {
		t.Errorf("updated_by=%q, want tp-076-06-06-operator (audit label not propagated)", updatedBy)
	}
	if paused, err := store.IsPaused(ctx, windowID); err != nil || paused {
		t.Fatalf("IsPaused post-resume: paused=%v err=%v — Resume did not clear effective state", paused, err)
	}

	// Residual row MUST survive Resume.
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count FROM assistant_legacy_retirement_residual
		 WHERE window_id = $1 AND command = '/weather' AND user_bucket = $2 AND day = $3`,
		windowID, bucket, day).Scan(&count); err != nil {
		t.Fatalf("residual survival check: %v", err)
	}
	if count != 7 {
		t.Errorf("residual count=%d post-resume, want 7 — Resume must not touch residual telemetry", count)
	}
}
