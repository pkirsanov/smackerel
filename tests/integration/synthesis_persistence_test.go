//go:build integration

// BUG-004-004 SCOPE-01 — live-DB coverage for SCN-004-004-01/02/03.
//
// These are the rows the packet was parked on. They need a real PostgreSQL
// because every property under test is a database property: a serializable
// transaction, a UNIQUE constraint doing the idempotence work, and a rollback
// that must leave an audit row standing. None of that can be proven against a
// fake, which is why they live here rather than in the unit lane.

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// synthesisTestPool mirrors authTestPool: fail fast rather than skip, so a
// missing DATABASE_URL is a visible failure instead of a silent no-op. A skip
// here would reproduce the false-green class BUG-069-005 was opened for.
func synthesisTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("synthesis persistence integration test requires DATABASE_URL — run via `./smackerel.sh test integration`")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 6
	cfg.MinConns = 0

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// resetSynthesisTables clears the BUG-004-004 tables in the disposable test
// database. The event ledger rejects row UPDATE/DELETE by design, so test
// isolation uses TRUNCATE rather than weakening the production immutability
// trigger.
func resetSynthesisTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	const statement = `
		TRUNCATE synthesis_run_events, synthesis_citations,
			synthesis_output_insights, synthesis_output_source_classes,
			synthesis_outputs, synthesis_run_attempts, synthesis_runs
		RESTART IDENTITY CASCADE`
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatalf("reset synthesis tables: %v", err)
	}
}

func synthesisTestKey(principal string) intelligence.SynthesisRunKey {
	return intelligence.SynthesisRunKey{
		Cadence:       intelligence.CadenceDaily,
		Principal:     principal,
		WindowStart:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		PolicyVersion: "v1",
		SourceIDs:     []string{"art-alpha", "art-beta", "art-gamma"},
	}
}

// synthesisTestPolicy declares one required class and one optional one, which
// is what lets the partial-output case be expressed at all.
func synthesisTestPolicy() intelligence.SourceClassPolicy {
	return intelligence.SourceClassPolicy{
		Required: []string{"article"},
		Optional: []string{"video"},
	}
}

func synthesisAuthorizedSources() []string {
	return []string{"art-alpha", "art-beta", "art-gamma"}
}

func synthesisCompleteCandidate(principal string) intelligence.SynthesisCandidate {
	return intelligence.SynthesisCandidate{
		Key:                    synthesisTestKey(principal),
		Kind:                   intelligence.OutputKindFull,
		Insights:               synthesisTestInsights(),
		EvaluatedArtifactCount: 3,
		IncludedClasses:        []string{"article", "video"},
	}
}

func synthesisTestInsights() []intelligence.SynthesisInsight {
	return []intelligence.SynthesisInsight{
		{
			ID:                ulid.Make().String(),
			InsightType:       intelligence.InsightThroughLine,
			ThroughLine:       "distributed systems reading converges on backpressure",
			KeyTension:        "throughput versus latency",
			SourceArtifactIDs: []string{"art-alpha", "art-beta"},
			Confidence:        0.72,
		},
		{
			ID:                ulid.Make().String(),
			InsightType:       intelligence.InsightThroughLine,
			ThroughLine:       "recurring interest in schema evolution",
			SourceArtifactIDs: []string{"art-gamma"},
			Confidence:        0.41,
		},
	}
}

func startSynthesisTestAttempt(
	t *testing.T,
	p *intelligence.SynthesisPersistence,
	key intelligence.SynthesisRunKey,
	trigger intelligence.SynthesisTriggerKind,
	holder string,
	now time.Time,
) intelligence.SynthesisAttempt {
	t.Helper()
	attempt, err := p.StartAttempt(context.Background(), key, trigger, holder, time.Minute, now)
	if err != nil {
		t.Fatalf("start synthesis attempt: %v", err)
	}
	return attempt
}

func commitSynthesisTestAttempt(
	t *testing.T,
	p *intelligence.SynthesisPersistence,
	attempt intelligence.SynthesisAttempt,
	candidate intelligence.SynthesisCandidate,
	authorizedSources []string,
	now time.Time,
) *intelligence.SynthesisAggregate {
	t.Helper()
	aggregate, err := p.CommitAttempt(context.Background(), attempt, candidate,
		synthesisTestPolicy(), authorizedSources, now)
	if err != nil {
		t.Fatalf("commit synthesis attempt: %v", err)
	}
	return aggregate
}

func installReplacementOutputFailure(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	ctx := context.Background()
	const install = `
		CREATE OR REPLACE FUNCTION test_fail_synthesis_replacement_output()
		RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced replacement output write failure'
				USING ERRCODE = '23514';
		END;
		$$;
		CREATE TRIGGER test_fail_synthesis_replacement_output
		BEFORE INSERT ON synthesis_outputs
		FOR EACH ROW EXECUTE FUNCTION test_fail_synthesis_replacement_output();`
	if _, err := pool.Exec(ctx, install); err != nil {
		t.Fatalf("install replacement output failure trigger: %v", err)
	}
	return func() {
		if _, err := pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS test_fail_synthesis_replacement_output ON synthesis_outputs;
			DROP FUNCTION IF EXISTS test_fail_synthesis_replacement_output();
		`); err != nil {
			t.Errorf("remove replacement output failure trigger: %v", err)
		}
	}
}

func installReadbackRelationFailure(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	ctx := context.Background()
	const install = `
		CREATE OR REPLACE FUNCTION test_break_synthesis_readback_relation()
		RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN
			ALTER TABLE synthesis_output_source_classes
				RENAME TO synthesis_output_source_classes_readback_fault;
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER test_break_synthesis_readback_relation
		AFTER INSERT ON synthesis_citations
		FOR EACH ROW WHEN (NEW.artifact_id = 'art-gamma')
		EXECUTE FUNCTION test_break_synthesis_readback_relation();`
	if _, err := pool.Exec(ctx, install); err != nil {
		t.Fatalf("install read-back relation failure trigger: %v", err)
	}

	repaired := false
	repair := func() {
		if repaired {
			return
		}
		if _, err := pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS test_break_synthesis_readback_relation ON synthesis_citations;
			DROP FUNCTION IF EXISTS test_break_synthesis_readback_relation();
		`); err != nil {
			t.Fatalf("remove read-back relation failure trigger: %v", err)
		}
		var faultRelationExists bool
		if err := pool.QueryRow(context.Background(), `
			SELECT to_regclass('public.synthesis_output_source_classes_readback_fault') IS NOT NULL
		`).Scan(&faultRelationExists); err != nil {
			t.Fatalf("probe read-back fault relation: %v", err)
		}
		if faultRelationExists {
			if _, err := pool.Exec(context.Background(), `
				ALTER TABLE synthesis_output_source_classes_readback_fault
					RENAME TO synthesis_output_source_classes
			`); err != nil {
				t.Fatalf("repair read-back relation: %v", err)
			}
		}
		repaired = true
	}
	return repair
}

func installTerminalEventFailure(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	const install = `
		CREATE OR REPLACE FUNCTION test_fail_synthesis_terminal_event()
		RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced terminal event write failure'
				USING ERRCODE = '23514';
		END;
		$$;
		CREATE TRIGGER test_fail_synthesis_terminal_event
		BEFORE INSERT ON synthesis_run_events
		FOR EACH ROW WHEN (NEW.event_type IN ('persisted', 'quiet', 'partial'))
		EXECUTE FUNCTION test_fail_synthesis_terminal_event();`
	if _, err := pool.Exec(context.Background(), install); err != nil {
		t.Fatalf("install terminal event failure trigger: %v", err)
	}
	return func() {
		if _, err := pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS test_fail_synthesis_terminal_event ON synthesis_run_events;
			DROP FUNCTION IF EXISTS test_fail_synthesis_terminal_event();
		`); err != nil {
			t.Errorf("remove terminal event failure trigger: %v", err)
		}
	}
}

// SCN-004-004-01 — a successful run commits ONE complete aggregate, and the
// PRODUCTION reader reads the same identity and counts back together.
func TestSynthesisPersistence_CommitsOneCompleteAggregate(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	ctx := context.Background()
	key := synthesisTestKey("operator-scn01")

	cand := synthesisCompleteCandidate("operator-scn01")
	insights := cand.Insights
	agg, err := p.Commit(ctx, cand, synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	// 2 insights, 3 citations total (2 + 1).
	if agg.InsightCount != 2 || agg.CitationCount != 3 {
		t.Fatalf("aggregate counts: got insights=%d citations=%d, want 2 and 3",
			agg.InsightCount, agg.CitationCount)
	}
	if len(agg.Insights) != 2 {
		t.Fatalf("read-back returned %d insights, want 2", len(agg.Insights))
	}

	// Same identity through the production reader, not a test-only query.
	reread, err := p.ReadAggregate(ctx, agg.OutputID)
	if err != nil {
		t.Fatalf("production read-back: %v", err)
	}
	if reread.OutputID != agg.OutputID || reread.RunID != agg.RunID {
		t.Fatalf("identity drift: commit gave output=%s run=%s, reader gave output=%s run=%s",
			agg.OutputID, agg.RunID, reread.OutputID, reread.RunID)
	}
	if reread.LogicalKey != key.LogicalKey() {
		t.Fatalf("logical key drift: stored %s, derived %s", reread.LogicalKey, key.LogicalKey())
	}

	// Ordinals must preserve the producer's order; a set-valued read would make
	// the rendered digest non-deterministic.
	if reread.Insights[0].ThroughLine != insights[0].ThroughLine {
		t.Fatalf("insight order not preserved: got %q first, want %q",
			reread.Insights[0].ThroughLine, insights[0].ThroughLine)
	}
	if len(reread.Insights[0].SourceArtifactIDs) != 2 {
		t.Fatalf("citations not attributed to their insight: got %v",
			reread.Insights[0].SourceArtifactIDs)
	}

	// The row counts must agree with the denormalized counters, so a future
	// writer cannot leave the counter and the rows disagreeing.
	var insightRows, citationRows int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_output_insights WHERE output_id = $1`,
		agg.OutputID).Scan(&insightRows); err != nil {
		t.Fatalf("count insight rows: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_citations c
		JOIN synthesis_output_insights i ON i.id = c.insight_id
		WHERE i.output_id = $1`, agg.OutputID).Scan(&citationRows); err != nil {
		t.Fatalf("count citation rows: %v", err)
	}
	if insightRows != 2 || citationRows != 3 {
		t.Fatalf("stored rows disagree with counters: insights=%d citations=%d", insightRows, citationRows)
	}
}

// SCN-004-004-02 — a duplicate logical run is idempotent: exactly one output,
// and the later attempt records idempotent no-change against the existing one.
func TestSynthesisPersistence_DuplicateLogicalRunIsIdempotent(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	ctx := context.Background()
	key := synthesisTestKey("operator-scn02")

	first, err := p.Commit(ctx, synthesisCompleteCandidate("operator-scn02"), synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// The same logical run again — a restart or a concurrent scheduler.
	_, err = p.Commit(ctx, synthesisCompleteCandidate("operator-scn02"), synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
	if err == nil {
		t.Fatal("second commit of the same logical run succeeded; exactly one output must exist")
	}
	if err != intelligence.ErrSynthesisRunExists {
		t.Fatalf("second commit returned %v, want ErrSynthesisRunExists", err)
	}

	// Exactly one output for this logical key.
	var outputs int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE r.logical_key = $1`, key.LogicalKey()).Scan(&outputs); err != nil {
		t.Fatalf("count outputs: %v", err)
	}
	if outputs != 1 {
		t.Fatalf("logical run produced %d outputs, want exactly 1", outputs)
	}

	// The retry resolves to the ALREADY-COMMITTED output rather than nothing.
	existing, err := p.FindOutputByLogicalKey(ctx, key.LogicalKey())
	if err != nil {
		t.Fatalf("resolve existing output: %v", err)
	}
	if existing != first.OutputID {
		t.Fatalf("retry resolved to output %s, want the committed %s", existing, first.OutputID)
	}

	if err := p.RecordAttempt(ctx, key.LogicalKey(),
		intelligence.AttemptIdempotentNoChange, "", ""); err != nil {
		t.Fatalf("record idempotent attempt: %v", err)
	}
	var noChange int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_run_attempts
		WHERE logical_key = $1 AND outcome = 'idempotent_no_change'`,
		key.LogicalKey()).Scan(&noChange); err != nil {
		t.Fatalf("count idempotent attempts: %v", err)
	}
	if noChange != 1 {
		t.Fatalf("got %d idempotent_no_change attempts, want 1", noChange)
	}
}

// SCN-004-004-03 — a required write failure rolls back atomically, and the
// separately recorded failure attempt references no uncommitted content.
//
// ADVERSARIAL BY CONSTRUCTION. The failure is forced at the SECOND insight, so
// the run row, the output row and the FIRST insight have already been written
// when it fires. A layer that committed incrementally, or that opened a fresh
// transaction per statement, would leave those rows behind and fail here.
func TestSynthesisPersistence_RequiredWriteFailureRollsBackAtomically(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	ctx := context.Background()
	key := synthesisTestKey("operator-scn03")

	// Two insights sharing one ID. The first inserts; the second violates the
	// primary key, after the output and first insight are already in the
	// transaction. This is a REQUIRED write failing mid-aggregate.
	dupID := ulid.Make().String()
	poisoned := []intelligence.SynthesisInsight{
		{
			ID:                dupID,
			InsightType:       intelligence.InsightThroughLine,
			ThroughLine:       "first insight commits inside the transaction",
			SourceArtifactIDs: []string{"art-alpha"},
			Confidence:        0.5,
		},
		{
			ID:                dupID, // duplicate PK — the forced failure
			InsightType:       intelligence.InsightThroughLine,
			ThroughLine:       "second insight must abort the whole aggregate",
			SourceArtifactIDs: []string{"art-beta"},
			Confidence:        0.5,
		},
	}

	poisonedCandidate := intelligence.SynthesisCandidate{
		Key:                    key,
		Kind:                   intelligence.OutputKindFull,
		Insights:               poisoned,
		EvaluatedArtifactCount: 2,
		IncludedClasses:        []string{"article", "video"},
	}
	_, commitErr := p.Commit(ctx, poisonedCandidate, synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
	if commitErr == nil {
		t.Fatal("commit succeeded despite a duplicate insight id; the aggregate must abort")
	}

	// NOTHING from that attempt survives — not the run, not the output, not the
	// first insight that had already been written when the failure fired.
	var runs, outputs, insights, citations int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_runs WHERE logical_key = $1`,
		key.LogicalKey()).Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM synthesis_outputs`).Scan(&outputs); err != nil {
		t.Fatalf("count outputs: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM synthesis_output_insights`).Scan(&insights); err != nil {
		t.Fatalf("count insights: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM synthesis_citations`).Scan(&citations); err != nil {
		t.Fatalf("count citations: %v", err)
	}
	if runs != 0 || outputs != 0 || insights != 0 || citations != 0 {
		t.Fatalf("rollback left rows behind: runs=%d outputs=%d insights=%d citations=%d (all must be 0)",
			runs, outputs, insights, citations)
	}

	// The failure attempt is recorded AFTER the rollback, in its own
	// transaction, so it survives while the content does not.
	if err := p.RecordAttempt(ctx, key.LogicalKey(),
		intelligence.AttemptFailed, "insight_write_conflict",
		fmt.Sprintf("aggregate aborted: %v", commitErr)); err != nil {
		t.Fatalf("record failed attempt: %v", err)
	}

	var outcome, failureClass, failureMessage string
	if err := pool.QueryRow(ctx, `
		SELECT outcome, failure_class, COALESCE(failure_message, '')
		FROM synthesis_run_attempts WHERE logical_key = $1`,
		key.LogicalKey()).Scan(&outcome, &failureClass, &failureMessage); err != nil {
		t.Fatalf("read failure attempt: %v", err)
	}
	if outcome != "failed" || failureClass != "insight_write_conflict" {
		t.Fatalf("attempt recorded as outcome=%q class=%q", outcome, failureClass)
	}

	// "references no uncommitted content": the failure row must not quote the
	// candidate text or any source artifact id from the aborted attempt.
	for _, forbidden := range []string{
		"first insight commits inside the transaction",
		"second insight must abort the whole aggregate",
		"art-alpha", "art-beta", "art-gamma",
	} {
		if contains(failureMessage, forbidden) {
			t.Fatalf("failure attempt leaked uncommitted content %q in %q", forbidden, failureMessage)
		}
	}
}

// RecordAttempt must refuse a malformed audit row rather than storing one that
// the CHECK constraint would have to catch.
func TestSynthesisPersistence_RejectsMalformedAttempts(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()

	if err := p.RecordAttempt(ctx, "k", intelligence.AttemptFailed, "", "no class"); err == nil {
		t.Fatal("a failed attempt with no failure class was accepted")
	}
	if err := p.RecordAttempt(ctx, "k", intelligence.AttemptSucceeded, "some-class", ""); err == nil {
		t.Fatal("a succeeded attempt carrying a failure class was accepted")
	}
	if err := p.RecordAttempt(ctx, "k", intelligence.SynthesisAttemptOutcome("invented"), "", ""); err == nil {
		t.Fatal("an unknown outcome was accepted")
	}
}

// SCN-004-004-07 — a valid window that produces nothing persists ONE explicit
// quiet output that reads differently from never-run and from failure.
func TestSynthesisPersistence_QuietOutputIsDurableNotMissing(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()

	cand := synthesisCompleteCandidate("operator-quiet")
	cand.Kind = intelligence.OutputKindQuiet
	cand.Insights = nil
	cand.EvaluatedArtifactCount = 412 // evaluated many, surfaced none

	agg, err := p.Commit(ctx, cand, synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
	if err != nil {
		t.Fatalf("commit quiet output: %v", err)
	}
	if agg.Kind != intelligence.OutputKindQuiet {
		t.Fatalf("stored kind %q, want quiet", agg.Kind)
	}
	if agg.InsightCount != 0 || agg.CitationCount != 0 {
		t.Fatalf("quiet output carries content: insights=%d citations=%d", agg.InsightCount, agg.CitationCount)
	}

	// This is the property that separates quiet from never-run. Both have zero
	// insights; only quiet asserts a window WAS evaluated, and the count is the
	// assertion. Losing it would make the two indistinguishable again.
	if agg.EvaluatedArtifactCount != 412 {
		t.Fatalf("quiet output lost its evaluated count: got %d, want 412", agg.EvaluatedArtifactCount)
	}

	// A durable row exists — "nothing to surface" is stored, not absent.
	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_outputs WHERE output_kind = 'quiet'`).Scan(&rows); err != nil {
		t.Fatalf("count quiet outputs: %v", err)
	}
	if rows != 1 {
		t.Fatalf("got %d quiet outputs, want exactly 1", rows)
	}
}

// SCN-004-004-08 — a policy-approved optional omission persists a partial
// output that NAMES what was included and what was left out.
func TestSynthesisPersistence_PartialOutputNamesOmissions(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()

	cand := synthesisCompleteCandidate("operator-partial")
	cand.Kind = intelligence.OutputKindPartial
	cand.IncludedClasses = []string{"article"}
	cand.OmittedClasses = []string{"video"} // optional class, permitted omission

	agg, err := p.Commit(ctx, cand, synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
	if err != nil {
		t.Fatalf("commit partial output: %v", err)
	}
	if agg.Kind != intelligence.OutputKindPartial {
		t.Fatalf("stored kind %q, want partial", agg.Kind)
	}
	if len(agg.IncludedClasses) != 1 || agg.IncludedClasses[0] != "article" {
		t.Fatalf("included classes not preserved: %v", agg.IncludedClasses)
	}
	if len(agg.OmittedClasses) != 1 || agg.OmittedClasses[0] != "video" {
		t.Fatalf("omitted classes not preserved: %v", agg.OmittedClasses)
	}

	// Omissions are ROWS, so a reader can count and filter them. Prose in a text
	// column would not be checkable, which is what SCN-08 forbids.
	var omitted int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_output_source_classes
		WHERE output_id = $1 AND disposition = 'omitted'`, agg.OutputID).Scan(&omitted); err != nil {
		t.Fatalf("count omissions: %v", err)
	}
	if omitted != 1 {
		t.Fatalf("got %d omitted-class rows, want 1", omitted)
	}
}

// SCN-004-004-C13 / T004-C13-ROLLBACK. The failure trigger fires on the
// replacement output insert, after the content transaction selected and
// temporarily superseded the prior current row. PostgreSQL rollback must undo
// that transition while the separate safe failure audit remains durable.
func TestSynthesisReplacement_FailedReplacementRestoresPriorCurrentAndAppendsFailure(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	principal := "test-c13-rollback"
	baseCandidate := synthesisCompleteCandidate(principal)
	baseAttempt := startSynthesisTestAttempt(t, persistence, baseCandidate.Key,
		intelligence.TriggerScheduled, "test-c13-initial", time.Now().UTC())
	prior := commitSynthesisTestAttempt(t, persistence, baseAttempt, baseCandidate,
		synthesisAuthorizedSources(), time.Now().UTC())

	replacementCandidate := synthesisCompleteCandidate(principal)
	replacementCandidate.Key.SourceIDs = append(replacementCandidate.Key.SourceIDs, "art-delta")
	authorized := append(synthesisAuthorizedSources(), "art-delta")
	replacementAttempt := startSynthesisTestAttempt(t, persistence, replacementCandidate.Key,
		intelligence.TriggerOperatorRetry, "test-c13-replacement", time.Now().UTC())
	removeFailure := installReplacementOutputFailure(t, pool)
	defer removeFailure()

	if _, err := persistence.CommitAttempt(ctx, replacementAttempt, replacementCandidate,
		synthesisTestPolicy(), authorized, time.Now().UTC()); err == nil {
		t.Fatal("replacement succeeded despite the required output-write failure")
	}

	var currentCount int
	var currentOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id)
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = $2
			AND window_start = $3 AND window_end = $4
			AND lifecycle_state = 'current'
	`, principal, string(baseCandidate.Key.Cadence), baseCandidate.Key.WindowStart,
		baseCandidate.Key.WindowEnd).Scan(&currentCount, &currentOutputID); err != nil {
		t.Fatalf("read current output after replacement rollback: %v", err)
	}
	if currentCount != 1 || currentOutputID != prior.OutputID {
		t.Fatalf("rollback current outputs=%d id=%s, want prior %s",
			currentCount, currentOutputID, prior.OutputID)
	}

	var partialOutputs int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_outputs WHERE run_id = $1`, replacementAttempt.RunID,
	).Scan(&partialOutputs); err != nil {
		t.Fatalf("count failed replacement outputs: %v", err)
	}
	if partialOutputs != 0 {
		t.Fatalf("failed replacement left %d partial output row(s), want 0", partialOutputs)
	}

	var priorOutputState, priorRunState, failedRunState, failedRunLifecycle string
	if err := pool.QueryRow(ctx, `
		SELECT o.lifecycle_state, r.lifecycle_state, failed.state, failed.lifecycle_state
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		JOIN synthesis_runs failed ON failed.id = $2
		WHERE o.id = $1
	`, prior.OutputID, replacementAttempt.RunID).Scan(
		&priorOutputState, &priorRunState, &failedRunState, &failedRunLifecycle,
	); err != nil {
		t.Fatalf("read rollback lifecycle: %v", err)
	}
	if priorOutputState != "current" || priorRunState != "current" {
		t.Fatalf("prior output/run lifecycle=%s/%s, want current/current",
			priorOutputState, priorRunState)
	}
	if failedRunState != "failed" || failedRunLifecycle != "superseded" {
		t.Fatalf("failed replacement run state/lifecycle=%s/%s, want failed/superseded",
			failedRunState, failedRunLifecycle)
	}

	var attemptState, failureCode string
	var finished bool
	var includedCount, omittedCount, insightCount, citationCount int
	if err := pool.QueryRow(ctx, `
		SELECT state, failure_code, finished_at IS NOT NULL,
		       cardinality(included_source_classes), cardinality(omitted_source_classes),
		       insight_count, citation_count
		FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no = $2
	`, replacementAttempt.RunID, replacementAttempt.AttemptNo).Scan(
		&attemptState, &failureCode, &finished, &includedCount, &omittedCount,
		&insightCount, &citationCount,
	); err != nil {
		t.Fatalf("read failed replacement attempt: %v", err)
	}
	if attemptState != "failed" || failureCode != "transaction_failed" || !finished ||
		includedCount != 0 || omittedCount != 0 || insightCount != 0 || citationCount != 0 {
		t.Fatalf("unsafe failed-attempt summary state=%s code=%s finished=%t included=%d omitted=%d insights=%d citations=%d",
			attemptState, failureCode, finished, includedCount, omittedCount, insightCount, citationCount)
	}

	var startedEvents, failedEvents int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE event_type = 'failed'
				AND failure_code = 'transaction_failed'
				AND output_id IS NULL AND related_output_id IS NULL)
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = $2
	`, replacementAttempt.RunID, replacementAttempt.AttemptNo).Scan(&startedEvents, &failedEvents); err != nil {
		t.Fatalf("read failed replacement event chain: %v", err)
	}
	if startedEvents != 1 || failedEvents != 1 {
		t.Fatalf("failed replacement events started=%d failed=%d, want exactly 1 each",
			startedEvents, failedEvents)
	}
}

func TestSynthesisReplacement_TerminalEventFailureRestoresPriorCurrent(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	principal := "test-c13-terminal-audit"
	baseCandidate := synthesisCompleteCandidate(principal)
	baseAttempt := startSynthesisTestAttempt(t, persistence, baseCandidate.Key,
		intelligence.TriggerScheduled, "test-c13-audit-initial", time.Now().UTC())
	prior := commitSynthesisTestAttempt(t, persistence, baseAttempt, baseCandidate,
		synthesisAuthorizedSources(), time.Now().UTC())

	replacementCandidate := synthesisCompleteCandidate(principal)
	replacementCandidate.Key.SourceIDs = append(replacementCandidate.Key.SourceIDs, "art-delta")
	authorized := append(synthesisAuthorizedSources(), "art-delta")
	replacementAttempt := startSynthesisTestAttempt(t, persistence, replacementCandidate.Key,
		intelligence.TriggerOperatorRetry, "test-c13-audit-replacement", time.Now().UTC())
	removeFailure := installTerminalEventFailure(t, pool)
	defer removeFailure()

	_, commitErr := persistence.CommitAttempt(ctx, replacementAttempt, replacementCandidate,
		synthesisTestPolicy(), authorized, time.Now().UTC())
	if commitErr == nil {
		t.Fatal("replacement returned success despite a rejected terminal event")
	}
	var auditErr *intelligence.SynthesisAuditPersistenceError
	if !errors.As(commitErr, &auditErr) {
		t.Fatalf("terminal event failure type=%T, want *SynthesisAuditPersistenceError", commitErr)
	}
	if auditErr.Operation != "terminal_event" {
		t.Fatalf("audit failure operation=%q, want terminal_event", auditErr.Operation)
	}

	var currentCount int
	var currentOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id)
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = $2
			AND window_start = $3 AND window_end = $4
			AND lifecycle_state = 'current'
	`, principal, string(baseCandidate.Key.Cadence), baseCandidate.Key.WindowStart,
		baseCandidate.Key.WindowEnd).Scan(&currentCount, &currentOutputID); err != nil {
		t.Fatalf("read current output after terminal event failure: %v", err)
	}
	if currentCount != 1 || currentOutputID != prior.OutputID {
		t.Fatalf("terminal event failure current outputs=%d id=%s, want prior %s",
			currentCount, currentOutputID, prior.OutputID)
	}

	var forensicOutputID, forensicLifecycle, runState, runLifecycle, attemptState string
	if err := pool.QueryRow(ctx, `
		SELECT o.id, o.lifecycle_state, r.state, r.lifecycle_state, a.state
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		JOIN synthesis_run_attempts a ON a.run_id = r.id AND a.attempt_no = $2
		WHERE r.id = $1
	`, replacementAttempt.RunID, replacementAttempt.AttemptNo).Scan(
		&forensicOutputID, &forensicLifecycle, &runState, &runLifecycle, &attemptState,
	); err != nil {
		t.Fatalf("read compensated replacement: %v", err)
	}
	if forensicOutputID == "" || forensicLifecycle != "superseded" || runLifecycle != "superseded" {
		t.Fatalf("compensated replacement output/lifecycle/run-lifecycle=%s/%s/%s, want forensic/superseded/superseded",
			forensicOutputID, forensicLifecycle, runLifecycle)
	}
	if runState != "running" || attemptState != "running" {
		t.Fatalf("rolled-back terminal summary state run/attempt=%s/%s, want running/running",
			runState, attemptState)
	}

	var startedEvents, falseTerminalEvents int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE event_type IN ('persisted', 'quiet', 'partial'))
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = $2
	`, replacementAttempt.RunID, replacementAttempt.AttemptNo).Scan(
		&startedEvents, &falseTerminalEvents,
	); err != nil {
		t.Fatalf("read terminal-event failure chain: %v", err)
	}
	if startedEvents != 1 || falseTerminalEvents != 0 {
		t.Fatalf("terminal-event failure events started=%d false-terminal=%d, want 1/0",
			startedEvents, falseTerminalEvents)
	}
}

// SCN-004-004-C15 / T004-C15-READBACK. A test-only PostgreSQL trigger renames
// a relation after the final citation write, so the content commit succeeds but
// the production aggregate reader fails. Repairing the relation and rerunning
// the same logical input must append recovered through production code.
func TestSynthesisReadbackFailurePersistsFailureBeforeVerifiedRecovery(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()
	principal := "test-c15-readback"
	baseCandidate := synthesisCompleteCandidate(principal)
	baseAttempt := startSynthesisTestAttempt(t, persistence, baseCandidate.Key,
		intelligence.TriggerScheduled, "test-c15-initial", time.Now().UTC())
	prior := commitSynthesisTestAttempt(t, persistence, baseAttempt, baseCandidate,
		synthesisAuthorizedSources(), time.Now().UTC())

	replacementCandidate := synthesisCompleteCandidate(principal)
	replacementCandidate.Key.SourceIDs = append(replacementCandidate.Key.SourceIDs, "art-delta")
	authorized := append(synthesisAuthorizedSources(), "art-delta")
	replacementAttempt := startSynthesisTestAttempt(t, persistence, replacementCandidate.Key,
		intelligence.TriggerOperatorRetry, "test-c15-replacement", time.Now().UTC())
	repairReadback := installReadbackRelationFailure(t, pool)
	defer repairReadback()

	_, readbackErr := persistence.CommitAttempt(ctx, replacementAttempt, replacementCandidate,
		synthesisTestPolicy(), authorized, time.Now().UTC())
	if readbackErr == nil {
		t.Fatal("replacement returned success while the production aggregate relation was unavailable")
	}
	var typedReadback *intelligence.SynthesisReadbackError
	if !errors.As(readbackErr, &typedReadback) {
		t.Fatalf("read-back failure type=%T, want *SynthesisReadbackError", readbackErr)
	}

	var forensicOutputID, forensicLifecycle string
	if err := pool.QueryRow(ctx, `
		SELECT id, lifecycle_state FROM synthesis_outputs WHERE run_id = $1
	`, replacementAttempt.RunID).Scan(&forensicOutputID, &forensicLifecycle); err != nil {
		t.Fatalf("read committed-unverified forensic output: %v", err)
	}
	if forensicLifecycle != "superseded" {
		t.Fatalf("committed-unverified output lifecycle=%q, want superseded", forensicLifecycle)
	}
	if typedReadback.OutputID != forensicOutputID {
		t.Fatalf("typed read-back output=%s, forensic output=%s", typedReadback.OutputID, forensicOutputID)
	}

	var currentCount int
	var currentOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id)
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = $2
			AND window_start = $3 AND window_end = $4
			AND lifecycle_state = 'current'
	`, principal, string(baseCandidate.Key.Cadence), baseCandidate.Key.WindowStart,
		baseCandidate.Key.WindowEnd).Scan(&currentCount, &currentOutputID); err != nil {
		t.Fatalf("read current output after read-back failure: %v", err)
	}
	if currentCount != 1 || currentOutputID != prior.OutputID {
		t.Fatalf("read-back failure current outputs=%d id=%s, want prior %s",
			currentCount, currentOutputID, prior.OutputID)
	}

	var failedEvents int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = $2
			AND event_type = 'readback_failed'
			AND output_id = $3 AND failure_code = 'readback_failed'
	`, replacementAttempt.RunID, replacementAttempt.AttemptNo, forensicOutputID).Scan(&failedEvents); err != nil {
		t.Fatalf("read durable readback_failed event: %v", err)
	}
	if failedEvents != 1 {
		t.Fatalf("got %d durable readback_failed events, want 1", failedEvents)
	}

	repairReadback()
	recoveryAttempt := startSynthesisTestAttempt(t, persistence, replacementCandidate.Key,
		intelligence.TriggerOperatorRetry, "test-c15-recovery", time.Now().UTC())
	recovered, err := persistence.CommitAttempt(ctx, recoveryAttempt, replacementCandidate,
		synthesisTestPolicy(), authorized, time.Now().UTC())
	if err != nil {
		t.Fatalf("verified recovery: %v", err)
	}
	if recovered.Outcome != intelligence.EventRecovered || recovered.OutputID != forensicOutputID {
		t.Fatalf("recovery outcome/output=%s/%s, want recovered/%s",
			recovered.Outcome, recovered.OutputID, forensicOutputID)
	}

	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), MAX(id)
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = $2
			AND window_start = $3 AND window_end = $4
			AND lifecycle_state = 'current'
	`, principal, string(baseCandidate.Key.Cadence), baseCandidate.Key.WindowStart,
		baseCandidate.Key.WindowEnd).Scan(&currentCount, &currentOutputID); err != nil {
		t.Fatalf("read current output after recovery: %v", err)
	}
	if currentCount != 1 || currentOutputID != forensicOutputID {
		t.Fatalf("recovery current outputs=%d id=%s, want recovered %s",
			currentCount, currentOutputID, forensicOutputID)
	}

	var recoveredEvents, supersededEvents int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'recovered' AND output_id = $2),
			COUNT(*) FILTER (WHERE event_type = 'superseded'
				AND output_id = $2 AND related_output_id = $3)
		FROM synthesis_run_events
		WHERE run_id = $1
	`, replacementAttempt.RunID, forensicOutputID, prior.OutputID).Scan(
		&recoveredEvents, &supersededEvents,
	); err != nil {
		t.Fatalf("read recovery event chain: %v", err)
	}
	if recoveredEvents != 1 || supersededEvents != 1 {
		t.Fatalf("recovery events recovered=%d superseded=%d, want exactly 1 each",
			recoveredEvents, supersededEvents)
	}

	var priorLifecycle string
	if err := pool.QueryRow(ctx,
		`SELECT lifecycle_state FROM synthesis_outputs WHERE id = $1`, prior.OutputID,
	).Scan(&priorLifecycle); err != nil {
		t.Fatalf("read prior lifecycle after recovery: %v", err)
	}
	if priorLifecycle != "superseded" {
		t.Fatalf("prior lifecycle after verified recovery=%q, want superseded", priorLifecycle)
	}
}

// SCN-004-004-04 — an invalid candidate is rejected BEFORE persistence is
// entered, so no output is stored.
//
// ADVERSARIAL ON ORDERING. Empty tables alone do NOT prove this: a validator
// running INSIDE the transaction would also end empty, via rollback. Nor does
// the failure code -- that survives being moved inside too. The one fact that
// separates them is that a pre-transaction rejection never acquires a pooled
// connection, so each case pins the pool's acquire count across the call. Move
// ValidateCandidate below BeginTx and the count rises by one and this fails.
func TestSynthesisPersistence_InvalidCandidateNeverEntersPersistence(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()

	p, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	ctx := context.Background()

	for name, tc := range map[string]struct {
		mutate func(*intelligence.SynthesisCandidate)
		want   intelligence.SynthesisFailureCode
	}{
		"missing citation": {
			mutate: func(c *intelligence.SynthesisCandidate) { c.Insights[0].SourceArtifactIDs = nil },
			want:   intelligence.FailureMissingCitation,
		},
		"unauthorized artifact": {
			mutate: func(c *intelligence.SynthesisCandidate) {
				c.Insights[0].SourceArtifactIDs = []string{"art-not-authorized"}
			},
			want: intelligence.FailureUnauthorizedSource,
		},
		"invalid payload": {
			mutate: func(c *intelligence.SynthesisCandidate) { c.Insights[0].ThroughLine = "" },
			want:   intelligence.FailureInvalidPayload,
		},
		"required source omitted": {
			mutate: func(c *intelligence.SynthesisCandidate) {
				c.Kind = intelligence.OutputKindPartial
				c.IncludedClasses = []string{"video"}
				c.OmittedClasses = []string{"article"}
			},
			want: intelligence.FailureRequiredSourceOmit,
		},
	} {
		t.Run(name, func(t *testing.T) {
			resetSynthesisTables(t, pool)
			cand := synthesisCompleteCandidate("operator-invalid-" + name)
			tc.mutate(&cand)

			acquiresBefore := pool.Stat().AcquireCount()
			_, err := p.Commit(ctx, cand, synthesisTestPolicy(), synthesisAuthorizedSources(), time.Now().UTC())
			if acquired := pool.Stat().AcquireCount() - acquiresBefore; acquired != 0 {
				t.Fatalf("rejection acquired %d pooled connection(s); a candidate refused before BeginTx must acquire none", acquired)
			}
			if err == nil {
				t.Fatal("invalid candidate was accepted")
			}
			var ve *intelligence.SynthesisValidationError
			if !errorAs(err, &ve) {
				t.Fatalf("expected a validation error (proving the pre-transaction gate fired), got %T: %v", err, err)
			}
			if ve.Code != tc.want {
				t.Fatalf("got failure code %q, want %q", ve.Code, tc.want)
			}

			for _, table := range []string{"synthesis_runs", "synthesis_outputs", "synthesis_output_insights"} {
				var n int
				if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
					t.Fatalf("count %s: %v", table, err)
				}
				if n != 0 {
					t.Fatalf("%s holds %d rows after a rejected candidate; persistence must not be entered", table, n)
				}
			}
		})
	}
}

func errorAs(err error, target **intelligence.SynthesisValidationError) bool {
	return errors.As(err, target)
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		func() bool {
			for i := 0; i+len(needle) <= len(haystack); i++ {
				if haystack[i:i+len(needle)] == needle {
					return true
				}
			}
			return false
		}()
}
