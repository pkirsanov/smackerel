//go:build integration

// Spec 075 SCOPE-2 — TP-075-05/06/07.
//
// Live-stack integration proof that the SQLNoticeLedger persists and
// dedups notices against the JSONB column on assistant_conversations
// added by migration 046. Covers SCN-075-A01..A03 at the storage
// boundary (the in-process policy tests in
// internal/assistant/legacyretirement/policy_test.go cover the
// decision logic without a live DB).
//
// Live-system rationale: this test exists to prove the SQL-side
// shape — CRUD against real Postgres — that the unit tests cannot
// reach. It is intentionally redundant with the in-memory ledger
// tests at the contract level; the value is in catching JSONB query
// drift, NOT NULL regressions, or migration version skew before they
// reach production.

package assistant_integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openNoticeLedgerPool(t *testing.T) *pgxpool.Pool {
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

func seedConversation(t *testing.T, pool *pgxpool.Pool, userID, transport, windowID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ledger := fmt.Sprintf(`{"schema_version":1,"window_id":%q,"commands":{}}`, windowID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO assistant_conversations
		    (user_id, transport, last_activity_at, legacy_retirement_notices)
		VALUES ($1, $2, now(), $3::jsonb)
		ON CONFLICT (user_id, transport)
		    DO UPDATE SET legacy_retirement_notices = EXCLUDED.legacy_retirement_notices,
		                  last_activity_at = EXCLUDED.last_activity_at
	`, userID, transport, ledger); err != nil {
		t.Fatalf("seed conversation (%s,%s): %v", userID, transport, err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	})
}

// TestSQLNoticeLedger_TP_075_05_MarkAndDedup covers TP-075-05 /
// SCN-075-A01: first MarkShown writes the entry; HasNotified
// observes it.
func TestSQLNoticeLedger_TP_075_05_MarkAndDedup(t *testing.T) {
	pool := openNoticeLedgerPool(t)
	const (
		windowID = "tp-075-05-window"
		command  = "/weather"
	)
	userID := fmt.Sprintf("tp-075-05-user-%d", time.Now().UnixNano())
	transport := "telegram"
	seedConversation(t, pool, userID, transport, windowID)

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()
	if ok, err := ledger.HasNotified(ctx, userID, command, windowID); err != nil || ok {
		t.Fatalf("HasNotified pre-mark: ok=%v err=%v", ok, err)
	}
	if err := ledger.MarkShown(ctx, userID, command, windowID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkShown: %v", err)
	}
	if ok, err := ledger.HasNotified(ctx, userID, command, windowID); err != nil || !ok {
		t.Fatalf("HasNotified post-mark: ok=%v err=%v", ok, err)
	}
	entry, ok, err := ledger.Get(ctx, userID, command, windowID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if entry.NoticeCount != 1 {
		t.Errorf("NoticeCount=%d, want 1", entry.NoticeCount)
	}
}

// TestSQLNoticeLedger_TP_075_06_RepeatMarkBumpsCountKeepsFirstTime
// covers TP-075-06 / SCN-075-A02: repeated MarkShown preserves
// first_notified_at and bumps notice_count.
func TestSQLNoticeLedger_TP_075_06_RepeatMarkBumpsCountKeepsFirstTime(t *testing.T) {
	pool := openNoticeLedgerPool(t)
	const windowID = "tp-075-06-window"
	const command = "/weather"
	userID := fmt.Sprintf("tp-075-06-user-%d", time.Now().UnixNano())
	seedConversation(t, pool, userID, "telegram", windowID)

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()
	first := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	if err := ledger.MarkShown(ctx, userID, command, windowID, first); err != nil {
		t.Fatalf("first MarkShown: %v", err)
	}
	if err := ledger.MarkShown(ctx, userID, command, windowID, second); err != nil {
		t.Fatalf("second MarkShown: %v", err)
	}
	entry, ok, err := ledger.Get(ctx, userID, command, windowID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if entry.NoticeCount != 2 {
		t.Errorf("NoticeCount=%d, want 2", entry.NoticeCount)
	}
	if !entry.FirstNotifiedAt.Equal(first) {
		t.Errorf("FirstNotifiedAt drift: got %s, want %s", entry.FirstNotifiedAt, first)
	}
	if !entry.LastSeenAt.Equal(second) {
		t.Errorf("LastSeenAt: got %s, want %s", entry.LastSeenAt, second)
	}
}

// TestSQLNoticeLedger_TP_075_07_PerCommandIndependence covers
// TP-075-07 / SCN-075-A03: a /weather entry MUST NOT mark /remind
// as notified.
func TestSQLNoticeLedger_TP_075_07_PerCommandIndependence(t *testing.T) {
	pool := openNoticeLedgerPool(t)
	const windowID = "tp-075-07-window"
	userID := fmt.Sprintf("tp-075-07-user-%d", time.Now().UnixNano())
	seedConversation(t, pool, userID, "telegram", windowID)

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()
	if err := ledger.MarkShown(ctx, userID, "/weather", windowID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkShown weather: %v", err)
	}
	if ok, _ := ledger.HasNotified(ctx, userID, "/remind", windowID); ok {
		t.Fatal("/remind dedup contaminated by /weather entry — per-command keying broken")
	}
	if ok, _ := ledger.HasNotified(ctx, userID, "/weather", windowID); !ok {
		t.Fatal("/weather entry missing after MarkShown")
	}
}
