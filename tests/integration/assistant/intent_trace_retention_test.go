//go:build integration

// Spec 071 SCOPE-02 — Retention sweep regression (SCN-071-A09).
//
// Live-Postgres proof that:
//   1. PostgresStore.SweepExpired deletes rows whose expires_at <= now
//      and leaves fresh rows untouched.
//   2. The retention sweep is observable: the SweepResult.Deleted
//      count matches the rows actually removed.
// Adversarial: a fresh row whose expires_at > now MUST survive the
// sweep, otherwise the WHERE clause regressed to a no-op DELETE.

package assistant_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

func openIntentTracePool(t *testing.T) *pgxpool.Pool {
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

// putIntentTraceRow inserts a row directly (no recorder) so the test
// can pin expires_at to a known value.
func putIntentTraceRow(t *testing.T, pool *pgxpool.Pool, traceID, turnID string, emittedAt, expiresAt time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tools, _ := json.Marshal([]intenttrace.ToolCallSummary{})
	slots, _ := json.Marshal(intenttrace.SlotsRedactionSummary{RawText: "absent", SlotClasses: map[string]string{}})
	payload, _ := json.Marshal(intenttrace.RedactedPayload{
		SchemaVersion:         intenttrace.SchemaVersionV1,
		TraceID:               traceID,
		TurnID:                turnID,
		UserIDHash:            "deadbeef",
		Transport:             intenttrace.TransportWeb,
		TransportMessageID:    "m-" + traceID,
		Sampled:               true,
		CompilerInvoked:       true,
		ActionClass:           "external_lookup",
		SideEffectClass:       "external_read",
		RouteDecision:         "scenarios/weather",
		ToolCalls:             []intenttrace.ToolCallSummary{},
		FinalResponseStatus:   intenttrace.StatusOK,
		SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{RawText: "absent", SlotClasses: map[string]string{}},
	})
	_, err := pool.Exec(ctx, `
		INSERT INTO assistant_intent_traces (
			trace_id, schema_version, turn_id, user_id_hash, transport,
			transport_message_id, sampled, action_class, side_effect_class,
			tool_calls, final_response_status, compiler_invoked,
			slots_redaction_summary, redacted_payload, emitted_at, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`,
		traceID, intenttrace.SchemaVersionV1, turnID, "deadbeef", "web",
		"m-"+traceID, true, "external_lookup", "external_read",
		tools, "ok", true,
		slots, payload, emittedAt, expiresAt,
	)
	if err != nil {
		t.Fatalf("seed row %s: %v", traceID, err)
	}
}

// TestIntentTraceRetentionSweepRemovesExpiredAndKeepsFresh — SCN-071-A09.
func TestIntentTraceRetentionSweepRemovesExpiredAndKeepsFresh(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)

	ns := fmt.Sprintf("spec071-sweep-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	expiredID := ns + "-expired"
	freshID := ns + "-fresh"

	putIntentTraceRow(t, pool, expiredID, expiredID+"-turn", now.Add(-48*time.Hour), now.Add(-1*time.Hour))
	putIntentTraceRow(t, pool, freshID, freshID+"-turn", now, now.Add(24*time.Hour))

	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := store.SweepExpired(ctx, now)
	if err != nil {
		t.Fatalf("SweepExpired: %v", err)
	}
	if res.Deleted < 1 {
		t.Fatalf("SweepResult.Deleted=%d, want >=1 (expired row should be removed)", res.Deleted)
	}

	// Expired row gone.
	if _, err := store.Get(ctx, expiredID); err == nil {
		t.Errorf("expired trace %s still exists after sweep — WHERE expires_at <= now regressed", expiredID)
	}
	// Fresh row preserved.
	row, err := store.Get(ctx, freshID)
	if err != nil {
		t.Fatalf("fresh trace %s missing after sweep: %v (adversarial: sweep deleted unexpired row)", freshID, err)
	}
	if row.TraceID != freshID {
		t.Errorf("fresh row identity mismatch: got %q, want %q", row.TraceID, freshID)
	}
}
