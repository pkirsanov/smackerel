// observationreport_test.go — spec 075 SCOPE-5 unit tests for the
// observation report producer + deletion-gate evaluator.
//
// The SQL persister is exercised against a stub pgxObservationQuerier
// so this test runs in the unit suite. The integration test
// (tests/integration/assistant/legacy_observation_report_test.go)
// exercises the SQL boundary against the live database.
package legacyretirement

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// --- fakes --------------------------------------------------------

type fakeInvocationSource struct {
	count int
	err   error
}

func (f *fakeInvocationSource) CountInvocations(_ context.Context, _, _ time.Time) (int, error) {
	return f.count, f.err
}

type fakeObservationDB struct {
	lastSQL  string
	lastArgs []any
	execErr  error
	rowErr   error
	row      *fakeRow
}

func (f *fakeObservationDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.lastSQL = sql
	f.lastArgs = args
	return pgconn.CommandTag{}, f.execErr
}

func (f *fakeObservationDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	if f.row != nil {
		return f.row
	}
	return &fakeRow{err: f.rowErr}
}

type fakeRow struct {
	vals []any
	err  error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.vals) {
		return errors.New("fakeRow: arity mismatch")
	}
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = r.vals[i].(string)
		case *int:
			*p = r.vals[i].(int)
		case *time.Time:
			*p = r.vals[i].(time.Time)
		default:
			return errors.New("fakeRow: unsupported destination type")
		}
	}
	return nil
}

// --- snapshot eligibility ----------------------------------------

func TestObservationSnapshot_EligibleForFinalDeletion(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		s    ObservationSnapshot
		min  time.Duration
		want bool
	}{
		{
			"zero count over required interval is eligible",
			ObservationSnapshot{
				ObservationStartedAt:      start,
				ObservationEndedAt:        start.Add(14 * 24 * time.Hour),
				RetiredHandlerInvocations: 0,
			},
			14 * 24 * time.Hour,
			true,
		},
		{
			"nonzero count is never eligible",
			ObservationSnapshot{
				ObservationStartedAt:      start,
				ObservationEndedAt:        start.Add(14 * 24 * time.Hour),
				RetiredHandlerInvocations: 1,
			},
			14 * 24 * time.Hour,
			false,
		},
		{
			"interval shorter than required is not eligible",
			ObservationSnapshot{
				ObservationStartedAt:      start,
				ObservationEndedAt:        start.Add(7 * 24 * time.Hour),
				RetiredHandlerInvocations: 0,
			},
			14 * 24 * time.Hour,
			false,
		},
		{
			"end-before-start is rejected",
			ObservationSnapshot{
				ObservationStartedAt:      start,
				ObservationEndedAt:        start.Add(-time.Hour),
				RetiredHandlerInvocations: 0,
			},
			time.Hour,
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.s.EligibleForFinalDeletion(tc.min); got != tc.want {
				t.Fatalf("EligibleForFinalDeletion=%v, want %v", got, tc.want)
			}
		})
	}
}

// --- generate persistence path -----------------------------------

func TestSQLObservationReport_Generate_PersistsSnapshot(t *testing.T) {
	db := &fakeObservationDB{}
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	r := &SQLObservationReport{
		db:     db,
		source: &fakeInvocationSource{count: 0},
		clock:  func() time.Time { return now },
		newID:  func() string { return "obs-fixed" },
	}
	snap, err := r.Generate(context.Background(), "window-a", 24*time.Hour)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if snap.WindowID != "window-a" {
		t.Errorf("WindowID=%q, want window-a", snap.WindowID)
	}
	if snap.RetiredHandlerInvocations != 0 {
		t.Errorf("count=%d, want 0", snap.RetiredHandlerInvocations)
	}
	if !snap.EligibleForFinalDeletion(24 * time.Hour) {
		t.Error("expected eligible snapshot")
	}
	if !strings.Contains(db.lastSQL, "assistant_legacy_retirement_observations") {
		t.Errorf("insert target table missing in SQL: %q", db.lastSQL)
	}
	if len(db.lastArgs) != 7 {
		t.Fatalf("insert arg count=%d, want 7", len(db.lastArgs))
	}
	if db.lastArgs[0].(string) != "obs-fixed" {
		t.Errorf("report_id arg=%v, want obs-fixed", db.lastArgs[0])
	}
}

// Adversarial: a regression that silently dropped the
// "source error → return error" propagation would let the deletion
// gate advance on a missing/broken metric source.
func TestSQLObservationReport_Generate_SourceErrorPropagates(t *testing.T) {
	r := &SQLObservationReport{
		db:     &fakeObservationDB{},
		source: &fakeInvocationSource{err: errors.New("boom")},
		clock:  func() time.Time { return time.Now().UTC() },
		newID:  func() string { return "x" },
	}
	if _, err := r.Generate(context.Background(), "w", time.Hour); err == nil {
		t.Fatal("expected error when source fails")
	}
}

func TestSQLObservationReport_Generate_RejectsInvalidInput(t *testing.T) {
	r := &SQLObservationReport{
		db:     &fakeObservationDB{},
		source: &fakeInvocationSource{count: 0},
		clock:  func() time.Time { return time.Now().UTC() },
		newID:  func() string { return "x" },
	}
	if _, err := r.Generate(context.Background(), "", time.Hour); err == nil {
		t.Error("expected error for empty windowID")
	}
	if _, err := r.Generate(context.Background(), "w", 0); err == nil {
		t.Error("expected error for non-positive duration")
	}
}

func TestNewSQLObservationReport_RequiresAllFields(t *testing.T) {
	if _, err := NewSQLObservationReport(SQLObservationReportConfig{}); err == nil {
		t.Error("expected error for nil pool")
	}
	if _, err := NewSQLObservationReport(SQLObservationReportConfig{Source: &fakeInvocationSource{}}); err == nil {
		t.Error("expected error for nil pool (with source)")
	}
}

// --- deletion-gate evaluator -------------------------------------

func TestEvaluateDeletionGate_AllConditionsRequired(t *testing.T) {
	start := time.Now().UTC().Add(-15 * 24 * time.Hour)
	end := time.Now().UTC()
	good := ObservationSnapshot{
		ObservationStartedAt:      start,
		ObservationEndedAt:        end,
		RetiredHandlerInvocations: 0,
	}
	min := 14 * 24 * time.Hour

	t.Run("no snapshot blocks", func(t *testing.T) {
		r := EvaluateDeletionGate(ObservationSnapshot{}, false, min, nil)
		if r.Eligible {
			t.Fatal("must block without snapshot")
		}
	})
	t.Run("nonzero invocations block", func(t *testing.T) {
		bad := good
		bad.RetiredHandlerInvocations = 2
		r := EvaluateDeletionGate(bad, true, min, nil)
		if r.Eligible {
			t.Fatal("must block when invocations > 0")
		}
		if !strings.Contains(r.Reason, "2") {
			t.Errorf("reason should mention count, got %q", r.Reason)
		}
	})
	t.Run("short interval blocks", func(t *testing.T) {
		shortSnap := good
		shortSnap.ObservationStartedAt = end.Add(-time.Hour)
		r := EvaluateDeletionGate(shortSnap, true, min, nil)
		if r.Eligible {
			t.Fatal("must block on short observation interval")
		}
	})
	t.Run("stale references block", func(t *testing.T) {
		r := EvaluateDeletionGate(good, true, min, []string{"x/y.go:42"})
		if r.Eligible {
			t.Fatal("must block on stale references")
		}
	})
	t.Run("all clean is eligible", func(t *testing.T) {
		r := EvaluateDeletionGate(good, true, min, nil)
		if !r.Eligible {
			t.Fatalf("expected eligible, got reason=%q", r.Reason)
		}
	})
}

// PrometheusInvocationSource sanity: count is non-negative and
// reading from the default gatherer does not error.
func TestPrometheusInvocationSource_CountIsNonNegative(t *testing.T) {
	s := &PrometheusInvocationSource{}
	n, err := s.CountInvocations(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("CountInvocations: %v", err)
	}
	if n < 0 {
		t.Fatalf("count=%d must be non-negative", n)
	}
}
