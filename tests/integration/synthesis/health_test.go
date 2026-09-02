//go:build integration

// BUG-004-004 · SCOPE-04 · T004-05-HEALTH · SCN-004-004-05.
//
// Live-stack proof that GET /api/health's "intelligence" service status is
// routed through the single truthful HEALTH-TRUTH authority
// (intelligence.DeriveSynthesisHealth) that internal/api/health.go wires in.
//
// Pre-fix bug (internal/api/health.go::getCachedIntelligenceHealth): a
// never-run synthesis (the epoch sentinel returned by GetLastSynthesisTime) AND
// a freshness-probe query error BOTH mapped to "up", so a system that never
// produced — or could not evaluate — any synthesis reported green.
//
// This test drives the REAL api.Dependencies.HealthHandler against the live
// disposable test PostgreSQL (DATABASE_URL). There is NO request interception
// and NO canned response: SynthesisReadModel executes its real output/run and
// attempt queries against the durable synthesis ledger. The intelligence TTL
// cache is disabled (IntelligenceHealthCacheTTL: 0) so every probe reflects the
// CURRENT durable state rather than a cached snapshot.
//
// Adversarial truths asserted — each would FAIL against the pre-fix mapping,
// so this test is a live regression guard for the "falsely healthy" bug:
//
//	1. never-run (empty durable ledger) => intelligence != "up" (== "down")
//	2. probe error (closed pool) => intelligence != "up" (== "down")
//	3. fresh linked output within budget => intelligence == "up"
//
// The disposable test DB is mutated (TRUNCATE/INSERT on the durable ledger);
// the inserted rows are removed on cleanup (ephemeral-store hygiene). The focused
// `--go-run TestNeverRunAndProbeFailureAreNeverUp` invocation runs only this
// test, so the table reset does not clobber a concurrently-running test.

package synthesis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/intelligence"
)

func TestNeverRunAndProbeFailureAreNeverUp(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("integration: DATABASE_URL not set — canonical integration lane configuration is required")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}

	// intelligenceStatus drives the REAL /api/health handler in-process against
	// the given engine and returns the reported "intelligence" service status.
	// IntelligenceHealthCacheTTL: 0 forces the always-fresh slow path so each
	// call re-queries the durable synthesis run/output/attempt state instead of
	// serving a cached snapshot. DB/NATS/ML deps are nil — HealthHandler is
	// nil-safe for them and we assert only on the intelligence status under test.
	intelligenceStatus := func(engine *intelligence.Engine) string {
		t.Helper()
		deps := &api.Dependencies{
			IntelligenceEngine:         engine,
			IntelligenceHealthCacheTTL: 0,
			StartTime:                  time.Now(),
		}
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		rec := httptest.NewRecorder()
		deps.HealthHandler(rec, req)

		var resp api.HealthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode /api/health body: %v (body=%q)", err, rec.Body.String())
		}
		return resp.Services["intelligence"].Status
	}

	liveEngine := intelligence.NewEngine(pool, nil)

	// ---- Truth 1: NEVER-RUN must never be "up" ----
	// Ensure the durable never-run state on the ephemeral test DB.
	if _, err := pool.Exec(ctx, `
		TRUNCATE synthesis_run_events, synthesis_citations,
			synthesis_output_insights, synthesis_output_source_classes,
			synthesis_outputs, synthesis_run_attempts, synthesis_runs
		RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("clear durable synthesis ledger for never-run state: %v", err)
	}
	neverRun := intelligenceStatus(liveEngine)
	t.Logf("never-run: GET /api/health services.intelligence.status = %q", neverRun)
	if neverRun == "up" {
		t.Fatalf("FALSELY HEALTHY: never-run synthesis reported intelligence=%q; the pre-fix bug is present (want NOT %q)", neverRun, "up")
	}
	if neverRun != "down" {
		t.Fatalf("never-run synthesis: want intelligence=%q, got %q", "down", neverRun)
	}

	// ---- Truth 2: a synthesis freshness PROBE ERROR must never be "up" ----
	// A closed (non-nil) pool makes SynthesisReadModel's query return an error
	// (not the nil-pool short-circuit), exercising the real error branch
	// that the pre-fix code mapped to "up".
	badPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open probe-error pool: %v", err)
	}
	badPool.Close() // subsequent queries return an error against a closed pool
	probeErr := intelligenceStatus(intelligence.NewEngine(badPool, nil))
	t.Logf("probe-error: GET /api/health services.intelligence.status = %q", probeErr)
	if probeErr == "up" {
		t.Fatalf("FALSELY HEALTHY: a synthesis freshness-probe error reported intelligence=%q; the pre-fix bug is present (want NOT %q)", probeErr, "up")
	}
	if probeErr != "down" {
		t.Fatalf("probe-error synthesis: want intelligence=%q, got %q", "down", probeErr)
	}

	// ---- Truth 3 (feasible): a FRESH persisted synthesis reports "up" ----
	// Seeds the DURABLE run ledger, not the legacy synthesis_insights table.
	// Health derives from synthesis_outputs joined to a succeeded run, so a
	// legacy insight row is correctly invisible to it and would assert nothing.
	runID := "bug-004-004-health-run-" + time.Now().UTC().Format("20060102T150405.000000000")
	outputID := "bug-004-004-health-out-" + time.Now().UTC().Format("20060102T150405.000000000")
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Migration 067 links attempts to runs with ON DELETE RESTRICT, so the
		// attempt must be removed before the run and its cascading output.
		if _, err := pool.Exec(cctx, "DELETE FROM synthesis_run_attempts WHERE logical_key = $1", runID); err != nil {
			t.Logf("cleanup synthesis_run_attempts for %s failed: %v", runID, err)
		}
		// synthesis_outputs cascades on the run delete.
		if _, err := pool.Exec(cctx, "DELETE FROM synthesis_runs WHERE id = $1", runID); err != nil {
			t.Logf("cleanup synthesis_runs row %s failed: %v", runID, err)
		}
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO synthesis_runs
		   (id, logical_key, cadence, principal, window_start, window_end,
		    policy_version, source_set_digest, state, created_at, updated_at)
		 VALUES ($1, $2, 'daily', 'health-truth-probe', NOW() - INTERVAL '1 hour', NOW(),
		         'v1', 'health-truth-probe-digest', 'succeeded', NOW(), NOW())`,
		runID, runID,
	); err != nil {
		t.Fatalf("insert fresh synthesis_runs row: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO synthesis_outputs
		   (id, run_id, insight_count, citation_count, output_kind,
		    evaluated_artifact_count, created_at)
		 VALUES ($1, $2, 1, 1, 'full', 1, NOW())`,
		outputID, runID,
	); err != nil {
		t.Fatalf("insert fresh synthesis_outputs row: %v", err)
	}
	// The read model takes the GLOBALLY latest attempt, not the attempt for this
	// run, so a failed attempt left by any earlier test would classify this as
	// failed-with-prior-output and report "down". Record a succeeded attempt so
	// the newest attempt belongs to this row.
	if _, err := pool.Exec(ctx,
		`INSERT INTO synthesis_run_attempts
		   (logical_key, outcome, recorded_at, run_id, attempt_no, trigger_kind,
		    state, output_id, started_at, finished_at, included_source_classes,
		    omitted_source_classes, insight_count, citation_count)
		 VALUES ($1, 'succeeded', NOW(), $1, 1, 'scheduled', 'persisted', $2,
		         NOW(), NOW(), '{}'::text[], '{}'::text[], 1, 1)`,
		runID, outputID,
	); err != nil {
		t.Fatalf("insert succeeded synthesis_run_attempts row: %v", err)
	}
	// The integration lane shares one database across packages that run in
	// parallel, so a concurrent cleanup can delete this row between the insert
	// and the health read. Poll briefly, and if it still is not "up", say
	// whether the row survived — otherwise the failure reads as a health defect
	// when it is actually cross-package interference.
	var fresh string
	deadline := time.Now().Add(3 * time.Second)
	for {
		fresh = intelligenceStatus(liveEngine)
		if fresh == "up" || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("fresh-success: GET /api/health services.intelligence.status = %q", fresh)
	if fresh != "up" {
		var survived bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM synthesis_outputs o
			   JOIN synthesis_runs r ON r.id = o.run_id
			  WHERE o.id = $1 AND r.state = 'succeeded')`, outputID,
		).Scan(&survived); err != nil {
			t.Fatalf("check seeded row survived: %v", err)
		}
		if !survived {
			t.Fatalf("seeded durable output %s was deleted before the health read; a concurrent package removed it, so this is test isolation, not a health defect", outputID)
		}
		// Health reads the NEWEST succeeded output, so a concurrent test's newer
		// row wins the LIMIT 1 even though this row survived. Name it.
		var winnerID, winnerKind string
		var winnerAt time.Time
		if err := pool.QueryRow(ctx,
			`SELECT o.id, o.output_kind, o.created_at
			   FROM synthesis_outputs o
			   JOIN synthesis_runs r ON r.id = o.run_id
			  WHERE r.state = 'succeeded'
			  ORDER BY o.created_at DESC LIMIT 1`,
		).Scan(&winnerID, &winnerKind, &winnerAt); err != nil {
			t.Fatalf("read newest succeeded output: %v", err)
		}
		if winnerID != outputID {
			t.Fatalf("health read the newest succeeded output %s (kind=%q at=%s), not this test's row %s; a concurrent package inserted a newer row, so this is test isolation, not a health defect",
				winnerID, winnerKind, winnerAt.UTC().Format(time.RFC3339Nano), outputID)
		}
		t.Fatalf("fresh persisted synthesis: want intelligence=%q, got %q (this test's row %s is the newest succeeded output, kind=%q)", "up", fresh, winnerID, winnerKind)
	}
}
