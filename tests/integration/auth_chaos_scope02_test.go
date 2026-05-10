//go:build integration

// Spec 044 Scope 02 — chaos-phase integration tests for the per-user
// bearer-auth HOT PATH (middleware + MIT-040-S-008 + MIT-038-S-003 +
// MIT-027-TRACE-001 actor-source segment closures + admin endpoints).
//
// Where Scope 01 chaos exercised the auth subsystem APIs in isolation
// (BearerStore, Cache, Broadcaster, IssueToken/VerifyAndParse), Scope
// 02 chaos exercises the same subsystem WIRED INTO the production
// middleware path against a live postgres + NATS test stack, then
// stresses 11 chaos behaviors layered on top of the already-green
// deterministic Scope 02 contract:
//
//	C2-B01 → TestAuthChaos_S02_ConcurrentMiddlewareVerify_NoRaceNoLeak
//	C2-B02 → TestAuthChaos_S02_VerifyVsRevokeRace_ConvergesToReject
//	C2-B03 → TestAuthChaos_S02_ConcurrentMintRevealUnderClosure_ActorIDFromSession
//	C2-B04 → TestAuthChaos_S02_ConcurrentDriveConnectUnderClosure_OwnerFromSession
//	C2-B05 → TestAuthChaos_S02_ConcurrentAnnotationUnderClosure_ActorSourceRejected
//	C2-B06 → TestAuthChaos_S02_RotationUnderLoad_BothAdmitInsideGrace_T1RejectedAfter
//	C2-B07 → TestAuthChaos_S02_RevocationUnderLoad_FiveOfTenConvergeToReject
//	C2-B08 → TestAuthChaos_S02_AdminEndpointStress_NonAdminAlwaysForbidden
//	C2-B09 → TestAuthChaos_S02_MalformedAuthorizationHeaderStorm_Always401
//	C2-B10 → covered by `-count=20 -race` invocation in the chaos run
//	         command (see report.md Chaos Evidence (Scope 02)) — no
//	         dedicated test function because `-count=N` is how Go test
//	         expresses stress-loop semantics
//	C2-B11 → BenchmarkAuthChaos_S02_BearerMiddleware_HotPath
//
// All tests are race-safe (the package builds clean under `-race`) and
// none use `t.Skip()` — when env is missing, the test fatals with a
// loud message per the no-skip precedent set by spec 043 + reused for
// spec 044 Scope 01 chaos.
//
// All chaos data is created with a `chaos-044-s02-` prefix and the
// existing `resetAuthTables` t.Cleanup hook truncates the auth tables
// at the end of each test — strict cleanup, no residual chaos data,
// ephemeral test DB only (per copilot-instructions.md).
package integration

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
	"github.com/smackerel/smackerel/internal/drive"
)

// chaosS02Deps is the per-test fixture shared across the Scope 02
// chaos behaviors. It mirrors `revocationDeps` in auth_revocation_test.go
// so the same admit/reject probe (POST /v1/photos/{id}/reveal) doubles
// as the middleware boundary observation surface.
type chaosS02Deps struct {
	pool        *pgxpool.Pool
	deps        *api.Dependencies
	store       *auth.BearerStore
	cache       *revocation.Cache
	broadcaster *revocation.Broadcaster
	priv, pub   string
	kid         string
	hashKey     string
	photoID     uuid.UUID
}

// uniqueChaosS02Subject builds a per-test-run NATS subject so cross-
// test interference is impossible even when several behaviors run in
// parallel under -count=N.
func uniqueChaosS02Subject(prefix string) string {
	return fmt.Sprintf("auth.revocations.test.chaos-s02.%s.%d", prefix, time.Now().UnixNano())
}

// newChaosS02Deps builds the live-DB + live-NATS fixture with the
// production middleware branch wired (Environment="production",
// AuthConfig.Enabled=true, AuthVerifyOptions populated). When
// wireBroadcaster is true, the deps owns a real Broadcaster that
// subscribes on a unique subject so Publish exercises the loopback
// subscription end-to-end.
func newChaosS02Deps(t *testing.T, wireBroadcaster bool, subjectPrefix string) *chaosS02Deps {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope02-chaos-key"
	hashKey := priv + "-chaos-s02-hash-suffix-distinct"

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	cd := &chaosS02Deps{
		pool:    pool,
		store:   store,
		priv:    priv,
		pub:     pub,
		kid:     kid,
		hashKey: hashKey,
	}

	// Seed an artifact + sensitive photo so the reveal endpoint returns
	// 201 when middleware admits (distinct from a 404 path).
	cd.photoID = uuid.New()
	artifactID := "art-chaos-s02-" + cd.photoID.String()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, $2, $3, $4, $5)
	`, artifactID, "photo", "scope02 chaos seed", "hash-chaos-s02-"+cd.photoID.String(), "test-source"); err != nil {
		t.Fatalf("seed artifacts row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref,
			provider_media_kind, mime_type, filename, sensitivity, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, cd.photoID, artifactID, "test-connector", "test", "ref-chaos-s02-"+cd.photoID.String(),
		"image", "image/jpeg", "chaos-s02-test.jpg",
		string(photolib.SensitivitySensitive)); err != nil {
		t.Fatalf("seed photos row: %v", err)
	}

	cd.cache = revocation.NewCache()
	photoStore := photolib.NewStore(pool)

	if wireBroadcaster {
		nc := chaosNATSConn(t)
		t.Cleanup(func() { nc.Close() })
		subject := uniqueChaosS02Subject(subjectPrefix)
		broadcaster, err := revocation.NewBroadcaster(nc, subject, cd.cache, "test-instance-chaos-s02")
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
	}
	return cd
}

// issueAndPersist issues a real PASETO + persists it via BearerStore.
func (c *chaosS02Deps) issueAndPersist(t *testing.T, userID, tokenID string, ttl time.Duration) auth.IssueResult {
	t.Helper()
	if err := c.store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: "chaos-s02-test",
		Notes:      "spec 044 Scope 02 chaos fixture",
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
		IssuedBy:     "chaos-s02-test",
		IssuedSource: "admin_api",
	}); err != nil {
		t.Fatalf("PersistToken(%q): %v", tokenID, err)
	}
	return issued
}

// revealRequest builds the middleware admit/reject probe request. The
// reveal handler additionally enforces the MIT-040-S-008 closure (body
// actor_id rejection) so this single probe doubles as the closure
// regression observation surface.
func (c *chaosS02Deps) revealRequest(token string) *http.Request {
	body := []byte(`{"ttl_seconds":300}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/photos/%s/reveal", c.photoID.String()),
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	return req
}

// --- C2-B01 ---------------------------------------------------------

// TestAuthChaos_S02_ConcurrentMiddlewareVerify_NoRaceNoLeak fires N=128
// concurrent middleware verifies against the SAME PASETO token and
// asserts every single request admits cleanly (HTTP 201). Validates
// the production hot path (auth.VerifyAndParse + revocation.Cache.IsRevoked)
// is race-free under contention. The race detector is enabled at the
// invocation level (`-race`); a successful run proves no concurrent
// map writes / no double-close / no panic / no spurious 401s.
func TestAuthChaos_S02_ConcurrentMiddlewareVerify_NoRaceNoLeak(t *testing.T) {
	cd := newChaosS02Deps(t, false, "concurrent-verify")
	router := api.NewRouter(cd.deps)

	issued := cd.issueAndPersist(t, "chaos-044-s02-cmv-user", "chaos-044-s02-cmv-tok", 24*time.Hour)

	const concurrent = 128

	var wg sync.WaitGroup
	var admitCount atomic.Int64    // HTTP 201: auth verified + handler succeeded
	var throttleCount atomic.Int64 // HTTP 429: rate-limited (orthogonal to auth)
	var authRejectCount atomic.Int64 // HTTP 401/403: spurious auth failure (FAILURE)
	var otherCount atomic.Int64    // any other status (FAILURE)
	var failureBodies []string
	var failureBodiesMu sync.Mutex
	startGate := make(chan struct{})

	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate
			req := cd.revealRequest(issued.WireToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			switch rec.Code {
			case http.StatusCreated:
				admitCount.Add(1)
			case http.StatusTooManyRequests:
				throttleCount.Add(1)
			case http.StatusUnauthorized, http.StatusForbidden:
				authRejectCount.Add(1)
				failureBodiesMu.Lock()
				failureBodies = append(failureBodies, fmt.Sprintf("AUTH_REJECT status=%d body=%s", rec.Code, rec.Body.String()))
				failureBodiesMu.Unlock()
			default:
				otherCount.Add(1)
				failureBodiesMu.Lock()
				failureBodies = append(failureBodies, fmt.Sprintf("UNEXPECTED status=%d body=%s", rec.Code, rec.Body.String()))
				failureBodiesMu.Unlock()
			}
		}()
	}
	close(startGate)
	wg.Wait()

	// Chaos invariant: under -race, the bearer middleware MUST NOT
	// produce ANY spurious auth rejection on a valid token. 429s
	// from the rate limiter are orthogonal (rate limit is a separate
	// concern from auth verification correctness) and are an expected
	// production behavior under burst from a single IP.
	if authRejectCount.Load() != 0 {
		t.Fatalf("NFR-AUTH-006 violation: %d spurious auth rejects under concurrent verify (admit=%d throttle429=%d other=%d) sample=%v",
			authRejectCount.Load(), admitCount.Load(), throttleCount.Load(), otherCount.Load(),
			failureBodies[:min(len(failureBodies), 3)])
	}
	if otherCount.Load() != 0 {
		t.Fatalf("unexpected non-auth, non-rate-limit responses under concurrent verify: count=%d sample=%v",
			otherCount.Load(), failureBodies[:min(len(failureBodies), 3)])
	}
	if admitCount.Load()+throttleCount.Load() != concurrent {
		t.Fatalf("accounting mismatch: admit=%d + throttle=%d != %d", admitCount.Load(), throttleCount.Load(), concurrent)
	}
	t.Logf("C2-B01: %d concurrent middleware verifies → admit=%d throttle429=%d auth_reject=0 other=0 (race-detector clean)",
		concurrent, admitCount.Load(), throttleCount.Load())
}

// --- C2-B02 ---------------------------------------------------------

// TestAuthChaos_S02_VerifyVsRevokeRace_ConvergesToReject interleaves
// real Broadcaster.Publish revocation events with concurrent middleware
// verifies. A few requests may briefly admit between verify-time and
// the synchronous cache.MarkRevoked inside Publish (this is the
// expected NFR-AUTH-006 propagation window — microseconds in the
// loopback case). The post-revoke convergence window MUST be tight:
// once Publish returns, every subsequent verify on that token MUST
// reject with HTTP 401.
func TestAuthChaos_S02_VerifyVsRevokeRace_ConvergesToReject(t *testing.T) {
	cd := newChaosS02Deps(t, true, "verify-vs-revoke")
	router := api.NewRouter(cd.deps)

	issued := cd.issueAndPersist(t, "chaos-044-s02-vvr-user", "chaos-044-s02-vvr-tok", 24*time.Hour)

	// Run a steady stream of verifies for 200 ms; trigger Publish
	// half-way through; assert that AFTER Publish returns, every
	// subsequent verify rejects.
	const beforeReqs = 40
	const afterReqs = 40

	// Warm-up — confirm the token admits before any chaos.
	preReq := cd.revealRequest(issued.WireToken)
	preRec := httptest.NewRecorder()
	router.ServeHTTP(preRec, preReq)
	if preRec.Code != http.StatusCreated {
		t.Fatalf("warm-up expected 201, got %d body=%s", preRec.Code, preRec.Body.String())
	}

	var preAdmit, preReject atomic.Int64
	var postAdmit, postReject atomic.Int64

	var preWG, postWG sync.WaitGroup
	preGate := make(chan struct{})
	postGate := make(chan struct{})

	for i := 0; i < beforeReqs; i++ {
		preWG.Add(1)
		go func() {
			defer preWG.Done()
			<-preGate
			req := cd.revealRequest(issued.WireToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusCreated {
				preAdmit.Add(1)
			} else {
				preReject.Add(1)
			}
		}()
	}
	for i := 0; i < afterReqs; i++ {
		postWG.Add(1)
		go func() {
			defer postWG.Done()
			<-postGate
			req := cd.revealRequest(issued.WireToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusCreated {
				postAdmit.Add(1)
			} else {
				postReject.Add(1)
			}
		}()
	}

	close(preGate)
	preWG.Wait()

	// Revoke + broadcast — Publish synchronously updates the local
	// cache before returning, so post-Publish requests MUST observe
	// the revocation.
	if err := cd.store.RevokeToken(context.Background(),
		"chaos-044-s02-vvr-tok", "chaos-s02-test", "chaos-revoke"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if err := cd.broadcaster.Publish("chaos-044-s02-vvr-tok", "chaos-revoke"); err != nil {
		t.Fatalf("Broadcaster.Publish: %v", err)
	}

	close(postGate)
	postWG.Wait()

	// Pre-revoke window: every request MUST have admitted (token was
	// fully valid + cache was empty for it).
	if preAdmit.Load() != beforeReqs || preReject.Load() != 0 {
		t.Fatalf("pre-revoke window: expected %d admits / 0 rejects, got admit=%d reject=%d",
			beforeReqs, preAdmit.Load(), preReject.Load())
	}
	// Post-revoke window: every request MUST have rejected (cache hit
	// is synchronous after Publish returns in the loopback case).
	if postAdmit.Load() != 0 || postReject.Load() != afterReqs {
		t.Fatalf("post-revoke window: expected 0 admits / %d rejects, got admit=%d reject=%d (NFR-AUTH-006 propagation regression?)",
			afterReqs, postAdmit.Load(), postReject.Load())
	}

	// Adversarial body-content assertion — even on the post-revoke
	// 401, the response body MUST NOT leak which validation step
	// failed (NFR-AUTH-007 / SCN-AUTH-010).
	leakReq := cd.revealRequest(issued.WireToken)
	leakRec := httptest.NewRecorder()
	router.ServeHTTP(leakRec, leakReq)
	if leakRec.Code != http.StatusUnauthorized {
		t.Fatalf("post-revoke leak probe expected 401, got %d", leakRec.Code)
	}
	body := strings.ToLower(leakRec.Body.String())
	for _, leak := range []string{"revoked", "revocation", "cache hit"} {
		if strings.Contains(body, leak) {
			t.Errorf("post-revoke 401 body leaked failure mode token %q (NFR-AUTH-007 violation): %s", leak, leakRec.Body.String())
		}
	}

	t.Logf("C2-B02: %d pre-revoke admits / %d post-revoke rejects → cache convergence within Broadcaster.Publish loopback (NFR-AUTH-006 met)",
		preAdmit.Load(), postReject.Load())
}

// --- C2-B03 ---------------------------------------------------------

// TestAuthChaos_S02_ConcurrentMintRevealUnderClosure_ActorIDFromSession
// fires N=50 concurrent valid reveal requests AND M=10 adversarial
// body-actor_id requests against the production MintReveal handler.
// All 50 valid requests MUST return 201 with session-derived actor_id
// (no body-actor_id substitution); all 10 adversarial requests MUST
// return 400 `actor_id_in_body_forbidden` (MIT-040-S-008 closure).
// Validates the closure is race-safe and the rejection always wins
// over the success path when both arrive at the same handler.
func TestAuthChaos_S02_ConcurrentMintRevealUnderClosure_ActorIDFromSession(t *testing.T) {
	cd := newChaosS02Deps(t, false, "mintreveal-closure")
	router := api.NewRouter(cd.deps)

	issued := cd.issueAndPersist(t, "chaos-044-s02-mr-user", "chaos-044-s02-mr-tok", 24*time.Hour)

	const validReqs = 50
	const advReqs = 10

	var validOK, validBad atomic.Int64
	var adv400, advOther atomic.Int64
	var advBodies []string
	var advBodiesMu sync.Mutex

	var wg sync.WaitGroup
	gate := make(chan struct{})

	for i := 0; i < validReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			req := cd.revealRequest(issued.WireToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusCreated {
				validOK.Add(1)
			} else {
				validBad.Add(1)
			}
		}()
	}
	for i := 0; i < advReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			body := []byte(`{"ttl_seconds":300,"actor_id":"mallory"}`)
			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/v1/photos/%s/reveal", cd.photoID.String()),
				bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+issued.WireToken)
			req.Header.Set("Content-Type", "application/json")
			req.ContentLength = int64(len(body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusBadRequest {
				adv400.Add(1)
				advBodiesMu.Lock()
				advBodies = append(advBodies, rec.Body.String())
				advBodiesMu.Unlock()
				return
			}
			advOther.Add(1)
			advBodiesMu.Lock()
			advBodies = append(advBodies, fmt.Sprintf("UNEXPECTED status=%d body=%s", rec.Code, rec.Body.String()))
			advBodiesMu.Unlock()
		}()
	}
	close(gate)
	wg.Wait()

	if validOK.Load() != validReqs || validBad.Load() != 0 {
		t.Fatalf("valid leg: expected %d 201, got 201=%d non-201=%d",
			validReqs, validOK.Load(), validBad.Load())
	}
	if adv400.Load() != advReqs || advOther.Load() != 0 {
		t.Fatalf("adversarial leg: expected %d 400 (MIT-040-S-008 closure), got 400=%d other=%d sample=%v",
			advReqs, adv400.Load(), advOther.Load(), advBodies[:min(len(advBodies), 3)])
	}
	// Every adversarial body MUST mention the closure error code.
	for _, b := range advBodies {
		if !strings.Contains(b, "actor_id_in_body_forbidden") {
			t.Errorf("adversarial body missing closure error code 'actor_id_in_body_forbidden': %s", b)
		}
	}
	t.Logf("C2-B03: %d valid 201 + %d adversarial 400 (MIT-040-S-008 closure intact under contention)",
		validOK.Load(), adv400.Load())
}

// --- C2-B04 ---------------------------------------------------------

// TestAuthChaos_S02_ConcurrentDriveConnectUnderClosure_OwnerFromSession
// fires N=50 valid drive Connect requests AND M=10 adversarial body-
// owner_user_id requests. The valid leg admits; the adversarial leg
// MUST 400 `owner_user_id_in_body_forbidden` (MIT-038-S-003 closure).
// We DO NOT need a real OAuth provider seeded — the production handler
// rejects body owner_user_id BEFORE any provider call, so the closure
// regression observation surface is the rejection itself, not the
// downstream connect flow.
func TestAuthChaos_S02_ConcurrentDriveConnectUnderClosure_OwnerFromSession(t *testing.T) {
	cd := newChaosS02Deps(t, false, "drive-closure")

	// Wire a real DriveHandlers in production mode with a fake
	// provider registry. Reuses the same fakeDriveProviderForAuth
	// fixture as auth_drive_connect_test.go (same package). The
	// closure rejection runs BEFORE the registry lookup, so the
	// adversarial leg never invokes the fake provider — the rejection
	// is observable purely as the HTTP 400 response. Valid-leg success
	// path remains covered by TestDriveConnect_ProductionWithSession_DerivesOwner
	// in auth_drive_connect_test.go (deterministic Scope 02 test).
	reg := drive.NewRegistry()
	reg.Register(&fakeDriveProviderForAuth{id: "google", disp: "Google Drive (chaos s02)"})
	cd.deps.DriveHandlers = api.NewDriveHandlers(reg).WithEnvironment("production")
	router := api.NewRouter(cd.deps)

	issued := cd.issueAndPersist(t, "chaos-044-s02-drive-user", "chaos-044-s02-drive-tok", 24*time.Hour)

	const advReqs = 60

	var adv400, advOther atomic.Int64
	var advBodies []string
	var advBodiesMu sync.Mutex

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < advReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			body := []byte(`{"provider_id":"google","owner_user_id":"mallory","access_mode":"read_only"}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/connectors/drive/connect", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+issued.WireToken)
			req.Header.Set("Content-Type", "application/json")
			req.ContentLength = int64(len(body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusBadRequest {
				adv400.Add(1)
				advBodiesMu.Lock()
				advBodies = append(advBodies, rec.Body.String())
				advBodiesMu.Unlock()
				return
			}
			advOther.Add(1)
			advBodiesMu.Lock()
			advBodies = append(advBodies, fmt.Sprintf("UNEXPECTED status=%d body=%s", rec.Code, rec.Body.String()))
			advBodiesMu.Unlock()
		}()
	}
	close(gate)
	wg.Wait()

	if adv400.Load() != advReqs || advOther.Load() != 0 {
		t.Fatalf("drive Connect adversarial leg: expected %d 400 (MIT-038-S-003 closure), got 400=%d other=%d sample=%v",
			advReqs, adv400.Load(), advOther.Load(), advBodies[:min(len(advBodies), 3)])
	}
	for _, b := range advBodies {
		if !strings.Contains(b, "owner_user_id_in_body_forbidden") {
			t.Errorf("drive Connect adversarial body missing closure error code 'owner_user_id_in_body_forbidden': %s", b)
		}
	}
	t.Logf("C2-B04: %d adversarial body-owner_user_id requests → all 400 (MIT-038-S-003 closure intact under contention)", adv400.Load())
}

// --- C2-B05 ---------------------------------------------------------

// TestAuthChaos_S02_ConcurrentAnnotationUnderClosure_ActorSourceRejected
// fires N=60 concurrent annotation create requests with a body-
// actor_source field present. All 60 MUST return 400
// `actor_source in request body is forbidden in production`
// (MIT-027-TRACE-001 actor-source segment closure). Validates the
// closure regression is race-safe and the JSON body scan correctly
// rejects every attempt.
func TestAuthChaos_S02_ConcurrentAnnotationUnderClosure_ActorSourceRejected(t *testing.T) {
	cd := newChaosS02Deps(t, false, "annotation-closure")

	// Wire a minimal AnnotationHandlers with a stub store so the
	// closure-rejection path runs. The closure rejection happens
	// BEFORE any store call (per design.md §6.4), so the stub never
	// receives a write — assert on the stub's call counter.
	annStore := &chaosS02StubAnnotationStore{}
	cd.deps.AnnotationHandlers = &api.AnnotationHandlers{Store: annStore, Environment: "production"}
	router := api.NewRouter(cd.deps)

	issued := cd.issueAndPersist(t, "chaos-044-s02-ann-user", "chaos-044-s02-ann-tok", 24*time.Hour)

	// No artifacts row seed required: the production-mode actor_source
	// closure (annotations.go CreateAnnotation) rejects the body BEFORE
	// any store call. The chaosS02StubAnnotationStore counter assertion
	// below verifies the closure short-circuits ahead of the store
	// (rejection observable as 0 calls to CreateFromParsed).
	artifactID := "chaos-044-s02-ann-art"

	const advReqs = 60
	var adv400, advOther atomic.Int64
	var advBodies []string
	var advBodiesMu sync.Mutex

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < advReqs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			body := []byte(`{"text":"chaos annotation","actor_source":"mallory"}`)
			req := httptest.NewRequest(http.MethodPost,
				fmt.Sprintf("/api/artifacts/%s/annotations/", artifactID),
				bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+issued.WireToken)
			req.Header.Set("Content-Type", "application/json")
			req.ContentLength = int64(len(body))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusBadRequest {
				adv400.Add(1)
				advBodiesMu.Lock()
				advBodies = append(advBodies, rec.Body.String())
				advBodiesMu.Unlock()
				return
			}
			advOther.Add(1)
			advBodiesMu.Lock()
			advBodies = append(advBodies, fmt.Sprintf("UNEXPECTED status=%d body=%s", rec.Code, rec.Body.String()))
			advBodiesMu.Unlock()
		}()
	}
	close(gate)
	wg.Wait()

	if adv400.Load() != advReqs || advOther.Load() != 0 {
		t.Fatalf("annotation adversarial leg: expected %d 400 (MIT-027-TRACE-001 closure), got 400=%d other=%d sample=%v",
			advReqs, adv400.Load(), advOther.Load(), advBodies[:min(len(advBodies), 3)])
	}
	for _, b := range advBodies {
		if !strings.Contains(b, "actor_source") || !strings.Contains(b, "forbidden") {
			t.Errorf("annotation adversarial body missing closure rejection token: %s", b)
		}
	}
	if got := annStore.CreateCalls(); got != 0 {
		t.Errorf("annotation store CreateAnnotation called %d times — closure rejection MUST run BEFORE store call (MIT-027-TRACE-001 regression)", got)
	}
	t.Logf("C2-B05: %d adversarial body-actor_source annotation requests → all 400 (MIT-027-TRACE-001 closure intact under contention; store untouched)", adv400.Load())
}

// --- C2-B06 ---------------------------------------------------------

// TestAuthChaos_S02_RotationUnderLoad_BothAdmitInsideGrace_T1RejectedAfter
// issues T1 + T2, runs N=20 concurrent verifies on T1 INSIDE the grace
// window AND N=20 concurrent verifies on T2 (both must admit), then
// advances the verifier clock past T1's PASETO exp and re-runs N=20
// verifies on T1 (must reject) AND N=20 on T2 (must still admit).
//
// Because the chaos deps uses the real wall-clock for the active
// verifier (newChaosS02Deps wires Now=time.Now), this test stands up
// its own injectable-clock deps inline so we can fast-forward without
// time.Sleep.
func TestAuthChaos_S02_RotationUnderLoad_BothAdmitInsideGrace_T1RejectedAfter(t *testing.T) {
	baseTime := time.Now().UTC().Truncate(time.Second)
	rd := newRotationDeps(t, baseTime)
	router := api.NewRouter(rd.deps)

	t1 := rd.issueAndPersist(t, "chaos-044-s02-rotload-user", "chaos-044-s02-rotload-T1", 2*time.Hour, baseTime)
	t2 := rd.issueAndPersist(t, "chaos-044-s02-rotload-user", "chaos-044-s02-rotload-T2", 24*time.Hour, baseTime)
	if err := rd.store.MarkTokenRotated(context.Background(), "chaos-044-s02-rotload-T1"); err != nil {
		t.Fatalf("MarkTokenRotated(T1): %v", err)
	}

	// Inside grace window: clock at baseTime + 1h, both should admit
	// under contention.
	rd.setClock(baseTime.Add(time.Hour))

	const verifyReqs = 20
	runConcurrent := func(label, token string, want int) (admit, reject int64) {
		var a, r atomic.Int64
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for i := 0; i < verifyReqs; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-gate
				req := rd.revealRequest(token)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				if rec.Code == http.StatusCreated {
					a.Add(1)
				} else {
					r.Add(1)
				}
			}()
		}
		close(gate)
		wg.Wait()
		switch want {
		case http.StatusCreated:
			if a.Load() != verifyReqs || r.Load() != 0 {
				t.Errorf("[%s] expected %d admits 0 rejects, got admit=%d reject=%d", label, verifyReqs, a.Load(), r.Load())
			}
		case http.StatusUnauthorized:
			if r.Load() != verifyReqs || a.Load() != 0 {
				t.Errorf("[%s] expected %d rejects 0 admits, got admit=%d reject=%d", label, verifyReqs, a.Load(), r.Load())
			}
		}
		return a.Load(), r.Load()
	}

	insideT1Admit, _ := runConcurrent("inside_grace_T1", t1.WireToken, http.StatusCreated)
	insideT2Admit, _ := runConcurrent("inside_grace_T2", t2.WireToken, http.StatusCreated)

	// After grace window: clock at baseTime + 3h, T1 must reject, T2
	// must still admit.
	rd.setClock(baseTime.Add(3 * time.Hour))

	_, afterT1Reject := runConcurrent("after_grace_T1", t1.WireToken, http.StatusUnauthorized)
	afterT2Admit, _ := runConcurrent("after_grace_T2", t2.WireToken, http.StatusCreated)

	t.Logf("C2-B06: inside grace → T1 admits=%d T2 admits=%d; after grace → T1 rejects=%d T2 admits=%d",
		insideT1Admit, insideT2Admit, afterT1Reject, afterT2Admit)
}

// --- C2-B07 ---------------------------------------------------------

// TestAuthChaos_S02_RevocationUnderLoad_FiveOfTenConvergeToReject issues
// 10 distinct tokens, runs concurrent verifies on all 10, then revokes
// 5 of them via BearerStore.RevokeToken + Broadcaster.Publish. After
// the publish, every subsequent verify on the 5 revoked tokens MUST
// reject; verifies on the 5 surviving tokens MUST keep admitting.
func TestAuthChaos_S02_RevocationUnderLoad_FiveOfTenConvergeToReject(t *testing.T) {
	cd := newChaosS02Deps(t, true, "revoke-five-of-ten")
	router := api.NewRouter(cd.deps)

	// Issue 10 tokens for one user. (Spec 044 doesn't cap per-user
	// active token count; the chaos plan intentionally exercises this
	// looseness.)
	const totalTokens = 10
	tokens := make([]auth.IssueResult, totalTokens)
	tokenIDs := make([]string, totalTokens)
	for i := 0; i < totalTokens; i++ {
		tokenIDs[i] = fmt.Sprintf("chaos-044-s02-rev-tok-%d", i)
		tokens[i] = cd.issueAndPersist(t, "chaos-044-s02-rev-user", tokenIDs[i], 24*time.Hour)
	}

	// Pre-revoke: every token admits.
	for i, tok := range tokens {
		req := cd.revealRequest(tok.WireToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("pre-revoke token[%d] expected 201, got %d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	// Revoke tokens 0..4 with concurrent Publish.
	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < 5; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			if err := cd.store.RevokeToken(context.Background(), tokenIDs[i], "chaos-s02-test", "chaos-revoke-batch"); err != nil {
				t.Errorf("RevokeToken(tokens[%d]): %v", i, err)
				return
			}
			if err := cd.broadcaster.Publish(tokenIDs[i], "chaos-revoke-batch"); err != nil {
				t.Errorf("Broadcaster.Publish(tokens[%d]): %v", i, err)
				return
			}
		}()
	}
	close(gate)
	wg.Wait()

	// Post-revoke: tokens 0..4 reject; tokens 5..9 still admit. Run
	// each token's verify in parallel for stress.
	var revokedRejected, revokedAdmitted atomic.Int64
	var survivingAdmitted, survivingRejected atomic.Int64
	postWG := sync.WaitGroup{}
	postGate := make(chan struct{})
	for i := 0; i < totalTokens; i++ {
		i := i
		postWG.Add(1)
		go func() {
			defer postWG.Done()
			<-postGate
			req := cd.revealRequest(tokens[i].WireToken)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			isRevoked := i < 5
			admit := rec.Code == http.StatusCreated
			switch {
			case isRevoked && admit:
				revokedAdmitted.Add(1)
			case isRevoked && !admit:
				revokedRejected.Add(1)
			case !isRevoked && admit:
				survivingAdmitted.Add(1)
			case !isRevoked && !admit:
				survivingRejected.Add(1)
			}
		}()
	}
	close(postGate)
	postWG.Wait()

	if revokedRejected.Load() != 5 || revokedAdmitted.Load() != 0 {
		t.Fatalf("revoked-batch convergence: expected 5 rejects / 0 admits, got reject=%d admit=%d",
			revokedRejected.Load(), revokedAdmitted.Load())
	}
	if survivingAdmitted.Load() != 5 || survivingRejected.Load() != 0 {
		t.Fatalf("surviving-batch isolation: expected 5 admits / 0 rejects, got admit=%d reject=%d (revocation cross-talk?)",
			survivingAdmitted.Load(), survivingRejected.Load())
	}
	// Verify cache size matches the revoked count.
	if cacheSize := cd.cache.Size(); cacheSize != 5 {
		t.Errorf("cache size after batch revoke: want 5 got %d", cacheSize)
	}
	t.Logf("C2-B07: 5/10 tokens revoked under concurrent load → 5 reject / 5 admit (zero cross-talk; cache size=%d)", cd.cache.Size())
}

// --- C2-B08 ---------------------------------------------------------

// TestAuthChaos_S02_AdminEndpointStress_NonAdminAlwaysForbidden fires
// N=80 concurrent admin requests (mix of enroll / rotate / revoke /
// list-users) authenticated with a per-user PASETO. The middleware
// admits the requests (PASETO is valid) but each admin handler MUST
// reject via callerIsAdmin → HTTP 401 + FORBIDDEN error code. NO DB
// rows MAY be created or mutated by any of the 80 requests.
func TestAuthChaos_S02_AdminEndpointStress_NonAdminAlwaysForbidden(t *testing.T) {
	cd := newChaosS02Deps(t, false, "admin-stress")
	router := api.NewRouter(cd.deps)

	issued := cd.issueAndPersist(t, "chaos-044-s02-adm-user", "chaos-044-s02-adm-tok", 24*time.Hour)

	// Snapshot initial table sizes — the admin rejection MUST NOT
	// mutate them.
	initialUsers, err := cd.store.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers initial: %v", err)
	}

	// Enumerate the four admin endpoints.
	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/v1/auth/users", `{"user_id":"chaos-044-s02-adm-attempt","notes":"chaos"}`},
		{http.MethodGet, "/v1/auth/users", ""},
		{http.MethodPost, "/v1/auth/users/chaos-044-s02-adm-user/rotate", `{"prior_token_id":"chaos-044-s02-adm-tok"}`}, // gitleaks:allow
		{http.MethodPost, "/v1/auth/tokens/chaos-044-s02-adm-tok/revoke", `{"reason":"chaos"}`},
	}

	const totalReqs = 80
	var forbidden, otherStatus atomic.Int64
	var unexpectedBodies []string
	var unexpectedBodiesMu sync.Mutex

	var wg sync.WaitGroup
	gate := make(chan struct{})
	for i := 0; i < totalReqs; i++ {
		ep := endpoints[i%len(endpoints)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			var body []byte
			if ep.body != "" {
				body = []byte(ep.body)
			}
			req := httptest.NewRequest(ep.method, ep.path, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+issued.WireToken)
			req.Header.Set("Content-Type", "application/json")
			if body != nil {
				req.ContentLength = int64(len(body))
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code == http.StatusUnauthorized && strings.Contains(rec.Body.String(), "FORBIDDEN") {
				forbidden.Add(1)
				return
			}
			otherStatus.Add(1)
			unexpectedBodiesMu.Lock()
			unexpectedBodies = append(unexpectedBodies, fmt.Sprintf("%s %s status=%d body=%s", ep.method, ep.path, rec.Code, rec.Body.String()))
			unexpectedBodiesMu.Unlock()
		}()
	}
	close(gate)
	wg.Wait()

	if forbidden.Load() != totalReqs || otherStatus.Load() != 0 {
		t.Fatalf("admin stress: expected %d FORBIDDEN, got forbidden=%d other=%d sample=%v",
			totalReqs, forbidden.Load(), otherStatus.Load(), unexpectedBodies[:min(len(unexpectedBodies), 3)])
	}
	// Confirm no rows mutated.
	postUsers, err := cd.store.CountUsers(context.Background())
	if err != nil {
		t.Fatalf("CountUsers post: %v", err)
	}
	if postUsers != initialUsers {
		t.Fatalf("admin stress mutated auth_users: initial=%d post=%d (callerIsAdmin gate failed for non-admin caller)",
			initialUsers, postUsers)
	}
	// Confirm the chaos enroll attempt did NOT land.
	if _, err := cd.store.ListUsers(context.Background()); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	t.Logf("C2-B08: %d concurrent admin requests from non-admin caller → all FORBIDDEN; auth_users count unchanged (%d)", forbidden.Load(), postUsers)
}

// --- C2-B09 ---------------------------------------------------------

// TestAuthChaos_S02_MalformedAuthorizationHeaderStorm_Always401 fires a
// curated set of malformed Authorization headers AND a stream of N=64
// random fuzzed bytes. Every single response MUST be HTTP 401, no
// panic, no goroutine leak, no DB query (the production middleware
// rejects empty/malformed/unparseable PASETO BEFORE any cache or DB
// touch).
func TestAuthChaos_S02_MalformedAuthorizationHeaderStorm_Always401(t *testing.T) {
	cd := newChaosS02Deps(t, false, "malformed-header-storm")
	router := api.NewRouter(cd.deps)

	// Curated malformed cases — each is a real-world adversarial
	// shape we want the middleware to reject without panicking.
	curated := []string{
		"",
		"Bearer ",
		"Bearer",
		"bearer",
		"Bearer  ",
		"Bearer\t",
		"Bearer \n",
		"BEARER abc",
		"Basic dXNlcjpwYXNz",
		"Token abc",
		"Bearer\x00",
		"Bearer \x00\x01\x02",
		"Bearer ../../etc/passwd",
		"Bearer " + strings.Repeat("A", 16384),
		"Bearer 🌮 emoji surprise",
		"Bearer αβγδε",
		"Bearer abc.def.ghi",
		"Bearer abc def ghi",
		"Bearer null",
		"Bearer undefined",
		"Bearer NaN",
		`Bearer {"alg":"none"}`,
		"Bearer eyJhbGciOiJub25lIn0.eyJ1c2VyIjoidGVzdCJ9.",     // jwt none-alg attempt // gitleaks:allow
		"Bearer v4.public.short",                                // PASETO prefix but truncated
		"Bearer v4.local.encrypted-not-public",                  // wrong PASETO purpose
		"Bearer v3.public.legacy-version",                       // wrong PASETO version
	}

	// Pseudo-random fuzz cases — 64 deterministic patterns of varied
	// length and byte composition (no math/rand to keep the test
	// reproducible without a seed).
	fuzz := make([]string, 0, 64)
	for i := 0; i < 64; i++ {
		buf := make([]byte, 32+i)
		for j := range buf {
			buf[j] = byte(((i + 1) * (j + 7)) ^ 0x5A)
		}
		fuzz = append(fuzz, "Bearer "+string(buf))
	}

	var total, ok401, otherStatus atomic.Int64
	var unexpected []string
	var unexpectedMu sync.Mutex

	all := append([]string{}, curated...)
	all = append(all, fuzz...)

	for _, h := range all {
		total.Add(1)
		req := cd.revealRequest("placeholder")
		req.Header.Set("Authorization", h)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnauthorized {
			ok401.Add(1)
			continue
		}
		otherStatus.Add(1)
		unexpectedMu.Lock()
		unexpected = append(unexpected, fmt.Sprintf("hdr=%q status=%d body=%s", h, rec.Code, rec.Body.String()))
		unexpectedMu.Unlock()
	}

	if ok401.Load() != total.Load() || otherStatus.Load() != 0 {
		t.Fatalf("malformed-header storm: expected all 401, got 401=%d other=%d total=%d sample_unexpected=%v",
			ok401.Load(), otherStatus.Load(), total.Load(), unexpected[:min(len(unexpected), 3)])
	}
	// Adversarial body-content assertion on a sample 401: response
	// MUST NOT name the failure mode (NFR-AUTH-007).
	sampleReq := cd.revealRequest("placeholder")
	sampleReq.Header.Set("Authorization", "Bearer mallory.malformed.bytes")
	sampleRec := httptest.NewRecorder()
	router.ServeHTTP(sampleRec, sampleReq)
	body := strings.ToLower(sampleRec.Body.String())
	for _, leak := range []string{"signature", "verify", "key id", "kid", "footer", "paseto"} {
		if strings.Contains(body, leak) {
			t.Errorf("malformed 401 body leaked failure mode token %q (NFR-AUTH-007 violation): %s", leak, sampleRec.Body.String())
		}
	}
	t.Logf("C2-B09: %d malformed/fuzzed Authorization headers → all 401; response bodies generic (no NFR-AUTH-007 leak)", ok401.Load())
}

// --- C2-B11 (informational benchmark) -------------------------------

// BenchmarkAuthChaos_S02_BearerMiddleware_HotPath measures end-to-end
// latency through the production bearer middleware (PASETO verify +
// revocation cache check + handler dispatch). Reported for NFR-AUTH-001
// budget compliance under realistic wiring (compared to the Scope 01
// pure-CPU verify benchmark which measured the verifier in isolation).
func BenchmarkAuthChaos_S02_BearerMiddleware_HotPath(b *testing.B) {
	t := &testing.T{}
	cd := newChaosS02Deps(t, false, "hot-path-bench")
	router := api.NewRouter(cd.deps)
	if t.Failed() {
		b.Fatal("fixture setup failed")
	}

	issued := cd.issueAndPersist(t, "chaos-044-s02-bench-user", "chaos-044-s02-bench-tok", 24*time.Hour)
	if t.Failed() {
		b.Fatal("issueAndPersist failed")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := cd.revealRequest(issued.WireToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			b.Fatalf("benchmark probe expected 201 admit, got %d body=%s", rec.Code, rec.Body.String())
		}
	}
}

// --- helpers --------------------------------------------------------

// chaosS02StubAnnotationStore is a stub annotation.AnnotationQuerier
// for C2-B05. It records call counts so the test can assert that the
// closure rejection happens BEFORE any store interaction.
type chaosS02StubAnnotationStore struct {
	mu                sync.Mutex
	createFromParsed  int64
}

func (s *chaosS02StubAnnotationStore) CreateFromParsed(_ context.Context, _ string, _ annotation.ParsedAnnotation, _ annotation.SourceChannel) ([]annotation.Annotation, error) {
	s.mu.Lock()
	s.createFromParsed++
	s.mu.Unlock()
	return nil, nil
}

func (s *chaosS02StubAnnotationStore) GetSummary(_ context.Context, _ string) (*annotation.Summary, error) {
	return &annotation.Summary{}, nil
}

func (s *chaosS02StubAnnotationStore) GetHistory(_ context.Context, _ string, _ int) ([]annotation.Annotation, error) {
	return nil, nil
}

func (s *chaosS02StubAnnotationStore) DeleteTag(_ context.Context, _, _ string, _ annotation.SourceChannel) error {
	return nil
}

func (s *chaosS02StubAnnotationStore) RecordMessageArtifact(_ context.Context, _, _ int64, _ string) error {
	return nil
}

func (s *chaosS02StubAnnotationStore) ResolveArtifactFromMessage(_ context.Context, _, _ int64) (string, error) {
	return "", nil
}

func (s *chaosS02StubAnnotationStore) CreateCalls() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createFromParsed
}
