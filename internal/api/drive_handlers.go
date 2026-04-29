package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/drive"
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
	ID           string            `json:"id"`
	ProviderID   string            `json:"provider_id"`
	AccountLabel string            `json:"account_label"`
	AccessMode   string            `json:"access_mode"`
	Status       string            `json:"status"`
	Scope        DriveConnectScope `json:"scope"`
	IndexedCount int64             `json:"indexed_count"`
	SkippedCount int64             `json:"skipped_count"`
	EmptyDrive   bool              `json:"empty_drive"`
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
		scopeJSON    []byte
	)
	err := h.pool.QueryRow(r.Context(),
		`SELECT provider_id, account_label, access_mode, status, scope
		   FROM drive_connections WHERE id=$1`, id,
	).Scan(&providerID, &accountLabel, &accessMode, &status, &scopeJSON)
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

	view := DriveConnectionView{
		ID:           id,
		ProviderID:   providerID,
		AccountLabel: accountLabel,
		AccessMode:   accessMode,
		Status:       status,
		Scope: DriveConnectScope{
			FolderIDs:     scopePayload.FolderIDs,
			IncludeShared: scopePayload.IncludeShared,
		},
		IndexedCount: indexed,
		SkippedCount: 0,
		EmptyDrive:   indexed == 0,
	}
	writeJSON(w, http.StatusOK, view)
}
