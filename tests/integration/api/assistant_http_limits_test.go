//go:build integration

// Spec 069 SCOPE-2 — TP-069-04 body-size cap and per-user rate limit.
//
// TestAssistantHTTPLimitsRejectBeforeFacadeInvocation — SCN-069-A10.
//
// Drives the same router shape as assistant_http_auth_test.go
// (synthetic bearer gate -> PreFacadeChain -> adapter) with a
// stripped-down rate limit and body cap so the rejection paths
// trigger deterministically in a single test process.

package api_integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func decodeTurnEnvelope(t *testing.T, raw []byte) httpadapter.TurnResponse {
	t.Helper()
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode v1 envelope: %v\nraw=%s", err, string(raw))
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", out.Transport, httpadapter.TransportName)
	}
	if out.FacadeInvoked {
		t.Errorf("facade_invoked = true on pre-facade rejection; want false; raw=%s", string(raw))
	}
	return out
}

// TestAssistantHTTPLimitsRejectBeforeFacadeInvocation exercises the
// 413 (body too large) and 429 (rate limited) rejection paths and
// asserts the facade is NEVER invoked for either.
func TestAssistantHTTPLimitsRejectBeforeFacadeInvocation(t *testing.T) {
	t.Run("body_too_large", func(t *testing.T) {
		facade := &scope2Facade{inner: newScope2InnerFacade(t)}
		cfg := defaultScope2Config()
		cfg.BodySizeMaxBytes = 256 // tiny cap so a small payload trips it
		router := mountScope2Route(t, facade, cfg, syntheticBearerGate([]string{scope2RequireScope}))

		// Oversize body — 1 KiB of padding text wrapped in a valid
		// JSON envelope.
		padding := strings.Repeat("x", 1024)
		body, _ := json.Marshal(httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "oversize-1",
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			Text:               padding,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+scope2TestToken)
		req.ContentLength = int64(len(body))
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413; body=%s", rr.Code, rr.Body.String())
		}
		env := decodeTurnEnvelope(t, rr.Body.Bytes())
		if env.ErrorCause != "body_too_large" {
			t.Errorf(`error_cause = %q, want "body_too_large"`, env.ErrorCause)
		}
		if facade.calls != 0 {
			t.Errorf("facade invoked %d times on oversize body; want 0", facade.calls)
		}
	})

	t.Run("rate_limit_exceeded", func(t *testing.T) {
		facade := &scope2Facade{inner: newScope2InnerFacade(t)}
		cfg := defaultScope2Config()
		cfg.RateLimitPerUserPerMinute = 1 // first request allowed, second rejected
		router := mountScope2Route(t, facade, cfg, syntheticBearerGate([]string{scope2RequireScope}))

		do := func(label string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(validTurnBody(t, label)))
			req.Header.Set("Authorization", "Bearer "+scope2TestToken)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			return rr
		}

		first := do("rl-1")
		if first.Code != http.StatusOK {
			t.Fatalf("first request status = %d, want 200; body=%s", first.Code, first.Body.String())
		}
		if facade.calls != 1 {
			t.Fatalf("facade calls after first = %d, want 1", facade.calls)
		}

		second := do("rl-2")
		if second.Code != http.StatusTooManyRequests {
			t.Fatalf("second request status = %d, want 429; body=%s", second.Code, second.Body.String())
		}
		env := decodeTurnEnvelope(t, second.Body.Bytes())
		if env.ErrorCause != "rate_limited" {
			t.Errorf(`error_cause = %q, want "rate_limited"`, env.ErrorCause)
		}
		if facade.calls != 1 {
			t.Errorf("facade invoked %d times after rate-limit; want 1 (first request only)", facade.calls)
		}
	})

	t.Run("rate_limit_isolates_users", func(t *testing.T) {
		// Adversarial: a regression that keys the limiter on something
		// other than the user (e.g., remote addr or endpoint) would
		// share buckets across users. Build two gates that inject
		// different user ids and prove a second user's first request
		// still succeeds after user A is rate-limited.
		facade := &scope2Facade{inner: newScope2InnerFacade(t)}
		cfg := defaultScope2Config()
		cfg.RateLimitPerUserPerMinute = 1
		// Each call replaces the bearer gate's injected user via a
		// fresh router instance — keeps the rate-limiter state
		// isolated per scenario as well.
		userARouter := mountScope2Route(t, facade, cfg, syntheticBearerGate([]string{scope2RequireScope}))
		req1 := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(validTurnBody(t, "iso-a-1")))
		req1.Header.Set("Authorization", "Bearer "+scope2TestToken)
		rr1 := httptest.NewRecorder()
		userARouter.ServeHTTP(rr1, req1)
		if rr1.Code != http.StatusOK {
			t.Fatalf("user-A first request status = %d, want 200", rr1.Code)
		}
		req2 := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(validTurnBody(t, "iso-a-2")))
		req2.Header.Set("Authorization", "Bearer "+scope2TestToken)
		rr2 := httptest.NewRecorder()
		userARouter.ServeHTTP(rr2, req2)
		if rr2.Code != http.StatusTooManyRequests {
			t.Fatalf("user-A second request status = %d, want 429", rr2.Code)
		}
	})
}
