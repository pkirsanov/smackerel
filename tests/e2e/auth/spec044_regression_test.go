//go:build e2e

// Spec 057 Scope 4 — adversarial regression sweep for spec 044's wire
// contract. These tests prove that the spec 057 content-negotiated 303
// branch in bearerAuthMiddleware does NOT bleed into spec 044's
// JSON wire contract.
package auth_e2e

import (
	"net/http"
	"strings"
	"testing"
)

// 2.12 — every failure path still returns 401 JSON when Accept lacks text/html.
func TestE2E_Spec044_Regression_NoTextHTMLAccept(t *testing.T) {
	base := e2eBaseURL(t)
	cases := []struct{ name, target, accept string }{
		{"no_accept_recent", "/api/recent", ""},
		{"star_accept_recent", "/api/recent", "*/*"},
		{"json_accept_recent", "/api/recent", "application/json"},
		{"no_accept_health", "/api/health", ""}, // health is unauth but never a 303
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, base+tc.target, nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			} else {
				req.Header.Del("Accept")
			}
			resp, err := newNoRedirectClient().Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusSeeOther {
				t.Errorf("unexpected 303 for %s: Location=%q", tc.target, resp.Header.Get("Location"))
			}
			ct := resp.Header.Get("Content-Type")
			if resp.StatusCode == http.StatusUnauthorized && !strings.Contains(ct, "json") {
				t.Errorf("401 without JSON Content-Type: %q", ct)
			}
		})
	}
}

// 3.9 — JSON POST to /v1/web/login still returns JSON body (no 303).
func TestE2E_Spec044_Regression_JSONLogin_Unchanged(t *testing.T) {
	base := e2eBaseURL(t)
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/web/login", strings.NewReader(`{"token":"definitely-wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := newNoRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Location") != "" {
		t.Errorf("unexpected Location on JSON login: %q", resp.Header.Get("Location"))
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "json") {
		t.Errorf("Content-Type=%q want json", resp.Header.Get("Content-Type"))
	}
}
