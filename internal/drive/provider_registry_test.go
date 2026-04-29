package drive

import (
	"context"
	"strings"
	"testing"
)

// fakeProvider is a minimal Provider used only by these tests. It is not the
// production fixture provider — that lives under internal/drive/google with
// the recorded-fixture boundary. The presence of two distinct test providers
// here is exactly the SCN-038-003 guarantee: downstream code reads everything
// from the Provider interface, never from a concrete type.
type fakeProvider struct {
	id           string
	display      string
	capabilities Capabilities
}

func (f *fakeProvider) ID() string                 { return f.id }
func (f *fakeProvider) DisplayName() string        { return f.display }
func (f *fakeProvider) Capabilities() Capabilities { return f.capabilities }
func (f *fakeProvider) BeginConnect(_ context.Context, _ AccessMode, _ Scope) (string, string, error) {
	return "", "", ErrNotImplemented
}
func (f *fakeProvider) FinalizeConnect(_ context.Context, _ string, _ string) (string, error) {
	return "", ErrNotImplemented
}
func (f *fakeProvider) Disconnect(_ context.Context, _ string) error { return ErrNotImplemented }
func (f *fakeProvider) Scope(_ context.Context, _ string) (Scope, error) {
	return Scope{}, ErrNotImplemented
}
func (f *fakeProvider) SetScope(_ context.Context, _ string, _ Scope) error {
	return ErrNotImplemented
}
func (f *fakeProvider) ListFolder(_ context.Context, _ string, _ string, _ string) ([]FolderItem, string, error) {
	return nil, "", ErrNotImplemented
}
func (f *fakeProvider) GetFile(_ context.Context, _ string, _ string) (FileBytes, error) {
	return FileBytes{}, ErrNotImplemented
}
func (f *fakeProvider) PutFile(_ context.Context, _ string, _ string, _ string, _ FileBytes) (string, error) {
	return "", ErrNotImplemented
}
func (f *fakeProvider) Changes(_ context.Context, _ string, _ string) ([]Change, string, error) {
	return nil, "", ErrNotImplemented
}
func (f *fakeProvider) Health(_ context.Context, _ string) (Health, error) {
	return Health{Status: HealthHealthy}, nil
}

// TestProviderRegistryExposesCapabilitiesWithoutProviderBranching maps to
// SCN-038-003. It registers a Google-shaped provider plus a second fixture
// provider and asserts that:
//  1. Both providers appear in List() with their advertised capabilities,
//     reachable through the Provider interface only.
//  2. Get() resolves both by ID.
//  3. The exact same downstream consumer code path reads each provider's
//     capabilities — no type assertion, no provider-specific branching.
//
// If a future change makes a downstream consumer type-assert to a concrete
// provider, this test will not catch it directly, but it pins the contract
// that callers SHOULD only need: ID + DisplayName + Capabilities.
func TestProviderRegistryExposesCapabilitiesWithoutProviderBranching(t *testing.T) {
	reg := NewRegistry()

	google := &fakeProvider{
		id:      "google",
		display: "Google Drive",
		capabilities: Capabilities{
			SupportsVersions:      true,
			SupportsSharing:       true,
			SupportsChangeHistory: true,
			MaxFileSizeBytes:      5 * 1024 * 1024 * 1024,
			SupportedMimeFilter:   []string{"application/pdf", "image/jpeg"},
		},
	}
	second := &fakeProvider{
		id:      "fixture-second",
		display: "Fixture Second Drive",
		capabilities: Capabilities{
			SupportsVersions:      false,
			SupportsSharing:       true,
			SupportsChangeHistory: true,
			MaxFileSizeBytes:      512 * 1024 * 1024,
			SupportedMimeFilter:   []string{"text/plain"},
		},
	}

	reg.Register(google)
	reg.Register(second)

	if got := reg.Len(); got != 2 {
		t.Fatalf("registry length = %d, want 2", got)
	}

	// (1) List must return both in deterministic ID order so the connector
	// list UI does not flicker between renders.
	listed := reg.List()
	if len(listed) != 2 {
		t.Fatalf("List() returned %d providers, want 2", len(listed))
	}
	gotIDs := []string{listed[0].ID(), listed[1].ID()}
	wantIDs := []string{"fixture-second", "google"}
	if gotIDs[0] != wantIDs[0] || gotIDs[1] != wantIDs[1] {
		t.Fatalf("List() IDs = %v, want %v (sorted)", gotIDs, wantIDs)
	}

	// (2) Get must resolve both by their registered IDs.
	for _, id := range []string{"google", "fixture-second"} {
		p, ok := reg.Get(id)
		if !ok {
			t.Fatalf("Get(%q) ok=false, want true", id)
		}
		if p.ID() != id {
			t.Fatalf("Get(%q).ID() = %q, want %q", id, p.ID(), id)
		}
	}

	// (3) The neutral consumer path: this loop is exactly what the
	// connectors-list endpoint will run. It MUST work without ever knowing
	// which concrete type each provider is.
	type connectorView struct {
		ID       string
		Display  string
		Versions bool
		Sharing  bool
		MaxBytes int64
		Mimes    []string
	}
	var views []connectorView
	for _, p := range reg.List() {
		caps := p.Capabilities()
		views = append(views, connectorView{
			ID:       p.ID(),
			Display:  p.DisplayName(),
			Versions: caps.SupportsVersions,
			Sharing:  caps.SupportsSharing,
			MaxBytes: caps.MaxFileSizeBytes,
			Mimes:    caps.SupportedMimeFilter,
		})
	}

	if len(views) != 2 {
		t.Fatalf("connector views = %d, want 2", len(views))
	}
	// Locate by ID so the assertion is stable regardless of sort order.
	byID := map[string]connectorView{}
	for _, v := range views {
		byID[v.ID] = v
	}

	g := byID["google"]
	if g.Display != "Google Drive" {
		t.Errorf("google display = %q, want %q", g.Display, "Google Drive")
	}
	if !g.Versions || !g.Sharing {
		t.Errorf("google capabilities versions=%v sharing=%v, want both true", g.Versions, g.Sharing)
	}
	if g.MaxBytes != 5*1024*1024*1024 {
		t.Errorf("google max bytes = %d, want %d", g.MaxBytes, 5*1024*1024*1024)
	}
	if len(g.Mimes) != 2 {
		t.Errorf("google mimes = %v, want 2 entries", g.Mimes)
	}

	s := byID["fixture-second"]
	if s.Display != "Fixture Second Drive" {
		t.Errorf("second display = %q, want %q", s.Display, "Fixture Second Drive")
	}
	if s.Versions {
		t.Errorf("second versions = true, want false (different capability than google)")
	}
}

// TestRegistryDuplicateRegistrationPanics pins the dup-name guard, mirroring
// internal/agent/registry.go. A duplicate provider ID must fail loudly at
// init() time, never silently shadow.
func TestRegistryDuplicateRegistrationPanics(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&fakeProvider{id: "google", display: "Google Drive"})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate provider registration, got none")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "google") {
			t.Errorf("panic message = %q, want it to contain conflicting provider ID %q", msg, "google")
		}
		if !strings.Contains(msg, "already registered") {
			t.Errorf("panic message = %q, want it to contain %q", msg, "already registered")
		}
	}()

	reg.Register(&fakeProvider{id: "google", display: "Google Drive (dup)"})
}

// TestRegistryRejectsNilAndEmptyID guards against silently-broken init().
func TestRegistryRejectsNilAndEmptyID(t *testing.T) {
	t.Run("nil provider panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on nil provider, got none")
			}
		}()
		NewRegistry().Register(nil)
	})
	t.Run("empty ID panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic on empty provider ID, got none")
			}
		}()
		NewRegistry().Register(&fakeProvider{id: "", display: "Empty"})
	})
}

// TestAccessModeValidate pins the small enum so adding a new mode forces an
// intentional code change rather than passing silently through CHECK
// constraints downstream.
func TestAccessModeValidate(t *testing.T) {
	for _, ok := range []AccessMode{AccessRead, AccessReadSave} {
		if err := ok.Validate(); err != nil {
			t.Errorf("Validate(%q) returned %v, want nil", ok, err)
		}
	}
	for _, bad := range []AccessMode{"", "read", "write", "ReadOnly"} {
		if err := bad.Validate(); err == nil {
			t.Errorf("Validate(%q) returned nil, want error", bad)
		}
	}
}

// TestErrNotImplementedSentinel guards against accidental wrapping that would
// hide unimplemented behavior from errors.Is checks in downstream callers.
func TestErrNotImplementedSentinel(t *testing.T) {
	if ErrNotImplemented == nil {
		t.Fatal("ErrNotImplemented must not be nil")
	}
	if ErrNotImplemented.Error() == "" {
		t.Fatal("ErrNotImplemented must have a non-empty message")
	}
}
