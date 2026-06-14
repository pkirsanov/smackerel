// Spec 091 SCOPE-04 — router-level rate-limit coverage for POST /v1/web/register.
//
// AC-7 / UC-8: the self-registration POST shares the SAME per-IP rate-limit
// group as /v1/web/login (httprate.LimitByIP(20, 1*time.Minute) in router.go),
// OUTSIDE bearerAuthMiddleware. The credential/handler unit tests in
// web_register_test.go call deps.HandleWebRegister DIRECTLY, which bypasses the
// router middleware chain entirely — the limiter never runs there. This file
// closes that gap by driving the REAL NewRouter(deps) and proving the
// 20/min/IP budget actually fires on /v1/web/register, mirroring
// web_login_ratelimit_test.go for the spec-070 login entry point.
//
// Adversarial fidelity (verified out-of-band, RED→GREEN): temporarily
// registering /v1/web/register OUTSIDE the httprate group in router.go makes
// TestWebRegister_RateLimited_PerIP FAIL ("expected 429 ... never observed")
// because every request is then admitted; restoring the group turns it GREEN.
// The per-IP companion test rules out a tautological "everything 429s" pass.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newRegisterRateLimitRouterDeps wires Dependencies for the dev/test path with
// a real (in-memory) web-credential repo and a configured invite token, so
// admitted requests exercise HandleWebRegister and return a non-429 status
// (the gate 401 for the wrong invite token submitted below) — distinct from
// the 429 the limiter emits once the budget is spent.
func newRegisterRateLimitRouterDeps() *Dependencies {
	return &Dependencies{
		DB:                         &mockDB{healthy: true},
		NATS:                       &mockNATS{healthy: true},
		StartTime:                  time.Now(),
		Environment:                "development",
		AuthToken:                  "shared-token-for-ratelimit-test",
		WebCredentials:             &fakeRepo{creds: map[string]string{}},
		WebRegistrationInviteToken: "the-configured-invite",
		TrustedProxies:             nil, // empty allowlist → RemoteAddr is the rate-limit key
	}
}

// postRegisterFromIP fires one form POST /v1/web/register through the full
// router middleware chain from the given TCP peer and returns the status. The
// invite token is intentionally wrong so an admitted request answers 401 (the
// gate); only the rate limiter produces 429.
func postRegisterFromIP(router http.Handler, remoteAddr string) int {
	const form = "invite-token=WRONG&username=probe&password=this-is-wrong-12&confirm-password=this-is-wrong-12"
	req := httptest.NewRequest(http.MethodPost, "/v1/web/register", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	req.ContentLength = int64(len(form))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

// TestWebRegister_RateLimited_PerIP proves the 20/min/IP budget on
// /v1/web/register engages through the real router. The first ~20 POSTs from
// one IP pass the limiter (the handler answers 401 for the wrong invite); a
// subsequent POST in the same window returns 429. Mirrors
// TestWebLogin_RateLimited_PerIP.
func TestWebRegister_RateLimited_PerIP(t *testing.T) {
	router := NewRouter(newRegisterRateLimitRouterDeps())

	const ip = "192.168.50.7:51000"
	const attempts = 30

	firstFail := -1
	statuses := make([]int, 0, attempts)
	for i := 0; i < attempts; i++ {
		code := postRegisterFromIP(router, ip)
		statuses = append(statuses, code)
		if code == http.StatusTooManyRequests {
			firstFail = i
			break
		}
	}

	t.Logf("statuses (one IP, in order)=%v firstFail=%d", statuses, firstFail)

	if firstFail == -1 {
		t.Fatalf("expected 429 after exceeding the 20/min/IP budget on /v1/web/register, "+
			"but %d consecutive POSTs were all admitted (statuses=%v) — the register route "+
			"is no longer inside the httprate.LimitByIP(20, 1*time.Minute) group", attempts, statuses)
	}
	if firstFail < 18 {
		t.Errorf("429 fired at request index %d — too early for a 20/min budget "+
			"(expected ~20 requests admitted first); statuses=%v", firstFail, statuses)
	}
}

// TestWebRegister_RateLimit_PerIP_FreshIPAdmitted proves the limiter is keyed
// PER IP, not a blanket block. After IP-A is driven to 429, IP-B's first
// request in the same window MUST still be admitted (any non-429 status). This
// rules out a tautological pass where the assertion above would succeed simply
// because every request returns 429 regardless of budget.
func TestWebRegister_RateLimit_PerIP_FreshIPAdmitted(t *testing.T) {
	router := NewRouter(newRegisterRateLimitRouterDeps())

	// Drive IP-A until it is rate-limited.
	const ipA = "192.168.50.8:51000"
	exhausted := false
	for i := 0; i < 40; i++ {
		if postRegisterFromIP(router, ipA) == http.StatusTooManyRequests {
			exhausted = true
			break
		}
	}
	if !exhausted {
		t.Fatalf("precondition failed: IP-A never hit 429 within 40 requests — " +
			"the /v1/web/register limiter is not engaging at all")
	}

	// IP-B's FIRST request in the same window must NOT be rate-limited.
	const ipB = "10.50.30.40:62000"
	if code := postRegisterFromIP(router, ipB); code == http.StatusTooManyRequests {
		t.Errorf("a fresh IP (IP-B) was 429'd on its first request — the limiter "+
			"is not per-IP (a blanket block would make TestWebRegister_RateLimited_PerIP "+
			"pass tautologically); got %d", code)
	}
}
