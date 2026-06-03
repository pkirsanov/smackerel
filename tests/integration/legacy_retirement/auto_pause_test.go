//go:build integration

// Spec 076 SCOPE-6d — TP-076-06-05 / SCN-075-A05.
//
// Live-stack integration proof: the ThresholdEvaluator, when fed a
// breaching rolling-7-day report against the live SQL pause store,
// writes a paused row to assistant_legacy_retirement_state for the
// configured window — proving the end-to-end auto-pause path from
// rolling report → evaluator decision → SQL pause row → resolver
// observation.

package legacyretirement_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

// TestRetirement_ThresholdAutoPausesWindow covers SCN-075-A05
// against the live test-stack Postgres.
func TestRetirement_ThresholdAutoPausesWindow(t *testing.T) {
	pool := openPool(t)
	windowID := fmt.Sprintf("tp-076-06-05-%d", time.Now().UnixNano())
	command := "/weather"

	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_state WHERE state_id = $1`, windowID)
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	// Seed three consecutive breaching days for /weather. Active
	// users denominator = 10, daily distinct users per breaching day
	// = 6 → 60% > 5% threshold = breach. DaysConsecutive=3 → auto
	// pause.
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i)
		for j := 0; j < 6; j++ {
			bucket := fmt.Sprintf("%064x", (i*16)+j+1)
			if _, err := pool.Exec(ctx, `
				INSERT INTO assistant_legacy_retirement_residual
				    (window_id, command, user_bucket, day, count, last_seen_at)
				VALUES ($1, $2, $3, $4, 1, $5)
				ON CONFLICT (window_id, command, user_bucket, day)
				    DO UPDATE SET count = assistant_legacy_retirement_residual.count + 1,
				                  last_seen_at = EXCLUDED.last_seen_at`,
				windowID, command, bucket, day, now); err != nil {
				t.Fatalf("seed residual day=%s bucket=%s: %v", day, bucket, err)
			}
		}
	}

	residualStore, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore: %v", err)
	}
	pauseStore, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		t.Fatalf("NewSQLPauseStateStore: %v", err)
	}
	evaluator, err := legacyretirement.NewThresholdEvaluator(
		legacyretirement.ThresholdConfig{
			WindowID:                  windowID,
			PercentActiveUsers:        5.0,
			DaysConsecutive:           3,
			ActiveUserWindowDays:      7,
			ThresholdEvaluatorUpdater: "tp-076-06-05",
		},
		residualStore,
		legacyretirement.StaticActiveUsersProvider{Count: 10},
		pauseStore,
	)
	if err != nil {
		t.Fatalf("NewThresholdEvaluator: %v", err)
	}

	results, err := evaluator.Evaluate(ctx, now)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	var weatherResult *legacyretirement.ThresholdEvaluation
	for i := range results {
		if results[i].Command == command {
			weatherResult = &results[i]
			break
		}
	}
	if weatherResult == nil {
		t.Fatalf("no evaluation for %q; results=%+v", command, results)
	}
	if !weatherResult.Breached {
		t.Fatalf("expected Breached=true; got %+v", *weatherResult)
	}
	if weatherResult.ConsecutiveDays != 3 {
		t.Errorf("ConsecutiveDays=%d, want 3", weatherResult.ConsecutiveDays)
	}

	// Live pause row must exist with effective_state='paused'.
	var effective, updatedBy, thresholdCommand string
	var days int
	if err := pool.QueryRow(ctx, `
		SELECT effective_state, consecutive_days_over_threshold, updated_by, threshold_command
		  FROM assistant_legacy_retirement_state
		 WHERE state_id = $1`, windowID).
		Scan(&effective, &days, &updatedBy, &thresholdCommand); err != nil {
		t.Fatalf("read pause row: %v", err)
	}
	if effective != "paused" || days != 3 || updatedBy != "tp-076-06-05" || thresholdCommand != command {
		t.Errorf("pause row mismatch: effective=%q days=%d updatedBy=%q command=%q",
			effective, days, updatedBy, thresholdCommand)
	}

	// Resolver observes the live paused state.
	if paused, err := pauseStore.IsPaused(ctx, windowID); err != nil || !paused {
		t.Fatalf("IsPaused post-evaluate: paused=%v err=%v", paused, err)
	}
}
