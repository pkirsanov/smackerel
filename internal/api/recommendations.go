package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
	"github.com/smackerel/smackerel/internal/recommendation/reactive"
	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

const recommendationReactiveScenarioID = reactive.ScenarioID

// RecommendationRequestStore is the persistence boundary used by API handlers.
type RecommendationRequestStore interface {
	CreateNoProviderRequest(ctx context.Context, input recstore.CreateRequestInput) (recstore.RequestRecord, error)
}

// RecommendationProviderRegistry is the narrow registry surface needed by the
// Scope 1 no-provider path.
type RecommendationProviderRegistry interface {
	Len() int
	List() []recprovider.Provider
}

// RecommendationHandlers exposes the typed recommendation API surface.
type RecommendationHandlers struct {
	store    RecommendationRequestStore
	registry RecommendationProviderRegistry
	cfg      config.RecommendationsConfig
}

// NewRecommendationHandlers wires the typed recommendation handlers.
func NewRecommendationHandlers(store RecommendationRequestStore, registry RecommendationProviderRegistry, cfg config.RecommendationsConfig) *RecommendationHandlers {
	if store == nil {
		panic("api: recommendation store is required")
	}
	if registry == nil {
		panic("api: recommendation provider registry is required")
	}
	return &RecommendationHandlers{store: store, registry: registry, cfg: cfg}
}

type createRecommendationRequest struct {
	Query           string   `json:"query"`
	Source          string   `json:"source"`
	LocationRef     string   `json:"location_ref"`
	NamedLocation   string   `json:"named_location"`
	PrecisionPolicy string   `json:"precision_policy"`
	Style           string   `json:"style"`
	ResultCount     *int     `json:"result_count"`
	AllowedSources  []string `json:"allowed_sources"`
}

type createRecommendationResponse struct {
	RequestID       string                            `json:"request_id"`
	Status          string                            `json:"status"`
	TraceID         string                            `json:"trace_id"`
	Recommendations []recstore.RenderedRecommendation `json:"recommendations"`
	Clarification   *recstore.Clarification           `json:"clarification,omitempty"`
}

type recommendationFeedbackRequest struct {
	FeedbackType   string         `json:"feedback_type"`
	SourceWatchID  string         `json:"source_watch_id"`
	PreferenceKey  string         `json:"preference_key"`
	CorrectionKind string         `json:"correction_kind"`
	Payload        map[string]any `json:"payload"`
}

type preferenceCorrectionRequest struct {
	CorrectionKind string         `json:"correction_kind"`
	Payload        map[string]any `json:"payload"`
}

// CreateRequest handles POST /api/recommendations/requests. Scope 1 persists
// and returns the no-provider outcome without invoking any provider calls.
func (h *RecommendationHandlers) CreateRequest(w http.ResponseWriter, r *http.Request) {
	var req createRecommendationRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid recommendation JSON")
		return
	}
	if err := h.validateCreateRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_recommendation_request", err.Error())
		return
	}

	style := req.Style
	if style == "" {
		style = h.cfg.Ranking.StandardStyle
	}
	resultCount := h.cfg.Ranking.StandardResultCount
	if req.ResultCount != nil {
		resultCount = *req.ResultCount
	}

	if h.registry.Len() > 0 {
		store, ok := h.store.(*recstore.Store)
		if !ok {
			writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "reactive recommendation store is unavailable")
			return
		}
		engine := reactive.NewEngine(reactive.Options{Store: store, Registry: h.registry, Config: h.cfg})
		outcome, err := engine.Run(r.Context(), reactive.Request{
			ActorUserID:     "local",
			Source:          req.Source,
			Query:           req.Query,
			LocationRef:     req.LocationRef,
			NamedLocation:   req.NamedLocation,
			PrecisionPolicy: recommendation.PrecisionPolicy(req.PrecisionPolicy),
			Style:           style,
			ResultCount:     resultCount,
			AllowedSources:  req.AllowedSources,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "recommendation_run_failed", "failed to run recommendation scenario")
			return
		}
		writeJSON(w, http.StatusOK, createRecommendationResponse{
			RequestID:       outcome.ID,
			Status:          outcome.Status,
			TraceID:         outcome.TraceID,
			Recommendations: outcome.Recommendations,
			Clarification:   outcome.Clarification,
		})
		return
	}

	record, err := h.store.CreateNoProviderRequest(r.Context(), recstore.CreateRequestInput{
		ActorUserID:                "local",
		Source:                     req.Source,
		ScenarioID:                 recommendationReactiveScenarioID,
		RawInput:                   req.Query,
		LocationPrecisionRequested: req.PrecisionPolicy,
		LocationPrecisionSent:      req.PrecisionPolicy,
		Status:                     "no_providers",
		ParsedRequest: map[string]any{
			"query":            req.Query,
			"source":           req.Source,
			"location_ref":     req.LocationRef,
			"named_location":   req.NamedLocation,
			"precision_policy": req.PrecisionPolicy,
			"style":            style,
			"result_count":     resultCount,
			"allowed_sources":  req.AllowedSources,
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recommendation_persist_failed", "failed to persist recommendation request")
		return
	}

	writeJSON(w, http.StatusOK, createRecommendationResponse{
		RequestID:       record.ID,
		Status:          record.Status,
		TraceID:         record.TraceID,
		Recommendations: []recstore.RenderedRecommendation{},
	})
}

// GetRequest handles GET /api/recommendations/requests/{id}.
func (h *RecommendationHandlers) GetRequest(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	requestID := strings.TrimSpace(chi.URLParam(r, "id"))
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "missing_request_id", "request id is required")
		return
	}
	outcome, err := store.GetRequest(r.Context(), requestID)
	if err != nil {
		writeError(w, http.StatusNotFound, "recommendation_request_not_found", "recommendation request not found")
		return
	}
	writeJSON(w, http.StatusOK, outcome)
}

// GetRecommendation handles GET /api/recommendations/{id}.
func (h *RecommendationHandlers) GetRecommendation(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	recommendationID := strings.TrimSpace(chi.URLParam(r, "id"))
	if recommendationID == "" {
		writeError(w, http.StatusBadRequest, "missing_recommendation_id", "recommendation id is required")
		return
	}
	recommendation, err := store.GetRecommendation(r.Context(), recommendationID)
	if err != nil {
		writeError(w, http.StatusNotFound, "recommendation_not_found", "recommendation not found")
		return
	}
	writeJSON(w, http.StatusOK, recommendation)
}

// GetWhy handles GET /api/recommendations/{id}/why.
func (h *RecommendationHandlers) GetWhy(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	recommendationID := strings.TrimSpace(chi.URLParam(r, "id"))
	if recommendationID == "" {
		writeError(w, http.StatusBadRequest, "missing_recommendation_id", "recommendation id is required")
		return
	}
	why, err := store.ExplainRecommendation(r.Context(), recommendationID)
	if err != nil {
		writeError(w, http.StatusNotFound, "recommendation_why_not_found", "recommendation why explanation not found")
		return
	}
	writeJSON(w, http.StatusOK, why)
}

// RecordFeedback handles POST /api/recommendations/{id}/feedback.
func (h *RecommendationHandlers) RecordFeedback(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	recommendationID := strings.TrimSpace(chi.URLParam(r, "id"))
	if recommendationID == "" {
		writeError(w, http.StatusBadRequest, "missing_recommendation_id", "recommendation id is required")
		return
	}
	var req recommendationFeedbackRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid recommendation feedback JSON")
		return
	}
	result, err := store.RecordFeedback(r.Context(), recstore.FeedbackInput{
		RecommendationID: recommendationID,
		ActorUserID:      "local",
		FeedbackType:     req.FeedbackType,
		SourceWatchID:    req.SourceWatchID,
		PreferenceKey:    req.PreferenceKey,
		CorrectionKind:   req.CorrectionKind,
		Payload:          req.Payload,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "recommendation_feedback_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ListPreferences handles GET /api/recommendations/preferences.
func (h *RecommendationHandlers) ListPreferences(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	view, err := store.ListPreferences(r.Context(), "local")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recommendation_preferences_failed", "failed to list recommendation preferences")
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// CreatePreferenceCorrection handles POST /api/recommendations/preferences/{key}/corrections.
func (h *RecommendationHandlers) CreatePreferenceCorrection(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	preferenceKey := strings.TrimSpace(chi.URLParam(r, "key"))
	if preferenceKey == "" {
		writeError(w, http.StatusBadRequest, "missing_preference_key", "preference key is required")
		return
	}
	var req preferenceCorrectionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid preference correction JSON")
		return
	}
	correction, err := store.CreatePreferenceCorrection(r.Context(), recstore.CreatePreferenceCorrectionInput{
		ActorUserID:    "local",
		PreferenceKey:  preferenceKey,
		CorrectionKind: req.CorrectionKind,
		Payload:        req.Payload,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "preference_correction_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, correction)
}

// RevokePreferenceCorrection handles DELETE /api/recommendations/preferences/{key}/corrections/{correctionID}.
func (h *RecommendationHandlers) RevokePreferenceCorrection(w http.ResponseWriter, r *http.Request) {
	store, ok := h.store.(*recstore.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "recommendation_store_unavailable", "recommendation store is unavailable")
		return
	}
	preferenceKey := strings.TrimSpace(chi.URLParam(r, "key"))
	correctionID := strings.TrimSpace(chi.URLParam(r, "correctionID"))
	if preferenceKey == "" || correctionID == "" {
		writeError(w, http.StatusBadRequest, "missing_preference_correction", "preference key and correction id are required")
		return
	}
	if err := store.RevokePreferenceCorrection(r.Context(), "local", preferenceKey, correctionID); err != nil {
		writeError(w, http.StatusNotFound, "preference_correction_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked", "correction_id": correctionID})
}

// providerView is the sanitized end-user view of one configured provider.
// This shape is what `GET /api/recommendations/providers` returns by default.
// The end-user view MUST NOT include API keys, quota state, last-error
// strings, or any operator-only telemetry.
type providerView struct {
	ProviderID  string   `json:"provider_id"`
	DisplayName string   `json:"display_name"`
	Categories  []string `json:"categories"`
	Status      string   `json:"status"`
}

// providerOperatorView is the operator-detail view returned when the caller
// explicitly requests `?view=operator`. It SHOULD NOT include API keys —
// keys live only in SST and are NEVER surfaced over HTTP — but it MAY
// include observed status reasons, observed_at timestamps, attribution
// labels, and quota window settings sourced from config (not from runtime
// secrets).
type providerOperatorView struct {
	providerView
	Reason               string   `json:"reason,omitempty"`
	ObservedAt           string   `json:"observed_at"`
	AttributionLabel     string   `json:"attribution_label,omitempty"`
	QuotaWindowSeconds   int      `json:"quota_window_seconds,omitempty"`
	MaxRequestsWindow    int      `json:"max_requests_per_window,omitempty"`
	ConfiguredCategories []string `json:"configured_categories,omitempty"`
}

// ListProviders handles GET /api/recommendations/providers. The default
// response is the sanitized end-user view; operators may pass
// `?view=operator` to request the detailed view that includes provider
// reasons, attribution labels, and config-sourced quota windows. API keys
// MUST NEVER be included in either view.
func (h *RecommendationHandlers) ListProviders(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"providers": []any{}})
		return
	}
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	providers := h.registry.List()
	if view == "operator" {
		out := make([]providerOperatorView, 0, len(providers))
		for _, p := range providers {
			health := p.Health(r.Context())
			cats := make([]string, 0, len(p.Categories()))
			for _, c := range p.Categories() {
				cats = append(cats, string(c))
			}
			detail := providerOperatorView{
				providerView: providerView{
					ProviderID:  p.ID(),
					DisplayName: p.DisplayName(),
					Categories:  cats,
					Status:      string(health.Status),
				},
				Reason:     health.Reason,
				ObservedAt: health.ObservedAt.UTC().Format(time.RFC3339),
			}
			detail = enrichOperatorViewFromConfig(detail, h.cfg)
			out = append(out, detail)
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": out, "view": "operator"})
		return
	}
	out := make([]providerView, 0, len(providers))
	for _, p := range providers {
		health := p.Health(r.Context())
		cats := make([]string, 0, len(p.Categories()))
		for _, c := range p.Categories() {
			cats = append(cats, string(c))
		}
		out = append(out, providerView{
			ProviderID:  p.ID(),
			DisplayName: p.DisplayName(),
			Categories:  cats,
			Status:      string(health.Status),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out, "view": "sanitized"})
}

// enrichOperatorViewFromConfig populates the operator-only fields by mapping
// the configured providers under recommendations.providers. Returns the view
// unchanged when no matching provider config exists. NEVER reads API keys.
func enrichOperatorViewFromConfig(view providerOperatorView, cfg config.RecommendationsConfig) providerOperatorView {
	candidates := map[string]config.RecommendationProviderConfig{
		"google_places":         cfg.Providers.GooglePlaces,
		"yelp":                  cfg.Providers.Yelp,
		"fixture_google_places": cfg.Providers.GooglePlaces,
		"fixture_yelp":          cfg.Providers.Yelp,
	}
	pc, ok := candidates[view.ProviderID]
	if !ok {
		return view
	}
	view.AttributionLabel = pc.AttributionLabel
	view.QuotaWindowSeconds = pc.QuotaWindowSeconds
	view.MaxRequestsWindow = pc.MaxRequestsPerWindow
	view.ConfiguredCategories = append([]string(nil), pc.Categories...)
	return view
}

func (h *RecommendationHandlers) validateCreateRequest(req createRecommendationRequest) error {
	if strings.TrimSpace(req.Query) == "" {
		return fmt.Errorf("query is required")
	}
	switch req.Source {
	case "web", "telegram", "api":
	default:
		return fmt.Errorf("source must be one of web, telegram, api")
	}
	precision := recommendation.PrecisionPolicy(req.PrecisionPolicy)
	if err := precision.Validate(); err != nil {
		return fmt.Errorf("precision_policy is required and must be one of exact, neighborhood, city")
	}
	style := req.Style
	if style == "" {
		style = h.cfg.Ranking.StandardStyle
	}
	switch style {
	case "balanced", "familiar", "novel":
	default:
		return fmt.Errorf("style must be one of balanced, familiar, novel")
	}
	resultCount := h.cfg.Ranking.StandardResultCount
	if req.ResultCount != nil {
		resultCount = *req.ResultCount
	}
	if resultCount < 1 || resultCount > 10 {
		return fmt.Errorf("result_count must be between 1 and 10")
	}
	if len(req.AllowedSources) > 0 && h.registry.Len() == 0 {
		return fmt.Errorf("allowed_sources cannot name providers when no providers are registered")
	}
	return nil
}
