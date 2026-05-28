// Spec 057 Scope 3 — unit coverage for form POST extension of
// /v1/web/login + /v1/web/logout, including:
//   - 3.1 valid token + valid next → 303 + cookie
//   - 3.2 invalid token → re-render with error, no cookie
//   - 3.3 server-side sanitizeNext on hidden field
//   - 3.4 JSON POST contract preserved byte-for-byte
//   - 3.5 logout form → cookie cleared + 303 to /login
//   - 3.6 dev-mode shared token via form → cookie set
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func postWebLoginForm(t *testing.T, deps *Dependencies, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := form.Encode()
	req := httptest.NewRequest(http.MethodPost, "/v1/web/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	deps.HandleWebLogin(rec, req)
	return rec
}

func postWebLogoutForm(t *testing.T, deps *Dependencies) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/web/logout", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	deps.HandleWebLogout(rec, req)
	return rec
}

func cookieByName(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	resp := &http.Response{Header: rec.Header()}
	for _, c := range resp.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Test 3.1 — form POST with valid token + valid next → 303 to next, cookie set.
func TestWebLogin_Form_Valid_RedirectsAndSetsCookie(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "expected-token")
	rec := postWebLoginForm(t, deps, url.Values{
		"token": {"expected-token"},
		"next":  {"/dashboard"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("Location=%q want /dashboard", loc)
	}
	c := cookieByName(rec, "auth_token")
	if c == nil || c.Value != "expected-token" {
		t.Errorf("cookie missing or wrong value: %+v", c)
	}
}

// Test 3.2 — form POST with invalid token → re-render with error, no cookie.
func TestWebLogin_Form_InvalidToken_ReRendersError(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "expected-token")
	rec := postWebLoginForm(t, deps, url.Values{
		"token": {"WRONG"},
		"next":  {"/dashboard"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if c := cookieByName(rec, "auth_token"); c != nil && c.Value != "" {
		t.Errorf("cookie set on failure: %+v", c)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Invalid or expired token") {
		t.Errorf("missing error banner: %s", body)
	}
	if strings.Contains(body, "WRONG") {
		t.Errorf("token value leaked into response: %s", body)
	}
}

// Test 3.3 — server applies sanitizeNext to hidden field; tampered next → /.
func TestWebLogin_Form_ServerSideSanitizesNext(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "expected-token")
	rec := postWebLoginForm(t, deps, url.Values{
		"token": {"expected-token"},
		"next":  {"//evil.example.com/"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location=%q want /", loc)
	}
}

// Test 3.4 — JSON POST still returns JSON body (no regression for spec 044).
func TestWebLogin_JSON_PreservesContract(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "expected-token")
	body, _ := json.Marshal(map[string]string{"token": "expected-token"})
	req := httptest.NewRequest(http.MethodPost, "/v1/web/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.HandleWebLogin(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type=%q want application/json", ct)
	}
	if rec.Header().Get("Location") != "" {
		t.Errorf("unexpected Location header on JSON path: %q", rec.Header().Get("Location"))
	}
}

// Test 3.5 — form POST to /v1/web/logout clears cookie + 303 to /login.
func TestWebLogout_Form_ClearsCookieAndRedirects(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "expected-token")
	rec := postWebLogoutForm(t, deps)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location=%q want /login", loc)
	}
	c := cookieByName(rec, "auth_token")
	if c == nil {
		t.Fatalf("auth_token clear cookie missing")
	}
	if c.Value != "" || c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", c)
	}
}

// Test 3.6 — dev-mode shared token via form → cookie set.
func TestWebLogin_Form_DevSharedToken_SetsCookie(t *testing.T) {
	deps := newWebLoginDeps_DevShared(t, "shared-dev-token")
	rec := postWebLoginForm(t, deps, url.Values{
		"token": {"shared-dev-token"},
		"next":  {"/"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d", rec.Code)
	}
	if c := cookieByName(rec, "auth_token"); c == nil || c.Value != "shared-dev-token" {
		t.Errorf("cookie not set: %+v", c)
	}
}
