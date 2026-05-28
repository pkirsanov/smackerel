// Package extensiondevices implements the read-only admin view
// GET /v1/admin/extension/devices for spec 058 Scope 5. It aggregates
// rows from raw_ingest_dedup grouped by source_device_id and returns
// first/last seen timestamps + 30-day visit count.
//
// Admin scoping: the handler MUST be mounted behind the existing
// bearer-auth + callerIsAdmin gate used by AuthAdminHandlers. Non-
// admin callers see ONLY rows whose owner_user_id equals their own
// session UserID (per spec 058 design §3.2).
package extensiondevices

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Device is one row of the admin devices view response.
type Device struct {
	SourceDeviceID string    `json:"source_device_id"`
	OwnerUserID    string    `json:"owner_user_id"`
	FirstSeenAt    time.Time `json:"first_seen_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	VisitCount30d  int64     `json:"visit_count_30d"`
}

// Response is the JSON body shape per spec 058 design §3.2.
type Response struct {
	Devices []Device `json:"devices"`
}

// Store abstracts the raw_ingest_dedup aggregation query so the
// handler is testable without a live Postgres pool.
type Store interface {
	// AggregateDevices returns one Device per (owner_user_id,
	// source_device_id) pair. If ownerUserIDFilter is non-empty,
	// results are scoped to that owner; otherwise (admin caller)
	// every owner is included.
	AggregateDevices(ctx context.Context, ownerUserIDFilter string) ([]Device, error)
}

// AdminPredicate is the cross-handler admin gate. The router supplies
// the same predicate that AuthAdminHandlers.callerIsAdmin uses.
type AdminPredicate func(r *http.Request) (ownerUserID string, isAdmin bool, ok bool)

// Handler serves GET /v1/admin/extension/devices.
type Handler struct {
	store   Store
	isAdmin AdminPredicate
}

// NewHandler constructs the admin devices handler. Both arguments are
// required; a nil store or admin predicate is a programming error.
func NewHandler(store Store, isAdmin AdminPredicate) *Handler {
	if store == nil {
		panic("extensiondevices: NewHandler requires non-nil Store")
	}
	if isAdmin == nil {
		panic("extensiondevices: NewHandler requires non-nil AdminPredicate")
	}
	return &Handler{store: store, isAdmin: isAdmin}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed",
			"GET required")
		return
	}

	ownerUserID, admin, ok := h.isAdmin(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthenticated",
			"bearer auth required")
		return
	}

	filter := ""
	if !admin {
		// Non-admin callers see ONLY their own devices.
		if ownerUserID == "" {
			writeJSONError(w, http.StatusForbidden, "owner_user_id_required",
				"non-admin session missing owner user id")
			return
		}
		filter = ownerUserID
	}

	devices, err := h.store.AggregateDevices(r.Context(), filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "store_error",
			err.Error())
		return
	}
	// Deterministic order: (owner_user_id, source_device_id).
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].OwnerUserID != devices[j].OwnerUserID {
			return devices[i].OwnerUserID < devices[j].OwnerUserID
		}
		return devices[i].SourceDeviceID < devices[j].SourceDeviceID
	})
	if devices == nil {
		devices = []Device{}
	}
	writeJSON(w, http.StatusOK, Response{Devices: devices})
}

// PostgresStore reads the aggregation from raw_ingest_dedup.
type PostgresStore struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewPostgresStore returns a Store backed by the supplied pgxpool.Pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	if pool == nil {
		panic("extensiondevices: NewPostgresStore requires a non-nil pool")
	}
	return &PostgresStore{pool: pool, now: time.Now}
}

// WithNow returns a copy of the store with the supplied clock — tests
// only. Production callers use NewPostgresStore directly.
func (s *PostgresStore) WithNow(now func() time.Time) *PostgresStore {
	cp := *s
	cp.now = now
	return &cp
}

// AggregateDevices implements Store.
//
// Aggregation contract (spec 058 design §3.2):
//   - source_id is pinned to 'browser-extension' so other future
//     ingestion paths cannot pollute the view.
//   - visit_count_30d sums visit_count for rows whose last_seen_at is
//     within the trailing 30 days; rows older than 30 days still
//     contribute first_seen_at/last_seen_at but a zero recent count.
//   - When ownerUserIDFilter is non-empty, the WHERE clause restricts
//     to that owner; otherwise every owner is returned.
func (s *PostgresStore) AggregateDevices(ctx context.Context, ownerUserIDFilter string) ([]Device, error) {
	now := s.now()
	cutoff := now.AddDate(0, 0, -30)

	const baseQuery = `
		SELECT
		    owner_user_id,
		    source_device_id,
		    MIN(first_seen_at)                                                AS first_seen_at,
		    MAX(last_seen_at)                                                 AS last_seen_at,
		    COALESCE(SUM(visit_count) FILTER (WHERE last_seen_at >= $1), 0)   AS visit_count_30d
		FROM raw_ingest_dedup
		WHERE source_id = 'browser-extension'
	`
	query := baseQuery
	args := []any{cutoff}
	if ownerUserIDFilter != "" {
		query += " AND owner_user_id = $2"
		args = append(args, ownerUserIDFilter)
	}
	query += " GROUP BY owner_user_id, source_device_id"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	devices := make([]Device, 0)
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.OwnerUserID, &d.SourceDeviceID,
			&d.FirstSeenAt, &d.LastSeenAt, &d.VisitCount30d); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return devices, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
