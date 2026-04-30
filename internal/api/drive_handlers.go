package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
	driveextract "github.com/smackerel/smackerel/internal/drive/extract"
)

// DriveProviderCapabilities is the JSON shape of the provider-neutral
// capabilities surface emitted by GET /v1/connectors/drive. It mirrors
// drive.Capabilities exactly so downstream consumers do not need to import
// internal/drive to reason about the wire shape.
type DriveProviderCapabilities struct {
	SupportsVersions      bool     `json:"supports_versions"`
	SupportsSharing       bool     `json:"supports_sharing"`
	SupportsChangeHistory bool     `json:"supports_change_history"`
	MaxFileSizeBytes      int64    `json:"max_file_size_bytes"`
	SupportedMimeFilter   []string `json:"supported_mime_filter"`
}

// DriveProviderView is one entry in the connectors-list response. Spec 038
// SCN-038-003 requires this shape to be provider-neutral — i.e. derived
// solely from the drive.Provider interface (ID, DisplayName, Capabilities)
// with no provider-specific branching.
type DriveProviderView struct {
	ID           string                    `json:"id"`
	DisplayName  string                    `json:"display_name"`
	Capabilities DriveProviderCapabilities `json:"capabilities"`
}

// DriveConnectorsResponse is the JSON response shape of
// GET /v1/connectors/drive.
type DriveConnectorsResponse struct {
	Providers []DriveProviderView `json:"providers"`
}

// DriveProviderRegistry is the minimal surface DriveHandlers needs from
// the drive package. Tests can substitute a fixture registry without
// mutating drive.DefaultRegistry. Production wiring passes
// drive.DefaultRegistry directly.
type DriveProviderRegistry interface {
	List() []drive.Provider
	Get(id string) (drive.Provider, bool)
}

// DriveHandlers exposes the drive connector HTTP surface (Spec 038
// Scope 1, DoD items 5/6). The handlers cover the connector list, the
// OAuth begin endpoint, the OAuth redirect-callback, and the per-
// connection state read. The pool is optional — when nil, only
// ListConnectors is functional and the other handlers return 503 so a
// partially-wired runtime fails loudly rather than silently succeeding
// against a nil DB.
type DriveHandlers struct {
	registry DriveProviderRegistry
	pool     *pgxpool.Pool
}

// NewDriveHandlers returns a DriveHandlers backed by the supplied
// provider registry. The registry is required — passing nil panics
// at construction so a misconfigured runtime fails loudly at startup
// rather than at first request.
func NewDriveHandlers(registry DriveProviderRegistry) *DriveHandlers {
	if registry == nil {
		panic("api: NewDriveHandlers called with nil DriveProviderRegistry")
	}
	return &DriveHandlers{registry: registry}
}

// NewDriveHandlersWithPool returns a fully-wired DriveHandlers including
// the Postgres pool needed by the connection-state endpoint. Production
// wiring MUST pass a non-nil pool.
func NewDriveHandlersWithPool(registry DriveProviderRegistry, pool *pgxpool.Pool) *DriveHandlers {
	if registry == nil {
		panic("api: NewDriveHandlersWithPool called with nil DriveProviderRegistry")
	}
	return &DriveHandlers{registry: registry, pool: pool}
}

// ListConnectors handles GET /v1/connectors/drive. It returns every
// registered drive provider through the neutral DriveProvider contract.
func (h *DriveHandlers) ListConnectors(w http.ResponseWriter, _ *http.Request) {
	providers := h.registry.List()
	views := make([]DriveProviderView, 0, len(providers))
	for _, p := range providers {
		caps := p.Capabilities()
		views = append(views, DriveProviderView{
			ID:          p.ID(),
			DisplayName: p.DisplayName(),
			Capabilities: DriveProviderCapabilities{
				SupportsVersions:      caps.SupportsVersions,
				SupportsSharing:       caps.SupportsSharing,
				SupportsChangeHistory: caps.SupportsChangeHistory,
				MaxFileSizeBytes:      caps.MaxFileSizeBytes,
				SupportedMimeFilter:   caps.SupportedMimeFilter,
			},
		})
	}
	writeJSON(w, http.StatusOK, DriveConnectorsResponse{Providers: views})
}

// DriveConnectRequest is the JSON request body for
// POST /v1/connectors/drive/connect. owner_user_id is required because
// the drive flow does not yet integrate with the platform auth surface
// (Scope 1 keeps the surface narrow); the PWA stores a stable per-browser
// owner UUID in localStorage and submits it here. A future scope will
// lift this into the authenticated session.
type DriveConnectRequest struct {
	ProviderID  string            `json:"provider_id"`
	OwnerUserID string            `json:"owner_user_id"`
	AccessMode  string            `json:"access_mode"`
	Scope       DriveConnectScope `json:"scope"`
}

// DriveConnectScope mirrors drive.Scope on the wire.
type DriveConnectScope struct {
	FolderIDs     []string `json:"folder_ids"`
	IncludeShared bool     `json:"include_shared"`
}

// DriveConnectResponse is the JSON response body for
// POST /v1/connectors/drive/connect.
type DriveConnectResponse struct {
	AuthURL string `json:"authURL"`
	State   string `json:"state"`
}

// Connect handles POST /v1/connectors/drive/connect. It resolves the
// provider from the registry, plumbs the owner into the request context,
// and proxies to provider.BeginConnect.
func (h *DriveHandlers) Connect(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing request body")
		return
	}
	defer r.Body.Close()

	var req DriveConnectRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body: "+err.Error())
		return
	}
	if req.ProviderID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "provider_id is required")
		return
	}
	if req.OwnerUserID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "owner_user_id is required")
		return
	}
	mode := drive.AccessMode(req.AccessMode)
	if err := mode.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	provider, ok := h.registry.Get(req.ProviderID)
	if !ok {
		writeError(w, http.StatusNotFound, "PROVIDER_NOT_FOUND", "drive provider "+req.ProviderID+" is not registered")
		return
	}

	ctx := drive.WithOwnerUserID(r.Context(), req.OwnerUserID)
	scope := drive.Scope{
		FolderIDs:     req.Scope.FolderIDs,
		IncludeShared: req.Scope.IncludeShared,
	}
	authURL, state, err := provider.BeginConnect(ctx, mode, scope)
	if err != nil {
		writeError(w, http.StatusBadGateway, "BEGIN_CONNECT_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, DriveConnectResponse{AuthURL: authURL, State: state})
}

// OAuthCallback handles GET /v1/connectors/drive/oauth/callback. The
// upstream OAuth provider redirects the user agent here with state +
// code query parameters. On success the handler issues a 302 redirect
// to the PWA connector-detail page; on failure it redirects back to
// the connectors page with an error query parameter so the PWA can
// surface the failure.
func (h *DriveHandlers) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		redirectWithDriveError(w, r, "missing state or code")
		return
	}

	provider, ok := h.registry.Get("google")
	if !ok {
		redirectWithDriveError(w, r, "google provider not registered")
		return
	}
	connID, err := provider.FinalizeConnect(r.Context(), state, code)
	if err != nil {
		redirectWithDriveError(w, r, err.Error())
		return
	}
	q := url.Values{}
	q.Set("id", connID)
	http.Redirect(w, r, "/pwa/connector-detail.html?"+q.Encode(), http.StatusFound)
}

func redirectWithDriveError(w http.ResponseWriter, r *http.Request, msg string) {
	q := url.Values{}
	q.Set("error", msg)
	http.Redirect(w, r, "/pwa/connectors.html?"+q.Encode(), http.StatusFound)
}

// DriveConnectionView is the JSON shape returned by GET
// /v1/connectors/drive/connection/{id}.
type DriveConnectionView struct {
	ID                 string              `json:"id"`
	ProviderID         string              `json:"provider_id"`
	AccountLabel       string              `json:"account_label"`
	AccessMode         string              `json:"access_mode"`
	Status             string              `json:"status"`
	HealthReason       string              `json:"health_reason"`
	Scope              DriveConnectScope   `json:"scope"`
	IndexedCount       int64               `json:"indexed_count"`
	SkippedCount       int64               `json:"skipped_count"`
	RetryableWorkCount int64               `json:"retryable_work_count"`
	EmptyDrive         bool                `json:"empty_drive"`
	Progress           *DriveProgressView  `json:"progress,omitempty"`
	RecentActivity     []DriveActivityView `json:"recent_activity"`
}

// DriveProgressView is the latest scan/monitor progress row rendered by
// Screen 3.
type DriveProgressView struct {
	Phase           string `json:"phase"`
	Status          string `json:"status"`
	TotalSeen       int64  `json:"total_seen"`
	IndexedCount    int64  `json:"indexed_count"`
	SkippedCount    int64  `json:"skipped_count"`
	UpsertedCount   int64  `json:"upserted_count"`
	MovedCount      int64  `json:"moved_count"`
	TombstonedCount int64  `json:"tombstoned_count"`
	LastError       string `json:"last_error"`
}

// DriveActivityView is one recent scan/monitor activity row.
type DriveActivityView struct {
	Phase        string `json:"phase"`
	Status       string `json:"status"`
	IndexedCount int64  `json:"indexed_count"`
	UpdatedAt    string `json:"updated_at"`
}

// DriveSkippedBlockedResponse is the Screen 4 review surface for files that
// could not be extracted automatically.
type DriveSkippedBlockedResponse struct {
	ConnectionID string                     `json:"connection_id"`
	Total        int                        `json:"total"`
	Groups       []DriveSkippedBlockedGroup `json:"groups"`
}

// DriveSkippedBlockedGroup groups review items by state and concrete reason.
type DriveSkippedBlockedGroup struct {
	ExtractionState   string                            `json:"extraction_state"`
	SkipReason        string                            `json:"skip_reason"`
	RecommendedAction string                            `json:"recommended_action"`
	Count             int                               `json:"count"`
	Items             []driveextract.SkippedBlockedItem `json:"items"`
}

// GetConnection handles GET /v1/connectors/drive/connection/{id}. It
// reads the persisted drive_connections row plus a count of drive_files
// and exposes the empty-drive contract: a connection with status=healthy
// and zero indexed files is rendered by Screen 3 as
// "Healthy — no in-scope files yet".
func (h *DriveHandlers) GetConnection(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "drive connection DB pool is not wired")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing connection id")
		return
	}

	var (
		providerID   string
		accountLabel string
		accessMode   string
		status       string
		healthReason string
		scopeJSON    []byte
	)
	err := h.pool.QueryRow(r.Context(),
		`SELECT provider_id, account_label, access_mode, status, COALESCE(last_health_reason, ''), scope
		   FROM drive_connections WHERE id=$1`, id,
	).Scan(&providerID, &accountLabel, &accessMode, &status, &healthReason, &scopeJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "no drive connection with id "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	var scopePayload struct {
		FolderIDs     []string `json:"folder_ids"`
		IncludeShared bool     `json:"include_shared"`
	}
	if len(scopeJSON) > 0 {
		_ = json.Unmarshal(scopeJSON, &scopePayload)
	}

	var indexed int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM drive_files WHERE connection_id=$1`, id,
	).Scan(&indexed); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	var skipped int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM drive_files WHERE connection_id=$1 AND extraction_state IN ('skipped', 'blocked')`, id,
	).Scan(&skipped); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	var retryableWork int64
	if err := h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM drive_provider_work_queue WHERE connection_id=$1 AND status IN ('queued', 'retryable', 'running')`, id,
	).Scan(&retryableWork); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	progress, err := h.latestDriveProgress(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	recentActivity, err := h.recentDriveActivity(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	view := DriveConnectionView{
		ID:           id,
		ProviderID:   providerID,
		AccountLabel: accountLabel,
		AccessMode:   accessMode,
		Status:       status,
		HealthReason: healthReason,
		Scope: DriveConnectScope{
			FolderIDs:     scopePayload.FolderIDs,
			IncludeShared: scopePayload.IncludeShared,
		},
		IndexedCount:       indexed,
		SkippedCount:       skipped,
		RetryableWorkCount: retryableWork,
		EmptyDrive:         indexed == 0,
		Progress:           progress,
		RecentActivity:     recentActivity,
	}
	writeJSON(w, http.StatusOK, view)
}

// GetArtifactDetail handles GET /v1/drive/artifacts/{id}. It returns the
// Screen 6 detail payload — preview/text/metadata/versions — for one
// drive artifact. Tombstoned and permission-lost artifacts MUST stay
// queryable (SCN-038-012, design.md §11) so this handler still serves
// the row but suppresses extracted bytes and surfaces a banner so the
// PWA can disable byte-delivery actions.
func (h *DriveHandlers) GetArtifactDetail(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "drive detail DB pool is not wired")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing artifact id")
		return
	}
	detail, err := LoadDriveArtifactDetail(r.Context(), h.pool, id)
	if errors.Is(err, errDriveDetailNotFound) {
		writeError(w, http.StatusNotFound, "ARTIFACT_NOT_FOUND", "no drive artifact with id "+id)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// GetSkippedBlocked handles GET
// /v1/connectors/drive/connection/{id}/skipped.
func (h *DriveHandlers) GetSkippedBlocked(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "drive connection DB pool is not wired")
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "missing connection id")
		return
	}

	items, err := driveextract.NewPostgresStore(h.pool).ListSkippedBlocked(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, DriveSkippedBlockedResponse{
		ConnectionID: id,
		Total:        len(items),
		Groups:       groupSkippedBlocked(items),
	})
}

func groupSkippedBlocked(items []driveextract.SkippedBlockedItem) []DriveSkippedBlockedGroup {
	groups := []DriveSkippedBlockedGroup{}
	indexByKey := map[string]int{}
	for _, item := range items {
		key := item.ExtractionState + "\x00" + item.SkipReason + "\x00" + item.RecommendedAction
		groupIndex, ok := indexByKey[key]
		if !ok {
			groups = append(groups, DriveSkippedBlockedGroup{
				ExtractionState:   item.ExtractionState,
				SkipReason:        item.SkipReason,
				RecommendedAction: item.RecommendedAction,
				Items:             []driveextract.SkippedBlockedItem{},
			})
			groupIndex = len(groups) - 1
			indexByKey[key] = groupIndex
		}
		groups[groupIndex].Items = append(groups[groupIndex].Items, item)
		groups[groupIndex].Count = len(groups[groupIndex].Items)
	}
	return groups
}

func (h *DriveHandlers) latestDriveProgress(ctx context.Context, connectionID string) (*DriveProgressView, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT phase, status, total_seen, indexed_count, skipped_count, upserted_count,
		        moved_count, tombstoned_count, last_error
		   FROM drive_scan_jobs
		  WHERE connection_id=$1
		  ORDER BY updated_at DESC
		  LIMIT 1`, connectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var progress DriveProgressView
	if err := rows.Scan(&progress.Phase, &progress.Status, &progress.TotalSeen, &progress.IndexedCount, &progress.SkippedCount, &progress.UpsertedCount, &progress.MovedCount, &progress.TombstonedCount, &progress.LastError); err != nil {
		return nil, err
	}
	return &progress, rows.Err()
}

func (h *DriveHandlers) recentDriveActivity(ctx context.Context, connectionID string) ([]DriveActivityView, error) {
	rows, err := h.pool.Query(ctx,
		`SELECT phase, status, indexed_count, updated_at::text
		   FROM drive_scan_jobs
		  WHERE connection_id=$1
		  ORDER BY updated_at DESC
		  LIMIT 5`, connectionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activity := []DriveActivityView{}
	for rows.Next() {
		var item DriveActivityView
		if err := rows.Scan(&item.Phase, &item.Status, &item.IndexedCount, &item.UpdatedAt); err != nil {
			return nil, err
		}
		activity = append(activity, item)
	}
	return activity, rows.Err()
}
