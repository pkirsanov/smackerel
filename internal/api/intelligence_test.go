package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/intelligence"
)

// newNilPoolEngine returns an engine with nil pool for handler error path testing.
func newNilPoolEngine() *intelligence.Engine {
	return intelligence.NewEngine(nil, nil)
}

// --- GAP-1: R-503 content-fuel endpoint ---

func TestContentFuelHandler_NilPool_Returns500(t *testing.T) {
	handler := ContentFuelHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/content-fuel", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "content_fuel_error" {
		t.Errorf("expected error code content_fuel_error, got %s", resp.Error.Code)
	}
}

// --- GAP-2: R-507 quick-references endpoint ---

func TestQuickReferencesHandler_NilPool_Returns500(t *testing.T) {
	handler := QuickReferencesHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/quick-references", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "quick_references_error" {
		t.Errorf("expected error code quick_references_error, got %s", resp.Error.Code)
	}
}

// --- Existing handler nil-pool error paths (regression) ---

func TestExpertiseHandler_NilPool_Returns500(t *testing.T) {
	handler := ExpertiseHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/expertise", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestLearningPathsHandler_NilPool_Returns500(t *testing.T) {
	handler := LearningPathsHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/learning-paths", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestSubscriptionsHandler_NilPool_Returns500(t *testing.T) {
	handler := SubscriptionsHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/subscriptions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

func TestSerendipityHandler_NilPool_Returns500(t *testing.T) {
	handler := SerendipityHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/serendipity", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}
}

// --- GAP-3: R-506 monthly-report endpoint ---

func TestMonthlyReportHandler_NilPool_Returns500(t *testing.T) {
	handler := MonthlyReportHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/monthly-report", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "monthly_report_error" {
		t.Errorf("expected error code monthly_report_error, got %s", resp.Error.Code)
	}
}

// --- GAP-4: R-508 seasonal-patterns endpoint ---

func TestSeasonalPatternsHandler_NilPool_Returns500(t *testing.T) {
	handler := SeasonalPatternsHandler(newNilPoolEngine())

	req := httptest.NewRequest(http.MethodGet, "/api/seasonal-patterns", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nil pool, got %d", rec.Code)
	}

	var resp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if resp.Error.Code != "seasonal_error" {
		t.Errorf("expected error code seasonal_error, got %s", resp.Error.Code)
	}
}

// --- Route registration: content-fuel and quick-references reachable ---

func TestRouter_ContentFuelAndQuickReferencesRoutes(t *testing.T) {
	engine := newNilPoolEngine()
	deps := &Dependencies{
		DB:                 &mockDB{healthy: true},
		NATS:               &mockNATS{healthy: true},
		AuthToken:          "test-token",
		IntelligenceEngine: engine,
	}

	router := NewRouter(deps)

	routes := []struct {
		path         string
		expectedCode int
	}{
		{"/api/content-fuel", http.StatusInternalServerError},      // nil pool → 500
		{"/api/quick-references", http.StatusInternalServerError},  // nil pool → 500
		{"/api/expertise", http.StatusInternalServerError},         // nil pool → 500
		{"/api/learning-paths", http.StatusInternalServerError},    // nil pool → 500
		{"/api/subscriptions", http.StatusInternalServerError},     // nil pool → 500
		{"/api/serendipity", http.StatusInternalServerError},       // nil pool → 500
		{"/api/monthly-report", http.StatusInternalServerError},    // nil pool → 500
		{"/api/seasonal-patterns", http.StatusInternalServerError}, // nil pool → 500
	}

	for _, rt := range routes {
		t.Run(rt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, rt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != rt.expectedCode {
				t.Errorf("%s: expected %d, got %d", rt.path, rt.expectedCode, rec.Code)
			}
		})
	}
}

func TestRouter_ContentFuelRequiresAuth(t *testing.T) {
	engine := newNilPoolEngine()
	deps := &Dependencies{
		DB:                 &mockDB{healthy: true},
		NATS:               &mockNATS{healthy: true},
		AuthToken:          "secret",
		IntelligenceEngine: engine,
	}

	router := NewRouter(deps)

	// No auth header → 401
	req := httptest.NewRequest(http.MethodGet, "/api/content-fuel", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}
