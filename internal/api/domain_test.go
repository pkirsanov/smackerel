package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/db"
)

func TestDomainDataHandler_ScaledRecipe(t *testing.T) {
	domainData := json.RawMessage(`{
		"domain": "recipe",
		"title": "Pasta Carbonara",
		"servings": 4,
		"timing": {"prep": "15 min", "cook": "20 min", "total": "35 min"},
		"cuisine": "Italian",
		"difficulty": "medium",
		"dietary_tags": [],
		"ingredients": [
			{"name": "guanciale", "quantity": "200", "unit": "g"},
			{"name": "egg yolks", "quantity": "4", "unit": ""},
			{"name": "salt", "quantity": "to taste", "unit": ""}
		],
		"steps": [{"number": 1, "instruction": "Cut guanciale into strips.", "duration_minutes": 5, "technique": "knife work"}]
	}`)

	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{
				ID:    "art-123",
				Title: "Pasta Carbonara",
			},
			DomainData: domainData,
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=8", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Check servings and scale_factor
	if result["servings"].(float64) != 8 {
		t.Errorf("expected servings=8, got %v", result["servings"])
	}
	if result["original_servings"].(float64) != 4 {
		t.Errorf("expected original_servings=4, got %v", result["original_servings"])
	}
	if result["scale_factor"].(float64) != 2.0 {
		t.Errorf("expected scale_factor=2.0, got %v", result["scale_factor"])
	}

	// Check ingredients are scaled
	ingredients, ok := result["ingredients"].([]interface{})
	if !ok || len(ingredients) != 3 {
		t.Fatalf("expected 3 ingredients, got %v", result["ingredients"])
	}

	ing0 := ingredients[0].(map[string]interface{})
	if ing0["scaled"] != true {
		t.Error("expected guanciale to be scaled")
	}

	// Salt should be unscaled
	ing2 := ingredients[2].(map[string]interface{})
	if ing2["scaled"] != false {
		t.Error("expected salt to be unscaled")
	}
}

func TestDomainDataHandler_NoServingsParam(t *testing.T) {
	domainData := json.RawMessage(`{"domain": "recipe", "title": "Test"}`)

	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{ID: "art-123"},
			DomainData:     domainData,
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Should return raw domain data
	var result map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Should NOT have scale_factor or original_servings
	if _, ok := result["scale_factor"]; ok {
		t.Error("unscaled response should not have scale_factor")
	}
	if _, ok := result["original_servings"]; ok {
		t.Error("unscaled response should not have original_servings")
	}
}

func TestDomainDataHandler_NotRecipe(t *testing.T) {
	domainData := json.RawMessage(`{"domain": "product", "title": "Headphones"}`)

	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{ID: "art-456"},
			DomainData:     domainData,
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-456/domain?servings=4", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}

	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result.Error.Code != "DOMAIN_NOT_SCALABLE" {
		t.Errorf("expected DOMAIN_NOT_SCALABLE error, got %q", result.Error.Code)
	}
}

func TestDomainDataHandler_InvalidServings(t *testing.T) {
	domainData := json.RawMessage(`{"domain": "recipe", "title": "Test", "servings": 4}`)

	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{ID: "art-123"},
			DomainData:     domainData,
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	// Test servings=0
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=0", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for servings=0, got %d", rec.Code)
	}

	// Test servings=-1
	req = httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=-1", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for servings=-1, got %d", rec.Code)
	}

	// Test servings=abc
	req = httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=abc", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for servings=abc, got %d", rec.Code)
	}
}

func TestDomainDataHandler_NoBaselineServings(t *testing.T) {
	domainData := json.RawMessage(`{"domain": "recipe", "title": "Test", "ingredients": []}`)

	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{ID: "art-123"},
			DomainData:     domainData,
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=8", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}

	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &result)
	if result.Error.Code != "NO_BASELINE_SERVINGS" {
		t.Errorf("expected NO_BASELINE_SERVINGS error, got %q", result.Error.Code)
	}
}

// TestDomainDataHandler_ServingsExceedsMax covers the API serving-scaler abuse
// cap (servings > 1000 -> 400 INVALID_SERVINGS).
//
// Round 29 (stochastic-quality-sweep, test trigger): this cap had no regression
// test on the API surface even though both Telegram entry points cap and test at
// 1000 (TestParseScaleTrigger_MaxServingsCap, TestParseCookTrigger_MaxServingsCap).
// Harden observation HARD-035-OBS-001 relies on this cap being present so a large
// scale ratio stays unreachable through spec-035 (UC-002 alternative flow A4).
//
// Adversarial: if the `> maxServings` guard were removed, the handler would scale
// and return 200; if it were weakened to `>= maxServings`, the 1000 boundary case
// below would start failing.
func TestDomainDataHandler_ServingsExceedsMax(t *testing.T) {
	domainData := json.RawMessage(`{"domain": "recipe", "title": "Test", "servings": 4}`)

	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{ID: "art-123"},
			DomainData:     domainData,
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	// servings=1001 is one above the maxServings=1000 abuse cap.
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=1001", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for servings=1001 (above abuse cap), got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if result.Error.Code != "INVALID_SERVINGS" {
		t.Errorf("expected INVALID_SERVINGS error, got %q", result.Error.Code)
	}

	// Boundary: servings=1000 (exactly the cap) MUST still succeed. This guards
	// against an off-by-one regression (`>` -> `>=`).
	req = httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=1000", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for servings=1000 (at cap boundary), got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestDomainDataHandler_NoDomainData covers the empty-domain_data 404 branch.
//
// Round 29 (test trigger): the NO_DOMAIN_DATA path was untested. Adversarial: if
// the len(a.DomainData)==0 guard were dropped, the handler would fall through to
// servings parsing / json.Unmarshal of empty bytes and surface a different status.
func TestDomainDataHandler_NoDomainData(t *testing.T) {
	store := &mockArtifactStore{
		artifactWithDom: &db.ArtifactWithDomain{
			ArtifactDetail: db.ArtifactDetail{ID: "art-123"},
			DomainData:     nil, // artifact exists but has no domain data
		},
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-123/domain?servings=8", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty domain data, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if result.Error.Code != "NO_DOMAIN_DATA" {
		t.Errorf("expected NO_DOMAIN_DATA error, got %q", result.Error.Code)
	}
}

// TestDomainDataHandler_ArtifactNotFound covers the store-error 404 branch.
//
// Round 29 (test trigger): the ARTIFACT_NOT_FOUND path was untested (the mock in
// every other test always returns an artifact). Adversarial: if the store error
// were not mapped to 404, a missing artifact would surface as a 500 or nil-deref
// panic on a.DomainData below.
func TestDomainDataHandler_ArtifactNotFound(t *testing.T) {
	store := &mockArtifactStore{
		artifactWithDomErr: errors.New("sql: no rows in result set"),
	}

	deps := &Dependencies{
		ArtifactStore: store,
		StartTime:     time.Now(),
	}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/domain", deps.DomainDataHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/missing/domain?servings=8", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing artifact, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if result.Error.Code != "ARTIFACT_NOT_FOUND" {
		t.Errorf("expected ARTIFACT_NOT_FOUND error, got %q", result.Error.Code)
	}
}
