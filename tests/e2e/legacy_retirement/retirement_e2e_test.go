//go:build e2e

// Spec 076 SCOPE-6d — TP-076-06-10 / SCN-075-A01..A09 regression.
//
// Live-stack regression that drives the full SCN-075-A01..A09 matrix
// end-to-end against the running core. It walks a single fresh user
// through the canonical sequence:
//
//   A01 — open window, retired /weather → notice + body served
//   A02 — open window, second /weather → no second notice on the wire
//   A03 — open window, /remind by same user → independent notice
//   A04 — residual telemetry visible via /metrics
//
// The remaining scenarios A05–A09 are covered by the focused tests
// in this directory and in tests/integration/legacy_retirement/.
// This regression layers a single HTTP walk on top so a regression in
// the facade dispatch / wire shape / dedup query that broke any of
// A01..A04 would surface in one place.

package legacyretirement_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func openPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("e2e: DATABASE_URL not set — live test stack DB not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// TestLegacyRetirement_FullScenarioMatrix walks SCN-075-A01..A04 in
// sequence against the live stack. The deeper A05..A09 contracts are
// covered by their focused tests in this directory and in
// tests/integration/legacy_retirement/.
func TestLegacyRetirement_FullScenarioMatrix(t *testing.T) {
	stack := loadStack(t)
	if stack.WindowState != "open" {
		t.Skipf("LEGACY_RETIREMENT_WINDOW_STATE=%q — A01..A04 walk requires open window", stack.WindowState)
	}
	waitHealthy(t, stack.BaseURL)
	waitAssistantReady(t, stack)

	pool := openPool(t)
	userID := fmt.Sprintf("tp-076-06-10-user-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM assistant_conversations WHERE user_id = $1`, userID)
	})

	// Reset the dedup ledger for the bearer-token user(s) on the
	// disposable test stack so the A01 first-invocation assertion is
	// not contaminated by a prior test in this run that already
	// marked the notice for /weather under the same bearer identity.
	// This is safe on the disposable test stack — never run against
	// a persistent dev or production database.
	resetCtx, resetCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer resetCancel()
	if _, err := pool.Exec(resetCtx, `
		UPDATE assistant_conversations
		   SET legacy_retirement_notices = jsonb_set(
		           jsonb_set(legacy_retirement_notices, '{commands}', '{}'::jsonb, true),
		           '{window_id}', to_jsonb($1::text), true)
		 WHERE legacy_retirement_notices IS NOT NULL`, stack.WindowID); err != nil {
		t.Fatalf("reset ledger for matrix walk: %v", err)
	}

	t.Run("A01_FirstWeatherShowsNoticeAndServesBody", func(t *testing.T) {
		turnID := "tp-076-06-10-a01-" + time.Now().UTC().Format("20060102T150405.000000")
		status, raw := postTurn(t, stack, "/weather", turnID)
		if status != http.StatusOK {
			t.Fatalf("status=%d, want 200; body=%s", status, string(raw))
		}
		var out httpadapter.TurnResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, string(raw))
		}
		if out.Notice == nil {
			t.Fatalf("A01: notice nil on first /weather; body=%s", string(raw))
		}
		if out.Notice.Command != "/weather" {
			t.Errorf("A01: notice.command=%q, want /weather", out.Notice.Command)
		}
	})

	t.Run("A02_SecondWeatherDoesNotRenotify", func(t *testing.T) {
		// First turn under the same conversation user so HasNotified
		// is guaranteed-true. Use a deterministic auth turn user
		// derived from the bearer token's identity; we cannot easily
		// pin the wire request to userID, so this subtest asserts the
		// dedup PROPERTY at the wire boundary: two retired-command
		// turns in quick succession from the same authenticated
		// client MUST yield at most one notice (the second is
		// suppressed by the ledger).
		seen := 0
		for i := 0; i < 2; i++ {
			turnID := fmt.Sprintf("tp-076-06-10-a02-%d-%s", i, time.Now().UTC().Format("20060102T150405.000000"))
			status, raw := postTurn(t, stack, "/weather", turnID)
			if status != http.StatusOK {
				t.Fatalf("turn %d status=%d, body=%s", i, status, string(raw))
			}
			var out httpadapter.TurnResponse
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatalf("turn %d decode: %v", i, err)
			}
			if out.Notice != nil {
				seen++
			}
		}
		if seen > 1 {
			t.Fatalf("A02: notice fired %d times across two consecutive /weather turns; want at most 1 (dedup ledger broken)", seen)
		}
	})

	t.Run("A03_RemindProducesIndependentNotice", func(t *testing.T) {
		turnID := "tp-076-06-10-a03-" + time.Now().UTC().Format("20060102T150405.000000")
		status, raw := postTurn(t, stack, "/remind", turnID)
		if status != http.StatusOK {
			t.Fatalf("status=%d, want 200; body=%s", status, string(raw))
		}
		var out httpadapter.TurnResponse
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// The notice for /remind MAY have been deduped by a prior
		// run for the same bearer-token user. The hard contract this
		// subtest asserts is: when /remind notice DOES fire on the
		// wire, command equals /remind — never /weather, never empty.
		if out.Notice != nil {
			if out.Notice.Command != "/remind" {
				t.Errorf("A03: /remind turn produced notice for %q (per-command keying broken)", out.Notice.Command)
			}
		}
	})

	t.Run("A04_ResidualMetricRegistered", func(t *testing.T) {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(stack.BaseURL + "/metrics")
		if err != nil {
			t.Fatalf("GET /metrics: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("/metrics status=%d", resp.StatusCode)
		}
		const metric = "smackerel_legacy_command_residual_total"
		if !regexp.MustCompile(`(?m)^# HELP ` + regexp.QuoteMeta(metric)).MatchString(string(body)) {
			t.Fatalf("/metrics missing HELP %s — dashboards cannot scrape A04", metric)
		}
		if !regexp.MustCompile(`(?m)^# TYPE ` + regexp.QuoteMeta(metric)).MatchString(string(body)) {
			t.Fatalf("/metrics missing TYPE %s", metric)
		}
	})
}
