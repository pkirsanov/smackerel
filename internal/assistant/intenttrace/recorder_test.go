// Spec 071 SCOPE-01 — IntentTrace recorder unit tests.
// Validates the pure builder + validator paths (no Postgres). Live
// persistence is covered by tests/integration/assistant/intent_trace_test.go.

package intenttrace

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeStore struct {
	got []IntentTraceRow
	err error
}

func (f *fakeStore) Put(_ context.Context, row IntentTraceRow) error {
	if f.err != nil {
		return f.err
	}
	f.got = append(f.got, row)
	return nil
}

func (f *fakeStore) Get(_ context.Context, _ string) (IntentTraceRow, error) {
	return IntentTraceRow{}, errors.New("not implemented for fake")
}

func (f *fakeStore) SweepExpired(_ context.Context, now time.Time) (SweepResult, error) {
	return SweepResult{SweptAt: now}, nil
}

func validSampledInput() TurnTraceInput {
	c := 0.9
	return TurnTraceInput{
		TraceID:             "trace-1",
		TurnID:              "turn-1",
		UserIDHash:          "deadbeef",
		Transport:           TransportTelegram,
		TransportMessageID:  "m-1",
		CompilerInvoked:     true,
		ActionClass:         "external_lookup",
		SideEffectClass:     "external_read",
		Confidence:          &c,
		RouteDecision:       "scenarios/weather",
		ToolCalls:           []ToolCallSummary{{Name: "weather.lookup", ArgumentsRedacted: true, Outcome: "ok"}},
		FinalResponseStatus: StatusCheckingWeather,
		Sampled:             true,
		SlotsRedactionSummary: SlotsRedactionSummary{
			RawText:     "absent",
			SlotClasses: map[string]string{"location": "safe"},
		},
		EmittedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestStoreRecorder_RecordsSampledRow(t *testing.T) {
	fs := &fakeStore{}
	r := NewStoreRecorder(fs, 14*24*time.Hour)
	res, err := r.Record(context.Background(), validSampledInput())
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !res.Recorded || res.TraceID != "trace-1" || res.PayloadHash == "" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if len(fs.got) != 1 {
		t.Fatalf("expected exactly 1 row written, got %d", len(fs.got))
	}
	row := fs.got[0]
	if row.SchemaVersion != SchemaVersionV1 {
		t.Errorf("schema_version drift: %q", row.SchemaVersion)
	}
	if row.ExpiresAt.Sub(row.EmittedAt) != 14*24*time.Hour {
		t.Errorf("ExpiresAt offset mismatch: %s", row.ExpiresAt.Sub(row.EmittedAt))
	}
	if row.RedactedPayload.SchemaVersion != SchemaVersionV1 {
		t.Errorf("payload schema drift")
	}
}

func TestStoreRecorder_SampledOutEnvelope(t *testing.T) {
	fs := &fakeStore{}
	r := NewStoreRecorder(fs, time.Hour)
	in := TurnTraceInput{
		TraceID:            "trace-2",
		TurnID:             "turn-2",
		UserIDHash:         "deadbeef",
		Transport:          TransportWeb,
		TransportMessageID: "m-2",
		Sampled:            false,
		SampledOutReason:   string(SampledOutDeterministic),
		EmittedAt:          time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
	}
	res, err := r.Record(context.Background(), in)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !res.Recorded || res.WasSampled {
		t.Fatalf("expected envelope recorded with WasSampled=false, got %+v", res)
	}
	if len(fs.got) != 1 {
		t.Fatalf("expected 1 envelope row, got %d", len(fs.got))
	}
	if fs.got[0].Sampled {
		t.Fatalf("expected sampled=false on envelope row")
	}
	if len(fs.got[0].ToolCalls) != 0 {
		t.Fatalf("envelope must not carry tool calls")
	}
}

func TestStoreRecorder_ValidationFailures(t *testing.T) {
	fs := &fakeStore{}
	r := NewStoreRecorder(fs, time.Hour)
	cases := []struct {
		name string
		mut  func(*TurnTraceInput)
	}{
		{"missing trace id", func(in *TurnTraceInput) { in.TraceID = "" }},
		{"missing turn id", func(in *TurnTraceInput) { in.TurnID = "" }},
		{"unknown transport", func(in *TurnTraceInput) { in.Transport = Transport("smoke-signal") }},
		{"missing action class", func(in *TurnTraceInput) { in.ActionClass = "" }},
		{"unknown status", func(in *TurnTraceInput) { in.FinalResponseStatus = FinalResponseStatus("bogus") }},
		{"missing redaction summary raw_text", func(in *TurnTraceInput) {
			in.SlotsRedactionSummary = SlotsRedactionSummary{}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validSampledInput()
			tc.mut(&in)
			if _, err := r.Record(context.Background(), in); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
	if len(fs.got) != 0 {
		t.Fatalf("validation failures must not write rows; got %d", len(fs.got))
	}
}

func TestNopRecorder_NoStoreWrite(t *testing.T) {
	res, err := NopRecorder{}.Record(context.Background(), validSampledInput())
	if err != nil {
		t.Fatalf("nop record: %v", err)
	}
	if res.Recorded {
		t.Fatalf("NopRecorder must not claim Recorded=true")
	}
}
