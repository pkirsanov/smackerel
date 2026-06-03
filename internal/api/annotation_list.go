// Spec 027 scope 9 — GET /api/annotations?actor=me&limit=N
// (PLAN-9-05 list-my-annotations endpoint).
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/auth"
)

// ListMyAnnotations handles GET /api/annotations.
//
// Required query parameters:
//   - actor: must equal "me" or the caller's resolved bearer subject;
//     any other value → 403 (single-tenant guard, T9-02).
//   - limit: integer 1..200; missing or out-of-range → 400.
//
// Optional:
//   - since: RFC3339 timestamp; filters created_at >= since.
//
// Backed by idx_annotations_actor_created (migration 055).
func (h *AnnotationHandlers) ListMyAnnotations(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.Error(w, `{"error":"annotations not configured"}`, http.StatusServiceUnavailable)
		return
	}

	subject := auth.UserIDFromContext(r.Context())
	if subject == "" {
		http.Error(w, `{"error":"authenticated subject required"}`, http.StatusForbidden)
		return
	}

	actorParam := r.URL.Query().Get("actor")
	if actorParam == "" {
		http.Error(w, `{"error":"actor query parameter required"}`, http.StatusBadRequest)
		return
	}
	if actorParam != "me" && actorParam != subject {
		http.Error(w, `{"error":"forbidden: actor must equal 'me' or caller subject"}`, http.StatusForbidden)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		http.Error(w, `{"error":"limit query parameter required"}`, http.StatusBadRequest)
		return
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 200 {
		http.Error(w, `{"error":"limit must be an integer in 1..200"}`, http.StatusBadRequest)
		return
	}

	var since *time.Time
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			http.Error(w, `{"error":"since must be RFC3339"}`, http.StatusBadRequest)
			return
		}
		since = &t
	}

	annotations, err := h.Store.ListByActor(r.Context(), subject, limit, since)
	if err != nil {
		slog.Error("failed to list annotations by actor", "actor", subject, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":"failed to list annotations: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	if annotations == nil {
		annotations = []annotation.Annotation{}
	}

	w.Header().Set("Content-Type", "application/json")
	type listMyAnnotationsResponse struct {
		ActorID     string                  `json:"actor_id,omitempty"`
		Limit       int                     `json:"limit"`
		Annotations []annotation.Annotation `json:"annotations"`
	}
	_ = json.NewEncoder(w).Encode(listMyAnnotationsResponse{
		ActorID:     subject,
		Limit:       limit,
		Annotations: annotations,
	})
}
