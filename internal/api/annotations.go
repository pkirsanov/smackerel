package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/annotation"
)

// CreateAnnotationRequest is the JSON body for POST /api/artifacts/{id}/annotations.
type CreateAnnotationRequest struct {
	Text string `json:"text"` // freeform annotation text to parse
}

// CreateAnnotationResponse is the response for annotation creation.
type CreateAnnotationResponse struct {
	Created []annotation.Annotation `json:"created"`
	Summary *annotation.Summary     `json:"summary"`
}

// AnnotationHandlers holds annotation API handler methods.
type AnnotationHandlers struct {
	Store *annotation.Store
}

// CreateAnnotation handles POST /api/artifacts/{id}/annotations.
func (h *AnnotationHandlers) CreateAnnotation(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		http.Error(w, `{"error":"artifact id required"}`, http.StatusBadRequest)
		return
	}

	var req CreateAnnotationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, `{"error":"text field required"}`, http.StatusBadRequest)
		return
	}

	parsed := annotation.Parse(req.Text)

	created, err := h.Store.CreateFromParsed(r.Context(), artifactID, parsed, annotation.ChannelAPI)
	if err != nil {
		slog.Error("failed to create annotations", "artifact_id", artifactID, "error", err)
		http.Error(w, `{"error":"failed to create annotations"}`, http.StatusInternalServerError)
		return
	}

	summary, _ := h.Store.GetSummary(r.Context(), artifactID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CreateAnnotationResponse{
		Created: created,
		Summary: summary,
	})
}

// GetAnnotations handles GET /api/artifacts/{id}/annotations.
func (h *AnnotationHandlers) GetAnnotations(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		http.Error(w, `{"error":"artifact id required"}`, http.StatusBadRequest)
		return
	}

	history, err := h.Store.GetHistory(r.Context(), artifactID, 50)
	if err != nil {
		slog.Error("failed to get annotations", "artifact_id", artifactID, "error", err)
		http.Error(w, `{"error":"failed to get annotations"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"artifact_id": artifactID,
		"annotations": history,
	})
}

// GetAnnotationSummary handles GET /api/artifacts/{id}/annotations/summary.
func (h *AnnotationHandlers) GetAnnotationSummary(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		http.Error(w, `{"error":"artifact id required"}`, http.StatusBadRequest)
		return
	}

	summary, err := h.Store.GetSummary(r.Context(), artifactID)
	if err != nil {
		slog.Error("failed to get annotation summary", "artifact_id", artifactID, "error", err)
		http.Error(w, `{"error":"failed to get summary"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(summary)
}
