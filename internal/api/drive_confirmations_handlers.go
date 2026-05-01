package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/drive/confirm"
	"github.com/smackerel/smackerel/internal/metrics"
)

// DriveConfirmationsHandlers exposes Spec 038 Scope 6
// /v1/drive/confirmations/{id} resolution endpoints (Screen 11 web
// modal + Telegram numbered replies route here).
type DriveConfirmationsHandlers struct {
	store ConfirmationsStore
}

// ConfirmationsStore is the slice of confirm.Store the HTTP handler
// needs. The interface lets tests substitute confirm.MemoryStore without
// dragging in pgxpool.
type ConfirmationsStore interface {
	Get(ctx context.Context, id string) (confirm.Confirmation, error)
	Resolve(ctx context.Context, id string, channel confirm.Channel, choice confirm.Choice) (confirm.Confirmation, error)
}

// NewDriveConfirmationsHandlers builds the handler set.
func NewDriveConfirmationsHandlers(store ConfirmationsStore) *DriveConfirmationsHandlers {
	if store == nil {
		return nil
	}
	return &DriveConfirmationsHandlers{store: store}
}

// DriveConfirmationView is the JSON shape for one drive_confirmation.
type DriveConfirmationView struct {
	ID               string                       `json:"id"`
	Kind             string                       `json:"kind"`
	SourceArtifactID string                       `json:"source_artifact_id"`
	SaveRequestID    string                       `json:"save_request_id,omitempty"`
	RuleID           string                       `json:"rule_id,omitempty"`
	Status           string                       `json:"status"`
	Channel          string                       `json:"channel,omitempty"`
	Payload          DriveConfirmationPayloadView `json:"payload"`
	Choice           DriveConfirmationChoiceView  `json:"choice"`
	DecidedAt        string                       `json:"decided_at,omitempty"`
	ExpiresAt        string                       `json:"expires_at"`
	CreatedAt        string                       `json:"created_at"`
}

// DriveConfirmationPayloadView mirrors confirm.Payload.
type DriveConfirmationPayloadView struct {
	Classification string  `json:"classification,omitempty"`
	Sensitivity    string  `json:"sensitivity,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	RenderedPath   string  `json:"rendered_path,omitempty"`
	Title          string  `json:"title,omitempty"`
	ProviderID     string  `json:"provider_id,omitempty"`
	RuleID         string  `json:"rule_id,omitempty"`
}

// DriveConfirmationChoiceView mirrors confirm.Choice.
type DriveConfirmationChoiceView struct {
	Outcome           string `json:"outcome"`
	NewClassification string `json:"new_classification,omitempty"`
	NewRuleID         string `json:"new_rule_id,omitempty"`
	NewRenderedPath   string `json:"new_rendered_path,omitempty"`
	NewSensitivity    string `json:"new_sensitivity,omitempty"`
	NoSaveReason      string `json:"no_save_reason,omitempty"`
}

// DriveConfirmationResolveRequest is the body for POST
// /v1/drive/confirmations/{id}.
type DriveConfirmationResolveRequest struct {
	Channel string                      `json:"channel"`
	Choice  DriveConfirmationChoiceView `json:"choice"`
}

// Get handles GET /v1/drive/confirmations/{id}. It returns the current
// confirmation row including status (pending|committed|...). Screen 11
// and Telegram fallbacks call Get to render the prompt before the user
// submits a choice.
func (h *DriveConfirmationsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing confirmation id")
		return
	}
	c, err := h.store.Get(r.Context(), id)
	if errors.Is(err, confirm.ErrNotFound) {
		writeError(w, http.StatusNotFound, "CONFIRMATION_NOT_FOUND", "no confirmation with id "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, confirmationToView(c))
}

// Resolve handles POST /v1/drive/confirmations/{id}. The handler is
// idempotent: re-posting the same choice for an already-resolved row
// returns the existing row with HTTP 409 (Conflict) so callers can
// distinguish "I won the race" from "someone else already answered".
//
// Successful resolution writes the user's choice and returns HTTP 200
// with the updated row. Expired rows return HTTP 410 (Gone).
func (h *DriveConfirmationsHandlers) Resolve(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing confirmation id")
		return
	}
	var req DriveConfirmationResolveRequest
	if !decodeJSONBody(w, r, &req, "INVALID_REQUEST", "invalid JSON body") {
		return
	}
	channel := confirm.Channel(strings.TrimSpace(req.Channel))
	if channel != confirm.ChannelWeb && channel != confirm.ChannelTelegram {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "channel must be 'web' or 'telegram'")
		return
	}
	choice := confirm.Choice{
		Outcome:           confirm.Outcome(strings.TrimSpace(req.Choice.Outcome)),
		NewClassification: req.Choice.NewClassification,
		NewRuleID:         req.Choice.NewRuleID,
		NewRenderedPath:   req.Choice.NewRenderedPath,
		NewSensitivity:    req.Choice.NewSensitivity,
		NoSaveReason:      req.Choice.NoSaveReason,
	}
	resolved, err := h.store.Resolve(r.Context(), id, channel, choice)
	if errors.Is(err, confirm.ErrNotFound) {
		writeError(w, http.StatusNotFound, "CONFIRMATION_NOT_FOUND", "no confirmation with id "+id)
		return
	}
	if errors.Is(err, confirm.ErrAlreadyResolved) {
		metrics.DriveConfirmationsTotal.WithLabelValues("already_resolved", string(channel)).Inc()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(confirmationToView(resolved))
		return
	}
	if errors.Is(err, confirm.ErrExpired) {
		metrics.DriveConfirmationsTotal.WithLabelValues("expired", string(channel)).Inc()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(confirmationToView(resolved))
		return
	}
	if errors.Is(err, confirm.ErrInvalidChoice) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "choice.outcome must be one of commit|reroute|no_save")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "RESOLVE_FAILED", err.Error())
		return
	}
	metrics.DriveConfirmationsTotal.WithLabelValues(string(resolved.Status), string(channel)).Inc()
	writeJSON(w, http.StatusOK, confirmationToView(resolved))
}

func confirmationToView(c confirm.Confirmation) DriveConfirmationView {
	view := DriveConfirmationView{
		ID:               c.ID,
		Kind:             string(c.Kind),
		SourceArtifactID: c.SourceArtifactID,
		SaveRequestID:    c.SaveRequestID,
		RuleID:           c.RuleID,
		Status:           string(c.Status),
		Channel:          string(c.Channel),
		Payload: DriveConfirmationPayloadView{
			Classification: c.Payload.Classification,
			Sensitivity:    c.Payload.Sensitivity,
			Confidence:     c.Payload.Confidence,
			RenderedPath:   c.Payload.RenderedPath,
			Title:          c.Payload.Title,
			ProviderID:     c.Payload.ProviderID,
			RuleID:         c.Payload.RuleID,
		},
		Choice: DriveConfirmationChoiceView{
			Outcome:           string(c.Choice.Outcome),
			NewClassification: c.Choice.NewClassification,
			NewRuleID:         c.Choice.NewRuleID,
			NewRenderedPath:   c.Choice.NewRenderedPath,
			NewSensitivity:    c.Choice.NewSensitivity,
			NoSaveReason:      c.Choice.NoSaveReason,
		},
		ExpiresAt: c.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		CreatedAt: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
	if !c.DecidedAt.IsZero() {
		view.DecidedAt = c.DecidedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return view
}
