//go:build e2e

// Spec 075 SCOPE-075-06.5 — TP-075-15.
//
// Live-stack e2e proof for SCN-075-A05 (paused branch): with the SST
// window_state="open" and the live SQLPauseStateStore reporting
// paused=true for LEGACY_RETIREMENT_WINDOW_ID, the Policy keeps the
// legacy serving mode active (ServeNL=true) AND suppresses the
// notice — no MarkShown is invoked and the user's JSONB ledger
// column in assistant_conversations is byte-identical before and
// after the paused turn.
//
// This is the SCOPE-075-06.4 / 06.5 companion to TP-075-13/14
// (which prove the store mechanics) and TP-075-16 (which proves the
// closed branch). It drives the same Policy the facade dispatches
// through, wired with the LIVE NoticeLedger and LIVE PauseStateStore
// against the test-stack Postgres.

package assistant_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

func openPausePool(t *testing.T) *pgxpool.Pool {
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

func readPauseSST(t *testing.T) (windowID, hmacKey string, notice, closed map[string]string) {
	t.Helper()
	windowID = os.Getenv("LEGACY_RETIREMENT_WINDOW_ID")
	if windowID == "" {
		t.Fatal("LEGACY_RETIREMENT_WINDOW_ID not set in test env (config generate --env test)")
	}
	hmacKey = os.Getenv("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY")
	if hmacKey == "" {
		t.Fatal("LEGACY_RETIREMENT_USER_BUCKET_HMAC_KEY not set in test env")
	}
	if v := os.Getenv("LEGACY_RETIREMENT_NOTICE_COPY_PER_COMMAND"); v != "" {
		if err := json.Unmarshal([]byte(v), &notice); err != nil {
			t.Fatalf("decode notice copy map: %v", err)
		}
	}
	if v := os.Getenv("LEGACY_RETIREMENT_POST_WINDOW_UNKNOWN_RESPONSE_COPY"); v != "" {
		if err := json.Unmarshal([]byte(v), &closed); err != nil {
			t.Fatalf("decode closed copy map: %v", err)
		}
	}
	if len(notice) == 0 || len(closed) == 0 {
		t.Fatalf("SST copy maps empty (notice=%d closed=%d)", len(notice), len(closed))
	}
	return windowID, hmacKey, notice, closed
}

// TestLegacyRetirementPauseE2E_PausedStateSuppressesNoticeAndKeepsServingNL
// is the SCOPE-075-06.5 / TP-075-15 live regression for SCN-075-A05's
// paused branch.
func TestLegacyRetirementPauseE2E_PausedStateSuppressesNoticeAndKeepsServingNL(t *testing.T) {
	pool := openPausePool(t)
	windowID, hmacKey, noticeCopy, closedCopy := readPauseSST(t)

	const command = "/weather"
	if _, ok := noticeCopy[command]; !ok {
		t.Fatalf("SST notice copy missing entry for %q — cannot drive paused branch", command)
	}

	userID := fmt.Sprintf("tp-075-15-user-%d", time.Now().UnixNano())
	transport := "web"

	// Seed an assistant_conversations row with an empty ledger so we
	// can byte-compare the JSONB column before and after the paused
	// turn. The pause branch MUST NOT mutate this column.
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer seedCancel()
	seedLedger := fmt.Sprintf(`{"schema_version":1,"window_id":%q,"commands":{}}`, windowID)
	if _, err := pool.Exec(seedCtx, `
		INSERT INTO assistant_conversations
		    (user_id, transport, last_activity_at, legacy_retirement_notices)
		VALUES ($1, $2, now(), $3::jsonb)
		ON CONFLICT (user_id, transport)
		    DO UPDATE SET legacy_retirement_notices = EXCLUDED.legacy_retirement_notices
	`, userID, transport, seedLedger); err != nil {
		t.Fatalf("seed conversation row: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	})

	// Pause the LIVE SST window via the real SQLPauseStateStore. The
	// resolver below reads back through the same store, so this is a
	// true round-trip against the test-stack Postgres. Cleanup
	// Resume() must run even on failure to avoid leaving the live
	// stack in a paused state for subsequent tests.
	pauseStore, err := legacyretirement.NewSQLPauseStateStore(pool)
	if err != nil {
		t.Fatalf("NewSQLPauseStateStore: %v", err)
	}
	pauseCtx, pauseCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer pauseCancel()
	pauseTime := time.Now().UTC()
	if err := pauseStore.Pause(pauseCtx, windowID, command, 3, pauseTime, "tp-075-15"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if err := pauseStore.Resume(cctx, windowID, time.Now().UTC(), "tp-075-15-cleanup"); err != nil {
			t.Logf("WARN: Resume cleanup failed: %v — live window may remain paused", err)
		}
	})

	// Confirm the pause is observable through the same store the
	// resolver will use. Without this assertion, a regression that
	// silently no-oped Pause() would still produce a false-positive
	// PAUSED decision below (because SST window_state="open" alone
	// produces WindowOpen, not WindowPaused).
	if paused, err := pauseStore.IsPaused(pauseCtx, windowID); err != nil || !paused {
		t.Fatalf("post-Pause IsPaused: paused=%v err=%v — Pause did not land", paused, err)
	}

	// Build the same Policy graph the facade wires (SCOPE-075-06.1)
	// but pointed at the LIVE SQL ledger + LIVE pause store. SST
	// window_state stays "open"; the paused effective state must
	// come from the runtime pause read.
	cat, err := legacyretirement.NewConfigCatalog(legacyretirement.CatalogConfig{
		NoticeCopyPerCommand:          noticeCopy,
		PostWindowUnknownResponseCopy: closedCopy,
	})
	if err != nil {
		t.Fatalf("NewConfigCatalog: %v", err)
	}
	ledger, err := legacyretirement.NewSQLNoticeLedger(pool)
	if err != nil {
		t.Fatalf("NewSQLNoticeLedger: %v", err)
	}
	resolver, err := legacyretirement.NewWindowStateResolver(
		legacyretirement.SSTStateConfig{WindowID: windowID, WindowState: "open"},
		pauseStore,
	)
	if err != nil {
		t.Fatalf("NewWindowStateResolver: %v", err)
	}
	hasher, err := legacyretirement.NewUserBucketHasher(hmacKey)
	if err != nil {
		t.Fatalf("NewUserBucketHasher: %v", err)
	}
	pol, err := legacyretirement.NewPolicy(legacyretirement.PolicyConfig{
		Catalog:       cat,
		Ledger:        ledger,
		StateResolver: resolver,
		BucketHasher:  hasher,
		WindowID:      windowID,
		Clock:         time.Now,
	})
	if err != nil {
		t.Fatalf("NewPolicy: %v", err)
	}

	// Snapshot the user's JSONB ledger column before the turn.
	snapCtx, snapCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer snapCancel()
	var before string
	if err := pool.QueryRow(snapCtx, `
		SELECT legacy_retirement_notices::text
		  FROM assistant_conversations
		 WHERE user_id = $1 AND transport = $2`, userID, transport).Scan(&before); err != nil {
		t.Fatalf("snapshot pre-ledger: %v", err)
	}

	// Drive the Policy through the same call path the facade uses.
	turnCtx, turnCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer turnCancel()
	decision, err := pol.Handle(turnCtx, legacyretirement.AssistantTurn{
		UserID:     userID,
		Transport:  transport,
		RawText:    command + " sample input",
		ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Policy.Handle: %v", err)
	}

	// Branch assertions: paused state, legacy serving mode active,
	// notice suppressed.
	if !decision.Matched {
		t.Fatalf("Matched=false; expected true for retired %q", command)
	}
	if decision.EffectiveState != legacyretirement.WindowPaused {
		t.Fatalf("EffectiveState=%q, want %q", decision.EffectiveState, legacyretirement.WindowPaused)
	}
	if !decision.ServeNL {
		t.Fatal("ServeNL=false; paused state MUST keep legacy serving mode active")
	}
	if decision.ShowNotice {
		t.Fatal("ShowNotice=true; paused state MUST suppress new notices")
	}
	if decision.Outcome != legacyretirement.OutcomePausedSuppressed {
		t.Fatalf("Outcome=%q, want %q", decision.Outcome, legacyretirement.OutcomePausedSuppressed)
	}

	// Adversarial proof (a): the user's JSONB ledger column is
	// byte-identical to the pre-turn snapshot — no MarkShown was
	// invoked. A regression that wrote a notice in the paused
	// branch would fail here even if the decision flags above were
	// faked.
	var after string
	if err := pool.QueryRow(snapCtx, `
		SELECT legacy_retirement_notices::text
		  FROM assistant_conversations
		 WHERE user_id = $1 AND transport = $2`, userID, transport).Scan(&after); err != nil {
		t.Fatalf("snapshot post-ledger: %v", err)
	}
	if after != before {
		t.Fatalf("JSONB ledger mutated during paused turn:\n before=%s\n after =%s", before, after)
	}

	// Adversarial proof (b): the dedup query that drives the OPEN
	// branch's notice gating MUST report not-notified — proves no
	// new ledger entry for this (user, command, window) exists.
	if ok, err := ledger.HasNotified(snapCtx, userID, command, windowID); err != nil || ok {
		t.Fatalf("HasNotified post-paused-turn: ok=%v err=%v — paused branch MUST NOT mark the ledger", ok, err)
	}
}
