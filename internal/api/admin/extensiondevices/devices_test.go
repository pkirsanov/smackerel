package extensiondevices
package extensiondevices

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeStore struct {
	devices  []Device
	gotOwner string
	err      error
}

func (f *fakeStore) AggregateDevices(_ context.Context, ownerUserIDFilter string) ([]Device, error) {
	f.gotOwner = ownerUserIDFilter
	if f.err != nil {
		return nil, f.err
	}
	out := make([]Device, len(f.devices))
	copy(out, f.devices)
	return out, nil
}

func adminAlways(_ *http.Request) (string, bool, bool) { return "u-admin", true, true }
func userAlice(_ *http.Request) (string, bool, bool)   { return "u-alice", false, true }
func unauth(_ *http.Request) (string, bool, bool)      { return "", false, false }

// TestHandler_AdminSeesAllOwnersSorted proves the admin caller sees
// every owner and the response is sorted by (owner_user_id, source_device_id).
func TestHandler_AdminSeesAllOwnersSorted(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{devices: []Device{
		{OwnerUserID: "u-bob", SourceDeviceID: "work-desktop", FirstSeenAt: now, LastSeenAt: now, VisitCount30d: 5},
		{OwnerUserID: "u-alice", SourceDeviceID: "phone", FirstSeenAt: now, LastSeenAt: now, VisitCount30d: 1},
		{OwnerUserID: "u-alice", SourceDeviceID: "laptop", FirstSeenAt: now, LastSeenAt: now, VisitCount30d: 12},
	}}
	h := NewHandler(store, adminAlways)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/extension/devices", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if store.gotOwner != "" {
		t.Fatalf("admin caller MUST pass empty owner filter; got %q", store.gotOwner)
	}
	var got Response
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Devices) != 3 {
		t.Fatalf("want 3 devices; got %d", len(got.Devices))
	}
	wantOrder := []string{"u-alice/laptop", "u-alice/phone", "u-bob/work-desktop"}
	for i, d := range got.Devices {
		key := d.OwnerUserID + "/" + d.SourceDeviceID
		if key != wantOrder[i] {
			t.Fatalf("idx %d: got %q, want %q", i, key, wantOrder[i])
		}
	}
}

// TestHandler_NonAdminSeesOnlyOwnDevices proves the WHERE filter is
// passed for non-admin callers. Adversarial twin for spec 058
// design §3.2 ("non-admin users see only their own devices").
func TestHandler_NonAdminSeesOnlyOwnDevices(t *testing.T) {
	store := &fakeStore{devices: []Device{
		{OwnerUserID: "u-alice", SourceDeviceID: "phone"},
	}}
	h := NewHandler(store, userAlice)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/extension/devices", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if store.gotOwner != "u-alice" {
		t.Fatalf("non-admin MUST scope to own user id; got filter=%q", store.gotOwner)
	}
}

// TestHandler_UnauthenticatedRejected returns 401 when the admin
// predicate reports no session.
func TestHandler_UnauthenticatedRejected(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(store, unauth)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/extension/devices", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if store.gotOwner != "" {
		t.Fatalf("store MUST NOT be called for unauthenticated requests")
	}
}

// TestHandler_RejectsNonGET pins the method matrix.
func TestHandler_RejectsNonGET(t *testing.T) {
	h := NewHandler(&fakeStore{}, adminAlways)

	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(m, "/v1/admin/extension/devices", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("method %s: status = %d, want 405", m, rr.Code)
		}
	}
}

// TestHandler_StoreErrorReturns500 surfaces internal aggregation
// failures as a structured 500.
func TestHandler_StoreErrorReturns500(t *testing.T) {
	store := &fakeStore{err: errors.New("db blew up")}
	h := NewHandler(store, adminAlways)

	req := httptest.NewRequest(http.MethodGet, "/v1/admin/extension/devices", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

// TestNewHandler_PanicsOnNilDeps fail-loud on missing wiring.
func TestNewHandler_PanicsOnNilDeps(t *testing.T) {
	t.Run("nil store", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("want panic on nil store")
			}
		}()
		_ = NewHandler(nil, adminAlways)
	})
	t.Run("nil predicate", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("want panic on nil predicate")
			}
		}()
		_ = NewHandler(&fakeStore{}, nil)
	})
}

// Ensure the empty-devices path emits an empty array, never null —
// JSON consumers (HTMX table renderer) MUST get a list.
func TestHandler_EmptyDevicesRendersEmptyArray(t *testing.T) {
	h := NewHandler(&fakeStore{}, adminAlways)
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/extension/devices", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if want := `"devices":[]`; !contains(body, want) {
		t.Fatalf("body %q does not contain %q", body, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
