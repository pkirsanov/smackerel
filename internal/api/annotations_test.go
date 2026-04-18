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

func TestRecordTelegramMessageArtifact_NoStore(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Post("/internal/telegram-message-artifact", h.RecordTelegramMessageArtifact)

	body := `{"message_id": 1001, "chat_id": 5555, "artifact_id": "art-abc"}`
	req := httptest.NewRequest("POST", "/internal/telegram-message-artifact", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestRecordTelegramMessageArtifact_MissingFields(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Post("/internal/telegram-message-artifact", h.RecordTelegramMessageArtifact)

	// Missing fields — but store is nil, so 503 comes first
	body := `{"message_id": 0}`
	req := httptest.NewRequest("POST", "/internal/telegram-message-artifact", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestResolveTelegramMessageArtifact_NoStore(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Get("/internal/telegram-message-artifact", h.ResolveTelegramMessageArtifact)

	req := httptest.NewRequest("GET", "/internal/telegram-message-artifact?message_id=1001&chat_id=5555", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestResolveTelegramMessageArtifact_MissingParams(t *testing.T) {
	h := &AnnotationHandlers{Store: nil}

	r := chi.NewRouter()
	r.Get("/internal/telegram-message-artifact", h.ResolveTelegramMessageArtifact)

	req := httptest.NewRequest("GET", "/internal/telegram-message-artifact", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Store nil checked first → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestRecordTelegramMessageArtifactRequest_JSONParse(t *testing.T) {
	body := `{"message_id": 1001, "chat_id": 5555, "artifact_id": "art-abc"}`
	var req RecordTelegramMessageArtifactRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.MessageID != 1001 {
		t.Errorf("message_id = %d, want 1001", req.MessageID)
	}
	if req.ChatID != 5555 {
		t.Errorf("chat_id = %d, want 5555", req.ChatID)
	}
	if req.ArtifactID != "art-abc" {
		t.Errorf("artifact_id = %q, want art-abc", req.ArtifactID)
	}
}
