// BUG-069-001 regression — PreFacadeChain wired into the live
// POST /api/assistant/turn route (spec 069 SCOPE-2).
//
// Why this file exists: the SCOPE-2 integration tests
// (tests/integration/api/assistant_http_{auth,limits}_test.go) hand-wire
// PreFacadeChain via a synthetic `mountScope2Route` helper
// (chi `r.Use(httpadapter.PreFacadeChain(cfg))`). That helper NEVER
// exercises the production wiring shape — the late-bound
// `svc.assistantHTTPHandler.SetMiddleware(httpadapter.PreFacadeChain(transportCfg))`
// call in cmd/core/wiring_assistant_facade.go. The bug
// (BUG-069-001) was that production installed an identity
// pass-through `func(next) { return next }` there, so the live route
// ran the adapter's unbounded io.ReadAll with no body cap (413), no
// rate limit (429), and no assistant:turn scope gate (403) — yet the
// synthetic tests stayed GREEN because they bypassed the broken seam.
//
// This regression closes that blind spot: it drives the REAL
// production wiring function `wireAssistantHTTPAdapter`, then sends
// genuine *http.Request values through the resulting LateBoundHandler
// and asserts 413 / 429 / 403 / 200(shared-token). It is
// non-tautological: reverting line 315 to the identity pass-through
// makes the 413/429/403 assertions FAIL (proven by mutate-prove-revert
// in the BUG-069-001 report.md RED→GREEN evidence).
//
// Bearer auth is router-owned and runs BEFORE the LateBoundHandler in
// production; this test injects the auth.Session into the request
// context exactly as the bearer middleware would, which is the
// faithful precondition for the seam under test. The bug is in the
// SetMiddleware argument, not in bearer auth.
//
// Scenarios: BUG-069-001-SCN-001..004 (+ SCN-005 via mutate-prove-revert).

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// preFacadeRegressionFacade counts Handle invocations so the
// rejection paths can assert the facade is NEVER reached, and returns
// a minimal successful response (CaptureRoute=false so the capture
// path — and thus svc.proc — is never touched) for the accept paths.
type preFacadeRegressionFacade struct {
	calls int
}

func (f *preFacadeRegressionFacade) Handle(_ context.Context, _ contracts.AssistantMessage) (contracts.AssistantResponse, error) {
	f.calls++
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		Body:         "ok",
		EmittedAt:    time.Unix(1735689600, 0).UTC(),
		CaptureRoute: false,
	}, nil
}

// basePreFacadeRegressionConfig returns a *config.Config whose
// assistant.transports.http block satisfies the same fail-loud SST
// contract wireAssistantHTTPAdapter consumes in production. Body cap
// and rate limit are generous by default; individual sub-tests tighten
// the single dimension they exercise.
func basePreFacadeRegressionConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Assistant.HTTP = config.AssistantHTTPTransportConfig{
		HTTPEnabled:                   true,
		HTTPSchemaVersion:             httpadapter.SchemaVersionV1,
		HTTPBodySizeMaxBytes:          65536,
		HTTPRateLimitPerUserPerMinute: 60,
		HTTPConversationTTL:           time.Hour,
		HTTPRequiredScope:             "assistant:turn",
		HTTPCORSAllowedOrigins:        []string{"https://app.example.test"},
		HTTPTransportHintAllowlist:    []string{"web", "mobile", "bridge"},
		HTTPSharedUserID:              "shared",
	}
	return cfg
}

// wirePreFacadeRegression builds the late-bound handler the SAME way
// production does — by calling the real wireAssistantHTTPAdapter, which
// performs svc.assistantHTTPHandler.SetMiddleware(httpadapter.PreFacadeChain(transportCfg)).
// It returns the production http.Handler (the LateBoundHandler) and the
// counting facade. Each call is a fresh wiring so the per-process
// httprate limiter state does not bleed across sub-tests.
func wirePreFacadeRegression(t *testing.T, mutate func(*config.Config)) (http.Handler, *preFacadeRegressionFacade) {
	t.Helper()
	cfg := basePreFacadeRegressionConfig()
	if mutate != nil {
		mutate(cfg)
	}
	facade := &preFacadeRegressionFacade{}
	svc := &coreServices{
		assistantHTTPHandler: httpadapter.NewLateBoundHandler(),
		// Non-nil placeholder: the capture path (the only proc
		// consumer) is never invoked because the accept-path facade
		// returns CaptureRoute=false and the reject paths never reach
		// the facade. wireAssistantHTTPAdapter only requires proc to be
		// non-nil at wiring time.
		proc: &pipeline.Processor{},
	}
	if err := wireAssistantHTTPAdapter(cfg, svc, facade); err != nil {
		t.Fatalf("wireAssistantHTTPAdapter: %v", err)
	}
	return svc.assistantHTTPHandler, facade
}

// postTurn drives one request through the wired production handler
// with the supplied session injected into the context (mirroring the
// router-owned bearer middleware), returning the recorder.
func postTurn(t *testing.T, h http.Handler, sess auth.Session, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/assistant/turn", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	req = req.WithContext(auth.WithSession(req.Context(), sess))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// smallValidTurnBody marshals a within-cap, schema-valid text turn.
func smallValidTurnBody(t *testing.T, id string) []byte {
	t.Helper()
	b, err := json.Marshal(httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: id,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "ping",
	})
	if err != nil {
		t.Fatalf("marshal turn: %v", err)
	}
	return b
}

func perUserSession(userID string, scopes []string) auth.Session {
	return auth.Session{
		UserID: userID,
		Source: auth.SessionSourcePerUserToken,
		Scopes: scopes,
	}
}

func sharedTokenSession() auth.Session {
	// Shared-token sessions carry no UserID and no scopes; the adapter
	// substitutes cfg.HTTPSharedUserID and RequireScope bypasses them.
	return auth.Session{Source: auth.SessionSourceSharedToken}
}

// TestAssistantHTTPPreFacadeChainWiredIntoLiveRoute is the BUG-069-001
// regression. It drives the production wireAssistantHTTPAdapter path
// (NOT the synthetic mountScope2Route) and proves the three SCOPE-2
// controls are actually enforced on the live route, plus that the dev
// shared-token bypass still passes.
func TestAssistantHTTPPreFacadeChainWiredIntoLiveRoute(t *testing.T) {
	// SCN-001 — oversized body → 413 before the facade.
	t.Run("oversized_body_returns_413_before_facade", func(t *testing.T) {
		handler, facade := wirePreFacadeRegression(t, func(c *config.Config) {
			c.Assistant.HTTP.HTTPBodySizeMaxBytes = 256 // tiny cap so a small payload trips it
		})
		body, err := json.Marshal(httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "oversize-1",
			Kind:               string(contracts.KindText),
			TransportHint:      "web",
			Text:               strings.Repeat("x", 1024), // > 256-byte cap
		})
		if err != nil {
			t.Fatalf("marshal oversize turn: %v", err)
		}
		// A per-user session WITH the required scope so the request
		// clears the scope + rate layers and the 413 is unambiguously
		// the body cap (not a 403/429).
		rr := postTurn(t, handler, perUserSession("user-413", []string{"assistant:turn"}), body)

		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413; body=%s", rr.Code, rr.Body.String())
		}
		env := decodePreFacadeEnvelope(t, rr)
		if env.ErrorCause != "body_too_large" {
			t.Errorf(`error_cause = %q, want "body_too_large"`, env.ErrorCause)
		}
		if facade.calls != 0 {
			t.Errorf("facade invoked %d times on oversized body; want 0", facade.calls)
		}
	})

	// SCN-002 — per-user rate limit → 429, second user unaffected.
	t.Run("per_user_rate_limit_returns_429", func(t *testing.T) {
		handler, facade := wirePreFacadeRegression(t, func(c *config.Config) {
			c.Assistant.HTTP.HTTPRateLimitPerUserPerMinute = 1 // 1st allowed, 2nd rejected
		})
		userA := perUserSession("rate-user-A", []string{"assistant:turn"})

		first := postTurn(t, handler, userA, smallValidTurnBody(t, "rl-a-1"))
		if first.Code != http.StatusOK {
			t.Fatalf("user-A first status = %d, want 200; body=%s", first.Code, first.Body.String())
		}
		if facade.calls != 1 {
			t.Fatalf("facade calls after user-A first = %d, want 1", facade.calls)
		}

		second := postTurn(t, handler, userA, smallValidTurnBody(t, "rl-a-2"))
		if second.Code != http.StatusTooManyRequests {
			t.Fatalf("user-A second status = %d, want 429; body=%s", second.Code, second.Body.String())
		}
		env := decodePreFacadeEnvelope(t, second)
		if env.ErrorCause != "rate_limited" {
			t.Errorf(`error_cause = %q, want "rate_limited"`, env.ErrorCause)
		}
		if facade.calls != 1 {
			t.Errorf("facade invoked %d times after rate-limit; want 1 (user-A first only)", facade.calls)
		}

		// Adversarial: a limiter keyed on something other than the user
		// (remote addr, endpoint) would share buckets. A second,
		// under-budget user MUST still succeed.
		userB := perUserSession("rate-user-B", []string{"assistant:turn"})
		third := postTurn(t, handler, userB, smallValidTurnBody(t, "rl-b-1"))
		if third.Code != http.StatusOK {
			t.Fatalf("user-B first status = %d, want 200 (per-user keying); body=%s", third.Code, third.Body.String())
		}
		if facade.calls != 2 {
			t.Errorf("facade calls after user-B first = %d, want 2", facade.calls)
		}
	})

	// SCN-003 — per-user PASETO without assistant:turn → 403 before the facade.
	t.Run("missing_turn_scope_returns_403_before_facade", func(t *testing.T) {
		handler, facade := wirePreFacadeRegression(t, nil)
		// Real per-user session whose scopes do NOT include assistant:turn.
		sess := perUserSession("user-403", []string{"connector:ingest"})
		rr := postTurn(t, handler, sess, smallValidTurnBody(t, "scope-1"))

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode 403 body: %v; raw=%s", err, rr.Body.String())
		}
		if got, _ := body["error"].(string); got != "scope_required" {
			t.Errorf(`403 body.error = %q, want "scope_required"; raw=%s`, got, rr.Body.String())
		}
		if facade.calls != 0 {
			t.Errorf("facade invoked %d times on missing-scope; want 0", facade.calls)
		}
	})

	// SCN-004 — dev shared-token bypass still passes (no regression).
	t.Run("shared_token_within_limits_returns_200", func(t *testing.T) {
		handler, facade := wirePreFacadeRegression(t, nil)
		rr := postTurn(t, handler, sharedTokenSession(), smallValidTurnBody(t, "shared-1"))

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
		}
		var env httpadapter.TurnResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode 200 envelope: %v; raw=%s", err, rr.Body.String())
		}
		if env.SchemaVersion != httpadapter.SchemaVersionV1 {
			t.Errorf("schema_version = %q, want %q", env.SchemaVersion, httpadapter.SchemaVersionV1)
		}
		if !env.FacadeInvoked {
			t.Errorf("facade_invoked = false on shared-token success; want true; raw=%s", rr.Body.String())
		}
		if facade.calls != 1 {
			t.Errorf("facade calls on shared-token success = %d, want 1", facade.calls)
		}
	})
}

// decodePreFacadeEnvelope decodes a v1 wire envelope from a pre-facade
// rejection and asserts the stable invariants (schema/transport and
// facade_invoked=false) that every rejection must satisfy.
func decodePreFacadeEnvelope(t *testing.T, rr *httptest.ResponseRecorder) httpadapter.TurnResponse {
	t.Helper()
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode v1 envelope: %v\nraw=%s", err, rr.Body.String())
	}
	if out.SchemaVersion != httpadapter.SchemaVersionV1 {
		t.Errorf("schema_version = %q, want %q", out.SchemaVersion, httpadapter.SchemaVersionV1)
	}
	if out.Transport != httpadapter.TransportName {
		t.Errorf("transport = %q, want %q", out.Transport, httpadapter.TransportName)
	}
	if out.FacadeInvoked {
		t.Errorf("facade_invoked = true on pre-facade rejection; want false; raw=%s", rr.Body.String())
	}
	return out
}
