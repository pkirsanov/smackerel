package graphapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EdgeRow is the source-layer projection of one row in the `edges`
// table for SCOPE-080-04 resolveEdges. It carries the minimal
// metadata needed by the reason resolver (target kind + label) plus
// the edge id so the cursor codec can encode pagination state. The
// label is resolved by the source via LEFT JOINs to the target
// table (topics, people, places, artifacts) so the resolver does not
// pay an N+1 round-trip per edge.
type EdgeRow struct {
	EdgeID      string
	TargetKind  string
	TargetID    string
	TargetLabel string
}

// EdgesSource is the data-access boundary used by EdgesHandlers.
// Production uses a pgx-pool-backed implementation
// (NewEdgesHandlers); unit tests substitute an in-memory stub.
type EdgesSource interface {
	ListEdges(ctx context.Context, sourceKind, sourceID string, limit, offset int) (rows []EdgeRow, hasNext bool, err error)
}

// EdgesHandlers serves the spec 080 SCOPE-080-04 graph edges
// endpoint (GET /api/graph/edges?source=kind:id).
type EdgesHandlers struct {
	Source EdgesSource
	Limits Limits
	Codec  *CursorCodec
}

// NewEdgesHandlers wires an EdgesHandlers against a live pgx pool.
func NewEdgesHandlers(pool *pgxpool.Pool, limits Limits, codec *CursorCodec) *EdgesHandlers {
	return &EdgesHandlers{Source: &pgxEdgesSource{pool: pool}, Limits: limits, Codec: codec}
}

// allowedSourceKinds is the closed-set of `source=` kinds accepted
// by GET /api/graph/edges. design.md §3 endpoint schema.
var allowedSourceKinds = []string{"artifact", "topic", "person", "place"}

type edgesListResponse struct {
	Items      []CrossLink `json:"items"`
	NextCursor string      `json:"nextCursor"`
}

// ListEdges handles GET /api/graph/edges?source=kind:id.
// SCN-080-08 happy path + SCN-080-14 unknown source kind.
func (h *EdgesHandlers) ListEdges(w http.ResponseWriter, r *http.Request) {
	rawSource := r.URL.Query().Get("source")
	if rawSource == "" {
		WriteAPIError(w, ErrMissingParam.WithField("source"))
		return
	}
	kind, id, err := parseSourceParam(rawSource)
	if err != nil {
		WriteError(w, http.StatusBadRequest, CodeInvalidKind, "source", err.Error())
		return
	}

	limit, offset, cursorErr := parseEdgesPagination(r, h.Limits, h.Codec)
	if cursorErr != nil {
		WriteAPIError(w, cursorErr)
		return
	}

	rows, hasNext, srcErr := h.Source.ListEdges(r.Context(), kind, id, limit, offset)
	if srcErr != nil {
		slog.Error("graphapi: list edges failed", "kind", kind, "id", id, "err", srcErr)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to list edges")
		return
	}

	items, resolveErr := resolveEdges(rows)
	if resolveErr != nil {
		// SCOPE-080-04 D04-4: fail loud on missing reason metadata
		// rather than emit a CrossLink with a blank reason.
		slog.Error("graphapi: resolve edges failed", "kind", kind, "id", id, "err", resolveErr)
		WriteError(w, http.StatusInternalServerError, "internal_reason_missing", "", "edge metadata missing required reason field")
		return
	}

	next := ""
	if hasNext && h.Codec != nil {
		encoded, encErr := h.Codec.Encode(CursorPayload{
			Resource: "edges",
			Offset:   int64(offset + len(items)),
		})
		if encErr == nil {
			next = encoded
		}
	}

	writeJSON(w, http.StatusOK, edgesListResponse{Items: items, NextCursor: next})
}

// parseSourceParam splits "kind:id" and validates kind against the
// closed-set allowlist (SCN-080-14). The returned error message
// lists the four allowed kinds verbatim so the client learns the
// taxonomy from the 400 response.
func parseSourceParam(raw string) (kind, id string, err error) {
	idx := strings.Index(raw, ":")
	if idx <= 0 || idx == len(raw)-1 {
		return "", "", fmt.Errorf("source must be \"kind:id\" (allowed kinds: %s)", strings.Join(allowedSourceKinds, ","))
	}
	kind = raw[:idx]
	id = raw[idx+1:]
	for _, k := range allowedSourceKinds {
		if k == kind {
			return kind, id, nil
		}
	}
	return "", "", fmt.Errorf("source kind %q not allowed (allowed: %s)", kind, strings.Join(allowedSourceKinds, ","))
}

// parseEdgesPagination mirrors parseListPagination but clamps via
// Limits.ClampEdgesLimit so the edges endpoint uses its own
// EdgesMax / EdgesDefault (design.md §6).
func parseEdgesPagination(r *http.Request, limits Limits, codec *CursorCodec) (limit, offset int, errOut *APIError) {
	q := r.URL.Query()
	reqLimit := 0
	if raw := q.Get("limit"); raw != "" {
		n, scanErr := strconv.Atoi(raw)
		if scanErr != nil {
			return 0, 0, ErrLimitExceeded.WithField("limit")
		}
		reqLimit = n
	}
	clamped, err := limits.ClampEdgesLimit(reqLimit)
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
		if payload.Resource != "" && payload.Resource != "edges" {
			return 0, 0, ErrMalformedCursor
		}
		if payload.Offset < 0 {
			return 0, 0, ErrMalformedCursor
		}
		return clamped, int(payload.Offset), nil
	}
	return clamped, 0, nil
}

// resolveEdges converts source-layer EdgeRow values into the
// CrossLink wire shape, deriving each row's `reason` via the shared
// reason taxonomy (reasons.go). Per design.md §4 + DoD D04-4 the
// resolver MUST fail loud when an edge cannot produce a non-empty
// reason — a missing label or unknown target kind both surface as a
// typed error so the handler returns 500 rather than emit a
// CrossLink with a blank `reason`.
func resolveEdges(rows []EdgeRow) ([]CrossLink, error) {
	out := make([]CrossLink, 0, len(rows))
	for i, e := range rows {
		if e.TargetKind == "" || e.TargetID == "" {
			return nil, fmt.Errorf("graphapi: edge row %d missing targetKind/targetId", i)
		}
		if e.TargetLabel == "" {
			return nil, fmt.Errorf("graphapi: edge row %d (edge=%s) %w", i, e.EdgeID, ErrReasonRenderEmpty)
		}
		kind, err := ReasonKindForTargetKind(e.TargetKind)
		if err != nil {
			return nil, fmt.Errorf("graphapi: edge row %d: %w", i, err)
		}
		reason, err := ResolveReason(kind, e.TargetLabel)
		if err != nil {
			return nil, fmt.Errorf("graphapi: edge row %d: %w", i, err)
		}
		out = append(out, CrossLink{
			TargetKind:  e.TargetKind,
			TargetID:    e.TargetID,
			TargetLabel: e.TargetLabel,
			Reason:      reason,
		})
	}
	return out, nil
}

// pgxEdgesSource is the production EdgesSource backed by Postgres.
// One query joins to all four target tables via LEFT JOINs so the
// label resolution happens in a single round-trip per page.
type pgxEdgesSource struct{ pool *pgxpool.Pool }

// edgesListSQL reads edges out of the canonical `edges` table
// (internal/db/migrations/001_initial_schema.sql) and resolves the
// destination label per target kind. Ordering by weight DESC enforces
// the design.md §4 "server policy; client must not re-sort" contract.
// `LIMIT $3 + 1 OFFSET $4` is the standard hasNext probe.
const edgesListSQL = `
SELECT e.id,
       e.dst_type,
       e.dst_id,
       CASE e.dst_type
         WHEN 'topic'    THEN COALESCE(t.name,  e.dst_id)
         WHEN 'person'   THEN COALESCE(p.name,  e.dst_id)
         WHEN 'place'    THEN COALESCE(pl.name, e.dst_id)
         WHEN 'artifact' THEN COALESCE(a.title, e.dst_id)
         ELSE e.dst_id
       END AS target_label
  FROM edges e
  LEFT JOIN topics    t  ON e.dst_type = 'topic'    AND t.id  = e.dst_id
  LEFT JOIN people    p  ON e.dst_type = 'person'   AND p.id  = e.dst_id
  LEFT JOIN ` + placesUnionIDNameSubquery + ` pl ON e.dst_type = 'place'    AND pl.id = e.dst_id
  LEFT JOIN artifacts a  ON e.dst_type = 'artifact' AND a.id  = e.dst_id
 WHERE e.src_type = $1
   AND e.src_id   = $2
   AND e.dst_type IN ('topic','person','place','artifact')
 ORDER BY e.weight DESC NULLS LAST, e.id ASC
 LIMIT $3 OFFSET $4
`

func (s *pgxEdgesSource) ListEdges(ctx context.Context, sourceKind, sourceID string, limit, offset int) ([]EdgeRow, bool, error) {
	if s.pool == nil {
		return nil, false, errors.New("graphapi: edges pool is nil")
	}
	rows, err := s.pool.Query(ctx, edgesListSQL, sourceKind, sourceID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]EdgeRow, 0, limit)
	for rows.Next() {
		var e EdgeRow
		if err := rows.Scan(&e.EdgeID, &e.TargetKind, &e.TargetID, &e.TargetLabel); err != nil {
			return nil, false, err
		}
		out = append(out, e)
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
