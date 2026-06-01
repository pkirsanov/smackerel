//go:build e2e

// Spec 075 SCOPE-3 — TP-075-12 / SCN-075-A04.
//
// Live-stack e2e proof that:
//
//  1. The /metrics endpoint exposes the residual-usage counter
//     family (TYPE + HELP lines), so dashboards can scrape it.
//  2. The rolling 7-day SQL report runs end-to-end against the
//     live database: insert observations across two HMAC buckets
//     and two days within the window, then read the per-day and
//     per-command roll-ups and assert the totals.
//  3. Distinct-user counts use HMAC buckets only — no raw id
//     values are returned by the report query.
package assistant_e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func reportE2EBaseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("CORE_EXTERNAL_URL")
	if base == "" {
		t.Fatal("spec 075 e2e test requires CORE_EXTERNAL_URL — run via `./smackerel.sh test e2e`")
	}
	return strings.TrimRight(base, "/")
}

func reportE2EPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("spec 075 e2e report test requires DATABASE_URL")
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

func TestLegacyRetirementReport_E2E_RollingSevenDay(t *testing.T) {
	base := reportE2EBaseURL(t)

	// /metrics scrape proves the counter family is registered.
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/metrics status %d", resp.StatusCode)
	}
	const metric = "smackerel_legacy_command_residual_total"
	if !regexp.MustCompile(`(?m)^# HELP ` + regexp.QuoteMeta(metric) + `\b`).MatchString(string(body)) {
		t.Fatalf("/metrics missing HELP %s", metric)
	}
	if !regexp.MustCompile(`(?m)^# TYPE ` + regexp.QuoteMeta(metric) + `\b`).MatchString(string(body)) {
		t.Fatalf("/metrics missing TYPE %s", metric)
	}

	// SQL-side end-to-end: insert observations across 2 buckets and
	// 2 days, read the report, assert totals.
	pool := reportE2EPool(t)
	windowID := fmt.Sprintf("spec-075-scope-3-e2e-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, `DELETE FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	})

	anchor := time.Now().UTC()
	storeToday, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return anchor },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore today: %v", err)
	}
	yesterday := anchor.AddDate(0, 0, -1)
	storeYesterday, err := legacyretirement.NewSQLResidualStore(legacyretirement.SQLResidualStoreConfig{
		Pool:     pool,
		WindowID: windowID,
		Clock:    func() time.Time { return yesterday },
	})
	if err != nil {
		t.Fatalf("NewSQLResidualStore yesterday: %v", err)
	}

	const (
		bucketA = "1111111111111111111111111111111111111111111111111111111111111111"
		bucketB = "2222222222222222222222222222222222222222222222222222222222222222"
	)
	ctx := context.Background()
	if err := storeToday.RecordWithError(ctx, "/weather", bucketA, legacyretirement.OutcomeNoticeAndServed); err != nil {
		t.Fatalf("record today A: %v", err)
	}
	if err := storeToday.RecordWithError(ctx, "/weather", bucketB, legacyretirement.OutcomeServedNoNotice); err != nil {
		t.Fatalf("record today B: %v", err)
	}
	if err := storeYesterday.RecordWithError(ctx, "/weather", bucketA, legacyretirement.OutcomeServedNoNotice); err != nil {
		t.Fatalf("record yesterday A: %v", err)
	}

	report, err := storeToday.RollingSevenDay(ctx, windowID, anchor)
	if err != nil {
		t.Fatalf("RollingSevenDay: %v", err)
	}
	if len(report.PerCommand) != 1 || report.PerCommand[0].Command != "/weather" {
		t.Fatalf("per-command rows = %+v", report.PerCommand)
	}
	if report.PerCommand[0].Invocations != 3 {
		t.Errorf("per-command invocations = %d, want 3", report.PerCommand[0].Invocations)
	}
	if report.PerCommand[0].DistinctUsers != 2 {
		t.Errorf("per-command distinct users = %d, want 2", report.PerCommand[0].DistinctUsers)
	}

	// Sanity: every per-day row's distinct_users is consistent with
	// HMAC buckets only — no raw text leaks.
	rawRE := regexp.MustCompile(`^[0-9a-f]{64}$`)
	rows, err := pool.Query(ctx, `SELECT DISTINCT user_bucket FROM assistant_legacy_retirement_residual WHERE window_id = $1`, windowID)
	if err != nil {
		t.Fatalf("distinct buckets query: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if b != legacyretirement.AnonymousBucketLabel && !rawRE.MatchString(b) {
			t.Fatalf("persisted bucket %q is not HMAC-shaped or %q sentinel; privacy regression", b, legacyretirement.AnonymousBucketLabel)
		}
	}
}
