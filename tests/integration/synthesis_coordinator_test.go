//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/oklog/ulid/v2"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-03 — T004-02-SCHED / T004-03-RETRY / T004-06-LIFECYCLE.
//
// Coordination is a database property. A process-local mutex would satisfy a
// single-process test and fail the moment a second scheduler or an operator
// retry arrived, which is exactly the case these cover.

func synthesisClaimKey(suffix string) intelligence.SynthesisRunKey {
	return intelligence.SynthesisRunKey{
		Cadence:       intelligence.CadenceDaily,
		Principal:     "claim-" + suffix,
		WindowStart:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		PolicyVersion: "synthesis/v1",
		SourceIDs:     []string{"art-a", "art-b"},
	}
}

func synthesisCoordinator(t *testing.T, holder string) (*intelligence.SynthesisCoordinator, *intelligence.SynthesisPersistence) {
	t.Helper()
	pool := synthesisTestPool(t)
	t.Cleanup(pool.Close)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	policy := intelligence.SynthesisRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		LeaseTTL:     200 * time.Millisecond,
	}
	coord, err := intelligence.NewSynthesisCoordinator(persistence, policy, holder)
	if err != nil {
		t.Fatalf("construct coordinator: %v", err)
	}
	return coord, persistence
}

// SCN-004-004-02. Two holders, standing in for a scheduled trigger and an
// operator retry in separate processes, race for the same window. Exactly one
// may claim it, and the loser must be told rather than silently proceeding.
func TestSynthesisCoordinator_SecondHolderCannotClaimSameWindow(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	first, _ := synthesisCoordinator(t, "scheduler-process-a")
	second, _ := synthesisCoordinator(t, "operator-process-b")

	ctx := context.Background()
	runKey := synthesisClaimKey("contended")
	key := runKey.LogicalKey()
	now := time.Now().UTC()

	if err := first.ClaimWindow(ctx, runKey, now); err != nil {
		t.Fatalf("first holder must win an uncontended claim: %v", err)
	}

	err := second.ClaimWindow(ctx, runKey, now)
	if !errors.Is(err, intelligence.ErrRunClaimedElsewhere) {
		t.Fatalf("second holder got %v, want ErrRunClaimedElsewhere; a claim that silently succeeds twice produces duplicate work", err)
	}

	// The claim is a row, not process memory. That is what makes it survive a
	// restart and hold across processes.
	var holder string
	var state string
	if err := pool.QueryRow(ctx,
		`SELECT lease_holder, state FROM synthesis_runs WHERE logical_key = $1`, key).Scan(&holder, &state); err != nil {
		t.Fatalf("read claim: %v", err)
	}
	if holder != "scheduler-process-a" {
		t.Fatalf("lease holder %q, want scheduler-process-a", holder)
	}
	if state != "running" {
		t.Fatalf("claimed run state %q, want running", state)
	}
}

// The same holder re-entering its own window is a restart, not contention.
func TestSynthesisCoordinator_SameHolderMayReclaimItsOwnLease(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	coord, _ := synthesisCoordinator(t, "scheduler-process-a")
	ctx := context.Background()
	runKey := synthesisClaimKey("reentrant")
	now := time.Now().UTC()

	if err := coord.ClaimWindow(ctx, runKey, now); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := coord.ClaimWindow(ctx, runKey, now.Add(time.Millisecond)); err != nil {
		t.Fatalf("a holder must be able to re-enter its own lease: %v", err)
	}
}

// A holder that died mid-run must not lock the window forever. Once its lease
// expires the work becomes claimable again.
func TestSynthesisCoordinator_ExpiredLeaseIsReclaimable(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	dead, _ := synthesisCoordinator(t, "crashed-process")
	live, _ := synthesisCoordinator(t, "recovery-process")

	ctx := context.Background()
	runKey := synthesisClaimKey("abandoned")
	start := time.Now().UTC()

	if err := dead.ClaimWindow(ctx, runKey, start); err != nil {
		t.Fatalf("initial claim: %v", err)
	}

	// Still held while the lease is live -- the reclaim must be driven by
	// expiry, not by merely asking twice.
	if err := live.ClaimWindow(ctx, runKey, start); !errors.Is(err, intelligence.ErrRunClaimedElsewhere) {
		t.Fatalf("a live lease must block another holder, got %v", err)
	}

	afterExpiry := start.Add(time.Second)
	reclaimed, err := live.ReclaimExpiredLeases(ctx, afterExpiry)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if reclaimed != 1 {
		t.Fatalf("reclaimed %d leases, want 1", reclaimed)
	}
	if err := live.ClaimWindow(ctx, runKey, afterExpiry); err != nil {
		t.Fatalf("an expired lease must be claimable: %v", err)
	}
}

// SCN-004-004-C18. Startup turns abandoned execution into immutable failure
// facts and refuses to certify content that cannot pass the production reader.
func TestSynthesisStartupReconciliation_ExpiresRunningAndRefusesUnverifiedCommit(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	coord, persistence := synthesisCoordinator(t, "startup-reconciler")
	ctx := context.Background()
	observedAt := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	actor := "claim-startup-reconciliation"

	expiredKey := synthesisClaimKey("startup-expired")
	expiredKey.Principal = actor
	expiredKey.SourceIDs = []string{"startup-expired-source"}
	expiredAttempt, err := persistence.StartAttempt(
		ctx,
		expiredKey,
		intelligence.TriggerScheduled,
		"crashed-before-content",
		time.Minute,
		observedAt.Add(-2*time.Minute),
	)
	if err != nil {
		t.Fatalf("start expired attempt: %v", err)
	}

	unverifiedKey := synthesisClaimKey("startup-unverified")
	unverifiedKey.Principal = actor
	unverifiedKey.SourceIDs = []string{"startup-unverified-source"}
	unverifiedAttempt, err := persistence.StartAttempt(
		ctx,
		unverifiedKey,
		intelligence.TriggerScheduled,
		"crashed-after-content",
		time.Hour,
		observedAt.Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("start unverified attempt: %v", err)
	}
	unverifiedOutputID := ulid.Make().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_outputs (
			id, run_id, output_kind, insight_count, citation_count,
			evaluated_artifact_count, created_at, principal, cadence,
			window_start, window_end, lifecycle_state
		) VALUES ($1, $2, 'quiet', 0, 0, 0, $3, $4, $5, $6, $7, 'current')
	`, unverifiedOutputID, unverifiedAttempt.RunID, observedAt.Add(-30*time.Second),
		unverifiedKey.Principal, string(unverifiedKey.Cadence),
		unverifiedKey.WindowStart, unverifiedKey.WindowEnd); err != nil {
		t.Fatalf("seed crash-after-content output: %v", err)
	}
	if _, err := persistence.ReadAggregate(ctx, unverifiedOutputID); err != nil {
		t.Fatalf("production reader must be able to read the unverified commit: %v", err)
	}

	summary, err := coord.ReconcileStartup(ctx, actor, intelligence.CadenceDaily, observedAt)
	if err != nil {
		t.Fatalf("reconcile startup: %v", err)
	}
	if summary.ExpiredAttempts != 1 {
		t.Fatalf("expired attempts = %d, want 1", summary.ExpiredAttempts)
	}
	if summary.ReadbackFailures != 1 {
		t.Fatalf("read-back failures = %d, want 1", summary.ReadbackFailures)
	}

	assertTerminal := func(runID string, attemptNo int, wantEvent, wantState string) {
		t.Helper()
		var eventType, attemptState string
		if err := pool.QueryRow(ctx, `
			SELECT e.event_type, a.state
			FROM synthesis_run_events e
			JOIN synthesis_run_attempts a
			  ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
			WHERE e.run_id = $1 AND e.attempt_no = $2
			  AND e.event_type IN ('persisted', 'quiet', 'partial', 'recovered',
				'failed', 'readback_failed')
		`, runID, attemptNo).Scan(&eventType, &attemptState); err != nil {
			t.Fatalf("read terminal event for %s/%d: %v", runID, attemptNo, err)
		}
		if eventType != wantEvent || attemptState != wantState {
			t.Fatalf("terminal event/state = %s/%s, want %s/%s", eventType, attemptState, wantEvent, wantState)
		}
	}
	assertTerminal(expiredAttempt.RunID, expiredAttempt.AttemptNo, "failed", "failed")
	assertTerminal(unverifiedAttempt.RunID, unverifiedAttempt.AttemptNo, "readback_failed", "readback_failed")

	var inventedSuccesses int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM synthesis_run_events
		WHERE run_id IN ($1, $2)
		  AND event_type IN ('persisted', 'quiet', 'partial', 'recovered')
	`, expiredAttempt.RunID, unverifiedAttempt.RunID).Scan(&inventedSuccesses); err != nil {
		t.Fatalf("count invented success events: %v", err)
	}
	if inventedSuccesses != 0 {
		t.Fatalf("startup reconciliation invented %d success event(s), want 0", inventedSuccesses)
	}
}

// SCN-004-004-03. Bounded retries exhaust, every attempt is audited, and no
// output is produced.
func TestSynthesisCoordinator_ExhaustedRetriesLeaveNoOutput(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	coord, _ := synthesisCoordinator(t, "scheduler-process-a")
	ctx := context.Background()
	runKey := synthesisClaimKey("exhausted")
	key := runKey.LogicalKey()

	if err := coord.ClaimWindow(ctx, runKey, time.Now().UTC()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// A serialization failure is the canonical transient: retrying it is the
	// correct response, which is what makes it exhaust rather than stop at one.
	transient := fmt.Errorf("write conflict: %w", &pgconn.PgError{
		Code:    "40001",
		Message: "could not serialize access due to read/write dependencies among transactions",
	})
	var calls int
	err := coord.RunWithRetry(ctx, key, func(context.Context) error {
		calls++
		return transient
	})
	if err == nil {
		t.Fatal("exhausted retries must surface an error, not a silent success")
	}
	if calls != 3 {
		t.Fatalf("operation ran %d time(s), want 3 (the configured budget)", calls)
	}

	// Every attempt is auditable. Without this, an operator reading the ledger
	// would see one failure where three occurred.
	var failed int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_run_attempts
		WHERE logical_key = $1 AND outcome = 'failed'`, key).Scan(&failed); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if failed != 3 {
		t.Fatalf("recorded %d failed attempts, want 3", failed)
	}

	// No content escaped. A partial output here would be worse than the failure.
	for _, table := range []string{"synthesis_outputs", "synthesis_output_insights", "synthesis_citations"} {
		if n := countSynthesisRows(t, pool, table); n != 0 {
			t.Fatalf("%s holds %d row(s) after exhausted retries; nothing may be delivered", table, n)
		}
	}
}

// A terminal failure must NOT consume the retry budget. Retrying a rejected
// candidate cannot change the outcome and only delays the alert.
func TestSynthesisCoordinator_TerminalFailureIsNotRetried(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	coord, _ := synthesisCoordinator(t, "scheduler-process-a")
	ctx := context.Background()
	runKey := synthesisClaimKey("terminal")
	key := runKey.LogicalKey()

	if err := coord.ClaimWindow(ctx, runKey, time.Now().UTC()); err != nil {
		t.Fatalf("claim: %v", err)
	}

	terminal := &intelligence.SynthesisValidationError{
		Code:   intelligence.FailureMissingCitation,
		Detail: "insight has no citations",
	}
	var calls int
	if err := coord.RunWithRetry(ctx, key, func(context.Context) error {
		calls++
		return terminal
	}); err == nil {
		t.Fatal("terminal failure must surface")
	}
	if calls != 1 {
		t.Fatalf("terminal failure ran %d time(s), want exactly 1; retrying a rejected candidate cannot change the answer", calls)
	}

	// The recorded class is the specific one, not a generic bucket -- an
	// operator needs to know it was a citation problem.
	var failureClass string
	if err := pool.QueryRow(ctx, `
		SELECT failure_class FROM synthesis_run_attempts
		WHERE logical_key = $1 ORDER BY recorded_at DESC LIMIT 1`, key).Scan(&failureClass); err != nil {
		t.Fatalf("read failure class: %v", err)
	}
	if failureClass != string(intelligence.FailureMissingCitation) {
		t.Fatalf("recorded failure class %q, want %q", failureClass, intelligence.FailureMissingCitation)
	}
}

// SCN-004-004-06. Lifecycle transitions never destroy audit provenance.
func TestSynthesisCoordinator_LifecycleTransitionsPreserveAudit(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	coord, persistence := synthesisCoordinator(t, "scheduler-process-a")
	ctx := context.Background()
	now := time.Now().UTC()

	cand := synthesisCompleteCandidate("operator-lifecycle")
	agg, err := persistence.Commit(ctx, cand, synthesisTestPolicy(), synthesisAuthorizedSources(), now)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	key := agg.LogicalKey

	if err := persistence.RecordAttempt(ctx, key, intelligence.AttemptSucceeded, "", ""); err != nil {
		t.Fatalf("record attempt: %v", err)
	}
	attemptsBefore := countSynthesisRows(t, pool, "synthesis_run_attempts")
	insightsBefore := countSynthesisRows(t, pool, "synthesis_output_insights")
	citationsBefore := countSynthesisRows(t, pool, "synthesis_citations")

	for _, step := range []struct {
		name string
		run  func() error
		want string
	}{
		{"stale", func() error { return coord.MarkStale(ctx, key, now) }, "stale"},
		{"superseded", func() error { return coord.MarkSuperseded(ctx, key, now) }, "superseded"},
	} {
		if err := step.run(); err != nil {
			t.Fatalf("mark %s: %v", step.name, err)
		}
		var state string
		if err := pool.QueryRow(ctx,
			`SELECT lifecycle_state FROM synthesis_runs WHERE logical_key = $1`, key).Scan(&state); err != nil {
			t.Fatalf("read lifecycle after %s: %v", step.name, err)
		}
		if state != step.want {
			t.Fatalf("lifecycle state %q after mark %s, want %q", state, step.name, step.want)
		}
	}

	archived, err := coord.ArchiveOlderThan(ctx, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if archived < 1 {
		t.Fatalf("archived %d runs, want at least 1", archived)
	}

	// The whole point: archiving changes a label, it does not delete history.
	// An audit trail that shrinks when a run ages out is not an audit trail.
	if n := countSynthesisRows(t, pool, "synthesis_run_attempts"); n != attemptsBefore {
		t.Fatalf("attempts went from %d to %d across lifecycle transitions; provenance must be append-preserving", attemptsBefore, n)
	}
	if n := countSynthesisRows(t, pool, "synthesis_output_insights"); n != insightsBefore {
		t.Fatalf("insights went from %d to %d; archiving must not destroy content", insightsBefore, n)
	}
	if n := countSynthesisRows(t, pool, "synthesis_citations"); n != citationsBefore {
		t.Fatalf("citations went from %d to %d; archiving must not destroy provenance", citationsBefore, n)
	}

	// The output is still readable after archiving -- it is historical, not gone.
	if _, err := persistence.ReadAggregate(ctx, agg.OutputID); err != nil {
		t.Fatalf("archived output must remain readable: %v", err)
	}
}
