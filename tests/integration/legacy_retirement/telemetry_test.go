//go:build integration

// Spec 076 SCOPE-6d — TP-076-06-04 / SCN-075-A04.
//
// Live-stack integration proof: the SQL residual store records
// retired-command invocations per-(command, user_bucket) row,
// proving the dashboard primary key (window_id, command,
// user_bucket, day) keeps invocations from different commands and
// different HMAC buckets independent. Mirrors the rolling 7-day
// report shape consumed by the spec 049 dashboard panel.

package legacyretirement_integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

const (
	bucketAlpha = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bucketBeta  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// TestRetirement_ResidualTelemetryCountsPerCommandAndBucket covers
// SCN-075-A04 against the live test-stack Postgres.
func TestRetirement_ResidualTelemetryCountsPerCommandAndBucket(t *testing.T) {
	pool := openPool(t)
	windowID := fmt.Sprintf("tp-076-06-04-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	anchor := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	store, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return anchor },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore: %v", err)
	}
	ctx := context.Background()

	// Drive observations across two commands and two HMAC buckets.
	if err := store.RecordWithError(ctx, "/weather", bucketAlpha, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("record /weather alpha 1: %v", err)
	}
	if err := store.RecordWithError(ctx, "/weather", bucketAlpha, legacyretirement.OutcomeServedNoNotice); err != nil {
		t.Fatalf("record /weather alpha 2: %v", err)
	}
	if err := store.RecordWithError(ctx, "/weather", bucketBeta, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("record /weather beta: %v", err)
	}
	if err := store.RecordWithError(ctx, "/remind", bucketAlpha, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("record /remind alpha: %v", err)
	}

	report, err := store.RollingSevenDay(ctx, windowID, anchor)
	if err != nil {
		t.Fatalf("RollingSevenDay: %v", err)
	}

	perCmd := make(map[string]legacyretirement.ResidualPerCommandRow, len(report.PerCommand))
	for _, r := range report.PerCommand {
		perCmd[r.Command] = r
	}
	w, ok := perCmd["/weather"]
	if !ok {
		t.Fatalf("/weather row missing from PerCommand: %+v", report.PerCommand)
	}
	if w.Invocations != 3 {
		t.Errorf("/weather invocations=%d, want 3 (2 from alpha + 1 from beta)", w.Invocations)
	}
	if w.DistinctUsers != 2 {
		t.Errorf("/weather distinct_users=%d, want 2 (alpha + beta buckets)", w.DistinctUsers)
	}
	r, ok := perCmd["/remind"]
	if !ok {
		t.Fatalf("/remind row missing from PerCommand: %+v", report.PerCommand)
	}
	if r.Invocations != 1 || r.DistinctUsers != 1 {
		t.Errorf("/remind row=%+v, want invocations=1 distinct_users=1 (per-command keying drift)", r)
	}

	// Adversarial: row count in the residual table is exactly 3
	// (two commands × two buckets, except /remind+beta is absent).
	var rowCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID).Scan(&rowCount); err != nil {
		t.Fatalf("row count: %v", err)
	}
	if rowCount != 3 {
		t.Errorf("residual table has %d rows for windowID %s, want 3 (composite PK regression?)", rowCount, windowID)
	}
}
