//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 SCOPE-04 — T004-05-HEALTH / T004-06-ALERT.
//
// The regression these guard is specific and was live in the tree: health read
// MAX(created_at) FROM synthesis_insights, the LEGACY table. Once the producer
// began committing to synthesis_outputs instead, that source would have reported
// never-run indefinitely while real output accumulated. Every case below reads
// through the model the API now uses, against rows the producer actually writes.

// SCN-004-004-05. Never-run is its own state and is never healthy.
func TestSynthesisHealth_NeverRunIsNotUp(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}

	query := intelligence.SynthesisReadQuery{
		Principal: "test-health-never-run", Cadence: intelligence.CadenceDaily,
		FreshnessBudget: 48 * time.Hour, ObservedAt: time.Now().UTC(),
	}
	snapshot, err := model.ReadSnapshot(context.Background(), query)
	if err != nil {
		t.Fatalf("read never-run snapshot: %v", err)
	}
	outcome := snapshot.Outcome
	if outcome.Phase != intelligence.PhaseNoRun {
		t.Fatalf("phase %q with no attempts and no outputs, want no-run", outcome.Phase)
	}

	health := intelligence.DeriveSynthesisHealth(outcome)
	if health.IntelligenceStatus == "up" {
		t.Fatal("never-run reported as up; a system that has never produced anything cannot be healthy")
	}

	// Also assert the read model refuses to invent a latest output. Returning a
	// zero-valued struct here would let a caller render an empty success.
	if _, found, err := model.LatestFor(context.Background(), query.Principal, query.Cadence); err != nil || found {
		t.Fatalf("LatestFor reported found=%v err=%v with no rows; never-run must be distinguishable from an empty output", found, err)
	}
}

// SCN-004-004-05 continued. A committed output must make health healthy --
// otherwise the never-run assertion above could pass simply because the model
// never returns anything.
func TestSynthesisHealth_CommittedOutputIsUpAndReadable(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	candidate := synthesisCompleteCandidate("operator-health")
	attempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
		intelligence.TriggerScheduled, "operator-health-holder", now.Add(-time.Minute))
	agg := commitSynthesisTestAttempt(t, persistence, attempt, candidate,
		synthesisAuthorizedSources(), now)

	snapshot := readSynthesisHealthSnapshot(t, pool, candidate.Key, now)
	outcome := snapshot.Outcome
	if outcome.Phase != intelligence.PhaseCommitted {
		t.Fatalf("phase %q after a successful commit, want committed", outcome.Phase)
	}
	if outcome.Stale {
		t.Fatal("a just-committed output reported stale")
	}
	if health := intelligence.DeriveSynthesisHealth(outcome); health.IntelligenceStatus != "up" {
		t.Fatalf("status %q after a verified commit, want up", health.IntelligenceStatus)
	}

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}
	latest, found, err := model.LatestFor(ctx, candidate.Key.Principal, candidate.Key.Cadence)
	if err != nil || !found {
		t.Fatalf("Latest found=%v err=%v after a commit", found, err)
	}
	if latest.OutputID != agg.OutputID {
		t.Fatalf("Latest returned %s, want the committed %s", latest.OutputID, agg.OutputID)
	}
	if latest.InsightCount != agg.InsightCount {
		t.Fatalf("Latest insight count %d, want %d", latest.InsightCount, agg.InsightCount)
	}
}

// SCN-004-004-06. An output past its freshness budget reports stale, and stale
// is not up.
func TestSynthesisHealth_AgedOutputIsStaleNotUp(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	committedAt := time.Now().UTC().Add(-72 * time.Hour)

	candidate := synthesisCompleteCandidate("operator-stale")
	attempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
		intelligence.TriggerScheduled, "operator-stale-holder", committedAt.Add(-time.Minute))
	_ = commitSynthesisTestAttempt(t, persistence, attempt, candidate,
		synthesisAuthorizedSources(), committedAt)

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}

	snapshot, err := model.ReadSnapshot(ctx, intelligence.SynthesisReadQuery{
		Principal: candidate.Key.Principal, Cadence: candidate.Key.Cadence,
		FreshnessBudget: 48 * time.Hour, ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("read stale snapshot: %v", err)
	}
	outcome := snapshot.Outcome
	if !outcome.Stale {
		t.Fatal("a 72h-old output against a 48h budget did not report stale")
	}
	if status := intelligence.DeriveSynthesisHealth(outcome).IntelligenceStatus; status == "up" {
		t.Fatalf("stale output reported %q; stale must not read as up", status)
	}
}

// SCN-004-004-06. A failed latest attempt stays visible even when an older
// verified output exists -- the prior success must not paper over the failure.
func TestSynthesisHealth_FailedAttemptIsNotClearedByAnOlderOutput(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	candidate := synthesisCompleteCandidate("operator-then-failed")
	priorAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
		intelligence.TriggerScheduled, "operator-then-failed-prior", now.Add(-2*time.Hour))
	_ = commitSynthesisTestAttempt(t, persistence, priorAttempt, candidate,
		synthesisAuthorizedSources(), now.Add(-90*time.Minute))
	// A later run fails. The output from the earlier run is still sitting there.
	failedAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
		intelligence.TriggerOperatorRetry, "operator-then-failed-current", now.Add(-time.Hour))
	if err := persistence.FinishAttemptFailure(ctx, failedAttempt, intelligence.EventFailed,
		string(intelligence.FailureInvalidPayload), intelligence.FailureTerminal, now.Add(-30*time.Minute)); err != nil {
		t.Fatalf("finish failure: %v", err)
	}

	snapshot := readSynthesisHealthSnapshot(t, pool, candidate.Key, now)
	outcome := snapshot.Outcome
	if outcome.Phase != intelligence.PhaseWriteFailed {
		t.Fatalf("phase %q when the latest attempt failed, want write-failed", outcome.Phase)
	}
	if !outcome.HasPriorVerifiedOutput {
		t.Fatal("the earlier verified output was not reported; an operator needs to know one exists")
	}
	if status := intelligence.DeriveSynthesisHealth(outcome).IntelligenceStatus; status == "up" {
		t.Fatalf("status %q with a failed latest attempt; an older success must not clear a current failure", status)
	}
}

// SCN-004-004-C16. Interleaved actors and cadences must not let a newer
// unrelated failure replace the requested actor's verified daily result.
func TestSynthesisReadSnapshot_IsCausalPerActorAndCadence(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	observedAt := time.Now().UTC()

	targetCandidate := synthesisCompleteCandidate("test-c16-target-actor")
	targetAttempt := startSynthesisTestAttempt(t, persistence, targetCandidate.Key,
		intelligence.TriggerScheduled, "test-c16-target-holder", observedAt.Add(-4*time.Hour))
	targetOutput := commitSynthesisTestAttempt(t, persistence, targetAttempt,
		targetCandidate, synthesisAuthorizedSources(), observedAt.Add(-3*time.Hour))

	unrelatedWeekly := synthesisCompleteCandidate("test-c16-other-actor")
	unrelatedWeekly.Key.Cadence = intelligence.CadenceWeekly
	unrelatedWeekly.Key.WindowStart = targetCandidate.Key.WindowStart.Add(-6 * 24 * time.Hour)
	unrelatedAttempt := startSynthesisTestAttempt(t, persistence, unrelatedWeekly.Key,
		intelligence.TriggerScheduled, "test-c16-weekly-holder", observedAt.Add(-2*time.Hour))
	_ = commitSynthesisTestAttempt(t, persistence, unrelatedAttempt,
		unrelatedWeekly, synthesisAuthorizedSources(), observedAt.Add(-90*time.Minute))

	failingWeekly := synthesisCompleteCandidate("test-c16-target-actor")
	failingWeekly.Key.Cadence = intelligence.CadenceWeekly
	failingWeekly.Key.WindowStart = targetCandidate.Key.WindowStart.Add(-6 * 24 * time.Hour)
	failingAttempt := startSynthesisTestAttempt(t, persistence, failingWeekly.Key,
		intelligence.TriggerOperatorRetry, "test-c16-failure-holder", observedAt.Add(-time.Hour))
	if err := persistence.FinishAttemptFailure(ctx, failingAttempt, intelligence.EventFailed,
		string(intelligence.FailureTransaction), intelligence.FailureTerminal, observedAt.Add(-30*time.Minute)); err != nil {
		t.Fatalf("finish unrelated weekly failure: %v", err)
	}

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}
	snapshot, err := model.ReadSnapshot(ctx, intelligence.SynthesisReadQuery{
		Principal:       targetCandidate.Key.Principal,
		Cadence:         intelligence.CadenceDaily,
		FreshnessBudget: 24 * time.Hour,
		ObservedAt:      observedAt,
	})
	if err != nil {
		t.Fatalf("read target daily snapshot: %v", err)
	}
	if snapshot.Outcome.Phase != intelligence.PhaseCommitted {
		t.Fatalf("target daily phase = %q, want committed from run %s and output %s; unrelated weekly histories must not replace it",
			snapshot.Outcome.Phase, targetAttempt.RunID, targetOutput.OutputID)
	}
	if snapshot.LatestAttempt == nil || snapshot.LatestAttempt.RunID != targetAttempt.RunID {
		t.Fatalf("latest attempt = %#v, want target daily run %s", snapshot.LatestAttempt, targetAttempt.RunID)
	}
	if snapshot.CurrentOutput == nil || snapshot.CurrentOutput.Latest.OutputID != targetOutput.OutputID {
		t.Fatalf("current output = %#v, want target daily output %s", snapshot.CurrentOutput, targetOutput.OutputID)
	}
	if snapshot.CausalRelation != intelligence.SynthesisCausalSameAttemptOutput {
		t.Fatalf("causal relation = %q, want same-attempt-output", snapshot.CausalRelation)
	}
}

// SCN-004-004-C17. Current health follows immutable event order for only the
// requested actor and cadence. An older verified output remains visible while
// newer work runs or fails, and only a later verified terminal event recovers.
func TestSynthesisHealth_RunningFailureAndRecoveryFollowCausalEventOrder(t *testing.T) {
	testCases := []struct {
		name string
		run  func(t *testing.T, pool *pgxpool.Pool)
	}{
		{
			name: "prior verified output then newer running attempt",
			run: func(t *testing.T, pool *pgxpool.Pool) {
				observedAt := time.Now().UTC()
				persistence := newSynthesisTestPersistence(t, pool)
				candidate := synthesisCompleteCandidate("test-c17-running-actor")
				priorAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
					intelligence.TriggerScheduled, "test-c17-running-prior", observedAt.Add(-4*time.Hour))
				priorOutput := commitSynthesisTestAttempt(t, persistence, priorAttempt,
					candidate, synthesisAuthorizedSources(), observedAt.Add(-3*time.Hour))

				interleaveSynthesisHealthHistory(t, persistence, candidate.Key,
					"test-c17-running-other", observedAt.Add(-2*time.Hour))
				runningAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
					intelligence.TriggerOperatorRetry, "test-c17-running-current", observedAt.Add(-time.Hour))

				snapshot := readSynthesisHealthSnapshot(t, pool, candidate.Key, observedAt)
				if snapshot.LatestAttempt == nil || snapshot.LatestAttempt.RunID != runningAttempt.RunID ||
					snapshot.LatestAttempt.EventType != intelligence.EventAttemptStarted {
					t.Fatalf("latest attempt = %#v, want running attempt %s", snapshot.LatestAttempt, runningAttempt.RunID)
				}
				if snapshot.CurrentOutput == nil || snapshot.CurrentOutput.Latest.OutputID != priorOutput.OutputID {
					t.Fatalf("current output = %#v, want prior verified output %s", snapshot.CurrentOutput, priorOutput.OutputID)
				}
				if snapshot.CausalRelation != intelligence.SynthesisCausalPriorAttemptOutput {
					t.Fatalf("causal relation = %q, want prior-attempt-output", snapshot.CausalRelation)
				}
				if snapshot.Outcome.Phase != intelligence.PhaseRunning || !snapshot.Outcome.HasPriorVerifiedOutput {
					t.Fatalf("outcome = %#v, want running with prior verified output", snapshot.Outcome)
				}
				if health := intelligence.DeriveSynthesisHealth(snapshot.Outcome); health.State != intelligence.SynthesisRunning || health.IntelligenceStatus == "up" {
					t.Fatalf("health = %#v, want non-up running state", health)
				}
			},
		},
		{
			name: "prior verified output then newer failed attempt",
			run: func(t *testing.T, pool *pgxpool.Pool) {
				ctx := context.Background()
				observedAt := time.Now().UTC()
				persistence := newSynthesisTestPersistence(t, pool)
				candidate := synthesisCompleteCandidate("test-c17-failed-actor")
				priorAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
					intelligence.TriggerScheduled, "test-c17-failed-prior", observedAt.Add(-4*time.Hour))
				priorOutput := commitSynthesisTestAttempt(t, persistence, priorAttempt,
					candidate, synthesisAuthorizedSources(), observedAt.Add(-3*time.Hour))

				interleaveSynthesisHealthHistory(t, persistence, candidate.Key,
					"test-c17-failed-other", observedAt.Add(-2*time.Hour))
				failedAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
					intelligence.TriggerOperatorRetry, "test-c17-failed-current", observedAt.Add(-time.Hour))
				if err := persistence.FinishAttemptFailure(ctx, failedAttempt, intelligence.EventFailed,
					string(intelligence.FailureTransaction), intelligence.FailureTerminal, observedAt.Add(-30*time.Minute)); err != nil {
					t.Fatalf("finish current failure: %v", err)
				}

				snapshot := readSynthesisHealthSnapshot(t, pool, candidate.Key, observedAt)
				if snapshot.LatestAttempt == nil || snapshot.LatestAttempt.RunID != failedAttempt.RunID ||
					snapshot.LatestAttempt.EventType != intelligence.EventFailed {
					t.Fatalf("latest attempt = %#v, want failed attempt %s", snapshot.LatestAttempt, failedAttempt.RunID)
				}
				if snapshot.CurrentOutput == nil || snapshot.CurrentOutput.Latest.OutputID != priorOutput.OutputID {
					t.Fatalf("current output = %#v, want prior verified output %s", snapshot.CurrentOutput, priorOutput.OutputID)
				}
				if snapshot.Outcome.Phase != intelligence.PhaseWriteFailed || !snapshot.Outcome.HasPriorVerifiedOutput {
					t.Fatalf("outcome = %#v, want write-failed with prior verified output", snapshot.Outcome)
				}
				if health := intelligence.DeriveSynthesisHealth(snapshot.Outcome); health.State != intelligence.SynthesisFailedWithPriorOutput || health.IntelligenceStatus == "up" {
					t.Fatalf("health = %#v, want non-up failed-with-prior-output state", health)
				}
			},
		},
		{
			name: "failed attempt then later persisted readback verified recovery",
			run: func(t *testing.T, pool *pgxpool.Pool) {
				ctx := context.Background()
				observedAt := time.Now().UTC()
				persistence := newSynthesisTestPersistence(t, pool)
				candidate := synthesisCompleteCandidate("test-c17-recovery-actor")
				failedAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
					intelligence.TriggerScheduled, "test-c17-recovery-failed", observedAt.Add(-4*time.Hour))
				if err := persistence.FinishAttemptFailure(ctx, failedAttempt, intelligence.EventFailed,
					string(intelligence.FailureTransaction), intelligence.FailureTerminal, observedAt.Add(-3*time.Hour)); err != nil {
					t.Fatalf("finish failure before recovery: %v", err)
				}

				interleaveSynthesisHealthHistory(t, persistence, candidate.Key,
					"test-c17-recovery-other", observedAt.Add(-2*time.Hour))
				recoveryAttempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
					intelligence.TriggerOperatorRetry, "test-c17-recovery-current", observedAt.Add(-time.Hour))
				recoveredOutput := commitSynthesisTestAttempt(t, persistence, recoveryAttempt,
					candidate, synthesisAuthorizedSources(), observedAt.Add(-30*time.Minute))

				snapshot := readSynthesisHealthSnapshot(t, pool, candidate.Key, observedAt)
				if snapshot.LatestAttempt == nil || snapshot.LatestAttempt.RunID != recoveryAttempt.RunID ||
					snapshot.LatestAttempt.EventType != intelligence.EventPersisted {
					t.Fatalf("latest attempt = %#v, want persisted recovery attempt %s", snapshot.LatestAttempt, recoveryAttempt.RunID)
				}
				if snapshot.CurrentOutput == nil || snapshot.CurrentOutput.Latest.OutputID != recoveredOutput.OutputID {
					t.Fatalf("current output = %#v, want recovered output %s", snapshot.CurrentOutput, recoveredOutput.OutputID)
				}
				if snapshot.CausalRelation != intelligence.SynthesisCausalSameAttemptOutput {
					t.Fatalf("causal relation = %q, want same-attempt-output", snapshot.CausalRelation)
				}
				if snapshot.Outcome.Phase != intelligence.PhaseCommitted || snapshot.Outcome.ReadBack != intelligence.ReadBackOK {
					t.Fatalf("outcome = %#v, want committed readback-ok recovery", snapshot.Outcome)
				}
				if health := intelligence.DeriveSynthesisHealth(snapshot.Outcome); health.State != intelligence.SynthesisReadyCurrent || health.IntelligenceStatus != "up" {
					t.Fatalf("health = %#v, want ready-current recovery", health)
				}

				var retainedFailures int
				if err := pool.QueryRow(ctx, `
					SELECT COUNT(*) FROM synthesis_run_events
					WHERE run_id = $1 AND attempt_no = $2 AND event_type = 'failed'
				`, failedAttempt.RunID, failedAttempt.AttemptNo).Scan(&retainedFailures); err != nil {
					t.Fatalf("count retained failure history: %v", err)
				}
				if retainedFailures != 1 {
					t.Fatalf("retained failure events = %d, want 1 after recovery", retainedFailures)
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			pool := synthesisTestPool(t)
			defer pool.Close()
			resetSynthesisTables(t, pool)
			testCase.run(t, pool)
		})
	}
}

func newSynthesisTestPersistence(t *testing.T, pool *pgxpool.Pool) *intelligence.SynthesisPersistence {
	t.Helper()
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	return persistence
}

func interleaveSynthesisHealthHistory(
	t *testing.T,
	persistence *intelligence.SynthesisPersistence,
	targetKey intelligence.SynthesisRunKey,
	principal string,
	now time.Time,
) {
	t.Helper()
	candidate := synthesisCompleteCandidate(principal)
	candidate.Key.Cadence = intelligence.CadenceWeekly
	candidate.Key.WindowStart = targetKey.WindowStart.Add(-6 * 24 * time.Hour)
	attempt := startSynthesisTestAttempt(t, persistence, candidate.Key,
		intelligence.TriggerScheduled, principal+"-holder", now)
	_ = commitSynthesisTestAttempt(t, persistence, attempt, candidate,
		synthesisAuthorizedSources(), now.Add(15*time.Minute))
}

func readSynthesisHealthSnapshot(
	t *testing.T,
	pool *pgxpool.Pool,
	key intelligence.SynthesisRunKey,
	observedAt time.Time,
) intelligence.SynthesisReadSnapshot {
	t.Helper()
	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}
	snapshot, err := model.ReadSnapshot(context.Background(), intelligence.SynthesisReadQuery{
		Principal:       key.Principal,
		Cadence:         key.Cadence,
		FreshnessBudget: 24 * time.Hour,
		ObservedAt:      observedAt,
	})
	if err != nil {
		t.Fatalf("read synthesis health snapshot: %v", err)
	}
	return snapshot
}

// The legacy table must not influence the verdict. Rows there predate the
// durable contract and never passed validation or read-back.
func TestSynthesisHealth_LegacyRowsDoNotMakeHealthUp(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_insights (id, insight_type, through_line, source_artifact_ids, confidence, created_at)
		VALUES ('legacy-health-1', 'through_line', 'a legacy row', ARRAY['art-a'], 0.9, NOW())`); err != nil {
		t.Fatalf("seed legacy insight: %v", err)
	}

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}

	snapshot, err := model.ReadSnapshot(ctx, intelligence.SynthesisReadQuery{
		Principal: "legacy-health-principal", Cadence: intelligence.CadenceDaily,
		FreshnessBudget: 48 * time.Hour, ObservedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("read legacy-only snapshot: %v", err)
	}
	outcome := snapshot.Outcome
	if outcome.Phase != intelligence.PhaseNoRun {
		t.Fatalf("phase %q with only a legacy row present, want no-run; the legacy table is not durable output", outcome.Phase)
	}
	if status := intelligence.DeriveSynthesisHealth(outcome).IntelligenceStatus; status == "up" {
		t.Fatalf("status %q from a legacy row alone; that row never passed validation or read-back", status)
	}
}
