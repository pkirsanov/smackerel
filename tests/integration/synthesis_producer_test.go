//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 T004-01-ADVERSARIAL / T004-01-PRODUCERS.
//
// This is the end-to-end proof that the defect is repaired, and it is written to
// stay honest permanently: it contains BOTH arms. One half exercises the old
// return-and-log path and asserts it leaves NOTHING durable. The other half runs
// the producer and asserts the same window becomes a row an operator can read.
// A future change that quietly reverts the wiring cannot make this pass, and a
// future change that makes RunSynthesis itself write would fail the control --
// so the test also pins WHERE the writing belongs.

func seedSynthesisCluster(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	for _, table := range []string{"edges", "artifacts", "topics"} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}

	// The cluster query requires >= 3 artifacts on one topic drawn from >= 2
	// distinct sources, so seed exactly that shape: fewer would produce a quiet
	// window and prove nothing about the insight path.
	topicID := "topic-synthesis-e2e"
	if _, err := pool.Exec(ctx,
		`INSERT INTO topics (id, name) VALUES ($1, $2)`, topicID, "cross-domain-cluster"); err != nil {
		t.Fatalf("seed topic: %v", err)
	}

	sources := []string{"src-email", "src-article", "src-video"}
	for i := 0; i < 3; i++ {
		artifactID := fmt.Sprintf("art-e2e-%d", i)
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
			VALUES ($1, 'article', $2, $3, $4)`,
			artifactID, fmt.Sprintf("seeded artifact %d", i),
			fmt.Sprintf("hash-e2e-%d", i), sources[i]); err != nil {
			t.Fatalf("seed artifact %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO edges (id, src_id, src_type, dst_id, dst_type, edge_type)
			VALUES ($1, $2, 'artifact', $3, 'topic', 'BELONGS_TO')`,
			fmt.Sprintf("edge-e2e-%d", i), artifactID, topicID); err != nil {
			t.Fatalf("seed edge %d: %v", i, err)
		}
	}
}

func countSynthesisRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func synthesisTestRunPolicy(actor string) intelligence.SynthesisRunPolicy {
	return intelligence.SynthesisRunPolicy{
		Actor:                 actor,
		PolicyVersion:         "synthesis-test/v1",
		RequiredSourceClasses: []string{"canonical-graph"},
		OptionalSourceClasses: []string{},
		Retention:             90 * 24 * time.Hour,
	}
}

func newSynthesisTestProducer(
	t *testing.T,
	engine *intelligence.Engine,
	persistence *intelligence.SynthesisPersistence,
	actor string,
	holder string,
) *intelligence.SynthesisProducer {
	t.Helper()
	coordinator, err := intelligence.NewSynthesisCoordinator(persistence,
		intelligence.SynthesisRetryPolicy{
			MaxAttempts: 3, InitialDelay: time.Millisecond,
			MaxDelay: 5 * time.Millisecond, LeaseTTL: time.Minute,
		}, holder)
	if err != nil {
		t.Fatalf("construct synthesis coordinator: %v", err)
	}
	producer, err := intelligence.NewSynthesisProducer(engine, persistence, synthesisTestRunPolicy(actor))
	if err != nil {
		t.Fatalf("construct synthesis producer: %v", err)
	}
	return producer.WithCoordinator(coordinator)
}

// legacySynthesisLogicalKey is the exact logical-key algorithm used by the
// certified prior source. It deliberately excludes the source-set digest; the
// current writer keeps that historical identity untouched and creates a new
// causal run when the source set changes.
func legacySynthesisLogicalKey(key intelligence.SynthesisRunKey) string {
	h := sha256.New()
	for _, part := range []string{
		string(key.Cadence),
		key.Principal,
		key.WindowStart.UTC().Format(time.RFC3339Nano),
		key.WindowEnd.UTC().Format(time.RFC3339Nano),
		key.PolicyVersion,
	} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// seedMigratedLegacySynthesisWrite executes the exact run/output INSERT column
// shapes from certified source 7c3838e3b2de9ecba2e6a7764493a0412c4ed268.
// Migration 067 must project output identity while leaving the separately
// recorded compatibility attempt entirely unlinked and event-free.
func seedMigratedLegacySynthesisWrite(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	persistence *intelligence.SynthesisPersistence,
	key intelligence.SynthesisRunKey,
	createdAt time.Time,
) (string, string, string) {
	t.Helper()
	logicalKey := legacySynthesisLogicalKey(key)
	prefix := fmt.Sprintf("test-c21-legacy-%s", key.Cadence)
	runID := prefix + "-run"
	outputID := prefix + "-output"
	insightID := prefix + "-insight"

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin certified prior-source write: %v", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_runs
			(id, logical_key, cadence, principal, window_start, window_end,
			 policy_version, source_set_digest, state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'succeeded', $9, $9)
	`, runID, logicalKey, string(key.Cadence), key.Principal,
		key.WindowStart.UTC(), key.WindowEnd.UTC(), key.PolicyVersion,
		key.SourceSetDigest(), createdAt.UTC()); err != nil {
		t.Fatalf("insert certified prior-source run: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_outputs
			(id, run_id, output_kind, insight_count, citation_count,
			 evaluated_artifact_count, created_at)
		VALUES ($1, $2, 'full', 1, 1, 3, $3)
	`, outputID, runID, createdAt.UTC()); err != nil {
		t.Fatalf("insert certified prior-source output: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_output_source_classes
			(output_id, source_class, disposition)
		VALUES ($1, 'canonical-graph', 'included')
	`, outputID); err != nil {
		t.Fatalf("insert certified prior-source source class: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_output_insights
			(id, output_id, ordinal, insight_type, through_line,
			 confidence, created_at)
		VALUES ($1, $2, 0, 'through_line', $3, 0.75, $4)
	`, insightID, outputID, prefix+"-through-line", createdAt.UTC()); err != nil {
		t.Fatalf("insert certified prior-source insight: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO synthesis_citations (insight_id, artifact_id, ordinal)
		VALUES ($1, 'art-e2e-0', 0)
	`, insightID); err != nil {
		t.Fatalf("insert certified prior-source citation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit certified prior-source write: %v", err)
	}

	if err := persistence.RecordAttempt(ctx, logicalKey,
		intelligence.AttemptSucceeded, "", ""); err != nil {
		t.Fatalf("record certified prior-source unlinked attempt: %v", err)
	}
	return runID, outputID, logicalKey
}

func TestSynthesisProducer_PersistsWhereReturnAndLogDidNot(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)

	ctx := context.Background()
	engine := &intelligence.Engine{Pool: pool}

	// ---- ARM 1: the old behaviour. RunSynthesis alone. ----
	//
	// It genuinely produces insights -- the count in the old log line was never
	// a lie. What it does not do is leave a trace, and that gap is the defect.
	insights, err := engine.RunSynthesis(ctx)
	if err != nil {
		t.Fatalf("run synthesis: %v", err)
	}
	if len(insights) == 0 {
		t.Fatal("seeded cluster produced no insights; the control arm proves nothing unless the producer path has real work to do")
	}
	for _, table := range []string{"synthesis_runs", "synthesis_outputs", "synthesis_output_insights"} {
		if n := countSynthesisRows(t, pool, table); n != 0 {
			t.Fatalf("RunSynthesis wrote %d row(s) to %s; the producer owns persistence, not the cluster query", n, table)
		}
	}

	// ---- ARM 2: the repaired behaviour. Same corpus, through the producer. ----
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer := newSynthesisTestProducer(t, engine, persistence, "scheduler", "producer-persist")

	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerScheduled, time.Now().UTC())
	if err != nil {
		t.Fatalf("run and persist: %v", err)
	}

	if agg.Kind != intelligence.OutputKindFull {
		t.Fatalf("stored kind %q, want full", agg.Kind)
	}
	if agg.InsightCount != len(insights) {
		t.Fatalf("stored %d insights, but the same corpus produced %d; the producer must not drop or invent work",
			agg.InsightCount, len(insights))
	}
	if agg.CitationCount == 0 {
		t.Fatal("stored output carries no citations; an uncitable insight should never have been accepted")
	}
	if agg.EvaluatedArtifactCount != 3 {
		t.Fatalf("evaluated-artifact count %d, want 3 seeded artifacts", agg.EvaluatedArtifactCount)
	}

	// The rows exist -- this is the assertion that fails against return-and-log.
	if n := countSynthesisRows(t, pool, "synthesis_runs"); n != 1 {
		t.Fatalf("got %d run rows, want 1", n)
	}
	if n := countSynthesisRows(t, pool, "synthesis_outputs"); n != 1 {
		t.Fatalf("got %d output rows, want 1", n)
	}
	if n := countSynthesisRows(t, pool, "synthesis_output_insights"); n != len(insights) {
		t.Fatalf("got %d insight rows, want %d", n, len(insights))
	}

	// A successful attempt is recorded, so health can tell this apart from a
	// window that never ran.
	var succeeded int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_run_attempts WHERE outcome = 'succeeded'`).Scan(&succeeded); err != nil {
		t.Fatalf("count attempts: %v", err)
	}
	if succeeded != 1 {
		t.Fatalf("got %d succeeded attempts, want 1", succeeded)
	}
}

// A restart re-running the same window must resolve to the stored output rather
// than storing a second one or reporting failure.
func TestSynthesisProducer_RerunOfSameWindowIsIdempotent(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)

	ctx := context.Background()
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer := newSynthesisTestProducer(t, &intelligence.Engine{Pool: pool}, persistence, "scheduler", "producer-replay")

	// Same instant for both runs, so both derive the same window and therefore
	// the same logical key -- which is what a restart within the day looks like.
	now := time.Now().UTC()

	first, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerScheduled, now)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerScheduled, now)
	if err != nil {
		t.Fatalf("re-run must resolve to the stored output, not fail: %v", err)
	}

	if first.OutputID != second.OutputID {
		t.Fatalf("re-run resolved to output %s, want the stored %s", second.OutputID, first.OutputID)
	}
	if n := countSynthesisRows(t, pool, "synthesis_outputs"); n != 1 {
		t.Fatalf("got %d outputs after a re-run, want exactly 1", n)
	}

	// The re-run is recorded as a no-change attempt rather than a second
	// success, so an operator reading attempts sees what actually happened.
	var noChange int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_run_attempts WHERE outcome = 'idempotent_no_change'`).Scan(&noChange); err != nil {
		t.Fatalf("count no-change attempts: %v", err)
	}
	if noChange != 1 {
		t.Fatalf("got %d idempotent-no-change attempts, want 1", noChange)
	}
}

// An empty corpus is a QUIET window, not a failure and not an absence.
func TestSynthesisProducer_EmptyCorpusPersistsQuietOutput(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	ctx := context.Background()
	for _, table := range []string{"edges", "artifacts", "topics"} {
		if _, err := pool.Exec(ctx, "DELETE FROM "+table); err != nil {
			t.Fatalf("clear %s: %v", table, err)
		}
	}

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer := newSynthesisTestProducer(t, &intelligence.Engine{Pool: pool}, persistence, "scheduler", "producer-quiet")

	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceWeekly, intelligence.TriggerScheduled, time.Now().UTC())
	if err != nil {
		t.Fatalf("an empty corpus is a quiet window, not an error: %v", err)
	}
	if agg.Kind != intelligence.OutputKindQuiet {
		t.Fatalf("stored kind %q, want quiet", agg.Kind)
	}
	if n := countSynthesisRows(t, pool, "synthesis_outputs"); n != 1 {
		t.Fatalf("got %d outputs, want 1 durable quiet row", n)
	}
}

// SCN-004-004-02 at the PRODUCER level. Wiring the coordinator into the
// producer is what makes a second scheduler or an operator retry harmless; the
// coordinator tests prove claiming works in isolation, this proves the producer
// actually consults it.
func TestSynthesisProducer_HonoursTheWindowClaim(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)

	ctx := context.Background()
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	policy := intelligence.SynthesisRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		LeaseTTL:     time.Minute,
	}

	build := func(holder string) *intelligence.SynthesisProducer {
		coord, err := intelligence.NewSynthesisCoordinator(persistence, policy, holder)
		if err != nil {
			t.Fatalf("construct coordinator %s: %v", holder, err)
		}
		prod, err := intelligence.NewSynthesisProducer(&intelligence.Engine{Pool: pool}, persistence, synthesisTestRunPolicy("scheduler"))
		if err != nil {
			t.Fatalf("construct producer %s: %v", holder, err)
		}
		return prod.WithCoordinator(coord)
	}

	now := time.Now().UTC()
	first := build("scheduler-process-a")
	second := build("operator-process-b")

	agg, err := first.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerScheduled, now)
	if err != nil {
		t.Fatalf("first producer: %v", err)
	}
	if agg.InsightCount == 0 {
		t.Fatal("first producer stored nothing; the claim test needs real work to guard")
	}

	// The first producer has already FINISHED, so there is no live lease for the
	// second to lose -- ErrRunClaimedElsewhere belongs to the still-running case,
	// which the coordinator tests cover directly. What must hold here is that a
	// second process cannot produce a SECOND answer for a window that already has
	// one: it resolves to the stored output.
	secondAgg, err := second.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerOperatorRetry, now)
	if err != nil {
		t.Fatalf("second producer must resolve to the stored output, not fail: %v", err)
	}
	if secondAgg.OutputID != agg.OutputID {
		t.Fatalf("second producer resolved to output %s, want the stored %s", secondAgg.OutputID, agg.OutputID)
	}

	if n := countSynthesisRows(t, pool, "synthesis_outputs"); n != 1 {
		t.Fatalf("got %d outputs after two producers ran the same window, want 1", n)
	}
	// One run row too. The claim and the commit must converge on a single row
	// rather than the claim leaving an orphan behind.
	if n := countSynthesisRows(t, pool, "synthesis_runs"); n != 1 {
		t.Fatalf("got %d run rows, want 1; the claim and the commit must be the same row", n)
	}
}

// SCN-004-004-C13 / T004-C13-REPLACE. A material source-set change is
// replacement input, not an idempotent replay. The production producer must
// retain both histories while moving the single current identity atomically.
func TestSynthesisReplacement_ChangedSourceSupersedesUnderWindowLock(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)

	ctx := context.Background()
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer := newSynthesisTestProducer(t, &intelligence.Engine{Pool: pool}, persistence, "scheduler", "producer-replacement")
	now := time.Now().UTC()

	first, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerScheduled, now)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// One new artifact lands, exactly as a live ingest would deliver between two
	// operator triggers seconds apart.
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ('art-drift-1', 'article', 'arrived between triggers', 'hash-drift-1', 'src-email')`); err != nil {
		t.Fatalf("seed drifting artifact: %v", err)
	}

	second, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, intelligence.TriggerOperatorRetry, now)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if first.OutputID == second.OutputID || first.RunID == second.RunID || first.LogicalKey == second.LogicalKey {
		t.Fatalf("changed source set reused prior identity: first output/run/key=%s/%s/%s second=%s/%s/%s",
			first.OutputID, first.RunID, first.LogicalKey,
			second.OutputID, second.RunID, second.LogicalKey)
	}
	if n := countSynthesisRows(t, pool, "synthesis_outputs"); n != 2 {
		t.Fatalf("got %d outputs after changed-source replacement, want both histories", n)
	}
	if n := countSynthesisRows(t, pool, "synthesis_runs"); n != 2 {
		t.Fatalf("got %d runs after changed-source replacement, want both histories", n)
	}

	var currentCount int
	var currentOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id)
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = 'daily'
			AND window_start = $2 AND window_end = $3
			AND lifecycle_state = 'current'
	`, "scheduler", first.WindowStart, first.WindowEnd).Scan(&currentCount, &currentOutputID); err != nil {
		t.Fatalf("read current replacement identity: %v", err)
	}
	if currentCount != 1 || currentOutputID != second.OutputID {
		t.Fatalf("current outputs=%d id=%s, want exactly replacement %s",
			currentCount, currentOutputID, second.OutputID)
	}

	var priorOutputState, priorRunState, replacementOutputState, replacementRunState string
	if err := pool.QueryRow(ctx, `
		SELECT prior.lifecycle_state, prior_run.lifecycle_state,
		       replacement.lifecycle_state, replacement_run.lifecycle_state
		FROM synthesis_outputs prior
		JOIN synthesis_runs prior_run ON prior_run.id = prior.run_id
		JOIN synthesis_outputs replacement ON replacement.id = $2
		JOIN synthesis_runs replacement_run ON replacement_run.id = replacement.run_id
		WHERE prior.id = $1
	`, first.OutputID, second.OutputID).Scan(
		&priorOutputState, &priorRunState, &replacementOutputState, &replacementRunState,
	); err != nil {
		t.Fatalf("read replacement lifecycle: %v", err)
	}
	if priorOutputState != "superseded" || priorRunState != "superseded" {
		t.Fatalf("prior lifecycle output/run=%s/%s, want superseded/superseded",
			priorOutputState, priorRunState)
	}
	if replacementOutputState != "current" || replacementRunState != "current" {
		t.Fatalf("replacement lifecycle output/run=%s/%s, want current/current",
			replacementOutputState, replacementRunState)
	}

	var terminalEvents, supersededEvents int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type IN ('persisted', 'quiet', 'partial')),
			COUNT(*) FILTER (WHERE event_type = 'superseded'
				AND output_id = $2 AND related_output_id = $3)
		FROM synthesis_run_events
		WHERE run_id = $1
	`, second.RunID, second.OutputID, first.OutputID).Scan(&terminalEvents, &supersededEvents); err != nil {
		t.Fatalf("read replacement event chain: %v", err)
	}
	if terminalEvents != 1 || supersededEvents != 1 {
		t.Fatalf("replacement events terminal=%d superseded=%d, want exactly 1 each",
			terminalEvents, supersededEvents)
	}

	priorHistory, err := persistence.ReadAggregate(ctx, first.OutputID)
	if err != nil {
		t.Fatalf("read preserved prior aggregate: %v", err)
	}
	if priorHistory.LifecycleState != "superseded" {
		t.Fatalf("preserved prior aggregate lifecycle=%q, want superseded", priorHistory.LifecycleState)
	}
}

// SCN-004-004-C21 regression. Migration-only tests exercise legacy and causal
// inserts independently, while the ordinary replacement test starts from a
// causal output. This case crosses the missing boundary: exact certified
// prior-source daily and weekly writes first land through migration 067 with
// unlinked attempts, then the real current producer replaces the daily output
// after the authorized source set changes.
func TestSynthesisReplacement_MigratedLegacyCurrentYieldsToCausalProducer(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)

	ctx := context.Background()
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	const actor = "test-c21-legacy-transition-actor"
	const policyVersion = "synthesis/v1"
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	baselineSourceIDs := []string{"art-e2e-0", "art-e2e-1", "art-e2e-2"}

	dailyWindow, err := intelligence.WindowFor(intelligence.CadenceDaily, now)
	if err != nil {
		t.Fatalf("derive daily window: %v", err)
	}
	weeklyWindow, err := intelligence.WindowFor(intelligence.CadenceWeekly, now)
	if err != nil {
		t.Fatalf("derive weekly window: %v", err)
	}
	dailyLegacyKey := intelligence.SynthesisRunKey{
		Cadence: intelligence.CadenceDaily, Principal: actor,
		WindowStart: dailyWindow.Start, WindowEnd: dailyWindow.End,
		PolicyVersion: policyVersion, SourceIDs: baselineSourceIDs,
	}
	weeklyLegacyKey := intelligence.SynthesisRunKey{
		Cadence: intelligence.CadenceWeekly, Principal: actor,
		WindowStart: weeklyWindow.Start, WindowEnd: weeklyWindow.End,
		PolicyVersion: policyVersion, SourceIDs: baselineSourceIDs,
	}
	dailyLegacyRunID, dailyLegacyOutputID, dailyLegacyLogicalKey := seedMigratedLegacySynthesisWrite(
		t, ctx, pool, persistence, dailyLegacyKey, now.Add(-2*time.Hour),
	)
	weeklyLegacyRunID, weeklyLegacyOutputID, weeklyLegacyLogicalKey := seedMigratedLegacySynthesisWrite(
		t, ctx, pool, persistence, weeklyLegacyKey, now.Add(-time.Hour),
	)

	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ('test-c21-artifact-drift', 'article', 'test C21 changed source',
			'test-c21-artifact-drift-hash', 'test-c21-source')
	`); err != nil {
		t.Fatalf("seed changed source set: %v", err)
	}

	coordinator, err := intelligence.NewSynthesisCoordinator(persistence,
		intelligence.SynthesisRetryPolicy{
			MaxAttempts: 3, InitialDelay: time.Millisecond,
			MaxDelay: 5 * time.Millisecond, LeaseTTL: time.Minute,
		}, "test-c21-current-candidate-holder")
	if err != nil {
		t.Fatalf("construct current coordinator: %v", err)
	}
	producer, err := intelligence.NewSynthesisProducer(
		&intelligence.Engine{Pool: pool}, persistence,
		intelligence.SynthesisRunPolicy{
			Actor: actor, PolicyVersion: policyVersion,
			RequiredSourceClasses: []string{"canonical-graph"},
			OptionalSourceClasses: []string{},
			Retention:             90 * 24 * time.Hour,
		},
	)
	if err != nil {
		t.Fatalf("construct current producer: %v", err)
	}
	producer.WithCoordinator(coordinator)

	candidate, candidateErr := producer.RunAndPersist(
		ctx, intelligence.CadenceDaily, intelligence.TriggerOperatorRetry, now,
	)
	if candidateErr != nil {
		var auditErr *intelligence.SynthesisAuditPersistenceError
		if errors.As(candidateErr, &auditErr) {
			cause := errors.Unwrap(auditErr)
			t.Fatalf("ERROR: current candidate audit failure: operation=%q wrapped_db_cause=%T: %v",
				auditErr.Operation, cause, cause)
		}
		t.Fatalf("current candidate failed: %T: %v", candidateErr, candidateErr)
	}

	var currentDailyCount int
	var currentDailyOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id)
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = 'daily'
			AND window_start = $2 AND window_end = $3
			AND lifecycle_state = 'current'
	`, actor, dailyWindow.Start, dailyWindow.End).Scan(
		&currentDailyCount, &currentDailyOutputID,
	); err != nil {
		t.Fatalf("read current daily output: %v", err)
	}
	if currentDailyCount != 1 || currentDailyOutputID != candidate.OutputID {
		t.Fatalf("current daily outputs=%d id=%s, want candidate %s",
			currentDailyCount, currentDailyOutputID, candidate.OutputID)
	}

	var dailyRunState, dailyRunLifecycle, dailyOutputLifecycle string
	if err := pool.QueryRow(ctx, `
		SELECT r.state, r.lifecycle_state, o.lifecycle_state
		FROM synthesis_runs r
		JOIN synthesis_outputs o ON o.run_id = r.id
		WHERE r.id = $1 AND o.id = $2
	`, dailyLegacyRunID, dailyLegacyOutputID).Scan(
		&dailyRunState, &dailyRunLifecycle, &dailyOutputLifecycle,
	); err != nil {
		t.Fatalf("read retained daily legacy history: %v", err)
	}
	if dailyRunState != "succeeded" || dailyRunLifecycle != "superseded" || dailyOutputLifecycle != "superseded" {
		t.Fatalf("daily legacy state/run-lifecycle/output-lifecycle=%s/%s/%s, want succeeded/superseded/superseded",
			dailyRunState, dailyRunLifecycle, dailyOutputLifecycle)
	}

	var weeklyRunState, weeklyRunLifecycle, weeklyOutputLifecycle string
	if err := pool.QueryRow(ctx, `
		SELECT r.state, r.lifecycle_state, o.lifecycle_state
		FROM synthesis_runs r
		JOIN synthesis_outputs o ON o.run_id = r.id
		WHERE r.id = $1 AND o.id = $2
	`, weeklyLegacyRunID, weeklyLegacyOutputID).Scan(
		&weeklyRunState, &weeklyRunLifecycle, &weeklyOutputLifecycle,
	); err != nil {
		t.Fatalf("read retained weekly legacy history: %v", err)
	}
	if weeklyRunState != "succeeded" || weeklyRunLifecycle != "current" || weeklyOutputLifecycle != "current" {
		t.Fatalf("weekly legacy state/run-lifecycle/output-lifecycle=%s/%s/%s, want succeeded/current/current",
			weeklyRunState, weeklyRunLifecycle, weeklyOutputLifecycle)
	}

	var legacyAttempts, legacyAttemptsWithoutCausalProjection int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE
			run_id IS NULL AND attempt_no IS NULL AND trigger_kind IS NULL
			AND state IS NULL AND output_id IS NULL AND started_at IS NULL
			AND finished_at IS NULL AND failure_code IS NULL
			AND included_source_classes IS NULL AND omitted_source_classes IS NULL
			AND insight_count IS NULL AND citation_count IS NULL)
		FROM synthesis_run_attempts
		WHERE logical_key = ANY($1)
	`, []string{dailyLegacyLogicalKey, weeklyLegacyLogicalKey}).Scan(
		&legacyAttempts, &legacyAttemptsWithoutCausalProjection,
	); err != nil {
		t.Fatalf("read retained legacy attempts: %v", err)
	}
	if legacyAttempts != 2 || legacyAttemptsWithoutCausalProjection != 2 {
		t.Fatalf("legacy attempts=%d wholly-unlinked=%d, want 2/2",
			legacyAttempts, legacyAttemptsWithoutCausalProjection)
	}

	var linkedAttempts int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no = 1 AND output_id = $2
	`, candidate.RunID, candidate.OutputID).Scan(&linkedAttempts); err != nil {
		t.Fatalf("count linked candidate attempt: %v", err)
	}
	if linkedAttempts != 1 {
		t.Fatalf("linked candidate attempts=%d, want exactly 1", linkedAttempts)
	}

	var candidateAttemptTerminal, candidateAttemptFinished bool
	var includedClassesNonNull, omittedClassesNonNull bool
	var omittedClassCount int
	if err := pool.QueryRow(ctx, `
		SELECT state IN ('persisted', 'quiet', 'partial', 'idempotent',
			'rolled_back', 'retryable_failure', 'failed', 'readback_failed', 'recovered'),
			finished_at IS NOT NULL,
			included_source_classes IS NOT NULL,
			omitted_source_classes IS NOT NULL,
			COALESCE(cardinality(omitted_source_classes), -1)
		FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no = 1 AND output_id = $2
	`, candidate.RunID, candidate.OutputID).Scan(
		&candidateAttemptTerminal, &candidateAttemptFinished,
		&includedClassesNonNull, &omittedClassesNonNull, &omittedClassCount,
	); err != nil {
		t.Fatalf("read linked candidate attempt summary: %v", err)
	}
	if !candidateAttemptTerminal || !candidateAttemptFinished ||
		!includedClassesNonNull || !omittedClassesNonNull || omittedClassCount != 0 {
		t.Fatalf("linked candidate attempt terminal=%t finished=%t included-non-null=%t omitted-non-null=%t omitted=%d, want true/true/true/true/0",
			candidateAttemptTerminal, candidateAttemptFinished,
			includedClassesNonNull, omittedClassesNonNull, omittedClassCount)
	}

	var startedEvents, terminalEvents, supersededEvents, totalEvents int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE event_type IN ('persisted', 'quiet', 'partial',
				'idempotent', 'rolled_back', 'retryable_failure', 'failed',
				'readback_failed', 'recovered')),
			COUNT(*) FILTER (WHERE event_type = 'superseded'
				AND output_id = $2 AND related_output_id = $3),
			COUNT(*)
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = 1
	`, candidate.RunID, candidate.OutputID, dailyLegacyOutputID).Scan(
		&startedEvents, &terminalEvents, &supersededEvents, &totalEvents,
	); err != nil {
		t.Fatalf("read candidate event chain: %v", err)
	}
	if startedEvents != 1 || terminalEvents != 1 || supersededEvents != 1 || totalEvents != 3 {
		t.Fatalf("candidate events started=%d terminal=%d superseded=%d total=%d, want 1/1/1/3",
			startedEvents, terminalEvents, supersededEvents, totalEvents)
	}

	var legacyEvents int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_run_events
		WHERE run_id = ANY($1)
	`, []string{dailyLegacyRunID, weeklyLegacyRunID}).Scan(&legacyEvents); err != nil {
		t.Fatalf("count fabricated legacy events: %v", err)
	}
	if legacyEvents != 0 {
		t.Fatalf("migration/current writer fabricated %d causal event(s) for legacy runs", legacyEvents)
	}

	var sourceSetChanged, identityPreserved bool
	if err := pool.QueryRow(ctx, `
		SELECT prior.source_set_digest <> replacement.source_set_digest,
			prior.principal = replacement.principal
			AND prior.cadence = replacement.cadence
			AND prior.window_start = replacement.window_start
			AND prior.window_end = replacement.window_end
			AND prior.policy_version = replacement.policy_version
		FROM synthesis_runs prior
		JOIN synthesis_runs replacement ON replacement.id = $2
		WHERE prior.id = $1
	`, dailyLegacyRunID, candidate.RunID).Scan(&sourceSetChanged, &identityPreserved); err != nil {
		t.Fatalf("compare legacy and candidate run identities: %v", err)
	}
	if !sourceSetChanged || !identityPreserved {
		t.Fatalf("replacement source-set-changed=%t actor/cadence/window/policy-preserved=%t, want true/true",
			sourceSetChanged, identityPreserved)
	}
}
