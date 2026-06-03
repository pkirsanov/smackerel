// Spec 027 scope 9 — T9-07 audit-source unit test.
//
// Verifies UI-originated POST records source_channel=web and the
// bearer subject as actor_id, by capturing the args the store sees.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/auth"
)

type auditCaptureStore struct {
	stubAnnotationStore
	gotChannel annotation.SourceChannel
	gotActor   string
}

func (s *auditCaptureStore) CreateFromParsedAs(_ context.Context, _ string, _ annotation.ParsedAnnotation, channel annotation.SourceChannel, actor string) ([]annotation.Annotation, error) {
	s.gotChannel = channel
	s.gotActor = actor
	return []annotation.Annotation{{ID: "x", SourceChannel: channel, ActorID: actor}}, nil
}

func TestCreateAnnotation_T9_07_RecordsWebChannelAndActor(t *testing.T) {
	store := &auditCaptureStore{}
	h := &AnnotationHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "5/5 made it"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Smackerel-Source", "web")
	req = req.WithContext(auth.WithSession(context.Background(), auth.Session{
		UserID: "alice", Source: auth.SessionSourcePerUserToken,
	}))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}
	if store.gotChannel != annotation.ChannelWeb {
		t.Errorf("channel = %q, want web", store.gotChannel)
	}
	if store.gotActor != "alice" {
		t.Errorf("actor = %q, want alice", store.gotActor)
	}
}

// Adversarial — missing header is rejected with 400, never silently
// defaulted to "api" or "web".
func TestCreateAnnotation_MissingSourceHeader_400(t *testing.T) {
	store := &auditCaptureStore{}
	h := &AnnotationHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "5/5"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// no X-Smackerel-Source
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing header)", w.Code)
	}
	if store.gotChannel != "" {
		t.Errorf("store should NOT have been called; gotChannel=%q", store.gotChannel)
	}
}

// Adversarial — unknown value is rejected.
func TestCreateAnnotation_UnknownSourceHeader_400(t *testing.T) {
	store := &auditCaptureStore{}
	h := &AnnotationHandlers{Store: store}

	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "5/5"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Smackerel-Source", "mystery-channel")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (unknown value)", w.Code)
	}
}
