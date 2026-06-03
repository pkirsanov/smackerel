// Spec 027 scope 9 — T9-03, T9-04 If-Match conflict path unit tests.
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
)

// versionStubStore returns a programmable version for the If-Match
// path and records whether CreateFromParsedAs was invoked.
type versionStubStore struct {
	stubAnnotationStore
	version     int64
	createCalls int
	gotActor    string
	gotChannel  annotation.SourceChannel
}

func (s *versionStubStore) GetSummaryVersion(_ context.Context, _ string) (int64, error) {
	return s.version, nil
}
func (s *versionStubStore) CreateFromParsedAs(_ context.Context, _ string, _ annotation.ParsedAnnotation, channel annotation.SourceChannel, actor string) ([]annotation.Annotation, error) {
	s.createCalls++
	s.gotActor = actor
	s.gotChannel = channel
	return []annotation.Annotation{{ID: "ann-1"}}, nil
}
func (s *versionStubStore) GetSummary(_ context.Context, artifactID string) (*annotation.Summary, error) {
	return &annotation.Summary{ArtifactID: artifactID, Version: s.version}, nil
}

func mountCreateAnnotation(h *AnnotationHandlers) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/artifacts/{id}/annotations", h.CreateAnnotation)
	return r
}

// T9-03 — stale If-Match returns 409 + current summary; no event recorded.
func TestCreateAnnotation_T9_03_StaleIfMatch_409(t *testing.T) {
	store := &versionStubStore{version: 7}
	h := &AnnotationHandlers{Store: store}

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "5/5 great"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Smackerel-Source", "web")
	req.Header.Set("If-Match", "3") // stale
	w := httptest.NewRecorder()
	mountCreateAnnotation(h).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	if store.createCalls != 0 {
		t.Errorf("CreateFromParsedAs called %d times; expected 0 on conflict", store.createCalls)
	}
	var sum annotation.Summary
	if err := json.Unmarshal(w.Body.Bytes(), &sum); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if sum.Version != 7 {
		t.Errorf("conflict body version = %d, want 7", sum.Version)
	}
}

// T9-04 — POST without If-Match preserves append semantics unchanged.
func TestCreateAnnotation_T9_04_NoIfMatch_Appends(t *testing.T) {
	store := &versionStubStore{version: 5}
	h := &AnnotationHandlers{Store: store}

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "made it"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Smackerel-Source", "web")
	w := httptest.NewRecorder()
	mountCreateAnnotation(h).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", w.Code, w.Body.String())
	}
	if store.createCalls != 1 {
		t.Errorf("CreateFromParsedAs called %d times; expected 1", store.createCalls)
	}
}

// Matching If-Match proceeds with insert.
func TestCreateAnnotation_MatchingIfMatch_201(t *testing.T) {
	store := &versionStubStore{version: 5}
	h := &AnnotationHandlers{Store: store}

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "5/5"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Smackerel-Source", "web")
	req.Header.Set("If-Match", "5")
	w := httptest.NewRecorder()
	mountCreateAnnotation(h).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

// Adversarial — non-integer If-Match is a 400.
func TestCreateAnnotation_BadIfMatch_400(t *testing.T) {
	store := &versionStubStore{version: 1}
	h := &AnnotationHandlers{Store: store}

	body, _ := json.Marshal(CreateAnnotationRequest{Text: "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/art-1/annotations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Smackerel-Source", "web")
	req.Header.Set("If-Match", "not-a-number")
	w := httptest.NewRecorder()
	mountCreateAnnotation(h).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
