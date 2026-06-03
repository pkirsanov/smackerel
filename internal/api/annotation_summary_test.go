// Spec 027 scope 9 — T9-05 summary version unit test.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/annotation"
)

type summaryWithVersionStore struct {
	stubAnnotationStore
	v int64
}

func (s *summaryWithVersionStore) GetSummary(_ context.Context, artifactID string) (*annotation.Summary, error) {
	return &annotation.Summary{ArtifactID: artifactID, Version: s.v, RatingCount: 3}, nil
}
func (s *summaryWithVersionStore) GetSummaryVersion(_ context.Context, _ string) (int64, error) {
	return s.v, nil
}

// T9-05 — summary response includes monotonic version field.
func TestGetAnnotationSummary_T9_05_IncludesVersion(t *testing.T) {
	h := &AnnotationHandlers{Store: &summaryWithVersionStore{v: 42}}
	r := chi.NewRouter()
	r.Get("/api/artifacts/{id}/annotations/summary", h.GetAnnotationSummary)

	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/art-1/annotations/summary", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	v, ok := got["version"]
	if !ok {
		t.Fatalf("response missing version field; got %v", got)
	}
	if vf, _ := v.(float64); vf != 42 {
		t.Errorf("version = %v, want 42", v)
	}
}
