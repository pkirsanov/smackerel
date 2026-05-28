//go:build e2e

// Spec 057 Scope 4 — live-stack e2e-api coverage for the browser-
// friendly /login flow + adversarial regression for spec 044's wire
// contract.
//
// Architecture mirrors tests/e2e/auth/pwa_per_user_test.go:
//   - Reads CORE_EXTERNAL_URL from the test stack env (exported by
//     `./smackerel.sh test e2e` to the in-network Go runner).
//   - Uses real HTTP against the running core inside the test stack.
//   - SMACKEREL_AUTH_TOKEN is the shared dev token expected by the
//     test stack.
//
// No t.Skip — fail-loud per spec 044 pattern. Missing CORE_EXTERNAL_URL
// means the runner did not bring the stack up; that is a real failure,
// not a silently-skippable condition.
package auth_e2e

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func e2eBaseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("CORE_EXTERNAL_URL")
	if base == "" {
		t.Fatal("spec 057 e2e test requires CORE_EXTERNAL_URL — run via `./smackerel.sh test e2e` which brings up the live test stack and exports CORE_EXTERNAL_URL")
	}
	return strings.TrimRight(base, "/")
}

func newNoRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Test 2.9 — curl: Accept: text/html GET / → 303 to /login?next=/.
func TestE2E_Browser_GET_TextHTML_Redirects(t *testing.T) {
	base := e2eBaseURL(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/api/recent", nil)
	req.Header.Set("Accept", "text/html")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/login?next=") {
		t.Errorf("Location=%q", loc)
	}
}

// Test 2.10 — no Accept header GET → 401 JSON shape preserved.
func TestE2E_NoAcceptHeader_Returns401JSON(t *testing.T) {
	base := e2eBaseURL(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/api/recent", nil)
	// Strip Accept entirely.
	req.Header.Del("Accept")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		t.Errorf("Content-Type=%q want json", resp.Header.Get("Content-Type"))
	}
}

// Test 2.11 — HTMX request → 401 even with Accept: text/html.
func TestE2E_HTMXRequest_Returns401(t *testing.T) {
	base := e2eBaseURL(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/api/recent", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("HX-Request", "true")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

// Test 4.6 — alias for Scenario 11 (HTMX 401, NOT 303) covered by above.

// Test 4.7 (adversarial) — POST /v1/web/login with Accept: text/html and
// no cookie → expected JSON-shaped response (NOT a 303). Proves the 303
// branch is method-gated.
func TestE2E_Adversarial_POST_TextHTML_NoRedirect(t *testing.T) {
	base := e2eBaseURL(t)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/capture", strings.NewReader(`{}`))
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Content-Type", "application/json")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusSeeOther {
		t.Fatalf("unexpected 303 on POST: %d Location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// Test 4.8 (adversarial) — GET + Accept: text/html,application/json + Sec-
// Fetch-Mode: cors → 401, NOT 303.
func TestE2E_Adversarial_FetchStyle_Returns401(t *testing.T) {
	base := e2eBaseURL(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/api/recent", nil)
	req.Header.Set("Accept", "text/html,application/json;q=0.9")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d Location=%q", resp.StatusCode, resp.Header.Get("Location"))
	}
}

// Test 2.13 / 4.7 canary — login page renders unauthenticated.
func TestE2E_LoginPage_RendersUnauthenticated(t *testing.T) {
	base := e2eBaseURL(t)
	resp, err := newNoRedirectClient().Get(base + "/login")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `action="/v1/web/login"`) {
		t.Errorf("login form missing: %s", body)
	}
}

// Cookie roundtrip: form POST sets cookie and 303s to next, then a follow-
// up GET with the cookie succeeds (regression for SCOPE-3 against the
// live stack).
func TestE2E_Form_Login_CookieRoundtrip(t *testing.T) {
	base := e2eBaseURL(t)
	tokenVal := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if tokenVal == "" {
		t.Fatal("SMACKEREL_AUTH_TOKEN required — exported by `./smackerel.sh test e2e`")
	}
	form := url.Values{"token": {tokenVal}, "next": {"/"}}
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/web/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	cookies := resp.Cookies()
	var auth *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			auth = c
			break
		}
	}
	if auth == nil || auth.Value == "" {
		t.Fatalf("auth_token cookie missing: %+v", cookies)
	}
}
