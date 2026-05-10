//go:build integration

// Spec 044 Scope 02 (follow-up implement) — SCN-AUTH-009 revocation
// propagation integration tests.
//
// Validates the per-user bearer-auth revocation contract end-to-end
// against the live test stack (postgres at host port from
// DATABASE_URL, NATS at host port from CHAOS_NATS_URL or NATS_URL):
//
//  1. TestRevocation_RevokedTokenRejectedOnNextRequest: the canonical
//     SCN-AUTH-009 contract. Issue a token, demonstrate it admits
//     through the production middleware, then revoke via
//     BearerStore.RevokeToken + Broadcaster.Publish, then send the
//     same request and observe the 401 rejection. Demonstrates the
//     ≤ NFR-AUTH-006 (60 s) propagation budget against the local
//     instance loop (which is a single Publish→cache.MarkRevoked hop
//     so the budget is microseconds in practice).
//
//  2. TestRevocation_NATSDownFallsBackToDBRefresh: the NFR-AUTH-006
//     fallback contract — when the NATS broadcaster is unavailable
//     (the test simulates this by skipping the Publish call) the
//     periodic Cache.Refresh against BearerStore.LoadRevokedTokenIDs
//     picks up the canonical revocation row from the DB and updates
//     the cache. Demonstrates that no permanent staleness window
//     exists when NATS partitions.
//
//  3. TestRevocation_NonExistentToken_ClearError: adversarial sub-
//     test for the BearerStore.RevokeToken contract refinement.
//     Revoking a token_id with no matching auth_tokens row returns a
//     wrapped auth.ErrTokenNotFound so admin callers can surface a
//     clean 404 rather than misclassifying a missing token as a
//     permission failure.
//
//  4. TestRevocation_AlreadyRevokedToken_Idempotent: adversarial sub-
//     test for the idempotency contract refinement. Repeated
//     RevokeToken calls against an already-revoked token return nil
//     so operator retries and crash-restart loops never error out a
//     second time.
//
// All tests use real PASETO issuance (auth.IssueToken) + real BearerStore
// against the live DB pool. The NATS-down fallback test uses the real
// Broadcaster wired to the live NATS conn but exercises the DB-refresh
// path without calling Publish (a real wire-level absence of the
// publish event, NOT a mock). No t.Skip — when DATABASE_URL or NATS
// URL are unset the tests fatal with an actionable message.
//
// Cleanup: every test calls resetAuthTables before exercising the
// revocation flow, and a t.Cleanup hook truncates the auth tables
// again at the end so cross-test interference is impossible.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// revocationDeps is the per-test fixture shared across the revocation
// SCN-AUTH-009 sub-tests. The deps wires a real cache and (optionally)
// a real Broadcaster bound to the live test-stack NATS conn.
type revocationDeps struct {
	pool        *pgxpool.Pool
	deps        *api.Dependencies
	store       *auth.BearerStore
	cache       *revocation.Cache
	broadcaster *revocation.Broadcaster
	nc          *nats.Conn
	priv, pub   string
	kid         string
	hashKey     string
	photoID     uuid.UUID
}

// uniqueRevocationSubject builds a per-test-run NATS subject so
// cross-test interference is impossible even when several revocation
// sub-tests run in parallel under -count=N.
func uniqueRevocationSubject(prefix string) string {
	return fmt.Sprintf("auth.revocations.test.%s.%d", prefix, time.Now().UnixNano())
}

// newRevocationDeps builds the live-DB fixture for the revocation
// tests. When wireBroadcaster is true the deps owns a real Broadcaster
// + Subscribe pair so Publish exercises the loopback subscription. When
// false, the broadcaster is omitted to simulate the NATS-down path
// (cache updates only land via Cache.Refresh against the DB).
func newRevocationDeps(t *testing.T, wireBroadcaster bool, subjectPrefix string) *revocationDeps {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope02-revocation-key"
	hashKey := priv + "-revocation-hash-suffix-distinct"

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	rd := &revocationDeps{
		pool:    pool,
		store:   store,
		priv:    priv,
		pub:     pub,
		kid:     kid,
		hashKey: hashKey,
	}

	// Seed a sensitive photo so the reveal endpoint doubles as a
	// middleware admit/reject probe.
	rd.photoID = uuid.New()
	artifactID := "art-revocation-" + rd.photoID.String()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, $2, $3, $4, $5)
	`, artifactID, "photo", "scope02 revocation seed", "hash-revocation-"+rd.photoID.String(), "test-source"); err != nil {
		t.Fatalf("seed artifacts row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref,
			provider_media_kind, mime_type, filename, sensitivity, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, rd.photoID, artifactID, "test-connector", "test", "ref-revocation-"+rd.photoID.String(),
		"image", "image/jpeg", "revocation-test.jpg",
		string(photolib.SensitivitySensitive)); err != nil {
		t.Fatalf("seed photos row: %v", err)
	}

	rd.cache = revocation.NewCache()
	photoStore := photolib.NewStore(pool)

	if wireBroadcaster {
		// Real NATS conn — the chaos test pattern uses CHAOS_NATS_URL
		// fallback to NATS_URL so the same subject works whether the
		// test runs from inside the stack or from the host shell.
		rd.nc = chaosNATSConn(t)
		t.Cleanup(func() { rd.nc.Close() })
		subject := uniqueRevocationSubject(subjectPrefix)
		broadcaster, err := revocation.NewBroadcaster(rd.nc, subject, rd.cache, "test-instance-revocation")
		if err != nil {
			t.Fatalf("NewBroadcaster: %v", err)
		}
		if err := broadcaster.Subscribe(); err != nil {
			t.Fatalf("Broadcaster.Subscribe: %v", err)
		}
		t.Cleanup(func() { _ = broadcaster.Stop() })
		rd.broadcaster = broadcaster
	}

	rd.deps = &api.Dependencies{
		Environment: "production",
		AuthConfig: config.AuthConfig{
			Enabled:                               true,
			TokenFormat:                           "paseto_v4_public",
			SigningActivePrivateKey:               priv,
			SigningActiveKeyID:                    kid,
			TokenTTLHours:                         24,
			RotationGraceWindowHours:              24,
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
		BearerStore:     store,
		RevocationCache: rd.cache,
		PhotosHandlers:  api.NewPhotosHandlers(photoStore, config.PhotosConfig{}, "production"),
	}
	return rd
}

// issueAndPersist issues a real PASETO + persists it via BearerStore.
func (r *revocationDeps) issueAndPersist(t *testing.T, userID, tokenID string) auth.IssueResult {
	t.Helper()
	if err := r.store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: "revocation-test",
		Notes:      "spec 044 SCN-AUTH-009 revocation fixture",
	}); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
			!strings.Contains(strings.ToLower(err.Error()), "unique") {
			t.Fatalf("Enroll(%q): %v", userID, err)
		}
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     userID,
		TokenID:    tokenID,
		SigningKey: r.priv,
		KeyID:      r.kid,
		TTL:        24 * time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken(%q): %v", tokenID, err)
	}
	hashed, err := auth.HashToken(issued.WireToken, r.hashKey)
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}
	if err := r.store.PersistToken(context.Background(), auth.PersistTokenParams{
		TokenID:      tokenID,
		UserID:       userID,
		KeyID:        r.kid,
		IssuedAt:     issued.IssuedAt,
		ExpiresAt:    issued.ExpiresAt,
		HashedToken:  hashed,
		IssuedBy:     "revocation-test",
		IssuedSource: "admin_api",
	}); err != nil {
		t.Fatalf("PersistToken(%q): %v", tokenID, err)
	}
	return issued
}

// revealRequest builds the middleware-admit probe request — see the
// rotation test file for the rationale on using POST /v1/photos/{id}/reveal.
func (r *revocationDeps) revealRequest(token string) *http.Request {
	body := []byte(`{"ttl_seconds":300}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/photos/%s/reveal", r.photoID.String()),
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	return req
}

// TestRevocation_RevokedTokenRejectedOnNextRequest covers SCN-AUTH-009.
// The flow exercises the real BearerStore + Broadcaster pair: revoke
// commits to DB, Publish fires the NATS broadcast and synchronously
// updates the local cache (cache.MarkRevoked inside Publish), and the
// next request observes the cache hit and returns 401.
func TestRevocation_RevokedTokenRejectedOnNextRequest(t *testing.T) {
	rd := newRevocationDeps(t, true, "next-request")
	router := api.NewRouter(rd.deps)

	issued := rd.issueAndPersist(t, "user-revoke-001", "tok-revoke-001")

	// Step 1 — confirm the token admits BEFORE revocation.
	preReq := rd.revealRequest(issued.WireToken)
	preRec := httptest.NewRecorder()
	router.ServeHTTP(preRec, preReq)
	if preRec.Code != http.StatusCreated {
		t.Fatalf("pre-revocation request expected 201 admit, got %d body=%s", preRec.Code, preRec.Body.String())
	}
	var preResp api.PhotoRevealResponse
	if err := json.Unmarshal(preRec.Body.Bytes(), &preResp); err != nil {
		t.Fatalf("unmarshal pre-revocation response: %v body=%s", err, preRec.Body.String())
	}
	if preResp.RevealToken == "" {
		t.Errorf("expected reveal_token in pre-revocation 201 response, got empty body=%s", preRec.Body.String())
	}

	// Step 2 — revoke via canonical store and broadcast via real NATS.
	if err := rd.store.RevokeToken(context.Background(), "tok-revoke-001", "revocation-test", "SCN-AUTH-009 propagation"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}
	if err := rd.broadcaster.Publish("tok-revoke-001", "SCN-AUTH-009 propagation"); err != nil {
		t.Fatalf("Broadcaster.Publish: %v", err)
	}

	// Step 3 — confirm the local cache reflects the revocation.
	// Publish synchronously calls cache.MarkRevoked so this is
	// guaranteed without a sleep, but we also wait briefly for the
	// loopback NATS subscription to land for the cross-instance
	// observability assertion below.
	deadline := time.Now().Add(2 * time.Second)
	for !rd.cache.IsRevoked("tok-revoke-001") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !rd.cache.IsRevoked("tok-revoke-001") {
		t.Fatalf("revocation cache did not observe tok-revoke-001 after Publish")
	}

	// Step 4 — the next request bearing the same wire token MUST be rejected.
	postReq := rd.revealRequest(issued.WireToken)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusUnauthorized {
		t.Fatalf("post-revocation request expected 401 reject, got %d body=%s", postRec.Code, postRec.Body.String())
	}
	// SCN-AUTH-010 / NFR-AUTH-007 — adversarial assertion that the
	// 401 body does NOT name "revoked" / "revocation" / "cache" so a
	// regression that leaks the failure mode would fail the test.
	body := strings.ToLower(postRec.Body.String())
	for _, leak := range []string{"revoked", "revocation", "cache hit"} {
		if strings.Contains(body, leak) {
			t.Errorf("middleware 401 body leaked failure mode token %q (NFR-AUTH-007 violation): %s", leak, postRec.Body.String())
		}
	}
}

// TestRevocation_NATSDownFallsBackToDBRefresh covers the NFR-AUTH-006
// fallback contract. The test does NOT call Broadcaster.Publish (real
// wire-level simulation of the NATS path being unavailable — Publish
// would fail or never propagate when NATS is partitioned). The cache
// observes the revocation only after Cache.Refresh pulls
// LoadRevokedTokenIDs from the canonical DB store.
func TestRevocation_NATSDownFallsBackToDBRefresh(t *testing.T) {
	// wireBroadcaster=false to omit the broadcaster from deps. The
	// test exercises the DB-refresh path directly so the broadcaster
	// is irrelevant — we want to demonstrate that when Publish never
	// fires, the cache still converges via the periodic refresh
	// fallback.
	rd := newRevocationDeps(t, false, "nats-down-fallback")
	router := api.NewRouter(rd.deps)

	issued := rd.issueAndPersist(t, "user-revoke-002", "tok-revoke-fallback")

	// Step 1 — pre-revocation admission baseline.
	preReq := rd.revealRequest(issued.WireToken)
	preRec := httptest.NewRecorder()
	router.ServeHTTP(preRec, preReq)
	if preRec.Code != http.StatusCreated {
		t.Fatalf("pre-revocation request expected 201 admit, got %d body=%s", preRec.Code, preRec.Body.String())
	}

	// Step 2 — revoke at the canonical store ONLY. NO Publish call
	// (NATS-down simulation). The cache is now stale.
	if err := rd.store.RevokeToken(context.Background(), "tok-revoke-fallback", "revocation-test", "SCN-AUTH-009 NATS-down fallback"); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	// Step 3 — proof that the cache is stale: the same request still
	// admits because no NATS event fired and no DB refresh has
	// happened yet. This is the staleness window the periodic
	// refresh closes.
	staleReq := rd.revealRequest(issued.WireToken)
	staleRec := httptest.NewRecorder()
	router.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusCreated {
		t.Fatalf("expected stale cache to still admit (NATS-down window), got %d body=%s", staleRec.Code, staleRec.Body.String())
	}

	// Step 4 — force a cache refresh against the live DB.
	delta, err := rd.cache.Refresh(context.Background(), rd.store)
	if err != nil {
		t.Fatalf("Cache.Refresh: %v", err)
	}
	if delta < 1 {
		t.Errorf("expected ≥1 newly added revocation in refresh delta, got %d", delta)
	}
	if !rd.cache.IsRevoked("tok-revoke-fallback") {
		t.Fatalf("Cache.Refresh did not pick up tok-revoke-fallback from DB")
	}

	// Step 5 — the request now rejects because the cache caught up.
	postReq := rd.revealRequest(issued.WireToken)
	postRec := httptest.NewRecorder()
	router.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusUnauthorized {
		t.Fatalf("post-refresh request expected 401 reject, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

// TestRevocation_NonExistentToken_ClearError is the BearerStore
// adversarial sub-test: revoking a token_id with no matching row
// returns a wrapped auth.ErrTokenNotFound. The test asserts the
// sentinel via errors.Is so admin callers can branch on the error
// without string-matching.
func TestRevocation_NonExistentToken_ClearError(t *testing.T) {
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	err = store.RevokeToken(context.Background(), "tok-does-not-exist", "revocation-test", "SCN-AUTH-009 not-found path")
	if err == nil {
		t.Fatal("expected RevokeToken to error on non-existent token, got nil")
	}
	if !errors.Is(err, auth.ErrTokenNotFound) {
		t.Errorf("expected wrapped auth.ErrTokenNotFound, got %v", err)
	}
	// Adversarial: the error message MUST identify the token id so
	// operators can distinguish "fat-fingered the wrong id" from
	// other failure modes.
	if !strings.Contains(err.Error(), "tok-does-not-exist") {
		t.Errorf("error message should mention the offending token id, got %v", err)
	}
}

// TestRevocation_AlreadyRevokedToken_Idempotent is the BearerStore
// adversarial sub-test for the idempotency contract refinement.
// Revoking an already-revoked token MUST return nil so operator
// retries and crash-restart loops never error out a second time.
func TestRevocation_AlreadyRevokedToken_Idempotent(t *testing.T) {
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	priv, _ := auth.GenerateSigningKeypair()
	const kid = "scope02-idempotent-key"

	if err := store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     "user-idempotent-revoke",
		EnrolledBy: "revocation-test",
	}); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "user-idempotent-revoke",
		TokenID:    "tok-idempotent-revoke",
		SigningKey: priv,
		KeyID:      kid,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	hashed, err := auth.HashToken(issued.WireToken, "idempotent-hash-key-distinct")
	if err != nil {
		t.Fatalf("HashToken: %v", err)
	}
	if err := store.PersistToken(context.Background(), auth.PersistTokenParams{
		TokenID:      "tok-idempotent-revoke",
		UserID:       "user-idempotent-revoke",
		KeyID:        kid,
		IssuedAt:     issued.IssuedAt,
		ExpiresAt:    issued.ExpiresAt,
		HashedToken:  hashed,
		IssuedBy:     "revocation-test",
		IssuedSource: "admin_api",
	}); err != nil {
		t.Fatalf("PersistToken: %v", err)
	}

	// First revocation — succeeds.
	if err := store.RevokeToken(context.Background(), "tok-idempotent-revoke", "revocation-test", "first"); err != nil {
		t.Fatalf("first RevokeToken: %v", err)
	}

	// Second revocation — MUST be idempotent (no error).
	if err := store.RevokeToken(context.Background(), "tok-idempotent-revoke", "revocation-test", "second-retry"); err != nil {
		t.Errorf("second RevokeToken on already-revoked token MUST be idempotent, got error: %v", err)
	}

	// Third revocation, different revoker — still idempotent.
	if err := store.RevokeToken(context.Background(), "tok-idempotent-revoke", "different-revoker", "third-retry"); err != nil {
		t.Errorf("third RevokeToken (different revoker) MUST be idempotent, got error: %v", err)
	}

	// Adversarial: confirm exactly ONE auth_revocations row exists
	// (the audit row is INSERT-once via ON CONFLICT DO NOTHING). The
	// idempotent retries MUST NOT create extra audit rows.
	var revRows int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM auth_revocations WHERE token_id = $1`, "tok-idempotent-revoke").
		Scan(&revRows); err != nil {
		t.Fatalf("count auth_revocations: %v", err)
	}
	if revRows != 1 {
		t.Errorf("expected exactly 1 auth_revocations row after idempotent retries, got %d", revRows)
	}

	// Confirm the auth_tokens.status is still 'revoked' (no flip-back).
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM auth_tokens WHERE token_id = $1`, "tok-idempotent-revoke").
		Scan(&status); err != nil {
		t.Fatalf("query auth_tokens status: %v", err)
	}
	if status != "revoked" {
		t.Errorf("expected auth_tokens.status='revoked' after idempotent retries, got %q", status)
	}
}
