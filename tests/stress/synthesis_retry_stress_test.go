//go:build stress

// BUG-004-004 SCOPE-03 — T004-03-STRESS. Concurrent triggers for one window.
//
// The integration coverage claims two holders cannot both win a window, but it
// proves it SEQUENTIALLY: holder A claims, then holder B is refused. That is a
// weaker statement than it appears. Sequential calls cannot exercise the
// advisory lock at all, because there is never a second transaction inside the
// critical section. A coordinator that dropped the lock entirely and relied on
// the read-then-write alone would pass the sequential test and lose windows to
// a real race.
//
// This runs N genuinely concurrent claims against one window and asserts
// exactly one wins.

package stress

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/db"
	"github.com/smackerel/smackerel/internal/intelligence"
)

const (
	synthesisConcurrentHolders      = 16
	synthesisStressAdminConnections = 1
	synthesisStressMaxConnections   = synthesisConcurrentHolders + 8
	synthesisStressOperationTimeout = 15 * time.Second
	synthesisStressMigrationTimeout = 60 * time.Second
)

func synthesisStressPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		// Fail rather than skip: a silent skip would let this stop running and
		// nobody would notice the concurrency claim had gone unproven.
		t.Fatal("stress: DATABASE_URL is empty; T004-03-STRESS needs a real PostgreSQL to exercise the advisory lock")
	}

	adminConfig := parseSynthesisStressPoolConfig(t, databaseURL)
	adminConfig.MaxConns = synthesisStressAdminConnections
	adminConfig.MinConns = 0
	adminPool := connectSynthesisStressPool(t, adminConfig, "admin")

	databaseName := "test_synthesis_stress_" + strings.ToLower(ulid.Make().String())
	quotedDatabase := pgx.Identifier{databaseName}.Sanitize()
	var isolatedPool *pgxpool.Pool
	databaseCreated := false
	t.Cleanup(func() {
		if isolatedPool != nil {
			isolatedPool.Close()
		}
		if databaseCreated {
			cleanupCtx, cleanupCancel := context.WithTimeout(
				context.Background(), synthesisStressOperationTimeout)
			_, cleanupErr := adminPool.Exec(
				cleanupCtx, "DROP DATABASE IF EXISTS "+quotedDatabase+" WITH (FORCE)")
			cleanupCancel()
			if cleanupErr != nil {
				t.Errorf("stress: drop isolated database %s: %v", databaseName, cleanupErr)
			} else {
				t.Logf("stress: dropped isolated PostgreSQL database %s", databaseName)
			}
		}
		adminPool.Close()
	})

	createCtx, createCancel := context.WithTimeout(
		context.Background(), synthesisStressOperationTimeout)
	_, err := adminPool.Exec(
		createCtx, "CREATE DATABASE "+quotedDatabase+" TEMPLATE template0")
	createCancel()
	if err != nil {
		t.Fatalf("stress: create isolated database %s: %v", databaseName, err)
	}
	databaseCreated = true

	isolatedConfig := parseSynthesisStressPoolConfig(t, databaseURL)
	isolatedConfig.ConnConfig.Database = databaseName
	isolatedConfig.MaxConns = synthesisStressMaxConnections
	isolatedConfig.MinConns = 0
	isolatedPool = connectSynthesisStressPool(t, isolatedConfig, "isolated")

	migrationCtx, migrationCancel := context.WithTimeout(
		context.Background(), synthesisStressMigrationTimeout)
	err = db.Migrate(migrationCtx, isolatedPool)
	migrationCancel()
	if err != nil {
		t.Fatalf("stress: migrate isolated database %s: %v", databaseName, err)
	}
	t.Logf("stress: migrated isolated PostgreSQL database %s", databaseName)
	return isolatedPool
}

func parseSynthesisStressPoolConfig(t *testing.T, databaseURL string) *pgxpool.Config {
	t.Helper()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("stress: parse canonical DATABASE_URL: %v", err)
	}
	return config
}

func connectSynthesisStressPool(
	t *testing.T,
	config *pgxpool.Config,
	purpose string,
) *pgxpool.Pool {
	t.Helper()
	connectCtx, connectCancel := context.WithTimeout(
		context.Background(), synthesisStressOperationTimeout)
	pool, err := pgxpool.NewWithConfig(connectCtx, config)
	if err == nil {
		err = pool.Ping(connectCtx)
	}
	connectCancel()
	if err != nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatalf("stress: connect %s pool: %v", purpose, err)
	}
	return pool
}

func TestSynthesisConcurrentClaims_ExactlyOneHolderWins(t *testing.T) {
	pool := synthesisStressPool(t)
	ctx := context.Background()

	const reset = `
		TRUNCATE synthesis_run_events, synthesis_citations,
			synthesis_output_insights, synthesis_output_source_classes,
			synthesis_outputs, synthesis_run_attempts, synthesis_runs
		RESTART IDENTITY CASCADE`
	if _, err := pool.Exec(ctx, reset); err != nil {
		t.Fatalf("clear synthesis stress tables: %v", err)
	}

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}

	key := intelligence.SynthesisRunKey{
		Cadence:       intelligence.CadenceDaily,
		Principal:     "stress-contended",
		WindowStart:   time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC),
		PolicyVersion: "synthesis/v1",
		SourceIDs:     []string{"art-a", "art-b"},
	}
	policy := intelligence.SynthesisRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		LeaseTTL:     time.Minute,
	}

	// A shared start gate, so the holders arrive together rather than trickling
	// in. Without it the goroutines would serialise on scheduling and this would
	// degrade into the sequential test it exists to strengthen.
	var start sync.WaitGroup
	start.Add(1)

	var wg sync.WaitGroup
	results := make([]error, synthesisConcurrentHolders)
	now := time.Now().UTC()

	for i := 0; i < synthesisConcurrentHolders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			coord, err := intelligence.NewSynthesisCoordinator(
				persistence, policy, "stress-holder-"+string(rune('a'+idx)))
			if err != nil {
				results[idx] = err
				return
			}
			start.Wait()
			results[idx] = coord.ClaimWindow(ctx, key, now)
		}(i)
	}

	start.Done()
	wg.Wait()

	var winners, refused, unexpected int
	for i, err := range results {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, intelligence.ErrRunClaimedElsewhere):
			refused++
		default:
			unexpected++
			t.Errorf("holder %d failed unexpectedly: %v", i, err)
		}
	}

	if winners != 1 {
		t.Fatalf("%d holders won the same window, want exactly 1 (refused=%d, unexpected=%d); duplicate winners produce duplicate work",
			winners, refused, unexpected)
	}
	if refused != synthesisConcurrentHolders-1 {
		t.Fatalf("refused=%d, want %d; every loser must be told it lost rather than proceeding",
			refused, synthesisConcurrentHolders-1)
	}

	// One row, one holder. The database is the arbiter, not process memory.
	var runs int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_runs WHERE logical_key = $1`, key.LogicalKey()).Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runs != 1 {
		t.Fatalf("got %d run rows for one logical window, want 1", runs)
	}

	var attemptCount int
	var holder string
	if err := pool.QueryRow(ctx,
		`SELECT attempt_count, COALESCE(lease_holder, '') FROM synthesis_runs WHERE logical_key = $1`,
		key.LogicalKey()).Scan(&attemptCount, &holder); err != nil {
		t.Fatalf("read claim: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("attempt_count is %d after one successful claim among %d racers, want 1; a losing holder incremented the budget",
			attemptCount, synthesisConcurrentHolders)
	}
	if holder == "" {
		t.Fatal("winning claim recorded no lease holder")
	}
}

// SCN-004-004-C14 / T004-C14-RACE. A production producer establishes the
// current output from a real eligible cluster. The public producer API has no
// hook between source discovery and commit, so the race phase uses the public
// StartAttempt/CommitAttempt boundary: all four attempts are durably started,
// then one channel releases same-source replays and changed-source replacements
// into the production actor/cadence/window lock together.
func TestSynthesisSameAndChangedSourceRacesLeaveOneCurrentAndCompleteEventChains(t *testing.T) {
	pool := synthesisStressPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const reset = `
		TRUNCATE synthesis_run_events, synthesis_citations,
			synthesis_output_insights, synthesis_output_source_classes,
			synthesis_outputs, synthesis_run_attempts, synthesis_runs
		RESTART IDENTITY CASCADE;
		DELETE FROM edges;
		DELETE FROM artifacts;
		DELETE FROM topics;`
	if _, err := pool.Exec(ctx, reset); err != nil {
		t.Fatalf("reset synthesis race state: %v", err)
	}

	const topicID = "test-scn-c14-topic"
	if _, err := pool.Exec(ctx,
		`INSERT INTO topics (id, name) VALUES ($1, $2)`,
		topicID, "test-scn-c14-cross-domain-cluster"); err != nil {
		t.Fatalf("seed synthesis race topic: %v", err)
	}

	baseSources := []string{
		"test-scn-c14-artifact-email",
		"test-scn-c14-artifact-article",
		"test-scn-c14-artifact-video",
	}
	sourceKinds := []string{
		"test-scn-c14-source-email",
		"test-scn-c14-source-article",
		"test-scn-c14-source-video",
	}
	for index, artifactID := range baseSources {
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
			VALUES ($1, 'article', $2, $3, $4)
		`, artifactID, "test-scn-c14-eligible-artifact",
			artifactID+"-content-hash", sourceKinds[index]); err != nil {
			t.Fatalf("seed synthesis race artifact %d: %v", index, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO edges (id, src_id, src_type, dst_id, dst_type, edge_type)
			VALUES ($1, $2, 'artifact', $3, 'topic', 'BELONGS_TO')
		`, "test-scn-c14-edge-"+artifactID, artifactID, topicID); err != nil {
			t.Fatalf("seed synthesis race edge %d: %v", index, err)
		}
	}

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct synthesis race persistence: %v", err)
	}
	engine := &intelligence.Engine{Pool: pool}
	retryPolicy := intelligence.SynthesisRetryPolicy{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		LeaseTTL:     time.Minute,
	}
	coordinator, err := intelligence.NewSynthesisCoordinator(
		persistence, retryPolicy, "test-scn-c14-baseline-holder")
	if err != nil {
		t.Fatalf("construct synthesis race coordinator: %v", err)
	}
	const actor = "test-scn-c14-actor"
	const policyVersion = "test-scn-c14-policy/v1"
	runPolicy := intelligence.SynthesisRunPolicy{
		Actor:                 actor,
		PolicyVersion:         policyVersion,
		RequiredSourceClasses: []string{"canonical-graph"},
		OptionalSourceClasses: []string{},
		Retention:             90 * 24 * time.Hour,
	}
	producer, err := intelligence.NewSynthesisProducer(engine, persistence, runPolicy)
	if err != nil {
		t.Fatalf("construct synthesis race producer: %v", err)
	}
	producer.WithCoordinator(coordinator)

	runAt := time.Now().UTC()
	window, err := intelligence.WindowFor(intelligence.CadenceDaily, runAt)
	if err != nil {
		t.Fatalf("derive synthesis race window: %v", err)
	}
	baseKey := intelligence.SynthesisRunKey{
		Cadence:       intelligence.CadenceDaily,
		Principal:     actor,
		WindowStart:   window.Start,
		WindowEnd:     window.End,
		PolicyVersion: policyVersion,
		SourceIDs:     append([]string(nil), baseSources...),
	}
	baseline, err := producer.RunAndPersist(
		ctx, intelligence.CadenceDaily, intelligence.TriggerScheduled, runAt)
	if err != nil {
		t.Fatalf("persist synthesis race baseline: %v", err)
	}
	if baseline.Kind != intelligence.OutputKindFull || baseline.InsightCount == 0 {
		t.Fatalf("baseline kind/insights = %s/%d, want full with real cluster output",
			baseline.Kind, baseline.InsightCount)
	}
	if baseline.LogicalKey != baseKey.LogicalKey() {
		t.Fatalf("baseline logical key %s does not match controlled source set %s",
			baseline.LogicalKey, baseKey.LogicalKey())
	}

	const changedSourceID = "test-scn-c14-artifact-late"
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, 'article', $2, $3, $4)
	`, changedSourceID, "test-scn-c14-late-authorized-artifact",
		changedSourceID+"-content-hash", "test-scn-c14-source-late"); err != nil {
		t.Fatalf("seed deterministic changed source: %v", err)
	}

	changedSources := append(append([]string(nil), baseSources...), changedSourceID)
	changedKey := baseKey
	changedKey.SourceIDs = changedSources
	if changedKey.LogicalKey() == baseKey.LogicalKey() {
		t.Fatal("changed source set did not produce a distinct replay identity")
	}
	changedInsights, err := engine.RunSynthesis(ctx)
	if err != nil {
		t.Fatalf("build changed-source synthesis candidate: %v", err)
	}
	if len(changedInsights) == 0 {
		t.Fatal("changed-source phase produced no insight from the eligible cluster")
	}

	sourcePolicy := intelligence.SourceClassPolicy{
		Required: []string{"canonical-graph"},
		Optional: []string{},
	}
	baseCandidate := intelligence.SynthesisCandidate{
		Key:                    baseKey,
		Kind:                   intelligence.OutputKindFull,
		Insights:               append([]intelligence.SynthesisInsight(nil), baseline.Insights...),
		EvaluatedArtifactCount: len(baseSources),
		IncludedClasses:        []string{"canonical-graph"},
	}
	changedCandidate := intelligence.SynthesisCandidate{
		Key:                    changedKey,
		Kind:                   intelligence.OutputKindFull,
		Insights:               changedInsights,
		EvaluatedArtifactCount: len(changedSources),
		IncludedClasses:        []string{"canonical-graph"},
	}

	type raceTrigger struct {
		label      string
		holder     string
		trigger    intelligence.SynthesisTriggerKind
		key        intelligence.SynthesisRunKey
		candidate  intelligence.SynthesisCandidate
		authorized []string
		attempt    intelligence.SynthesisAttempt
		aggregate  *intelligence.SynthesisAggregate
		err        error
	}
	triggers := []raceTrigger{
		{
			label: "same-source scheduled", holder: "test-scn-c14-same-scheduled",
			trigger: intelligence.TriggerScheduled, key: baseKey,
			candidate: baseCandidate, authorized: append([]string(nil), baseSources...),
		},
		{
			label: "same-source operator", holder: "test-scn-c14-same-operator",
			trigger: intelligence.TriggerOperatorRetry, key: baseKey,
			candidate: baseCandidate, authorized: append([]string(nil), baseSources...),
		},
		{
			label: "changed-source scheduled", holder: "test-scn-c14-changed-scheduled",
			trigger: intelligence.TriggerScheduled, key: changedKey,
			candidate: changedCandidate, authorized: append([]string(nil), changedSources...),
		},
		{
			label: "changed-source operator", holder: "test-scn-c14-changed-operator",
			trigger: intelligence.TriggerOperatorRetry, key: changedKey,
			candidate: changedCandidate, authorized: append([]string(nil), changedSources...),
		},
	}

	// Stage every causal attempt before opening the commit gate. This avoids a
	// scheduler-timing proxy: every racer is known to have its own durable
	// attempt_started event before any contender can acquire the commit lock.
	for index := range triggers {
		triggers[index].attempt, err = persistence.StartAttempt(
			ctx, triggers[index].key, triggers[index].trigger,
			triggers[index].holder, retryPolicy.LeaseTTL,
			runAt.Add(time.Duration(index+1)*time.Millisecond))
		if err != nil {
			t.Fatalf("start %s attempt: %v", triggers[index].label, err)
		}
	}

	ready := make(chan struct{}, len(triggers))
	commitGate := make(chan struct{})
	var racers sync.WaitGroup
	for index := range triggers {
		index := index
		racers.Add(1)
		go func() {
			defer racers.Done()
			ready <- struct{}{}
			select {
			case <-commitGate:
			case <-ctx.Done():
				triggers[index].err = ctx.Err()
				return
			}
			triggers[index].aggregate, triggers[index].err = persistence.CommitAttempt(
				ctx, triggers[index].attempt, triggers[index].candidate,
				sourcePolicy, triggers[index].authorized,
				runAt.Add(time.Minute+time.Duration(index)*time.Millisecond))
		}()
	}
	for range triggers {
		select {
		case <-ready:
		case <-ctx.Done():
			close(commitGate)
			racers.Wait()
			t.Fatalf("race participants did not reach the commit barrier: %v", ctx.Err())
		}
	}
	close(commitGate)
	racers.Wait()

	var unexpectedErrors []string
	for index := range triggers {
		if triggers[index].err != nil {
			detail := triggers[index].err.Error()
			var auditErr *intelligence.SynthesisAuditPersistenceError
			if errors.As(triggers[index].err, &auditErr) && auditErr.Cause != nil {
				detail += ": " + auditErr.Cause.Error()
			}
			unexpectedErrors = append(unexpectedErrors,
				triggers[index].label+": "+detail)
		}
	}
	if len(unexpectedErrors) != 0 {
		t.Fatalf("concurrent synthesis triggers returned unexpected errors: %s",
			strings.Join(unexpectedErrors, "; "))
	}

	for _, index := range []int{0, 1} {
		aggregate := triggers[index].aggregate
		if aggregate == nil {
			t.Fatalf("%s returned no aggregate", triggers[index].label)
		}
		if aggregate.OutputID != baseline.OutputID || aggregate.RunID != baseline.RunID ||
			aggregate.Outcome != intelligence.EventIdempotent {
			t.Fatalf("%s resolved output/run/outcome %s/%s/%s, want baseline %s/%s/idempotent",
				triggers[index].label, aggregate.OutputID, aggregate.RunID,
				aggregate.Outcome, baseline.OutputID, baseline.RunID)
		}
	}

	changedOutputID := triggers[2].aggregate.OutputID
	changedRunID := triggers[2].aggregate.RunID
	if changedOutputID == baseline.OutputID || changedRunID == baseline.RunID {
		t.Fatalf("changed source reused baseline output/run %s/%s",
			changedOutputID, changedRunID)
	}
	if triggers[3].aggregate.OutputID != changedOutputID ||
		triggers[3].aggregate.RunID != changedRunID {
		t.Fatalf("changed-source racers diverged: first=%s/%s second=%s/%s",
			changedOutputID, changedRunID,
			triggers[3].aggregate.OutputID, triggers[3].aggregate.RunID)
	}
	changedPersisted := 0
	changedIdempotent := 0
	for _, index := range []int{2, 3} {
		switch triggers[index].aggregate.Outcome {
		case intelligence.EventPersisted:
			changedPersisted++
		case intelligence.EventIdempotent:
			changedIdempotent++
		default:
			t.Fatalf("%s outcome=%s, want persisted or idempotent",
				triggers[index].label, triggers[index].aggregate.Outcome)
		}
	}
	if changedPersisted < 1 || changedPersisted+changedIdempotent != 2 {
		t.Fatalf("changed-source outcomes persisted=%d idempotent=%d, want at least one persisted and two terminal outcomes",
			changedPersisted, changedIdempotent)
	}

	var runCount, sourceDigestCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT source_set_digest)
		FROM synthesis_runs
		WHERE principal = $1 AND cadence = $2
			AND window_start = $3 AND window_end = $4
	`, actor, string(intelligence.CadenceDaily), window.Start, window.End).Scan(
		&runCount, &sourceDigestCount); err != nil {
		t.Fatalf("read synthesis race run histories: %v", err)
	}
	if runCount != 2 || sourceDigestCount != 2 {
		t.Fatalf("run/source histories=%d/%d, want two retained source identities",
			runCount, sourceDigestCount)
	}

	var outputCount, currentCount int
	var currentOutputID string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE lifecycle_state = 'current'),
			COALESCE(MAX(id) FILTER (WHERE lifecycle_state = 'current'), '')
		FROM synthesis_outputs
		WHERE principal = $1 AND cadence = $2
			AND window_start = $3 AND window_end = $4
	`, actor, string(intelligence.CadenceDaily), window.Start, window.End).Scan(
		&outputCount, &currentCount, &currentOutputID); err != nil {
		t.Fatalf("read synthesis race output histories: %v", err)
	}
	if outputCount != 2 || currentCount != 1 || currentOutputID != changedOutputID {
		t.Fatalf("output histories/current/current-id=%d/%d/%s, want 2/1/%s",
			outputCount, currentCount, currentOutputID, changedOutputID)
	}

	var priorLifecycle, changedLifecycle string
	if err := pool.QueryRow(ctx, `
		SELECT prior.lifecycle_state, changed.lifecycle_state
		FROM synthesis_outputs prior
		JOIN synthesis_outputs changed ON changed.id = $2
		WHERE prior.id = $1
	`, baseline.OutputID, changedOutputID).Scan(&priorLifecycle, &changedLifecycle); err != nil {
		t.Fatalf("read synthesis race lifecycle states: %v", err)
	}
	if priorLifecycle != "superseded" || changedLifecycle != "current" {
		t.Fatalf("prior/changed lifecycle=%s/%s, want superseded/current",
			priorLifecycle, changedLifecycle)
	}

	var attemptCount, incompleteChains int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE started_count <> 1 OR terminal_count <> 1)
		FROM (
			SELECT a.run_id, a.attempt_no,
				COUNT(*) FILTER (WHERE e.event_type = 'attempt_started') AS started_count,
				COUNT(*) FILTER (WHERE e.event_type IN (
					'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
					'retryable_failure', 'failed', 'readback_failed', 'recovered'
				)) AS terminal_count
			FROM synthesis_run_attempts a
			JOIN synthesis_runs r ON r.id = a.run_id
			LEFT JOIN synthesis_run_events e
				ON e.run_id = a.run_id AND e.attempt_no = a.attempt_no
			WHERE r.principal = $1 AND r.cadence = $2
				AND r.window_start = $3 AND r.window_end = $4
			GROUP BY a.run_id, a.attempt_no
		) chains
	`, actor, string(intelligence.CadenceDaily), window.Start, window.End).Scan(
		&attemptCount, &incompleteChains); err != nil {
		t.Fatalf("read synthesis race event-chain cardinality: %v", err)
	}
	if attemptCount != len(triggers)+1 || incompleteChains != 0 {
		t.Fatalf("attempts/incomplete-event-chains=%d/%d, want %d/0",
			attemptCount, incompleteChains, len(triggers)+1)
	}

	var mismatchedTerminalLinks int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM synthesis_run_events e
		JOIN synthesis_runs r ON r.id = e.run_id
		LEFT JOIN synthesis_run_attempts a
			ON a.run_id = e.run_id AND a.attempt_no = e.attempt_no
		LEFT JOIN synthesis_outputs o
			ON o.id = e.output_id AND o.run_id = e.run_id
		WHERE r.principal = $1 AND r.cadence = $2
			AND r.window_start = $3 AND r.window_end = $4
			AND e.event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed', 'recovered'
			)
			AND (a.run_id IS NULL OR e.output_id IS NULL OR o.id IS NULL
				OR a.output_id IS DISTINCT FROM e.output_id)
	`, actor, string(intelligence.CadenceDaily), window.Start, window.End).Scan(
		&mismatchedTerminalLinks); err != nil {
		t.Fatalf("read synthesis race terminal links: %v", err)
	}
	if mismatchedTerminalLinks != 0 {
		t.Fatalf("found %d terminal events without their matching run, attempt, and output",
			mismatchedTerminalLinks)
	}

	var persistedEvents, idempotentEvents, unexpectedTerminalEvents int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE e.event_type = 'persisted'),
			COUNT(*) FILTER (WHERE e.event_type = 'idempotent'),
			COUNT(*) FILTER (WHERE e.event_type IN (
				'quiet', 'partial', 'rolled_back', 'retryable_failure',
				'failed', 'readback_failed', 'recovered'
			))
		FROM synthesis_run_events e
		JOIN synthesis_runs r ON r.id = e.run_id
		WHERE r.principal = $1 AND r.cadence = $2
			AND r.window_start = $3 AND r.window_end = $4
	`, actor, string(intelligence.CadenceDaily), window.Start, window.End).Scan(
		&persistedEvents, &idempotentEvents, &unexpectedTerminalEvents); err != nil {
		t.Fatalf("read synthesis race terminal outcomes: %v", err)
	}
	wantPersistedEvents := 1 + changedPersisted
	wantIdempotentEvents := 2 + changedIdempotent
	if persistedEvents != wantPersistedEvents || idempotentEvents != wantIdempotentEvents || unexpectedTerminalEvents != 0 {
		t.Fatalf("terminal events persisted/idempotent/unexpected=%d/%d/%d, want %d/%d/0",
			persistedEvents, idempotentEvents, unexpectedTerminalEvents,
			wantPersistedEvents, wantIdempotentEvents)
	}

	var supersededEvents int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_run_events
		WHERE run_id = $1 AND event_type = 'superseded'
			AND output_id = $2 AND related_output_id = $3
	`, changedRunID, changedOutputID, baseline.OutputID).Scan(&supersededEvents); err != nil {
		t.Fatalf("read synthesis race superseded event: %v", err)
	}
	if supersededEvents != 1 {
		t.Fatalf("superseded events=%d, want one changed-to-prior output link", supersededEvents)
	}
}
