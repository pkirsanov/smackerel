package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	photolib "github.com/smackerel/smackerel/internal/connector/photos"
)

// PhotoActionsPlanRequest is the body of `POST /v1/photos/actions/plan`.
type PhotoActionsPlanRequest struct {
	Action        string               `json:"action"`
	Scope         photolib.ActionScope `json:"scope"`
	BytesEstimate int64                `json:"bytes_estimate,omitempty"`
	ConfidenceMin *float64             `json:"confidence_min,omitempty"`
	ConfidenceMax *float64             `json:"confidence_max,omitempty"`
}

// PhotoActionsPlanResponse mirrors the design.md ActionPlan shape.
type PhotoActionsPlanResponse struct {
	ActionToken     string             `json:"action_token"`
	Action          string             `json:"action"`
	PhotoCount      int                `json:"photo_count"`
	BytesEstimate   int64              `json:"bytes_estimate"`
	ConfidenceRange map[string]float64 `json:"confidence_range,omitempty"`
	RequiresText    bool               `json:"requires_text"`
	ExpiresAt       time.Time          `json:"expires_at"`
}

// PhotoActionsConfirmRequest is the body of `POST /v1/photos/actions/confirm`.
type PhotoActionsConfirmRequest struct {
	ActionToken      string               `json:"action_token"`
	TextConfirmation string               `json:"text_confirmation,omitempty"`
	Scope            photolib.ActionScope `json:"scope"`
}

// PhotoActionsConfirmResponse summarises the confirmed mutations.
type PhotoActionsConfirmResponse struct {
	ActionToken   string `json:"action_token"`
	Action        string `json:"action"`
	PhotoCount    int    `json:"photo_count"`
	ProviderCalls int    `json:"provider_calls"`
	Outcome       string `json:"outcome"`
	AuditEventIDs int    `json:"audit_events_written"`
}

// PlanAction handles `POST /v1/photos/actions/plan`. The endpoint is
// non-mutating (FR-020): it only mints an action token. The confirm
// endpoint is the sole mutation entry point.
func (h *PhotosHandlers) PlanAction(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	var request PhotoActionsPlanRequest
	if !decodeJSONBody(w, r, &request, "invalid_action_plan", "request body must be JSON") {
		return
	}
	action := photolib.ActionKind(request.Action)
	ttl := h.actionTokenTTL(action)
	mintInput := photolib.MintPhotoActionTokenInput{
		ActorID:       actorIDFromRequest(r),
		Action:        action,
		Scope:         request.Scope,
		BytesEstimate: request.BytesEstimate,
		ConfidenceMin: request.ConfidenceMin,
		ConfidenceMax: request.ConfidenceMax,
		TTL:           ttl,
	}
	token, err := h.store.MintActionToken(r.Context(), mintInput, time.Now().UTC())
	if err != nil {
		switch {
		case errors.Is(err, photolib.ErrActionTokenInvalidAction):
			writeError(w, http.StatusBadRequest, "invalid_action", "unsupported action kind")
		case errors.Is(err, photolib.ErrActionTokenScopeEmpty):
			writeError(w, http.StatusBadRequest, "empty_scope", "action scope must include photo_ids, removal_ids, or cluster_id")
		default:
			writeError(w, http.StatusInternalServerError, "action_plan_failed", err.Error())
		}
		return
	}
	resp := PhotoActionsPlanResponse{
		ActionToken:   token.ID.String(),
		Action:        string(token.Action),
		PhotoCount:    token.PhotoCount,
		BytesEstimate: token.BytesEstimate,
		RequiresText:  token.RequiresText,
		ExpiresAt:     token.ExpiresAt,
	}
	if token.ConfidenceMin != nil || token.ConfidenceMax != nil {
		rng := map[string]float64{}
		if token.ConfidenceMin != nil {
			rng["min"] = *token.ConfidenceMin
		}
		if token.ConfidenceMax != nil {
			rng["max"] = *token.ConfidenceMax
		}
		resp.ConfidenceRange = rng
	}
	if err := h.store.WriteAuditEvent(r.Context(), photolib.AuditEvent{
		Action:    "action_plan",
		Outcome:   "minted",
		Reason:    string(token.Action),
		Actor:     token.ActorID,
		Metadata:  map[string]any{"action_token": token.ID.String(), "photo_count": token.PhotoCount},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		// Audit failure is logged via the standard slog writer in the
		// store; we still respond to the caller because the token was
		// persisted successfully.
		_ = err
	}
	writeJSON(w, http.StatusOK, resp)
}

// ConfirmAction handles `POST /v1/photos/actions/confirm`. Confirmation
// validates scope, expiry, actor, and text-confirmation BEFORE invoking
// any provider mutation. The endpoint records audit events for both
// success (`action_confirm` with outcome=executed) and failure
// (`action_confirm` with outcome=blocked).
func (h *PhotosHandlers) ConfirmAction(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	var request PhotoActionsConfirmRequest
	if !decodeJSONBody(w, r, &request, "invalid_action_confirm", "request body must be JSON") {
		return
	}
	tokenID, err := uuid.Parse(request.ActionToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_action_token", "action_token must be a UUID")
		return
	}
	confirmInput := photolib.ConfirmPhotoActionTokenInput{
		TokenID:          tokenID,
		ActorID:          actorIDFromRequest(r),
		Scope:            request.Scope,
		TextConfirmation: request.TextConfirmation,
	}
	now := time.Now().UTC()
	token, err := h.store.ConfirmActionToken(r.Context(), confirmInput, now)
	if err != nil {
		_ = h.store.WriteAuditEvent(r.Context(), photolib.AuditEvent{
			Action:    "action_confirm",
			Outcome:   "blocked",
			Reason:    err.Error(),
			Actor:     confirmInput.ActorID,
			Metadata:  map[string]any{"action_token": tokenID.String()},
			CreatedAt: now,
		})
		switch {
		case errors.Is(err, photolib.ErrActionTokenNotFound):
			writeError(w, http.StatusNotFound, "action_token_not_found", "action token not found")
		case errors.Is(err, photolib.ErrActionTokenExpired):
			writeError(w, http.StatusConflict, "action_token_expired", "action token expired before confirmation")
		case errors.Is(err, photolib.ErrActionTokenAlreadyConsumed):
			writeError(w, http.StatusConflict, "action_token_consumed", "action token has already been consumed")
		case errors.Is(err, photolib.ErrActionTokenScopeDrift):
			writeError(w, http.StatusConflict, "action_scope_drift", "action token scope does not match request scope")
		case errors.Is(err, photolib.ErrActionTokenActorMismatch):
			writeError(w, http.StatusForbidden, "action_actor_mismatch", "action token belongs to a different actor")
		case errors.Is(err, photolib.ErrActionTokenTextMissing):
			writeError(w, http.StatusBadRequest, "action_text_required", "action requires exact text confirmation")
		default:
			writeError(w, http.StatusInternalServerError, "action_confirm_failed", err.Error())
		}
		return
	}
	// In Scope 3 we record removal-decision rows for every removal_id in
	// the action token scope. Provider mutations against Immich/Telegram
	// are scoped to Scope 4; here we prove the audit + state transition
	// shape so confirm-time guarantees are independently testable.
	providerCalls := 0
	if len(token.Scope.RemovalIDs) > 0 {
		ids := make([]uuid.UUID, 0, len(token.Scope.RemovalIDs))
		for _, raw := range token.Scope.RemovalIDs {
			id, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, "invalid_removal_id", "removal_id must be a UUID")
				return
			}
			ids = append(ids, id)
		}
		decision := decisionForAction(token.Action)
		for _, id := range ids {
			if _, err := h.store.MarkRemovalDecision(r.Context(), id, decision, token.ActorID, token.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "removal_decision_failed", err.Error())
				return
			}
			providerCalls++
		}
	}
	if len(token.Scope.PhotoIDs) > 0 && len(token.Scope.RemovalIDs) == 0 {
		// For non-removal scopes (pure photo IDs) we record one
		// per-photo audit row with the outcome=executed marker so
		// validate-side queries can prove the token executed exactly the
		// scope it was minted with.
		for _, raw := range token.Scope.PhotoIDs {
			id, parseErr := uuid.Parse(raw)
			if parseErr != nil {
				writeError(w, http.StatusBadRequest, "invalid_photo_id", "photo_ids must be UUIDs")
				return
			}
			if err := h.store.WriteAuditEvent(r.Context(), photolib.AuditEvent{
				Action:    "action_confirm",
				PhotoID:   &id,
				Outcome:   "executed",
				Reason:    string(token.Action),
				Actor:     token.ActorID,
				Metadata:  map[string]any{"action_token": token.ID.String()},
				CreatedAt: now,
			}); err != nil {
				writeError(w, http.StatusInternalServerError, "audit_failed", err.Error())
				return
			}
			providerCalls++
		}
	}
	if err := h.store.WriteAuditEvent(r.Context(), photolib.AuditEvent{
		Action:    "action_confirm",
		Outcome:   "executed",
		Reason:    string(token.Action),
		Actor:     token.ActorID,
		Metadata:  map[string]any{"action_token": token.ID.String(), "photo_count": providerCalls},
		CreatedAt: now,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "audit_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, PhotoActionsConfirmResponse{
		ActionToken:   token.ID.String(),
		Action:        string(token.Action),
		PhotoCount:    token.PhotoCount,
		ProviderCalls: providerCalls,
		Outcome:       "executed",
		AuditEventIDs: providerCalls + 1,
	})
}

func (h *PhotosHandlers) actionTokenTTL(action photolib.ActionKind) time.Duration {
	if h == nil {
		return time.Minute
	}
	seconds := 0
	switch action {
	case photolib.ActionDelete:
		seconds = h.config.Policy.DeleteActionTokenTTLSeconds
	case photolib.ActionArchive:
		seconds = h.config.Policy.ArchiveActionTokenTTLSeconds
	}
	// Fall back to the archive TTL for any action without its own policy
	// value (covers ActionAlbumRemove / ActionTag / ActionMarkSensitive /
	// ActionFavorite plus the case where a configured action TTL is 0).
	if seconds <= 0 {
		seconds = h.config.Policy.ArchiveActionTokenTTLSeconds
	}
	if seconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func decisionForAction(kind photolib.ActionKind) string {
	switch kind {
	case photolib.ActionDelete:
		return "deleted"
	case photolib.ActionArchive:
		return "archived"
	case photolib.ActionMarkSensitive:
		return "exempted"
	default:
		return "kept"
	}
}

func actorIDFromRequest(r *http.Request) string {
	// The runtime bearer-token middleware sets the actor in a header for
	// downstream handlers; fall back to "system" when no header is set
	// (test/internal callers).
	if r == nil {
		return "system"
	}
	if value := r.Header.Get("X-Actor-Id"); value != "" {
		return value
	}
	return "system"
}

// SetClusterBestPick handles `POST /v1/photos/health/duplicates/{id}/best-pick`.
type clusterBestPickRequest struct {
	PhotoID  string `json:"photo_id"`
	PickedBy string `json:"picked_by,omitempty"`
}

func (h *PhotosHandlers) SetClusterBestPick(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	clusterID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cluster_id", "cluster id must be a UUID")
		return
	}
	var request clusterBestPickRequest
	if !decodeJSONBody(w, r, &request, "invalid_best_pick", "request body must be JSON") {
		return
	}
	photoID, err := uuid.Parse(request.PhotoID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_photo_id", "photo_id must be a UUID")
		return
	}
	pickedBy := request.PickedBy
	if pickedBy == "" {
		pickedBy = "user"
	}
	cluster, err := h.store.SetBestPick(r.Context(), clusterID, photoID, pickedBy, actorIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, "set_best_pick_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

// ResolveCluster handles `POST /v1/photos/health/duplicates/{id}/resolve`.
type clusterResolveRequest struct {
	Action      string `json:"action"`
	ActionToken string `json:"action_token,omitempty"`
}

func (h *PhotosHandlers) ResolveCluster(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	clusterID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cluster_id", "cluster id must be a UUID")
		return
	}
	var request clusterResolveRequest
	if !decodeJSONBody(w, r, &request, "invalid_resolve", "request body must be JSON") {
		return
	}
	if request.Action == "" {
		writeError(w, http.StatusBadRequest, "missing_action", "resolve action is required")
		return
	}
	if request.Action == "archive_non_best" || request.Action == "delete_non_best" {
		if request.ActionToken == "" {
			writeError(w, http.StatusConflict, "action_token_required", "destructive cluster resolution requires action_token")
			return
		}
	}
	cluster, err := h.store.ResolveCluster(r.Context(), clusterID, request.Action, actorIDFromRequest(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "resolve_cluster_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

// HealthLifecycle handles `GET /v1/photos/health/lifecycle`.
func (h *PhotosHandlers) HealthLifecycle(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	threshold := h.config.Policy.LifecycleConfirmationThreshold
	summary, err := h.store.SummarizeLifecycle(r.Context(), threshold, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lifecycle_summary_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// HealthDuplicates handles `GET /v1/photos/health/duplicates`. Filters
// are supplied via query parameters: `kind` (optional) and `state`
// (defaults to `open`).
func (h *PhotosHandlers) HealthDuplicates(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "open"
	}
	kind := r.URL.Query().Get("kind")
	var clusters []photolib.PhotoCluster
	var err error
	if kind != "" {
		clusters, err = h.store.ListClustersByKind(r.Context(), photolib.ClusterKind(kind), state)
	} else {
		clusters, err = h.store.ListClusters(r.Context(), state)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "duplicates_query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": clusters, "total": len(clusters)})
}

// HealthDuplicatesGet handles `GET /v1/photos/health/duplicates/{id}`.
func (h *PhotosHandlers) HealthDuplicatesGet(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cluster_id", "cluster id must be a UUID")
		return
	}
	cluster, err := h.store.GetCluster(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "cluster_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cluster)
}

// HealthRemoval handles `GET /v1/photos/health/removal`. The status
// filter defaults to `pending_review` so the dashboard surfaces only
// open work.
func (h *PhotosHandlers) HealthRemoval(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending_review"
	}
	candidates, err := h.store.ListRemovalCandidates(r.Context(), status, 200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "removal_query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates, "total": len(candidates)})
}

// HealthQuality handles `GET /v1/photos/health/quality`. Scope-3 returns
// a derived breakdown from the existing classification rows so the UI
// has live data without waiting for the dedicated aesthetic pipeline.
func (h *PhotosHandlers) HealthQuality(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos_store_unavailable", "photo store is unavailable")
		return
	}
	rows, err := h.store.QualityHistogram(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "quality_query_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": rows})
}

// MarshalActionPlanForTest exposes the response shape used by integration tests.
func MarshalActionPlanForTest(token *photolib.ActionToken) ([]byte, error) {
	if token == nil {
		return nil, errors.New("nil token")
	}
	return json.Marshal(PhotoActionsPlanResponse{
		ActionToken:   token.ID.String(),
		Action:        string(token.Action),
		PhotoCount:    token.PhotoCount,
		BytesEstimate: token.BytesEstimate,
		RequiresText:  token.RequiresText,
		ExpiresAt:     token.ExpiresAt,
	})
}
