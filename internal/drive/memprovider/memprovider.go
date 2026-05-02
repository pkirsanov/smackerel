// Package memprovider provides an in-memory drive.Provider used by spec
// 038 Scope 8 multi-provider tests. It is a real drive.Provider
// implementation (not a mock) so the cross-feature, observability, and
// search tests can prove the production code paths work identically
// regardless of which concrete provider produced the artifacts.
//
// The provider is registered in drive.DefaultRegistry under the stable
// ID "memdrive" via init() (matching the established pattern in
// internal/drive/google). Production wiring SHOULD NOT depend on this
// package — it exists to enumerate "more than one provider" inside test
// stacks. The package has no external network dependencies and stores
// state purely in process memory keyed by the connection ID.
//
// Per spec 038 design.md §1, downstream consumers MUST NOT depend on a
// concrete provider type. This package therefore exposes no symbol the
// spec expects callers to type-assert against — every interaction goes
// through drive.Provider / drive.DefaultRegistry.
package memprovider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/drive"
)

// providerID is the stable identifier used in config and persisted to
// drive_connections.provider_id for fixture connections.
const providerID = "memdrive"

// Provider is the in-memory drive.Provider implementation.
type Provider struct {
	mu          sync.Mutex
	connections map[string]connectionState
	files       map[string]map[string]*memFile // connectionID -> providerFileID -> file
	changeLogs  map[string][]drive.Change      // connectionID -> ordered change log
	caps        drive.Capabilities
}

type connectionState struct {
	owner string
	mode  drive.AccessMode
	scope drive.Scope
}

type memFile struct {
	item    drive.FolderItem
	bytes   []byte
	deleted bool
}

// New constructs a fresh in-memory provider. Tests SHOULD construct a
// new Provider per test to avoid cross-test bleed. The caps argument is
// the Capabilities advertised by Capabilities(); pass
// DefaultCapabilities() to get the test-friendly defaults.
func New(caps drive.Capabilities) *Provider {
	return &Provider{
		connections: map[string]connectionState{},
		files:       map[string]map[string]*memFile{},
		changeLogs:  map[string][]drive.Change{},
		caps:        caps,
	}
}

// DefaultCapabilities returns the in-memory provider's advertised
// capabilities. Limits are deliberately small enough to exercise the
// extract-skip and save-refuse paths in tests.
func DefaultCapabilities() drive.Capabilities {
	return drive.Capabilities{
		SupportsVersions:      false,
		SupportsSharing:       true,
		SupportsChangeHistory: true,
		MaxFileSizeBytes:      256 * 1024 * 1024,
		SupportedMimeFilter:   nil,
	}
}

// SeedConnection creates a connection row in memory and returns its ID
// without going through the OAuth begin/finalize round trip. Tests use
// this to pre-establish connections at known IDs that align with
// drive_connections rows they create directly in Postgres.
func (p *Provider) SeedConnection(connectionID string, owner string, mode drive.AccessMode, scope drive.Scope) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connections[connectionID] = connectionState{owner: owner, mode: mode, scope: scope}
	if _, ok := p.files[connectionID]; !ok {
		p.files[connectionID] = map[string]*memFile{}
	}
}

// AddFile inserts (or replaces) a fixture file on the named connection.
// It also appends a `change_kind=upsert` entry to the connection's
// change log so subsequent Changes(cursor) calls observe the delta.
func (p *Provider) AddFile(connectionID string, item drive.FolderItem, content []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn := p.files[connectionID]
	if conn == nil {
		conn = map[string]*memFile{}
		p.files[connectionID] = conn
	}
	conn[item.ProviderFileID] = &memFile{item: item, bytes: append([]byte(nil), content...)}
	p.changeLogs[connectionID] = append(p.changeLogs[connectionID], drive.Change{
		ProviderFileID: item.ProviderFileID,
		Kind:           drive.ChangeUpsert,
		Item:           item,
	})
}

// ID implements drive.Provider.
func (p *Provider) ID() string { return providerID }

// DisplayName implements drive.Provider.
func (p *Provider) DisplayName() string { return "Memory Drive (test)" }

// Capabilities implements drive.Provider.
func (p *Provider) Capabilities() drive.Capabilities { return p.caps }

// BeginConnect implements drive.Provider.
func (p *Provider) BeginConnect(ctx context.Context, mode drive.AccessMode, scope drive.Scope) (string, string, error) {
	if err := mode.Validate(); err != nil {
		return "", "", err
	}
	owner, err := drive.OwnerUserIDFromContext(ctx)
	if err != nil {
		return "", "", err
	}
	state, err := randomToken()
	if err != nil {
		return "", "", err
	}
	connectionID, err := randomToken()
	if err != nil {
		return "", "", err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connections["pending:"+state] = connectionState{owner: owner, mode: mode, scope: scope}
	// Encode the eventual connection ID into the URL fragment so
	// FinalizeConnect can pick it up — production providers carry this
	// via the OAuth token exchange, but the in-memory provider doesn't
	// need a real OAuth round trip.
	authURL := "memdrive://oauth/auth?state=" + state + "&conn=" + connectionID
	return authURL, state, nil
}

// FinalizeConnect implements drive.Provider. The "code" is interpreted
// as the connection ID minted by BeginConnect.
func (p *Provider) FinalizeConnect(ctx context.Context, state string, code string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pending, ok := p.connections["pending:"+state]
	if !ok {
		return "", fmt.Errorf("memprovider: unknown state token")
	}
	delete(p.connections, "pending:"+state)
	p.connections[code] = pending
	if _, ok := p.files[code]; !ok {
		p.files[code] = map[string]*memFile{}
	}
	return code, nil
}

// Disconnect implements drive.Provider.
func (p *Provider) Disconnect(_ context.Context, connectionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.connections, connectionID)
	delete(p.files, connectionID)
	delete(p.changeLogs, connectionID)
	return nil
}

// Scope implements drive.Provider.
func (p *Provider) Scope(_ context.Context, connectionID string) (drive.Scope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn, ok := p.connections[connectionID]
	if !ok {
		return drive.Scope{}, fmt.Errorf("memprovider: unknown connection %q", connectionID)
	}
	return conn.scope, nil
}

// SetScope implements drive.Provider.
func (p *Provider) SetScope(_ context.Context, connectionID string, scope drive.Scope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn, ok := p.connections[connectionID]
	if !ok {
		return fmt.Errorf("memprovider: unknown connection %q", connectionID)
	}
	conn.scope = scope
	p.connections[connectionID] = conn
	return nil
}

// ListFolder implements drive.Provider. The in-memory implementation
// returns every non-deleted file in stable provider-file-id order.
// Pagination is not implemented (every call returns the full list and
// an empty next-page token) because tests exercise small fixture sets
// and the design contract permits a single-page response for empty
// page tokens.
func (p *Provider) ListFolder(_ context.Context, connectionID string, _ string, _ string) ([]drive.FolderItem, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn := p.files[connectionID]
	if conn == nil {
		return nil, "", fmt.Errorf("memprovider: unknown connection %q", connectionID)
	}
	out := make([]drive.FolderItem, 0, len(conn))
	for _, f := range conn {
		if f.deleted {
			continue
		}
		out = append(out, f.item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProviderFileID < out[j].ProviderFileID })
	return out, "", nil
}

// GetFile implements drive.Provider.
func (p *Provider) GetFile(_ context.Context, connectionID string, providerFileID string) (drive.FileBytes, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	conn := p.files[connectionID]
	if conn == nil {
		return drive.FileBytes{}, fmt.Errorf("memprovider: unknown connection %q", connectionID)
	}
	f, ok := conn[providerFileID]
	if !ok || f.deleted {
		return drive.FileBytes{}, fmt.Errorf("memprovider: file %q not found", providerFileID)
	}
	return drive.FileBytes{
		MimeType: f.item.MimeType,
		Reader:   io.NopCloser(bytes.NewReader(f.bytes)),
		Size:     int64(len(f.bytes)),
	}, nil
}

// PutFile implements drive.Provider.
func (p *Provider) PutFile(_ context.Context, connectionID string, folderID string, title string, body drive.FileBytes) (string, error) {
	p.mu.Lock()
	state, ok := p.connections[connectionID]
	p.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("memprovider: unknown connection %q", connectionID)
	}
	if state.mode != drive.AccessReadSave {
		return "", fmt.Errorf("memprovider: connection %q is read-only", connectionID)
	}
	if body.Reader == nil {
		return "", fmt.Errorf("memprovider: body reader is nil")
	}
	defer body.Reader.Close()
	bs, err := io.ReadAll(body.Reader)
	if err != nil {
		return "", fmt.Errorf("memprovider: read upload body: %w", err)
	}
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	folderPath := folderPathFromID(folderID)
	item := drive.FolderItem{
		ProviderFileID: id,
		Title:          title,
		MimeType:       body.MimeType,
		SizeBytes:      int64(len(bs)),
		FolderPath:     folderPath,
		ProviderURL:    "memdrive://files/" + id,
		ModifiedAt:     time.Now().UTC(),
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	conn := p.files[connectionID]
	if conn == nil {
		conn = map[string]*memFile{}
		p.files[connectionID] = conn
	}
	conn[id] = &memFile{item: item, bytes: bs}
	p.changeLogs[connectionID] = append(p.changeLogs[connectionID], drive.Change{
		ProviderFileID: id,
		Kind:           drive.ChangeUpsert,
		Item:           item,
	})
	return id, nil
}

// Changes implements drive.Provider. The cursor is the decimal index of
// the next change to return; "" means "from the beginning".
func (p *Provider) Changes(_ context.Context, connectionID string, cursor string) ([]drive.Change, string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	log := p.changeLogs[connectionID]
	start := 0
	if cursor != "" {
		var n int
		if _, err := fmt.Sscanf(cursor, "%d", &n); err == nil && n >= 0 && n <= len(log) {
			start = n
		}
	}
	out := make([]drive.Change, 0, len(log)-start)
	out = append(out, log[start:]...)
	return out, fmt.Sprintf("%d", len(log)), nil
}

// Health implements drive.Provider.
func (p *Provider) Health(_ context.Context, connectionID string) (drive.Health, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.connections[connectionID]; !ok {
		return drive.Health{Status: drive.HealthDisconnected, Reason: "unknown connection"}, nil
	}
	return drive.Health{Status: drive.HealthHealthy, Reason: "memprovider healthy", ObservedAt: time.Now().UTC()}, nil
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func folderPathFromID(folderID string) []string {
	folderID = strings.TrimSpace(folderID)
	if folderID == "" || folderID == "root" {
		return []string{"Memdrive"}
	}
	// folderID is a slash-separated path in this in-memory provider;
	// callers SHOULD pass canonical paths produced by drive_folder_resolutions.
	parts := strings.Split(folderID, "/")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return append([]string(nil), out...)
}

// init registers the in-memory provider in drive.DefaultRegistry so
// tests that rely on registry lookup (e.g. retrieve.NewProviderBytesFetcher,
// save.NewService) can resolve "memdrive" without manual wiring.
//
// Production binaries link this package only when test builds bring it
// in transitively; production wiring under cmd/core does not import
// memprovider.
func init() {
	drive.DefaultRegistry.Register(New(DefaultCapabilities()))
}
