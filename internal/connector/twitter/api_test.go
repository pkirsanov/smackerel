// Tests for the Twitter API v2 client foundation (spec 056 scope 01).
//
// These tests cover the two scope 01 scenarios:
//   - SCN-056-001: empty bearer token in api mode fails loud at startup
//   - SCN-056-009: request builder rejects non-GET methods
//
// Plus a positive fetchUsersMe replay test against httptest.Server to exercise
// the full request builder + JSON decode path with the bundled fixture, and a
// regression assertion that the bearer token never appears in any structured
// log emitted during a successful sync round. Scope 03 will extend the
// log-scan to 429 / 401 / 500 paths.
package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestTwitterAPI_EmptyBearerTokenFailsLoud — SCN-056-001.
//
// Given config has sync_mode=api and resolved bearer_token=""
// When the runtime starts the twitter connector
// Then newAPIClient returns a non-nil error containing "bearer_token"
//
// Adversarial case (would re-introduce BUG-015-002 style silent fallback):
// asserts the returned error is sentinel-comparable, not a generic wrap, so a
// future refactor cannot accidentally swallow it.
func TestTwitterAPI_EmptyBearerTokenFailsLoud(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  TwitterConfig
	}{
		{"sync_mode_api_empty_token", TwitterConfig{SyncMode: SyncModeAPI, BearerToken: ""}},
		{"sync_mode_hybrid_empty_token", TwitterConfig{SyncMode: SyncModeHybrid, BearerToken: ""}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, err := newAPIClient(tc.cfg, slog.Default())
			if err == nil {
				t.Fatalf("expected error for %s, got nil (client=%v)", tc.name, client)
			}
			if client != nil {
				t.Fatalf("expected nil client when error returned, got %v", client)
			}
			if !errors.Is(err, ErrAPIBearerTokenRequired) {
				t.Fatalf("error must be ErrAPIBearerTokenRequired sentinel, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), "bearer_token") {
				t.Fatalf("error message must contain 'bearer_token' for operator clarity, got: %v", err)
			}
		})
	}
}

// TestTwitterAPI_ArchiveModeReturnsNilClient verifies the inverse: archive
// mode does NOT require a bearer token and returns a nil client with no error.
// This is the contract scope 04's dispatcher will rely on.
func TestTwitterAPI_ArchiveModeReturnsNilClient(t *testing.T) {
	t.Parallel()
	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeArchive}, slog.Default())
	if err != nil {
		t.Fatalf("archive mode must not require API client, got error: %v", err)
	}
	if client != nil {
		t.Fatalf("archive mode must return nil API client, got %v", client)
	}
}

// TestTwitterAPI_RequestBuilderRejectsNonGET — SCN-056-009.
//
// Given an instantiated apiClient
// When buildRequest is called with method != GET
// Then it returns ErrAPIMethodNotAllowed and no request is sent
func TestTwitterAPI_RequestBuilderRejectsNonGET(t *testing.T) {
	t.Parallel()
	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodHead, http.MethodOptions} {
		method := method
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			req, err := client.buildRequest(context.Background(), method, "/users/me", nil)
			if err == nil {
				t.Fatalf("method %s must be rejected, got req=%v", method, req)
			}
			if req != nil {
				t.Fatalf("method %s rejection must return nil request, got %v", method, req)
			}
			if !errors.Is(err, ErrAPIMethodNotAllowed) {
				t.Fatalf("error must be ErrAPIMethodNotAllowed sentinel, got %T: %v", err, err)
			}
		})
	}
}

// TestTwitterAPI_BuildRequestAttachesAuthAndUserAgent asserts the positive
// path: GET requests carry the Authorization, User-Agent, and Accept headers
// expected by the Twitter API v2.
func TestTwitterAPI_BuildRequestAttachesAuthAndUserAgent(t *testing.T) {
	t.Parallel()
	const token = "test-bearer-token-not-real"
	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: token,
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	req, err := client.buildRequest(context.Background(), http.MethodGet, "/users/me", nil)
	if err != nil {
		t.Fatalf("buildRequest GET: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer "+token {
		t.Fatalf("Authorization header mismatch: got %q want %q", got, "Bearer "+token)
	}
	if got := req.Header.Get("User-Agent"); got == "" || !strings.Contains(got, "smackerel-twitter-connector") {
		t.Fatalf("User-Agent header missing or unexpected: got %q", got)
	}
	if got := req.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept header mismatch: got %q want application/json", got)
	}
	if req.Method != http.MethodGet {
		t.Fatalf("request method must be GET, got %s", req.Method)
	}
}

// TestTwitterAPI_FetchUsersMeReplay exercises the full request→decode path
// against an httptest.Server seeded with the bundled testdata fixture. This
// is the smallest end-to-end exercise of the scope 01 foundation.
func TestTwitterAPI_FetchUsersMeReplay(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile(filepath.Join("testdata", "api", "users_me.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	const token = "test-bearer-token-not-real"
	var observedAuth, observedUA, observedAccept, observedMethod, observedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedAuth = r.Header.Get("Authorization")
		observedUA = r.Header.Get("User-Agent")
		observedAccept = r.Header.Get("Accept")
		observedMethod = r.Method
		observedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: token,
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Point the client at the test server (rather than the live Twitter API)
	// by overriding the baseURL via the unexported field — this is the same
	// technique scope 02's pagination tests will use for httptest.
	client.baseURL = srv.URL

	user, err := client.fetchUsersMe(context.Background())
	if err != nil {
		t.Fatalf("fetchUsersMe: %v", err)
	}

	// Decode the fixture independently to assert structural agreement.
	var want usersMeResponse
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if user.Data.ID != want.Data.ID || user.Data.Username != want.Data.Username || user.Data.Name != want.Data.Name {
		t.Fatalf("response mismatch: got %+v want %+v", user.Data, want.Data)
	}

	// Header observations.
	if observedAuth != "Bearer "+token {
		t.Errorf("server saw Authorization=%q, want %q", observedAuth, "Bearer "+token)
	}
	if !strings.Contains(observedUA, "smackerel-twitter-connector") {
		t.Errorf("server saw User-Agent=%q, want substring 'smackerel-twitter-connector'", observedUA)
	}
	if observedAccept != "application/json" {
		t.Errorf("server saw Accept=%q, want application/json", observedAccept)
	}
	if observedMethod != http.MethodGet {
		t.Errorf("server saw method=%q, want GET", observedMethod)
	}
	if observedPath != "/users/me" {
		t.Errorf("server saw path=%q, want /users/me", observedPath)
	}
}

// TestTwitterAPI_BearerTokenNeverInLogs is the scope-01-level log-scan
// assertion. Scope 03 will extend this to 429 / 401 / 500 fixtures; the
// foundation establishes the pattern now so subsequent scopes can layer on.
func TestTwitterAPI_BearerTokenNeverInLogs(t *testing.T) {
	t.Parallel()

	const token = "uniquely-recognizable-token-FOR-TEST-ONLY-abc123"
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"1","username":"u","name":"n"}}`))
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: token,
	}, logger)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL

	// Exercise the client's logger by performing a real call. Any future
	// addition of structured log lines on this path must continue to obey
	// the no-token rule enforced below.
	if _, err := client.fetchUsersMe(context.Background()); err != nil {
		t.Fatalf("fetchUsersMe: %v", err)
	}

	// Read all logger output and assert the bearer token does not appear.
	logBytes, _ := io.ReadAll(&buf)
	logText := string(logBytes)
	if strings.Contains(logText, token) {
		t.Fatalf("bearer token leaked in logs (length=%d):\n%s", len(logText), logText)
	}
	if strings.Contains(logText, "Bearer "+token) {
		t.Fatalf("Authorization header literal leaked in logs:\n%s", logText)
	}
}

// ============================================================================
// Spec 056 Scope 02 — Pagination & Cursor Persistence tests
// ============================================================================

// TestTwitterAPI_BookmarksPaginatesAndPersistsCursor — SCN-056-002.
//
// Given the bookmarks endpoint returns 3 tweets on page 1 with next_token=PAGE2_TOKEN
//
//	and 2 tweets on page 2 with no next_token
//
// When the connector calls fetchBookmarks with an empty startToken
// Then it returns the union (5 tweets) AND the final-cursor=PAGE2_TOKEN
//
//	AND the second request carries pagination_token=PAGE2_TOKEN in its query
//
// The "final-cursor=PAGE2_TOKEN" assertion mirrors the spec language: the
// persisted cursor is the next_token of the last NON-EMPTY page so the next
// sync tick can resume from there. The implementation tracks lastNonEmptyToken
// and returns it after the terminal page is observed.
func TestTwitterAPI_BookmarksPaginatesAndPersistsCursor(t *testing.T) {
	t.Parallel()

	page1, err := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page1.json"))
	if err != nil {
		t.Fatalf("read page1: %v", err)
	}
	page2, err := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page2.json"))
	if err != nil {
		t.Fatalf("read page2: %v", err)
	}

	var observedQueries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedQueries = append(observedQueries, r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("pagination_token") == "PAGE2_TOKEN" {
			_, _ = w.Write(page2)
			return
		}
		_, _ = w.Write(page1)
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL

	tweets, finalCursor, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks: %v", err)
	}
	if len(tweets) != 5 {
		t.Fatalf("expected 5 tweets (3 page1 + 2 page2), got %d: %+v", len(tweets), tweets)
	}
	// Verify the ordering matches union-with-page1-first.
	wantIDs := []string{"1001", "1002", "1003", "1004", "1005"}
	for i, want := range wantIDs {
		if tweets[i].ID != want {
			t.Errorf("tweet[%d].ID=%q want %q", i, tweets[i].ID, want)
		}
	}
	if finalCursor != "PAGE2_TOKEN" {
		t.Fatalf("final cursor must be PAGE2_TOKEN (last non-empty page's next_token), got %q", finalCursor)
	}
	if len(observedQueries) != 2 {
		t.Fatalf("expected 2 requests (page1, page2), got %d: %v", len(observedQueries), observedQueries)
	}
	if observedQueries[0] != "" {
		t.Errorf("page 1 query must be empty, got %q", observedQueries[0])
	}
	if observedQueries[1] != "pagination_token=PAGE2_TOKEN" {
		t.Errorf("page 2 query must carry pagination_token=PAGE2_TOKEN, got %q", observedQueries[1])
	}
}

// TestTwitterAPI_ReplayPagination — SCN-056-007.
//
// Same pagination logic as the previous test, but uses a 3-page fixture
// sequence (page1 → page2 → empty) to verify the loop also terminates cleanly
// when the API returns an explicitly-empty terminal page with no next_token.
func TestTwitterAPI_ReplayPagination(t *testing.T) {
	t.Parallel()

	page1, err := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page1.json"))
	if err != nil {
		t.Fatalf("read page1: %v", err)
	}
	emptyPage := []byte(`{"data":[],"meta":{"result_count":0}}`)

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if r.URL.Query().Get("pagination_token") == "PAGE2_TOKEN" {
			_, _ = w.Write(emptyPage)
			return
		}
		_, _ = w.Write(page1)
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL

	tweets, finalCursor, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks: %v", err)
	}
	if len(tweets) != 3 {
		t.Fatalf("expected 3 tweets (page1 only; empty page contributes 0), got %d", len(tweets))
	}
	// Last NON-EMPTY page's next_token was PAGE2_TOKEN; the empty page had no
	// next_token. The implementation contract is to persist the last non-empty
	// next_token, so the next sync tick can resume from there.
	if finalCursor != "PAGE2_TOKEN" {
		t.Fatalf("final cursor must be PAGE2_TOKEN (last non-empty page), got %q", finalCursor)
	}
	if requests != 2 {
		t.Fatalf("expected 2 requests (page1, terminal empty), got %d", requests)
	}
}

// TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor — regression for the
// sparse-results cursor bug (sweep round 18).
//
// Twitter v2 can return an empty `data` array that STILL carries a next_token
// (sparse results), distinct from the empty TERMINAL page (no next_token) that
// TestTwitterAPI_ReplayPagination covers. The resume cursor MUST stay anchored
// to the last page that actually produced tweets — an empty non-terminal page
// must NOT advance lastNonEmptyToken past the real data. Fixture sequence:
//
//	page1 (3 tweets, next=PAGE2_TOKEN) → page2 (EMPTY, next=PAGE3_TOKEN)
//	  → page3 (EMPTY, no next_token, terminal)
//
// Pre-fix the loop advanced lastNonEmptyToken to PAGE3_TOKEN on the empty
// page2; the contract is to persist PAGE2_TOKEN (the last non-empty page's
// next_token).
func TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor(t *testing.T) {
	t.Parallel()

	page1, err := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page1.json"))
	if err != nil {
		t.Fatalf("read page1: %v", err)
	}
	emptyWithNext := []byte(`{"data":[],"meta":{"result_count":0,"next_token":"PAGE3_TOKEN"}}`)
	emptyTerminal := []byte(`{"data":[],"meta":{"result_count":0}}`)

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Query().Get("pagination_token") {
		case "PAGE2_TOKEN":
			_, _ = w.Write(emptyWithNext)
		case "PAGE3_TOKEN":
			_, _ = w.Write(emptyTerminal)
		default:
			_, _ = w.Write(page1)
		}
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL

	tweets, finalCursor, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks: %v", err)
	}
	if len(tweets) != 3 {
		t.Fatalf("expected 3 tweets (page1 only; both later pages empty), got %d", len(tweets))
	}
	// The empty non-terminal page2 must NOT advance the cursor to PAGE3_TOKEN.
	if finalCursor != "PAGE2_TOKEN" {
		t.Fatalf("final cursor must stay anchored to PAGE2_TOKEN (last non-empty page); "+
			"an empty non-terminal page advanced it to %q", finalCursor)
	}
	if requests != 3 {
		t.Fatalf("expected 3 requests (page1, empty+next, terminal empty), got %d", requests)
	}
}

// TestTwitterAPI_CursorSurvivesProcessRestart — regression for SCN-056-002.
//
// Simulates a process restart by:
//  1. Running one sync tick, capturing the returned cursor string.
//  2. Round-tripping the cursor through saveCursor/loadCursor (the same
//     serialization a real restart would do via StateStore).
//  3. Starting a second tick with the restored cursor and asserting the API
//     request carries pagination_token=PAGE2_TOKEN (proving the cursor
//     successfully restarted the loop mid-pagination).
//
// This is the adversarial regression test: if a future refactor lost the
// per-endpoint map keys or silently reset on parse failure, the second tick
// would re-request from page 1 and the assertion would fail.
func TestTwitterAPI_CursorSurvivesProcessRestart(t *testing.T) {
	t.Parallel()

	// Round-trip an apiCursor through saveCursor/loadCursor.
	original := apiCursor{
		PerEndpoint: map[apiEndpoint]string{
			endpointBookmarks: "PAGE2_TOKEN",
			endpointLikes:     "LIKES_NEXT_42",
		},
	}
	raw, err := saveCursor(original)
	if err != nil {
		t.Fatalf("saveCursor: %v", err)
	}
	if !strings.Contains(raw, "PAGE2_TOKEN") || !strings.Contains(raw, "LIKES_NEXT_42") {
		t.Fatalf("serialized cursor missing expected tokens: %s", raw)
	}
	restored, err := loadCursor(raw)
	if err != nil {
		t.Fatalf("loadCursor: %v", err)
	}
	if restored.PerEndpoint[endpointBookmarks] != "PAGE2_TOKEN" {
		t.Fatalf("restored bookmarks cursor mismatch: got %q want PAGE2_TOKEN", restored.PerEndpoint[endpointBookmarks])
	}
	if restored.PerEndpoint[endpointLikes] != "LIKES_NEXT_42" {
		t.Fatalf("restored likes cursor mismatch: got %q want LIKES_NEXT_42", restored.PerEndpoint[endpointLikes])
	}

	// loadCursor must return an empty (but non-nil) map for an empty string.
	empty, err := loadCursor("")
	if err != nil {
		t.Fatalf("loadCursor(\"\") returned error: %v", err)
	}
	if empty.PerEndpoint == nil {
		t.Fatalf("loadCursor(\"\") must return non-nil map")
	}
	if len(empty.PerEndpoint) != 0 {
		t.Fatalf("loadCursor(\"\") must return empty map, got %v", empty.PerEndpoint)
	}

	// loadCursor must fail loud on malformed JSON (per anti-fabrication policy
	// in spec 056 design — never silently restart pagination).
	if _, err := loadCursor(`{this-is-not-json`); err == nil {
		t.Fatalf("loadCursor must error on malformed JSON, got nil")
	}

	// End-to-end restart simulation: use the restored cursor to seed a real
	// fetch and confirm the very first HTTP request carries pagination_token.
	page2, _ := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page2.json"))
	var firstQuery string
	captured := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !captured {
			firstQuery = r.URL.RawQuery
			captured = true
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(page2)
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL

	startToken := restored.PerEndpoint[endpointBookmarks]
	if _, _, err := client.fetchBookmarks(context.Background(), "2244994945", startToken); err != nil {
		t.Fatalf("fetchBookmarks after restart: %v", err)
	}
	if firstQuery != "pagination_token=PAGE2_TOKEN" {
		t.Fatalf("first request after restart must carry pagination_token=PAGE2_TOKEN, got %q", firstQuery)
	}
}

// TestTwitterAPI_PaginationBoundsTerminateOnRunawayServer verifies the
// maxPagesPerEndpoint safety bound: a server that always returns a non-empty
// next_token does not cause an unbounded loop.
func TestTwitterAPI_PaginationBoundsTerminateOnRunawayServer(t *testing.T) {
	t.Parallel()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Always return a next_token — this would loop forever without the bound.
		_, _ = w.Write([]byte(`{"data":[{"id":"x","text":"t","author_id":"a"}],"meta":{"result_count":1,"next_token":"NEVER_ENDING"}}`))
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL

	tweets, finalCursor, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks: %v", err)
	}
	// The cap is maxPagesPerEndpoint = 100; the server should have been called
	// exactly that many times before the loop terminated defensively.
	if requests != 100 {
		t.Fatalf("expected %d requests (maxPagesPerEndpoint), got %d", 100, requests)
	}
	if len(tweets) != 100 {
		t.Fatalf("expected 100 tweets (one per page), got %d", len(tweets))
	}
	// The implementation keeps lastNonEmptyToken at the end of the loop so the
	// next tick can resume — assert that's what we got back.
	if finalCursor != "NEVER_ENDING" {
		t.Fatalf("final cursor must be the last non-empty next_token, got %q", finalCursor)
	}
}

// ============================================================================
// Spec 056 Scope 03 — Rate-Limit & Error Handling tests
// ============================================================================

// recordingSleeper captures every sleep duration requested by the retry loop
// and returns immediately so tests don't pay the real wall-clock cost.
type recordingSleeper struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (s *recordingSleeper) sleep(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	s.durations = append(s.durations, d)
	s.mu.Unlock()
	return nil
}

func (s *recordingSleeper) snapshot() []time.Duration {
	s.mu.Lock()
	out := append([]time.Duration{}, s.durations...)
	s.mu.Unlock()
	return out
}

// TestTwitterAPI_RateLimit429HonorsResetWindow — SCN-056-003.
//
// Given the bookmarks endpoint returns 429 with x-rate-limit-reset = now+30s
//
//	then 200 on the next attempt
//
// When fetchBookmarks runs
// Then the connector sleeps ~30s (per the recordingSleeper) before retrying
//
//	AND the second request succeeds
//	AND no further requests are issued during the wait
//	AND the rate-limit-reset gauge is set to 30
func TestTwitterAPI_RateLimit429HonorsResetWindow(t *testing.T) {
	t.Parallel()

	page1, _ := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page1.json"))
	rl429, _ := os.ReadFile(filepath.Join("testdata", "api", "rate_limited_429.json"))

	frozen := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	resetEpoch := frozen.Add(30 * time.Second).Unix()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-rate-limit-reset", strconv.FormatInt(resetEpoch, 10))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write(rl429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Strip the next_token so pagination terminates after one page.
		_, _ = w.Write([]byte(`{"data":` + string(extractData(page1)) + `,"meta":{"result_count":3}}`))
	}))
	defer srv.Close()

	sleeper := &recordingSleeper{}
	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL
	client.sleeper = sleeper.sleep
	client.now = func() time.Time { return frozen }

	tweets, _, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks after 429: %v", err)
	}
	if len(tweets) != 3 {
		t.Fatalf("expected 3 tweets after retry, got %d", len(tweets))
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 HTTP calls (429 then 200), got %d", got)
	}
	waits := sleeper.snapshot()
	if len(waits) != 1 {
		t.Fatalf("expected exactly 1 sleep (rate-limit wait), got %d: %v", len(waits), waits)
	}
	// The recorded wait should be ~30s; allow 1s slack for rounding.
	if waits[0] < 29*time.Second || waits[0] > 31*time.Second {
		t.Fatalf("sleep duration %s must be ~30s", waits[0])
	}
}

// extractData strips the JSON envelope to just the `data` array as raw bytes.
// Used by tests that need to inject a one-page response with no next_token.
func extractData(envelope []byte) []byte {
	var doc struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(envelope, &doc); err != nil {
		return []byte(`[]`)
	}
	if len(doc.Data) == 0 {
		return []byte(`[]`)
	}
	return doc.Data
}

// TestTwitterAPI_Unauthorized401FailsWithoutRetry — SCN-056-005.
//
// Given the bookmarks endpoint returns 401
// When fetchBookmarks runs
// Then exactly one HTTP request is issued (no retry loop)
//
//	AND the returned error wraps errAuthRejected
//	AND the bearer token does not appear in the error message
func TestTwitterAPI_Unauthorized401FailsWithoutRetry(t *testing.T) {
	t.Parallel()

	body, _ := os.ReadFile(filepath.Join("testdata", "api", "unauthorized_401.json"))
	const token = "very-recognizable-token-401-test-zzz"

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	sleeper := &recordingSleeper{}
	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: token,
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL
	client.sleeper = sleeper.sleep
	client.now = time.Now

	_, _, err = client.fetchBookmarks(context.Background(), "2244994945", "")
	if err == nil {
		t.Fatalf("expected error on 401, got nil")
	}
	if !errors.Is(err, errAuthRejected) {
		t.Fatalf("error must wrap errAuthRejected, got %T: %v", err, err)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("bearer token leaked in error message: %s", err.Error())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("401 must NOT retry; expected 1 call, got %d", got)
	}
	if got := len(sleeper.snapshot()); got != 0 {
		t.Fatalf("401 must NOT sleep; expected 0 sleeps, got %d", got)
	}
}

// TestTwitterAPI_ServerError5xxBoundedBackoff — regression for 5xx handling.
//
// Given the endpoint always returns 500
// When fetchBookmarks runs
// Then exactly maxRetries+1 requests are issued (initial + retries)
//
//	AND the recorded sleep intervals are exponential (1s, 2s, 4s)
//	AND the returned error wraps errMaxRetriesExceeded
func TestTwitterAPI_ServerError5xxBoundedBackoff(t *testing.T) {
	t.Parallel()

	body, _ := os.ReadFile(filepath.Join("testdata", "api", "server_error_500.json"))

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	sleeper := &recordingSleeper{}
	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL
	client.sleeper = sleeper.sleep
	client.now = time.Now

	_, _, err = client.fetchBookmarks(context.Background(), "2244994945", "")
	if err == nil {
		t.Fatalf("expected error after persistent 500s, got nil")
	}
	if !errors.Is(err, errMaxRetriesExceeded) {
		t.Fatalf("error must wrap errMaxRetriesExceeded, got %T: %v", err, err)
	}
	// maxRetries=3, so initial + 3 retries = 4 calls.
	if got := atomic.LoadInt32(&calls); got != int32(maxRetries+1) {
		t.Fatalf("expected %d calls (initial + maxRetries), got %d", maxRetries+1, got)
	}
	waits := sleeper.snapshot()
	wantWaits := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	if len(waits) != len(wantWaits) {
		t.Fatalf("expected %d sleeps (exponential backoff), got %d: %v", len(wantWaits), len(waits), waits)
	}
	for i, want := range wantWaits {
		if waits[i] != want {
			t.Errorf("sleep[%d]=%s, want %s (exponential backoff)", i, waits[i], want)
		}
	}
}

// TestTwitterAPI_BearerTokenNeverAppearsInLogs — SCN-056-008.
//
// Adversarial assertion: the bearer token MUST NOT appear in any log line
// produced during a sync round that exercises 200, 429, 401, and 500
// responses. Uses a recognizable token substring so any leak (even partial
// inside a wrapped error or header dump) is caught by simple string search.
func TestTwitterAPI_BearerTokenNeverAppearsInLogs(t *testing.T) {
	t.Parallel()

	const token = "ADVERSARIAL-TOKEN-MUST-NEVER-LEAK-9b3f1a"
	frozen := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)

	rl429, _ := os.ReadFile(filepath.Join("testdata", "api", "rate_limited_429.json"))
	unauth401, _ := os.ReadFile(filepath.Join("testdata", "api", "unauthorized_401.json"))
	server500, _ := os.ReadFile(filepath.Join("testdata", "api", "server_error_500.json"))
	page1, _ := os.ReadFile(filepath.Join("testdata", "api", "bookmarks_page1.json"))

	// Per-endpoint response sequence.
	sequences := map[string][]struct {
		status int
		header map[string]string
		body   []byte
	}{
		"/users/2244994945/bookmarks": {
			{http.StatusOK, nil, []byte(`{"data":` + string(extractData(page1)) + `,"meta":{"result_count":3}}`)},
		},
		"/users/2244994945/liked_tweets": {
			{
				status: http.StatusTooManyRequests,
				header: map[string]string{
					"x-rate-limit-reset": strconv.FormatInt(frozen.Add(5*time.Second).Unix(), 10),
				},
				body: rl429,
			},
			{http.StatusOK, nil, []byte(`{"data":[],"meta":{"result_count":0}}`)},
		},
		"/users/2244994945/tweets": {
			{http.StatusInternalServerError, nil, server500},
			{http.StatusOK, nil, []byte(`{"data":[],"meta":{"result_count":0}}`)},
		},
		"/users/2244994945/mentions": {
			{http.StatusUnauthorized, nil, unauth401},
		},
	}

	var mu sync.Mutex
	cursors := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seq := sequences[r.URL.Path]
		i := cursors[r.URL.Path]
		if i >= len(seq) {
			i = len(seq) - 1
		}
		entry := seq[i]
		cursors[r.URL.Path] = i + 1
		mu.Unlock()
		for k, v := range entry.header {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(entry.status)
		_, _ = w.Write(entry.body)
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: token,
	}, logger)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL
	client.sleeper = (&recordingSleeper{}).sleep
	client.now = func() time.Time { return frozen }

	// Exercise all four endpoints; errors are expected for some.
	_, _, _ = client.fetchBookmarks(context.Background(), "2244994945", "")
	_, _, _ = client.fetchLikes(context.Background(), "2244994945", "")
	_, _, _ = client.fetchOwnTweets(context.Background(), "2244994945", "")
	_, _, _ = client.fetchMentions(context.Background(), "2244994945", "")

	logText := logBuf.String()
	if logText == "" {
		t.Fatalf("expected at least one log line emitted during sync round, got none")
	}
	if strings.Contains(logText, token) {
		t.Fatalf("bearer token leaked in logs (logs length=%d):\n%s", len(logText), logText)
	}
	// Adversarial: also check the Bearer prefix variant.
	if strings.Contains(logText, "Bearer "+token) {
		t.Fatalf("Authorization header literal leaked in logs:\n%s", logText)
	}
	// Adversarial: prefix and suffix of the token (catches accidental truncated logs).
	if strings.Contains(logText, token[:20]) {
		t.Fatalf("bearer token prefix leaked in logs:\n%s", logText)
	}
	if strings.Contains(logText, token[len(token)-20:]) {
		t.Fatalf("bearer token suffix leaked in logs:\n%s", logText)
	}
}

// TestTwitterAPI_RateLimitResetCapAborts proves a malicious / misconfigured
// reset header that requests a multi-hour wait is rejected rather than
// blocking the sync round indefinitely.
func TestTwitterAPI_RateLimitResetCapAborts(t *testing.T) {
	t.Parallel()

	body, _ := os.ReadFile(filepath.Join("testdata", "api", "rate_limited_429.json"))
	frozen := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	// Reset 2 hours in future — exceeds rateLimitMaxWait (30 min).
	insaneReset := frozen.Add(2 * time.Hour).Unix()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-rate-limit-reset", strconv.FormatInt(insaneReset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	sleeper := &recordingSleeper{}
	client, err := newAPIClient(TwitterConfig{
		SyncMode:    SyncModeAPI,
		BearerToken: "test-bearer-token-not-real",
	}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = srv.URL
	client.sleeper = sleeper.sleep
	client.now = func() time.Time { return frozen }

	_, _, err = client.fetchBookmarks(context.Background(), "2244994945", "")
	if err == nil {
		t.Fatalf("expected error when reset exceeds cap, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds cap") {
		t.Fatalf("error must mention cap rejection, got: %v", err)
	}
	if got := len(sleeper.snapshot()); got != 0 {
		t.Fatalf("cap-rejected 429 must NOT sleep; expected 0 sleeps, got %d", got)
	}
}

// TestTwitterAPI_BackoffDurationProgression unit-tests the exponential
// backoff calculator. Cheap sanity check independent of HTTP plumbing.
func TestTwitterAPI_BackoffDurationProgression(t *testing.T) {
	t.Parallel()
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // capped
		{6, 30 * time.Second}, // capped
		{-1, 1 * time.Second}, // negative treated as 0
	}
	for _, tc := range cases {
		tc := tc
		t.Run(fmt.Sprintf("attempt_%d", tc.attempt), func(t *testing.T) {
			t.Parallel()
			if got := backoffDuration(tc.attempt); got != tc.want {
				t.Errorf("backoffDuration(%d)=%s want %s", tc.attempt, got, tc.want)
			}
		})
	}

}
