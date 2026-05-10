//go:build integration

// Spec 044 Scope 03 — Admin token-management UI integration test (T3-04 closure).
//
// Validates that:
//
//  1. GET /admin/auth/tokens with a valid per-user PASETO bearer
//     returns 200 + Content-Type starting with "text/html" + a body
//     containing the expected page title/markers.
//  2. The page advertises the four admin REST endpoints it actually
//     calls (GET /v1/auth/users, POST /v1/auth/users, rotate, revoke).
//     Pinning these strings means a future contract drift in the page
//     (or accidental deletion of a button) is caught here.
//  3. GET /admin/auth/tokens without a bearer in production is
//     rejected with HTTP 401 — adversarial coverage for the route
//     registration (proves it is actually behind bearerAuthMiddleware
//     and not accidentally registered outside the group).
//  4. Disallowed methods (POST/PUT/DELETE) return 405 — the page is
//     read-only.
//
// SCN-AUTH-001 (admin UI surface) closure evidence.
package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// productionAdminUIDeps wires a production-mode router with a
// PhotosHandlers instance (so /v1 routes register and the /v1 group
// guard for AuthAdminHandlers != nil isn't strictly required) and the
// admin UI handler (which is unconditional). Returns the deps + the
// signing material so the test body can mint bearers.
func productionAdminUIDeps(t *testing.T) (deps *api.Dependencies, signingKey, keyID string) {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope03-adminui-key"

	deps = &api.Dependencies{
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                              true,
			TokenFormat:                          "paseto_v4_public",
			SigningActivePrivateKey:              priv,
			SigningActiveKeyID:                   kid,
			TokenTTLHours:                        24,
			RotationGraceWindowHours:             24,
			ClockSkewToleranceSeconds:            60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                     priv + "-hash-suffix-distinct",
			ProductionSharedTokenFallbackEnabled: false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),
		PhotosHandlers:  api.NewPhotosHandlers(photolib.NewStore(pool), config.PhotosConfig{}, "production"),
	}
	return deps, priv, kid
}

// TestAdminUI_WithBearer_Returns200HTML proves the happy path.
func TestAdminUI_WithBearer_Returns200HTML(t *testing.T) {
	deps, signingKey, keyID := productionAdminUIDeps(t)

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "admin-ui-test-user",
		TokenID:    "tok-adminui-001",
		SigningKey: signingKey,
		KeyID:      keyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/admin/auth/tokens", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200; body=%s", resp.StatusCode, string(body))
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type=%q want text/html...", ct)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control=%q want no-store", resp.Header.Get("Cache-Control"))
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("X-Content-Type-Options=%q want nosniff", resp.Header.Get("X-Content-Type-Options"))
	}
	if csp := resp.Header.Get("Content-Security-Policy"); csp == "" {
		t.Errorf("Content-Security-Policy header is missing — page MUST ship a CSP")
	}

	// Pin the expected REST endpoints the page calls. A regression
	// that drops one of these surfaces in JS would silently break the
	// admin workflow without this assertion.
	for _, marker := range []string{
		"Smackerel — Per-User Bearer Tokens",
		"/v1/auth/users",
		"/v1/auth/users/'",                 // rotate path is built via string concat
		"/v1/auth/tokens/'",                // revoke path is built via string concat
		"Mint a New User",
		"Enrolled Users",
		"Revoke a Specific Token",
	} {
		if !strings.Contains(string(body), marker) {
			t.Errorf("admin UI body missing marker %q", marker)
		}
	}

	// The wire token MUST NOT appear in the response.
	if strings.Contains(string(body), issued.WireToken) {
		t.Errorf("admin UI body leaked the bearer token")
	}
}

// TestAdminUI_WithoutBearer_Production_Returns401 confirms the route
// is actually behind bearerAuthMiddleware.
func TestAdminUI_WithoutBearer_Production_Returns401(t *testing.T) {
	deps, _, _ := productionAdminUIDeps(t)

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/admin/auth/tokens", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 (adversarial: missing bearer); body=%s", resp.StatusCode, string(body))
	}
}

// TestAdminUI_DisallowedMethods_Return405 proves the route does not
// silently accept POST/PUT/DELETE — defense against accidental
// expansion if future code is added that mounts the same handler
// under a chi method-flexible pattern.
func TestAdminUI_DisallowedMethods_Return405(t *testing.T) {
	deps, signingKey, keyID := productionAdminUIDeps(t)

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "admin-ui-test-user",
		TokenID:    "tok-adminui-method-001",
		SigningKey: signingKey,
		KeyID:      keyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(),
				method, srv.URL+"/admin/auth/tokens", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+issued.WireToken)
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			if resp.StatusCode != http.StatusMethodNotAllowed {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("method=%s status=%d want 405 (page is read-only); body=%s",
					method, resp.StatusCode, string(body))
			}
		})
	}
}
