package graphapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TopicRow is the wire shape for a single row in the topics list
// response (SCN-080-01). Server-derived field tags only; never reuse
// an internal-layer struct over the wire.
type TopicRow struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	LinkedArtifactCount int    `json:"linkedArtifactCount"`
	PeopleCount         int    `json:"peopleCount"`
	PlaceCount          int    `json:"placeCount"`
}

// TopicDetail is the wire shape for GET /api/topics/{id} (SCN-080-02).
// Cross-link arrays are empty (non-nil) when nothing is linked so the
// JSON contract is stable (`[]` not `null`).
type TopicDetail struct {
	ID              string      `json:"id"`
	Label           string      `json:"label"`
	LinkedArtifacts []CrossLink `json:"linkedArtifacts"`
	RelatedPeople   []CrossLink `json:"relatedPeople"`
	RelatedPlaces   []CrossLink `json:"relatedPlaces"`
}

// topicsListResponse is the wire envelope for GET /api/topics.
type topicsListResponse struct {
	Items      []TopicRow `json:"items"`
	NextCursor string     `json:"nextCursor"`
}

// TopicsSource is the data-access boundary used by TopicsHandlers.
// Production uses a pgx-pool-backed implementation (see
// NewTopicsHandlers); unit tests substitute an in-memory stub.
type TopicsSource interface {
	ListTopics(ctx context.Context, limit, offset int) (rows []TopicRow, hasNext bool, err error)
	GetTopic(ctx context.Context, id string) (*TopicDetail, error)
}

// TopicsHandlers serves the spec 080 topics endpoints.
type TopicsHandlers struct {
	Source TopicsSource
	Limits Limits
	Codec  *CursorCodec
}

// NewTopicsHandlers wires a TopicsHandlers against a live pgx pool.
func NewTopicsHandlers(pool *pgxpool.Pool, limits Limits, codec *CursorCodec) *TopicsHandlers {
	return &TopicsHandlers{Source: &pgxTopicsSource{pool: pool}, Limits: limits, Codec: codec}
}

// ErrTopicNotFound signals that GetTopic could not resolve {id}. The
// handler translates this into a 404 with the graphapi envelope.
var ErrTopicNotFound = errors.New("graphapi: topic not found")

// ListTopics handles GET /api/topics.
func (h *TopicsHandlers) ListTopics(w http.ResponseWriter, r *http.Request) {
	limit, offset, cursorErr := parseListPagination(r, h.Limits, h.Codec, "topics")
	if cursorErr != nil {
		WriteAPIError(w, cursorErr)
		return
	}

	rows, hasNext, err := h.Source.ListTopics(r.Context(), limit, offset)
	if err != nil {
		slog.Error("graphapi: list topics failed", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to list topics")
		return
	}
	if rows == nil {
		rows = []TopicRow{}
	}

	next := ""
	if hasNext && h.Codec != nil {
		encoded, encErr := h.Codec.Encode(CursorPayload{
			Resource: "topics",
			Offset:   int64(offset + len(rows)),
		})
		if encErr == nil {
			next = encoded
		}
	}

	writeJSON(w, http.StatusOK, topicsListResponse{Items: rows, NextCursor: next})
}

// GetTopic handles GET /api/topics/{id}.
func (h *TopicsHandlers) GetTopic(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		WriteAPIError(w, ErrMissingParam.WithField("id"))
		return
	}
	detail, err := h.Source.GetTopic(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrTopicNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "id", "topic not found")
			return
		}
		slog.Error("graphapi: get topic failed", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to load topic")
		return
	}
	if detail.LinkedArtifacts == nil {
		detail.LinkedArtifacts = []CrossLink{}
	}
	if detail.RelatedPeople == nil {
		detail.RelatedPeople = []CrossLink{}
	}
	if detail.RelatedPlaces == nil {
		detail.RelatedPlaces = []CrossLink{}
	}
	writeJSON(w, http.StatusOK, detail)
}

// parseListPagination centralizes ?limit + ?cursor parsing for the
// list endpoints. Returns the clamped limit, the decoded offset, and
// a typed APIError on validation failure (limit_exceeded /
// invalid_cursor) ready for WriteAPIError.
func parseListPagination(r *http.Request, limits Limits, codec *CursorCodec, resource string) (limit, offset int, errOut *APIError) {
	q := r.URL.Query()
	reqLimit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return 0, 0, ErrLimitExceeded.WithField("limit")
		}
		reqLimit = n
	}
	clamped, err := limits.ClampLimit(reqLimit)
	if err != nil {
		return 0, 0, ErrLimitExceeded
	}

	if raw := q.Get("cursor"); raw != "" {
		if codec == nil {
			return 0, 0, ErrMalformedCursor
		}
		payload, decErr := codec.Decode(raw)
		if decErr != nil {
			return 0, 0, ErrMalformedCursor
		}
		if payload.Resource != "" && payload.Resource != resource {
			return 0, 0, ErrMalformedCursor
		}
		if payload.Offset < 0 {
			return 0, 0, ErrMalformedCursor
		}
		return clamped, int(payload.Offset), nil
	}
	return clamped, 0, nil
}

// writeJSON is a small helper for the success path. The error path
// uses WriteError / WriteAPIError to guarantee the graphapi envelope.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// pgxTopicsSource is the production TopicsSource backed by Postgres.
// Counts are computed via subqueries against the `edges` table; the
// list query also asks for one extra row beyond `limit` to detect
// hasNext without a separate COUNT(*).
type pgxTopicsSource struct{ pool *pgxpool.Pool }

const topicsListSQL = `
SELECT t.id,
       t.name,
       COALESCE(t.capture_count_total, 0) AS linked_artifact_count,
       (SELECT COUNT(DISTINCT e.dst_id) FROM edges e
          WHERE e.src_type = 'topic' AND e.src_id = t.id AND e.dst_type = 'person') AS people_count,
       (SELECT COUNT(DISTINCT e.dst_id) FROM edges e
          WHERE e.src_type = 'topic' AND e.src_id = t.id AND e.dst_type = 'place') AS place_count
  FROM topics t
 ORDER BY t.momentum_score DESC NULLS LAST, t.capture_count_total DESC NULLS LAST, t.id ASC
 LIMIT $1 OFFSET $2
`

func (s *pgxTopicsSource) ListTopics(ctx context.Context, limit, offset int) ([]TopicRow, bool, error) {
	if s.pool == nil {
		return nil, false, errors.New("graphapi: topics pool is nil")
	}
	rows, err := s.pool.Query(ctx, topicsListSQL, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]TopicRow, 0, limit)
	for rows.Next() {
		var t TopicRow
		if err := rows.Scan(&t.ID, &t.Label, &t.LinkedArtifactCount, &t.PeopleCount, &t.PlaceCount); err != nil {
			return nil, false, err
		}
		out = append(out, t)
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

const topicSelectSQL = `
SELECT id, name
  FROM topics
 WHERE id = $1
`

const topicLinkedArtifactsSQL = `
SELECT a.id, COALESCE(a.title, a.id)
  FROM edges e
  JOIN artifacts a ON a.id = e.dst_id
 WHERE e.src_type = 'topic' AND e.src_id = $1 AND e.dst_type = 'artifact'
 ORDER BY a.created_at DESC NULLS LAST, a.id ASC
 LIMIT 50
`

const topicRelatedPeopleSQL = `
SELECT p.id, COALESCE(p.name, p.id)
  FROM edges e
  JOIN people p ON p.id = e.dst_id
 WHERE e.src_type = 'topic' AND e.src_id = $1 AND e.dst_type = 'person'
 ORDER BY p.name ASC
 LIMIT 50
`

const topicRelatedPlacesSQL = `
SELECT pl.id, COALESCE(pl.name, pl.id)
  FROM edges e
  JOIN ` + placesUnionIDNameSubquery + ` pl ON pl.id = e.dst_id
 WHERE e.src_type = 'topic' AND e.src_id = $1 AND e.dst_type = 'place'
 ORDER BY pl.name ASC
 LIMIT 50
`

func (s *pgxTopicsSource) GetTopic(ctx context.Context, id string) (*TopicDetail, error) {
	if s.pool == nil {
		return nil, errors.New("graphapi: topics pool is nil")
	}
	var d TopicDetail
	err := s.pool.QueryRow(ctx, topicSelectSQL, id).Scan(&d.ID, &d.Label)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTopicNotFound
		}
		return nil, err
	}
	d.LinkedArtifacts = collectCrossLinks(ctx, s.pool, topicLinkedArtifactsSQL, id, "artifact",
		func(label string) string { return RenderReason(ReasonMentionedInArtifact, label) })
	d.RelatedPeople = collectCrossLinks(ctx, s.pool, topicRelatedPeopleSQL, id, "person",
		func(label string) string { return RenderReason(ReasonCoOccursWithTopic, d.Label) })
	d.RelatedPlaces = collectCrossLinks(ctx, s.pool, topicRelatedPlacesSQL, id, "place",
		func(label string) string { return RenderReason(ReasonNearPlace, label) })
	return &d, nil
}

// collectCrossLinks runs `sql` with the single `id` argument and
// converts every row to a CrossLink via reasonFn. Soft-fails on query
// errors (logs + returns empty slice) so a missing related table
// (e.g., `places` not present in older fixtures) does not 500 the
// whole detail response — the missing related-edges contract lands in
// SCOPE-080-04 anyway.
func collectCrossLinks(ctx context.Context, pool *pgxpool.Pool, sql, id, kind string, reasonFn func(label string) string) []CrossLink {
	rows, err := pool.Query(ctx, sql, id)
	if err != nil {
		slog.Debug("graphapi: cross-link query failed (returning empty)", "kind", kind, "id", id, "err", err)
		return []CrossLink{}
	}
	defer rows.Close()
	out := []CrossLink{}
	for rows.Next() {
		var tID, tLabel string
		if err := rows.Scan(&tID, &tLabel); err != nil {
			continue
		}
		out = append(out, CrossLink{
			TargetKind:  kind,
			TargetID:    tID,
			TargetLabel: tLabel,
			Reason:      reasonFn(tLabel),
		})
	}
	return out
}
