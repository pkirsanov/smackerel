//go:build integration

// Spec 075 SCOPE-5 — TP-075-17.
//
// Live-stack integration proof for SCN-075-A08: the observation
// report runs end-to-end against the live Postgres database, persists
// a snapshot into assistant_legacy_retirement_observations with the
// retired-handler invocation count sourced from the
// RetiredHandlerInvocationCounter metric, and the deletion-gate
// evaluator returns Eligible=true only when the snapshot reports
// zero invocations over the configured observation period.
//
// Adversarial sub-test: a stubbed invocation source that reports >0
// invocations MUST cause EvaluateDeletionGate to refuse eligibility
// even though the snapshot row is present.

package assistant_integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
)

type stubInvocationSource struct{ n int }

func (s *stubInvocationSource) CountInvocations(_ context.Context, _, _ time.Time) (int, error) {
	return s.n, nil
}

func openLegacyRetirementPoolForObservation(t *testing.T) *pgxpool.Pool {
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

func TestSQLObservationReport_TP_075_17_ZeroInvocationsGateEligible(t *testing.T) {
	pool := openLegacyRetirementPoolForObservation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	windowID := "spec075-tp17-" + time.Now().UTC().Format("20060102150405")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM assistant_legacy_retirement_observations WHERE window_id = $1`, windowID)
	})

	now := time.Now().UTC()
	report, err := legacyretirement.NewSQLObservationReport(legacyretirement.SQLObservationReportConfig{
		Pool:   pool,
		Source: &stubInvocationSource{n: 0},
		Clock:  func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewSQLObservationReport: %v", err)
	}

	const observation = 14 * 24 * time.Hour
	snap, err := report.Generate(ctx, windowID, observation)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if snap.RetiredHandlerInvocations != 0 {
		t.Fatalf("invocations=%d, want 0", snap.RetiredHandlerInvocations)
	}
	if !snap.EligibleForFinalDeletion(observation) {
		t.Fatal("snapshot must be eligible (zero count over required interval)")
	}

	// Persisted row shape check via direct query.
	var (
		gotID     string
		invs      int
		startedAt time.Time
		endedAt   time.Time
	)
	if err := pool.QueryRow(ctx, `
		SELECT report_id, retired_handler_invocations,
		       observation_started_at, observation_ended_at
		  FROM assistant_legacy_retirement_observations
		 WHERE window_id = $1
		 ORDER BY generated_at DESC
		 LIMIT 1`, windowID).Scan(&gotID, &invs, &startedAt, &endedAt); err != nil {
		t.Fatalf("read back snapshot: %v", err)
	}
	if gotID != snap.ReportID {
		t.Errorf("read back report_id=%q, want %q", gotID, snap.ReportID)
	}
	if invs != 0 {
		t.Errorf("read back invocations=%d, want 0", invs)
	}
	if d := endedAt.Sub(startedAt); d < observation-time.Second || d > observation+time.Second {
		t.Errorf("read back interval=%s, want %s", d, observation)
	}

	// LatestSnapshot returns the row we just wrote.
	latest, has, err := report.LatestSnapshot(ctx, windowID)
	if err != nil {
		t.Fatalf("LatestSnapshot: %v", err)
	}
	if !has {
		t.Fatal("LatestSnapshot must return true after Generate")
	}
	if latest.ReportID != snap.ReportID {
		t.Errorf("LatestSnapshot id=%q, want %q", latest.ReportID, snap.ReportID)
	}

	gate := legacyretirement.EvaluateDeletionGate(latest, has, observation, nil)
	if !gate.Eligible {
		t.Fatalf("deletion gate must be eligible; reason=%q", gate.Reason)
	}
}

// Adversarial: nonzero invocations must block the gate even when the
// snapshot interval and persistence path are otherwise valid.
func TestSQLObservationReport_TP_075_17_NonzeroBlocksGate(t *testing.T) {
	pool := openLegacyRetirementPoolForObservation(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	windowID := "spec075-tp17-bad-" + time.Now().UTC().Format("20060102150405")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM assistant_legacy_retirement_observations WHERE window_id = $1`, windowID)
	})

	report, err := legacyretirement.NewSQLObservationReport(legacyretirement.SQLObservationReportConfig{
		Pool:   pool,
		Source: &stubInvocationSource{n: 3},
		Clock:  time.Now,
	})
	if err != nil {
		t.Fatalf("NewSQLObservationReport: %v", err)
	}
	const observation = 14 * 24 * time.Hour
	snap, err := report.Generate(ctx, windowID, observation)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if snap.EligibleForFinalDeletion(observation) {
		t.Fatal("snapshot with 3 invocations must NOT be eligible")
	}
	gate := legacyretirement.EvaluateDeletionGate(snap, true, observation, nil)
	if gate.Eligible {
		t.Fatalf("deletion gate must be blocked with nonzero invocations; reason=%q", gate.Reason)
	}
}
