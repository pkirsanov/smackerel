// Spec 070 — router-level rate-limit coverage for POST /v1/web/login.
//
// Spec 070's Security Model states: "Form posts are rate-limited by the
// existing r.Use(httprate.LimitByIP(20, 1*time.Minute)) group in
// internal/api/router.go." The OAuth browser entry points have explicit
// router-level rate-limit regression tests (TestOAuthStart_RateLimited,
// TestOAuthCallback_RateLimited, TestSecR30_OAuthRateLimit_*), but the
// spec-070 credential entry point /v1/web/login had NONE:
//
//   - the credential/form unit tests (web_login_credential_test.go,
//     web_login_form_test.go) call deps.HandleWebLogin DIRECTLY, which
//     bypasses the router middleware chain entirely — the limiter never
//     runs in those tests; and
//   - the Scope-03 chaos test (tests/integration/auth_chaos_scope03_test.go)
//     deliberately gives every goroutine a DISTINCT RemoteAddr "so the
//     per-IP rate-limiter on /v1/web/login does not engage", so it proves
//     the limiter does NOT interfere, never that it engages.
//
// Net effect: a future router refactor that moved /v1/web/login OUT of
// the LimitByIP(20, ...) group (e.g. registering it directly on r, or
// folding it into a group with a different/absent budget) would weaken
// brute-force / credential-stuffing protection on the human login surface
// and ZERO existing test would fail. This file closes that gap by driving
// the REAL NewRouter(deps) and proving the 20/min/IP budget actually fires.
//
// Adversarial fidelity (verified out-of-band, RED→GREEN): temporarily
// re-registering /v1/web/login OUTSIDE the httprate group in router.go
// makes TestWebLogin_RateLimited_PerIP FAIL ("expected 429 ... never
// observed") because every request is then admitted; restoring the group
// turns it GREEN. The per-IP companion test rules out a tautological
// "everything 429s" pass.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newRateLimitRouterDeps wires Dependencies for the dev/test shared-token
// path with a real (in-memory) web-credential repo, so admitted requests
// exercise the spec-070 credential branch and return 401 (distinct from
// the 429 the limiter emits once the budget is spent).
func newRateLimitRouterDeps() *Dependencies {
	return &Dependencies{
		DB:             &mockDB{healthy: true},
		NATS:           &mockNATS{healthy: true},
		StartTime:      time.Now(),
		Environment:    "development",
		AuthToken:      "shared-token-for-ratelimit-test",
		WebCredentials: &fakeRepo{creds: map[string]string{"operator": "correct-horse-battery-staple"}},
		TrustedProxies: nil, // empty allowlist → RemoteAddr is the rate-limit key (no XFF spoof)
	}
}

// postLoginFromIP fires one form POST /v1/web/login through the full
// router middleware chain from the given TCP peer and returns the status.
// Credentials are intentionally wrong so an admitted request answers 401;
// only the rate limiter produces 429.
func postLoginFromIP(router http.Handler, remoteAddr string) int {
	const form = "username=operator&password=this-password-is-wrong"
	req := httptest.NewRequest(http.MethodPost, "/v1/web/login", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = remoteAddr
	req.ContentLength = int64(len(form))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code
}

// TestWebLogin_RateLimited_PerIP proves the 20/min/IP budget on
// /v1/web/login engages through the real router. The first ~20 POSTs
// from one IP pass the limiter (the handler answers 401 for the bad
// creds); a subsequent POST in the same window returns 429. Mirrors
// TestOAuthStart_RateLimited but for the spec-070 credential entry point.
//
// The firstFail >= 18 floor proves the limiter that fired is the
// documented 20-budget (not a tighter limiter and not the 10-budget
// OAuth group); the got429 requirement proves it fired AT ALL — the
// exact regression a "route fell out of the group" refactor introduces.
func TestWebLogin_RateLimited_PerIP(t *testing.T) {
	router := NewRouter(newRateLimitRouterDeps())

	const ip = "192.168.40.7:51000"
	const attempts = 30

	firstFail := -1
	statuses := make([]int, 0, attempts)
	for i := 0; i < attempts; i++ {
		code := postLoginFromIP(router, ip)
		statuses = append(statuses, code)
		if code == http.StatusTooManyRequests {
			firstFail = i
			break
		}
	}

	t.Logf("statuses (one IP, in order)=%v firstFail=%d", statuses, firstFail)

	if firstFail == -1 {
		t.Fatalf("expected 429 after exceeding the 20/min/IP budget on /v1/web/login, "+
			"but %d consecutive POSTs were all admitted (statuses=%v) — the login route "+
			"is no longer inside the httprate.LimitByIP(20, 1*time.Minute) group", attempts, statuses)
	}
	if firstFail < 18 {
		t.Errorf("429 fired at request index %d — too early for a 20/min budget "+
			"(expected ~20 requests admitted first); statuses=%v", firstFail, statuses)
	}
}

// TestWebLogin_RateLimit_PerIP_FreshIPAdmitted proves the limiter is keyed
// PER IP, not a blanket block. After IP-A is driven to 429, IP-B's first
// request in the same window MUST still be admitted (any non-429 status).
// This rules out a tautological pass where the assertion above would
// succeed simply because every request returns 429 regardless of budget.
func TestWebLogin_RateLimit_PerIP_FreshIPAdmitted(t *testing.T) {
	router := NewRouter(newRateLimitRouterDeps())

	// Drive IP-A until it is rate-limited.
	const ipA = "192.168.40.8:51000"
	exhausted := false
	for i := 0; i < 40; i++ {
		if postLoginFromIP(router, ipA) == http.StatusTooManyRequests {
			exhausted = true
			break
		}
	}
	if !exhausted {
		t.Fatalf("precondition failed: IP-A never hit 429 within 40 requests — " +
			"the /v1/web/login limiter is not engaging at all")
	}

	// IP-B's FIRST request in the same window must NOT be rate-limited.
	const ipB = "10.20.30.40:62000"
	if code := postLoginFromIP(router, ipB); code == http.StatusTooManyRequests {
		t.Errorf("a fresh IP (IP-B) was 429'd on its first request — the limiter "+
			"is not per-IP (a blanket block would make TestWebLogin_RateLimited_PerIP "+
			"pass tautologically); got %d", code)
	}
}
