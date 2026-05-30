// Spec 063 — credential-path coverage for /v1/web/login. Verifies:
//   - username + password (matching) → 303 + cookie set to shared AuthToken
//   - username + wrong password → 200 re-render with error, no cookie
//   - unknown username → same generic error, no cookie
//   - missing password → error
//   - missing username → error
//   - token-only POST (regression) → existing path unchanged
//   - WebCredentials nil + creds posted → error (deployment not enabled)
package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/smackerel/smackerel/internal/auth/webcreds"
)

// fakeRepo implements webcreds.Repo against an in-memory map of
// username → password (plaintext is fine in tests; production path
// uses argon2id PHC strings via the real repo).
type fakeRepo struct {
	mu    sync.Mutex
	creds map[string]string // username → plaintext password
}

func (r *fakeRepo) VerifyAndTouch(_ context.Context, username, password string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	want, ok := r.creds[username]
	if !ok || want != password {
		return webcreds.ErrInvalidCredentials
	}
	return nil
}

func (r *fakeRepo) UpsertPassword(_ context.Context, username, password string, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creds[username] = password
	return nil
}

func (r *fakeRepo) List(_ context.Context) ([]webcreds.UserRow, error) { return nil, nil }
func (r *fakeRepo) Exists(_ context.Context, _ string) (bool, error)   { return false, nil }

func newCredDeps(t *testing.T, sharedToken string, users map[string]string) *Dependencies {
	t.Helper()
	deps := newWebLoginDeps_DevShared(t, sharedToken)
	deps.WebCredentials = &fakeRepo{creds: users}
	return deps
}

func TestWebLogin_Credential_ValidMatch_RedirectsAndSetsCookie(t *testing.T) {
	deps := newCredDeps(t, "shared-token-123", map[string]string{
		"testuser": "super-secret-password",
	})
	rec := postWebLoginForm(t, deps, url.Values{
		"username": {"testuser"},
		"password": {"super-secret-password"},
		"next":     {"/dashboard"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/dashboard" {
		t.Errorf("Location=%q want /dashboard", loc)
	}
	c := cookieByName(rec, "auth_token")
	if c == nil {
		t.Fatal("auth_token cookie missing")
	}
	if c.Value != "shared-token-123" {
		t.Errorf("cookie value=%q want shared-token-123 (credential path MUST reuse shared AuthToken)", c.Value)
	}
}

func TestWebLogin_Credential_WrongPassword_NoCookie(t *testing.T) {
	deps := newCredDeps(t, "shared-token-123", map[string]string{
		"testuser": "super-secret-password",
	})
	rec := postWebLoginForm(t, deps, url.Values{
		"username": {"testuser"},
		"password": {"wrong"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401 (re-render with error)", rec.Code)
	}
	if cookieByName(rec, "auth_token") != nil {
		t.Error("auth_token cookie MUST NOT be set on bad password")
	}
	if !strings.Contains(rec.Body.String(), "Invalid username or password") {
		t.Errorf("body missing generic error: %q", rec.Body.String())
	}
}

func TestWebLogin_Credential_UnknownUser_NoCookie_SameError(t *testing.T) {
	deps := newCredDeps(t, "shared-token-123", map[string]string{
		"testuser": "super-secret-password",
	})
	rec := postWebLoginForm(t, deps, url.Values{
		"username": {"ghost"},
		"password": {"anything"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rec.Code)
	}
	if cookieByName(rec, "auth_token") != nil {
		t.Error("auth_token cookie MUST NOT be set for unknown user")
	}
	if !strings.Contains(rec.Body.String(), "Invalid username or password") {
		t.Errorf("body missing generic error (MUST be identical to wrong-pw to avoid user-enum leak): %q", rec.Body.String())
	}
}

func TestWebLogin_Credential_MissingPassword(t *testing.T) {
	deps := newCredDeps(t, "shared-token-123", map[string]string{"testuser": "p"})
	rec := postWebLoginForm(t, deps, url.Values{
		"username": {"testuser"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Errorf("body missing required-field error: %q", rec.Body.String())
	}
}

func TestWebLogin_Credential_MissingUsername(t *testing.T) {
	deps := newCredDeps(t, "shared-token-123", map[string]string{"testuser": "p"})
	rec := postWebLoginForm(t, deps, url.Values{
		"password": {"anything"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "required") {
		t.Errorf("body missing required-field error: %q", rec.Body.String())
	}
}

func TestWebLogin_TokenOnly_RegressionUnchanged(t *testing.T) {
	// Adversarial: token-form path MUST still work when no user/pass
	// fields are present. WebCredentials is wired (live deployment),
	// but the credential branch MUST NOT fire when both fields are
	// absent.
	deps := newCredDeps(t, "expected-token", map[string]string{"testuser": "p"})
	rec := postWebLoginForm(t, deps, url.Values{
		"token": {"expected-token"},
		"next":  {"/dashboard"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	c := cookieByName(rec, "auth_token")
	if c == nil || c.Value != "expected-token" {
		t.Errorf("token-form regression: cookie=%+v want value=expected-token", c)
	}
}

func TestWebLogin_Credential_NilRepo_RejectedWithError(t *testing.T) {
	// Deployment without WebCredentials repo (e.g. config-only run)
	// MUST reject credential posts with a clear error, NOT silently
	// fall through to the token path.
	deps := newWebLoginDeps_DevShared(t, "shared-token-123")
	deps.WebCredentials = nil
	rec := postWebLoginForm(t, deps, url.Values{
		"username": {"testuser"},
		"password": {"anything"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d want 401", rec.Code)
	}
	if cookieByName(rec, "auth_token") != nil {
		t.Error("auth_token cookie MUST NOT be set when WebCredentials is nil")
	}
	if !strings.Contains(rec.Body.String(), "not enabled") {
		t.Errorf("body missing 'not enabled' error: %q", rec.Body.String())
	}
}
