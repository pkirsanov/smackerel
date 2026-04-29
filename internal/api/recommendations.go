package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
