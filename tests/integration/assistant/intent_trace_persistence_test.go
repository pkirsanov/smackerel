//go:build integration

// Spec 071 SCOPE-02 — Recorder + store + export persistence regression.
//
// Live-Postgres proof for SCN-071-A01, SCN-071-A02, and SCN-071-A03:
//
//   SCN-071-A01 — A successful Record() call persists exactly one
//   v1 IntentTrace row with the full canonical contract.
//
//   SCN-071-A02 — A sampled-out Record() call (sampler returns false)
//   still persists exactly one minimal envelope row, and the total
//   number of persisted rows equals sampled + sampled-out (no
//   under-count regression).
//
//   SCN-071-A03 — A persisted row with source_policy.PersistRawText
//   == false carries slots_redaction_summary and NO raw slot VALUE
//   anywhere in the JSONB payload. Adversarial: an injected secret
//   value MUST NOT appear in the round-tripped row text.

package assistant_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

// TestIntentTracePersistsExactlyOneV1RowPerRecordCall — SCN-071-A01.
func TestIntentTracePersistsExactlyOneV1RowPerRecordCall(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})

	ns := fmt.Sprintf("spec071-a01-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	conf := 0.92
	traceID := ns + "-trace"
	turnID := ns + "-turn"
	emit := time.Now().UTC()
	in := intenttrace.TurnTraceInput{
		TraceID:             traceID,
		TurnID:              turnID,
		UserIDHash:          "deadbeefdeadbeef",
		Transport:           intenttrace.TransportTelegram,
		TransportMessageID:  "tg-1",
		CompilerInvoked:     true,
		Sampled:             true,
		ActionClass:         "external_lookup",
		SideEffectClass:     "external_read",
		Confidence:          &conf,
		RouteDecision:       "scenarios/weather",
		ToolCalls:           []intenttrace.ToolCallSummary{{Name: "weather.lookup", ArgumentsRedacted: true, Outcome: "ok"}},
		FinalResponseStatus: intenttrace.StatusCheckingWeather,
		SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
			RawText:     "absent",
			SlotClasses: map[string]string{"location": "safe"},
		},
		EmittedAt: emit,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := recorder.Record(ctx, in)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !res.Recorded || res.TraceID != traceID || res.PayloadHash == "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	var rowCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id = $1`, traceID).Scan(&rowCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("persisted row count = %d, want exactly 1 (SCN-071-A01 invariant)", rowCount)
	}

	row, err := store.Get(ctx, traceID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if row.SchemaVersion != intenttrace.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", row.SchemaVersion, intenttrace.SchemaVersionV1)
	}
	if !row.Sampled {
		t.Errorf("Sampled=false on a sampled input; row drifted")
	}
	if !row.CompilerInvoked {
		t.Errorf("CompilerInvoked=false; full trace requires compiler invocation")
	}
	if row.ActionClass != "external_lookup" || row.RouteDecision != "scenarios/weather" {
		t.Errorf("identity drift: action_class=%q route_decision=%q", row.ActionClass, row.RouteDecision)
	}
	if row.RedactedPayload.SchemaVersion != intenttrace.SchemaVersionV1 {
		t.Errorf("payload schema drift")
	}
	// Adversarial — a second Record() with the same TurnID must fail
	// because the migration's UNIQUE INDEX on turn_id pins one-trace-per-turn.
	if _, err := recorder.Record(ctx, in); err == nil {
		t.Errorf("expected duplicate TurnID Record() to fail (UNIQUE INDEX on turn_id); got nil")
	}
}

// TestIntentTraceSampledOutPreservesTotalTurnAccounting — SCN-071-A02.
func TestIntentTraceSampledOutPreservesTotalTurnAccounting(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})

	ns := fmt.Sprintf("spec071-a02-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const sampledN, sampledOutN = 3, 4
	for i := 0; i < sampledN; i++ {
		conf := 0.8
		_, err := recorder.Record(ctx, intenttrace.TurnTraceInput{
			TraceID:             fmt.Sprintf("%s-s-%d", ns, i),
			TurnID:              fmt.Sprintf("%s-s-%d-turn", ns, i),
			UserIDHash:          "deadbeef",
			Transport:           intenttrace.TransportWeb,
			TransportMessageID:  fmt.Sprintf("web-s-%d", i),
			CompilerInvoked:     true,
			Sampled:             true,
			ActionClass:         "external_lookup",
			SideEffectClass:     "external_read",
			Confidence:          &conf,
			FinalResponseStatus: intenttrace.StatusOK,
			SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{
				RawText: "absent", SlotClasses: map[string]string{},
			},
		})
		if err != nil {
			t.Fatalf("sampled Record %d: %v", i, err)
		}
	}
	for i := 0; i < sampledOutN; i++ {
		_, err := recorder.Record(ctx, intenttrace.TurnTraceInput{
			TraceID:            fmt.Sprintf("%s-o-%d", ns, i),
			TurnID:             fmt.Sprintf("%s-o-%d-turn", ns, i),
			UserIDHash:         "deadbeef",
			Transport:          intenttrace.TransportWeb,
			TransportMessageID: fmt.Sprintf("web-o-%d", i),
			CompilerInvoked:    true,
			Sampled:            false,
			SampledOutReason:   string(intenttrace.SampledOutDeterministic),
		})
		if err != nil {
			t.Fatalf("sampled-out Record %d: %v", i, err)
		}
	}

	var sampledCount, sampledOutCount, total int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1 AND sampled = true`, ns+"%").Scan(&sampledCount); err != nil {
		t.Fatalf("count sampled: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1 AND sampled = false`, ns+"%").Scan(&sampledOutCount); err != nil {
		t.Fatalf("count sampled-out: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%").Scan(&total); err != nil {
		t.Fatalf("count total: %v", err)
	}
	if sampledCount != sampledN {
		t.Errorf("sampled rows = %d, want %d", sampledCount, sampledN)
	}
	if sampledOutCount != sampledOutN {
		t.Errorf("sampled-out rows = %d, want %d", sampledOutCount, sampledOutN)
	}
	if total != sampledN+sampledOutN {
		t.Errorf("total rows = %d, want %d (sampled-out under-counted)", total, sampledN+sampledOutN)
	}
}

// TestIntentTraceRedactionLeavesNoRawSlotValueInPayload — SCN-071-A03.
func TestIntentTraceRedactionLeavesNoRawSlotValueInPayload(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})
	redactor := intenttrace.NewDefaultRedactor()
	policy := intenttrace.NewSourcePolicy(false, []string{"phone_number"})

	ns := fmt.Sprintf("spec071-a03-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	const secretPhone = "1-555-CANARY-LEAK-7777"
	const secretText = "MY-PRIVATE-USER-TEXT-CANARY"
	red := redactor.Redact(policy, secretText, map[string]any{
		"phone_number": secretPhone,
		"location":     "palm springs",
	})
	traceID := ns + "-trace"
	turnID := ns + "-turn"
	conf := 0.9
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := recorder.Record(ctx, intenttrace.TurnTraceInput{
		TraceID:               traceID,
		TurnID:                turnID,
		UserIDHash:            "deadbeef",
		Transport:             intenttrace.TransportTelegram,
		TransportMessageID:    "tg-canary",
		CompilerInvoked:       true,
		Sampled:               true,
		ActionClass:           "external_lookup",
		SideEffectClass:       "external_read",
		Confidence:            &conf,
		FinalResponseStatus:   intenttrace.StatusOK,
		SlotsRedactionSummary: red.Summary,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	// Pull the raw JSONB back out and assert no secret appears.
	var slotsJSON, payloadJSON []byte
	if err := pool.QueryRow(ctx, `SELECT slots_redaction_summary, redacted_payload FROM assistant_intent_traces WHERE trace_id = $1`, traceID).Scan(&slotsJSON, &payloadJSON); err != nil {
		t.Fatalf("read back: %v", err)
	}
	for _, blob := range [][]byte{slotsJSON, payloadJSON} {
		s := string(blob)
		if strings.Contains(s, secretPhone) {
			t.Errorf("persisted JSONB contains raw sensitive slot value (privacy leak): %s", s)
		}
		if strings.Contains(s, secretText) {
			t.Errorf("persisted JSONB contains raw user text (privacy leak): %s", s)
		}
	}

	var summary intenttrace.SlotsRedactionSummary
	if err := json.Unmarshal(slotsJSON, &summary); err != nil {
		t.Fatalf("decode slots summary: %v", err)
	}
	if summary.SlotClasses["phone_number"] != "redacted" {
		t.Errorf("phone_number class = %q, want %q", summary.SlotClasses["phone_number"], "redacted")
	}
	if summary.SlotClasses["location"] != "safe" {
		t.Errorf("location class = %q, want %q", summary.SlotClasses["location"], "safe")
	}
	if summary.RawText != "absent" {
		t.Errorf("RawText = %q, want %q (policy.PersistRawText=false)", summary.RawText, "absent")
	}
	if summary.RedactedCount < 1 {
		t.Errorf("RedactedCount = %d, want >= 1", summary.RedactedCount)
	}
}
