// Adversarial regression tests for MIT-040-S-004 (spec 040 SST hardening).
//
// MIT-040-S-004 requires bearerAuthMiddleware to reject empty-token
// requests when Environment == "production" as defense-in-depth (the
// wiring constructor already fails fast in production, but the middleware
// must enforce the same contract). In development/test environments, an
// empty AuthToken continues to bypass auth (dev-mode ergonomic).
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestBearerMiddleware_S004_ProductionEnvRejectsEmptyTokenRequest asserts
// that a request to an authenticated API route is rejected with 401 when
// the middleware is configured with Environment="production" and an empty
// AuthToken.
//
// Adversarial proof: removing the production-mode branch in
// bearerAuthMiddleware (i.e. unconditionally calling next.ServeHTTP when
// AuthToken=="") makes this test fail because the request would be
// processed (and the route either succeeds or returns a non-401 status).
func TestBearerMiddleware_S004_ProductionEnvRejectsEmptyTokenRequest(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		AuthToken:   "",
		Environment: "production",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("production env with empty AuthToken should reject /api/digest with 401, got %d", rec.Code)
	}
}

// TestBearerMiddleware_S004_DevelopmentEnvAllowsEmptyTokenRequest asserts
// that the dev-mode bypass remains in effect when Environment is
// "development" — the request must NOT receive a 401 from the middleware
// (it may receive other status codes from the handler depending on
// dependencies, but never 401).
//
// Adversarial proof: making the production-mode branch fire on any
// non-empty Environment (e.g. != "development") makes this test fail.
func TestBearerMiddleware_S004_DevelopmentEnvAllowsEmptyTokenRequest(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		AuthToken:   "",
		Environment: "development",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Errorf("development env with empty AuthToken should not reject /api/digest with 401, got %d", rec.Code)
	}
}

// TestBearerMiddleware_S004_TestEnvAllowsEmptyTokenRequest mirrors the
// development case for the test environment so disposable test stacks
// generated under SMACKEREL_ENV=test continue to work without an explicit
// auth token.
func TestBearerMiddleware_S004_TestEnvAllowsEmptyTokenRequest(t *testing.T) {
	deps := &Dependencies{
		DB:          &mockDB{healthy: true},
		NATS:        &mockNATS{healthy: true},
		StartTime:   time.Now(),
		AuthToken:   "",
		Environment: "test",
	}

	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/api/digest", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Errorf("test env with empty AuthToken should not reject /api/digest with 401, got %d", rec.Code)
	}
}
