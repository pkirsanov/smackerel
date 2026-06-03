//go:build e2e

// Spec 076 SCOPE-6d — TP-076-06-09 / SCN-075-A09.
//
// Live-stack e2e proof: the SQL NoticeLedger dedups across transports
// for the same (user_id, command, window_id). A first MarkShown on
// the Telegram conversation row MUST cause HasNotified to return
// true even when queried via a separate transport (web), because the
// ledger key includes only user_id + command + window_id and the
// dedup query unions across all conversation rows for the same
// user_id and command.

package transports_e2e

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
		t.Skip("e2e: DATABASE_URL not set — live test stack DB not available")
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

func seedCrossTransportConversation(t *testing.T, pool *pgxpool.Pool, userID, transport, windowID string) {
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
}

// TestRetirement_DedupSurvivesAcrossTransports covers SCN-075-A09
// against the live test-stack Postgres. A single user with rows in
// two transports (telegram + web) sees the notice exactly once for
// the same retired command, because the ledger key is transport-blind.
func TestRetirement_DedupSurvivesAcrossTransports(t *testing.T) {
	pool := openCrossTransportPool(t)
	const (
		windowID = "tp-076-06-09-window"
		command  = "/weather"
	)
	userID := fmt.Sprintf("tp-076-06-09-user-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	})

	seedCrossTransportConversation(t, pool, userID, "telegram", windowID)
	seedCrossTransportConversation(t, pool, userID, "web", windowID)

	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	// Pre: neither row reports notified.
	if ok, _ := ledger.HasNotified(ctx, userID, command, windowID); ok {
		t.Fatal("HasNotified=true pre-mark — fixture leaked from a prior run")
	}

	// MarkShown — implementation may persist into any one of the
	// user's conversation rows.
	if err := ledger.MarkShown(ctx, userID, command, windowID, now); err != nil {
		t.Fatalf("MarkShown: %v", err)
	}

	// Adversarial: HasNotified MUST return true regardless of which
	// transport the next turn arrives on. A regression that keyed the
	// dedup query by (user_id, transport) would fail here on the
	// transport whose JSONB column was not updated.
	if ok, err := ledger.HasNotified(ctx, userID, command, windowID); err != nil || !ok {
		t.Fatalf("HasNotified post-mark: ok=%v err=%v — cross-transport dedup broken", ok, err)
	}

	// Verify by direct SQL that at least one of the two transport
	// rows carries the entry and dedup is observable regardless of
	// which row was the writer.
	var telegramHas, webHas bool
	if err := pool.QueryRow(ctx, `
		SELECT legacy_retirement_notices->'commands' ? $1
		  FROM assistant_conversations
		 WHERE user_id = $2 AND transport = 'telegram'`, command, userID).Scan(&telegramHas); err != nil {
		t.Fatalf("telegram row check: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT legacy_retirement_notices->'commands' ? $1
		  FROM assistant_conversations
		 WHERE user_id = $2 AND transport = 'web'`, command, userID).Scan(&webHas); err != nil {
		t.Fatalf("web row check: %v", err)
	}
	// MarkShown updates every conversation row for the user, so BOTH
	// transport rows MUST carry the entry — proving the dedup contract
	// does not depend on which transport sent the original turn.
	if !telegramHas || !webHas {
		t.Fatalf("cross-transport persistence broken: telegram=%v web=%v (both must be true)", telegramHas, webHas)
	}
}
