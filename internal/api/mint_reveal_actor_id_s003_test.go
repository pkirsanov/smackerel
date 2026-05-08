package api

// Adversarial regression tests for MIT-040-S-003 partial closure
// (2026-05-08 — spec 040 hardening).
//
// Closure summary: `POST /v1/photos/{id}/reveal` no longer accepts
// `actor_id` in the request body. The handler reads the actor identity
// ONLY from the `X-Actor-Id` header. In production deployments the
// header is required (fail-closed). Dev/test ergonomics fall back to
// the "system" actor when the header is absent.
//
// Each test below is designed to FAIL if the corresponding safeguard
// is reverted:
//
//   - TestMintReveal_S003_BodySourceForActorIDIsRejected — proves the
//     body smuggling guard. FAILS if the body-source path is restored
//     or the rejection error code drifts.
//
//   - TestMintReveal_S003_HeaderSourcedActorIDIsAccepted — proves the
//     header-source path is intact. The validation gate must pass so
//     the request reaches the store layer (which short-circuits with
//     503 because the test handler has no store). FAILS if the
//     header-source path is broken (request would be rejected at
//     validation with a 4xx).
//
//   - TestMintReveal_S003_ProductionEnvRequiresActorHeader — proves
//     the production-mode strictness gate. FAILS if the gate is
//     removed or the environment string check is widened.
//
//   - TestMintReveal_S003_DevelopmentEnvAllowsMissingActorHeader —
//     proves the dev/test ergonomic still allows missing headers
//     through the gate. FAILS if the production-mode gate fires for
//     non-production environments.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// newMintRevealRequest builds an HTTP request hitting the MintReveal
// handler through a chi router so `chi.URLParam(r, "id")` resolves
// correctly. Body may be nil for an empty body. The returned request
// already has the `Content-Type: application/json` header set when a
// body is supplied; callers can mutate headers before serving.
func newMintRevealRequest(t *testing.T, photoID string, body []byte) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	var req *http.Request
	if rdr != nil {
		req = httptest.NewRequest(http.MethodPost, "/v1/photos/"+photoID+"/reveal", rdr)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(http.MethodPost, "/v1/photos/"+photoID+"/reveal", nil)
	}
	return req, httptest.NewRecorder()
}

// serveMintReveal runs the handler through a minimal chi router so
// chi.URLParam resolves the {id} segment.
func serveMintReveal(handlers *PhotosHandlers, req *http.Request, rec *httptest.ResponseRecorder) {
	router := chi.NewRouter()
	router.Post("/v1/photos/{id}/reveal", handlers.MintReveal)
	router.ServeHTTP(rec, req)
}

// TestMintReveal_S003_BodySourceForActorIDIsRejected — Adversarial
// regression for MIT-040-S-003 (body-source removal). A request that
// includes `actor_id` in the JSON body MUST be rejected with HTTP 400
// `actor_id_in_body_forbidden`, regardless of environment. FAILS if
// the body-source field is reintroduced on `PhotoRevealRequest` or
// the smuggling guard is removed from MintReveal.
func TestMintReveal_S003_BodySourceForActorIDIsRejected(t *testing.T) {
	handlers := &PhotosHandlers{environment: "development"}
	photoID := uuid.NewString()
	body := []byte(`{"actor_id":"alice"}`)
	req, rec := newMintRevealRequest(t, photoID, body)
	req.Header.Set("X-Actor-Id", "header-actor")

	serveMintReveal(handlers, req, rec)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("MIT-040-S-003 regression: body-sourced actor_id must be rejected with 400, got %d body=%s",
			rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "actor_id_in_body_forbidden") {
		t.Fatalf("MIT-040-S-003 regression: rejection must use code actor_id_in_body_forbidden, got body=%s",
			rec.Body.String())
	}
}

// TestMintReveal_S003_HeaderSourcedActorIDIsAccepted — Positive
// control for the header-source path. With `X-Actor-Id` set and no
// body smuggling, the validation gate must pass so the request
// reaches the store call. The test handler has no store, so the
// store-nil short-circuit returns 503 `photos_store_unavailable`.
// Reaching that error code proves the validation chain accepted the
// request; getting back a 400 `actor_id_in_body_forbidden` or
// `actor_id_required` would indicate the header-source path broke.
func TestMintReveal_S003_HeaderSourcedActorIDIsAccepted(t *testing.T) {
	handlers := &PhotosHandlers{environment: "development"}
	photoID := uuid.NewString()
	// Empty body — no actor_id smuggling.
	req, rec := newMintRevealRequest(t, photoID, []byte(`{}`))
	req.Header.Set("X-Actor-Id", "alice")

	serveMintReveal(handlers, req, rec)

	body := rec.Body.String()
	if strings.Contains(body, "actor_id_in_body_forbidden") {
		t.Fatalf("MIT-040-S-003 regression: header-only request was wrongly flagged as body-sourced; body=%s", body)
	}
	if strings.Contains(body, "actor_id_required") {
		t.Fatalf("MIT-040-S-003 regression: header-source request was wrongly rejected for missing actor; body=%s", body)
	}
	// We expect to reach the store-nil short-circuit (503) because the
	// test handler has no store, which proves the validation gate
	// passed cleanly for the header-sourced actor path.
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("MIT-040-S-003 regression: header-source path must reach the store layer; expected 503 got %d body=%s",
			rec.Code, body)
	}
	if !strings.Contains(body, "photos_store_unavailable") {
		t.Fatalf("MIT-040-S-003 regression: expected photos_store_unavailable from store-nil short-circuit, got body=%s", body)
	}
}

// TestMintReveal_S003_ProductionEnvRequiresActorHeader — Production
// deployments MUST refuse mint requests that lack the `X-Actor-Id`
// header. FAILS if the production-mode strictness gate is removed or
// the environment check is broadened.
func TestMintReveal_S003_ProductionEnvRequiresActorHeader(t *testing.T) {
	handlers := &PhotosHandlers{environment: "production"}
	photoID := uuid.NewString()
	req, rec := newMintRevealRequest(t, photoID, []byte(`{}`))
	// Deliberately do NOT set X-Actor-Id.

	serveMintReveal(handlers, req, rec)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("MIT-040-S-003 regression: production mint without X-Actor-Id must be rejected with 400, got %d body=%s",
			rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "actor_id_required") {
		t.Fatalf("MIT-040-S-003 regression: rejection must use code actor_id_required, got body=%s",
			rec.Body.String())
	}
}

// TestMintReveal_S003_DevelopmentEnvAllowsMissingActorHeader — Dev/
// test ergonomics: missing `X-Actor-Id` in non-production environments
// MUST NOT trip the production-mode strictness gate. The request
// should pass the validation chain (and then hit the store-nil
// short-circuit on this test handler). FAILS if the production-mode
// gate fires for development/test environments.
func TestMintReveal_S003_DevelopmentEnvAllowsMissingActorHeader(t *testing.T) {
	for _, env := range []string{"development", "test"} {
		t.Run(env, func(t *testing.T) {
			handlers := &PhotosHandlers{environment: env}
			photoID := uuid.NewString()
			req, rec := newMintRevealRequest(t, photoID, []byte(`{}`))
			// Deliberately do NOT set X-Actor-Id.

			serveMintReveal(handlers, req, rec)

			body := rec.Body.String()
			if strings.Contains(body, "actor_id_required") {
				t.Fatalf("MIT-040-S-003 regression: %s environment must NOT require X-Actor-Id; body=%s",
					env, body)
			}
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("MIT-040-S-003 regression: %s environment must reach store layer (expected 503 from nil store); got %d body=%s",
					env, rec.Code, body)
			}
			if !strings.Contains(body, "photos_store_unavailable") {
				t.Fatalf("MIT-040-S-003 regression: expected photos_store_unavailable from store-nil short-circuit in %s; body=%s",
					env, body)
			}
		})
	}
}
