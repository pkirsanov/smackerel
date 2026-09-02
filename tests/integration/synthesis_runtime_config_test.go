//go:build integration

package integration

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
)

type synthesisRuntimeResult struct {
	aggregate *intelligence.SynthesisAggregate
	err       error
}

type synthesisLeaseObservation struct {
	runID        string
	holder       string
	leaseExpires time.Time
	updatedAt    time.Time
	attemptCount int
}

func loadDistinctSynthesisRuntimeConfig(t *testing.T) *config.Config {
	t.Helper()

	fixtureID := testID(t)
	actor := fixtureID + "-configured-actor"
	policyVersion := "synthesis/runtime-config-" + fixtureID

	t.Setenv("SYNTHESIS_ACTOR_USER_ID", actor)
	t.Setenv("SYNTHESIS_RETRY_BUDGET", "4")
	t.Setenv("SYNTHESIS_RETRY_BACKOFF_SECONDS", "3")
	t.Setenv("SYNTHESIS_RETRY_MAX_BACKOFF_SECONDS", "17")
	t.Setenv("SYNTHESIS_LEASE_SECONDS", "61")
	t.Setenv("SYNTHESIS_POLICY_VERSION", policyVersion)
	t.Setenv("SYNTHESIS_REQUIRED_SOURCE_CLASSES", `["canonical-graph"]`)
	t.Setenv("SYNTHESIS_OPTIONAL_SOURCE_CLASSES", `[]`)
	t.Setenv("SYNTHESIS_RETENTION_SECONDS", "133200")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load generated test environment with distinct synthesis policy: %v", err)
	}
	if cfg.Synthesis.ActorUserID == "global-corpus" || cfg.Synthesis.PolicyVersion == "synthesis/v1" {
		t.Fatalf("runtime-config fixture accidentally retained default-like identity: actor=%q policy=%q", cfg.Synthesis.ActorUserID, cfg.Synthesis.PolicyVersion)
	}
	return cfg
}

func observeRunningSynthesisLease(
	t *testing.T,
	pool *pgxpool.Pool,
	actor string,
	policyVersion string,
	cadence intelligence.SynthesisCadence,
) synthesisLeaseObservation {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		var observation synthesisLeaseObservation
		err := pool.QueryRow(ctx, `
			SELECT id, lease_holder, lease_expires_at, updated_at, attempt_count
			FROM synthesis_runs
			WHERE principal = $1 AND policy_version = $2 AND cadence = $3
				AND state = 'running'
		`, actor, policyVersion, string(cadence)).Scan(
			&observation.runID,
			&observation.holder,
			&observation.leaseExpires,
			&observation.updatedAt,
			&observation.attemptCount,
		)
		switch {
		case err == nil:
			return observation
		case !errors.Is(err, pgx.ErrNoRows):
			t.Fatalf("observe running %s synthesis lease: %v", cadence, err)
		}

		select {
		case <-ctx.Done():
			t.Fatalf("running %s synthesis lease did not become observable: %v", cadence, ctx.Err())
		case <-ticker.C:
		}
	}
}

func assertSynthesisTerminalReadback(
	t *testing.T,
	pool *pgxpool.Pool,
	persistence *intelligence.SynthesisPersistence,
	aggregate *intelligence.SynthesisAggregate,
	actor string,
	policyVersion string,
	cadence intelligence.SynthesisCadence,
) {
	t.Helper()
	ctx := context.Background()

	readback, err := persistence.ReadAggregate(ctx, aggregate.OutputID)
	if err != nil {
		t.Fatalf("read back %s aggregate through production reader: %v", cadence, err)
	}
	if readback.OutputID != aggregate.OutputID || readback.RunID != aggregate.RunID || readback.LogicalKey != aggregate.LogicalKey {
		t.Fatalf("%s readback identity mismatch: returned=%+v readback=%+v", cadence, aggregate, readback)
	}
	if readback.Principal != actor || readback.Cadence != cadence {
		t.Fatalf("%s readback policy identity mismatch: principal=%q cadence=%q", cadence, readback.Principal, readback.Cadence)
	}
	if readback.InsightCount != aggregate.InsightCount || readback.CitationCount != aggregate.CitationCount {
		t.Fatalf("%s readback count mismatch: returned=%d/%d readback=%d/%d", cadence,
			aggregate.InsightCount, aggregate.CitationCount,
			readback.InsightCount, readback.CitationCount)
	}
	if !reflect.DeepEqual(readback.IncludedClasses, []string{"canonical-graph"}) || len(readback.OmittedClasses) != 0 {
		t.Fatalf("%s readback source policy mismatch: included=%v omitted=%v", cadence, readback.IncludedClasses, readback.OmittedClasses)
	}

	var storedActor, storedPolicy, storedCadence string
	var runAttemptCount int
	var leaseHolder *string
	var leaseExpires *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT principal, policy_version, cadence, attempt_count,
		       lease_holder, lease_expires_at
		FROM synthesis_runs WHERE id = $1
	`, aggregate.RunID).Scan(
		&storedActor,
		&storedPolicy,
		&storedCadence,
		&runAttemptCount,
		&leaseHolder,
		&leaseExpires,
	); err != nil {
		t.Fatalf("read %s persisted run policy: %v", cadence, err)
	}
	if storedActor != actor || storedPolicy != policyVersion || storedCadence != string(cadence) {
		t.Fatalf("%s persisted run ignored loaded policy: actor=%q policy=%q cadence=%q", cadence, storedActor, storedPolicy, storedCadence)
	}
	if runAttemptCount != 1 {
		t.Fatalf("%s run attempt_count=%d, want one real scheduled attempt", cadence, runAttemptCount)
	}
	if leaseHolder != nil || leaseExpires != nil {
		t.Fatalf("%s terminal run retained lease: holder=%v expires=%v", cadence, leaseHolder, leaseExpires)
	}

	var triggerKind, attemptState, attemptOutputID string
	var finishedAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT trigger_kind, state, output_id, finished_at
		FROM synthesis_run_attempts
		WHERE run_id = $1 AND attempt_no = 1
	`, aggregate.RunID).Scan(&triggerKind, &attemptState, &attemptOutputID, &finishedAt); err != nil {
		t.Fatalf("read %s scheduled attempt: %v", cadence, err)
	}
	if triggerKind != string(intelligence.TriggerScheduled) {
		t.Fatalf("%s trigger_kind=%q, want %q", cadence, triggerKind, intelligence.TriggerScheduled)
	}
	if attemptOutputID != aggregate.OutputID || finishedAt.IsZero() {
		t.Fatalf("%s terminal attempt missing verified output/time: output=%q finished=%s", cadence, attemptOutputID, finishedAt)
	}

	var startedCount, terminalCount int
	var terminalType string
	var eventInsightCount, eventCitationCount int
	if err := pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE event_type = 'attempt_started'),
			COUNT(*) FILTER (WHERE event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed', 'recovered'
			)),
			COALESCE(MAX(event_type) FILTER (WHERE event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
				'retryable_failure', 'failed', 'readback_failed', 'recovered'
			)), ''),
			COALESCE(MAX(insight_count) FILTER (WHERE event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'recovered'
			)), -1),
			COALESCE(MAX(citation_count) FILTER (WHERE event_type IN (
				'idempotent', 'persisted', 'quiet', 'partial', 'recovered'
			)), -1)
		FROM synthesis_run_events
		WHERE run_id = $1 AND attempt_no = 1
	`, aggregate.RunID).Scan(
		&startedCount,
		&terminalCount,
		&terminalType,
		&eventInsightCount,
		&eventCitationCount,
	); err != nil {
		t.Fatalf("read %s immutable event chain: %v", cadence, err)
	}
	if startedCount != 1 || terminalCount != 1 {
		t.Fatalf("%s event chain has started=%d terminal=%d, want exactly one each", cadence, startedCount, terminalCount)
	}
	if terminalType != string(aggregate.Outcome) || attemptState != terminalType {
		t.Fatalf("%s terminal truth diverged: aggregate=%q attempt=%q event=%q", cadence, aggregate.Outcome, attemptState, terminalType)
	}
	if eventInsightCount != readback.InsightCount || eventCitationCount != readback.CitationCount {
		t.Fatalf("%s terminal event count mismatch: event=%d/%d readback=%d/%d", cadence,
			eventInsightCount, eventCitationCount,
			readback.InsightCount, readback.CitationCount)
	}
}

// TestSynthesisRunPolicyReachesCoordinatorAndBothCadences proves the Scope 7
// runtime-config contract against the disposable PostgreSQL stack. Dynamic
// actor and policy identities are loaded through config.Load, mapped by the
// same production functions cmd/core calls, and must survive both cadence paths
// into read-back-verified outputs and immutable terminal events.
func TestSynthesisRunPolicyReachesCoordinatorAndBothCadences(t *testing.T) {
	requireDisposableStack(t)
	cfg := loadDistinctSynthesisRuntimeConfig(t)

	maxAttempts, err := cfg.Synthesis.MaxAttempts()
	if err != nil {
		t.Fatalf("derive configured synthesis attempt budget: %v", err)
	}
	runPolicy := intelligence.SynthesisRunPolicyFromConfig(cfg.Synthesis)
	retryPolicy := intelligence.SynthesisRetryPolicyFromConfig(cfg.Synthesis, maxAttempts)

	if runPolicy.Actor != cfg.Synthesis.ActorUserID || runPolicy.PolicyVersion != cfg.Synthesis.PolicyVersion || runPolicy.Retention != cfg.Synthesis.Retention {
		t.Fatalf("production run-policy mapping lost loaded values: config=%+v policy=%+v", cfg.Synthesis, runPolicy)
	}
	if !reflect.DeepEqual(runPolicy.RequiredSourceClasses, cfg.Synthesis.RequiredSourceClasses) || !reflect.DeepEqual(runPolicy.OptionalSourceClasses, cfg.Synthesis.OptionalSourceClasses) {
		t.Fatalf("production source-policy mapping lost loaded classes: config=%+v policy=%+v", cfg.Synthesis, runPolicy)
	}
	if retryPolicy.MaxAttempts != cfg.Synthesis.RetryBudget+1 ||
		retryPolicy.InitialDelay != cfg.Synthesis.RetryBackoff ||
		retryPolicy.MaxDelay != cfg.Synthesis.RetryMaxBackoff ||
		retryPolicy.LeaseTTL != cfg.Synthesis.LeaseTTL {
		t.Fatalf("production retry-policy mapping lost loaded values: config=%+v policy=%+v", cfg.Synthesis, retryPolicy)
	}

	pool := synthesisTestPool(t)
	defer pool.Close()
	defer resetSynthesisTables(t, pool)
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct real synthesis persistence: %v", err)
	}
	holder := testID(t) + "-coordinator-holder"
	coordinator, err := intelligence.NewSynthesisCoordinator(persistence, retryPolicy, holder)
	if err != nil {
		t.Fatalf("construct real synthesis coordinator from loaded retry policy: %v", err)
	}
	producer, err := intelligence.NewSynthesisProducer(
		intelligence.NewEngine(pool, nil),
		persistence,
		runPolicy,
	)
	if err != nil {
		t.Fatalf("construct real synthesis producer from loaded run policy: %v", err)
	}
	producer.WithCoordinator(coordinator)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	runAt := time.Now().UTC().Truncate(time.Second)

	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire PostgreSQL connection for lease observation: %v", err)
	}
	lockTx, err := lockConn.Begin(ctx)
	if err != nil {
		lockConn.Release()
		t.Fatalf("begin PostgreSQL lease-observation lock: %v", err)
	}
	lockReleased := false
	t.Cleanup(func() {
		if lockReleased {
			return
		}
		_ = lockTx.Rollback(context.Background())
		lockConn.Release()
	})
	if _, err := lockTx.Exec(ctx, `LOCK TABLE edges IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock synthesis cluster boundary: %v", err)
	}

	dailyResult := make(chan synthesisRuntimeResult, 1)
	go func() {
		aggregate, runErr := producer.RunAndPersist(
			ctx,
			intelligence.CadenceDaily,
			intelligence.TriggerScheduled,
			runAt,
		)
		dailyResult <- synthesisRuntimeResult{aggregate: aggregate, err: runErr}
	}()

	lease := observeRunningSynthesisLease(
		t,
		pool,
		cfg.Synthesis.ActorUserID,
		cfg.Synthesis.PolicyVersion,
		intelligence.CadenceDaily,
	)
	if lease.holder != holder || lease.attemptCount != 1 {
		t.Fatalf("running lease ignored configured coordinator: holder=%q attempts=%d", lease.holder, lease.attemptCount)
	}
	if got := lease.leaseExpires.Sub(lease.updatedAt); got != cfg.Synthesis.LeaseTTL {
		t.Fatalf("running lease duration=%s, want loaded %s", got, cfg.Synthesis.LeaseTTL)
	}

	if err := lockTx.Rollback(ctx); err != nil {
		t.Fatalf("release synthesis cluster boundary: %v", err)
	}
	lockConn.Release()
	lockReleased = true

	var daily *intelligence.SynthesisAggregate
	select {
	case result := <-dailyResult:
		if result.err != nil {
			t.Fatalf("execute configured daily scheduled synthesis: %v", result.err)
		}
		daily = result.aggregate
	case <-ctx.Done():
		t.Fatalf("daily scheduled synthesis did not finish after releasing PostgreSQL boundary: %v", ctx.Err())
	}
	assertSynthesisTerminalReadback(t, pool, persistence, daily,
		cfg.Synthesis.ActorUserID, cfg.Synthesis.PolicyVersion, intelligence.CadenceDaily)

	agedCreatedAt := runAt.Add(-cfg.Synthesis.Retention - time.Second)
	tag, err := pool.Exec(ctx,
		`UPDATE synthesis_runs SET created_at = $1 WHERE id = $2`,
		agedCreatedAt,
		daily.RunID,
	)
	if err != nil {
		t.Fatalf("age real daily run for retention boundary: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("age real daily run affected %d rows, want 1", tag.RowsAffected())
	}

	weekly, err := producer.RunAndPersist(
		ctx,
		intelligence.CadenceWeekly,
		intelligence.TriggerScheduled,
		runAt,
	)
	if err != nil {
		t.Fatalf("execute configured weekly scheduled synthesis: %v", err)
	}
	assertSynthesisTerminalReadback(t, pool, persistence, weekly,
		cfg.Synthesis.ActorUserID, cfg.Synthesis.PolicyVersion, intelligence.CadenceWeekly)

	if daily.RunID == weekly.RunID || daily.OutputID == weekly.OutputID || daily.LogicalKey == weekly.LogicalKey {
		t.Fatalf("daily and weekly cadence identities collapsed: daily=%+v weekly=%+v", daily, weekly)
	}
	if daily.WindowEnd.Sub(daily.WindowStart) != 24*time.Hour || weekly.WindowEnd.Sub(weekly.WindowStart) != 7*24*time.Hour {
		t.Fatalf("cadence windows are not distinct: daily=%s weekly=%s",
			daily.WindowEnd.Sub(daily.WindowStart), weekly.WindowEnd.Sub(weekly.WindowStart))
	}

	var cadenceCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT cadence)
		FROM synthesis_runs
		WHERE principal = $1 AND policy_version = $2 AND id = ANY($3)
	`, cfg.Synthesis.ActorUserID, cfg.Synthesis.PolicyVersion, []string{daily.RunID, weekly.RunID}).Scan(&cadenceCount); err != nil {
		t.Fatalf("read persisted cadence count: %v", err)
	}
	if cadenceCount != 2 {
		t.Fatalf("persisted cadence count=%d, want daily and weekly", cadenceCount)
	}

	var dailyLifecycle string
	if err := pool.QueryRow(ctx,
		`SELECT lifecycle_state FROM synthesis_runs WHERE id = $1`,
		daily.RunID,
	).Scan(&dailyLifecycle); err != nil {
		t.Fatalf("read retained daily lifecycle: %v", err)
	}
	if dailyLifecycle != "archived" {
		t.Fatalf("configured retention did not execute through weekly cadence: daily lifecycle=%q, want archived", dailyLifecycle)
	}

	t.Logf("loaded policy reached runtime: actor=%s policy=%s attempts=%d backoff=%s..%s lease=%s retention=%s",
		cfg.Synthesis.ActorUserID,
		cfg.Synthesis.PolicyVersion,
		retryPolicy.MaxAttempts,
		retryPolicy.InitialDelay,
		retryPolicy.MaxDelay,
		retryPolicy.LeaseTTL,
		runPolicy.Retention,
	)
	t.Logf("read-back verified terminal events: daily=%s/%s weekly=%s/%s",
		daily.RunID, daily.Outcome, weekly.RunID, weekly.Outcome)
}
