package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/smackerel/smackerel/internal/drive"
)

// fakeDriveProvider is a minimal drive.Provider used to drive the
// DriveHandlers.ListConnectors test without depending on the production
// Google provider's init() registration. It is intentionally a hand-rolled
// fake (rather than imported from internal/drive/google) so the test can
// assert that the HTTP layer round-trips ANY provider through the neutral
// surface — i.e. that the endpoint truly is provider-neutral.
type fakeDriveProvider struct {
	id    string
	disp  string
	caps  drive.Capabilities
	scope drive.Scope
}

func (f *fakeDriveProvider) ID() string          { return f.id }
func (f *fakeDriveProvider) DisplayName() string { return f.disp }
func (f *fakeDriveProvider) Capabilities() drive.Capabilities {
	return f.caps
}
func (f *fakeDriveProvider) BeginConnect(_ context.Context, _ drive.AccessMode, _ drive.Scope) (string, string, error) {
	return "", "", drive.ErrNotImplemented
}
func (f *fakeDriveProvider) FinalizeConnect(_ context.Context, _ string, _ string) (string, error) {
	return "", drive.ErrNotImplemented
}
func (f *fakeDriveProvider) Disconnect(_ context.Context, _ string) error {
	return drive.ErrNotImplemented
}
func (f *fakeDriveProvider) Scope(_ context.Context, _ string) (drive.Scope, error) {
	return f.scope, nil
}
func (f *fakeDriveProvider) SetScope(_ context.Context, _ string, _ drive.Scope) error {
	return drive.ErrNotImplemented
}
func (f *fakeDriveProvider) ListFolder(_ context.Context, _ string, _ string, _ string) ([]drive.FolderItem, string, error) {
	return nil, "", drive.ErrNotImplemented
}
func (f *fakeDriveProvider) GetFile(_ context.Context, _ string, _ string) (drive.FileBytes, error) {
	return drive.FileBytes{}, drive.ErrNotImplemented
}
func (f *fakeDriveProvider) PutFile(_ context.Context, _ string, _ string, _ string, _ drive.FileBytes) (string, error) {
	return "", drive.ErrNotImplemented
}
func (f *fakeDriveProvider) Changes(_ context.Context, _ string, _ string) ([]drive.Change, string, error) {
	return nil, "", drive.ErrNotImplemented
}
func (f *fakeDriveProvider) Health(_ context.Context, _ string) (drive.Health, error) {
	return drive.Health{Status: drive.HealthHealthy}, nil
}

// TestNewDriveHandlersPanicsOnNilRegistry pins the fail-loud constructor
// behavior. A nil registry indicates a wiring bug, and panicking at
// construction surfaces it at process start (consistent with the SST
// no-defaults discipline) rather than at first request.
func TestNewDriveHandlersPanicsOnNilRegistry(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("NewDriveHandlers(nil) did not panic; want fail-loud panic")
		}
	}()
	NewDriveHandlers(nil)
}

// TestDriveHandlersListConnectorsReturnsNeutralProviderList maps to
// SCN-038-003. It registers two providers (google + a second fixture)
// against a private drive.Registry and asserts the JSON response carries
// both providers, in deterministic ID order, with every Capabilities
// field round-tripped exactly. This proves the connectors-list endpoint
// is provider-neutral: the wire shape is derived solely from the
// drive.Provider interface, with no provider-specific branching.
func TestDriveHandlersListConnectorsReturnsNeutralProviderList(t *testing.T) {
	reg := drive.NewRegistry()
	reg.Register(&fakeDriveProvider{
		id:   "google",
		disp: "Google Drive",
		caps: drive.Capabilities{
			SupportsVersions:      true,
			SupportsSharing:       true,
			SupportsChangeHistory: true,
			MaxFileSizeBytes:      5 * 1024 * 1024 * 1024,
			SupportedMimeFilter:   []string{"application/pdf", "image/jpeg"},
		},
	})
	reg.Register(&fakeDriveProvider{
		id:   "fixture-second",
		disp: "Fixture Second Drive",
		caps: drive.Capabilities{
			SupportsVersions:      false,
			SupportsSharing:       true,
			SupportsChangeHistory: true,
			MaxFileSizeBytes:      512 * 1024 * 1024,
			SupportedMimeFilter:   []string{"text/plain"},
		},
	})

	h := NewDriveHandlers(reg)

	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/drive", nil)
	rec := httptest.NewRecorder()
	h.ListConnectors(rec, req)

	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, string(body))
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var resp DriveConnectorsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v; body=%s", err, rec.Body.String())
	}

	if len(resp.Providers) != 2 {
		t.Fatalf("providers length = %d, want 2; body=%s", len(resp.Providers), rec.Body.String())
	}

	// drive.Registry.List() returns IDs in sorted order.
	if resp.Providers[0].ID != "fixture-second" || resp.Providers[1].ID != "google" {
		t.Fatalf("provider order = [%q, %q], want [\"fixture-second\", \"google\"]",
			resp.Providers[0].ID, resp.Providers[1].ID)
	}

	// Spot-check the Google provider's full capabilities round-trip exactly.
	g := resp.Providers[1]
	if g.DisplayName != "Google Drive" {
		t.Errorf("google DisplayName = %q, want %q", g.DisplayName, "Google Drive")
	}
	if !g.Capabilities.SupportsVersions || !g.Capabilities.SupportsSharing || !g.Capabilities.SupportsChangeHistory {
		t.Errorf("google capabilities flags = %+v, want all true", g.Capabilities)
	}
	if g.Capabilities.MaxFileSizeBytes != 5*1024*1024*1024 {
		t.Errorf("google MaxFileSizeBytes = %d, want %d", g.Capabilities.MaxFileSizeBytes, 5*1024*1024*1024)
	}
	if len(g.Capabilities.SupportedMimeFilter) != 2 ||
		g.Capabilities.SupportedMimeFilter[0] != "application/pdf" ||
		g.Capabilities.SupportedMimeFilter[1] != "image/jpeg" {
		t.Errorf("google SupportedMimeFilter = %v, want [application/pdf image/jpeg]",
			g.Capabilities.SupportedMimeFilter)
	}

	// And the second provider's distinguishing fields.
	s := resp.Providers[0]
	if s.DisplayName != "Fixture Second Drive" {
		t.Errorf("second DisplayName = %q, want %q", s.DisplayName, "Fixture Second Drive")
	}
	if s.Capabilities.SupportsVersions {
		t.Errorf("second SupportsVersions = true, want false (proves capabilities are not shared across providers)")
	}
	if s.Capabilities.MaxFileSizeBytes != 512*1024*1024 {
		t.Errorf("second MaxFileSizeBytes = %d, want %d", s.Capabilities.MaxFileSizeBytes, 512*1024*1024)
	}
}

// TestDriveHandlersListConnectorsEmptyRegistryReturnsEmptyArray asserts the
// adversarial case: when no providers are registered, the response is a
// well-formed empty array (NOT null) so the PWA can render the "no
// connectors installed" empty state without a JSON-shape branch.
func TestDriveHandlersListConnectorsEmptyRegistryReturnsEmptyArray(t *testing.T) {
	reg := drive.NewRegistry()
	h := NewDriveHandlers(reg)

	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/drive", nil)
	rec := httptest.NewRecorder()
	h.ListConnectors(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// The wire shape must be {"providers":[]}, not {"providers":null}.
	body := rec.Body.String()
	const want = `{"providers":[]}`
	// Trim newline emitted by json.Encoder.
	if got := body; got != want+"\n" {
		t.Fatalf("body = %q, want %q", got, want+"\n")
	}
}
