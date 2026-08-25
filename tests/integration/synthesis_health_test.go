//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

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

	outcome := model.LatestOutcome(context.Background(), 48*time.Hour, time.Now().UTC())
	if outcome.Phase != intelligence.PhaseNoRun {
		t.Fatalf("phase %q with no attempts and no outputs, want no-run", outcome.Phase)
	}

	health := intelligence.DeriveSynthesisHealth(outcome)
	if health.IntelligenceStatus == "up" {
		t.Fatal("never-run reported as up; a system that has never produced anything cannot be healthy")
	}

	// Also assert the read model refuses to invent a latest output. Returning a
	// zero-valued struct here would let a caller render an empty success.
	if _, found, err := model.Latest(context.Background()); err != nil || found {
		t.Fatalf("Latest reported found=%v err=%v with no rows; never-run must be distinguishable from an empty output", found, err)
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

	agg, err := persistence.Commit(ctx, synthesisCompleteCandidate("operator-health"),
		synthesisTestPolicy(), synthesisAuthorizedSources(), now)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := persistence.RecordAttempt(ctx, agg.LogicalKey, intelligence.AttemptSucceeded, "", ""); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}

	outcome := model.LatestOutcome(ctx, 48*time.Hour, now)
	if outcome.Phase != intelligence.PhaseCommitted {
		t.Fatalf("phase %q after a successful commit, want committed", outcome.Phase)
	}
	if outcome.Stale {
		t.Fatal("a just-committed output reported stale")
	}
	if health := intelligence.DeriveSynthesisHealth(outcome); health.IntelligenceStatus != "up" {
		t.Fatalf("status %q after a verified commit, want up", health.IntelligenceStatus)
	}

	latest, found, err := model.Latest(ctx)
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

	agg, err := persistence.Commit(ctx, synthesisCompleteCandidate("operator-stale"),
		synthesisTestPolicy(), synthesisAuthorizedSources(), committedAt)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := persistence.RecordAttempt(ctx, agg.LogicalKey, intelligence.AttemptSucceeded, "", ""); err != nil {
		t.Fatalf("record attempt: %v", err)
	}

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}

	outcome := model.LatestOutcome(ctx, 48*time.Hour, time.Now().UTC())
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

	agg, err := persistence.Commit(ctx, synthesisCompleteCandidate("operator-then-failed"),
		synthesisTestPolicy(), synthesisAuthorizedSources(), now)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := persistence.RecordAttempt(ctx, agg.LogicalKey, intelligence.AttemptSucceeded, "", ""); err != nil {
		t.Fatalf("record success: %v", err)
	}
	// A later run fails. The output from the earlier run is still sitting there.
	if err := persistence.RecordAttempt(ctx, agg.LogicalKey, intelligence.AttemptFailed,
		string(intelligence.FailureInvalidPayload), "later run failed"); err != nil {
		t.Fatalf("record failure: %v", err)
	}

	model, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct read model: %v", err)
	}

	outcome := model.LatestOutcome(ctx, 48*time.Hour, now)
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

	outcome := model.LatestOutcome(ctx, 48*time.Hour, time.Now().UTC())
	if outcome.Phase != intelligence.PhaseNoRun {
		t.Fatalf("phase %q with only a legacy row present, want no-run; the legacy table is not durable output", outcome.Phase)
	}
	if status := intelligence.DeriveSynthesisHealth(outcome).IntelligenceStatus; status == "up" {
		t.Fatalf("status %q from a legacy row alone; that row never passed validation or read-back", status)
	}
}
