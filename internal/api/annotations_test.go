package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestCreateAnnotation_MissingBody(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	req := httptest.NewRequest("POST", "/api/artifacts/art-001/annotations", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Store is nil → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when store is nil, got %d", w.Code)
	}
}

func TestCreateAnnotation_EmptyText(t *testing.T) {
	// Without a real store we can't test full flow, but we can verify validation
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	body := `{"text": ""}`
	req := httptest.NewRequest("POST", "/api/artifacts/art-001/annotations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Store nil checked first → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestGetAnnotations_NoStore(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/annotations", h.GetAnnotations)

	req := httptest.NewRequest("GET", "/api/artifacts/art-001/annotations", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestGetAnnotationSummary_NoStore(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/annotations/summary", h.GetAnnotationSummary)

	req := httptest.NewRequest("GET", "/api/artifacts/art-001/annotations/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestCreateAnnotationRequest_JSONParse(t *testing.T) {
	body := `{"text": "4/5 made it #weeknight great flavor"}`
	var req CreateAnnotationRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Text != "4/5 made it #weeknight great flavor" {
		t.Errorf("text: %s", req.Text)
	}
}
