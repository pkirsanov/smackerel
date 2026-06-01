//go:build e2e

// Spec 075 SCOPE-2 — TP-075-08 / SCN-075-A09.
//
// Live-stack regression that the JSONB ledger surfaces a notice
// recorded against one transport when the same user is queried via
// another transport. Demonstrates the cross-transport dedup
// invariant at the SQL boundary rather than at the policy level.
//
// This test seeds an assistant_conversations row for (user,
// telegram), records a notice via SQLNoticeLedger, then asserts
// HasNotified returns true for the same user under transport="web"
// (the EXISTS query is keyed on user_id only, not transport).

package assistant_e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openCrossTransportPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live stack DB not available")
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

func seedRow(t *testing.T, pool *pgxpool.Pool, userID, transport, windowID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ledger := fmt.Sprintf(`{"schema_version":1,"window_id":%q,"commands":{}}`, windowID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO assistant_conversations
		    (user_id, transport, last_activity_at, legacy_retirement_notices)
		VALUES ($1, $2, now(), $3::jsonb)
		ON CONFLICT (user_id, transport)
		    DO UPDATE SET legacy_retirement_notices = EXCLUDED.legacy_retirement_notices
	`, userID, transport, ledger); err != nil {
		t.Fatalf("seed (%s,%s): %v", userID, transport, err)
	}
}

// TestSQLNoticeLedger_TP_075_08_CrossTransportDedup proves
// SCN-075-A09: a notice recorded against one transport suppresses
// the notice when the same user is later queried via another
// transport, because the ledger key excludes transport.
func TestSQLNoticeLedger_TP_075_08_CrossTransportDedup(t *testing.T) {
	pool := openCrossTransportPool(t)
	const windowID = "tp-075-08-window"
	const command = "/weather"
	userID := fmt.Sprintf("tp-075-08-user-%d", time.Now().UnixNano())

	// Seed both transports for this user.
	seedRow(t, pool, userID, "telegram", windowID)
	seedRow(t, pool, userID, "web", windowID)
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	})

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()

	// Telegram first.
	if err := ledger.MarkShown(ctx, userID, command, windowID, time.Now().UTC()); err != nil {
		t.Fatalf("MarkShown: %v", err)
	}

	// Cross-transport query: the EXISTS predicate must find the
	// entry regardless of which transport row is matched first.
	if ok, err := ledger.HasNotified(ctx, userID, command, windowID); err != nil || !ok {
		t.Fatalf("cross-transport HasNotified: ok=%v err=%v — dedup broken across transports", ok, err)
	}

	// Adversarial: a different user must NOT be affected.
	otherUserID := userID + "-other"
	seedRow(t, pool, otherUserID, "web", windowID)
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, otherUserID)
	})
	if ok, err := ledger.HasNotified(ctx, otherUserID, command, windowID); err != nil || ok {
		t.Fatalf("HasNotified for unrelated user must be false; got ok=%v err=%v — user keying broken", ok, err)
	}
}
