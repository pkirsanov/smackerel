//go:build integration

// Spec 044 Scope 03 — Browser-extension per-user PASETO integration tests (T3-02 closure).
//
// The browser extension already attaches `Authorization: Bearer <token>`
// to every internal call. Historically the token slot held a shared
// bearer (SMACKEREL_AUTH_TOKEN). Spec 044 makes per-user PASETO the
// production identity carrier; this test suite proves the extension
// flow works end-to-end against a production-mode router:
//
//  1. Mint a per-user PASETO via the same primitives the operator CLI
//     uses (auth.IssueToken).
//  2. Hit a representative bearer-auth-protected endpoint
//     (GET /v1/photos/connectors) that the extension consults during
//     setup; assert 200.
//  3. Adversarial — malformed bearer → 401, no token leak in body.
//  4. Adversarial — revoked PASETO → 401 (RevocationCache hit).
//
// SCN-AUTH-002 (extension surface) closure evidence.
//
// Pattern mirrors auth_mintreveal_test.go's productionAuthDepsForReveal
// for consistency; the helper here is named productionExtensionDeps and
// does NOT seed photos rows because the connectors endpoint tolerates
// an empty store (it falls back to the immich/photoprism placeholders).
package integration

import (
	"context"
	"encoding/json"
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

// productionExtensionDeps builds an api.Dependencies suitable for
// validating the extension flow against the live test stack with the
// per-user PASETO middleware path active. Returns the deps plus the
// signing private key + key id so the test body can mint tokens.
func productionExtensionDeps(t *testing.T) (deps *api.Dependencies, signingKey, keyID string, cache *revocation.Cache) {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope03-extension-key"

	store := photolib.NewStore(pool)
	cache = revocation.NewCache()

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
		RevocationCache: cache,
		PhotosHandlers:  api.NewPhotosHandlers(store, config.PhotosConfig{}, "production"),
	}
	return deps, priv, kid, cache
}

// TestExtensionAuth_PerUserPASETO_AdmitsAndAttachesSession proves the
// happy path: a fresh per-user PASETO bearer minted by the same primitive
// the operator CLI uses passes the production bearer middleware and
// the protected GET returns 200.
func TestExtensionAuth_PerUserPASETO_AdmitsAndAttachesSession(t *testing.T) {
	deps, signingKey, keyID, _ := productionExtensionDeps(t)

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "ext-user-001",
		TokenID:    "tok-ext-happy-001",
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

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet,
		srv.URL+"/v1/photos/connectors", nil)
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

	// Confirm the response is a sane JSON envelope — proves the handler
	// (not the auth middleware) executed.
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("response is not JSON: %v body=%s", err, string(body))
	}
	if _, ok := payload["connectors"]; !ok {
		t.Errorf("payload missing 'connectors' field; body=%s", string(body))
	}

	// The wire token MUST NOT appear in the response body — defense
	// against accidental echo.
	if strings.Contains(string(body), issued.WireToken) {
		t.Errorf("wire token leaked in response body — extension flow MUST NOT echo bearer material")
	}
}

// TestExtensionAuth_MalformedBearer_Production_Returns401 covers the
// spec 044 NFR-AUTH-007 / SCN-AUTH-010 contract: invalid bearers
// receive a generic 401 with no token material in the response.
// Adversarial — proves the middleware does not silently downgrade to
// a permissive branch.
func TestExtensionAuth_MalformedBearer_Production_Returns401(t *testing.T) {
	deps, _, _, _ := productionExtensionDeps(t)
	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	cases := []struct {
		name   string
		header string
	}{
		{"empty bearer", "Bearer "},
		{"garbage bearer", "Bearer not-a-paseto-token-at-all"},
		{"wrong scheme", "Basic dXNlcjpwYXNz"},
		{"missing space", "Bearernospace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				context.Background(), http.MethodGet,
				srv.URL+"/v1/photos/connectors", nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401; body=%s", resp.StatusCode, string(body))
			}
			// The 401 body MUST NOT name the failing validation step
			// (NFR-AUTH-007) and MUST NOT echo any header content.
			if strings.Contains(string(body), tc.header) && tc.header != "" {
				t.Errorf("401 body echoed Authorization header verbatim — body=%s", string(body))
			}
		})
	}
}

// TestExtensionAuth_RevokedPerUserToken_Returns401 mints a real token,
// puts it in the revocation cache, then expects 401. Adversarial —
// proves the cache check actually fires during the bearer middleware's
// PASETO branch.
func TestExtensionAuth_RevokedPerUserToken_Returns401(t *testing.T) {
	deps, signingKey, keyID, cache := productionExtensionDeps(t)

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "ext-user-002",
		TokenID:    "tok-ext-revoked-001",
		SigningKey: signingKey,
		KeyID:      keyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	// Mark revoked BEFORE the request fires.
	cache.MarkRevoked("tok-ext-revoked-001")

	srv := httptest.NewServer(api.NewRouter(deps))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodGet,
		srv.URL+"/v1/photos/connectors", nil)
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

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token MUST 401; got=%d body=%s", resp.StatusCode, string(body))
	}
	// Belt-and-brace: the wire token MUST NOT appear in the 401 body.
	if strings.Contains(string(body), issued.WireToken) {
		t.Errorf("revoked-token 401 body leaked the bearer; body=%s", string(body))
	}
}
