package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/auth"
)

const (
	// maxAnnotationBodySize limits request body for annotation endpoints (64 KB).
	maxAnnotationBodySize = 64 << 10
	// maxAnnotationTextLen limits the freeform annotation text length (2000 chars).
	maxAnnotationTextLen = 2000
)

// validTagRe matches the tag pattern accepted by the annotation parser: word chars and hyphens.
var validTagRe = regexp.MustCompile(`^[\w-]+$`)

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
//
// Spec 044 Scope 02 closure of MIT-027-TRACE-001 (actor-source segment):
// when `Environment == "production"`, CreateAnnotation defensively
// rejects requests whose body smuggles an `actor_source` or `actor_id`
// JSON key (HTTP 400 `actor_source_in_body_forbidden` /
// `actor_id_in_body_forbidden`). The actor-source channel is now
// resolved from the authenticated session attached by
// `bearerAuthMiddleware` (`auth.UserIDFromContext`); when present it
// is logged alongside the parse outcome so the audit trail records the
// authenticated principal. Full schema-level actor_source persistence
// (annotations table column + downstream consumers) is documented as
// deferred follow-up implement work; the smuggling-rejection +
// session-actor logging close the trace residual.
type AnnotationHandlers struct {
	Store annotation.AnnotationQuerier
	// Environment is the deployment environment string (allowed:
	// development | test | production). Sourced from
	// runtime.environment in smackerel.yaml via SMACKEREL_ENV. Wiring
	// sets this field at startup; tests omit it (empty default keeps
	// the legacy dev/test ergonomic).
	Environment string

	// ShadowComparator wires the spec 076 SCOPE-4b dual-write shadow
	// comparator: every annotation parse is mirrored to the new
	// `annotation.classify.v1` classifier path, and divergences emit
	// telemetry (Prometheus counter + structured log). The primary
	// (inline interactionMap) result is unaffected; the shadow path
	// is fire-and-compare for telemetry only. A nil value is a safe
	// no-op so tests and pre-bridge boot stages can omit wiring.
	ShadowComparator *annotation.ShadowComparator
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

	// Spec 044 Scope 02 — read body once so we can defensively scan
	// for `actor_source`/`actor_id` smuggling before the typed
	// unmarshal. We bound the read at maxAnnotationBodySize; a body
	// that exceeds the cap returns the standard MaxBytesReader error
	// path through json.Unmarshal below.
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAnnotationBodySize))
	if err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if h.Environment == "production" {
		if bytes.Contains(bodyBytes, []byte(`"actor_source"`)) {
			http.Error(w, `{"error":"actor_source in request body is forbidden in production"}`, http.StatusBadRequest)
			return
		}
		if bytes.Contains(bodyBytes, []byte(`"actor_id"`)) {
			http.Error(w, `{"error":"actor_id in request body is forbidden in production"}`, http.StatusBadRequest)
			return
		}
	}

	var req CreateAnnotationRequest
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, `{"error":"text field required"}`, http.StatusBadRequest)
		return
	}

	if len(req.Text) > maxAnnotationTextLen {
		http.Error(w, `{"error":"annotation text too long (max 2000 chars)"}`, http.StatusBadRequest)
		return
	}

	parsed := annotation.Parse(req.Text)

	// Spec 076 SCOPE-4b — dual-write shadow comparator. The PRIMARY
	// InteractionType remains the inline interactionMap result above
	// (parsed.InteractionType); the comparator runs the new
	// `annotation.classify.v1` path in shadow mode and emits
	// divergence telemetry. Nil = no-op (bridge not yet wired).
	if h.ShadowComparator != nil {
		h.ShadowComparator.Compare(r.Context(), req.Text, annotation.ChannelAPI, parsed.InteractionType)
	}

	created, err := h.Store.CreateFromParsed(r.Context(), artifactID, parsed, annotation.ChannelAPI)
	if err != nil {
		slog.Error("failed to create annotations", "artifact_id", artifactID, "error", err)
		http.Error(w, `{"error":"failed to create annotations"}`, http.StatusInternalServerError)
		return
	}

	// Spec 044 Scope 02 — log the authenticated principal alongside
	// the parse outcome so the audit trail records the actor source
	// even before the schema column lands (deferred follow-up).
	if sessionUser := auth.UserIDFromContext(r.Context()); sessionUser != "" {
		slog.Info("annotation created",
			"artifact_id", artifactID,
			"actor_source", "session",
			"actor_user_id", sessionUser,
			"created_count", len(created))
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

	r.Body = http.MaxBytesReader(w, r.Body, maxAnnotationBodySize)
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

// DeleteTag handles DELETE /api/artifacts/{id}/tags/{tag}.
func (h *AnnotationHandlers) DeleteTag(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	artifactID := chi.URLParam(r, "id")
	if artifactID == "" {
		http.Error(w, `{"error":"artifact id required"}`, http.StatusBadRequest)
		return
	}

	tag := chi.URLParam(r, "tag")
	if tag == "" {
		http.Error(w, `{"error":"tag required"}`, http.StatusBadRequest)
		return
	}

	if !validTagRe.MatchString(tag) {
		http.Error(w, `{"error":"invalid tag format"}`, http.StatusBadRequest)
		return
	}

	if err := h.Store.DeleteTag(r.Context(), artifactID, tag, annotation.ChannelAPI); err != nil {
		slog.Error("failed to delete tag", "artifact_id", artifactID, "tag", tag, "error", err)
		http.Error(w, `{"error":"failed to remove tag"}`, http.StatusInternalServerError)
		return
	}

	summary, _ := h.Store.GetSummary(r.Context(), artifactID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"removed": tag,
		"summary": summary,
	})
}
