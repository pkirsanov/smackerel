//go:build integration

package integration

import (
	"context"
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
	producer, err := intelligence.NewSynthesisProducer(engine, persistence)
	if err != nil {
		t.Fatalf("construct producer: %v", err)
	}

	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, "scheduler", time.Now().UTC())
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
	producer, err := intelligence.NewSynthesisProducer(&intelligence.Engine{Pool: pool}, persistence)
	if err != nil {
		t.Fatalf("construct producer: %v", err)
	}

	// Same instant for both runs, so both derive the same window and therefore
	// the same logical key -- which is what a restart within the day looks like.
	now := time.Now().UTC()

	first, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, "scheduler", now)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	second, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily, "scheduler", now)
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
	producer, err := intelligence.NewSynthesisProducer(&intelligence.Engine{Pool: pool}, persistence)
	if err != nil {
		t.Fatalf("construct producer: %v", err)
	}

	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceWeekly, "scheduler", time.Now().UTC())
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
		prod, err := intelligence.NewSynthesisProducer(&intelligence.Engine{Pool: pool}, persistence)
		if err != nil {
			t.Fatalf("construct producer %s: %v", holder, err)
		}
		return prod.WithCoordinator(coord)
	}

	now := time.Now().UTC()
	first := build("scheduler-process-a")
	second := build("operator-process-b")

	agg, err := first.RunAndPersist(ctx, intelligence.CadenceDaily, "scheduler", now)
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
	secondAgg, err := second.RunAndPersist(ctx, intelligence.CadenceDaily, "scheduler", now)
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
