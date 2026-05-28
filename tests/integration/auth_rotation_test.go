//go:build integration

// Spec 044 Scope 02 (follow-up implement) — SCN-AUTH-004 rotation
// grace-window integration tests.
//
// Validates the per-user bearer-auth rotation contract end-to-end
// against the live test stack (postgres at host port from
// DATABASE_URL):
//
//  1. TestRotation_GraceWindow_BothTokensValid: a prior token T1 and a
//     freshly-rotated token T2 BOTH admit through the production
//     bearer-auth middleware while T1's PASETO exp claim is still in
//     the future per the verifier's clock. Demonstrates that
//     MarkTokenRotated is informational and does NOT block the prior
//     token during the grace window.
//
//  2. TestRotation_AfterGraceWindow_OldTokenRejected: when the
//     verifier's clock advances past T1's PASETO exp, T1 surfaces an
//     ErrTokenExpired-driven 401 while T2 continues to admit. The
//     rejection happens inside auth.VerifyAndParse, not inside the
//     handler, so the middleware response body MUST NOT name the
//     failure mode (NFR-AUTH-007 / SCN-AUTH-010).
//
//  3. TestRotation_AdminEndpoint_RejectsNonAdminCaller: the admin
//     POST /v1/auth/users/{user_id}/rotate route is admin-gated via
//     AuthAdminHandlers.callerIsAdmin. A per-user PASETO session
//     (Source = SessionSourcePerUserToken) MUST NOT be admitted as
//     admin in production. The middleware lets the request through
//     because the PASETO is valid, then the handler returns 401 with
//     the FORBIDDEN error code so the caller can distinguish auth
//     failure from authorization failure.
//
// All tests use real PASETO issuance (auth.IssueToken) + real BearerStore
// against the live DB pool from authTestPool. No mocks. No t.Skip — when
// DATABASE_URL is unset the test fatals with an actionable message per
// the no-skip precedent set by spec 043 and reaffirmed in spec 044
// Scope 01.
//
// Cleanup: every test calls resetAuthTables before exercising the
// rotation flow, and a t.Cleanup hook truncates the auth tables again
// at the end so cross-test interference is impossible.
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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// rotationDeps is the per-test fixture shared across the rotation
// SCN-AUTH-004 sub-tests. The deps embeds a clock-injectable
// AuthVerifyOptions.Now so test sub-cases can advance "wall time"
// past T1's exp without sleeping.
type rotationDeps struct {
	pool       *pgxpool.Pool
	deps       *api.Dependencies
	store      *auth.BearerStore
	cache      *revocation.Cache
	priv, pub  string
	kid        string
	hashKey    string
	photoID    uuid.UUID
	clockMu    sync.Mutex
	clockValue time.Time
}

// clock returns the current injected wall-clock for the verifier.
func (r *rotationDeps) clock() time.Time {
	r.clockMu.Lock()
	defer r.clockMu.Unlock()
	return r.clockValue
}

// setClock advances the injected clock for subsequent verifier calls.
func (r *rotationDeps) setClock(t time.Time) {
	r.clockMu.Lock()
	defer r.clockMu.Unlock()
	r.clockValue = t
}

// newRotationDeps builds the live-DB fixture for the rotation grace-
// window tests. Seeds a sensitive photo so the reveal endpoint can
// serve as a middleware-admission probe (it returns 201 on session-
// derived actor with no body smuggling; the middleware admit/reject
// outcome is observable as the HTTP status code).
func newRotationDeps(t *testing.T, baseTime time.Time) *rotationDeps {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)
	t.Cleanup(func() { resetAuthTables(t, pool) })

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope02-rotation-key"
	hashKey := priv + "-rotation-hash-suffix-distinct"

	store, err := auth.NewBearerStore(pool)
	if err != nil {
		t.Fatalf("NewBearerStore: %v", err)
	}

	rd := &rotationDeps{
		pool:       pool,
		store:      store,
		priv:       priv,
		pub:        pub,
		kid:        kid,
		hashKey:    hashKey,
		clockValue: baseTime,
	}

	// Seed an artifact + sensitive photo so MintReveal returns 201
	// when the middleware admits and the rejection-mode signal can be
	// distinguished from a 404. The seed pattern mirrors
	// productionAuthDepsForReveal in auth_mintreveal_test.go.
	rd.photoID = uuid.New()
	artifactID := "art-rotation-" + rd.photoID.String()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, $2, $3, $4, $5)
	`, artifactID, "photo", "scope02 rotation seed", "hash-rotation-"+rd.photoID.String(), "test-source"); err != nil {
		t.Fatalf("seed artifacts row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref,
			provider_media_kind, mime_type, filename, sensitivity, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, rd.photoID, artifactID, "test-connector", "test", "ref-rotation-"+rd.photoID.String(),
		"image", "image/jpeg", "rotation-test.jpg",
		string(photolib.SensitivitySensitive)); err != nil {
		t.Fatalf("seed photos row: %v", err)
	}

	rd.cache = revocation.NewCache()
	photoStore := photolib.NewStore(pool)

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

	rd.deps = &api.Dependencies{
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
			Now:                rd.clock,
		},
		BearerStore:       store,
		RevocationCache:   rd.cache,
		AuthAdminHandlers: authAdmin,
		PhotosHandlers:    api.NewPhotosHandlers(photoStore, config.PhotosConfig{}, "production"),
	}
	return rd
}

// issueAndPersist issues a real PASETO token bound to userID with the
// supplied TTL and persists it via BearerStore so the admin surface
// observes a row that can be subsequently rotated/revoked.
func (r *rotationDeps) issueAndPersist(t *testing.T, userID, tokenID string, ttl time.Duration, baseTime time.Time) auth.IssueResult {
	t.Helper()
	if err := r.store.Enroll(context.Background(), auth.EnrollUserParams{
		UserID:     userID,
		EnrolledBy: "rotation-test",
		Notes:      "spec 044 SCN-AUTH-004 rotation grace-window fixture",
	}); err != nil {
		// Enroll is idempotent here only when the test exercises the
		// same user twice; otherwise surface the error.
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
		TTL:        ttl,
		Issuer:     "smackerel",
		Now:        func() time.Time { return baseTime },
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
		IssuedBy:     "rotation-test",
		IssuedSource: "admin_api",
	}); err != nil {
		t.Fatalf("PersistToken(%q): %v", tokenID, err)
	}
	return issued
}

// revealRequest builds a POST /v1/photos/{id}/reveal request bound to
// the supplied bearer token. The body contains only ttl_seconds — no
// actor_id smuggling, no header smuggling — so the response code
// reflects the middleware admission outcome (201 admitted, 401
// rejected).
func (r *rotationDeps) revealRequest(token string) *http.Request {
	body := []byte(`{"ttl_seconds":300}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/photos/%s/reveal", r.photoID.String()),
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	return req
}

// TestRotation_GraceWindow_BothTokensValid covers SCN-AUTH-004 happy
// path. Issues T1 with a 2-hour TTL (representing the grace window
// budget) and T2 with the full 24-hour TTL after marking T1 as rotated.
// With the verifier clock set to baseTime+1h, BOTH tokens admit
// through the production middleware.
func TestRotation_GraceWindow_BothTokensValid(t *testing.T) {
	baseTime := time.Now().UTC().Truncate(time.Second)
	rd := newRotationDeps(t, baseTime)
	router := api.NewRouter(rd.deps)

	t1 := rd.issueAndPersist(t, "user-rotation-001", "tok-rotation-T1", 2*time.Hour, baseTime)
	t2 := rd.issueAndPersist(t, "user-rotation-001", "tok-rotation-T2", 24*time.Hour, baseTime)

	// Mark T1 as rotated to simulate the admin Rotate flow having run.
	if err := rd.store.MarkTokenRotated(context.Background(), "tok-rotation-T1"); err != nil {
		t.Fatalf("MarkTokenRotated(T1): %v", err)
	}

	// Set the verifier clock to baseTime + 1 hour — strictly inside
	// T1's 2-hour grace window so T1's PASETO exp has not been
	// crossed.
	rd.setClock(baseTime.Add(time.Hour))

	for _, tc := range []struct {
		name  string
		token string
	}{
		{"T1_inside_grace_window_admits", t1.WireToken},
		{"T2_freshly_rotated_admits", t2.WireToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := rd.revealRequest(tc.token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201 admit, got %d body=%s", rec.Code, rec.Body.String())
			}
			var resp api.PhotoRevealResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal reveal response: %v body=%s", err, rec.Body.String())
			}
			if resp.RevealToken == "" {
				t.Errorf("expected reveal_token in 201 response, got empty body=%s", rec.Body.String())
			}
		})
	}
}

// TestRotation_AfterGraceWindow_OldTokenRejected covers the negative
// half of SCN-AUTH-004: once the verifier clock advances past T1's
// PASETO exp, the middleware refuses T1 with a generic 401 while T2
// continues to admit. The rejection MUST NOT name the failure mode in
// the response body (NFR-AUTH-007 / SCN-AUTH-010).
func TestRotation_AfterGraceWindow_OldTokenRejected(t *testing.T) {
	baseTime := time.Now().UTC().Truncate(time.Second)
	rd := newRotationDeps(t, baseTime)
	router := api.NewRouter(rd.deps)

	t1 := rd.issueAndPersist(t, "user-rotation-002", "tok-rotation-T1b", 2*time.Hour, baseTime)
	t2 := rd.issueAndPersist(t, "user-rotation-002", "tok-rotation-T2b", 24*time.Hour, baseTime)
	if err := rd.store.MarkTokenRotated(context.Background(), "tok-rotation-T1b"); err != nil {
		t.Fatalf("MarkTokenRotated(T1b): %v", err)
	}

	// Advance clock to baseTime + 3 hours — strictly past T1's 2-hour
	// PASETO exp + the 1-minute clock-skew tolerance.
	rd.setClock(baseTime.Add(3 * time.Hour))

	t.Run("T1_after_grace_window_rejected", func(t *testing.T) {
		req := rd.revealRequest(t1.WireToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 reject after grace window, got %d body=%s", rec.Code, rec.Body.String())
		}
		// SCN-AUTH-010 / NFR-AUTH-007 — the response body MUST NOT
		// name the verifier failure mode. Adversarial assertion that
		// would catch a regression that leaked "expired" / "exp" /
		// "verify" / "signature" tokens to the client.
		body := strings.ToLower(rec.Body.String())
		for _, leak := range []string{"expired", "exp claim", "signature", "verify"} {
			if strings.Contains(body, leak) {
				t.Errorf("middleware 401 body leaked failure mode token %q (NFR-AUTH-007 violation): %s", leak, rec.Body.String())
			}
		}
	})

	t.Run("T2_freshly_rotated_still_admits_after_grace_window", func(t *testing.T) {
		req := rd.revealRequest(t2.WireToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 admit for T2 after T1 grace window, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

// TestRotation_AdminEndpoint_RejectsNonAdminCaller is the SCN-AUTH-004
// adversarial sub-test: a per-user PASETO holder who calls the admin
// rotate endpoint MUST be rejected with 401 + FORBIDDEN error code.
// The middleware admits the request (PASETO is valid) but the handler
// runs callerIsAdmin which returns false for SessionSourcePerUserToken
// per the spec 044 design.md §6.4 admin scope contract.
func TestRotation_AdminEndpoint_RejectsNonAdminCaller(t *testing.T) {
	baseTime := time.Now().UTC().Truncate(time.Second)
	rd := newRotationDeps(t, baseTime)
	router := api.NewRouter(rd.deps)

	// Issue a per-user PASETO for user "alice" (NOT bootstrap, NOT shared-token).
	t1 := rd.issueAndPersist(t, "user-rotation-adversarial", "tok-rotation-adv", time.Hour, baseTime)
	rd.setClock(baseTime.Add(time.Minute))

	body := []byte(`{"prior_token_id":"tok-rotation-adv"}`)
	req := httptest.NewRequest(http.MethodPost,
		"/v1/auth/users/user-rotation-adversarial/rotate",
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+t1.WireToken)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-admin per-user caller hitting admin rotate endpoint, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "FORBIDDEN") {
		t.Errorf("expected FORBIDDEN error code in 401 body (admin scope rejection), got body=%s", rec.Body.String())
	}
	// Adversarial coverage: the rejection MUST surface BEFORE the
	// store rotation runs. Verify by querying auth_tokens — T1's
	// status MUST still be 'active' (no MarkTokenRotated call ran).
	var status string
	if err := rd.pool.QueryRow(context.Background(),
		`SELECT status FROM auth_tokens WHERE token_id = $1`, "tok-rotation-adv").
		Scan(&status); err != nil {
		t.Fatalf("query auth_tokens status: %v", err)
	}
	if status != "active" {
		t.Errorf("non-admin call rotated T1 status to %q — handler ran past callerIsAdmin gate", status)
	}
}
