package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// maxQueryParamLen limits the maximum length of query parameter strings.
const maxQueryParamLen = 1000

// parseListParams extracts and validates q, sort, limit, offset from query params.
func parseListParams(r *http.Request) (q, sort string, limit, offset int, err string) {
	q = r.URL.Query().Get("q")
	if len(q) > maxQueryParamLen {
		return "", "", 0, 0, "Query parameter 'q' exceeds maximum length of 1000 characters"
	}

	sort = r.URL.Query().Get("sort")

	limit = 20
	if l := r.URL.Query().Get("limit"); l != "" {
		v, e := strconv.Atoi(l)
		if e != nil || v < 1 || v > 100 {
			return "", "", 0, 0, "Parameter 'limit' must be an integer between 1 and 100"
		}
		limit = v
	}

	offset = 0
	if o := r.URL.Query().Get("offset"); o != "" {
		v, e := strconv.Atoi(o)
		if e != nil || v < 0 {
			return "", "", 0, 0, "Parameter 'offset' must be a non-negative integer"
		}
		offset = v
	}

	return q, sort, limit, offset, ""
}

// ConceptListItem is a single concept in a list response.
type ConceptListItem struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary"`
	CitationCount     int      `json:"citation_count"`
	EntityCount       int      `json:"entity_count"`
	SourceTypes       []string `json:"source_types"`
	HasContradictions bool     `json:"has_contradictions"`
	UpdatedAt         string   `json:"updated_at"`
}

// ConceptListResponse is the response for GET /api/knowledge/concepts.
type ConceptListResponse struct {
	Concepts []ConceptListItem `json:"concepts"`
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

// EntityListItem is a single entity in a list response.
type EntityListItem struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	EntityType       string   `json:"entity_type"`
	Summary          string   `json:"summary"`
	SourceTypes      []string `json:"source_types"`
	InteractionCount int      `json:"interaction_count"`
	UpdatedAt        string   `json:"updated_at"`
}

// EntityListResponse is the response for GET /api/knowledge/entities.
type EntityListResponse struct {
	Entities []EntityListItem `json:"entities"`
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
}

// KnowledgeConceptsHandler handles GET /api/knowledge/concepts.
func (d *Dependencies) KnowledgeConceptsHandler(w http.ResponseWriter, r *http.Request) {
	if d.KnowledgeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "Knowledge layer is not enabled")
		return
	}

	q, sort, limit, offset, errMsg := parseListParams(r)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", errMsg)
		return
	}

	concepts, total, err := d.KnowledgeStore.ListConceptsFiltered(r.Context(), q, sort, limit, offset)
	if err != nil {
		slog.Error("list concepts failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list concepts")
		return
	}

	items := make([]ConceptListItem, 0, len(concepts))
	for _, c := range concepts {
		items = append(items, ConceptListItem{
			ID:            c.ID,
			Title:         c.Title,
			Summary:       c.Summary,
			CitationCount: len(c.SourceArtifactIDs),
			SourceTypes:   c.SourceTypeDiversity,
			UpdatedAt:     c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, ConceptListResponse{
		Concepts: items,
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	})
}

// KnowledgeConceptDetailHandler handles GET /api/knowledge/concepts/{id}.
func (d *Dependencies) KnowledgeConceptDetailHandler(w http.ResponseWriter, r *http.Request) {
	if d.KnowledgeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "Knowledge layer is not enabled")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "Concept ID is required")
		return
	}

	concept, err := d.KnowledgeStore.GetConceptByID(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Concept not found")
			return
		}
		slog.Error("get concept failed", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get concept")
		return
	}

	writeJSON(w, http.StatusOK, concept)
}

// KnowledgeEntitiesHandler handles GET /api/knowledge/entities.
func (d *Dependencies) KnowledgeEntitiesHandler(w http.ResponseWriter, r *http.Request) {
	if d.KnowledgeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "Knowledge layer is not enabled")
		return
	}

	q, sort, limit, offset, errMsg := parseListParams(r)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", errMsg)
		return
	}

	entities, total, err := d.KnowledgeStore.ListEntitiesFiltered(r.Context(), q, sort, limit, offset)
	if err != nil {
		slog.Error("list entities failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list entities")
		return
	}

	items := make([]EntityListItem, 0, len(entities))
	for _, e := range entities {
		items = append(items, EntityListItem{
			ID:               e.ID,
			Name:             e.Name,
			EntityType:       e.EntityType,
			Summary:          e.Summary,
			SourceTypes:      e.SourceTypes,
			InteractionCount: e.InteractionCount,
			UpdatedAt:        e.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, EntityListResponse{
		Entities: items,
		Total:    total,
		Limit:    limit,
		Offset:   offset,
	})
}

// KnowledgeEntityDetailHandler handles GET /api/knowledge/entities/{id}.
func (d *Dependencies) KnowledgeEntityDetailHandler(w http.ResponseWriter, r *http.Request) {
	if d.KnowledgeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "Knowledge layer is not enabled")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "Entity ID is required")
		return
	}

	entity, err := d.KnowledgeStore.GetEntityByID(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Entity not found")
			return
		}
		slog.Error("get entity failed", "error", err, "id", id)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get entity")
		return
	}

	writeJSON(w, http.StatusOK, entity)
}

// KnowledgeLintHandler handles GET /api/knowledge/lint.
func (d *Dependencies) KnowledgeLintHandler(w http.ResponseWriter, r *http.Request) {
	if d.KnowledgeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "Knowledge layer is not enabled")
		return
	}

	report, err := d.KnowledgeStore.GetLatestLintReport(r.Context())
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "NO_LINT_REPORT", "No lint report exists yet")
			return
		}
		slog.Error("get lint report failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get lint report")
		return
	}

	writeJSON(w, http.StatusOK, report)
}

// KnowledgeStatsHandler handles GET /api/knowledge/stats.
func (d *Dependencies) KnowledgeStatsHandler(w http.ResponseWriter, r *http.Request) {
	if d.KnowledgeStore == nil {
		writeError(w, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "Knowledge layer is not enabled")
		return
	}

	stats, err := d.KnowledgeStore.GetStats(r.Context())
	if err != nil {
		slog.Error("get knowledge stats failed", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get knowledge stats")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// isNotFound checks if an error indicates a not-found condition.
func isNotFound(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
