// Spec 058 BUG-058-EXTERNAL-INFRA-MISSING (BLOCKER-3) — tests for the shared
// admin scaffold + the extension-devices page. No DB needed: the page is
// exercised through a fake extensiondevices.Store and a scripted AuthGate.
package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api/admin/extensiondevices"
)

// fakeStore is an in-memory extensiondevices.Store for tests.
type fakeStore struct {
	devices    []extensiondevices.Device
	err        error
	gotFilter  string
	filterSeen bool
}

func (f *fakeStore) AggregateDevices(_ context.Context, ownerUserIDFilter string) ([]extensiondevices.Device, error) {
	f.gotFilter = ownerUserIDFilter
	f.filterSeen = true
	if f.err != nil {
		return nil, f.err
	}
	return f.devices, nil
}

func adminGate(owner string, isAdmin, ok bool) AuthGate {
	return func(_ *http.Request) (string, bool, bool) { return owner, isAdmin, ok }
}

func sampleDevices() []extensiondevices.Device {
	t0 := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	return []extensiondevices.Device{
		{OwnerUserID: "user-b", SourceDeviceID: "dev-2", FirstSeenAt: t0, LastSeenAt: t0.Add(48 * time.Hour), VisitCount30d: 7},
		{OwnerUserID: "user-a", SourceDeviceID: "dev-1", FirstSeenAt: t0, LastSeenAt: t0.Add(24 * time.Hour), VisitCount30d: 3},
	}
}

func TestDevicesHandler_RendersTableForAdmin(t *testing.T) {
	store := &fakeStore{devices: sampleDevices()}
	h := NewDevicesHandler(store, adminGate("", true, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rr.Body.String()
	// Shared scaffold chrome present.
	for _, want := range []string{"<!DOCTYPE html>", "Smackerel Admin", "/admin/agent/traces", "/admin/auth/tokens", `href="/admin/extension/devices"`} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page missing scaffold element %q", want)
		}
	}
	// Active nav highlight on the devices link.
	if !strings.Contains(body, `href="/admin/extension/devices" class="active"`) {
		t.Errorf("devices nav link should be marked active:\n%s", body)
	}
	// Both device rows rendered.
	for _, want := range []string{"dev-1", "dev-2", "user-a", "user-b"} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered page missing device value %q", want)
		}
	}
	// Admin caller ⇒ no owner filter applied.
	if store.gotFilter != "" {
		t.Errorf("admin caller filter = %q, want empty (all owners)", store.gotFilter)
	}
}

func TestDevicesHandler_EmptyState(t *testing.T) {
	store := &fakeStore{devices: nil}
	h := NewDevicesHandler(store, adminGate("", true, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "No extension devices") {
		t.Errorf("empty state message missing:\n%s", rr.Body.String())
	}
}

func TestDevicesHandler_NonAdminScopedToOwnOwner(t *testing.T) {
	store := &fakeStore{devices: sampleDevices()}
	h := NewDevicesHandler(store, adminGate("user-a", false, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if store.gotFilter != "user-a" {
		t.Errorf("non-admin filter = %q, want user-a (own owner only)", store.gotFilter)
	}
}

func TestDevicesHandler_NonAdminMissingOwnerForbidden(t *testing.T) {
	store := &fakeStore{devices: sampleDevices()}
	h := NewDevicesHandler(store, adminGate("", false, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if store.filterSeen {
		t.Error("store must not be queried when a non-admin has no owner id")
	}
}

func TestDevicesHandler_UnauthenticatedRejected(t *testing.T) {
	store := &fakeStore{devices: sampleDevices()}
	h := NewDevicesHandler(store, adminGate("", false, false))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if store.filterSeen {
		t.Error("store must not be queried for an unauthenticated caller")
	}
}

func TestDevicesHandler_StoreErrorIs500(t *testing.T) {
	store := &fakeStore{err: errors.New("boom")}
	h := NewDevicesHandler(store, adminGate("", true, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestDevicesHandler_MethodNotAllowed(t *testing.T) {
	store := &fakeStore{devices: sampleDevices()}
	h := NewDevicesHandler(store, adminGate("", true, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/admin/extension/devices", nil))

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

func TestDevicesHandler_EscapesUserInfluencedValues(t *testing.T) {
	store := &fakeStore{devices: []extensiondevices.Device{
		{OwnerUserID: "user-x", SourceDeviceID: `<script>alert(1)</script>`, FirstSeenAt: time.Now(), LastSeenAt: time.Now(), VisitCount30d: 1},
	}}
	h := NewDevicesHandler(store, adminGate("", true, true))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/admin/extension/devices", nil))

	body := rr.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Errorf("XSS: raw script tag rendered unescaped:\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("expected the device id to be HTML-escaped:\n%s", body)
	}
}

func TestNewDevicesHandler_NilArgsPanic(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic on nil store")
			}
		}()
		NewDevicesHandler(nil, adminGate("", true, true))
	})
	t.Run("nil gate", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("expected panic on nil gate")
			}
		}()
		NewDevicesHandler(&fakeStore{}, nil)
	})
}

func TestNavLinks_MarksActive(t *testing.T) {
	links := navLinks("/admin/auth/tokens")
	var activeCount int
	for _, l := range links {
		if l.Active {
			activeCount++
			if l.Href != "/admin/auth/tokens" {
				t.Errorf("wrong link active: %q", l.Href)
			}
		}
	}
	if activeCount != 1 {
		t.Errorf("expected exactly 1 active nav link, got %d", activeCount)
	}
}
