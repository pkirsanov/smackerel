//go:build integration

// Spec 044 Scope 03 — chaos-phase integration tests for the per-user
// bearer-auth WEB SURFACES (PWA login + cookie-fallback middleware,
// browser-extension token rotation race, Telegram chat→user mapping
// under concurrent reads, admin UI under bearer revocation, Telegram
// per-user PASETO mint under DB pressure).
//
// Where Scope 02 chaos exercised the production middleware path on
// the API hot path (admit/reject + closures), Scope 03 chaos exercises
// the surfaces that LAYER on top of that path: the cookie-derived
// session, extension-style header bearer, the in-memory chat→user
// mapping consulted by the Telegram bot, and the admin UI HTML
// served behind the same middleware.
//
//	C3-B01 → TestAuthChaos_S03_PWALoginCookieJarChurn_NoSessionInterleave
//	C3-B02 → TestAuthChaos_S03_ExtensionTokenRotationRace_GraceWindowSurvives
//	C3-B03 → TestAuthChaos_S03_TelegramMappingConcurrentReads_NoRaceNoLeak
//	C3-B04 → TestAuthChaos_S03_AdminUIUnderRevocationRace_HTMLOrCleanReject
//	C3-B05 → TestAuthChaos_S03_TelegramMintUnderDBPressure_AllSucceed
//	C3-Hot → BenchmarkAuthChaos_S03_PWACookieDerivedSession_HotPath
//
// All tests are race-safe (the package builds clean under `-race`),
// none use `t.Skip()`, and all chaos data is created with a
// `chaos-044-s03-` prefix + per-run nanosecond suffix so the
// `resetAuthTables` t.Cleanup hook truncates without cross-talk
// across `-count=N` stress loops.
//
// Live-stack expectations (per copilot-instructions.md ephemeral test
// DB rule): tests run against the live PostgreSQL on
// `127.0.0.1:47001` (DATABASE_URL) and the live NATS on
// `127.0.0.1:47002` (CHAOS_NATS_URL or NATS_URL). When env is missing
// the test fatals — no silent skips.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/telegram"
)

// chaosS03Deps is the per-test fixture for Scope 03 chaos behaviors.
// It mirrors `chaosS02Deps` in shape but additionally wires the
// AnnotationHandlers (so claim-binding paths reachable from the
// Telegram surface stay observable) and uses a per-test signing
// keypair so revocation cross-talk between concurrent test runs
// (under `-count=N`) is impossible.
type chaosS03Deps struct {
	pool        *pgxpool.Pool
	deps        *api.Dependencies
	store       *auth.BearerStore
	cache       *revocation.Cache
	broadcaster *revocation.Broadcaster
	priv, pub   string
	kid         string
	hashKey     string
	runID       string
}

// uniqueChaosS03Subject returns a per-run NATS subject so concurrent
// chaos tests do not cross-talk on the revocation broadcaster.
func uniqueChaosS03Subject(prefix string) string {
	return fmt.Sprintf("auth.revocations.test.chaos-s03.%s.%d", prefix, time.Now().UnixNano())
}

// uniqueChaosS03RunID returns a per-test-run identifier so user_ids
// and token_ids do not collide across `-count=N` stress iterations.
// All chaos rows are prefixed with `chaos-044-s03-<runID>-...`.
func uniqueChaosS03RunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// newChaosS03Deps wires the live-DB + (optional) live-NATS fixture
// with the production middleware branch active. AnnotationHandlers is
// wired with a stub store so the Telegram body-claim chaos test can
// observe the rejection path without persisting rows; PhotosHandlers
// uses the real store so the cookie/header probe (GET /v1/photos/
// connectors) returns a real JSON envelope.
func newChaosS03Deps(t *testing.T, wireBroadcaster bool, subjectPrefix, runIDPrefix string) *chaosS03Deps {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	priv, pub := auth.GenerateSigningKeypair()
	kid := "scope03-chaos-key-" + runIDPrefix
	hashKey := priv + "-chaos-s03-hash-suffix-distinct"

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	cd := &chaosS03Deps{
		pool:    pool,
		store:   store,
		priv:    priv,
		pub:     pub,
		kid:     kid,
		hashKey: hashKey,
		runID:   uniqueChaosS03RunID(runIDPrefix),
	}

	cd.cache = revocation.NewCache()
	photoStore := photolib.NewStore(pool)
	annotationStore := annotation.NewStore(pool, nil) // nil NATS tolerated by the store

	if wireBroadcaster {
		nc := chaosNATSConn(t)
		t.Cleanup(func() { nc.Close() })
		subject := uniqueChaosS03Subject(subjectPrefix)
		broadcaster, err := revocation.NewBroadcaster(nc, subject, cd.cache, "test-instance-chaos-s03-"+cd.runID)
		if err != nil {
			t.Fatalf("NewBroadcaster: %v", err)
		}
		if err := broadcaster.Subscribe(); err != nil {
			t.Fatalf("Broadcaster.Subscribe: %v", err)
		}
		t.Cleanup(func() { _ = broadcaster.Stop() })
		cd.broadcaster = broadcaster
	}

	authAdmin, err := api.NewAuthAdminHandlers(store, &config.Config{
		Environment: "production",
		Auth: config.AuthConfig{
			Enabled:                               true,
			TokenFormat:                           "paseto_v4_public",
			SigningActivePrivateKey:               priv,
			SigningActiveKeyID:                    kid,
			TokenTTLHours:                         24,
			RotationGraceWindowHours:              2,
			ClockSkewToleranceSeconds:             60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                      hashKey,
			ProductionSharedTokenFallbackEnabled:  false,
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewAuthAdminHandlers: %v", err)
	}

	cd.deps = &api.Dependencies{
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                               true,
			TokenFormat:                           "paseto_v4_public",
			SigningActivePrivateKey:               priv,
			SigningActiveKeyID:                    kid,
			TokenTTLHours:                         24,
			RotationGraceWindowHours:              2,
			ClockSkewToleranceSeconds:             60,
			RevocationCacheRefreshIntervalSeconds: 60,
			AtRestHashingKey:                      hashKey,
			ProductionSharedTokenFallbackEnabled:  false,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    pub,
			ActiveKeyID:        kid,
			Issuer:             "smackerel",
			ClockSkewTolerance: time.Minute,
			Now:                time.Now,
		},
		BearerStore:       store,
		RevocationCache:   cd.cache,
		AuthAdminHandlers: authAdmin,
		PhotosHandlers:    api.NewPhotosHandlers(photoStore, config.PhotosConfig{}, "production"),
		AnnotationHandlers: &api.AnnotationHandlers{
			Store:       annotationStore,
			Environment: "production",
		},
	}
	return cd
}

// enrollAndIssue enrolls a user (idempotent on uniqueness violations)
// and issues + persists a real PASETO token for them. Returns the
// IssueResult so the caller can use the wire token directly.
func (c *chaosS03Deps) enrollAndIssue(t *testing.T, userID, tokenID string, ttl time.Duration) auth.IssueResult {
	t.Helper()
	if err := c.store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: "chaos-s03-test",
		Notes:      "spec 044 Scope 03 chaos fixture",
	}); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
			!strings.Contains(strings.ToLower(err.Error()), "unique") {
			t.Fatalf("Enroll(%q): %v", userID, err)
		}
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: c.priv,
		KeyID:      c.kid,
		TTL:        ttl,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken(%q): %v", tokenID, err)
	}
	hashed, err := auth.HashToken(issued.WireToken, c.hashKey)
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}
	if err := c.store.PersistToken(context.Background(), auth.PersistTokenParams{
		TokenID:      tokenID,
		UserID:       userID,
		KeyID:        c.kid,
		IssuedAt:     issued.IssuedAt,
		ExpiresAt:    issued.ExpiresAt,
		HashedToken:  hashed,
		IssuedBy:     "chaos-s03-test",
		IssuedSource: "admin_api",
	}); err != nil {
		t.Fatalf("PersistToken(%q): %v", tokenID, err)
	}
	return issued
}

// loginRequest builds a POST /v1/web/login request carrying the supplied
// PASETO bearer in the JSON body. RemoteAddr is set so the per-IP
// rate limiter (httprate.LimitByIP(20, 1*time.Minute)) does not
// cross-talk between concurrent goroutines that share the same
// jar/user.
func loginRequest(token, remoteAddr string) *http.Request {
	body := []byte(fmt.Sprintf(`{"token":%q}`, token))
	req := httptest.NewRequest(http.MethodPost, "/v1/web/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	req.ContentLength = int64(len(body))
	return req
}

// connectorsRequestWithCookie builds a GET /v1/photos/connectors
// request carrying ONLY the cookie (no Authorization header). This
// exercises the Scope 03 cookie-fallback path inside
// extractBearerToken end-to-end.
func connectorsRequestWithCookie(cookieValue, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/v1/photos/connectors", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: cookieValue})
	req.RemoteAddr = remoteAddr
	return req
}

// connectorsRequestWithBearer builds a GET /v1/photos/connectors
// request carrying ONLY the Authorization header bearer (extension
// surface).
func connectorsRequestWithBearer(token, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/v1/photos/connectors", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = remoteAddr
	return req
}

// adminTokensUIRequest builds GET /admin/auth/tokens with the bearer
// in the Authorization header. Used by C3-B04.
func adminTokensUIRequest(token, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/admin/auth/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = remoteAddr
	return req
}

// --- C3-B01 ---------------------------------------------------------

// TestAuthChaos_S03_PWALoginCookieJarChurn_NoSessionInterleave proves
// the PWA login + cookie-fallback middleware is jar-isolated under
// concurrent contention. Each of N=50 goroutines drives 10 iterations
// of (login → use cookie → assert correct admit). A regression that
// shared writer state across responses (e.g., a misplaced package-
// level cookie buffer) would surface as a goroutine receiving someone
// else's Set-Cookie value, an admit failure with the wrong session,
// or a race-detector hit on the cookie-set path.
//
// Each goroutine uses a distinct synthetic RemoteAddr so the per-IP
// rate-limiter on /v1/web/login does not engage (20 req/min/IP).
// The 10 inner iterations use only the cookie path (no further
// /v1/web/login calls), so rate-limit interference is impossible.
func TestAuthChaos_S03_PWALoginCookieJarChurn_NoSessionInterleave(t *testing.T) {
	cd := newChaosS03Deps(t, false, "pwa-cookie-jar", "pwa-jar")
	router := api.NewRouter(cd.deps)

	const concurrent = 50
	const iterations = 10

	// Mint one user + one PASETO per goroutine. The wire token is
	// the value that MUST be returned in Set-Cookie; if any goroutine
	// observes a Set-Cookie value that does not match its own token,
	// that proves cross-jar interleaving.
	tokens := make([]auth.IssueResult, concurrent)
	for i := 0; i < concurrent; i++ {
		userID := fmt.Sprintf("chaos-044-s03-%s-jar-user-%d", cd.runID, i)
		tokenID := fmt.Sprintf("chaos-044-s03-%s-jar-tok-%d", cd.runID, i)
		tokens[i] = cd.enrollAndIssue(t, userID, tokenID, time.Hour)
	}

	var loginAdmit, loginReject atomic.Int64
	var sessionAdmit, sessionReject atomic.Int64
	var jarLeakCount atomic.Int64
	var failures []string
	var failuresMu sync.Mutex

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < concurrent; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate

			// Distinct RemoteAddr per goroutine — the IP is the rate-
			// limit key, so each "jar" gets its own 20-req/min budget.
			remoteAddr := fmt.Sprintf("10.0.0.%d:1%03d", (i%250)+1, i)

			// Step 1: POST /v1/web/login with the token.
			loginReq := loginRequest(tokens[i].WireToken, remoteAddr)
			loginRec := httptest.NewRecorder()
			router.ServeHTTP(loginRec, loginReq)
			if loginRec.Code != http.StatusOK {
				loginReject.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("jar=%d login status=%d body=%s", i, loginRec.Code, loginRec.Body.String()))
				failuresMu.Unlock()
				return
			}
			loginAdmit.Add(1)

			// Step 2: read Set-Cookie from response. The cookie value
			// MUST be the token we sent — anything else is a jar leak.
			resp := http.Response{Header: loginRec.Header()}
			cookies := resp.Cookies()
			var observed string
			for _, c := range cookies {
				if c.Name == "auth_token" {
					observed = c.Value
					break
				}
			}
			if observed != tokens[i].WireToken {
				jarLeakCount.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("jar=%d cookie LEAK: observed=%q expected=%q",
					i, truncForLog(observed), truncForLog(tokens[i].WireToken)))
				failuresMu.Unlock()
				return
			}

			// Step 3: 10 iterations of (use cookie on /v1/photos/
			// connectors → assert 200 admit). Every iteration uses
			// the cookie value the goroutine just received; a leak
			// would surface here as a 401 (wrong cookie value or
			// wrong session). HTML response body is parsed as JSON
			// — admit responses MUST yield a {connectors: [...]}
			// envelope.
			for iter := 0; iter < iterations; iter++ {
				probeReq := connectorsRequestWithCookie(observed, remoteAddr)
				probeRec := httptest.NewRecorder()
				router.ServeHTTP(probeRec, probeReq)
				if probeRec.Code != http.StatusOK {
					sessionReject.Add(1)
					failuresMu.Lock()
					failures = append(failures, fmt.Sprintf("jar=%d iter=%d cookie probe status=%d body=%s",
						i, iter, probeRec.Code, probeRec.Body.String()))
					failuresMu.Unlock()
					return
				}
				var payload map[string]any
				if err := json.Unmarshal(probeRec.Body.Bytes(), &payload); err != nil {
					sessionReject.Add(1)
					failuresMu.Lock()
					failures = append(failures, fmt.Sprintf("jar=%d iter=%d body not JSON: %v body=%s",
						i, iter, err, probeRec.Body.String()))
					failuresMu.Unlock()
					return
				}
				if _, ok := payload["connectors"]; !ok {
					sessionReject.Add(1)
					failuresMu.Lock()
					failures = append(failures, fmt.Sprintf("jar=%d iter=%d body missing 'connectors': %s",
						i, iter, probeRec.Body.String()))
					failuresMu.Unlock()
					return
				}
				sessionAdmit.Add(1)
			}
		}()
	}
	close(gate)
	wg.Wait()

	if jarLeakCount.Load() != 0 {
		t.Fatalf("C3-B01: %d cookie-jar interleaving LEAKS detected (NFR-AUTH-006 violation) sample=%v",
			jarLeakCount.Load(), failures[:min(len(failures), 3)])
	}
	if loginReject.Load() != 0 {
		t.Fatalf("C3-B01: %d/%d logins rejected (rate-limit cross-talk?) sample=%v",
			loginReject.Load(), concurrent, failures[:min(len(failures), 3)])
	}
	if loginAdmit.Load() != concurrent {
		t.Fatalf("C3-B01: login admit count mismatch: want=%d got=%d", concurrent, loginAdmit.Load())
	}
	wantSession := int64(concurrent * iterations)
	if sessionReject.Load() != 0 || sessionAdmit.Load() != wantSession {
		t.Fatalf("C3-B01: session admit accounting: want=%d admit got=%d admit + %d reject; sample=%v",
			wantSession, sessionAdmit.Load(), sessionReject.Load(), failures[:min(len(failures), 3)])
	}
	t.Logf("C3-B01: %d concurrent jars × %d iterations → %d logins admitted, %d cookie-derived sessions admitted, ZERO jar leaks (race-detector clean)",
		concurrent, iterations, loginAdmit.Load(), sessionAdmit.Load())
}

// --- C3-B02 ---------------------------------------------------------

// TestAuthChaos_S03_ExtensionTokenRotationRace_GraceWindowSurvives
// simulates a realistic browser-extension rotation event: the user has
// an active extension session with token T1; the operator rotates the
// user's token via the admin REST surface mid-stream; the extension
// keeps firing background sync calls with T1 (still in the grace
// window) AND the freshly-issued T2 must admit immediately. Every
// in-flight request MUST land in {200 admit, 401 reject post-grace}
// — never dropped, never panic, never mis-attributed.
//
// Because rotation grace is parameterized by the verifier's clock,
// the test wires Now=time.Now (the grace window is 2h, so the
// 1h-TTL T1 lives well inside grace for the duration of the test).
//
// Concurrency shape: 100 goroutines pre-rotation hammer with T1; one
// rotation goroutine marks T1 rotated and issues T2; 100 goroutines
// post-rotation hammer with T1 (still inside grace) AND 100 goroutines
// hammer with T2.
func TestAuthChaos_S03_ExtensionTokenRotationRace_GraceWindowSurvives(t *testing.T) {
	cd := newChaosS03Deps(t, false, "ext-rotation", "ext-rot")
	router := api.NewRouter(cd.deps)

	userID := fmt.Sprintf("chaos-044-s03-%s-ext-rot-user", cd.runID)
	t1ID := fmt.Sprintf("chaos-044-s03-%s-ext-rot-t1", cd.runID)
	t2ID := fmt.Sprintf("chaos-044-s03-%s-ext-rot-t2", cd.runID)

	// T1: 1-hour TTL — well inside the 2-hour grace window.
	t1 := cd.enrollAndIssue(t, userID, t1ID, time.Hour)

	const preReqs = 100
	const postT1Reqs = 100
	const postT2Reqs = 100

	// Status accounting: admit (200) is the auth-correctness signal we
	// care about; throttle (503 from chi middleware.Throttle, which is
	// a global capacity ceiling orthogonal to auth) is a known router
	// invariant — it MUST NOT count as an auth-reject. authReject
	// (401/403) is the chaos invariant: under rotation race, NO
	// in-flight request may be wrongly auth-rejected.
	var preAdmit, preThrottle, preAuthReject atomic.Int64
	var postT1Admit, postT1Throttle, postT1AuthReject atomic.Int64
	var postT2Admit, postT2Throttle, postT2AuthReject atomic.Int64
	var unexpectedStatuses []string
	var unexpectedMu sync.Mutex

	recordUnexpected := func(label string, code int, body string) {
		unexpectedMu.Lock()
		unexpectedStatuses = append(unexpectedStatuses, fmt.Sprintf("[%s] status=%d body=%s", label, code, truncForLog(body)))
		unexpectedMu.Unlock()
	}

	classify := func(label string, code int, body string, admit, throttle, authReject *atomic.Int64) {
		switch {
		case code == http.StatusOK:
			admit.Add(1)
		case code == http.StatusServiceUnavailable, code == http.StatusTooManyRequests:
			// chi middleware.Throttle returns 503 when the global in-flight
			// ceiling is reached; httprate per-IP returns 429 (not relevant
			// here because we randomize RemoteAddr). Both are orthogonal to
			// auth correctness — they mean "router refused to handle", not
			// "auth wrongly rejected".
			throttle.Add(1)
		case code == http.StatusUnauthorized, code == http.StatusForbidden:
			authReject.Add(1)
			recordUnexpected(label, code, body)
		default:
			authReject.Add(1)
			recordUnexpected(label+"-other", code, body)
		}
	}

	// Pre-rotation: T1 must admit cleanly under load (no auth-rejects).
	{
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for i := 0; i < preReqs; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				req := connectorsRequestWithBearer(t1.WireToken, fmt.Sprintf("10.1.0.%d:1234", (i%250)+1))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				classify("pre-rot-T1", rec.Code, rec.Body.String(), &preAdmit, &preThrottle, &preAuthReject)
			}()
		}
		close(gate)
		wg.Wait()
	}
	if preAuthReject.Load() != 0 || (preAdmit.Load()+preThrottle.Load()) != preReqs {
		sample := unexpectedStatuses
		if len(sample) > 3 {
			sample = sample[:3]
		}
		t.Fatalf("C3-B02 pre-rotation: T1 must never auth-reject; admit=%d throttle=%d authReject=%d total_expected=%d sample=%v",
			preAdmit.Load(), preThrottle.Load(), preAuthReject.Load(), preReqs, sample)
	}

	// Rotate: mark T1 rotated, persist T2.
	if err := cd.store.MarkTokenRotated(context.Background(), t1ID); err != nil {
		t.Fatalf("MarkTokenRotated(T1): %v", err)
	}
	t2 := cd.enrollAndIssue(t, userID, t2ID, 24*time.Hour)

	// Post-rotation concurrent: T1 (inside grace) must admit AND T2
	// must admit. Both leg sets run simultaneously to maximize the
	// race surface.
	{
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for i := 0; i < postT1Reqs; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				req := connectorsRequestWithBearer(t1.WireToken, fmt.Sprintf("10.2.0.%d:1234", (i%250)+1))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				classify("post-rot-T1", rec.Code, rec.Body.String(), &postT1Admit, &postT1Throttle, &postT1AuthReject)
			}()
		}
		for i := 0; i < postT2Reqs; i++ {
			i := i
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				req := connectorsRequestWithBearer(t2.WireToken, fmt.Sprintf("10.3.0.%d:1234", (i%250)+1))
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				classify("post-rot-T2", rec.Code, rec.Body.String(), &postT2Admit, &postT2Throttle, &postT2AuthReject)
			}()
		}
		close(gate)
		wg.Wait()
	}

	if postT1AuthReject.Load() != 0 || (postT1Admit.Load()+postT1Throttle.Load()) != postT1Reqs {
		sample := unexpectedStatuses
		if len(sample) > 3 {
			sample = sample[:3]
		}
		t.Fatalf("C3-B02 post-rotation T1 (inside grace) must never auth-reject; admit=%d throttle=%d authReject=%d total_expected=%d sample=%v",
			postT1Admit.Load(), postT1Throttle.Load(), postT1AuthReject.Load(), postT1Reqs, sample)
	}
	if postT2AuthReject.Load() != 0 || (postT2Admit.Load()+postT2Throttle.Load()) != postT2Reqs {
		sample := unexpectedStatuses
		if len(sample) > 3 {
			sample = sample[:3]
		}
		t.Fatalf("C3-B02 post-rotation T2 (active) must never auth-reject; admit=%d throttle=%d authReject=%d total_expected=%d sample=%v",
			postT2Admit.Load(), postT2Throttle.Load(), postT2AuthReject.Load(), postT2Reqs, sample)
	}
	// Chaos invariant proof: at least ONE post-rotation T1 (grace)
	// admit MUST be observed — otherwise the test could pass via 100%
	// throttle even if the grace path were broken. The test would be
	// non-adversarial without this lower bound.
	if postT1Admit.Load() == 0 {
		t.Fatalf("C3-B02 post-rotation T1: zero admits observed (all throttled) — cannot prove grace path; admit=%d throttle=%d",
			postT1Admit.Load(), postT1Throttle.Load())
	}
	if postT2Admit.Load() == 0 {
		t.Fatalf("C3-B02 post-rotation T2: zero admits observed (all throttled) — cannot prove active path; admit=%d throttle=%d",
			postT2Admit.Load(), postT2Throttle.Load())
	}
	t.Logf("C3-B02: pre admit=%d throttle=%d authReject=%d | post-rot T1(grace) admit=%d throttle=%d authReject=%d | post-rot T2(active) admit=%d throttle=%d authReject=%d (race-detector clean; throttle is orthogonal to auth correctness)",
		preAdmit.Load(), preThrottle.Load(), preAuthReject.Load(),
		postT1Admit.Load(), postT1Throttle.Load(), postT1AuthReject.Load(),
		postT2Admit.Load(), postT2Throttle.Load(), postT2AuthReject.Load())
}

// --- C3-B03 ---------------------------------------------------------

// TestAuthChaos_S03_TelegramMappingConcurrentReads_NoRaceNoLeak fires
// N=200 concurrent resolveActorUserID calls against a *Bot whose
// userMapping holds 50 distinct chat→user entries. Half the goroutines
// query mapped chat ids (must return the right user_id); the other
// half query unmapped chat ids (must return ErrNoUserMappingForChat
// in production). At the same time, M=20 ParseUserMapping calls run
// concurrently with a varied input set so the parser's allocation
// path is also stressed. The map itself is NEVER mutated post-
// construction (Bot.userMapping is set once at NewBot time); the
// chaos surface here is concurrent READ correctness, which is the
// only guarantee the implementation makes today.
//
// Note on hot-reload: the production code does NOT support runtime
// hot-reload of TELEGRAM_USER_MAPPING — restart is required. The
// chaos contract therefore proves that under heavy concurrent reads
// (which is the realistic load: every inbound Telegram message
// triggers one resolveActorUserID call) the map access is race-free
// and returns deterministic results. ParseUserMapping is exercised
// in parallel to prove the parser allocates a fresh map per call (so
// even a hypothetical reload-on-each-message strategy would be
// race-safe).
func TestAuthChaos_S03_TelegramMappingConcurrentReads_NoRaceNoLeak(t *testing.T) {
	const mappingSize = 50
	mapping := make(map[int64]string, mappingSize)
	for i := 0; i < mappingSize; i++ {
		mapping[int64(1000+i)] = fmt.Sprintf("tg-chaos-s03-user-%d", i)
	}
	bot := telegram.NewBotForTest("production", mapping)

	// resolveActorUserID is unexported — exercise it via the
	// per-user PASETO minter (the only public surface that calls it)
	// so the chaos test stays inside the public API.
	priv, _ := auth.GenerateSigningKeypair()
	const kid = "scope03-chaos-mapping-key"
	minter, err := telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      kid,
		Issuer:     "smackerel",
		TTL:        time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}

	const mappedReqs = 100
	const unmappedReqs = 100
	const parseStress = 20

	var mappedOK, mappedWrong atomic.Int64
	var unmappedRefused, unmappedAccepted atomic.Int64
	var parseOK, parseFail atomic.Int64
	var failures []string
	var failuresMu sync.Mutex

	var wg sync.WaitGroup
	gate := make(chan struct{})

	// Mapped chat reads — each goroutine resolves a distinct chat id
	// and asserts the resulting MintForChat returns the mapped user.
	for i := 0; i < mappedReqs; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			chatID := int64(1000 + (i % mappingSize))
			expectedUser := mapping[chatID]
			tok, err := minter.MintForChat(chatID)
			if err != nil {
				mappedWrong.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("mapped chat=%d unexpected err=%v", chatID, err))
				failuresMu.Unlock()
				return
			}
			if tok.UserID != expectedUser {
				mappedWrong.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("mapped chat=%d UserID=%q want=%q (mapping LEAK)", chatID, tok.UserID, expectedUser))
				failuresMu.Unlock()
				return
			}
			mappedOK.Add(1)
		}()
	}

	// Unmapped chat reads — production MUST refuse with
	// ErrNoUserMappingForChat. A goroutine that mints a token here
	// would prove a cross-read leak (e.g., reading the wrong slot of
	// a concurrently-rebuilt map).
	for i := 0; i < unmappedReqs; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			chatID := int64(900_000 + i) // guaranteed unmapped
			_, err := minter.MintForChat(chatID)
			if err == nil {
				unmappedAccepted.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("unmapped chat=%d MINTED a token (production rejection regression)", chatID))
				failuresMu.Unlock()
				return
			}
			if !strings.Contains(err.Error(), "no production user mapping") {
				unmappedAccepted.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("unmapped chat=%d unexpected err=%v", chatID, err))
				failuresMu.Unlock()
				return
			}
			unmappedRefused.Add(1)
		}()
	}

	// Parse-side stress — each goroutine builds a synthetic raw
	// mapping string and parses it. Each call returns a fresh map
	// (the parser does not share state); a race here would surface
	// as either a wrong returned map size or a panic on map writes.
	for i := 0; i < parseStress; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			pairs := make([]string, 0, 5)
			for k := 0; k < 5; k++ {
				pairs = append(pairs, fmt.Sprintf("%d:tg-chaos-parse-user-%d-%d", 50000+i*100+k, i, k))
			}
			raw := strings.Join(pairs, ",")
			parsed, err := telegram.ParseUserMapping(raw)
			if err != nil || len(parsed) != 5 {
				parseFail.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("parse iter=%d err=%v size=%d", i, err, len(parsed)))
				failuresMu.Unlock()
				return
			}
			parseOK.Add(1)
		}()
	}

	close(gate)
	wg.Wait()

	if mappedWrong.Load() != 0 || mappedOK.Load() != mappedReqs {
		t.Fatalf("C3-B03: mapped reads: want %d ok / 0 wrong, got ok=%d wrong=%d sample=%v",
			mappedReqs, mappedOK.Load(), mappedWrong.Load(), failures[:min(len(failures), 3)])
	}
	if unmappedAccepted.Load() != 0 || unmappedRefused.Load() != unmappedReqs {
		t.Fatalf("C3-B03: unmapped reads: want %d refused / 0 accepted, got refused=%d accepted=%d sample=%v",
			unmappedReqs, unmappedRefused.Load(), unmappedAccepted.Load(), failures[:min(len(failures), 3)])
	}
	if parseFail.Load() != 0 || parseOK.Load() != parseStress {
		t.Fatalf("C3-B03: parser stress: want %d ok / 0 fail, got ok=%d fail=%d sample=%v",
			parseStress, parseOK.Load(), parseFail.Load(), failures[:min(len(failures), 3)])
	}
	t.Logf("C3-B03: %d mapped reads + %d unmapped reads + %d parser stress → all deterministic, race-detector clean",
		mappedOK.Load(), unmappedRefused.Load(), parseOK.Load())
}

// --- C3-B04 ---------------------------------------------------------

// TestAuthChaos_S03_AdminUIUnderRevocationRace_HTMLOrCleanReject fires
// N=80 concurrent GET /admin/auth/tokens requests with the same
// per-user PASETO bearer; mid-stream, M=1 revoker goroutine revokes
// that bearer via store.RevokeToken + Broadcaster.Publish. Every
// response MUST be one of:
//
//   - 200 + valid HTML body (admit happened before the revocation
//     reached the cache)
//   - 401 + a clean error body (admit attempted after revocation; no
//     HTML payload is rendered in that case because bearerAuthMiddleware
//     short-circuits before HandleAdminTokensUI runs)
//
// Forbidden outcomes:
//
//   - HTTP 5xx (panic / nil-deref / corrupted state)
//   - 200 with a non-HTML / partial / empty body (response writer
//     race producing torn output)
//   - 200 that contains the wire token (NFR-AUTH-007 leak)
//
// The Broadcaster is wired against the live test-stack NATS so the
// Publish↔cache loopback is exercised end-to-end (the same path
// production uses).
func TestAuthChaos_S03_AdminUIUnderRevocationRace_HTMLOrCleanReject(t *testing.T) {
	cd := newChaosS03Deps(t, true, "admin-revoke", "admin-rev")
	router := api.NewRouter(cd.deps)

	userID := fmt.Sprintf("chaos-044-s03-%s-admin-user", cd.runID)
	tokenID := fmt.Sprintf("chaos-044-s03-%s-admin-tok", cd.runID)
	issued := cd.enrollAndIssue(t, userID, tokenID, time.Hour)

	const totalReqs = 80
	// Schedule the revoker to fire roughly halfway through the
	// concurrent burst by injecting it as just another goroutine.
	const revokerSlot = 40

	var admit200HTML, reject401Clean, reject401Other atomic.Int64
	var fiveXX atomic.Int64
	var torn200 atomic.Int64
	var tokenLeak atomic.Int64
	var unexpected []string
	var unexpectedMu sync.Mutex

	revokerDone := make(chan struct{})

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < totalReqs; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			// Stagger the revoker into the middle of the burst so
			// requests both before and after it observe the
			// transition.
			if i == revokerSlot {
				go func() {
					defer close(revokerDone)
					if err := cd.store.RevokeToken(context.Background(), tokenID, "chaos-s03", "chaos-revoke-admin"); err != nil {
						unexpectedMu.Lock()
						unexpected = append(unexpected, fmt.Sprintf("RevokeToken err=%v", err))
						unexpectedMu.Unlock()
						return
					}
					if err := cd.broadcaster.Publish(tokenID, "chaos-revoke-admin"); err != nil {
						unexpectedMu.Lock()
						unexpected = append(unexpected, fmt.Sprintf("Broadcaster.Publish err=%v", err))
						unexpectedMu.Unlock()
					}
				}()
			}
			req := adminTokensUIRequest(issued.WireToken, fmt.Sprintf("10.4.0.%d:1234", (i%250)+1))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			body := rec.Body.String()
			switch {
			case rec.Code == http.StatusOK:
				ct := rec.Header().Get("Content-Type")
				if !strings.HasPrefix(ct, "text/html") {
					torn200.Add(1)
					unexpectedMu.Lock()
					unexpected = append(unexpected, fmt.Sprintf("200 with non-HTML CT=%q body=%s", ct, truncForLog(body)))
					unexpectedMu.Unlock()
					return
				}
				if !strings.Contains(body, "Smackerel — Per-User Bearer Tokens") {
					torn200.Add(1)
					unexpectedMu.Lock()
					unexpected = append(unexpected, fmt.Sprintf("200 with HTML missing page title body=%s", truncForLog(body)))
					unexpectedMu.Unlock()
					return
				}
				if strings.Contains(body, issued.WireToken) {
					tokenLeak.Add(1)
					unexpectedMu.Lock()
					unexpected = append(unexpected, fmt.Sprintf("200 body LEAKED bearer token (NFR-AUTH-007)"))
					unexpectedMu.Unlock()
					return
				}
				admit200HTML.Add(1)
			case rec.Code == http.StatusUnauthorized:
				// 401 body must not leak the token or fail-mode tokens.
				lower := strings.ToLower(body)
				for _, leak := range []string{"revoked", "revocation", "cache hit"} {
					if strings.Contains(lower, leak) {
						reject401Other.Add(1)
						unexpectedMu.Lock()
						unexpected = append(unexpected, fmt.Sprintf("401 body LEAKED failure mode token %q (NFR-AUTH-007): %s", leak, truncForLog(body)))
						unexpectedMu.Unlock()
						return
					}
				}
				if strings.Contains(body, issued.WireToken) {
					tokenLeak.Add(1)
					unexpectedMu.Lock()
					unexpected = append(unexpected, fmt.Sprintf("401 body LEAKED bearer token (NFR-AUTH-007)"))
					unexpectedMu.Unlock()
					return
				}
				reject401Clean.Add(1)
			case rec.Code >= 500:
				fiveXX.Add(1)
				unexpectedMu.Lock()
				unexpected = append(unexpected, fmt.Sprintf("5xx status=%d body=%s", rec.Code, truncForLog(body)))
				unexpectedMu.Unlock()
			default:
				reject401Other.Add(1)
				unexpectedMu.Lock()
				unexpected = append(unexpected, fmt.Sprintf("unexpected status=%d body=%s", rec.Code, truncForLog(body)))
				unexpectedMu.Unlock()
			}
		}()
	}
	close(gate)
	wg.Wait()
	<-revokerDone // ensure the revoker goroutine has finished

	if fiveXX.Load() != 0 {
		t.Fatalf("C3-B04: %d 5xx responses (panic/handler corruption?) sample=%v",
			fiveXX.Load(), unexpected[:min(len(unexpected), 3)])
	}
	if torn200.Load() != 0 {
		t.Fatalf("C3-B04: %d 200 responses with corrupted/non-HTML body (writer race?) sample=%v",
			torn200.Load(), unexpected[:min(len(unexpected), 3)])
	}
	if tokenLeak.Load() != 0 {
		t.Fatalf("C3-B04: %d responses leaked the bearer token (NFR-AUTH-007 violation) sample=%v",
			tokenLeak.Load(), unexpected[:min(len(unexpected), 3)])
	}
	if reject401Other.Load() != 0 {
		t.Fatalf("C3-B04: %d 401 responses with non-clean body (failure-mode leak) sample=%v",
			reject401Other.Load(), unexpected[:min(len(unexpected), 3)])
	}
	if admit200HTML.Load()+reject401Clean.Load() != totalReqs {
		t.Fatalf("C3-B04: accounting mismatch: 200=%d + 401=%d != %d",
			admit200HTML.Load(), reject401Clean.Load(), totalReqs)
	}
	// Confirm the revocation actually landed: a fresh probe AFTER
	// the wait MUST 401.
	probeReq := adminTokensUIRequest(issued.WireToken, "10.4.0.250:9999")
	probeRec := httptest.NewRecorder()
	router.ServeHTTP(probeRec, probeReq)
	if probeRec.Code != http.StatusUnauthorized {
		t.Fatalf("C3-B04: post-burst probe expected 401 (revocation should be permanent), got %d body=%s",
			probeRec.Code, probeRec.Body.String())
	}
	t.Logf("C3-B04: %d/80 admit-with-HTML, %d/80 clean-401-reject, 0 panic/torn/leak (cache convergence intact under contention)",
		admit200HTML.Load(), reject401Clean.Load())
}

// --- C3-B05 ---------------------------------------------------------

// TestAuthChaos_S03_TelegramMintUnderDBPressure_AllSucceed proves the
// Telegram per-user PASETO mint path is decoupled from the DB pool
// — by design, MintForChat performs ONLY in-memory crypto + an
// in-memory map lookup; it never opens a pgx connection. The chaos
// scenario:
//
//  1. Saturate the BearerStore-side DB pool with a long-lived burst
//     of CountUsers / ListUsers calls (real database round-trips).
//  2. Concurrently fire N=50 MintForChat calls.
//  3. Assert every mint succeeded, produced a unique TokenID, and
//     produces a token that round-trips through auth.VerifyAndParse
//     to the expected user_id.
//
// A regression that accidentally introduced a DB query into the mint
// path would surface either as a mint timeout (DB pool exhausted) or
// as an inconsistent UserID claim under contention.
func TestAuthChaos_S03_TelegramMintUnderDBPressure_AllSucceed(t *testing.T) {
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	// Build a Bot with 50 mapped chats; the minter picks the user_id
	// from this map purely in-memory.
	const mappingSize = 50
	mapping := make(map[int64]string, mappingSize)
	for i := 0; i < mappingSize; i++ {
		mapping[int64(2000+i)] = fmt.Sprintf("tg-chaos-s03-mint-user-%d", i)
	}
	bot := telegram.NewBotForTest("production", mapping)

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope03-chaos-mint-key"
	minter, err := telegram.NewPerUserTokenMinter(telegram.PerUserTokenMinterOptions{
		Bot:        bot,
		SigningKey: priv,
		KeyID:      kid,
		Issuer:     "smackerel",
		TTL:        5 * time.Minute,
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("NewPerUserTokenMinter: %v", err)
	}

	const mintReqs = 50
	const dbHogReqs = 200

	// Pre-seed a few users so CountUsers / ListUsers do real work.
	for i := 0; i < 5; i++ {
		uid := fmt.Sprintf("chaos-044-s03-mint-precounter-%d-%d", time.Now().UnixNano(), i)
		if err := store.Enroll(context.Background(), auth.EnrollUserParams{
			UserID:     uid,
			EnrolledBy: "chaos-s03-mint",
			Notes:      "DB hog seed",
		}); err != nil {
			t.Fatalf("Enroll seed: %v", err)
		}
	}

	dbPressureDone := make(chan struct{})
	dbPressureCtx, dbPressureCancel := context.WithCancel(context.Background())
	t.Cleanup(dbPressureCancel)

	var dbCalls atomic.Int64
	go func() {
		defer close(dbPressureDone)
		var wg sync.WaitGroup
		// Spawn dbHogReqs short DB queries running concurrently; the
		// pool's MaxConns=4 in authTestPool means the queries
		// queue and exercise pool contention.
		for i := 0; i < dbHogReqs; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if dbPressureCtx.Err() != nil {
					return
				}
				if _, err := store.CountUsers(dbPressureCtx); err == nil {
					dbCalls.Add(1)
				}
				if _, err := store.ListUsers(dbPressureCtx); err == nil {
					dbCalls.Add(1)
				}
			}()
		}
		wg.Wait()
	}()

	// Concurrent mints — the minter does NOT touch the DB so all
	// must succeed regardless of pool contention.
	var mintOK, mintFail atomic.Int64
	var verifyOK, verifyFail atomic.Int64
	var seenTokenIDsMu sync.Mutex
	seenTokenIDs := make(map[string]struct{})
	var dupTokenIDs atomic.Int64
	var failures []string
	var failuresMu sync.Mutex

	verifyOpts := auth.VerifyOptions{
		ActivePublicKey:    pub,
		ActiveKeyID:        kid,
		Issuer:             "smackerel",
		ClockSkewTolerance: time.Minute,
		Now:                time.Now,
	}

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < mintReqs; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			chatID := int64(2000 + (i % mappingSize))
			expectedUser := mapping[chatID]
			tok, err := minter.MintForChat(chatID)
			if err != nil {
				mintFail.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("mint chat=%d err=%v", chatID, err))
				failuresMu.Unlock()
				return
			}
			mintOK.Add(1)
			seenTokenIDsMu.Lock()
			if _, dup := seenTokenIDs[tok.TokenID]; dup {
				dupTokenIDs.Add(1)
			} else {
				seenTokenIDs[tok.TokenID] = struct{}{}
			}
			seenTokenIDsMu.Unlock()
			parsed, err := auth.VerifyAndParse(tok.WireToken, verifyOpts)
			if err != nil {
				verifyFail.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("verify chat=%d err=%v", chatID, err))
				failuresMu.Unlock()
				return
			}
			if parsed.UserID != expectedUser {
				verifyFail.Add(1)
				failuresMu.Lock()
				failures = append(failures, fmt.Sprintf("verify chat=%d UserID=%q want=%q",
					chatID, parsed.UserID, expectedUser))
				failuresMu.Unlock()
				return
			}
			verifyOK.Add(1)
		}()
	}
	close(gate)
	wg.Wait()

	// Drain the DB-pressure goroutines.
	dbPressureCancel()
	<-dbPressureDone

	if mintFail.Load() != 0 || mintOK.Load() != mintReqs {
		t.Fatalf("C3-B05: mint accounting: want %d ok / 0 fail, got ok=%d fail=%d sample=%v",
			mintReqs, mintOK.Load(), mintFail.Load(), failures[:min(len(failures), 3)])
	}
	if verifyFail.Load() != 0 || verifyOK.Load() != mintReqs {
		t.Fatalf("C3-B05: verify accounting: want %d ok / 0 fail, got ok=%d fail=%d sample=%v",
			mintReqs, verifyOK.Load(), verifyFail.Load(), failures[:min(len(failures), 3)])
	}
	if dupTokenIDs.Load() != 0 {
		t.Fatalf("C3-B05: %d duplicate TokenIDs across %d mints (newTelegramTokenID PRNG regression?)",
			dupTokenIDs.Load(), mintReqs)
	}
	t.Logf("C3-B05: %d mints + %d verifies under %d concurrent DB queries → all succeed, all unique TokenIDs (mint path is DB-independent)",
		mintOK.Load(), verifyOK.Load(), dbCalls.Load())
}

// --- C3-Hot-Path Benchmark ------------------------------------------

// BenchmarkAuthChaos_S03_PWACookieDerivedSession_HotPath measures the
// end-to-end PWA cookie-derived session resolution path:
//
//	GET /v1/photos/connectors
//	  ↓ chi router
//	  ↓ bearerAuthMiddleware
//	  ↓ extractBearerToken (cookie fallback branch)
//	  ↓ auth.VerifyAndParse (PASETO Ed25519 verify)
//	  ↓ revocation.Cache.IsRevoked (in-memory map)
//	  ↓ session attached to context
//	  ↓ PhotosHandlers.ListConnectors
//	  ↓ JSON response
//
// Target: NFR-AUTH-001 budget (~5ms/op for in-process). The cache-hit
// path SHOULD be well under 1ms once warm because the dominant cost
// is Ed25519 signature verification (~100µs amortized).
func BenchmarkAuthChaos_S03_PWACookieDerivedSession_HotPath(b *testing.B) {
	t := &testing.T{}
	cd := newChaosS03Deps(t, false, "hot-path-bench", "hot-path")
	router := api.NewRouter(cd.deps)
	if t.Failed() {
		b.Fatal("fixture setup failed")
	}

	userID := fmt.Sprintf("chaos-044-s03-%s-bench-user", cd.runID)
	tokenID := fmt.Sprintf("chaos-044-s03-%s-bench-tok", cd.runID)
	issued := cd.enrollAndIssue(t, userID, tokenID, time.Hour)
	if t.Failed() {
		b.Fatal("enrollAndIssue failed")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := connectorsRequestWithCookie(issued.WireToken, "10.5.0.1:1234")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("benchmark probe expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

// --- helpers --------------------------------------------------------

// truncForLog truncates a string for safe inclusion in failure logs
// — bearer tokens are long and would dominate test output.
func truncForLog(s string) string {
	const maxLen = 64
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(" + fmt.Sprintf("%d total", len(s)) + ")"
}
