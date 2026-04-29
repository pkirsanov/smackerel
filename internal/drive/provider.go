// Package drive defines the provider-neutral cloud-drive abstraction that
// downstream Smackerel features (search, save rules, retrieval, agent tools)
// depend on. The interface is deliberately the only contract callers see — no
// caller MAY type-assert to a concrete provider, so adding a second provider
// (or swapping Google for a fixture in tests) does not require branching in
// downstream code.
//
// Spec 038 Scope 1 establishes the contract and the registry. Concrete
// behavior (BeginConnect, FinalizeConnect, ListFolder, GetFile, PutFile,
// Changes, Health) is filled in by provider-specific packages such as
// internal/drive/google. Methods that land in later scopes return
// ErrNotImplemented from the scaffold so callers fail loudly rather than
// silently succeed against a stub.
//
// Connect was split into BeginConnect (returns the provider auth URL plus a
// server-side state token bound to the in-flight redirect) and FinalizeConnect
// (consumes the state + authorization code returned by the provider on
// callback) per the design decision logged in design.md §2.1 / §10. A single
// Connect method cannot drive the OAuth redirect leg.
package drive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// ErrNotImplemented is returned by provider methods whose behavior is planned
// for a later scope. Callers MUST treat this as a hard failure — never a soft
// skip — so partially-shipped providers cannot pretend to deliver scope work
// they have not yet implemented.
var ErrNotImplemented = errors.New("drive: not implemented")

// AccessMode describes whether a connection has read-only or read-save scope.
// Storing the mode on the connection lets the save service refuse writes
// against a read-only connection without consulting OAuth scopes at runtime.
type AccessMode string

const (
	// AccessRead grants read-only access to in-scope drive files.
	AccessRead AccessMode = "read_only"
	// AccessReadSave grants read access plus the ability to write to the
	// in-scope folders via PutFile.
	AccessReadSave AccessMode = "read_save"
)

// Validate returns an error if the mode is not one of the recognized values.
func (m AccessMode) Validate() error {
	switch m {
	case AccessRead, AccessReadSave:
		return nil
	}
	return fmt.Errorf("drive: invalid access mode %q (want %q or %q)", string(m), AccessRead, AccessReadSave)
}

// HealthStatus is the connector-level health enum surfaced to UI and to the
// save/retrieve workers so they can short-circuit before issuing provider
// calls that are guaranteed to fail.
type HealthStatus string

const (
	HealthHealthy      HealthStatus = "healthy"
	HealthDegraded     HealthStatus = "degraded"
	HealthFailing      HealthStatus = "failing"
	HealthDisconnected HealthStatus = "disconnected"
)

// Health is the snapshot returned by Provider.Health.
type Health struct {
	Status     HealthStatus
	Reason     string
	ObservedAt time.Time
}

// Capabilities advertises which optional behaviors a provider supports so that
// downstream code can branch on declared capabilities, never on the concrete
// provider type. New capability flags MUST be added here rather than via type
// assertions.
type Capabilities struct {
	SupportsVersions      bool
	SupportsSharing       bool
	SupportsChangeHistory bool
	MaxFileSizeBytes      int64
	SupportedMimeFilter   []string
}

// Scope describes a per-connection access scope. FolderIDs are
// provider-specific identifiers that MUST be returned by ListFolder when the
// scope is configured. Empty FolderIDs means "entire connected drive".
type Scope struct {
	FolderIDs     []string
	IncludeShared bool
}

// FolderItem is a single entry returned by ListFolder.
type FolderItem struct {
	ProviderFileID     string
	ProviderRevisionID string
	Title              string
	MimeType           string
	SizeBytes          int64
	FolderPath         []string
	IsFolder           bool
	OwnerLabel         string
	ProviderURL        string
	ModifiedAt         time.Time
}

// FileBytes is the payload returned by GetFile / accepted by PutFile. The
// caller is responsible for closing Reader.
type FileBytes struct {
	MimeType string
	Reader   io.ReadCloser
	Size     int64
}

// Change represents a single delta returned by Changes(cursor).
type Change struct {
	ProviderFileID string
	Kind           ChangeKind
	NewCursor      string
}

// ChangeKind enumerates the possible delta types so downstream code does not
// have to inspect provider-specific change payloads.
type ChangeKind string

const (
	ChangeUpsert    ChangeKind = "upsert"
	ChangeMove      ChangeKind = "move"
	ChangeTrash     ChangeKind = "trash"
	ChangeDelete    ChangeKind = "delete"
	ChangePermLost  ChangeKind = "permission_lost"
	ChangeCursorInv ChangeKind = "cursor_invalid"
)

// Provider is the only contract downstream code is allowed to depend on. New
// providers register themselves through the package registry.
type Provider interface {
	// ID returns the stable provider identifier (e.g. "google", "fixture").
	// IDs are case-sensitive and MUST match the provider key in
	// config/smackerel.yaml (drive.providers.<id>).
	ID() string

	// DisplayName returns the human-facing label rendered in the connectors
	// list. UI code MUST NOT compose its own label from the provider ID.
	DisplayName() string

	// Capabilities reports the provider's declared capabilities. Downstream
	// branching MUST go through this surface, never a type assertion.
	Capabilities() Capabilities

	// BeginConnect starts the provider authorization flow. Implementations
	// MUST generate a cryptographically random state token, persist the
	// (owner, provider, accessMode, scope) tuple to drive_oauth_states
	// keyed by that token, and return the provider authorization URL plus
	// the state token. The HTTP layer is responsible for redirecting the
	// user agent to authURL; the provider returns control synchronously
	// after persisting state.
	BeginConnect(ctx context.Context, accessMode AccessMode, scope Scope) (authURL string, state string, err error)

	// FinalizeConnect completes the provider authorization flow after the
	// user agent has been redirected back to the OAuth callback endpoint
	// with state + code. Implementations MUST look up the persisted
	// drive_oauth_states row, verify it has not expired, exchange the
	// authorization code for provider tokens, persist a drive_connections
	// row with expires_at, and delete the consumed drive_oauth_states row
	// before returning the connection identifier.
	FinalizeConnect(ctx context.Context, state string, code string) (connectionID string, err error)

	// Disconnect tears down the connection and revokes any provider-side
	// tokens. The drive_connections row is marked disconnected, not deleted.
	Disconnect(ctx context.Context, connectionID string) error

	// Scope returns the active access scope for the connection.
	Scope(ctx context.Context, connectionID string) (Scope, error)

	// SetScope replaces the connection's active access scope.
	SetScope(ctx context.Context, connectionID string, scope Scope) error

	// ListFolder returns one page of items beneath the given folder. An
	// empty pageToken means "first page". A returned nextPageToken of "" means
	// no more pages.
	ListFolder(ctx context.Context, connectionID string, folderID string, pageToken string) (items []FolderItem, nextPageToken string, err error)

	// GetFile fetches the bytes for a provider file. Implementations MUST
	// stream and MUST NOT buffer the full file in memory.
	GetFile(ctx context.Context, connectionID string, providerFileID string) (FileBytes, error)

	// PutFile writes a file to the provider under the resolved folder path.
	// Implementations MUST honor AccessMode and refuse writes against
	// read-only connections with a non-nil error.
	PutFile(ctx context.Context, connectionID string, folderID string, title string, body FileBytes) (providerFileID string, err error)

	// Changes returns the next batch of deltas since cursor. An empty cursor
	// means "from the beginning". A returned cursor MAY be invalid; callers
	// MUST handle ChangeCursorInv by issuing a bounded rescan.
	Changes(ctx context.Context, connectionID string, cursor string) (changes []Change, nextCursor string, err error)

	// Health returns the current connection health snapshot. Implementations
	// MUST NOT perform expensive provider calls here — Health is read on the
	// hot path of save/retrieve workers.
	Health(ctx context.Context, connectionID string) (Health, error)
}

// Registry holds the set of registered providers. The default registry
// (DefaultRegistry) is intentionally exported so package init() functions in
// concrete provider packages can register themselves at startup, mirroring
// the established pattern in internal/agent/registry.go.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty Registry. Tests SHOULD construct their own
// registry rather than mutating the package default to keep cases isolated.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds p to the registry. It panics with a deterministic message
// when a duplicate ID is registered, matching the dup-name guard in
// internal/agent/registry.go. Panicking at init() forces config conflicts to
// surface at process start rather than silently shadowing a provider.
func (r *Registry) Register(p Provider) {
	if p == nil {
		panic("drive: Register called with nil provider")
	}
	id := p.ID()
	if id == "" {
		panic("drive: Register called with empty provider ID")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.providers[id]; ok {
		panic(fmt.Sprintf("drive: provider %q already registered (existing %T, attempted %T)", id, existing, p))
	}
	r.providers[id] = p
}

// Get returns the provider with the given ID, or (nil, false) if no such
// provider is registered.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// List returns all registered providers in stable sorted ID order so the
// connectors list and tests are deterministic.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Provider, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.providers[id])
	}
	return out
}

// Len returns the number of registered providers.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.providers)
}

// DefaultRegistry is the process-wide registry that concrete provider
// packages register themselves into via init().
var DefaultRegistry = NewRegistry()
