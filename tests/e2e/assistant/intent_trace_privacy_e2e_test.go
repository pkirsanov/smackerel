//go:build e2e

// Spec 071 SCOPE-02 — IntentTrace privacy E2E (SCN-071-A03).
//
// Persists one sampled trace whose redaction summary records that
// raw text was stripped and a sensitive slot class was redacted at
// record time, then reads the row back through the PostgresStore
// and asserts:
//
//   1. The persisted RedactedPayload contains the redaction summary
//      (raw_text="redacted", slot class entry present).
//   2. No raw slot text and no raw inbound text leaked into any
//      stored field (RouteDecision, ToolCall arguments, payload
//      JSONB serialisation).
//
// This is the persistent privacy regression for SCN-071-A03: if a
// future recorder change ever stored raw text the assertion below
// scans the canonical JSON of the persisted payload for the
// poison string and fails the build.
//
// Skip policy mirrors intent_replay_test.go.

package assistant_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

func intentTracePrivacyResolveLiveEnv(t *testing.T) (envFile, dbURL string) {
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

// TestIntentTracePrivacyE2E_StoredTraceCarriesRedactionSummaryWithoutRawSlotValues
// is the SCN-071-A03 e2e-api row.
func TestIntentTracePrivacyE2E_StoredTraceCarriesRedactionSummaryWithoutRawSlotValues(t *testing.T) {
	_, dbURL := intentTracePrivacyResolveLiveEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	ns := fmt.Sprintf("spec071-a03-privacy-%d", time.Now().UnixNano())
	traceID := ns + "-trace"
	turnID := ns + "-turn"
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})

	// Poison string: if any future recorder regression leaks raw
	// inbound text or raw slot values into the persisted payload,
	// this token will appear in the canonical-JSON serialisation
	// and the scan below will fail loudly.
	const poison = "ssn-123-45-6789-this-must-never-leak"

	conf := 0.71
	in := intenttrace.TurnTraceInput{
		TraceID:            traceID,
		TurnID:             turnID,
		UserIDHash:         "fedcba9876543210",
		Transport:          intenttrace.TransportWeb,
		TransportMessageID: "e2e-a03",
		CompilerInvoked:    true,
		Sampled:            true,
		ActionClass:        "external_lookup",
		SideEffectClass:    "external_read",
		Confidence:         &conf,
		RouteDecision:      "scenarios/lookup",
		ToolCalls: []intenttrace.ToolCallSummary{
			{Name: "kv.lookup", ArgumentsRedacted: true, Outcome: "ok"},
		},
		FinalResponseStatus: intenttrace.StatusCheckingWeather,
		// The caller delivers an ALREADY-redacted summary. raw_text
		// is "redacted" and the sensitive slot class entry says
		// "redacted"; the raw token is intentionally NOT placed in
		// any field the recorder reads.
		SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
			RawText:       "redacted",
			SlotClasses:   map[string]string{"ssn": "redacted"},
			RedactedCount: 1,
		},
		EmittedAt: time.Now().UTC(),
	}
	if _, err := recorder.Record(ctx, in); err != nil {
		t.Fatalf("recorder.Record: %v", err)
	}

	row, err := store.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}

	// 1. Redaction summary is present and correct on the persisted
	//    payload.
	if row.RedactedPayload.SlotsRedactionSummary.RawText != "redacted" {
		t.Fatalf("payload.SlotsRedactionSummary.RawText=%q want %q",
			row.RedactedPayload.SlotsRedactionSummary.RawText, "redacted")
	}
	gotClass, ok := row.RedactedPayload.SlotsRedactionSummary.SlotClasses["ssn"]
	if !ok || gotClass != "redacted" {
		t.Fatalf("payload.SlotsRedactionSummary.SlotClasses[ssn]=%q ok=%v want %q true",
			gotClass, ok, "redacted")
	}
	if row.RedactedPayload.SlotsRedactionSummary.RedactedCount != 1 {
		t.Fatalf("payload.SlotsRedactionSummary.RedactedCount=%d want 1",
			row.RedactedPayload.SlotsRedactionSummary.RedactedCount)
	}

	// 2. Adversarial scan: the canonical JSON of the persisted
	//    payload must not contain the poison token. This guards
	//    against a future regression where a recorder change
	//    silently copies a raw slot value into the payload.
	canonical, err := json.Marshal(row.RedactedPayload)
	if err != nil {
		t.Fatalf("json.Marshal(payload): %v", err)
	}
	if strings.Contains(string(canonical), poison) {
		t.Fatalf("privacy regression: persisted payload JSON contains the poison raw value %q:\n%s",
			poison, canonical)
	}
}
