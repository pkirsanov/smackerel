package cardrewards

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- ParseGCalCredential -----------------------------------------------------

func TestParseGCalCredential_Valid(t *testing.T) {
	raw := `{"client_id":"cid","client_secret":"csec","refresh_token":"rt","token_uri":"https://example/token"}`
	c, err := ParseGCalCredential(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.ClientID != "cid" || c.ClientSecret != "csec" || c.RefreshToken != "rt" {
		t.Fatalf("fields not parsed: %+v", c)
	}
	if c.TokenURI != "https://example/token" {
		t.Fatalf("token_uri not parsed: %q", c.TokenURI)
	}
}

func TestParseGCalCredential_DefaultsTokenURI(t *testing.T) {
	raw := `{"client_id":"cid","client_secret":"csec","refresh_token":"rt"}`
	c, err := ParseGCalCredential(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.TokenURI != defaultGCalTokenURL {
		t.Fatalf("expected default token uri, got %q", c.TokenURI)
	}
}

func TestParseGCalCredential_Empty(t *testing.T) {
	if _, err := ParseGCalCredential("   "); err == nil {
		t.Fatal("expected error for empty credential")
	}
}

func TestParseGCalCredential_BadJSON(t *testing.T) {
	if _, err := ParseGCalCredential("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseGCalCredential_MissingFields(t *testing.T) {
	cases := []string{
		`{"client_secret":"s","refresh_token":"r"}`,                // no client_id
		`{"client_id":"c","refresh_token":"r"}`,                    // no client_secret
		`{"client_id":"c","client_secret":"s"}`,                    // no refresh_token
		`{"client_id":"","client_secret":"s","refresh_token":"r"}`, // empty client_id
	}
	for _, raw := range cases {
		if _, err := ParseGCalCredential(raw); err == nil {
			t.Fatalf("expected missing-field error for %s", raw)
		}
	}
}

// --- eventID determinism -----------------------------------------------------

func TestEventID_DeterministicAndValid(t *testing.T) {
	uid := "smackerel-cardrec-2026-06-restaurants"
	a := eventID(uid)
	b := eventID(uid)
	if a != b {
		t.Fatalf("eventID not deterministic: %q != %q", a, b)
	}
	if eventID("other-uid") == a {
		t.Fatal("different UIDs produced the same event id")
	}
	// Google event ids: base32hex alphabet (0-9a-v), 5..1024 chars. sha1 hex is
	// 40 chars of 0-9a-f — a valid subset.
	if len(a) != 40 {
		t.Fatalf("expected 40-char sha1 hex, got %d", len(a))
	}
	for _, r := range a {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'v')) {
			t.Fatalf("event id char %q outside base32hex alphabet", r)
		}
	}
}

// --- test server helpers -----------------------------------------------------

// newFakeGoogle builds an httptest server emulating the token endpoint and the
// calendar events endpoints, plus a client pointed at it. The eventsStore tracks
// which event ids exist so insert/update/get/delete behave realistically.
func newFakeGoogle(t *testing.T) (*GoogleCalendarClient, *fakeGoogleState, func()) {
	t.Helper()
	st := &fakeGoogleState{events: map[string]map[string]any{}}

	mux := http.NewServeMux()
	// Token endpoint.
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&st.tokenCalls, 1)
		_ = r.ParseForm()
		if r.PostFormValue("grant_type") != "refresh_token" || r.PostFormValue("refresh_token") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_request"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fresh-access-token","expires_in":3600,"token_type":"Bearer"}`)
	})
	// Calendar events: GET/PUT/DELETE /calendars/{cal}/events/{id}, POST /calendars/{cal}/events
	mux.HandleFunc("/calendars/", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fresh-access-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// path: /calendars/<cal>/events[/<id>]
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/calendars/"), "/")
		// parts[0]=cal, parts[1]="events", parts[2]=id?
		var id string
		if len(parts) >= 3 {
			id = parts[2]
		}
		st.mu.Lock()
		defer st.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			if _, ok := st.events[id]; ok {
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, `{"id":"`+id+`"}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			newID, _ := body["id"].(string)
			st.events[newID] = body
			atomic.AddInt32(&st.inserts, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"`+newID+`"}`)
		case http.MethodPut:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			st.events[id] = body
			atomic.AddInt32(&st.updates, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"id":"`+id+`"}`)
		case http.MethodDelete:
			if _, ok := st.events[id]; ok {
				delete(st.events, id)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(mux)
	client, err := NewGoogleCalendarClient("cal@group.calendar.google.com", GCalCredential{
		ClientID: "cid", ClientSecret: "csec", RefreshToken: "rt", TokenURI: srv.URL + "/token",
	}, srv.Client())
	if err != nil {
		srv.Close()
		t.Fatalf("construct client: %v", err)
	}
	client.apiBase = srv.URL
	return client, st, srv.Close
}

type fakeGoogleState struct {
	mu         sync.Mutex
	events     map[string]map[string]any
	tokenCalls int32
	inserts    int32
	updates    int32
}

// --- PutEvent / DeleteEvent --------------------------------------------------

func TestPutEvent_InsertsThenUpdates_Idempotent(t *testing.T) {
	client, st, done := newFakeGoogle(t)
	defer done()
	ctx := context.Background()
	uid := "smackerel-cardrec-2026-06-restaurants"
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)

	// First PutEvent → insert.
	if err := client.PutEvent(ctx, uid, "Restaurants: use Chase (5%)", "desc", start, end, []string{"smackerel-cardrewards"}, map[string]string{"X-SMACKEREL-CARDREC-ID": "rec-1"}); err != nil {
		t.Fatalf("first PutEvent: %v", err)
	}
	if got := atomic.LoadInt32(&st.inserts); got != 1 {
		t.Fatalf("expected 1 insert, got %d", got)
	}
	// Second PutEvent with the SAME uid → update, not a duplicate insert.
	if err := client.PutEvent(ctx, uid, "Restaurants: use Chase (5%) updated", "desc2", start, end, nil, nil); err != nil {
		t.Fatalf("second PutEvent: %v", err)
	}
	if got := atomic.LoadInt32(&st.inserts); got != 1 {
		t.Fatalf("expected still 1 insert after re-sync, got %d", got)
	}
	if got := atomic.LoadInt32(&st.updates); got != 1 {
		t.Fatalf("expected 1 update on re-sync, got %d", got)
	}
	// Exactly one event stored under the deterministic id.
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.events) != 1 {
		t.Fatalf("expected exactly 1 stored event, got %d", len(st.events))
	}
	if _, ok := st.events[eventID(uid)]; !ok {
		t.Fatal("event not stored under deterministic id")
	}
}

func TestPutEvent_StoresUIDAndCategories(t *testing.T) {
	client, st, done := newFakeGoogle(t)
	defer done()
	uid := "smackerel-cardrec-2026-06-gas-stations"
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	if err := client.PutEvent(context.Background(), uid, "sum", "desc", start, start.Add(time.Hour), []string{"a", "b"}, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("PutEvent: %v", err)
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	ev := st.events[eventID(uid)]
	ext, _ := ev["extendedProperties"].(map[string]any)
	priv, _ := ext["private"].(map[string]any)
	if priv[gcalExtPropUID] != uid {
		t.Fatalf("uid not stored in extended props: %v", priv[gcalExtPropUID])
	}
	if priv[gcalExtPropCategories] != "a,b" {
		t.Fatalf("categories not stored: %v", priv[gcalExtPropCategories])
	}
	if priv["k"] != "v" {
		t.Fatalf("extra prop not stored: %v", priv["k"])
	}
}

func TestPutEvent_EmptyUID(t *testing.T) {
	client, _, done := newFakeGoogle(t)
	defer done()
	if err := client.PutEvent(context.Background(), "", "s", "d", time.Now(), time.Now(), nil, nil); err == nil {
		t.Fatal("expected error for empty uid")
	}
}

func TestDeleteEvent_RemovesThenIdempotent(t *testing.T) {
	client, st, done := newFakeGoogle(t)
	defer done()
	ctx := context.Background()
	uid := "smackerel-cardrec-2026-06-travel"
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	if err := client.PutEvent(ctx, uid, "s", "d", start, start.Add(time.Hour), nil, nil); err != nil {
		t.Fatalf("PutEvent: %v", err)
	}
	if err := client.DeleteEvent(ctx, uid); err != nil {
		t.Fatalf("DeleteEvent: %v", err)
	}
	st.mu.Lock()
	if len(st.events) != 0 {
		st.mu.Unlock()
		t.Fatalf("event not deleted")
	}
	st.mu.Unlock()
	// Deleting again (now 404) is a no-op success (idempotent cleanup).
	if err := client.DeleteEvent(ctx, uid); err != nil {
		t.Fatalf("second DeleteEvent should be a no-op success, got: %v", err)
	}
}

// --- access token refresh + caching ------------------------------------------

func TestAccessToken_CachedAcrossCalls(t *testing.T) {
	client, st, done := newFakeGoogle(t)
	defer done()
	ctx := context.Background()
	start := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	// Two PutEvents → multiple API calls, but the access token should be minted once.
	_ = client.PutEvent(ctx, "uid-a", "s", "d", start, start.Add(time.Hour), nil, nil)
	_ = client.PutEvent(ctx, "uid-b", "s", "d", start, start.Add(time.Hour), nil, nil)
	if got := atomic.LoadInt32(&st.tokenCalls); got != 1 {
		t.Fatalf("expected exactly 1 token refresh (cached), got %d", got)
	}
}

func TestAccessToken_RefreshFailureSurfaces(t *testing.T) {
	// Token endpoint that always 401s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	}))
	defer srv.Close()
	client, err := NewGoogleCalendarClient("cal@group.calendar.google.com", GCalCredential{
		ClientID: "c", ClientSecret: "s", RefreshToken: "r", TokenURI: srv.URL,
	}, srv.Client())
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	client.apiBase = srv.URL
	err = client.PutEvent(context.Background(), "uid", "s", "d", time.Now(), time.Now().Add(time.Hour), nil, nil)
	if err == nil {
		t.Fatal("expected error when token refresh fails")
	}
	if !strings.Contains(err.Error(), "token endpoint returned HTTP 401") {
		t.Fatalf("error should surface the token failure, got: %v", err)
	}
}

// --- constructor validation --------------------------------------------------

func TestNewGoogleCalendarClient_EmptyCalendarID(t *testing.T) {
	_, err := NewGoogleCalendarClient("", GCalCredential{ClientID: "c", ClientSecret: "s", RefreshToken: "r"}, nil)
	if err == nil {
		t.Fatal("expected error for empty calendar id")
	}
}

func TestNewGoogleCalendarClient_IncompleteCred(t *testing.T) {
	_, err := NewGoogleCalendarClient("cal", GCalCredential{ClientID: "c"}, nil)
	if err == nil {
		t.Fatal("expected error for incomplete credential")
	}
}
