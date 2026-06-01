//go:build integration

// Spec 075 SCOPE-3 — TP-075-10 / SCN-075-A04.
//
// Live-stack integration proof of the residual-usage SQL roll-up
// and the rolling 7-day report:
//
//  1. SQLResidualStore.Record UPSERTs into
//     assistant_legacy_retirement_residual with the correct
//     per-day count.
//  2. Repeat observations on the same UTC day bump the count
//     (not insert a duplicate row).
//  3. RollingSevenDay returns per-(command, day) rows whose
//     invocations sum matches the inserted observations and whose
//     distinct_users == number of distinct HMAC buckets observed
//     for that (command, day).
//  4. The per-command summary aggregates over the trailing 7 days
//     with correct distinct-user counts.
//  5. Observations outside the trailing 7-day window are excluded
//     (adversarial: insert an 8-day-old row and assert the report
//     does NOT count it).
package monitoring_integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openResidualPool(t *testing.T) *pgxpool.Pool {
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

func cleanupResidualWindow(t *testing.T, pool *pgxpool.Pool, windowID string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel(cancel)
		_, _ = pool.Exec(ctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})
}

func ccancel(c context.CancelFunc) { c() }

const hmacBucketA = "0000000000000000000000000000000000000000000000000000000000000001"
const hmacBucketB = "0000000000000000000000000000000000000000000000000000000000000002"

func TestSQLResidualStore_UpsertAndRollingSevenDay(t *testing.T) {
	pool := openResidualPool(t)
	windowID := fmt.Sprintf("spec-075-scope-3-int-%d", time.Now().UnixNano())
	cleanupResidualWindow(t, pool, windowID)

	// Anchor the test clock to a stable UTC midnight so the rolling
	// window boundaries are deterministic.
	anchor := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return anchor },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore: %v", err)
	}

	ctx := context.Background()
	// Two observations for /weather by bucket A on the anchor day,
	// one by bucket B on the anchor day.
	if err := store.RecordWithError(ctx, "/weather", hmacBucketA, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("Record /weather A 1: %v", err)
	}
	if err := store.RecordWithError(ctx, "/weather", hmacBucketA, legacyretirement.OutcomeServedNoNotice); err != nil {
		t.Fatalf("Record /weather A 2: %v", err)
	}
	if err := store.RecordWithError(ctx, "/weather", hmacBucketB, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("Record /weather B: %v", err)
	}

	report, err := store.RollingSevenDay(ctx, windowID, anchor)
	if err != nil {
		t.Fatalf("RollingSevenDay: %v", err)
	}

	if got, want := len(report.PerDay), 1; got != want {
		t.Fatalf("per-day rows = %d, want %d (rows=%+v)", got, want, report.PerDay)
	}
	row := report.PerDay[0]
	if row.Command != "/weather" {
		t.Errorf("PerDay[0].Command = %q, want /weather", row.Command)
	}
	if row.Invocations != 3 {
		t.Errorf("PerDay[0].Invocations = %d, want 3", row.Invocations)
	}
	if row.DistinctUsers != 2 {
		t.Errorf("PerDay[0].DistinctUsers = %d, want 2", row.DistinctUsers)
	}

	if got, want := len(report.PerCommand), 1; got != want {
		t.Fatalf("per-command rows = %d, want %d", got, want)
	}
	if report.PerCommand[0].Invocations != 3 || report.PerCommand[0].DistinctUsers != 2 {
		t.Errorf("PerCommand[0] = %+v, want invocations=3 distinct=2", report.PerCommand[0])
	}
}

// TestSQLResidualStore_RollingWindowExcludesOlderDays inserts an
// 8-day-old row directly via SQL (bypassing the clock-driven store)
// and asserts the rolling 7-day report does NOT count it.
// Adversarial: a regression that widened the WHERE clause to
// "day <= now" would surface the older row here.
func TestSQLResidualStore_RollingWindowExcludesOlderDays(t *testing.T) {
	pool := openResidualPool(t)
	windowID := fmt.Sprintf("spec-075-scope-3-window-int-%d", time.Now().UnixNano())
	cleanupResidualWindow(t, pool, windowID)

	anchor := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return anchor },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore: %v", err)
	}

	ctx := context.Background()
	// In-window observation on anchor day.
	if err := store.RecordWithError(ctx, "/remind", hmacBucketA, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("Record in-window: %v", err)
	}

	// Out-of-window: 8 days before anchor.
	oldDay := time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)
	oldTs := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO assistant_legacy_retirement_residual
		    (window_id, command, user_bucket, day, count, last_seen_at)
		VALUES ($1, $2, $3, $4, 5, $5)
	`, windowID, "/remind", hmacBucketB, oldDay, oldTs); err != nil {
		t.Fatalf("direct insert old row: %v", err)
	}

	report, err := store.RollingSevenDay(ctx, windowID, anchor)
	if err != nil {
		t.Fatalf("RollingSevenDay: %v", err)
	}

	// Only the in-window row may appear.
	if got := len(report.PerDay); got != 1 {
		t.Fatalf("per-day rows = %d, want 1 (the 8-day-old row must be excluded); rows=%+v", got, report.PerDay)
	}
	if report.PerDay[0].Day.Equal(oldDay) {
		t.Fatal("report contains the 8-day-old row; rolling-window WHERE clause regression")
	}
	if report.PerCommand[0].Invocations != 1 {
		t.Errorf("PerCommand invocations = %d, want 1 (old row of 5 must NOT be summed)", report.PerCommand[0].Invocations)
	}
}
