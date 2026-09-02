//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// BUG-004-004 T004-01-MIGRATE / T004-01-ROLLBACK-COMPAT.
//
// Two properties that a passing feature test would not catch:
//
//  1. BOOTSTRAP CANARY. A brand-new database must end up with the full durable
//     shape after migration. The feature tests all run against a database that
//     already migrated, so they would keep passing if a migration silently
//     stopped shipping a table an operator's fresh install needs.
//
//  2. NON-DESTRUCTIVE. The synthesis migrations must only ADD. If one dropped or
//     rewrote a pre-existing table, rolling back would take an operator's data
//     with it, and no amount of green feature tests would reveal that.

func TestSynthesisMigration_BootstrapCanaryCreatesFullShape(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	// Every table the durable contract depends on. Naming them explicitly is the
	// point: a migration that quietly stopped creating one would fail here even
	// though the feature tests, running on an already-migrated database, would
	// not notice.
	for _, table := range []string{
		"synthesis_runs",
		"synthesis_run_attempts",
		"synthesis_outputs",
		"synthesis_output_insights",
		"synthesis_citations",
		"synthesis_output_source_classes",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists); err != nil {
			t.Fatalf("probe %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %s is absent after bootstrap; a fresh install would not work", table)
		}
	}

	// The columns that carry the meaning, not merely the table names. An output
	// without output_kind cannot distinguish quiet from full, which is the whole
	// point of SCN-07 and SCN-08.
	for _, col := range []string{"output_kind", "evaluated_artifact_count"} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'synthesis_outputs' AND column_name = $1
			)`, col).Scan(&exists); err != nil {
			t.Fatalf("probe column %s: %v", col, err)
		}
		if !exists {
			t.Fatalf("synthesis_outputs.%s is absent; quiet and full outputs would be indistinguishable", col)
		}
	}

	// Idempotence is a database constraint, not application logic. If this index
	// vanished, the duplicate test would still pass whenever timing happened to
	// serialise the two runs -- and fail in production under real concurrency.
	var uniqueOnLogicalKey bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'synthesis_runs' AND indexdef ILIKE '%UNIQUE%logical_key%'
		)`).Scan(&uniqueOnLogicalKey); err != nil {
		t.Fatalf("probe unique index: %v", err)
	}
	if !uniqueOnLogicalKey {
		t.Fatal("synthesis_runs has no UNIQUE index on logical_key; idempotence would rest on a race, not a constraint")
	}
}

// SCN-004-004-C11 / T004-C11-MIGRATE. The corrective migration must work as
// the next additive step after 066 and as part of a fresh bootstrap. The test
// database is freshly provisioned by the integration runner, while the
// schema_migrations assertion proves 067 followed the already-applied 066
// contract rather than rewriting it.
func TestSynthesisMigration_AddsImmutableCausalEventLedgerFrom066AndFreshBootstrap(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()

	t.Run("upgrade_from_066_preserves_and_truthfully_backfills_rows", func(t *testing.T) {
		isolatedPool := newIsolatedSynthesisMigrationDatabase(t, pool)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		applySynthesisMigrationFiles(t, ctx, isolatedPool,
			"064_synthesis_durable_persistence.sql",
			"065_synthesis_output_kind.sql",
			"066_synthesis_run_lifecycle.sql",
		)
		seedRepresentativeSynthesis066Rows(t, ctx, isolatedPool)

		if err := validateSynthesis067Shape(ctx, isolatedPool); err == nil {
			t.Fatal("pre-067 negative control unexpectedly satisfied the causal event contract")
		} else {
			t.Logf("pre-067 negative control rejected the proxy schema: %v", err)
		}

		applySynthesisMigrationFiles(t, ctx, isolatedPool,
			"067_synthesis_causal_event_truth.sql",
		)
		if err := validateSynthesis067Shape(ctx, isolatedPool); err != nil {
			t.Fatalf("067 did not install the causal event contract: %v", err)
		}
		assertSynthesis066RowsAfter067(t, ctx, isolatedPool)
		exerciseSynthesis067Constraints(t, ctx, isolatedPool)
	})

	t.Run("fresh_bootstrap_builds_the_same_causal_shape", func(t *testing.T) {
		isolatedPool := newIsolatedSynthesisMigrationDatabase(t, pool)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		applySynthesisMigrationFiles(t, ctx, isolatedPool,
			"064_synthesis_durable_persistence.sql",
			"065_synthesis_output_kind.sql",
			"066_synthesis_run_lifecycle.sql",
			"067_synthesis_causal_event_truth.sql",
		)
		if err := validateSynthesis067Shape(ctx, isolatedPool); err != nil {
			t.Fatalf("fresh bootstrap lacks the causal event contract: %v", err)
		}
	})
}

// SCN-004-004-C11 / T004-C11-MIGRATE. Commit
// 7c3838e3b2de9ecba2e6a7764493a0412c4ed268 is the certified prior source.
// These run, output, and attempt column lists are copied from that commit's
// internal/intelligence/synthesis_persistence.go. Keeping the historical
// shapes literal makes this test fail if migration 067 requires a prior writer
// to invent any causal or output-projection field it did not know about.
func TestSynthesisMigration_PreservesLegacy064Through066InsertShapesAndEnforcesCausal067Writes(t *testing.T) {
	adminPool := synthesisTestPool(t)
	t.Cleanup(adminPool.Close)
	isolatedPool := newIsolatedSynthesisMigrationDatabase(t, adminPool)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	applySynthesisMigrationFiles(t, ctx, isolatedPool,
		"064_synthesis_durable_persistence.sql",
		"065_synthesis_output_kind.sql",
		"066_synthesis_run_lifecycle.sql",
		"067_synthesis_causal_event_truth.sql",
	)

	const (
		legacyRunID           = "test-scope03a-legacy-run"
		legacyOutputID        = "test-scope03a-legacy-output"
		legacyLogicalIdentity = "test-scope03a-legacy-key"
		legacyPrincipal       = "test-scope03a-legacy-principal"
	)
	windowStart := time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	recordedAt := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)

	var insertedRunID string
	if err := isolatedPool.QueryRow(ctx, `
		INSERT INTO synthesis_runs
			(id, logical_key, cadence, principal, window_start, window_end,
			 policy_version, source_set_digest, state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'succeeded', $9, $9)
		ON CONFLICT (logical_key) DO UPDATE
		SET state = 'succeeded', updated_at = $9
		WHERE synthesis_runs.state <> 'succeeded'
		RETURNING id
	`, legacyRunID, legacyLogicalIdentity, "daily", legacyPrincipal,
		windowStart, windowEnd, "test-scope03a-policy", "test-scope03a-source-set",
		recordedAt).Scan(&insertedRunID); err != nil {
		t.Fatalf("execute certified prior-source synthesis_runs INSERT shape: %v", err)
	}
	if insertedRunID != legacyRunID {
		t.Fatalf("certified prior-source run returned id %q, want %q", insertedRunID, legacyRunID)
	}

	var legacyAttemptID int64
	legacyAttemptErr := isolatedPool.QueryRow(ctx, `
		INSERT INTO synthesis_run_attempts
			(logical_key, outcome, failure_class, failure_kind, failure_message)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, legacyLogicalIdentity, "failed", "database_unavailable", "transient",
		"test-scope03a-safe-legacy-detail").Scan(&legacyAttemptID)
	_, legacyOutputErr := isolatedPool.Exec(ctx, `
		INSERT INTO synthesis_outputs
			(id, run_id, output_kind, insight_count, citation_count,
			 evaluated_artifact_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, legacyOutputID, legacyRunID, "full", 0, 0, 1, recordedAt)
	if legacyAttemptErr != nil || legacyOutputErr != nil {
		t.Fatalf("migration 067 rejected certified prior-source INSERT shapes: attempt=%v; output=%v",
			legacyAttemptErr, legacyOutputErr)
	}

	var (
		outputPrincipal, outputCadence, outputLifecycle string
		outputWindowStart, outputWindowEnd              time.Time
		projectionMatchesRun                            bool
	)
	if err := isolatedPool.QueryRow(ctx, `
		SELECT o.principal, o.cadence, o.window_start, o.window_end,
		       o.lifecycle_state,
		       o.principal = r.principal
		       AND o.cadence = r.cadence
		       AND o.window_start = r.window_start
		       AND o.window_end = r.window_end
		       AND o.lifecycle_state = r.lifecycle_state
		FROM synthesis_outputs o
		JOIN synthesis_runs r ON r.id = o.run_id
		WHERE o.id = $1
	`, legacyOutputID).Scan(
		&outputPrincipal, &outputCadence, &outputWindowStart, &outputWindowEnd,
		&outputLifecycle, &projectionMatchesRun,
	); err != nil {
		t.Fatalf("read certified prior-source output projection: %v", err)
	}
	if !projectionMatchesRun || outputPrincipal != legacyPrincipal || outputCadence != "daily" ||
		!outputWindowStart.Equal(windowStart) || !outputWindowEnd.Equal(windowEnd) || outputLifecycle != "current" {
		t.Fatalf("legacy output projection=(%q,%q,%s,%s,%q,runMatch=%t), want referenced run (%q,daily,%s,%s,current,true)",
			outputPrincipal, outputCadence, outputWindowStart, outputWindowEnd,
			outputLifecycle, projectionMatchesRun, legacyPrincipal, windowStart, windowEnd)
	}

	assertLegacySynthesisAttemptHasNoCausalProjection(t, ctx, isolatedPool, legacyAttemptID)
	assertSynthesisMigrationRowCount(t, ctx, isolatedPool, "synthesis_run_events", 0)

	projectionConflicts := []struct {
		name string
		sql  string
	}{
		{name: "principal", sql: `
			INSERT INTO synthesis_outputs
				(id, run_id, output_kind, insight_count, citation_count,
				 evaluated_artifact_count, created_at, principal)
			VALUES ($1, $2, 'full', 0, 0, 1, $3, 'test-scope03a-conflicting-principal')`},
		{name: "cadence", sql: `
			INSERT INTO synthesis_outputs
				(id, run_id, output_kind, insight_count, citation_count,
				 evaluated_artifact_count, created_at, cadence)
			VALUES ($1, $2, 'full', 0, 0, 1, $3, 'weekly')`},
		{name: "window_start", sql: `
			INSERT INTO synthesis_outputs
				(id, run_id, output_kind, insight_count, citation_count,
				 evaluated_artifact_count, created_at, window_start)
			VALUES ($1, $2, 'full', 0, 0, 1, $3, '2026-08-27T00:00:00Z')`},
		{name: "window_end", sql: `
			INSERT INTO synthesis_outputs
				(id, run_id, output_kind, insight_count, citation_count,
				 evaluated_artifact_count, created_at, window_end)
			VALUES ($1, $2, 'full', 0, 0, 1, $3, '2026-08-30T00:00:00Z')`},
		{name: "lifecycle_state", sql: `
			INSERT INTO synthesis_outputs
				(id, run_id, output_kind, insight_count, citation_count,
				 evaluated_artifact_count, created_at, lifecycle_state)
			VALUES ($1, $2, 'full', 0, 0, 1, $3, 'stale')`},
	}
	for _, conflict := range projectionConflicts {
		t.Run("reject_conflicting_"+conflict.name+"_projection", func(t *testing.T) {
			_, err := isolatedPool.Exec(ctx, conflict.sql,
				"test-scope03a-conflicting-projection-"+conflict.name, legacyRunID, recordedAt)
			requireSynthesisMigrationSQLState(t, err, "23514", "insert conflicting "+conflict.name+" projection")
		})
	}

	const conflictingRunID = "test-scope03a-one-current-run"
	if _, err := isolatedPool.Exec(ctx, `
		INSERT INTO synthesis_runs
			(id, logical_key, cadence, principal, window_start, window_end,
			 policy_version, source_set_digest, state, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'succeeded', $9, $9)
	`, conflictingRunID, "test-scope03a-one-current-key", "daily", legacyPrincipal,
		windowStart, windowEnd, "test-scope03a-policy-v2", "test-scope03a-source-set-v2",
		recordedAt.Add(time.Hour)); err != nil {
		t.Fatalf("insert same-window run for one-current constraint: %v", err)
	}
	_, oneCurrentErr := isolatedPool.Exec(ctx, `
		INSERT INTO synthesis_outputs
			(id, run_id, output_kind, insight_count, citation_count,
			 evaluated_artifact_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, "test-scope03a-one-current-output", conflictingRunID, "quiet", 0, 0, 1,
		recordedAt.Add(time.Hour))
	requireSynthesisMigrationSQLState(t, oneCurrentErr, "23505", "insert second current output for one actor/cadence/window")

	_, incompleteAttemptErr := isolatedPool.Exec(ctx, `
		INSERT INTO synthesis_run_attempts (
			logical_key, outcome, failure_class, failure_message, recorded_at,
			failure_kind, run_id, attempt_no, trigger_kind, state, started_at
		) VALUES (
			$1, 'running', NULL, NULL, $2, NULL, $3, 1, 'scheduled', 'running', $2
		)
	`, legacyLogicalIdentity, recordedAt.Add(2*time.Hour), legacyRunID)
	requireSynthesisMigrationSQLState(t, incompleteAttemptErr, "23514", "insert incomplete linked causal attempt")

	if _, err := isolatedPool.Exec(ctx, `
		INSERT INTO synthesis_run_attempts (
			logical_key, outcome, failure_class, failure_message, recorded_at,
			failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
			started_at, finished_at, failure_code, included_source_classes,
			omitted_source_classes, insight_count, citation_count
		) VALUES (
			$1, 'succeeded', NULL, NULL, $3, NULL, $2, 1, 'scheduled',
			'persisted', $4, $3, $5, NULL, ARRAY['capture'], ARRAY[]::TEXT[], 0, 0
		)
	`, legacyLogicalIdentity, legacyRunID, recordedAt.Add(2*time.Hour), legacyOutputID,
		recordedAt.Add(2*time.Hour+time.Minute)); err != nil {
		t.Fatalf("insert complete linked causal attempt: %v", err)
	}
	if _, err := isolatedPool.Exec(ctx, `
		INSERT INTO synthesis_run_events (
			id, run_id, attempt_no, event_type, output_id, related_output_id,
			failure_code, insight_count, citation_count, created_at
		) VALUES
			('test-scope03a-legacy-shape-started', $1, 1, 'attempt_started',
			 NULL, NULL, NULL, NULL, NULL, $2),
			('test-scope03a-legacy-shape-persisted', $1, 1, 'persisted',
			 $3, NULL, NULL, 0, 0, $4)
	`, legacyRunID, recordedAt.Add(2*time.Hour), legacyOutputID,
		recordedAt.Add(2*time.Hour+time.Minute)); err != nil {
		t.Fatalf("insert complete linked causal event chain: %v", err)
	}
	assertSynthesisMigrationRowCount(t, ctx, isolatedPool, "synthesis_run_events", 2)

	// Migration files in this repository are deliberately rerunnable. Reapply
	// 067 after both legacy and causal rows exist and prove it neither invents
	// legacy causal history nor damages the strict linked chain.
	applySynthesisMigrationFiles(t, ctx, isolatedPool, "067_synthesis_causal_event_truth.sql")
	assertLegacySynthesisAttemptHasNoCausalProjection(t, ctx, isolatedPool, legacyAttemptID)
	assertSynthesisMigrationRowCount(t, ctx, isolatedPool, "synthesis_run_events", 2)
}

func assertLegacySynthesisAttemptHasNoCausalProjection(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	attemptID int64,
) {
	t.Helper()
	var allCausalFieldsNull bool
	if err := pool.QueryRow(ctx, `
		SELECT run_id IS NULL
		   AND attempt_no IS NULL
		   AND trigger_kind IS NULL
		   AND state IS NULL
		   AND output_id IS NULL
		   AND started_at IS NULL
		   AND finished_at IS NULL
		   AND failure_code IS NULL
		   AND included_source_classes IS NULL
		   AND omitted_source_classes IS NULL
		   AND insight_count IS NULL
		   AND citation_count IS NULL
		FROM synthesis_run_attempts
		WHERE id = $1
	`, attemptID).Scan(&allCausalFieldsNull); err != nil {
		t.Fatalf("read legacy attempt causal projection: %v", err)
	}
	if !allCausalFieldsNull {
		t.Fatal("migration 067 fabricated causal identity, state, time, source-class, or count fields for a legacy attempt")
	}
}

func assertSynthesisMigrationRowCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
	want int,
) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&got); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s row count=%d, want %d", table, got, want)
	}
}

func newIsolatedSynthesisMigrationDatabase(t *testing.T, adminPool *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	databaseName := "test_synthesis_migration_" + strings.ToLower(ulid.Make().String())
	quotedDatabase := pgx.Identifier{databaseName}.Sanitize()

	createCtx, createCancel := context.WithTimeout(context.Background(), 15*time.Second)
	if _, err := adminPool.Exec(createCtx, "CREATE DATABASE "+quotedDatabase+" TEMPLATE template0"); err != nil {
		createCancel()
		t.Fatalf("create isolated synthesis migration database: %v", err)
	}
	createCancel()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		t.Fatal("isolated synthesis migration database requires DATABASE_URL from the integration runner")
	}
	isolateConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal("parse integration DATABASE_URL for isolated synthesis migration database")
	}
	isolateConfig.ConnConfig.Database = databaseName
	isolateConfig.MaxConns = 4
	isolateConfig.MinConns = 0

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	isolatedPool, err := pgxpool.NewWithConfig(connectCtx, isolateConfig)
	if err == nil {
		err = isolatedPool.Ping(connectCtx)
	}
	connectCancel()
	if err != nil {
		if isolatedPool != nil {
			isolatedPool.Close()
		}
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, _ = adminPool.Exec(dropCtx, "DROP DATABASE IF EXISTS "+quotedDatabase+" WITH (FORCE)")
		dropCancel()
		t.Fatalf("connect isolated synthesis migration database: %v", err)
	}

	t.Cleanup(func() {
		isolatedPool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if _, err := adminPool.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quotedDatabase+" WITH (FORCE)"); err != nil {
			t.Errorf("drop isolated synthesis migration database: %v", err)
		}
	})
	return isolatedPool
}

func applySynthesisMigrationFiles(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	names ...string,
) {
	t.Helper()
	dir := filepath.Join("..", "..", "internal", "db", "migrations")
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read synthesis migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply synthesis migration %s: %v", name, err)
		}
		t.Logf("applied %s to isolated PostgreSQL database", name)
	}
}

func seedRepresentativeSynthesis066Rows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	const seedSQL = `
		INSERT INTO synthesis_runs (
			id, logical_key, cadence, principal, window_start, window_end,
			policy_version, source_set_digest, state, created_at, updated_at
		) VALUES
			('test-scope03a-upgrade-run-old', 'test-scope03a-upgrade-key-old',
			 'daily', 'test-scope03a-upgrade-actor',
			 '2026-08-29T00:00:00Z', '2026-08-30T00:00:00Z',
			 'policy-v1', 'source-set-old', 'succeeded',
			 '2026-08-30T10:00:00Z', '2026-08-30T10:00:00Z'),
			('test-scope03a-upgrade-run-new', 'test-scope03a-upgrade-key-new',
			 'daily', 'test-scope03a-upgrade-actor',
			 '2026-08-29T00:00:00Z', '2026-08-30T00:00:00Z',
			 'policy-v2', 'source-set-new', 'succeeded',
			 '2026-08-30T11:00:00Z', '2026-08-30T11:00:00Z');

		INSERT INTO synthesis_run_attempts (
			logical_key, outcome, failure_class, failure_message, recorded_at, failure_kind
		) VALUES
			('test-scope03a-upgrade-key-old', 'succeeded', NULL, NULL,
			 '2026-08-30T10:00:00Z', NULL),
			('test-scope03a-upgrade-key-failed', 'failed', 'database_unavailable',
			 'safe pre-067 failure', '2026-08-30T10:30:00Z', 'transient');

		INSERT INTO synthesis_outputs (
			id, run_id, insight_count, citation_count, created_at,
			output_kind, evaluated_artifact_count
		) VALUES
			('test-scope03a-upgrade-output-old', 'test-scope03a-upgrade-run-old',
			 1, 1, '2026-08-30T10:00:00Z', 'full', 1),
			('test-scope03a-upgrade-output-new', 'test-scope03a-upgrade-run-new',
			 1, 1, '2026-08-30T11:00:00Z', 'full', 1);

		INSERT INTO synthesis_output_insights (
			id, output_id, ordinal, insight_type, through_line, confidence, created_at
		) VALUES (
			'test-scope03a-upgrade-insight', 'test-scope03a-upgrade-output-new',
			1, 'through_line', 'test-scope03a-upgrade-through-line', 0.75,
			'2026-08-30T11:00:00Z'
		);

		INSERT INTO synthesis_citations (insight_id, artifact_id, ordinal)
		VALUES ('test-scope03a-upgrade-insight', 'test-scope03a-upgrade-artifact', 1);

		INSERT INTO synthesis_output_source_classes (output_id, source_class, disposition)
		VALUES ('test-scope03a-upgrade-output-new', 'capture', 'included');
	`
	if _, err := pool.Exec(ctx, seedSQL); err != nil {
		t.Fatalf("seed representative 066 synthesis rows: %v", err)
	}
}

func validateSynthesis067Shape(ctx context.Context, pool *pgxpool.Pool) error {
	var eventsExists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = 'synthesis_run_events'
		)`).Scan(&eventsExists); err != nil {
		return fmt.Errorf("probe synthesis_run_events: %w", err)
	}
	if !eventsExists {
		return errors.New("synthesis_run_events is absent")
	}

	requiredColumns := map[string][]string{
		"synthesis_run_attempts": {
			"run_id", "attempt_no", "trigger_kind", "state", "output_id",
			"started_at", "finished_at", "failure_code",
		},
		"synthesis_outputs": {
			"principal", "cadence", "window_start", "window_end",
			"lifecycle_state", "superseded_at",
		},
		"synthesis_run_events": {
			"id", "run_id", "attempt_no", "event_type", "output_id",
			"related_output_id", "failure_code", "insight_count",
			"citation_count", "created_at",
		},
	}
	for table, columns := range requiredColumns {
		for _, column := range columns {
			var exists bool
			if err := pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
					  AND table_name = $1 AND column_name = $2
				)`, table, column).Scan(&exists); err != nil {
				return fmt.Errorf("probe %s.%s: %w", table, column, err)
			}
			if !exists {
				return fmt.Errorf("required causal column %s.%s is absent", table, column)
			}
		}
	}

	for _, column := range []string{
		"synthesis_text", "through_line", "source_id", "source_ref",
		"artifact_id", "source_set_digest", "failure_message",
	} {
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema()
				  AND table_name = 'synthesis_run_events' AND column_name = $1
			)`, column).Scan(&exists); err != nil {
			return fmt.Errorf("probe forbidden event content column %s: %w", column, err)
		}
		if exists {
			return fmt.Errorf("synthesis_run_events exposes forbidden content column %s", column)
		}
	}

	var immutableTrigger, oneCurrentIndex bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_trigger t
			JOIN pg_class c ON c.oid = t.tgrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema()
			  AND c.relname = 'synthesis_run_events'
			  AND t.tgname = 'synthesis_run_events_reject_mutation'
			  AND NOT t.tgisinternal
		)`).Scan(&immutableTrigger); err != nil {
		return fmt.Errorf("probe immutable trigger: %w", err)
	}
	if !immutableTrigger {
		return errors.New("synthesis_run_events has no update/delete rejection trigger")
	}
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = current_schema()
			  AND tablename = 'synthesis_outputs'
			  AND indexname = 'synthesis_outputs_one_current_per_window_idx'
			  AND indexdef ILIKE '%UNIQUE%'
		)`).Scan(&oneCurrentIndex); err != nil {
		return fmt.Errorf("probe one-current index: %w", err)
	}
	if !oneCurrentIndex {
		return errors.New("synthesis_outputs lacks the one-current-per-actor/cadence/window unique index")
	}
	return nil
}

func assertSynthesis066RowsAfter067(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	for _, expectation := range []struct {
		table string
		want  int
	}{
		{table: "synthesis_runs", want: 2},
		{table: "synthesis_run_attempts", want: 2},
		{table: "synthesis_outputs", want: 2},
		{table: "synthesis_output_insights", want: 1},
		{table: "synthesis_citations", want: 1},
		{table: "synthesis_output_source_classes", want: 1},
	} {
		var got int
		if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM "+pgx.Identifier{expectation.table}.Sanitize()).Scan(&got); err != nil {
			t.Fatalf("count preserved %s rows: %v", expectation.table, err)
		}
		if got != expectation.want {
			t.Fatalf("preserved %s row count=%d, want %d", expectation.table, got, expectation.want)
		}
	}

	for _, expectation := range []struct {
		outputID         string
		wantLifecycle    string
		wantSupersededAt bool
		wantRunLifecycle string
	}{
		{
			outputID:         "test-scope03a-upgrade-output-old",
			wantLifecycle:    "superseded",
			wantSupersededAt: true,
			wantRunLifecycle: "superseded",
		},
		{
			outputID:         "test-scope03a-upgrade-output-new",
			wantLifecycle:    "current",
			wantSupersededAt: false,
			wantRunLifecycle: "current",
		},
	} {
		var lifecycle, runLifecycle string
		var supersededAtPresent bool
		if err := pool.QueryRow(ctx, `
			SELECT o.lifecycle_state, o.superseded_at IS NOT NULL, r.lifecycle_state
			FROM synthesis_outputs o
			JOIN synthesis_runs r ON r.id = o.run_id
			WHERE o.id = $1
		`, expectation.outputID).Scan(&lifecycle, &supersededAtPresent, &runLifecycle); err != nil {
			t.Fatalf("read backfilled output %s: %v", expectation.outputID, err)
		}
		if lifecycle != expectation.wantLifecycle || supersededAtPresent != expectation.wantSupersededAt || runLifecycle != expectation.wantRunLifecycle {
			t.Fatalf("output %s lifecycle=(%q,%t,%q), want (%q,%t,%q)",
				expectation.outputID, lifecycle, supersededAtPresent, runLifecycle,
				expectation.wantLifecycle, expectation.wantSupersededAt, expectation.wantRunLifecycle)
		}
	}

	var currentOutput string
	if err := pool.QueryRow(ctx, `
		SELECT id FROM synthesis_outputs
		WHERE principal = 'test-scope03a-upgrade-actor'
		  AND cadence = 'daily'
		  AND window_start = '2026-08-29T00:00:00Z'
		  AND window_end = '2026-08-30T00:00:00Z'
		  AND lifecycle_state = 'current'
	`).Scan(&currentOutput); err != nil {
		t.Fatalf("read unique current output after 067: %v", err)
	}
	if currentOutput != "test-scope03a-upgrade-output-new" {
		t.Fatalf("current output=%q, want newest preserved output", currentOutput)
	}

	for _, expectation := range []struct {
		logicalKey   string
		outcome      string
		failureClass string
		failureKind  string
	}{
		{logicalKey: "test-scope03a-upgrade-key-old", outcome: "succeeded"},
		{
			logicalKey:   "test-scope03a-upgrade-key-failed",
			outcome:      "failed",
			failureClass: "database_unavailable",
			failureKind:  "transient",
		},
	} {
		var attemptID int64
		var outcome, failureClass, failureKind string
		if err := pool.QueryRow(ctx, `
			SELECT id, outcome, COALESCE(failure_class, ''), COALESCE(failure_kind, '')
			FROM synthesis_run_attempts
			WHERE logical_key = $1
		`, expectation.logicalKey).Scan(&attemptID, &outcome, &failureClass, &failureKind); err != nil {
			t.Fatalf("read preserved legacy attempt %s: %v", expectation.logicalKey, err)
		}
		if outcome != expectation.outcome || failureClass != expectation.failureClass || failureKind != expectation.failureKind {
			t.Fatalf("legacy attempt %s summary=(%q,%q,%q), want (%q,%q,%q)",
				expectation.logicalKey, outcome, failureClass, failureKind,
				expectation.outcome, expectation.failureClass, expectation.failureKind)
		}
		assertLegacySynthesisAttemptHasNoCausalProjection(t, ctx, pool, attemptID)
	}

	var throughLine, artifactID, sourceClass string
	if err := pool.QueryRow(ctx, `
		SELECT i.through_line, c.artifact_id, sc.source_class
		FROM synthesis_output_insights i
		JOIN synthesis_citations c ON c.insight_id = i.id
		JOIN synthesis_output_source_classes sc ON sc.output_id = i.output_id
		WHERE i.id = 'test-scope03a-upgrade-insight'
	`).Scan(&throughLine, &artifactID, &sourceClass); err != nil {
		t.Fatalf("read representative child rows after 067: %v", err)
	}
	if throughLine != "test-scope03a-upgrade-through-line" ||
		artifactID != "test-scope03a-upgrade-artifact" || sourceClass != "capture" {
		t.Fatalf("representative child rows changed during 067: through_line=%q artifact=%q source_class=%q",
			throughLine, artifactID, sourceClass)
	}

	var fabricatedEvents int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM synthesis_run_events`).Scan(&fabricatedEvents); err != nil {
		t.Fatalf("count events after historical backfill: %v", err)
	}
	if fabricatedEvents != 0 {
		t.Fatalf("migration fabricated %d causal event(s) for unlinked historical attempts", fabricatedEvents)
	}
}

func exerciseSynthesis067Constraints(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_attempts (
			logical_key, outcome, failure_class, failure_message, recorded_at,
			failure_kind, run_id, attempt_no, trigger_kind, state, output_id,
			started_at, finished_at, failure_code, included_source_classes,
			omitted_source_classes, insight_count, citation_count
		) VALUES (
			'test-scope03a-upgrade-key-new', 'succeeded', NULL, NULL,
			'2026-08-30T11:01:00Z', NULL, 'test-scope03a-upgrade-run-new', 1,
			'operator_retry', 'persisted', 'test-scope03a-upgrade-output-new',
			'2026-08-30T11:00:30Z', '2026-08-30T11:01:00Z', NULL,
			ARRAY['capture'], ARRAY[]::TEXT[], 1, 1
		)
	`); err != nil {
		t.Fatalf("insert post-067 causal attempt: %v", err)
	}

	_, crossRunErr := pool.Exec(ctx, `
		INSERT INTO synthesis_run_events (
			id, run_id, attempt_no, event_type, output_id, related_output_id,
			failure_code, insight_count, citation_count, created_at
		) VALUES (
			'test-scope03a-upgrade-event-cross-run',
			'test-scope03a-upgrade-run-new', 1, 'persisted',
			'test-scope03a-upgrade-output-old', NULL, NULL, 1, 1,
			'2026-08-30T11:00:45Z'
		)
	`)
	requireSynthesisMigrationSQLState(t, crossRunErr, "23503", "cross-pair event output and run")

	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_run_events (
			id, run_id, attempt_no, event_type, output_id, related_output_id,
			failure_code, insight_count, citation_count, created_at
		) VALUES
			('test-scope03a-upgrade-event-started', 'test-scope03a-upgrade-run-new',
			 1, 'attempt_started', NULL, NULL, NULL, NULL, NULL,
			 '2026-08-30T11:00:30Z'),
			('test-scope03a-upgrade-event-persisted', 'test-scope03a-upgrade-run-new',
			 1, 'persisted', 'test-scope03a-upgrade-output-new', NULL, NULL, 1, 1,
			 '2026-08-30T11:01:00Z')
	`); err != nil {
		t.Fatalf("insert post-067 causal attempt and event chain: %v", err)
	}

	_, updateErr := pool.Exec(ctx, `
		UPDATE synthesis_run_events
		SET citation_count = citation_count + 1
		WHERE id = 'test-scope03a-upgrade-event-persisted'
	`)
	requireSynthesisMigrationSQLState(t, updateErr, "55000", "update immutable event")

	_, deleteErr := pool.Exec(ctx, `
		DELETE FROM synthesis_run_events
		WHERE id = 'test-scope03a-upgrade-event-persisted'
	`)
	requireSynthesisMigrationSQLState(t, deleteErr, "55000", "delete immutable event")

	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_runs (
			id, logical_key, cadence, principal, window_start, window_end,
			policy_version, source_set_digest, state, created_at, updated_at
		) VALUES (
			'test-scope03a-upgrade-run-conflict', 'test-scope03a-upgrade-key-conflict',
			'daily', 'test-scope03a-upgrade-actor',
			'2026-08-29T00:00:00Z', '2026-08-30T00:00:00Z',
			'policy-v3', 'source-set-conflict', 'succeeded',
			'2026-08-30T12:00:00Z', '2026-08-30T12:00:00Z'
		)
	`); err != nil {
		t.Fatalf("seed conflicting post-067 run: %v", err)
	}
	_, currentErr := pool.Exec(ctx, `
		INSERT INTO synthesis_outputs (
			id, run_id, insight_count, citation_count, created_at,
			output_kind, evaluated_artifact_count, principal, cadence,
			window_start, window_end, lifecycle_state
		) VALUES (
			'test-scope03a-upgrade-output-conflict', 'test-scope03a-upgrade-run-conflict',
			0, 0, '2026-08-30T12:00:00Z', 'quiet', 1,
			'test-scope03a-upgrade-actor', 'daily',
			'2026-08-29T00:00:00Z', '2026-08-30T00:00:00Z', 'current'
		)
	`)
	requireSynthesisMigrationSQLState(t, currentErr, "23505", "insert second current output")

	var events, currentOutputs int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM synthesis_run_events`).Scan(&events); err != nil {
		t.Fatalf("count post-067 events: %v", err)
	}
	if events != 2 {
		t.Fatalf("post-067 causal event count=%d, want 2", events)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM synthesis_outputs
		WHERE principal = 'test-scope03a-upgrade-actor'
		  AND cadence = 'daily'
		  AND window_start = '2026-08-29T00:00:00Z'
		  AND window_end = '2026-08-30T00:00:00Z'
		  AND lifecycle_state = 'current'
	`).Scan(&currentOutputs); err != nil {
		t.Fatalf("count post-067 current outputs: %v", err)
	}
	if currentOutputs != 1 {
		t.Fatalf("post-067 current output count=%d, want 1", currentOutputs)
	}
}

func requireSynthesisMigrationSQLState(t *testing.T, err error, want, operation string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s succeeded; want PostgreSQL SQLSTATE %s", operation, want)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s returned non-PostgreSQL error %T: %v", operation, err, err)
	}
	if pgErr.Code != want {
		t.Fatalf("%s SQLSTATE=%s, want %s: %v", operation, pgErr.Code, want, err)
	}
}

func TestSynthesisMigration_IsNonDestructive(t *testing.T) {
	dir := filepath.Join("..", "..", "internal", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}

	// Pre-existing tables an operator already has data in. Constraint and trigger
	// replacement in 066/067 is legitimate additive enforcement; table, row, or
	// column destruction against these data-bearing relations is not.
	preExisting := []string{
		"artifacts", "topics", "edges", "synthesis_insights",
		"synthesis_runs", "synthesis_run_attempts", "synthesis_outputs",
		"synthesis_output_insights", "synthesis_citations",
		"synthesis_output_source_classes", "synthesis_run_events",
	}
	destructiveDataChange := regexp.MustCompile(`(?i)\b(DROP\s+TABLE|TRUNCATE|DELETE\s+FROM|DROP\s+COLUMN)\b`)
	expectedMigrations := map[string]struct{}{
		"064_synthesis_durable_persistence.sql": {},
		"065_synthesis_output_kind.sql":         {},
		"066_synthesis_run_lifecycle.sql":       {},
		"067_synthesis_causal_event_truth.sql":  {},
	}

	var checked int
	for _, e := range entries {
		name := e.Name()
		if _, expected := expectedMigrations[name]; !expected {
			continue
		}
		checked++

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		// Strip comments first -- prose describing what the migration deliberately
		// does NOT do would otherwise trip the scan and teach everyone to ignore it.
		var sql strings.Builder
		for _, line := range strings.Split(string(body), "\n") {
			if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "--") {
				continue
			}
			sql.WriteString(line)
			sql.WriteString("\n")
		}

		for _, stmt := range strings.Split(sql.String(), ";") {
			if !destructiveDataChange.MatchString(stmt) {
				continue
			}
			for _, table := range preExisting {
				if regexp.MustCompile(`(?i)\b` + table + `\b`).MatchString(stmt) {
					t.Fatalf("%s contains a destructive statement against pre-existing table %q; rolling back would take operator data with it:\n%s",
						name, table, strings.TrimSpace(stmt))
				}
			}
		}
	}

	if checked != len(expectedMigrations) {
		t.Fatalf("expected to check migrations 064 through 067, checked %d of %d; a renamed migration would silently skip this guard",
			checked, len(expectedMigrations))
	}
}

// The pre-existing synthesis_insights table is LEGACY. The durable contract is
// the new tables, and a legacy row must never be mixed into a verified output --
// otherwise an operator reading "3 insights" could be seeing rows that never
// passed validation, citation checks, or read-back.
func TestSynthesisMigration_LegacyInsightsAreNotReadAsDurableOutput(t *testing.T) {
	pool := synthesisTestPool(t)
	defer pool.Close()
	resetSynthesisTables(t, pool)
	seedSynthesisCluster(t, pool)
	ctx := context.Background()

	// A legacy row of exactly the kind the old producer would have left behind,
	// had it ever written one.
	// t.Fatalf, never t.Skipf: a skip here would turn "the legacy table changed
	// shape" into a silent pass, which is the failure mode this whole packet is
	// about.
	if _, err := pool.Exec(ctx, `
		INSERT INTO synthesis_insights (id, insight_type, through_line, source_artifact_ids, confidence)
		VALUES ('legacy-insight-1', 'through_line', 'a legacy row that never passed validation', ARRAY['art-e2e-0'], 0.9)`); err != nil {
		t.Fatalf("seed legacy insight: %v", err)
	}

	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct persistence: %v", err)
	}
	producer := newSynthesisTestProducer(t, &intelligence.Engine{Pool: pool}, persistence,
		"scheduler", "migration-canary-holder")

	agg, err := producer.RunAndPersist(ctx, intelligence.CadenceDaily,
		intelligence.TriggerScheduled, time.Now().UTC())
	if err != nil {
		t.Fatalf("run and persist: %v", err)
	}

	for _, insight := range agg.Insights {
		if insight.ID == "legacy-insight-1" {
			t.Fatal("a legacy synthesis_insights row surfaced inside a verified output; legacy rows never passed validation or read-back and must stay out")
		}
		if insight.ThroughLine == "a legacy row that never passed validation" {
			t.Fatal("legacy content surfaced in a verified output")
		}
	}

	// The legacy row is still there -- classification is not deletion. An
	// operator's old data is untouched; it simply is not read as durable output.
	var legacyRows int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM synthesis_insights WHERE id = 'legacy-insight-1'`).Scan(&legacyRows); err != nil {
		t.Fatalf("count legacy rows: %v", err)
	}
	if legacyRows != 1 {
		t.Fatalf("legacy row count %d, want 1; classifying legacy data must not destroy it", legacyRows)
	}
}
