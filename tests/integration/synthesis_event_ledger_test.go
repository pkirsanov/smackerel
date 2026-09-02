//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
	"github.com/smackerel/smackerel/internal/intelligence"
)

// seedCausalEventAttempt creates one linked run and attempt using only safe,
// content-free values. It deliberately writes through PostgreSQL so the tests
// exercise the migration constraints rather than a Go-only representation.
func seedCausalEventAttempt(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	principal, cadence string,
	attemptNo int,
) string {
	t.Helper()
	now := time.Now().UTC()
	runID := ulid.Make().String()
	logicalKey := ulid.Make().String()

	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_runs (
			id, logical_key, cadence, principal, window_start, window_end,
			policy_version, source_set_digest, state, created_at, updated_at,
			lifecycle_state, attempt_count, lease_holder, lease_expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'test-policy/v1', $7,
			'running', $8, $8, 'current', $9, 'test-holder', $10)
	`, runID, logicalKey, cadence, principal, now.Add(-24*time.Hour), now,
		ulid.Make().String(), now, attemptNo, now.Add(time.Minute)); err != nil {
		t.Fatalf("seed causal run: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_attempts (
			logical_key, outcome, recorded_at, run_id, attempt_no,
			trigger_kind, state, started_at, included_source_classes,
			omitted_source_classes, insight_count, citation_count
		) VALUES ($1, 'running', $2, $3, $4, 'scheduled', 'running', $2,
			'{}'::text[], '{}'::text[], 0, 0)
	`, logicalKey, now, runID, attemptNo); err != nil {
		t.Fatalf("seed causal attempt: %v", err)
	}
	return runID
}

// SCN-004-004-C11 / T004-C11-IMMUTABLE. The database, not application
// convention, rejects mutation and deletion. The column audit also prevents a
// later migration from quietly turning the event ledger into a content store.
func TestSynthesisRunEvents_RejectUpdateDeleteAndContentLeakage(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	ctx := context.Background()
	runID := seedCausalEventAttempt(t, pool, ctx, "test-event-immutability", "daily", 1)
	now := time.Now().UTC()
	eventID := ulid.Make().String()
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_events
			(id, run_id, attempt_no, event_type, created_at)
		VALUES ($1, $2, 1, 'attempt_started', $3)
	`, eventID, runID, now); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	if _, err := pool.Exec(ctx,
		`UPDATE synthesis_run_events SET event_type = 'failed' WHERE id = $1`, eventID,
	); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("event UPDATE was not rejected by the immutable ledger trigger: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`DELETE FROM synthesis_run_events WHERE id = $1`, eventID,
	); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("event DELETE was not rejected by the immutable ledger trigger: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'synthesis_run_events'
	`)
	if err != nil {
		t.Fatalf("read event columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan event column: %v", err)
		}
		for _, forbidden := range []string{"text", "title", "artifact", "source_set", "principal", "raw_error", "sql"} {
			if strings.Contains(column, forbidden) {
				t.Fatalf("event ledger exposes forbidden content-bearing column %q", column)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate event columns: %v", err)
	}
}

// SCN-004-004-C11 / T004-C11-CAUSAL. Started and terminal events must resolve
// through the composite attempt identity, and a second terminal result for the
// same attempt must fail at the database boundary.
func TestSynthesisRunEvents_LinkStartedAndTerminalEventToOneAttempt(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)
	ctx := context.Background()
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer := newSynthesisTestProducer(t, &intelligence.Engine{Pool: pool}, persistence,
		"test-event-causality", "event-causality-holder")
	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily,
		intelligence.TriggerScheduled, time.Now().UTC())
	if err != nil {
		t.Fatalf("run real synthesis trigger: %v", err)
	}

	var started, terminal int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed'
			))
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = 1
	`, agg.RunID).Scan(&started, &terminal); err != nil {
		t.Fatalf("read event chain: %v", err)
	}
	if started != 1 || terminal != 1 {
		t.Fatalf("event chain has started=%d terminal=%d, want exactly 1 each", started, terminal)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_events
			(id, run_id, attempt_no, event_type, created_at)
		VALUES ($1, $2, 1, 'failed', $3)
	`, ulid.Make().String(), agg.RunID, time.Now().UTC().Add(time.Millisecond)); err == nil {
		t.Fatal("a second terminal event for one attempt was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_events
			(id, run_id, attempt_no, event_type, created_at)
		VALUES ($1, $2, 2, 'attempt_started', $3)
	`, ulid.Make().String(), agg.RunID, time.Now().UTC()); err == nil {
		t.Fatal("an event referencing a missing run attempt was accepted")
	}
}

// SCN-004-004-C14 / T004-C14-ACTOR-CADENCE. Interleaved actors and cadences
// must retain independent run, attempt, output, and event histories even when
// attempt numbers overlap and another actor has the globally newest event.
func TestSynthesisAttempts_DoNotCrossPairActorsCadencesRunsOrAttempts(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	ctx := context.Background()
	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	type runFixture struct {
		runID       string
		logicalKey  string
		outputID    string
		principal   string
		cadence     string
		outputKind  string
		windowStart time.Time
		windowEnd   time.Time
		createdAt   time.Time
		attempts    int
	}

	daily := runFixture{
		runID:       ulid.Make().String(),
		logicalKey:  "test-scn-c14-daily-logical-" + ulid.Make().String(),
		outputID:    ulid.Make().String(),
		principal:   "test-scn-c14-actor-alpha",
		cadence:     "daily",
		outputKind:  "full",
		windowStart: base.Add(-24 * time.Hour),
		windowEnd:   base,
		createdAt:   base.Add(15 * time.Minute),
		attempts:    3,
	}
	weekly := runFixture{
		runID:       ulid.Make().String(),
		logicalKey:  "test-scn-c14-weekly-logical-" + ulid.Make().String(),
		outputID:    ulid.Make().String(),
		principal:   "test-scn-c14-actor-beta",
		cadence:     "weekly",
		outputKind:  "quiet",
		windowStart: base.Add(-7 * 24 * time.Hour),
		windowEnd:   base,
		createdAt:   base.Add(45 * time.Minute),
		attempts:    2,
	}

	for _, run := range []runFixture{weekly, daily} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO synthesis_runs (
				id, logical_key, cadence, principal, window_start, window_end,
				policy_version, source_set_digest, state, created_at, updated_at,
				lifecycle_state, attempt_count
			) VALUES ($1, $2, $3, $4, $5, $6, 'test-policy/v1', $7,
				'succeeded', $8, $8, 'current', $9)
		`, run.runID, run.logicalKey, run.cadence, run.principal,
			run.windowStart, run.windowEnd, "test-scn-c14-source-"+ulid.Make().String(),
			run.createdAt, run.attempts); err != nil {
			t.Fatalf("seed %s run: %v", run.cadence, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO synthesis_outputs (
				id, run_id, output_kind, insight_count, citation_count,
				evaluated_artifact_count, created_at, principal, cadence,
				window_start, window_end, lifecycle_state
			) VALUES ($1, $2, $3, 0, 0, 1, $4, $5, $6, $7, $8, 'current')
		`, run.outputID, run.runID, run.outputKind, run.createdAt,
			run.principal, run.cadence, run.windowStart, run.windowEnd); err != nil {
			t.Fatalf("seed %s output: %v", run.cadence, err)
		}
	}

	type attemptFixture struct {
		runID      string
		logicalKey string
		outputID   string
		attemptNo  int
		trigger    string
		state      string
		outcome    string
		terminal   string
		startedAt  time.Time
		finishedAt time.Time
	}

	// Insertion order alternates actors/cadences and is intentionally unrelated
	// to event time. Attempt 1 overlaps; attempt 2 exists only on the weekly run.
	attempts := []attemptFixture{
		{weekly.runID, weekly.logicalKey, weekly.outputID, 2, "operator_retry", "idempotent", "idempotent_no_change", "idempotent", base.Add(110 * time.Minute), base.Add(120 * time.Minute)},
		{daily.runID, daily.logicalKey, daily.outputID, 1, "scheduled", "persisted", "succeeded", "persisted", base.Add(10 * time.Minute), base.Add(20 * time.Minute)},
		{weekly.runID, weekly.logicalKey, weekly.outputID, 1, "scheduled", "quiet", "succeeded", "quiet", base.Add(40 * time.Minute), base.Add(50 * time.Minute)},
		{daily.runID, daily.logicalKey, daily.outputID, 3, "operator_retry", "idempotent", "idempotent_no_change", "idempotent", base.Add(70 * time.Minute), base.Add(80 * time.Minute)},
	}

	for _, attempt := range attempts {
		if _, err := pool.Exec(ctx, `
			INSERT INTO synthesis_run_attempts (
				logical_key, outcome, recorded_at, run_id, attempt_no,
				trigger_kind, state, output_id, started_at, finished_at,
				included_source_classes, omitted_source_classes,
				insight_count, citation_count
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $3, $9,
				'{}'::text[], '{}'::text[], 0, 0)
		`, attempt.logicalKey, attempt.outcome, attempt.startedAt, attempt.runID,
			attempt.attemptNo, attempt.trigger, attempt.state, attempt.outputID,
			attempt.finishedAt); err != nil {
			t.Fatalf("seed attempt %s/%d: %v", attempt.runID, attempt.attemptNo, err)
		}
	}

	// Attempt 2 exists, but only for the weekly run. Combining it with the daily
	// run must fail the composite (run_id, attempt_no) event foreign key.
	_, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_events
			(id, run_id, attempt_no, event_type, created_at)
		VALUES ($1, $2, 2, 'attempt_started', $3)
	`, ulid.Make().String(), daily.runID, base.Add(60*time.Minute))
	if err == nil {
		t.Fatal("event ledger accepted an attempt number belonging to another run")
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		t.Fatalf("cross-run attempt rejection returned non-PostgreSQL error: %v", err)
	}
	if postgresError.Code != "23503" || postgresError.ConstraintName != "synthesis_run_events_attempt_fk" {
		t.Fatalf("cross-run attempt error = code %s constraint %s, want event-attempt FK", postgresError.Code, postgresError.ConstraintName)
	}

	// The daily attempt exists, but the selected output belongs to the weekly
	// run. The output/run composite FK must reject this otherwise-valid terminal.
	_, err = pool.Exec(ctx, `
		INSERT INTO synthesis_run_events (
			id, run_id, attempt_no, event_type, output_id,
			insight_count, citation_count, created_at
		) VALUES ($1, $2, 1, 'persisted', $3, 0, 0, $4)
	`, ulid.Make().String(), daily.runID, weekly.outputID, base.Add(20*time.Minute))
	if err == nil {
		t.Fatal("event ledger accepted an output belonging to another run")
	}
	postgresError = nil
	if !errors.As(err, &postgresError) {
		t.Fatalf("cross-run output rejection returned non-PostgreSQL error: %v", err)
	}
	if postgresError.Code != "23503" || postgresError.ConstraintName != "synthesis_run_events_output_run_fk" {
		t.Fatalf("cross-run output error = code %s constraint %s, want event-output/run FK", postgresError.Code, postgresError.ConstraintName)
	}

	for _, attempt := range attempts {
		if _, err := pool.Exec(ctx, `
			INSERT INTO synthesis_run_events
				(id, run_id, attempt_no, event_type, created_at)
			VALUES ($1, $2, $3, 'attempt_started', $4)
		`, ulid.Make().String(), attempt.runID, attempt.attemptNo, attempt.startedAt); err != nil {
			t.Fatalf("seed attempt_started %s/%d: %v", attempt.runID, attempt.attemptNo, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO synthesis_run_events (
				id, run_id, attempt_no, event_type, output_id,
				insight_count, citation_count, created_at
			) VALUES ($1, $2, $3, $4, $5, 0, 0, $6)
		`, ulid.Make().String(), attempt.runID, attempt.attemptNo,
			attempt.terminal, attempt.outputID, attempt.finishedAt); err != nil {
			t.Fatalf("seed terminal %s/%d: %v", attempt.runID, attempt.attemptNo, err)
		}
	}

	// The production History method does not yet accept actor/cadence. Exercise
	// the future event-rooted query shape directly: event -> attempt -> run and
	// the composite event-output/run relation, with scope applied before order.
	rows, err := pool.Query(ctx, `
		SELECT e.run_id, e.attempt_no, e.event_type, o.id, e.created_at
		FROM synthesis_run_events e
		JOIN synthesis_run_attempts a
			ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
		JOIN synthesis_runs r ON r.id = a.run_id
		JOIN synthesis_outputs o ON o.id = e.output_id AND o.run_id = e.run_id
		WHERE r.principal = $1 AND r.cadence = $2
			AND e.event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed', 'recovered'
			)
		ORDER BY e.created_at DESC, e.id DESC
	`, daily.principal, daily.cadence)
	if err != nil {
		t.Fatalf("query actor/cadence history: %v", err)
	}

	type historyEntry struct {
		runID     string
		attemptNo int
		eventType string
		outputID  string
		createdAt time.Time
	}
	var history []historyEntry
	for rows.Next() {
		var entry historyEntry
		if err := rows.Scan(&entry.runID, &entry.attemptNo, &entry.eventType,
			&entry.outputID, &entry.createdAt); err != nil {
			rows.Close()
			t.Fatalf("scan actor/cadence history: %v", err)
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate actor/cadence history: %v", err)
	}
	rows.Close()

	expectedHistory := []historyEntry{
		{daily.runID, 3, "idempotent", daily.outputID, base.Add(80 * time.Minute)},
		{daily.runID, 1, "persisted", daily.outputID, base.Add(20 * time.Minute)},
	}
	if len(history) != len(expectedHistory) {
		t.Fatalf("daily actor history length = %d, want %d: %+v", len(history), len(expectedHistory), history)
	}
	for index, expected := range expectedHistory {
		actual := history[index]
		if actual.runID != expected.runID || actual.attemptNo != expected.attemptNo ||
			actual.eventType != expected.eventType || actual.outputID != expected.outputID ||
			!actual.createdAt.Equal(expected.createdAt) {
			t.Fatalf("daily actor history[%d] = %+v, want %+v", index, actual, expected)
		}
	}

	var globallyNewestRun string
	var globallyNewestAttempt int
	if err := pool.QueryRow(ctx, `
		SELECT run_id, attempt_no
		FROM synthesis_run_events
		WHERE event_type IN (
			'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
			'retryable_failure', 'failed', 'readback_failed', 'recovered'
		)
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`).Scan(&globallyNewestRun, &globallyNewestAttempt); err != nil {
		t.Fatalf("read globally newest terminal event: %v", err)
	}
	if globallyNewestRun != weekly.runID || globallyNewestAttempt != 2 {
		t.Fatalf("global negative control = %s/%d, want newer weekly %s/2", globallyNewestRun, globallyNewestAttempt, weekly.runID)
	}
	if history[0].runID == globallyNewestRun {
		t.Fatal("actor/cadence-scoped history selected the globally newer weekly event")
	}

	type attemptKey struct {
		runID     string
		attemptNo int
	}
	expectedAttempts := make(map[attemptKey]struct{}, len(attempts))
	for _, attempt := range attempts {
		expectedAttempts[attemptKey{attempt.runID, attempt.attemptNo}] = struct{}{}
	}

	rows, err = pool.Query(ctx, `
		SELECT a.run_id, a.attempt_no,
			COUNT(*) FILTER (WHERE e.event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE e.event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed', 'recovered'
			))
		FROM synthesis_run_attempts a
		JOIN synthesis_run_events e
			ON e.run_id = a.run_id AND e.attempt_no = a.attempt_no
		WHERE a.run_id IN ($1, $2)
		GROUP BY a.run_id, a.attempt_no
		ORDER BY a.run_id, a.attempt_no
	`, daily.runID, weekly.runID)
	if err != nil {
		t.Fatalf("query event-chain cardinality: %v", err)
	}
	for rows.Next() {
		var key attemptKey
		var started, terminal int
		if err := rows.Scan(&key.runID, &key.attemptNo, &started, &terminal); err != nil {
			rows.Close()
			t.Fatalf("scan event-chain cardinality: %v", err)
		}
		if _, ok := expectedAttempts[key]; !ok {
			rows.Close()
			t.Fatalf("unexpected event chain for %s/%d", key.runID, key.attemptNo)
		}
		if started != 1 || terminal != 1 {
			rows.Close()
			t.Fatalf("event chain %s/%d has started=%d terminal=%d, want exactly 1 each", key.runID, key.attemptNo, started, terminal)
		}
		delete(expectedAttempts, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatalf("iterate event-chain cardinality: %v", err)
	}
	rows.Close()
	if len(expectedAttempts) != 0 {
		t.Fatalf("event chains missing for %d seeded attempts: %+v", len(expectedAttempts), expectedAttempts)
	}

	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM synthesis_run_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count causal events: %v", err)
	}
	if eventCount != len(attempts)*2 {
		t.Fatalf("event count = %d, want %d (one started and one terminal per attempt)", eventCount, len(attempts)*2)
	}
}
