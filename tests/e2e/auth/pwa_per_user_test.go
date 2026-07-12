//go:build e2e

// Spec 044 Scope 03 — PWA per-user session foundation.
//
// Discharges FINALIZE-PREREQ-044-V7-001 and SCN-AUTH-002 [PWA path]:
// the PWA browser-side flow MUST be able to convert a per-user PASETO
// into an HttpOnly cookie that subsequent same-origin requests carry
// automatically, AND production-mode requests without that cookie
// MUST be rejected with 401.
//
// Test architecture (live, not mocked):
//
//  1. Open a real pgxpool against the live test stack DATABASE_URL
//     (test-stack PostgreSQL on 127.0.0.1:47001 from
//     config/generated/test.env). Apply migrations.
//  2. Reset the spec 044 auth tables so each test starts clean.
//  3. Enroll a user via the real auth.BearerStore.Enroll.
//  4. Mint a real PASETO v4.public token via auth.IssueToken.
//  5. Build api.Dependencies with Environment="production" and
//     AuthConfig.Enabled=true so bearerAuthMiddleware runs Branch 1
//     (per-user PASETO + RevocationCache) — the same code path the
//     self-hosted production deployment runs.
//  6. Spin up the real Chi router under httptest.NewTLSServer so
//     Secure cookies survive the round-trip in the cookie jar.
//  7. POST /v1/web/login with the PASETO body, capture Set-Cookie,
//     assert the auth_token cookie attributes match design.md §10.4
//     (HttpOnly + Secure + SameSite=Lax + Path=/).
//  8. GET /v1/photos/connectors carrying the cookie via cookiejar;
//     assert 200 and that the bearerAuthMiddleware accepted the
//     cookie-sourced bearer token.
//  9. Adversarial — bare GET (no cookie) MUST return 401.
//  10. Adversarial — POST /v1/web/login without a token body MUST
//     return 400 missing_token; with an invalid PASETO MUST return
//     401 invalid_token.
//
// No t.Skip — when DATABASE_URL is unset the test fails with a clear
// message because spec 043 set the no-skip precedent for live-stack
// tests, and spec 044 Scope 02 carried the same rule forward.
//
// SST: DATABASE_URL is sourced from config/generated/test.env via
// `./smackerel.sh test e2e` (or a manual `set -a; source ...; set +a`
// before `go test -tags e2e`). The test does NOT hardcode any host,
// port, or credential.
package auth_e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/db"
)

// authTestPool opens a pgx pool against the live test stack
// DATABASE_URL and applies migrations. Fail-loud on missing env so
// the e2e tests never silently turn into no-ops (spec 043 / spec 044
// no-skip precedent).
func authTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("PWA per-user e2e test requires DATABASE_URL — run via `./smackerel.sh test e2e` which brings up the live test stack and exports DATABASE_URL")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping DATABASE_URL: %v", err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// resetAuthTables clears every spec-044 auth table so each test starts
// from a clean slate. The live test stack is shared but disposable;
// rows do not leak across runs.
func resetAuthTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, sql := range []string{
		`DELETE FROM auth_revocations`,
		`DELETE FROM auth_tokens`,
		`DELETE FROM auth_users`,
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("reset %q: %v", sql, err)
		}
	}
}

// productionPWADeps builds an *api.Dependencies wired for the
// production-mode PWA bearer-auth path. Returns the dependencies, the
// freshly-minted PASETO wire token, and the user_id the token is
// bound to.
func productionPWADeps(t *testing.T, pool *pgxpool.Pool) (*api.Dependencies, string, string) {
	t.Helper()

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope03-pwa-key-2026-05"
	const userID = "scope03-pwa-user"

	bearerStore, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}
	if err := bearerStore.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: "scope03-pwa-test",
		Notes:      "spec 044 Scope 03 PWA per-user session test",
	}); err != nil {
		t.Fatalf("Enroll: %v", err)
	}

	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    "tok-scope03-pwa-001", // gitleaks:allow — opaque test JTI, not a secret
		SigningKey: priv,
		KeyID:      kid,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	deps := &api.Dependencies{
		Environment: "production",
		AuthToken:   "", // production + per-user; no shared token.
		AuthConfig: config.AuthConfig{
			Enabled:                               true,
			TokenFormat:                           "paseto_v4_public",
			SigningActivePrivateKey:               priv,
			SigningActiveKeyID:                    kid,
			TokenTTLHours:                         1,
			RotationGraceWindowHours:              1,
			ClockSkewToleranceSeconds:             60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                      priv + "-hash-suffix-distinct",
			ProductionSharedTokenFallbackEnabled:  false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),
		// PhotosHandlers is the easiest authenticated GET surface
		// available without standing up the entire dependency graph
		// (NewPhotosHandlers tolerates a nil store and returns the
		// default capability matrix).
		PhotosHandlers: api.NewPhotosHandlers(nil, config.PhotosConfig{}, "production"),
	}
	return deps, issued.WireToken, userID
}

// TestE2E_PWAAuth_Production_PerUserSession discharges
// FINALIZE-PREREQ-044-V7-001. The PWA flow MUST work end-to-end:
// POST /v1/web/login → cookie → authenticated GET → 200.
func TestE2E_PWAAuth_Production_PerUserSession(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)

	deps, wireToken, wantUserID := productionPWADeps(t, pool)

	// httptest.NewTLSServer is required: the production-mode login
	// handler emits a cookie with Secure=true (per design.md §10.4),
	// and net/http/cookiejar refuses to surface Secure cookies to
	// http:// origins. Using a TLS test server lets us prove the
	// production-shaped cookie roundtrips correctly.
	server := httptest.NewTLSServer(api.NewRouter(deps))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := server.Client()
	client.Jar = jar
	client.Timeout = 15 * time.Second
	// Belt-and-brace — the test server's self-signed cert is already
	// trusted via server.Client(); InsecureSkipVerify defends against
	// any future TLS hardening test runner that resets the transport.
	if tr, ok := client.Transport.(*http.Transport); ok && tr.TLSClientConfig != nil {
		tr.TLSClientConfig.InsecureSkipVerify = true
	} else {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	// Step 1 — POST /v1/web/login with the PASETO body. Expect 200
	// with user_id derived from PASETO claims, and Set-Cookie
	// auth_token=<wire>; HttpOnly; Secure; SameSite=Lax; Path=/.
	loginBody, err := json.Marshal(map[string]string{"token": wireToken})
	if err != nil {
		t.Fatalf("marshal login body: %v", err)
	}
	loginResp, err := client.Post(server.URL+"/v1/web/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("POST /v1/web/login: %v", err)
	}
	defer loginResp.Body.Close()
	loginBytes, _ := io.ReadAll(loginResp.Body)
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("/v1/web/login status=%d body=%s", loginResp.StatusCode, string(loginBytes))
	}

	var parsedLogin struct {
		UserID    string `json:"user_id"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(loginBytes, &parsedLogin); err != nil {
		t.Fatalf("decode login response: %v body=%s", err, string(loginBytes))
	}
	if parsedLogin.UserID != wantUserID {
		t.Fatalf("login user_id=%q want %q (must come from PASETO sub claim, not request body)", parsedLogin.UserID, wantUserID)
	}
	if parsedLogin.ExpiresAt == "" {
		t.Errorf("login response missing expires_at")
	}

	// Inspect Set-Cookie attributes — design.md §10.4 cookie model.
	var authCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatalf("login did not Set-Cookie auth_token; response headers=%v", loginResp.Header)
	}
	if !authCookie.HttpOnly {
		t.Errorf("auth_token cookie missing HttpOnly (design.md §10.4)")
	}
	if !authCookie.Secure {
		t.Errorf("auth_token cookie missing Secure in production (design.md §10.4)")
	}
	if authCookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("auth_token cookie SameSite=%v want Lax (design.md §10.4)", authCookie.SameSite)
	}
	if authCookie.Path != "/" {
		t.Errorf("auth_token cookie Path=%q want / (design.md §10.4)", authCookie.Path)
	}
	if authCookie.Value != wireToken {
		t.Errorf("auth_token cookie value does not match minted PASETO wire token")
	}

	// Step 2 — GET /v1/photos/connectors with the cookie. The
	// cookiejar carries auth_token automatically. The handler is
	// behind bearerAuthMiddleware Branch 1 (production per-user
	// PASETO); cookie-sourced extraction is the new Scope 03 path
	// added to extractBearerToken().
	authedReq, err := http.NewRequest(http.MethodGet, server.URL+"/v1/photos/connectors", nil)
	if err != nil {
		t.Fatalf("build authed request: %v", err)
	}
	authedResp, err := client.Do(authedReq)
	if err != nil {
		t.Fatalf("GET /v1/photos/connectors with cookie: %v", err)
	}
	defer authedResp.Body.Close()
	if authedResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(authedResp.Body)
		t.Fatalf("authed GET /v1/photos/connectors status=%d body=%s — cookie-sourced bearer auth must succeed", authedResp.StatusCode, string(body))
	}
	authedBody, _ := io.ReadAll(authedResp.Body)
	if !bytes.Contains(authedBody, []byte(`"connectors"`)) {
		t.Errorf("authed GET response missing connectors field; body=%s", string(authedBody))
	}

	// Adversarial 1 — bare GET (no cookie, no header) MUST be 401.
	bareJar, _ := cookiejar.New(nil)
	bareClient := &http.Client{
		Jar:       bareJar,
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	bareResp, err := bareClient.Get(server.URL + "/v1/photos/connectors")
	if err != nil {
		t.Fatalf("bare GET: %v", err)
	}
	defer bareResp.Body.Close()
	if bareResp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(bareResp.Body)
		t.Errorf("bare GET status=%d want 401 body=%s — production must reject unauthenticated reads", bareResp.StatusCode, string(body))
	}
}

// TestE2E_PWAAuth_Production_LoginRejectsMissingToken proves the
// happy-path adversarial: a malformed login request does NOT create
// a session.
func TestE2E_PWAAuth_Production_LoginRejectsMissingToken(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)
	deps, _, _ := productionPWADeps(t, pool)
	server := httptest.NewTLSServer(api.NewRouter(deps))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second

	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty body", `{}`, "missing_token"},
		{"empty token", `{"token":""}`, "missing_token"},
		{"whitespace token", `{"token":"   "}`, "missing_token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Post(server.URL+"/v1/web/login", "application/json", bytes.NewReader([]byte(tc.body)))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status=%d want 400 body=%s", resp.StatusCode, string(body))
			}
			if !bytes.Contains(body, []byte(tc.want)) {
				t.Errorf("body=%s want code=%s", string(body), tc.want)
			}
			if len(resp.Cookies()) > 0 {
				for _, c := range resp.Cookies() {
					if c.Name == "auth_token" {
						t.Errorf("rejected login set auth_token cookie %v", c)
					}
				}
			}
		})
	}
}

// TestE2E_PWAAuth_Production_LoginRejectsInvalidToken proves a
// signed-but-foreign or malformed PASETO is rejected with 401 and
// no cookie is set.
func TestE2E_PWAAuth_Production_LoginRejectsInvalidToken(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)
	deps, _, _ := productionPWADeps(t, pool)
	server := httptest.NewTLSServer(api.NewRouter(deps))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second

	// PASETO minted under an UNRELATED keypair so the production
	// verifier rejects it (kid mismatch and/or signature mismatch).
	otherPriv, _ := auth.GenerateSigningKeypair()
	foreign, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "mallory",
		TokenID:    "tok-foreign-001", // gitleaks:allow — opaque test JTI, not a secret
		SigningKey: otherPriv,
		KeyID:      "foreign-kid-not-trusted",
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken (foreign): %v", err)
	}

	cases := []struct {
		name  string
		token string
	}{
		{"random garbage", "this-is-not-a-paseto-token"},
		{"foreign-signed paseto", foreign.WireToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"token": tc.token})
			resp, err := client.Post(server.URL+"/v1/web/login", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST: %v", err)
			}
			defer resp.Body.Close()
			respBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status=%d want 401 body=%s", resp.StatusCode, string(respBytes))
			}
			if !bytes.Contains(respBytes, []byte("invalid_token")) {
				t.Errorf("body=%s want code=invalid_token", string(respBytes))
			}
			for _, c := range resp.Cookies() {
				if c.Name == "auth_token" {
					t.Errorf("rejected login set auth_token cookie %v", c)
				}
			}
		})
	}
}

// TestE2E_PWAAuth_Production_AuthorizationHeaderStillWorks proves
// the cookie fallback in extractBearerToken did NOT regress the
// existing Authorization header path. Same wire PASETO, same
// production middleware, but the test attaches the bearer via the
// header instead of via the cookie.
func TestE2E_PWAAuth_Production_AuthorizationHeaderStillWorks(t *testing.T) {
	pool := authTestPool(t)
	defer pool.Close()
	resetAuthTables(t, pool)
	deps, wireToken, _ := productionPWADeps(t, pool)
	server := httptest.NewTLSServer(api.NewRouter(deps))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second

	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/photos/connectors", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+wireToken)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Authorization header GET status=%d body=%s — header path must still work", resp.StatusCode, string(body))
	}
}

// debugDump is retained for ad-hoc diagnostics during failing test
// triage; suppressed via a build-time guard so it compiles even when
// unused.
var _ = func() {
	_, _ = fmt.Println, debugDump
}

func debugDump(resp *http.Response) string {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("status=%d headers=%v body=%s", resp.StatusCode, resp.Header, string(body))
}
