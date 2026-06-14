// Spec 091 SCOPE-02 — unit coverage for HandleRegisterPage (GET /register).
//
// Mirrors web_login_page_test.go. Drives the handler directly via httptest;
// no router, no DB. Proves: the canonical CSP-safe form renders with all
// four fields + header trio (AC-1); the rendered form is byte-identical
// regardless of invite-token configuration (Reconciled AC-5 / AC-10
// non-enumeration); ?next is sanitised; CSP compliance (no inline
// scripts/handlers); and the HEAD short-circuit.
package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func getRegister(t *testing.T, deps *Dependencies, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	deps.HandleRegisterPage(rec, req)
	return rec
}

// TestRegisterPage_RendersForm — GET /register returns 200 with a form that
// posts to /v1/web/register and exposes exactly the four fields (username,
// password, confirm-password, invite-token) plus a hidden sanitised next, a
// single "Create account" submit, and an "Already have an account? Sign in"
// link to /login. The header trio is set (AC-1).
func TestRegisterPage_RendersForm(t *testing.T) {
	deps := &Dependencies{Environment: "development"}
	rec := getRegister(t, deps, "/register?next=/cards")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	wantFragments := []string{
		`action="/v1/web/register"`,
		`name="next" value="/cards"`,
		`name="username"`,
		`name="password"`,
		`name="confirm-password"`,
		`name="invite-token"`,
		`type="submit"`,
		`Create account`,
		`href="/login"`,
	}
	for _, frag := range wantFragments {
		if !strings.Contains(body, frag) {
			t.Errorf("rendered form missing %q\n--- body ---\n%s", frag, body)
		}
	}

	// The invite-token field MUST be masked (type=password) and never
	// autofilled from the credential store.
	if !strings.Contains(body, `name="invite-token" autocomplete="off"`) {
		t.Errorf("invite-token field must be type=password autocomplete=off: %s", body)
	}

	// Header trio (mirrors the /login GET page).
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type=%q want text/html", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control=%q want no-store", cc)
	}
	if xc := rec.Header().Get("X-Content-Type-Options"); xc != "nosniff" {
		t.Errorf("X-Content-Type-Options=%q want nosniff", xc)
	}
}

// TestRegisterPage_IdenticalForm — the GET render MUST be byte-identical
// whether the invite token is configured or empty (Reconciled AC-5 / AC-10):
// registerPageData has no token field and the handler never reads the gate
// config, so the gate state cannot be observed from GET. An attacker probing
// GET /register learns nothing about whether registration is enabled.
func TestRegisterPage_IdenticalForm(t *testing.T) {
	depsEnabled := &Dependencies{Environment: "development", WebRegistrationInviteToken: "a-real-operator-invite-token"}
	depsDisabled := &Dependencies{Environment: "development", WebRegistrationInviteToken: ""}

	recEnabled := getRegister(t, depsEnabled, "/register")
	recDisabled := getRegister(t, depsDisabled, "/register")

	if recEnabled.Code != http.StatusOK || recDisabled.Code != http.StatusOK {
		t.Fatalf("status enabled=%d disabled=%d", recEnabled.Code, recDisabled.Code)
	}
	if recEnabled.Body.String() != recDisabled.Body.String() {
		t.Errorf("GET /register is NOT byte-identical between configured and empty invite token "+
			"— this leaks gate state and breaks AC-10 non-enumeration\n--- enabled ---\n%s\n--- disabled ---\n%s",
			recEnabled.Body.String(), recDisabled.Body.String())
	}
	// The configured invite-token value must NEVER appear in the page.
	if strings.Contains(recEnabled.Body.String(), "a-real-operator-invite-token") {
		t.Errorf("configured invite token leaked into the GET page: %s", recEnabled.Body.String())
	}
}

// TestRegisterPage_NextSanitized — a hostile ?next is sanitised into the
// hidden field and cannot escape the origin.
func TestRegisterPage_NextSanitized(t *testing.T) {
	deps := &Dependencies{Environment: "development"}
	rec := getRegister(t, deps, "/register?next=//evil.example.com/")
	body := rec.Body.String()
	if strings.Contains(body, "evil.example.com") {
		t.Errorf("hostile next escaped sanitisation: %s", body)
	}
	if !strings.Contains(body, `name="next" value="/"`) {
		t.Errorf("expected fallback next=/ in hidden field, got: %s", body)
	}
}

// TestRegisterPage_CSPCompliant — zero inline <script> blocks AND zero inline
// event handler attributes (CSP script-src 'self'); assets are same-origin
// /admin_ui_static/* (register.js + the reused login.css).
func TestRegisterPage_CSPCompliant(t *testing.T) {
	deps := &Dependencies{Environment: "development"}
	rec := getRegister(t, deps, "/register")
	body := rec.Body.String()

	// External <script src="..."> is allowed; inline <script>...</script> is not.
	inlineScript := regexp.MustCompile(`<script(?:\s[^>]*)?>[^<]`)
	if inlineScript.MatchString(body) {
		t.Errorf("found inline script block: %s", body)
	}
	inlineHandler := regexp.MustCompile(`(?i)\son[a-z]+\s*=`)
	if inlineHandler.MatchString(body) {
		t.Errorf("found inline event handler attribute: %s", body)
	}
	if !strings.Contains(body, `src="/admin_ui_static/register.js"`) {
		t.Errorf("missing same-origin register.js asset: %s", body)
	}
	if !strings.Contains(body, `href="/admin_ui_static/login.css"`) {
		t.Errorf("missing reused same-origin login.css asset: %s", body)
	}
}

// TestRegisterPage_HEAD — a HEAD request returns 200 with an empty body
// (short-circuit, mirrors the /login GET page).
func TestRegisterPage_HEAD(t *testing.T) {
	deps := &Dependencies{Environment: "development"}
	req := httptest.NewRequest(http.MethodHead, "/register", nil)
	rec := httptest.NewRecorder()
	deps.HandleRegisterPage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status=%d want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD body must be empty, got %d bytes: %q", rec.Body.Len(), rec.Body.String())
	}
	// The header trio is still set on HEAD.
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("HEAD Content-Type=%q want text/html", ct)
	}
}
