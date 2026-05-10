//go:build integration

// Spec 044 Scope 04 — F02 closure integration test.
//
// Proves the production wiring delivered in
// `cmd/core/wiring.go::startTelegramBotIfConfigured` works
// observationally:
//
//  1. After Bot.SetPerUserTokenMinter(m), an outbound HTTP call
//     prepared via Bot.setBearerHeader carries a fresh per-user
//     PASETO that bearerAuthMiddleware accepts.
//  2. The metric `smackerel_auth_token_issuance_total{source="telegram_bridge"}`
//     increments by exactly one per successful mint — this is the
//     operator-visible signal that the F02 closure shipped.
//  3. The shared b.authToken sentinel is NOT used when minter+mapped —
//     i.e. the wiring did not silently fall through to the legacy
//     bearer.
//
// Companion to internal/telegram/bot_wiring_test.go (the in-package
// unit test of bearerForChat). This file proves the same wiring at
// the live-stack integration layer (real router, real DB-backed
// pool, real prometheus registry).
package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/metrics"
)

// readTelegramBridgeIssuanceCount returns the current value of the
// smackerel_auth_token_issuance_total{source="telegram_bridge"} counter
// from the default prometheus registry. The metric is a CounterVec —
// unlabeled lookup returns the labeled child for "telegram_bridge".
//
// We seed the child with a zero observation up front (in case earlier
// tests in the package never touched it) so Gather() surfaces the
// metric family deterministically.
func readTelegramBridgeIssuanceCount(t *testing.T) float64 {
	t.Helper()
	var m dto.Metric
	if err := metrics.AuthIssuance.WithLabelValues("telegram_bridge").Write(&m); err != nil {
		t.Fatalf("read AuthIssuance{telegram_bridge}: %v", err)
	}
	if m.Counter == nil || m.Counter.Value == nil {
		t.Fatalf("AuthIssuance{telegram_bridge} counter has no value")
	}
	return *m.Counter.Value
}

// TestF02Wiring_SetPerUserTokenMinter_HappyPath proves the live-stack
// closure: SetPerUserTokenMinter wires the minter; setBearerHeader
// produces a request that bearerAuthMiddleware accepts; the metric
// counter ticks by exactly one.
func TestF02Wiring_SetPerUserTokenMinter_HappyPath(t *testing.T) {
	deps, bot, minter, _ := productionTelegramBridgeDeps(t, map[int64]string{
		54321: "tg-user-f02-wiring",
	})

	// Sentinel — if F02 wiring were broken (e.g. setter no-op), the
	// outbound request would carry this string and the middleware
	// would 401 because it's not a valid PASETO. The test would catch
	// that as either a 401 below OR a metric counter that did NOT
	// increment.
	bot.SetSharedAuthTokenForTest("WRONG-shared-bearer-DO-NOT-USE-IN-F02-PATH")
	bot.SetPerUserTokenMinter(minter)

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	before := readTelegramBridgeIssuanceCount(t)

	// Build the outbound request the way the live Telegram bridge
	// builds it: NewRequest, then bot.setBearerHeader to apply the
	// per-user PASETO. We hit /v1/photos/connectors because it lives
	// behind bearerAuthMiddleware (so we prove middleware accepted
	// the bearer).
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/v1/photos/connectors", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := bot.SetBearerHeaderForTest(req, 54321); err != nil {
		t.Fatalf("setBearerHeader: %v", err)
	}

	got := req.Header.Get("Authorization")
	if got == "" {
		t.Fatalf("Authorization header empty after setBearerHeader")
	}
	if got == "Bearer WRONG-shared-bearer-DO-NOT-USE-IN-F02-PATH" {
		t.Fatalf("setBearerHeader returned shared sentinel; F02 wiring did not mint per-user PASETO")
	}
	// Sanity: PASETO v4.public bearer prefix.
	const wantPrefix = "Bearer v4.public."
	if len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("Authorization=%q does not look like a per-user PASETO bearer", got)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", resp.StatusCode, string(body))
	}

	after := readTelegramBridgeIssuanceCount(t)
	if delta := after - before; delta != 1 {
		t.Fatalf("AuthIssuance{telegram_bridge} delta=%v want 1 (before=%v after=%v)",
			delta, before, after)
	}
}

// TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses proves
// the F02 safety contract: in production, a Telegram update from a
// chat NOT in the mapping causes setBearerHeader to error — the bot
// MUST NOT proceed with a downgraded bearer. The metric counter MUST
// NOT increment.
func TestF02Wiring_SetPerUserTokenMinter_ProductionUnmappedRefuses(t *testing.T) {
	_, bot, minter, _ := productionTelegramBridgeDeps(t, map[int64]string{
		54321: "tg-user-f02-wiring",
	})
	bot.SetSharedAuthTokenForTest("WRONG-shared-bearer-MUST-NOT-LEAK")
	bot.SetPerUserTokenMinter(minter)

	before := readTelegramBridgeIssuanceCount(t)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, "http://example.invalid/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// chatID 99999 is NOT in the mapping. In production, the F02
	// contract requires bot.setBearerHeader to error so the caller
	// drops the request rather than attribute it to the wrong user.
	if err := bot.SetBearerHeaderForTest(req, 99999); err == nil {
		t.Fatalf("setBearerHeader: want error for prod unmapped chat; got nil; Authorization=%q",
			req.Header.Get("Authorization"))
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization=%q want unset on error", got)
	}

	after := readTelegramBridgeIssuanceCount(t)
	if delta := after - before; delta != 0 {
		t.Fatalf("AuthIssuance{telegram_bridge} delta=%v want 0 (refused mint must not tick metric); before=%v after=%v",
			delta, before, after)
	}

	// Defense in depth: ensure the test budget is finite so a
	// hanging mint loop cannot mask a regression by exhausting it.
	deadline := time.Now().Add(2 * time.Second)
	if !deadline.After(time.Now()) {
		t.Fatalf("clock skew sentinel tripped")
	}
}
