package graphapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PersonRow is the wire shape for a single row in the people list
// response (SCN-080-03).
type PersonRow struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	ArtifactCount int    `json:"artifactCount"`
}

// ArtifactTimelineEntry is one entry in PersonDetail.ArtifactTimeline.
type ArtifactTimelineEntry struct {
	ArtifactID string    `json:"artifactId"`
	Title      string    `json:"title"`
	CapturedAt time.Time `json:"capturedAt"`
}

// PersonDetail is the wire shape for GET /api/people/{id} (SCN-080-04).
type PersonDetail struct {
	ID               string                  `json:"id"`
	DisplayName      string                  `json:"displayName"`
	ArtifactTimeline []ArtifactTimelineEntry `json:"artifactTimeline"`
	RelatedTopics    []CrossLink             `json:"relatedTopics"`
	RelatedPlaces    []CrossLink             `json:"relatedPlaces"`
}

type peopleListResponse struct {
	Items      []PersonRow `json:"items"`
	NextCursor string      `json:"nextCursor"`
}

// PeopleSource is the data-access boundary used by PeopleHandlers.
type PeopleSource interface {
	ListPeople(ctx context.Context, limit, offset int) (rows []PersonRow, hasNext bool, err error)
	GetPerson(ctx context.Context, id string) (*PersonDetail, error)
}

// PeopleHandlers serves the spec 080 people endpoints.
type PeopleHandlers struct {
	Source PeopleSource
	Limits Limits
	Codec  *CursorCodec
}

// NewPeopleHandlers wires a PeopleHandlers against a live pgx pool.
// The intelligence layer derives people via the same `edges` /
// `people` tables that internal/intelligence.GetPeopleIntelligence
// uses; this adapter is the minimal source-of-truth for the spec 080
// list/detail wire contract (the full intelligence dossier remains in
// /api/expertise — this surface is graph-shaped only).
func NewPeopleHandlers(pool *pgxpool.Pool, limits Limits, codec *CursorCodec) *PeopleHandlers {
	return &PeopleHandlers{Source: &pgxPeopleSource{pool: pool}, Limits: limits, Codec: codec}
}

// ErrPersonNotFound signals that GetPerson could not resolve {id}.
var ErrPersonNotFound = errors.New("graphapi: person not found")

// ListPeople handles GET /api/people.
func (h *PeopleHandlers) ListPeople(w http.ResponseWriter, r *http.Request) {
	limit, offset, cursorErr := parseListPagination(r, h.Limits, h.Codec, "people")
	if cursorErr != nil {
		WriteAPIError(w, cursorErr)
		return
	}

	rows, hasNext, err := h.Source.ListPeople(r.Context(), limit, offset)
	if err != nil {
		slog.Error("graphapi: list people failed", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to list people")
		return
	}
	if rows == nil {
		rows = []PersonRow{}
	}

	next := ""
	if hasNext && h.Codec != nil {
		encoded, encErr := h.Codec.Encode(CursorPayload{
			Resource: "people",
			Offset:   int64(offset + len(rows)),
		})
		if encErr == nil {
			next = encoded
		}
	}

	writeJSON(w, http.StatusOK, peopleListResponse{Items: rows, NextCursor: next})
}

// GetPerson handles GET /api/people/{id}.
func (h *PeopleHandlers) GetPerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		WriteAPIError(w, ErrMissingParam.WithField("id"))
		return
	}
	detail, err := h.Source.GetPerson(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPersonNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "id", "person not found")
			return
		}
		slog.Error("graphapi: get person failed", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to load person")
		return
	}
	if detail.ArtifactTimeline == nil {
		detail.ArtifactTimeline = []ArtifactTimelineEntry{}
	}
	if detail.RelatedTopics == nil {
		detail.RelatedTopics = []CrossLink{}
	}
	if detail.RelatedPlaces == nil {
		detail.RelatedPlaces = []CrossLink{}
	}
	writeJSON(w, http.StatusOK, detail)
}

// pgxPeopleSource is the production PeopleSource backed by Postgres.
type pgxPeopleSource struct{ pool *pgxpool.Pool }

const peopleListSQL = `
SELECT p.id,
       COALESCE(p.name, p.id) AS display_name,
       COUNT(DISTINCT e.src_id) AS artifact_count
  FROM people p
  LEFT JOIN edges e
    ON e.dst_id = p.id AND e.dst_type = 'person' AND e.src_type = 'artifact'
 GROUP BY p.id, p.name
 ORDER BY artifact_count DESC, p.name ASC
 LIMIT $1 OFFSET $2
`

func (s *pgxPeopleSource) ListPeople(ctx context.Context, limit, offset int) ([]PersonRow, bool, error) {
	if s.pool == nil {
		return nil, false, errors.New("graphapi: people pool is nil")
	}
	rows, err := s.pool.Query(ctx, peopleListSQL, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]PersonRow, 0, limit)
	for rows.Next() {
		var p PersonRow
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.ArtifactCount); err != nil {
			return nil, false, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasNext := len(out) > limit
	if hasNext {
		out = out[:limit]
	}
	return out, hasNext, nil
}

const personSelectSQL = `SELECT id, COALESCE(name, id) FROM people WHERE id = $1`

const personTimelineSQL = `
SELECT a.id, COALESCE(a.title, a.id), a.created_at
  FROM edges e
  JOIN artifacts a ON a.id = e.src_id
 WHERE e.dst_type = 'person' AND e.dst_id = $1 AND e.src_type = 'artifact'
 ORDER BY a.created_at DESC NULLS LAST, a.id ASC
 LIMIT 50
`

const personRelatedTopicsSQL = `
SELECT t.id, COALESCE(t.name, t.id)
  FROM edges et
  JOIN edges ep ON ep.src_id = et.src_id AND ep.src_type = et.src_type AND ep.dst_type = 'person' AND ep.dst_id = $1
  JOIN topics t ON t.id = et.dst_id
 WHERE et.dst_type = 'topic'
 GROUP BY t.id, t.name
 ORDER BY COUNT(*) DESC, t.name ASC
 LIMIT 25
`

const personRelatedPlacesSQL = `
SELECT pl.id, COALESCE(pl.name, pl.id)
  FROM edges et
  JOIN edges ep ON ep.src_id = et.src_id AND ep.src_type = et.src_type AND ep.dst_type = 'person' AND ep.dst_id = $1
  JOIN ` + placesUnionIDNameSubquery + ` pl ON pl.id = et.dst_id
 WHERE et.dst_type = 'place'
 GROUP BY pl.id, pl.name
 ORDER BY COUNT(*) DESC, pl.name ASC
 LIMIT 25
`

func (s *pgxPeopleSource) GetPerson(ctx context.Context, id string) (*PersonDetail, error) {
	if s.pool == nil {
		return nil, errors.New("graphapi: people pool is nil")
	}
	var d PersonDetail
	err := s.pool.QueryRow(ctx, personSelectSQL, id).Scan(&d.ID, &d.DisplayName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPersonNotFound
		}
		return nil, err
	}
	d.ArtifactTimeline = s.scanTimeline(ctx, id)
	d.RelatedTopics = collectCrossLinks(ctx, s.pool, personRelatedTopicsSQL, id, "topic",
		func(label string) string { return RenderReason(ReasonCoOccursWithTopic, label) })
	d.RelatedPlaces = collectCrossLinks(ctx, s.pool, personRelatedPlacesSQL, id, "place",
		func(label string) string { return RenderReason(ReasonNearPlace, label) })
	return &d, nil
}

func (s *pgxPeopleSource) scanTimeline(ctx context.Context, id string) []ArtifactTimelineEntry {
	rows, err := s.pool.Query(ctx, personTimelineSQL, id)
	if err != nil {
		slog.Debug("graphapi: person timeline query failed (returning empty)", "id", id, "err", err)
		return []ArtifactTimelineEntry{}
	}
	defer rows.Close()
	out := []ArtifactTimelineEntry{}
	for rows.Next() {
		var e ArtifactTimelineEntry
		if err := rows.Scan(&e.ArtifactID, &e.Title, &e.CapturedAt); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out
}
