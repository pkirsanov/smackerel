// Spec 057 Scope 1 — unit coverage for HandleLoginPage.
package api

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/config"
)

func newLoginPageDeps_DevShared() *Dependencies {
	return &Dependencies{
		Environment: "development",
		AuthToken:   "dev-shared-token",
	}
}

func newLoginPageDeps_AuthDisabled() *Dependencies {
	return &Dependencies{
		Environment: "development",
		AuthToken:   "",
		AuthConfig:  config.AuthConfig{Enabled: false},
	}
}

func getLogin(t *testing.T, deps *Dependencies, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	deps.HandleLoginPage(rec, req)
	return rec
}

// Test 1.1 — form renders with action=/v1/web/login and hidden next field.
func TestLoginPage_RendersForm(t *testing.T) {
	deps := newLoginPageDeps_DevShared()
	rec := getLogin(t, deps, "/login?next=/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/v1/web/login"`) {
		t.Errorf("missing form action: %s", body)
	}
	if !strings.Contains(body, `name="next" value="/dashboard"`) {
		t.Errorf("missing hidden next field: %s", body)
	}
	if !strings.Contains(body, `name="token"`) {
		t.Errorf("missing token field: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type=%q want text/html", ct)
	}
}

// Spec 070 AC-6 — the rendered login form MUST expose username + password
// fields as the primary credential intake, with the legacy token field
// demoted into a collapsible <details> "machine client login" block.
// TestLoginPage_RendersForm only asserts name="token" + the form action,
// which the spec-057 token-only form already satisfied; it does NOT prove
// the spec-070 credential fields render. This test closes that gap.
func TestLoginPage_RendersCredentialFields(t *testing.T) {
	deps := newLoginPageDeps_DevShared() // AuthEnabled => credential branch
	rec := getLogin(t, deps, "/login?next=/dashboard")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="username"`) {
		t.Errorf("AC-6: missing username field in rendered form: %s", body)
	}
	if !strings.Contains(body, `name="password"`) {
		t.Errorf("AC-6: missing password field in rendered form: %s", body)
	}
	// The token field MUST survive as a machine-client fallback, demoted
	// inside the collapsible <details> block (not the primary inputs).
	if !strings.Contains(body, "<details") {
		t.Errorf("AC-6: missing collapsible <details> machine-login block: %s", body)
	}
	if !strings.Contains(body, `name="token"`) {
		t.Errorf("AC-6: token fallback field disappeared: %s", body)
	}
	// Ordering guard: username/password must appear above the token
	// fallback so the human credential path is primary (AC-6 "above the
	// existing token field").
	if idxUser, idxToken := strings.Index(body, `name="username"`), strings.Index(body, `name="token"`); idxUser == -1 || idxToken == -1 || idxUser > idxToken {
		t.Errorf("AC-6: username field (idx=%d) must render above token field (idx=%d)", idxUser, idxToken)
	}
}

// Test 1.2 — `?token=` query parameter ignored (must not leak into HTML).
func TestLoginPage_IgnoresTokenQueryParam(t *testing.T) {
	deps := newLoginPageDeps_DevShared()
	rec := getLogin(t, deps, "/login?token=SECRET_VALUE_123")
	body := rec.Body.String()
	if strings.Contains(body, "SECRET_VALUE_123") {
		t.Errorf("token query value leaked into HTML: %s", body)
	}
}

// Test 1.3 — Scenario 12: when no auth is configured, render disabled banner.
func TestLoginPage_AuthDisabled_RendersBanner(t *testing.T) {
	deps := newLoginPageDeps_AuthDisabled()
	rec := getLogin(t, deps, "/login")
	body := rec.Body.String()
	if !strings.Contains(body, "banner-disabled") {
		t.Errorf("missing disabled banner: %s", body)
	}
	if !strings.Contains(body, "disabled") {
		t.Errorf("missing disabled controls: %s", body)
	}
}

// Test 1.5 — FR-002: zero <script> blocks AND zero inline event handler attrs.
func TestLoginPage_CSPCompliant(t *testing.T) {
	deps := newLoginPageDeps_DevShared()
	rec := getLogin(t, deps, "/login")
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
}

// Test 1.1 (companion) — `next` is sanitised, hostile inputs default to "/".
func TestLoginPage_SanitisesNext(t *testing.T) {
	deps := newLoginPageDeps_DevShared()
	rec := getLogin(t, deps, "/login?next=//evil.example.com/")
	body := rec.Body.String()
	if strings.Contains(body, "evil.example.com") {
		t.Errorf("hostile next escaped sanitisation: %s", body)
	}
	if !strings.Contains(body, `name="next" value="/"`) {
		t.Errorf("expected fallback next=/, got: %s", body)
	}
}
