//go:build integration

// Spec 074 SCOPE-074-04C — TP-074-13 / SCN-074-A06.
//
// Live integration proof that an abandoned spec 068 compiler clarify
// turn captures the ORIGINAL pre-clarification prompt with provenance
// = capture-as-fallback, fallback_cause = clarify_abandoned,
// abandoned_clarification = true, AND that the adversarial sub-case —
// a user reply arriving before the timeout — clears pending_clarify
// and produces NO Idea on the sweeper's next pass.
//
// The test drives the public capturefallback.ClarifyAbandonSweeper
// against:
//   - a real Postgres pool (live disposable test stack via DATABASE_URL),
//   - a real assistantctx.PgStore for the conversation+pending_clarify
//     row round-trip and the database-side
//     ListAbandonedClarifies/ClearPendingClarify queries,
//   - a real capturefallback.NewPostgresDedupStore + the pgIdeaWriter
//     test-only writer (FK-correct artifacts rows; same pattern as
//     SCOPE-04A TP-074-12).
//
// The sweeper's RunOnce method is invoked directly so the test does
// not depend on a wall-clock ticker. The "user replied in time"
// adversarial sub-case clears pending_clarify via the same
// PgStore.ClearPendingClarify path the facade would use on the reply
// turn (the facade hook for that is also exercised by the unit/build
// of the facade, but here we keep the SCOPE-04C contract test focused
// on the sweeper boundary).

package assistant_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
	assistantctx "github.com/smackerel/smackerel/internal/assistant/context"
)

const (
	tp074_13_AbandonTimeout = 30 * time.Second
)

// pgClarifyLister adapts assistantctx.PgStore to the
// capturefallback.AbandonedClarifyLister interface. Defined here (and
// not in either package) to keep the production packages free of
// cross-imports: the facade owns the wiring boundary, and tests own
// the adapter shape.
type pgClarifyLister struct {
	store *assistantctx.PgStore
}

func (l *pgClarifyLister) ListAbandoned(ctx context.Context, timeout time.Duration) ([]capturefallback.AbandonedClarifyRow, error) {
	rows, err := l.store.ListAbandonedClarifies(ctx, timeout)
	if err != nil {
		return nil, err
	}
	out := make([]capturefallback.AbandonedClarifyRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, capturefallback.AbandonedClarifyRow{
			UserID:          r.UserID,
			Transport:       r.Transport,
			OriginalPrompt:  r.Payload.OriginalPrompt,
			OriginalTurnID:  r.Payload.OriginalTurnID,
			ClarifyIntentID: r.Payload.ClarifyIntentID,
			EmitTime:        r.Payload.EmitTime,
		})
	}
	return out, nil
}

func (l *pgClarifyLister) Clear(ctx context.Context, userID, transport string) error {
	return l.store.ClearPendingClarify(ctx, userID, transport)
}

// seedPendingClarify writes an assistant_conversations row whose
// pending_clarify column carries the design v1 payload with
// emit_time = now - emitAge so the sweeper's "older than timeout"
// predicate selects (or rejects) the row deterministically.
//
// The INSERT goes through raw SQL rather than assistantctx.PgStore.Persist
// because spec 075 SCOPE-1 (migration 046) added a NOT NULL
// legacy_retirement_notices column without updating PgStore.Persist's
// INSERT column list. Until that gap is addressed by the owning spec
// (see route_required finding in this scope's report), tests that
// need to seed a fresh assistant_conversations row MUST populate
// legacy_retirement_notices explicitly. The empty-ledger shape used
// here matches the migration-time backfill exactly.
func seedPendingClarify(t *testing.T, pool *pgxpool.Pool, userID, transport, originalPrompt, originalTurnID, clarifyIntentID string, emitAge time.Duration) {
	t.Helper()
	emit := time.Now().UTC().Add(-emitAge)
	pc := assistantctx.PendingClarify{
		SchemaVersion:   assistantctx.PendingClarifySchemaV1,
		OriginalPrompt:  originalPrompt,
		EmitTime:        emit,
		ClarifyIntentID: clarifyIntentID,
		OriginalTurnID:  originalTurnID,
		UserID:          userID,
	}
	pcRaw, err := json.Marshal(pc)
	if err != nil {
		t.Fatalf("seedPendingClarify marshal: %v", err)
	}
	const legacyLedger = `{"schema_version":1,"window_id":"","commands":{}}`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = pool.Exec(ctx, `
		INSERT INTO assistant_conversations
		    (user_id, transport, working_context, pending_clarify, last_activity_at, schema_version, legacy_retirement_notices)
		VALUES ($1, $2, '{"turns":[]}'::jsonb, $3::jsonb, $4, 1, $5::jsonb)
	`, userID, transport, string(pcRaw), emit, legacyLedger)
	if err != nil {
		t.Fatalf("seedPendingClarify INSERT: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		if _, derr := pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1 AND transport = $2`, userID, transport); derr != nil {
			t.Logf("cleanup conversation %s/%s: %v", userID, transport, derr)
		}
	})
}

func loadPendingClarify(t *testing.T, pool *pgxpool.Pool, userID, transport string) *assistantctx.PendingClarify {
	t.Helper()
	store := assistantctx.NewPgStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conv, found, err := store.Load(ctx, userID, transport)
	if err != nil {
		t.Fatalf("Load conversation: %v", err)
	}
	if !found {
		return nil
	}
	return conv.PendingClarify
}

func countCapturePolicyRowsByCause(t *testing.T, pool *pgxpool.Pool, userID string, cause string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var n int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		  FROM artifact_capture_policy
		 WHERE user_id = $1
		   AND fallback_cause = $2
		   AND abandoned_clarification = TRUE
	`, userID, cause).Scan(&n)
	if err != nil {
		t.Fatalf("countCapturePolicyRowsByCause: %v", err)
	}
	return n
}

// TestCaptureFallbackPolicy_TP_074_13_ClarifyAbandoned — SCN-074-A06.
//
// Primary: seed an abandoned clarify whose emit_time is well past the
// configured timeout, invoke ClarifyAbandonSweeper.RunOnce, assert
// exactly one artifact_capture_policy row exists for this user with
// fallback_cause=clarify_abandoned and abandoned_clarification=true,
// AND pending_clarify is now NULL on the conversation row.
func TestCaptureFallbackPolicy_TP_074_13_ClarifyAbandoned(t *testing.T) {
	pool := openScope2Pool(t)

	stamp := time.Now().UnixNano()
	userID := fmt.Sprintf("scope074-04c-user-%d", stamp)
	transport := "telegram"
	originalPrompt := "what's the weather in springfield"
	originalTurnID := fmt.Sprintf("tg:orig-%d", stamp)
	clarifyIntentID := fmt.Sprintf("trace-clarify-%d", stamp)

	// Seed an abandoned clarify (emitted twice the timeout ago).
	seedPendingClarify(t, pool, userID, transport, originalPrompt, originalTurnID, clarifyIntentID, 2*tp074_13_AbandonTimeout)

	// Build the policy + sweeper against live Postgres.
	policy, _ := newScope4APolicy(t, pool, fmt.Sprintf("scope074-04c-art-%d", stamp))
	lister := &pgClarifyLister{store: assistantctx.NewPgStore(pool)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sweeper, err := capturefallback.NewClarifyAbandonSweeper(lister, policy, tp074_13_AbandonTimeout, logger)
	if err != nil {
		t.Fatalf("NewClarifyAbandonSweeper: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	captured, failed, err := sweeper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if captured != 1 {
		t.Errorf("captured = %d, want 1", captured)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}

	// Live-Postgres assertions: one capture-policy row, pending cleared.
	if got := countCapturePolicyRowsByCause(t, pool, userID, "clarify_abandoned"); got != 1 {
		t.Errorf("artifact_capture_policy rows for user=%s cause=clarify_abandoned = %d, want 1", userID, got)
	}
	if pc := loadPendingClarify(t, pool, userID, transport); pc != nil {
		t.Errorf("pending_clarify still set after sweep: %+v", pc)
	}
}

// TestCaptureFallbackPolicy_TP_074_13_ClarifyAbandoned_UserRepliesInTime
// — adversarial sub-case for SCN-074-A06.
//
// A user reply BEFORE the timeout clears pending_clarify (the facade
// does this at the top of Handle; this test drives the same
// ClearPendingClarify path the facade uses). The sweeper's next pass
// MUST capture nothing for this user — proving the sweeper does not
// fire on still-active clarifications.
//
// Adversarial property: if the sweeper ignored the "cleared" state
// and captured anyway, OR if the facade's clear path silently failed,
// the artifact-policy row count for this user would be > 0 and this
// test would trip.
func TestCaptureFallbackPolicy_TP_074_13_ClarifyAbandoned_UserRepliesInTime(t *testing.T) {
	pool := openScope2Pool(t)

	stamp := time.Now().UnixNano()
	userID := fmt.Sprintf("scope074-04c-replyuser-%d", stamp)
	transport := "telegram"
	originalPrompt := "what's the weather in springfield"
	originalTurnID := fmt.Sprintf("tg:orig-reply-%d", stamp)
	clarifyIntentID := fmt.Sprintf("trace-clarify-reply-%d", stamp)

	// Seed an abandoned-looking clarify (emit_time well past
	// timeout) so the ONLY thing that should keep the sweeper from
	// firing is the user reply path clearing pending_clarify.
	seedPendingClarify(t, pool, userID, transport, originalPrompt, originalTurnID, clarifyIntentID, 2*tp074_13_AbandonTimeout)

	// Simulate the facade's reply-side clear path (same SQL the
	// facade uses through PgStore.ClearPendingClarify, invoked by
	// the in-Handle "hadPendingClarify -> conv.PendingClarify=nil"
	// hook plus the subsequent Persist).
	store := assistantctx.NewPgStore(pool)
	clrCtx, clrCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := store.ClearPendingClarify(clrCtx, userID, transport); err != nil {
		t.Fatalf("ClearPendingClarify (reply path): %v", err)
	}
	clrCancel()

	// Sanity: the row's pending_clarify is NULL before the sweep.
	if pc := loadPendingClarify(t, pool, userID, transport); pc != nil {
		t.Fatalf("pending_clarify still set after reply-path clear: %+v", pc)
	}

	// Now run the sweeper. It MUST capture nothing for this user.
	policy, _ := newScope4APolicy(t, pool, fmt.Sprintf("scope074-04c-noart-%d", stamp))
	lister := &pgClarifyLister{store: store}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	sweeper, err := capturefallback.NewClarifyAbandonSweeper(lister, policy, tp074_13_AbandonTimeout, logger)
	if err != nil {
		t.Fatalf("NewClarifyAbandonSweeper: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	captured, failed, err := sweeper.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if failed != 0 {
		t.Errorf("failed = %d, want 0", failed)
	}
	// The sweeper may legitimately capture OTHER abandoned clarifies
	// left by parallel tests; assert per-user not global.
	if got := countCapturePolicyRowsByCause(t, pool, userID, "clarify_abandoned"); got != 0 {
		t.Errorf("artifact_capture_policy rows for user=%s (replied in time) = %d, want 0; captured(global)=%d", userID, got, captured)
	}
	if pc := loadPendingClarify(t, pool, userID, transport); pc != nil {
		t.Errorf("pending_clarify reappeared after sweep: %+v", pc)
	}
}
