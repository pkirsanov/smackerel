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
// and NO canned response: GetLastSynthesisTime executes its real
//
//	SELECT COALESCE(MAX(created_at), '1970-01-01') FROM synthesis_insights
//
// query against the real database, and the intelligence TTL cache is disabled
// (IntelligenceHealthCacheTTL: 0) so every probe reflects the CURRENT durable
// table state rather than a cached snapshot.
//
// Adversarial truths asserted — each would FAIL against the pre-fix mapping,
// so this test is a live regression guard for the "falsely healthy" bug:
//
//	1. never-run   (empty synthesis_insights)          => intelligence != "up" (== "down")
//	2. probe error (query against a closed pool)         => intelligence != "up" (== "down")
//	3. fresh insight (created_at = NOW(), within budget) => intelligence == "up"
//
// The disposable test DB is mutated (DELETE/INSERT synthesis_insights); the
// inserted row is removed on cleanup (ephemeral-store hygiene). The focused
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
		t.Skip("integration: DATABASE_URL not set — live stack not available")
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
	// call re-queries the durable synthesis_insights state instead of serving a
	// cached snapshot. DB/NATS/ML deps are nil — HealthHandler is nil-safe for
	// them and we assert only on the intelligence status under test.
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
	if _, err := pool.Exec(ctx, "DELETE FROM synthesis_insights"); err != nil {
		t.Fatalf("clear synthesis_insights for never-run state: %v", err)
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
	// A closed (non-nil) pool makes GetLastSynthesisTime's query return an
	// error (not the nil-pool short-circuit), exercising the real error branch
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
	// Insert one synthesis_insights row with created_at = NOW() (within the
	// freshness budget). Cleaned up on exit for ephemeral-store hygiene.
	insightID := "bug-004-004-health-truth-" + time.Now().UTC().Format("20060102T150405.000000000")
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := pool.Exec(cctx, "DELETE FROM synthesis_insights WHERE id = $1", insightID); err != nil {
			t.Logf("cleanup synthesis_insights row %s failed: %v", insightID, err)
		}
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO synthesis_insights (id, insight_type, through_line, source_artifact_ids, created_at)
		 VALUES ($1, $2, $3, $4, NOW())`,
		insightID, "health-truth-probe", "BUG-004-004 live health-truth fresh-success probe", []string{},
	); err != nil {
		t.Fatalf("insert fresh synthesis_insights row: %v", err)
	}
	fresh := intelligenceStatus(liveEngine)
	t.Logf("fresh-success: GET /api/health services.intelligence.status = %q", fresh)
	if fresh != "up" {
		t.Fatalf("fresh persisted synthesis: want intelligence=%q, got %q", "up", fresh)
	}
}
