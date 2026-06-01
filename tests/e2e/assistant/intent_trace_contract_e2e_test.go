//go:build e2e

// Spec 071 SCOPE-01 — IntentTrace v1 contract E2E (SCN-071-A01).
//
// Drives a live recorded IntentTrace through the test-stack
// Postgres and verifies that the persisted row + RedactedPayload
// satisfy the v1 wire contract: SchemaVersion="v1", every required
// field populated, closed-vocabulary transport/status values, and
// a non-empty payload hash. The row is read back through the
// PostgresStore (the same surface the replay CLI uses), so this
// is the canonical persistent-regression anchor for "exactly one
// v1 IntentTrace per compiled turn surfaced through the live
// contract".
//
// Skip policy mirrors intent_replay_test.go: legitimate "no live
// stack" skip when both SMACKEREL_TEST_ENV_FILE and DATABASE_URL
// are unset; partial environment is a wiring bug and fails loud.

package assistant_e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

func intentTraceContractResolveLiveEnv(t *testing.T) (envFile, dbURL string) {
	t.Helper()
	envFile = os.Getenv("SMACKEREL_TEST_ENV_FILE")
	dbURL = os.Getenv("DATABASE_URL")
	if envFile == "" && dbURL == "" {
		t.Skip("e2e: neither SMACKEREL_TEST_ENV_FILE nor DATABASE_URL set — live test stack not available")
	}
	if envFile == "" || dbURL == "" {
		t.Fatalf("e2e: partial test env — SMACKEREL_TEST_ENV_FILE=%q DATABASE_URL=%q (must be both set or both unset)", envFile, dbURL)
	}
	return envFile, dbURL
}

// TestIntentTraceContractE2E_LiveCompiledTurnExposesV1Contract is
// the SCN-071-A01 e2e-api row: persist one full sampled trace via
// the live recorder, read it back via the PostgresStore, and assert
// the v1 contract invariants are intact end-to-end.
func TestIntentTraceContractE2E_LiveCompiledTurnExposesV1Contract(t *testing.T) {
	_, dbURL := intentTraceContractResolveLiveEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	ns := fmt.Sprintf("spec071-a01-contract-%d", time.Now().UnixNano())
	traceID := ns + "-trace"
	turnID := ns + "-turn"
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})

	conf := 0.88
	in := intenttrace.TurnTraceInput{
		TraceID:            traceID,
		TurnID:             turnID,
		UserIDHash:         "0123456789abcdef",
		Transport:          intenttrace.TransportTelegram,
		TransportMessageID: "e2e-a01",
		CompilerInvoked:    true,
		Sampled:            true,
		ActionClass:        "external_lookup",
		SideEffectClass:    "external_read",
		Confidence:         &conf,
		RouteDecision:      "scenarios/weather",
		ToolCalls: []intenttrace.ToolCallSummary{
			{Name: "weather.lookup", ArgumentsRedacted: true, Outcome: "ok"},
		},
		FinalResponseStatus: intenttrace.StatusCheckingWeather,
		SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
			RawText:     "absent",
			SlotClasses: map[string]string{"location": "safe"},
		},
		EmittedAt: time.Now().UTC(),
	}
	res, err := recorder.Record(ctx, in)
	if err != nil {
		t.Fatalf("recorder.Record: %v", err)
	}
	if !res.Recorded || !res.WasSampled {
		t.Fatalf("Record result: recorded=%v sampled=%v (both must be true)", res.Recorded, res.WasSampled)
	}
	if res.PayloadHash == "" {
		t.Fatal("Record result: PayloadHash empty — v1 contract requires a stable canonical-JSON hash")
	}

	row, err := store.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}

	// v1 wire-contract invariants. Every assertion below is the
	// persistent regression anchor for SCN-071-A01.
	if row.SchemaVersion != intenttrace.SchemaVersionV1 {
		t.Fatalf("row.SchemaVersion=%q want %q", row.SchemaVersion, intenttrace.SchemaVersionV1)
	}
	if row.RedactedPayload.SchemaVersion != intenttrace.SchemaVersionV1 {
		t.Fatalf("payload.SchemaVersion=%q want %q", row.RedactedPayload.SchemaVersion, intenttrace.SchemaVersionV1)
	}
	if row.TraceID != traceID || row.TurnID != turnID {
		t.Fatalf("identifiers drifted: row=(%s,%s) want=(%s,%s)", row.TraceID, row.TurnID, traceID, turnID)
	}
	if row.RedactedPayload.TraceID != traceID || row.RedactedPayload.TurnID != turnID {
		t.Fatalf("payload identifiers drifted: payload=(%s,%s) want=(%s,%s)", row.RedactedPayload.TraceID, row.RedactedPayload.TurnID, traceID, turnID)
	}
	if !row.CompilerInvoked || !row.RedactedPayload.CompilerInvoked {
		t.Fatal("CompilerInvoked must be true on both row and payload for a compiled turn")
	}
	if !row.Sampled || !row.RedactedPayload.Sampled {
		t.Fatal("Sampled must be true on both row and payload for the full-trace path")
	}
	if row.ActionClass != "external_lookup" || row.RedactedPayload.ActionClass != "external_lookup" {
		t.Fatalf("ActionClass drift: row=%q payload=%q", row.ActionClass, row.RedactedPayload.ActionClass)
	}
	if row.SideEffectClass != "external_read" || row.RedactedPayload.SideEffectClass != "external_read" {
		t.Fatalf("SideEffectClass drift: row=%q payload=%q", row.SideEffectClass, row.RedactedPayload.SideEffectClass)
	}
	if row.RouteDecision != "scenarios/weather" {
		t.Fatalf("RouteDecision=%q want scenarios/weather", row.RouteDecision)
	}
	if len(row.ToolCalls) != 1 || row.ToolCalls[0].Name != "weather.lookup" || !row.ToolCalls[0].ArgumentsRedacted {
		t.Fatalf("ToolCalls did not round-trip: %+v", row.ToolCalls)
	}
	if row.FinalResponseStatus != intenttrace.StatusCheckingWeather {
		t.Fatalf("FinalResponseStatus=%q want %q", row.FinalResponseStatus, intenttrace.StatusCheckingWeather)
	}
	if row.Transport != intenttrace.TransportTelegram {
		t.Fatalf("Transport=%q want telegram", row.Transport)
	}
	if row.EmittedAt.IsZero() || row.ExpiresAt.IsZero() || !row.ExpiresAt.After(row.EmittedAt) {
		t.Fatalf("EmittedAt/ExpiresAt malformed: emitted=%s expires=%s", row.EmittedAt, row.ExpiresAt)
	}
}
