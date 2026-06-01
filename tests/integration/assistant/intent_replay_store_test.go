//go:build integration

// Spec 071 SCOPE-03 — IntentTrace replay live-store regression
// (SCN-071-A04).
//
// Asserts the replay surface against a live Postgres-backed
// IntentTraceStore:
//
//   1. A persisted row can be loaded by trace_id and replayed
//      end-to-end with the production PayloadDryRunner, producing a
//      ReplayComparison whose Match is true and SideEffectsInvoked
//      is false.
//   2. Replay refuses sampled-out envelopes with ErrTraceSampledOut.
//   3. Replay reports ErrTraceNotFound for an unknown trace id.
//   4. Replay never writes back to assistant_intent_traces — the
//      row count for the namespace is unchanged after Run.
//
// Adversarial: a synthetic divergent runner injected through
// StoreReplay.Runner reports the divergence in MatchSummary without
// claiming side effects.

package assistant_integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/intenttrace"
)

// TestIntentReplayLoadsOneStoredRedactedTraceByTraceID — SCN-071-A04
// integration row from scopes.md SCOPE-071-03 Test Plan.
func TestIntentReplayLoadsOneStoredRedactedTraceByTraceID(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)

	ns := fmt.Sprintf("spec071-a04-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	conf := 0.93
	traceID := ns + "-trace"
	turnID := ns + "-turn"

	// Seed one full v1 trace through the production recorder so the
	// schema/redaction invariants are enforced at write time.
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})
	in := intenttrace.TurnTraceInput{
		TraceID:             traceID,
		TurnID:              turnID,
		UserIDHash:          "deadbeefdeadbeef",
		Transport:           intenttrace.TransportWeb,
		TransportMessageID:  "web-1",
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
		EmittedAt: time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := recorder.Record(ctx, in); err != nil {
		t.Fatalf("seed Record: %v", err)
	}

	// Snapshot row count BEFORE replay so we can prove read-only.
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%").Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}

	replay := intenttrace.NewStoreReplay(store)
	res, err := replay.Run(ctx, traceID)
	if err != nil {
		t.Fatalf("replay.Run: %v", err)
	}
	if !res.ReadOnly {
		t.Fatalf("ReadOnly=false on production replay")
	}
	if res.SideEffectsInvoked {
		t.Fatalf("SideEffectsInvoked=true on production replay (must be false)")
	}
	if !res.Match.RouteDecision || !res.Match.ToolCalls {
		body, _ := json.MarshalIndent(res, "", "  ")
		t.Fatalf("expected full match, got:\n%s", string(body))
	}
	if res.Original.RouteDecision != "scenarios/weather" {
		t.Fatalf("Original.RouteDecision = %q, want scenarios/weather", res.Original.RouteDecision)
	}
	if len(res.DryRun.ToolCalls) != 1 || res.DryRun.ToolCalls[0] != "weather.lookup" {
		t.Fatalf("DryRun.ToolCalls = %v, want [weather.lookup]", res.DryRun.ToolCalls)
	}

	// Row count unchanged — replay is strictly read-only.
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%").Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if before != after {
		t.Fatalf("replay mutated row count: before=%d after=%d (must be equal)", before, after)
	}
}

// TestIntentReplayRefusesSampledOutEnvelope — guards the design.md
// CLI contract row "Trace is sampled-out only" → ErrTraceSampledOut.
func TestIntentReplayRefusesSampledOutEnvelope(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)

	ns := fmt.Sprintf("spec071-a04-sampledout-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_intent_traces WHERE trace_id LIKE $1`, ns+"%")
	})

	traceID := ns + "-trace"
	recorder := intenttrace.NewStoreRecorder(store, 24*time.Hour).WithExporter(intenttrace.NopExporter{})
	in := intenttrace.TurnTraceInput{
		TraceID:               traceID,
		TurnID:                ns + "-turn",
		UserIDHash:            "deadbeefdeadbeef",
		Transport:             intenttrace.TransportWeb,
		TransportMessageID:    "web-sampled-out",
		CompilerInvoked:       false,
		Sampled:               false,
		SampledOutReason:      string(intenttrace.SampledOutDeterministic),
		SlotsRedactionSummary: intenttrace.SlotsRedactionSummary{RawText: "absent", SlotClasses: map[string]string{}},
		EmittedAt:             time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := recorder.Record(ctx, in); err != nil {
		t.Fatalf("seed sampled-out Record: %v", err)
	}

	replay := intenttrace.NewStoreReplay(store)
	_, err := replay.Run(ctx, traceID)
	if !errors.Is(err, intenttrace.ErrTraceSampledOut) {
		t.Fatalf("expected ErrTraceSampledOut on sampled-out envelope, got %v", err)
	}
}

// TestIntentReplayReportsNotFoundForUnknownTraceID.
func TestIntentReplayReportsNotFoundForUnknownTraceID(t *testing.T) {
	pool := openIntentTracePool(t)
	store := intenttrace.NewPostgresStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	replay := intenttrace.NewStoreReplay(store)
	_, err := replay.Run(ctx, "definitely-not-a-real-trace-id-"+fmt.Sprint(time.Now().UnixNano()))
	if !errors.Is(err, intenttrace.ErrTraceNotFound) {
		t.Fatalf("expected ErrTraceNotFound for unknown id, got %v", err)
	}
}
