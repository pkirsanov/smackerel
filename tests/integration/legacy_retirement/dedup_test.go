//go:build integration

// Spec 076 SCOPE-6d — TP-076-06-02 / SCN-075-A02.
//
// Live-stack integration proof: the SQL NoticeLedger dedups a second
// MarkShown for the same (user, command, window) — HasNotified
// returns true after the first mark, the second mark preserves
// first_notified_at and bumps notice_count to 2 without creating a
// second logical entry.

package legacyretirement_integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openPool(t *testing.T) *pgxpool.Pool {
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

// TestRetirement_SecondInvocationDoesNotRenotify covers SCN-075-A02
// against the live SQL ledger on the disposable test stack.
func TestRetirement_SecondInvocationDoesNotRenotify(t *testing.T) {
	pool := openPool(t)
	const (
		windowID = "tp-076-06-02-window"
		command  = "/weather"
	)
	userID := fmt.Sprintf("tp-076-06-02-user-%d", time.Now().UnixNano())
	seedConversation(t, pool, userID, "telegram", windowID)

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()

	first := time.Date(2026, 6, 2, 9, 0, 0, 0, time.UTC)
	second := first.Add(45 * time.Minute)

	if ok, err := ledger.HasNotified(ctx, userID, command, windowID); err != nil || ok {
		t.Fatalf("HasNotified pre-mark: ok=%v err=%v", ok, err)
	}
	if err := ledger.MarkShown(ctx, userID, command, windowID, first); err != nil {
		t.Fatalf("first MarkShown: %v", err)
	}
	if ok, err := ledger.HasNotified(ctx, userID, command, windowID); err != nil || !ok {
		t.Fatalf("HasNotified post-first-mark: ok=%v err=%v — dedup gate broken", ok, err)
	}
	if err := ledger.MarkShown(ctx, userID, command, windowID, second); err != nil {
		t.Fatalf("second MarkShown: %v", err)
	}

	entry, ok, err := ledger.Get(ctx, userID, command, windowID)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if entry.NoticeCount != 2 {
		t.Errorf("NoticeCount=%d, want 2 (second MarkShown must bump count, not create new entry)", entry.NoticeCount)
	}
	if !entry.FirstNotifiedAt.Equal(first) {
		t.Errorf("FirstNotifiedAt drift: got %s, want %s — second MarkShown rewrote the audit timestamp", entry.FirstNotifiedAt, first)
	}
	if !entry.LastSeenAt.Equal(second) {
		t.Errorf("LastSeenAt=%s, want %s — second MarkShown did not advance last_seen_at", entry.LastSeenAt, second)
	}

	// Adversarial: ensure only ONE row in the JSONB commands map for
	// this (user, command, window). A regression that wrote a second
	// entry under a perturbed key would surface here.
	var commands map[string]any
	if err := pool.QueryRow(ctx, `
		SELECT legacy_retirement_notices->'commands'
		  FROM assistant_conversations
		 WHERE user_id = $1`, userID).Scan(&commands); err != nil {
		t.Fatalf("read commands map: %v", err)
	}
	if len(commands) != 1 {
		t.Errorf("commands map has %d entries (%v); want exactly 1 — dedup keying drift", len(commands), commands)
	}
}
