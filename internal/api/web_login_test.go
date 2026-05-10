// Spec 044 Scope 03 — unit coverage for /v1/web/login + /v1/web/logout.
//
// The integration roundtrip (cookie jar + httptest.NewTLSServer +
// real PostgreSQL + real PASETO) lives in
// tests/e2e/auth/pwa_per_user_test.go. These unit tests exercise the
// handler logic in isolation without touching the DB so they run as
// part of `./smackerel.sh test unit` and catch regressions in the
// handler-level branch logic (production PASETO path, dev shared-token
// path, dev-bypass refusal path, body-validation failures).
package api

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
)

// newWebLoginDeps_DevShared returns Dependencies wired for the
// dev/test shared-token path (AuthConfig.Enabled = false).
func newWebLoginDeps_DevShared(t *testing.T, sharedToken string) *Dependencies {
	t.Helper()
	return &Dependencies{
		Environment: "development",
		AuthToken:   sharedToken,
	}
}

// newWebLoginDeps_Production returns Dependencies wired for the
// production per-user PASETO path along with a freshly minted token.
func newWebLoginDeps_Production(t *testing.T) (*Dependencies, string, string) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	const kid = "web-login-unit-key"
	const userID = "web-login-unit-user"
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    "tok-web-login-unit-001",
		SigningKey: priv,
		KeyID:      kid,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	return &Dependencies{
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                              true,
			SigningActivePrivateKey:              priv,
			SigningActiveKeyID:                   kid,
			TokenTTLHours:                        1,
			RotationGraceWindowHours:             1,
			ClockSkewToleranceSeconds:            60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                     priv + "-hash",
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),
	}, issued.WireToken, userID
}

func postWebLogin(t *testing.T, deps *Dependencies, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/web/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	deps.HandleWebLogin(rec, req)
	return rec
}

func TestWebLogin_Production_AcceptsValidPASETO(t *testing.T) {
	deps, wireToken, wantUserID := newWebLoginDeps_Production(t)
	body, _ := json.Marshal(map[string]string{"token": wireToken})
	rec := postWebLogin(t, deps, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp webLoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.UserID != wantUserID {
		t.Errorf("user_id=%q want %q (must come from PASETO sub claim, not request body)", resp.UserID, wantUserID)
	}
	if resp.ExpiresAt == "" {
		t.Errorf("expires_at empty — must be present for production PASETO path")
	}

	// Cookie attributes per design.md §10.4.
	cookies := rec.Result().Cookies()
	var auth *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			auth = c
			break
		}
	}
	if auth == nil {
		t.Fatalf("Set-Cookie auth_token missing; headers=%v", rec.Header())
	}
	if !auth.HttpOnly {
		t.Errorf("HttpOnly missing")
	}
	if !auth.Secure {
		t.Errorf("Secure missing in production")
	}
	if auth.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite=%v want Lax", auth.SameSite)
	}
	if auth.Path != "/" {
		t.Errorf("Path=%q want /", auth.Path)
	}
	if subtle.ConstantTimeCompare([]byte(auth.Value), []byte(wireToken)) != 1 {
		t.Errorf("cookie value does not match wire token")
	}
}

func TestWebLogin_Production_RejectsForeignPASETO(t *testing.T) {
	deps, _, _ := newWebLoginDeps_Production(t)
	otherPriv, _ := auth.GenerateSigningKeypair()
	foreign, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "mallory",
		TokenID:    "tok-foreign",
		SigningKey: otherPriv,
		KeyID:      "foreign-kid",
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue foreign: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"token": foreign.WireToken})
	rec := postWebLogin(t, deps, body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Errorf("body=%s want code=invalid_token", rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "auth_token" {
			t.Errorf("rejected login set auth_token cookie %v", c)
		}
	}
}

func TestWebLogin_Production_RejectsRevokedToken(t *testing.T) {
	deps, wireToken, _ := newWebLoginDeps_Production(t)
	parsed, err := auth.VerifyAndParse(wireToken, deps.AuthVerifyOptions)
	if err != nil {
		t.Fatalf("VerifyAndParse: %v", err)
	}
	deps.RevocationCache.MarkRevoked(parsed.TokenID)

	body, _ := json.Marshal(map[string]string{"token": wireToken})
	rec := postWebLogin(t, deps, body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "revoked_token") {
		t.Errorf("body=%s want code=revoked_token", rec.Body.String())
	}
}

func TestWebLogin_DevShared_AcceptsMatchingToken(t *testing.T) {
	const shared = "dev-shared-token-secret"
	deps := newWebLoginDeps_DevShared(t, shared)
	body, _ := json.Marshal(map[string]string{"token": shared})
	rec := postWebLogin(t, deps, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	var auth *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			auth = c
			break
		}
	}
	if auth == nil {
		t.Fatalf("dev login did not set cookie")
	}
	if auth.Secure {
		t.Errorf("Secure set in non-production environment (dev/test runs over HTTP)")
	}
	if !auth.HttpOnly {
		t.Errorf("HttpOnly missing in dev")
	}
	if auth.Value != shared {
		t.Errorf("dev cookie value mismatch")
	}
}

func TestWebLogin_DevShared_RejectsWrongToken(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "right-token")
	body, _ := json.Marshal(map[string]string{"token": "wrong-token"})
	rec := postWebLogin(t, deps, body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Errorf("body=%s want code=invalid_token", rec.Body.String())
	}
}

func TestWebLogin_DevBypass_RefusesLogin(t *testing.T) {
	// AuthToken empty AND AuthConfig.Enabled false = dev-bypass mode.
	// /v1/web/login has nothing to validate against; refuse with 400.
	deps := &Dependencies{Environment: "development", AuthToken: ""}
	body, _ := json.Marshal(map[string]string{"token": "anything"})
	rec := postWebLogin(t, deps, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported_no_auth_token") {
		t.Errorf("body=%s want code=unsupported_no_auth_token", rec.Body.String())
	}
}

func TestWebLogin_BodyValidation(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "x")

	cases := []struct {
		name     string
		body     string
		wantCode int
		wantErr  string
	}{
		{"empty body", `{}`, http.StatusBadRequest, "missing_token"},
		{"empty token field", `{"token":""}`, http.StatusBadRequest, "missing_token"},
		{"whitespace token", `{"token":"  "}`, http.StatusBadRequest, "missing_token"},
		{"unknown field", `{"token":"x","actor_id":"mallory"}`, http.StatusBadRequest, "invalid_json"},
		{"not json", `not json`, http.StatusBadRequest, "invalid_json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postWebLogin(t, deps, []byte(tc.body))
			if rec.Code != tc.wantCode {
				t.Fatalf("status=%d want %d body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantErr) {
				t.Errorf("body=%s want code=%s", rec.Body.String(), tc.wantErr)
			}
			for _, c := range rec.Result().Cookies() {
				if c.Name == "auth_token" {
					t.Errorf("rejected request set cookie %v", c)
				}
			}
		})
	}
}

func TestWebLogin_RejectsNonPOST(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "x")
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/v1/web/login", nil)
		rec := httptest.NewRecorder()
		deps.HandleWebLogin(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s status=%d want 405", method, rec.Code)
		}
	}
}

func TestWebLogout_ClearsCookie(t *testing.T) {
	for _, env := range []string{"production", "development"} {
		t.Run(env, func(t *testing.T) {
			deps := &Dependencies{Environment: env, AuthToken: "x"}
			req := httptest.NewRequest(http.MethodPost, "/v1/web/logout", nil)
			rec := httptest.NewRecorder()
			deps.HandleWebLogout(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("logout status=%d body=%s", rec.Code, rec.Body.String())
			}
			cookies := rec.Result().Cookies()
			var auth *http.Cookie
			for _, c := range cookies {
				if c.Name == "auth_token" {
					auth = c
					break
				}
			}
			if auth == nil {
				t.Fatalf("logout did not set Set-Cookie to clear auth_token")
			}
			if auth.Value != "" {
				t.Errorf("logout cookie value=%q want empty", auth.Value)
			}
			if auth.MaxAge >= 0 {
				t.Errorf("logout MaxAge=%d want negative (delete cookie)", auth.MaxAge)
			}
			if env == "production" && !auth.Secure {
				t.Errorf("logout production cookie missing Secure")
			}
		})
	}
}

// TestExtractBearerToken_CookieFallback proves the Scope 03
// extension to extractBearerToken: when no Authorization header is
// present, the auth_token cookie value is used as the bearer token.
// This is what makes the cookie-only PWA flow work for any route
// behind bearerAuthMiddleware.
func TestExtractBearerToken_CookieFallback(t *testing.T) {
	cases := []struct {
		name      string
		header    string
		cookieVal string
		want      string
	}{
		{"header preferred", "Bearer header-token", "cookie-token", "header-token"},
		{"header malformed wins (returns empty)", "garbage", "cookie-token", ""},
		{"no header → cookie used", "", "cookie-token", "cookie-token"},
		{"no header no cookie", "", "", ""},
		{"empty cookie value ignored", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/anything", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.cookieVal != "" {
				req.AddCookie(&http.Cookie{Name: "auth_token", Value: tc.cookieVal})
			}
			got := extractBearerToken(req)
			if got != tc.want {
				t.Errorf("got=%q want=%q", got, tc.want)
			}
		})
	}
}
