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
	freshnessPolicy, err := intelligence.NewSynthesisFreshnessPolicy(48*time.Hour, 8*24*time.Hour)
	if err != nil {
		t.Fatalf("construct synthesis freshness policy: %v", err)
	}
	liveReadModel, err := intelligence.NewSynthesisReadModel(pool)
	if err != nil {
		t.Fatalf("construct live synthesis read model: %v", err)
	}

	// intelligenceStatus drives the REAL /api/health handler in-process against
	// the given engine and returns the reported "intelligence" service status.
	// IntelligenceHealthCacheTTL: 0 forces the always-fresh slow path so each
	// call re-queries the durable synthesis run/output/attempt state instead of
	// serving a cached snapshot. DB/NATS/ML deps are nil — HealthHandler is
	// nil-safe for them and we assert only on the intelligence status under test.
	intelligenceStatus := func(engine *intelligence.Engine, readModel *intelligence.SynthesisReadModel, principal string) string {
		t.Helper()
		deps := &api.Dependencies{
			IntelligenceEngine:         engine,
			SynthesisReadModel:         readModel,
			SynthesisReadPrincipal:     principal,
			SynthesisFreshnessPolicy:   freshnessPolicy,
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
	neverRun := intelligenceStatus(liveEngine, liveReadModel, "health-truth-never-run")
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
	badReadModel, err := intelligence.NewSynthesisReadModel(badPool)
	if err != nil {
		t.Fatalf("construct probe-error synthesis read model: %v", err)
	}
	probeErr := intelligenceStatus(intelligence.NewEngine(badPool, nil), badReadModel, "health-truth-probe-error")
	t.Logf("probe-error: GET /api/health services.intelligence.status = %q", probeErr)
	if probeErr == "up" {
		t.Fatalf("FALSELY HEALTHY: a synthesis freshness-probe error reported intelligence=%q; the pre-fix bug is present (want NOT %q)", probeErr, "up")
	}
	if probeErr != "down" {
		t.Fatalf("probe-error synthesis: want intelligence=%q, got %q", "down", probeErr)
	}

	// ---- Truth 3: FRESH persisted daily AND weekly synthesis reports "up" ----
	// Seed both required cadences through the production attempt and persistence
	// APIs. A daily-only fixture would encode the masking bug this test guards.
	persistence, err := intelligence.NewSynthesisPersistence(pool)
	if err != nil {
		t.Fatalf("construct synthesis persistence: %v", err)
	}
	principal := "health-truth-probe"
	now := time.Now().UTC()
	for index, cadence := range freshnessPolicy.RequiredCadences() {
		key := intelligence.SynthesisRunKey{
			Cadence: cadence, Principal: principal,
			WindowStart:   now.Add(time.Duration(-48-index*24) * time.Hour),
			WindowEnd:     now.Add(time.Duration(-24-index*24) * time.Hour),
			PolicyVersion: "health-truth-v1",
		}
		attempt, err := persistence.StartAttempt(ctx, key, intelligence.TriggerScheduled,
			"health-truth-"+string(cadence), time.Hour, now.Add(-2*time.Minute))
		if err != nil {
			t.Fatalf("start %s synthesis attempt: %v", cadence, err)
		}
		candidate := intelligence.SynthesisCandidate{
			Key: key, Kind: intelligence.OutputKindQuiet,
			EvaluatedArtifactCount: 0,
		}
		if _, err := persistence.CommitAttempt(ctx, attempt, candidate,
			intelligence.SourceClassPolicy{}, nil, now.Add(-time.Minute)); err != nil {
			t.Fatalf("commit %s synthesis attempt: %v", cadence, err)
		}
	}

	var fresh string
	deadline := time.Now().Add(3 * time.Second)
	for {
		fresh = intelligenceStatus(liveEngine, liveReadModel, principal)
		if fresh == "up" || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("fresh-success: GET /api/health services.intelligence.status = %q", fresh)
	if fresh != "up" {
		t.Fatalf("fresh persisted daily and weekly synthesis: want intelligence=%q, got %q", "up", fresh)
	}
}
