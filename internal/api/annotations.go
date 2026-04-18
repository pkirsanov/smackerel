package api

import (
	"encoding/json"
	"fmt"
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

// RecordTelegramMessageArtifactRequest is the body for POST /internal/telegram-message-artifact.
type RecordTelegramMessageArtifactRequest struct {
	MessageID  int64  `json:"message_id"`
	ChatID     int64  `json:"chat_id"`
	ArtifactID string `json:"artifact_id"`
}

// RecordTelegramMessageArtifact handles POST /internal/telegram-message-artifact.
func (h *AnnotationHandlers) RecordTelegramMessageArtifact(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	var req RecordTelegramMessageArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if req.MessageID == 0 || req.ChatID == 0 || req.ArtifactID == "" {
		http.Error(w, `{"error":"message_id, chat_id, and artifact_id are required"}`, http.StatusBadRequest)
		return
	}

	if err := h.Store.RecordMessageArtifact(r.Context(), req.MessageID, req.ChatID, req.ArtifactID); err != nil {
		slog.Error("failed to record message-artifact mapping", "error", err)
		http.Error(w, `{"error":"failed to record mapping"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

// ResolveTelegramMessageArtifact handles GET /internal/telegram-message-artifact.
func (h *AnnotationHandlers) ResolveTelegramMessageArtifact(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	messageIDStr := r.URL.Query().Get("message_id")
	chatIDStr := r.URL.Query().Get("chat_id")

	if messageIDStr == "" || chatIDStr == "" {
		http.Error(w, `{"error":"message_id and chat_id query params required"}`, http.StatusBadRequest)
		return
	}

	var messageID, chatID int64
	if _, err := fmt.Sscanf(messageIDStr, "%d", &messageID); err != nil {
		http.Error(w, `{"error":"invalid message_id"}`, http.StatusBadRequest)
		return
	}
	if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
		http.Error(w, `{"error":"invalid chat_id"}`, http.StatusBadRequest)
		return
	}

	artifactID, err := h.Store.ResolveArtifactFromMessage(r.Context(), messageID, chatID)
	if err != nil || artifactID == "" {
		w.WriteHeader(http.StatusNotFound)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"artifact_id": ""})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"artifact_id": artifactID})
}
