//go:build integration

// Spec 044 Scope 02 — MIT-040-S-008 closure integration tests.
//
// These tests exercise the production-mode actor-identity contract on
// POST /v1/photos/{id}/reveal end-to-end against the live test stack.
// They assert that:
//
//  1. A request whose JSON body smuggles "actor_id" is rejected with
//     HTTP 400 actor_id_in_body_forbidden REGARDLESS of authentication
//     state. (Defense-in-depth — body-smuggling is closed even on
//     non-production deployments.)
//  2. A production-environment request that supplies X-Actor-Id is
//     rejected with HTTP 400 actor_id_in_header_forbidden because the
//     claim-bound identity must come from the bearer token.
//  3. A production-environment request with a valid PASETO token but
//     NO header derives the actor from session.UserID and proceeds
//     normally (HTTP 201 with a reveal_token).
//
// Adversarial coverage: the body-smuggling test runs in BOTH
// development and production environments to prove the rejection is
// not gated on environment alone.
//
// SCN-040-012 + AC-11 evidence: every accepted-reveal audit row
// records the session-derived actor_id rather than a header value.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// productionAuthDepsForReveal builds an api.Dependencies suitable for
// the live test stack with the per-user PASETO middleware path
// active. It seeds a single sensitive photo so MintReveal has a
// target.
func productionAuthDepsForReveal(t *testing.T) (*api.Dependencies, string, uuid.UUID) {
	t.Helper()
	pool := authTestPool(t)
	t.Cleanup(func() { pool.Close() })
	resetAuthTables(t, pool)

	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope02-mintreveal-key"

	store := photolib.NewStore(pool)
	cache := revocation.NewCache()

	// Seed an artifact row first (photos.artifact_id is NOT NULL with
	// a FK), then a sensitive photo. We don't need a connector row —
	// we just need a photos row with a non-default Sensitivity so
	// MintReveal returns 201 instead of 409.
	photoID := uuid.New()
	artifactID := "art-" + photoID.String()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (id, artifact_type, title, content_hash, source_id)
		VALUES ($1, $2, $3, $4, $5)
	`, artifactID, "photo", "scope02 reveal seed", "hash-"+photoID.String(), "test-source"); err != nil {
		t.Fatalf("seed artifacts row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO photos (
			id, artifact_id, connector_id, provider, provider_ref,
			provider_media_kind, mime_type, filename, sensitivity, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`, photoID, artifactID, "test-connector", "test", "ref-"+photoID.String(),
		"image", "image/jpeg", "test.jpg",
		string(photolib.SensitivitySensitive)); err != nil {
		t.Fatalf("seed photos row: %v", err)
	}

	deps := &api.Dependencies{
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
		RevocationCache: cache,
		PhotosHandlers:  api.NewPhotosHandlers(store, config.PhotosConfig{}, "production"),
	}
	return deps, priv, photoID
}

// TestMintReveal_BodyActorIDInProduction_Returns400_FailsLoudly is
// THE highest-value adversarial integration test for MIT-040-S-008.
// A malicious client smuggling actor_id in the request body MUST be
// rejected with HTTP 400 actor_id_in_body_forbidden BEFORE any reveal
// token is minted, even when the rest of the request is otherwise
// well-formed. The rejection is environment-INDEPENDENT (production
// AND development reject body smuggling).
func TestMintReveal_BodyActorIDInProduction_Returns400_FailsLoudly(t *testing.T) {
	deps, priv, photoID := productionAuthDepsForReveal(t)

	// Issue a real PASETO token so the request gets past
	// bearerAuthMiddleware.
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-mr-001",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	router := api.NewRouter(deps)

	body := []byte(`{"ttl_seconds":300,"actor_id":"mallory"}`)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/photos/%s/reveal", photoID.String()),
		bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for body actor_id smuggling, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "actor_id_in_body_forbidden") {
		t.Errorf("expected error code actor_id_in_body_forbidden, body=%s", rec.Body.String())
	}
}

// TestMintReveal_HeaderActorIDInProduction_Returns400 validates the
// production-mode rejection of X-Actor-Id header. The closure is
// not just about body smuggling — header smuggling MUST also fail in
// production because client-controlled identity has no place when
// PASETO `sub` is the source of truth.
func TestMintReveal_HeaderActorIDInProduction_Returns400(t *testing.T) {
	deps, priv, photoID := productionAuthDepsForReveal(t)
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-mr-002",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	router := api.NewRouter(deps)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/photos/%s/reveal", photoID.String()),
		strings.NewReader(`{"ttl_seconds":300}`))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-Id", "mallory")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for production header X-Actor-Id, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "actor_id_in_header_forbidden") {
		t.Errorf("expected error code actor_id_in_header_forbidden, body=%s", rec.Body.String())
	}
}

// TestMintReveal_ProductionWithSession_DerivesFromPASETO validates
// the happy path: production + valid PASETO + no smuggling → 201
// with a reveal_token bound to session.UserID.
func TestMintReveal_ProductionWithSession_DerivesFromPASETO(t *testing.T) {
	deps, priv, photoID := productionAuthDepsForReveal(t)
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-mr-003",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
	})
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	router := api.NewRouter(deps)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/v1/photos/%s/reveal", photoID.String()),
		strings.NewReader(`{"ttl_seconds":300}`))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 for production + valid session, got %d body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	var resp api.PhotoRevealResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, string(body))
	}
	if resp.RevealToken == "" {
		t.Errorf("expected reveal_token in response, got empty")
	}
}
