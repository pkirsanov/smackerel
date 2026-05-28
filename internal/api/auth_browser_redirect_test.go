// Spec 057 Scope 2 — unit coverage for content-negotiated 303 vs 401
// behaviour in bearerAuthMiddleware (via isBrowserNavigation).
//
// These tests assemble a minimal Dependencies + the middleware so we
// can drive failure paths with crafted requests. They exercise the
// dev-empty-token branch where d.AuthToken != "" — when no token
// matches, the middleware should branch on isBrowserNavigation.
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

// nopHandler returns 200; we never expect to reach it on failure paths.
func nopHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// newBrowserRedirectDeps wires a dev/test Dependencies that requires
// a shared token. Requests without the token fall through to the
// final shared_token_mismatch branch.
func newBrowserRedirectDeps() *Dependencies {
	return &Dependencies{
		Environment: "development",
		AuthToken:   "expected-shared-token",
		AuthConfig:  config.AuthConfig{Enabled: false},
	}
}

func driveMiddleware(t *testing.T, deps *Dependencies, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	deps.bearerAuthMiddleware(nopHandler()).ServeHTTP(rec, req)
	return rec
}

// Test 2.1 — GET + Accept: text/html → 303 with sanitized next.
func TestBearerAuth_Browser_GET_TextHTML_Redirects(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent?q=1", nil)
	req.Header.Set("Accept", "text/html")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?next=") {
		t.Fatalf("unexpected Location=%q", loc)
	}
	// next must be URL-encoded sanitized value
	if !strings.Contains(loc, "next=%2Fapi%2Frecent%3Fq%3D1") {
		t.Errorf("next not encoded properly: %q", loc)
	}
}

// Test 2.2 — GET + Accept: */* → 401 (curl default).
func TestBearerAuth_GET_StarAccept_Returns401(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	req.Header.Set("Accept", "*/*")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Test 2.3 — GET + Accept: application/json → 401.
func TestBearerAuth_GET_JSON_Returns401(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	req.Header.Set("Accept", "application/json")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Test 2.4 — HEAD + Accept: text/html → 303 with empty body.
func TestBearerAuth_HEAD_TextHTML_Redirects(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodHead, "/api/recent", nil)
	req.Header.Set("Accept", "text/html")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Test 2.5 — POST + Accept: text/html → 401 (unsafe method, no redirect).
func TestBearerAuth_POST_TextHTML_Returns401(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodPost, "/api/capture", strings.NewReader(`{}`))
	req.Header.Set("Accept", "text/html")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Test 2.6 — GET + HX-Request: true + Accept: text/html → 401 (HTMX suppression).
func TestBearerAuth_HTMX_Returns401(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("HX-Request", "true")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Test 2.7 — GET + Sec-Fetch-Mode: cors + Accept: text/html → 401.
func TestBearerAuth_SecFetchModeCORS_Returns401(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Test 2.8 — GET + Sec-Fetch-Mode: navigate + Accept: text/html → 303.
func TestBearerAuth_SecFetchModeNavigate_Redirects(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// Defence-in-depth: missing-token branch (no Authorization, no cookie)
// also respects content negotiation when the browser asks for HTML.
func TestBearerAuth_MissingToken_Browser_Redirects(t *testing.T) {
	deps := newBrowserRedirectDeps()
	req := httptest.NewRequest(http.MethodGet, "/api/recent", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	// no Authorization header, no cookie → token == ""
	rec := driveMiddleware(t, deps, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
