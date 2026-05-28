//go:build integration

// Spec 044 Scope 02 — MIT-027-TRACE-001 (actor-source segment)
// closure integration test.
//
// Exercises the production-mode defensive rejection of body-smuggled
// `actor_source` and `actor_id` keys on POST
// /api/artifacts/{id}/annotations end-to-end against the router. The
// test does not require DATABASE_URL because the rejection happens
// BEFORE any store call (a no-op stub is sufficient).
package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
)

// stubAnnotationStoreForAuth implements annotation.AnnotationQuerier
// with no-op behaviors. Production-mode body rejection MUST happen
// before any store call so the stub never sees CreateFromParsed.
type stubAnnotationStoreForAuth struct {
	createCalls int
}

func (s *stubAnnotationStoreForAuth) CreateFromParsed(_ context.Context, _ string, _ annotation.ParsedAnnotation, _ annotation.SourceChannel) ([]annotation.Annotation, error) {
	s.createCalls++
	return nil, nil
}
func (s *stubAnnotationStoreForAuth) GetSummary(_ context.Context, _ string) (*annotation.Summary, error) {
	return &annotation.Summary{}, nil
}
func (s *stubAnnotationStoreForAuth) GetHistory(_ context.Context, _ string, _ int) ([]annotation.Annotation, error) {
	return nil, nil
}
func (s *stubAnnotationStoreForAuth) DeleteTag(_ context.Context, _ string, _ string, _ annotation.SourceChannel) error {
	return nil
}
func (s *stubAnnotationStoreForAuth) RecordMessageArtifact(_ context.Context, _, _ int64, _ string) error {
	return nil
}
func (s *stubAnnotationStoreForAuth) ResolveArtifactFromMessage(_ context.Context, _, _ int64) (string, error) {
	return "", nil
}

// productionAuthDepsForAnnotation builds the per-user PASETO subsystem
// + a stub annotation store. No DB needed.
func productionAuthDepsForAnnotation(t *testing.T) (*api.Dependencies, string, *stubAnnotationStoreForAuth) {
	t.Helper()
	priv, pub := auth.GenerateSigningKeypair()
	const kid = "scope02-annotation-key"

	stub := &stubAnnotationStoreForAuth{}
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
			AtRestHashingKey:                      priv + "-anntest-hash",
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
		AnnotationHandlers: &api.AnnotationHandlers{
			Store:       stub,
			Environment: "production",
		},
	}
	return deps, priv, stub
}

// TestAnnotation_BodyActorSourceInProduction_Rejected validates the
// MIT-027-TRACE-001 actor-source segment closure: a request whose
// body smuggles `actor_source` MUST be rejected with HTTP 400 in
// production BEFORE any store call. The stub store's createCalls
// counter MUST remain zero.
func TestAnnotation_BodyActorSourceInProduction_Rejected(t *testing.T) {
	deps, priv, stub := productionAuthDepsForAnnotation(t)
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-ann-001",
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

	body := `{"text":"this is fine #project","actor_source":"telegram"}`
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/abc-123/annotations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for body actor_source in production, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "actor_source in request body is forbidden in production") {
		t.Errorf("expected actor_source rejection message, body=%s", rec.Body.String())
	}
	if stub.createCalls != 0 {
		t.Errorf("expected store.CreateFromParsed NOT to be called when body is rejected, got %d calls", stub.createCalls)
	}
}

// TestAnnotation_BodyActorIDInProduction_Rejected validates the same
// rejection for body-smuggled `actor_id`.
func TestAnnotation_BodyActorIDInProduction_Rejected(t *testing.T) {
	deps, priv, stub := productionAuthDepsForAnnotation(t)
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "alice",
		TokenID:    "tok-ann-002",
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

	body := `{"text":"#tag","actor_id":"mallory"}`
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/abc-456/annotations", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+issued.WireToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for body actor_id in production, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "actor_id in request body is forbidden in production") {
		t.Errorf("expected actor_id rejection message, body=%s", rec.Body.String())
	}
	if stub.createCalls != 0 {
		t.Errorf("expected no CreateFromParsed call, got %d", stub.createCalls)
	}
}
