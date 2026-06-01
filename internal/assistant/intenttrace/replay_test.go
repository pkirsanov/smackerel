// Spec 071 SCOPE-03 — IntentTrace replay unit tests (SCN-071-A04).
//
// These cover: happy-path match, not-found, sampled-out rejection,
// schema-invalid rejection, side-effects-blocked propagation from a
// custom DryRunner, and divergence reporting in MatchSummary.

package intenttrace

import (
	"context"
	"errors"
	"testing"
	"time"
)

func validSampledRow(t *testing.T) IntentTraceRow {
	t.Helper()
	r := NewStoreRecorder(&fakeStore{}, time.Hour)
	out, err := r.Record(context.Background(), validSampledInput())
	if err != nil {
		t.Fatalf("seed Record: %v", err)
	}
	if !out.Recorded {
		t.Fatalf("seed expected Recorded=true")
	}
	// Re-marshal via store: but use buildRow directly instead.
	row := buildRow(validSampledInput(), time.Now().UTC(), time.Hour)
	return row
}

type singleRowStore struct {
	row     IntentTraceRow
	present bool
	getErr  error
}

func (s *singleRowStore) Put(_ context.Context, _ IntentTraceRow) error { return nil }
func (s *singleRowStore) Get(_ context.Context, _ string) (IntentTraceRow, error) {
	if s.getErr != nil {
		return IntentTraceRow{}, s.getErr
	}
	if !s.present {
		return IntentTraceRow{}, errors.New("no rows")
	}
	return s.row, nil
}
func (s *singleRowStore) SweepExpired(_ context.Context, now time.Time) (SweepResult, error) {
	return SweepResult{Deleted: 0, SweptAt: now}, nil
}

func TestStoreReplay_HappyPath_PayloadDryRunner(t *testing.T) {
	row := validSampledRow(t)
	row.RouteDecision = "scenarios/weather"
	row.ToolCalls = []ToolCallSummary{{Name: "weather.lookup", Outcome: "ok"}}
	row.RedactedPayload.RouteDecision = "scenarios/weather"
	row.RedactedPayload.ToolCalls = row.ToolCalls
	replay := NewStoreReplay(&singleRowStore{row: row, present: true})
	got, err := replay.Run(context.Background(), row.TraceID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.SideEffectsInvoked {
		t.Fatalf("SideEffectsInvoked must be false on replay")
	}
	if !got.ReadOnly {
		t.Fatalf("ReadOnly must be true")
	}
	if !got.Match.RouteDecision || !got.Match.ToolCalls {
		t.Fatalf("expected full match, got %+v", got.Match)
	}
	if got.Original.RouteDecision != "scenarios/weather" || got.DryRun.RouteDecision != "scenarios/weather" {
		t.Fatalf("decision not propagated: %+v", got)
	}
	if got.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("schema version: %q", got.SchemaVersion)
	}
}

func TestStoreReplay_NotFound(t *testing.T) {
	replay := NewStoreReplay(&singleRowStore{present: false})
	_, err := replay.Run(context.Background(), "missing-id")
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("expected ErrTraceNotFound, got %v", err)
	}
}

func TestStoreReplay_EmptyTraceID(t *testing.T) {
	replay := NewStoreReplay(&singleRowStore{present: true})
	_, err := replay.Run(context.Background(), "")
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("expected ErrTraceNotFound for empty id, got %v", err)
	}
}

func TestStoreReplay_SampledOutRejected(t *testing.T) {
	row := validSampledRow(t)
	row.Sampled = false
	row.SampledOutReason = string(SampledOutDeterministic)
	row.RedactedPayload.Sampled = false
	replay := NewStoreReplay(&singleRowStore{row: row, present: true})
	_, err := replay.Run(context.Background(), row.TraceID)
	if !errors.Is(err, ErrTraceSampledOut) {
		t.Fatalf("expected ErrTraceSampledOut, got %v", err)
	}
}

func TestStoreReplay_SchemaInvalidRejected(t *testing.T) {
	row := validSampledRow(t)
	row.SchemaVersion = "v0"
	replay := NewStoreReplay(&singleRowStore{row: row, present: true})
	_, err := replay.Run(context.Background(), row.TraceID)
	if !errors.Is(err, ErrTraceSchemaInvalid) {
		t.Fatalf("expected ErrTraceSchemaInvalid for unknown schema_version, got %v", err)
	}
}

type sideEffectRunner struct{}

func (sideEffectRunner) DryRun(_ context.Context, _ IntentTraceRow) (DryRunDecision, error) {
	return DryRunDecision{}, ErrSideEffectsBlocked
}

func TestStoreReplay_DryRunnerSideEffectIsBlocked(t *testing.T) {
	row := validSampledRow(t)
	replay := &StoreReplay{Store: &singleRowStore{row: row, present: true}, Runner: sideEffectRunner{}}
	_, err := replay.Run(context.Background(), row.TraceID)
	if !errors.Is(err, ErrSideEffectsBlocked) {
		t.Fatalf("expected ErrSideEffectsBlocked, got %v", err)
	}
}

type divergentRunner struct{}

func (divergentRunner) DryRun(_ context.Context, _ IntentTraceRow) (DryRunDecision, error) {
	return DryRunDecision{RouteDecision: "scenarios/other", ToolCalls: []string{"other.tool"}}, nil
}

func TestStoreReplay_MatchSummaryReportsDivergence(t *testing.T) {
	row := validSampledRow(t)
	row.RouteDecision = "scenarios/weather"
	row.ToolCalls = []ToolCallSummary{{Name: "weather.lookup", Outcome: "ok"}}
	row.RedactedPayload.RouteDecision = "scenarios/weather"
	row.RedactedPayload.ToolCalls = row.ToolCalls
	replay := &StoreReplay{Store: &singleRowStore{row: row, present: true}, Runner: divergentRunner{}}
	got, err := replay.Run(context.Background(), row.TraceID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Match.RouteDecision || got.Match.ToolCalls {
		t.Fatalf("expected divergence reported, got %+v", got.Match)
	}
	if got.SideEffectsInvoked {
		t.Fatalf("SideEffectsInvoked must remain false even on divergence")
	}
}
