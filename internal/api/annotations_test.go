package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/annotation"
)

// stubAnnotationStore is a minimal mock for security tests that need a non-nil store.
type stubAnnotationStore struct{}

func (s *stubAnnotationStore) CreateFromParsed(_ context.Context, _ string, _ annotation.ParsedAnnotation, _ annotation.SourceChannel) ([]annotation.Annotation, error) {
	return nil, nil
}
func (s *stubAnnotationStore) GetSummary(_ context.Context, _ string) (*annotation.Summary, error) {
	return &annotation.Summary{}, nil
}
func (s *stubAnnotationStore) GetHistory(_ context.Context, _ string, _ int) ([]annotation.Annotation, error) {
	return nil, nil
}
func (s *stubAnnotationStore) DeleteTag(_ context.Context, _, _ string, _ annotation.SourceChannel) error {
	return nil
}
func (s *stubAnnotationStore) RecordMessageArtifact(_ context.Context, _, _ int64, _ string) error {
	return nil
}
func (s *stubAnnotationStore) ResolveArtifactFromMessage(_ context.Context, _, _ int64) (string, error) {
	return "", nil
}

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

// --- Security tests (SEC-027) ---

// SEC-027-001: Oversized request body must be rejected by MaxBytesReader.
func TestCreateAnnotation_OversizedBody(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	// Build a body larger than maxAnnotationBodySize (64 KB)
	bigText := strings.Repeat("x", 70*1024)
	body := `{"text":"` + bigText + `"}`
	req := httptest.NewRequest("POST", "/api/artifacts/art-001/annotations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("SEC-027-001: oversized body should be rejected, got %d", w.Code)
	}
}

// SEC-027-002: Annotation text exceeding maxAnnotationTextLen must be rejected.
func TestCreateAnnotation_TextTooLong(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	longText := strings.Repeat("a", 2001)
	body, _ := json.Marshal(CreateAnnotationRequest{Text: longText})
	req := httptest.NewRequest("POST", "/api/artifacts/art-001/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("SEC-027-002: text over 2000 chars should be rejected, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "too long") {
		t.Errorf("SEC-027-002: response should mention 'too long', got %s", w.Body.String())
	}
}

// SEC-027-002: Annotation text at exactly the limit should be accepted.
func TestCreateAnnotation_TextAtLimit(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	exactText := strings.Repeat("b", 2000)
	body, _ := json.Marshal(CreateAnnotationRequest{Text: exactText})
	req := httptest.NewRequest("POST", "/api/artifacts/art-001/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("SEC-027-002: text at 2000 chars should be accepted, got %d", w.Code)
	}
}

// SEC-027-001: Oversized body on internal telegram-message-artifact endpoint.
func TestRecordTelegramMessageArtifact_OversizedBody(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}

	r := chi.NewRouter()
	r.Post("/internal/telegram-message-artifact", h.RecordTelegramMessageArtifact)

	bigBody := `{"message_id":1001,"chat_id":5555,"artifact_id":"` + strings.Repeat("x", 70*1024) + `"}`
	req := httptest.NewRequest("POST", "/internal/telegram-message-artifact", bytes.NewReader([]byte(bigBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("SEC-027-001: oversized body should be rejected, got %d", w.Code)
	}
}

// SEC-027-003: Tag with invalid characters must be rejected.
func TestDeleteTag_InvalidTagFormat(t *testing.T) {
	h := &AnnotationHandlers{Store: &stubAnnotationStore{}}

	r := chi.NewRouter()
	r.Delete("/api/artifacts/{id}/tags/{tag}", h.DeleteTag)

	cases := []struct {
		tag  string
		want int
	}{
		{"valid-tag", http.StatusOK},
		{"weeknight", http.StatusOK},
		{"tag_with_underscore", http.StatusOK},
		{"has%20spaces", http.StatusBadRequest},    // URL-encoded space
		{"tag%3Bdrop", http.StatusBadRequest},      // URL-encoded semicolon
		{"%3Cscript%3E", http.StatusBadRequest},    // URL-encoded angle brackets
	}

	for _, tc := range cases {
		req := httptest.NewRequest("DELETE", "/api/artifacts/art-001/tags/"+tc.tag, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != tc.want {
			t.Errorf("SEC-027-003: DeleteTag(%q) = %d, want %d", tc.tag, w.Code, tc.want)
		}
	}
}
