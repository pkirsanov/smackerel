package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/smackerel/smackerel/internal/drive"
)

// fakeDriveProvider is a minimal drive.Provider used to drive the
// DriveHandlers.ListConnectors test without depending on the production
// Google provider's init() registration. It is intentionally a hand-rolled
// fake (rather than imported from internal/drive/google) so the test can
// assert that the HTTP layer round-trips ANY provider through the neutral
// surface — i.e. that the endpoint truly is provider-neutral.
type fakeDriveProvider struct {
	id              string
	disp            string
	caps            drive.Capabilities
	scope           drive.Scope
	beginConnectErr error
}

func (f *fakeDriveProvider) ID() string          { return f.id }
func (f *fakeDriveProvider) DisplayName() string { return f.disp }
func (f *fakeDriveProvider) Capabilities() drive.Capabilities {
	return f.caps
}
func (f *fakeDriveProvider) BeginConnect(_ context.Context, _ drive.AccessMode, _ drive.Scope) (string, string, error) {
	if f.beginConnectErr != nil {
		return "", "", f.beginConnectErr
	}
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

// TestDriveHandlersConnectDoesNotLeakInternalErrors is an adversarial
// regression test for the MIT-038-S-005 error leakage fix. When
// BeginConnect fails with an internal error containing DB schema details,
// the HTTP response MUST NOT contain that internal error text. Prior to
// the fix, err.Error() was passed directly to the JSON response, leaking
// database constraint names, column names, and internal state.
//
// Removing the slog + generic message pattern and reverting to err.Error()
// would cause this test to fail.
func TestDriveHandlersConnectDoesNotLeakInternalErrors(t *testing.T) {
	sensitiveErr := errors.New("pq: duplicate key value violates unique constraint \"drive_oauth_states_pkey\" DETAIL: Key (state_token)=(abc123) already exists")
	fakeProvider := &fakeDriveProvider{
		id:              "google",
		disp:            "Google Drive",
		caps:            drive.Capabilities{MaxFileSizeBytes: 1024},
		beginConnectErr: sensitiveErr,
	}
	reg := drive.NewRegistry()
	reg.Register(fakeProvider)
	h := NewDriveHandlers(reg)

	body := `{"provider_id":"google","owner_user_id":"test-owner","access_mode":"read_only","scope":{}}`
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/drive/connect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Connect(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}

	respBody := rec.Body.String()

	// The response MUST NOT contain the internal error details.
	if strings.Contains(respBody, "drive_oauth_states_pkey") {
		t.Fatalf("ADVERSARIAL FAILURE: response body contains internal constraint name.\n"+
			"Error leakage fix may have been reverted.\nBody: %s", respBody)
	}
	if strings.Contains(respBody, "state_token") {
		t.Fatalf("ADVERSARIAL FAILURE: response body contains internal column name.\n"+
			"Error leakage fix may have been reverted.\nBody: %s", respBody)
	}
	if strings.Contains(respBody, "abc123") {
		t.Fatalf("ADVERSARIAL FAILURE: response body contains internal state token value.\n"+
			"Error leakage fix may have been reverted.\nBody: %s", respBody)
	}

	// The response SHOULD contain the generic error code.
	if !strings.Contains(respBody, "BEGIN_CONNECT_FAILED") {
		t.Fatalf("response missing expected error code BEGIN_CONNECT_FAILED.\nBody: %s", respBody)
	}
}

// TestDriveHandlersGetConnectionRejectsNonUUID is an adversarial regression
// test for chaos finding C-002. Prior to the fix, passing a non-UUID path
// parameter caused PostgreSQL to return a 22P02 (invalid input syntax)
// error which the handler wrapped as a 500 with the raw SQL state message
// in the response. The fix validates UUID format at the handler boundary
// and returns 400 with a stable error code.
func TestDriveHandlersGetConnectionRejectsNonUUID(t *testing.T) {
	reg := drive.NewRegistry()
	reg.Register(&fakeDriveProvider{id: "google", disp: "Google Drive"})
	h := NewDriveHandlersWithPool(reg, nil) // pool is nil — UUID check fires before DB

	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/drive/connection/not-a-uuid", nil)
	rec := httptest.NewRecorder()

	// chi URL params must be injected for handler tests.
	rctx := chiRouteContext(t, map[string]string{"id": "not-a-uuid"})
	req = req.WithContext(rctx)

	h.GetConnection(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INVALID_CONNECTION_ID") {
		t.Fatalf("body missing INVALID_CONNECTION_ID error code; body=%s", rec.Body.String())
	}
	// Adversarial: raw SQL error must NOT appear.
	if strings.Contains(rec.Body.String(), "SQLSTATE") || strings.Contains(rec.Body.String(), "invalid input syntax") {
		t.Fatalf("ADVERSARIAL FAILURE: response leaks raw SQL error; body=%s", rec.Body.String())
	}
}

// TestDriveHandlersGetSkippedBlockedRejectsNonUUID mirrors the UUID
// validation test for the /skipped sibling endpoint (chaos finding C-002).
func TestDriveHandlersGetSkippedBlockedRejectsNonUUID(t *testing.T) {
	reg := drive.NewRegistry()
	reg.Register(&fakeDriveProvider{id: "google", disp: "Google Drive"})
	h := NewDriveHandlersWithPool(reg, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/drive/connection/not-a-uuid/skipped", nil)
	rec := httptest.NewRecorder()

	rctx := chiRouteContext(t, map[string]string{"id": "not-a-uuid"})
	req = req.WithContext(rctx)

	h.GetSkippedBlocked(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INVALID_CONNECTION_ID") {
		t.Fatalf("body missing INVALID_CONNECTION_ID error code; body=%s", rec.Body.String())
	}
}

// chiRouteContext builds a chi route context with the given URL params
// for handler-level unit tests.
func chiRouteContext(t *testing.T, params map[string]string) context.Context {
	t.Helper()
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return context.WithValue(context.Background(), chi.RouteCtxKey, rctx)
}
