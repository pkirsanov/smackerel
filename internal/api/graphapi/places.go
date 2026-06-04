package graphapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PlaceRow is the wire shape for one item in GET /api/places
// (SCN-080-05). Source field flags the place provenance so the client
// can render badges; both sources collapse to a single id when they
// reference the same canonical place.
type PlaceRow struct {
	ID            string `json:"id"`
	DisplayName   string `json:"displayName"`
	ArtifactCount int    `json:"artifactCount"`
	Source        string `json:"source"`
}

// PlaceLocation is the optional lat/lon pair returned by GET /api/places/{id}.
type PlaceLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// PlaceDetail is the wire shape for GET /api/places/{id} (SCN-080-06).
type PlaceDetail struct {
	ID              string         `json:"id"`
	DisplayName     string         `json:"displayName"`
	Location        *PlaceLocation `json:"location"`
	LinkedArtifacts []CrossLink    `json:"linkedArtifacts"`
}

type placesListResponse struct {
	Items      []PlaceRow `json:"items"`
	NextCursor string     `json:"nextCursor"`
}

// PlacesSource is the data-access boundary used by PlacesHandlers.
type PlacesSource interface {
	ListPlaces(ctx context.Context, limit, offset int) (rows []PlaceRow, hasNext bool, err error)
	GetPlace(ctx context.Context, id string) (*PlaceDetail, error)
}

// PlacesHandlers serves the spec 080 places endpoints.
type PlacesHandlers struct {
	Source PlacesSource
	Limits Limits
	Codec  *CursorCodec
}

// NewPlacesHandlers wires a PlacesHandlers against a live pgx pool.
//
// No first-class `places` table exists yet (see
// internal/db/migrations/001_initial_schema.sql); this scope
// (SCOPE-080-03) ships a minimal best-effort aggregator that unions
// two real sources and dedupes by canonical id:
//
//  1. Maps connector — `location_clusters.end_cluster_lat,lng` rows
//     bucketed to a stable id `mp:<lat6>:<lng6>`.
//  2. Artifact-derived — `artifacts.location_geo->>'name'` values
//     bucketed to a stable id `ar:<sha-of-name>`.
//
// The artifact_count column is a literal count of distinct artifacts
// observed per bucket. When the same canonical id appears in both
// streams the row's `source` becomes `merged`.
func NewPlacesHandlers(pool *pgxpool.Pool, limits Limits, codec *CursorCodec) *PlacesHandlers {
	return &PlacesHandlers{Source: &pgxPlacesSource{pool: pool}, Limits: limits, Codec: codec}
}

// ErrPlaceNotFound signals GetPlace could not resolve {id}.
var ErrPlaceNotFound = errors.New("graphapi: place not found")

// ListPlaces handles GET /api/places.
func (h *PlacesHandlers) ListPlaces(w http.ResponseWriter, r *http.Request) {
	limit, offset, cursorErr := parseListPagination(r, h.Limits, h.Codec, "places")
	if cursorErr != nil {
		WriteAPIError(w, cursorErr)
		return
	}
	rows, hasNext, err := h.Source.ListPlaces(r.Context(), limit, offset)
	if err != nil {
		slog.Error("graphapi: list places failed", "err", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to list places")
		return
	}
	if rows == nil {
		rows = []PlaceRow{}
	}
	next := ""
	if hasNext && h.Codec != nil {
		encoded, encErr := h.Codec.Encode(CursorPayload{
			Resource: "places",
			Offset:   int64(offset + len(rows)),
		})
		if encErr == nil {
			next = encoded
		}
	}
	writeJSON(w, http.StatusOK, placesListResponse{Items: rows, NextCursor: next})
}

// GetPlace handles GET /api/places/{id}.
func (h *PlacesHandlers) GetPlace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		WriteAPIError(w, ErrMissingParam.WithField("id"))
		return
	}
	detail, err := h.Source.GetPlace(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrPlaceNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "id", "place not found")
			return
		}
		slog.Error("graphapi: get place failed", "id", id, "err", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "", "failed to load place")
		return
	}
	if detail.LinkedArtifacts == nil {
		detail.LinkedArtifacts = []CrossLink{}
	}
	writeJSON(w, http.StatusOK, detail)
}

// renderSamePlaceReason is the design.md §2 reason taxonomy entry
// for place co-occurrence: `same place <place label>`. SCOPE-080-04
// aligned the package-wide reasons.go templates to the literal
// design strings; this wrapper now routes through the shared
// RenderReason resolver so every cross-link rendered anywhere in
// graphapi flows through the same taxonomy.
func renderSamePlaceReason(label string) string {
	return RenderReason(ReasonNearPlace, label)
}

// pgxPlacesSource is the production PlacesSource backed by Postgres.
type pgxPlacesSource struct{ pool *pgxpool.Pool }

// placesUnionIDNameSubquery is the canonical (id, name) projection of
// the two real place sources (location_clusters + artifacts.location_geo).
// Shared with edges.go / topics.go / people.go so the same canonical id
// scheme resolves labels for edge dst_type='place'. md5() is a built-in
// (no pgcrypto extension); place ids are opaque dedupe keys, not security
// material, so md5 is sufficient and stable across processes.
const placesUnionIDNameSubquery = `(
  SELECT id, max(name) AS name FROM (
    SELECT
      'mp:' || to_char(round(end_cluster_lat::numeric, 4), 'FM0.0000') ||
          ':' || to_char(round(end_cluster_lng::numeric, 4), 'FM0.0000') AS id,
      'cluster ' || to_char(round(end_cluster_lat::numeric, 4), 'FM0.0000') ||
          ', ' || to_char(round(end_cluster_lng::numeric, 4), 'FM0.0000') AS name
    FROM location_clusters
    UNION ALL
    SELECT
      'ar:' || md5(lower(trim(location_geo->>'name'))) AS id,
      location_geo->>'name' AS name
    FROM artifacts
    WHERE location_geo IS NOT NULL
      AND location_geo ? 'name'
      AND length(coalesce(location_geo->>'name','')) > 0
  ) u
  GROUP BY id
)`

// placesUnionSQL aggregates the two real sources of place data
// (location_clusters.end_cluster_* + artifacts.location_geo) into a
// single deduped list. See NewPlacesHandlers for the id scheme.
const placesUnionSQL = `
WITH maps_places AS (
  SELECT
    'mp:' || to_char(round(end_cluster_lat::numeric, 4), 'FM0.0000') ||
        ':' || to_char(round(end_cluster_lng::numeric, 4), 'FM0.0000') AS id,
    'cluster ' || to_char(round(end_cluster_lat::numeric, 4), 'FM0.0000') ||
        ', ' || to_char(round(end_cluster_lng::numeric, 4), 'FM0.0000') AS display_name,
    COUNT(*)::int AS artifact_count,
    'maps'::text AS source
  FROM location_clusters
  GROUP BY 1, 2
),
artifact_places AS (
  SELECT
    'ar:' || md5(lower(trim(location_geo->>'name'))) AS id,
    location_geo->>'name' AS display_name,
    COUNT(*)::int AS artifact_count,
    'artifact'::text AS source
  FROM artifacts
  WHERE location_geo IS NOT NULL
    AND location_geo ? 'name'
    AND length(coalesce(location_geo->>'name','')) > 0
  GROUP BY 1, 2
),
unioned AS (
  SELECT * FROM maps_places
  UNION ALL
  SELECT * FROM artifact_places
),
deduped AS (
  SELECT id,
         max(display_name) AS display_name,
         sum(artifact_count)::int AS artifact_count,
         CASE WHEN count(DISTINCT source) > 1 THEN 'merged'
              ELSE max(source) END AS source
  FROM unioned
  GROUP BY id
)
SELECT id, display_name, artifact_count, source
  FROM deduped
 ORDER BY artifact_count DESC, display_name ASC, id ASC
 LIMIT $1 OFFSET $2
`

func (s *pgxPlacesSource) ListPlaces(ctx context.Context, limit, offset int) ([]PlaceRow, bool, error) {
	if s.pool == nil {
		return nil, false, errors.New("graphapi: places pool is nil")
	}
	rows, err := s.pool.Query(ctx, placesUnionSQL, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]PlaceRow, 0, limit)
	for rows.Next() {
		var p PlaceRow
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.ArtifactCount, &p.Source); err != nil {
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

const placeDetailMapsSQL = `
SELECT
  end_cluster_lat,
  end_cluster_lng,
  'cluster ' || to_char(round(end_cluster_lat::numeric, 4), 'FM0.0000') ||
      ', ' || to_char(round(end_cluster_lng::numeric, 4), 'FM0.0000') AS display_name
  FROM location_clusters
 WHERE 'mp:' || to_char(round(end_cluster_lat::numeric, 4), 'FM0.0000') ||
        ':' || to_char(round(end_cluster_lng::numeric, 4), 'FM0.0000') = $1
 LIMIT 1
`

const placeDetailArtifactSQL = `
SELECT location_geo->>'name'
  FROM artifacts
 WHERE location_geo IS NOT NULL
   AND location_geo ? 'name'
   AND 'ar:' || md5(lower(trim(location_geo->>'name'))) = $1
 LIMIT 1
`

const placeLinkedArtifactsSQL = `
SELECT a.id, COALESCE(a.title, a.id)
  FROM artifacts a
 WHERE a.location_geo IS NOT NULL
   AND a.location_geo ? 'name'
   AND 'ar:' || md5(lower(trim(a.location_geo->>'name'))) = $1
 ORDER BY a.created_at DESC NULLS LAST, a.id ASC
 LIMIT 50
`

func (s *pgxPlacesSource) GetPlace(ctx context.Context, id string) (*PlaceDetail, error) {
	if s.pool == nil {
		return nil, errors.New("graphapi: places pool is nil")
	}
	var d PlaceDetail
	d.ID = id

	// Resolve label + location depending on source prefix.
	switch {
	case len(id) > 3 && id[:3] == "mp:":
		var lat, lng float64
		var name string
		err := s.pool.QueryRow(ctx, placeDetailMapsSQL, id).Scan(&lat, &lng, &name)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPlaceNotFound
			}
			return nil, err
		}
		d.DisplayName = name
		d.Location = &PlaceLocation{Lat: lat, Lon: lng}
	case len(id) > 3 && id[:3] == "ar:":
		var name string
		err := s.pool.QueryRow(ctx, placeDetailArtifactSQL, id).Scan(&name)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPlaceNotFound
			}
			return nil, err
		}
		d.DisplayName = name
		d.LinkedArtifacts = collectCrossLinks(ctx, s.pool, placeLinkedArtifactsSQL, id, "artifact",
			func(label string) string { return renderSamePlaceReason(d.DisplayName) })
		return &d, nil
	default:
		return nil, ErrPlaceNotFound
	}

	// Maps-sourced place: linked artifacts come from artifacts with
	// matching location_geo names; for the minimal aggregator we
	// return an empty list (Scope 04 owns true cross-source edges).
	d.LinkedArtifacts = []CrossLink{}
	return &d, nil
}
