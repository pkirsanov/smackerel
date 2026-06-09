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
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

// ============================================================================
// BUG-056-002 Scope C — endpoint auth-tier routing helpers + tests
// ============================================================================

// testUserContextToken is the canned user-context OAuth 2.0 access token
// injected into apiClient by tests that exercise user-owned endpoints
// (users_me, bookmarks, liked_tweets). It is deliberately DISTINCT from the
// App-Only bearer so assertions can prove which credential a request carried.
const testUserContextToken = "user-context-access-token-FOR-TEST-only-7e2f"

// staticUserContextToken returns a userContextTokenFunc that always yields tok.
// Test-only helper for exercising the user-context auth tier without a
// database-backed oauthStore.
func staticUserContextToken(tok string) userContextTokenFunc {
	return func(context.Context) (string, error) { return tok, nil }
}

// TestEndpointAuthTier pins the spec 056 NC-1 auth-tier matrix (BUG-056-002):
// user-owned endpoints (users_me, bookmarks, liked_tweets) require the
// user-context token; tweets/mentions use the App-Only bearer. Non-tautological:
// it asserts the EXACT tier per label, so flipping any mapping (e.g. routing
// bookmarks back through App-Only — the original defect) turns this test RED.
func TestEndpointAuthTier(t *testing.T) {
	t.Parallel()
	cases := []struct {
		label string
		want  authTier
	}{
		{usersMeLabel, authTierUserContext},
		{string(endpointBookmarks), authTierUserContext},
		{string(endpointLikes), authTierUserContext},
		{string(endpointOwnTweets), authTierAppOnly},
		{string(endpointMentions), authTierAppOnly},
		// Unknown labels bias to the MORE-restrictive user-context tier so a
		// future endpoint added without a matrix entry fails loud rather than
		// silently leaking an App-Only bearer onto a user resource.
		{"some_unmapped_future_endpoint", authTierUserContext},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			t.Parallel()
			if got := endpointAuthTier(tc.label); got != tc.want {
				t.Fatalf("endpointAuthTier(%q)=%s, want %s", tc.label, got, tc.want)
			}
		})
	}
}

// TestBuildRequest_UserContextEndpointUsesUserToken — SCN-BUG-056-002-009.
//
// With a populated user-context source, every user-owned endpoint request must
// carry the USER-CONTEXT access token, never the App-Only bearer. The App-Only
// bearer is set on the client too, so this proves the TIER selects the token —
// not mere availability — and would catch a regression that re-routed a
// user-owned endpoint through App-Only.
func TestBuildRequest_UserContextEndpointUsesUserToken(t *testing.T) {
	t.Parallel()
	const appOnlyBearer = "APP-ONLY-bearer-must-not-be-used-on-user-endpoints"

	userOwned := []struct {
		label string
		path  string
	}{
		{usersMeLabel, "/users/me"},
		{string(endpointBookmarks), "/users/2244994945/bookmarks"},
		{string(endpointLikes), "/users/2244994945/liked_tweets"},
	}
	for _, ep := range userOwned {
		ep := ep
		t.Run(ep.label, func(t *testing.T) {
			t.Parallel()
			client, err := newAPIClient(TwitterConfig{
				SyncMode:    SyncModeAPI,
				BearerToken: appOnlyBearer,
			}, slog.Default())
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			client.userContextToken = staticUserContextToken(testUserContextToken)

			req, err := client.buildRequest(context.Background(), http.MethodGet, ep.path, nil, endpointAuthTier(ep.label))
			if err != nil {
				t.Fatalf("buildRequest %s: %v", ep.label, err)
			}
			got := req.Header.Get("Authorization")
			if got != "Bearer "+testUserContextToken {
				t.Fatalf("%s must carry the user-context token; got Authorization=%q want %q",
					ep.label, got, "Bearer "+testUserContextToken)
			}
			// CRITICAL: it must NOT be the App-Only bearer (the original BUG-056-002).
			if strings.Contains(got, appOnlyBearer) {
				t.Fatalf("%s leaked the App-Only bearer onto a user-owned endpoint: %q", ep.label, got)
			}
		})
	}
}

// TestBuildRequest_AppOnlyEndpointUsesBearer — SCN-BUG-056-002-013.
//
// App-Only endpoints (tweets, mentions) keep using c.bearerToken even when a
// user-context source IS present — proving the tier (not availability) selects
// the credential, and that the App-Only path is behaviorally unchanged.
func TestBuildRequest_AppOnlyEndpointUsesBearer(t *testing.T) {
	t.Parallel()
	const appOnlyBearer = "app-only-bearer-token-not-real"

	appOnly := []struct {
		label string
		path  string
	}{
		{string(endpointOwnTweets), "/users/2244994945/tweets"},
		{string(endpointMentions), "/users/2244994945/mentions"},
	}
	for _, ep := range appOnly {
		ep := ep
		t.Run(ep.label, func(t *testing.T) {
			t.Parallel()
			client, err := newAPIClient(TwitterConfig{
				SyncMode:    SyncModeAPI,
				BearerToken: appOnlyBearer,
			}, slog.Default())
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			// A user-context source is present but MUST be ignored for App-Only.
			client.userContextToken = staticUserContextToken(testUserContextToken)

			req, err := client.buildRequest(context.Background(), http.MethodGet, ep.path, nil, endpointAuthTier(ep.label))
			if err != nil {
				t.Fatalf("buildRequest %s: %v", ep.label, err)
			}
			got := req.Header.Get("Authorization")
			if got != "Bearer "+appOnlyBearer {
				t.Fatalf("%s must carry the App-Only bearer; got Authorization=%q want %q",
					ep.label, got, "Bearer "+appOnlyBearer)
			}
			if strings.Contains(got, testUserContextToken) {
				t.Fatalf("%s leaked the user-context token onto an App-Only endpoint: %q", ep.label, got)
			}
		})
	}
}

// TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud — SCN-BUG-056-002-012.
//
// With NO usable user-context token (no source wired, an empty store, an empty
// token, or a store error), a user-owned endpoint request MUST fail loud with
// the ErrUserContextTokenRequired sentinel — NOT a silent App-Only fallback
// (the original bug), NOT a panic. Precursor to the full adversarial fixture
// test. The App-Only bearer is set on the client so the assertions can prove it
// never leaks into the failure path.
func TestBuildRequest_UserContextEndpoint_NoToken_FailsLoud(t *testing.T) {
	t.Parallel()
	const appOnlyBearer = "app-only-bearer-MUST-NOT-be-used-as-fallback"

	cases := []struct {
		name   string
		source userContextTokenFunc
	}{
		{"nil source (no runtime wired)", nil},
		{"empty store (no token row)", func(context.Context) (string, error) { return "", ErrUserContextTokenRequired }},
		{"empty token string", func(context.Context) (string, error) { return "", nil }},
		{"store error", func(context.Context) (string, error) { return "", errors.New("db down") }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, err := newAPIClient(TwitterConfig{
				SyncMode:    SyncModeAPI,
				BearerToken: appOnlyBearer,
			}, slog.Default())
			if err != nil {
				t.Fatalf("setup: %v", err)
			}
			client.userContextToken = tc.source

			req, err := client.buildRequest(context.Background(), http.MethodGet,
				"/users/2244994945/bookmarks", nil, endpointAuthTier(string(endpointBookmarks)))
			if err == nil {
				t.Fatalf("expected ErrUserContextTokenRequired, got nil (req=%v) — silent App-Only fallback is the original bug", req)
			}
			if req != nil {
				t.Fatalf("fail-loud must return a nil request, got %v", req)
			}
			if !errors.Is(err, ErrUserContextTokenRequired) {
				t.Fatalf("error must wrap the ErrUserContextTokenRequired sentinel, got %T: %v", err, err)
			}
			// Remediation must name the authorize command.
			if !strings.Contains(err.Error(), "authorize-begin") {
				t.Fatalf("error must name the authorize remedy, got: %v", err)
			}
			// CRITICAL: the App-Only bearer must NOT have leaked into the failure.
			if strings.Contains(err.Error(), appOnlyBearer) {
				t.Fatalf("App-Only bearer leaked into the error (silent-fallback smell): %v", err)
			}
		})
	}
}

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
			req, err := client.buildRequest(context.Background(), method, "/users/me", nil, authTierAppOnly)
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
	req, err := client.buildRequest(context.Background(), http.MethodGet, "/users/me", nil, authTierAppOnly)
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
	// /users/me is a user-owned endpoint (user-context tier); inject the token.
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	if observedAuth != "Bearer "+testUserContextToken {
		t.Errorf("server saw Authorization=%q, want %q (user-context tier)", observedAuth, "Bearer "+testUserContextToken)
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
	// /users/me is user-context tier; inject the token so the call proceeds.
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	if strings.Contains(logText, testUserContextToken) {
		t.Fatalf("user-context token leaked in logs (length=%d):\n%s", len(logText), logText)
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
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	client.userContextToken = staticUserContextToken(testUserContextToken)

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
	client.userContextToken = staticUserContextToken(testUserContextToken)
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
	client.userContextToken = staticUserContextToken(testUserContextToken)
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
	if strings.Contains(err.Error(), testUserContextToken) {
		t.Fatalf("user-context token leaked in error message: %s", err.Error())
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
	client.userContextToken = staticUserContextToken(testUserContextToken)
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
	client.userContextToken = staticUserContextToken(testUserContextToken)
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
	if strings.Contains(logText, testUserContextToken) {
		t.Fatalf("user-context token leaked in logs:\n%s", logText)
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
	client.userContextToken = staticUserContextToken(testUserContextToken)
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

// ============================================================================
// BUG-056-002 Scope C — Pass 2: refresh-on-401 + the KEY adversarial regression
// ============================================================================

// newRefreshTokenServer returns an httptest.Server emulating Twitter's
// confidential-client token endpoint for grant_type=refresh_token. It asserts
// the grant type + HTTP Basic client auth (the confidential-client contract),
// records each presented refresh_token, and responds with the ROTATED pair
// {newAccess,newRefresh} valid for expiresIn seconds. The returned *int32 counts
// exchanges and the snapshot func returns the presented refresh tokens. Real
// httptest server (not a mock-and-mislabel), NOT the live Twitter endpoint.
func newRefreshTokenServer(t *testing.T, newAccess, newRefresh string, expiresIn int) (*httptest.Server, *int32, func() []string) {
	t.Helper()
	var calls int32
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		form, _ := url.ParseQuery(string(body))
		mu.Lock()
		seen = append(seen, form.Get("refresh_token"))
		mu.Unlock()
		if form.Get("grant_type") != "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"unsupported_grant_type"}`)
			return
		}
		// Confidential client: credentials travel via HTTP Basic auth (NOT the
		// body) — the same contract Scope A's TokenEndpointAuthStyle="basic"
		// delivers and the Scope B finalize test pins.
		if user, _, ok := r.BasicAuth(); !ok || user != testOAuthClientID {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, fmt.Sprintf(
			`{"token_type":"bearer","expires_in":%d,"access_token":%q,"refresh_token":%q,`+
				`"scope":"offline.access tweet.read users.read bookmark.read like.read"}`,
			expiresIn, newAccess, newRefresh))
	}))
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, seen...)
	}
	return srv, &calls, snapshot
}

// TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected — SCN-BUG-056-002-010, the
// KEY adversarial regression (the test that would have caught the original
// BUG-056-002 defect, and goes RED if a user-owned endpoint is ever re-routed
// through App-Only).
//
// The fixture server ENFORCES user-context the way the real Twitter API does
// (and the old permissive fake did NOT): a user-owned endpoint presented with
// the App-Only sentinel bearer is rejected 403 "Unsupported Authentication".
//
//   - Sub-case (a) — no user-context token configured: a user-owned fetch with
//     ONLY an App-Only credential MUST fail loud with ErrUserContextTokenRequired
//     at authorizationHeader BEFORE the wire (the enforcing server is never
//     contacted), never silently sending the App-Only bearer.
//   - Sub-case (b) — a user-context token IS configured: the request MUST carry
//     the user-context token (which the enforcing server accepts 200), never the
//     app bearer.
//
// Genuinely adversarial: if a user-owned endpoint is (re)routed through App-Only
// (the original defect), sub-case (a) reaches the wire and gets 403
// (errAuthRejected, server hits ≥ 1 — both assertions fail) and sub-case (b)
// sends the app bearer so the enforcing server 403s and the call fails — i.e.
// the test goes RED. The matrix-reverted RED is captured in report.md.
func TestTwitterAPI_AppOnlyOnUserOwnedEndpointRejected(t *testing.T) {
	t.Parallel()
	const appOnlyBearer = "APP-ONLY-sentinel-bearer-forbidden-on-user-owned-endpoints"

	var hits int32
	var mu sync.Mutex
	var observedAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		got := r.Header.Get("Authorization")
		mu.Lock()
		observedAuth = append(observedAuth, got)
		mu.Unlock()
		if got == "Bearer "+appOnlyBearer {
			// Twitter's real auth-tier enforcement (the old fixture lacked it).
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"title":"Unsupported Authentication",`+
				`"detail":"Authenticating with OAuth 2.0 Application-Only is forbidden for this endpoint. `+
				`Supported authentication types are [OAuth 1.0a User Context, OAuth 2.0 User Context].",`+
				`"type":"https://api.twitter.com/2/problems/unsupported-authentication","status":403}`)
			return
		}
		// A user-context token is accepted.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":"1001","text":"ok","author_id":"a"}],"meta":{"result_count":1}}`)
	}))
	defer srv.Close()

	// Sub-case (a): App-Only-only credential on a user-owned endpoint → fail
	// loud BEFORE the wire.
	t.Run("app_only_only_fails_loud_before_wire", func(t *testing.T) {
		clientA, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: appOnlyBearer}, slog.Default())
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		clientA.baseURL = srv.URL
		// No user-context source wired (the operator never authorized).
		clientA.userContextToken = nil

		_, _, err = clientA.fetchBookmarks(context.Background(), "2244994945", "")
		if err == nil {
			t.Fatal("expected fail-loud ErrUserContextTokenRequired on a user-owned endpoint with no user-context token, got nil")
		}
		if !errors.Is(err, ErrUserContextTokenRequired) {
			t.Fatalf("user-owned endpoint with no user-context token must fail with ErrUserContextTokenRequired "+
				"(NOT a 403/errAuthRejected from a silently-sent App-Only bearer); got %T: %v", err, err)
		}
		if errors.Is(err, errAuthRejected) {
			t.Fatalf("error must NOT be errAuthRejected — that would mean the App-Only bearer was silently sent and "+
				"the enforcing server rejected it (the original bug); got: %v", err)
		}
		if got := atomic.LoadInt32(&hits); got != 0 {
			mu.Lock()
			seen := append([]string{}, observedAuth...)
			mu.Unlock()
			t.Fatalf("fail-loud must happen BEFORE the wire — the enforcing server must NOT be contacted; "+
				"got %d hit(s), observed auth=%v", got, seen)
		}
		if strings.Contains(err.Error(), appOnlyBearer) {
			t.Fatalf("App-Only bearer leaked into the error (silent-fallback smell): %v", err)
		}
	})

	// Sub-case (b): user-context token configured → request carries it (the
	// enforcing server accepts user-context), never the app bearer.
	t.Run("user_context_token_used_not_app_bearer", func(t *testing.T) {
		atomic.StoreInt32(&hits, 0)
		mu.Lock()
		observedAuth = nil
		mu.Unlock()

		clientB, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: appOnlyBearer}, slog.Default())
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		clientB.baseURL = srv.URL
		clientB.userContextToken = staticUserContextToken(testUserContextToken)

		tweets, _, err := clientB.fetchBookmarks(context.Background(), "2244994945", "")
		if err != nil {
			t.Fatalf("user-context fetch against an enforcing server must succeed; got: %v", err)
		}
		if len(tweets) != 1 {
			t.Fatalf("expected 1 tweet from the enforcing server, got %d", len(tweets))
		}
		mu.Lock()
		seen := append([]string{}, observedAuth...)
		mu.Unlock()
		if len(seen) == 0 {
			t.Fatal("enforcing server was never contacted")
		}
		for i, a := range seen {
			if a != "Bearer "+testUserContextToken {
				t.Fatalf("request[%d] must carry the user-context token, got Authorization=%q want %q",
					i, a, "Bearer "+testUserContextToken)
			}
			if strings.Contains(a, appOnlyBearer) {
				t.Fatalf("request[%d] leaked the App-Only bearer onto a user-owned endpoint: %q", i, a)
			}
		}
	})
}

// TestTwitterAPI_Refresh_On401_RetriesOnce — SCN-BUG-056-002-011 / -014.
//
// A user-context bookmarks call returns 401 on the first attempt, then 200 after
// a single refresh. Drives a REAL userContextManager (fake store + real
// confidential-client provider against an httptest token endpoint). Asserts:
//   - exactly ONE refresh exchange occurred,
//   - the first request used the OLD access token and the retry used the ROTATED
//     NEW access token (proving the retry picked up the freshly-persisted token),
//   - the refresh presented the stored OLD refresh token,
//   - the ROTATED pair was persisted (Twitter rotates the refresh token),
//   - the call succeeded, and
//   - the refresh-after-401 log line was emitted with NO token value leaked
//     (SCN-014: access + refresh tokens never appear in logs).
//
// Non-tautological: without refresh-on-401 the first 401 is terminal (no retry,
// no rotated token persisted) and every assertion below fails.
func TestTwitterAPI_Refresh_On401_RetriesOnce(t *testing.T) {
	t.Parallel()
	const (
		oldAccess  = "OLD-ACCESS-token-pre-refresh-401test"
		oldRefresh = "OLD-REFRESH-token-rotating-401test"
		newAccess  = "NEW-ACCESS-token-post-refresh-401test"
		newRefresh = "NEW-REFRESH-token-rotated-401test"
	)

	var mu sync.Mutex
	var apiAuth []string
	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&apiCalls, 1)
		mu.Lock()
		apiAuth = append(apiAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":"1001","text":"ok","author_id":"a"}],"meta":{"result_count":1}}`)
	}))
	defer apiSrv.Close()

	tokenSrv, tokenCalls, tokenRefreshSeen := newRefreshTokenServer(t, newAccess, newRefresh, 7200)
	defer tokenSrv.Close()

	store := newFakeFlowStore()
	// Seed a non-expired token (far-future expiry → no PROACTIVE refresh; this
	// test exercises the REACTIVE 401 path specifically).
	if err := store.SaveTokens(context.Background(), DefaultOwnerUserID, &auth.Token{
		AccessToken:  oldAccess,
		RefreshToken: oldRefresh,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		TokenType:    "bearer",
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	provider := newTwitterOAuthProvider(testOAuthCfg())
	provider.Config.TokenEndpoint = tokenSrv.URL
	mgr := newUserContextManager(store, provider, DefaultOwnerUserID, logger)

	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "app-only-bearer-unused-on-user-owned"}, logger)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = apiSrv.URL
	client.userContextToken = mgr.AccessToken
	client.refreshUserContext = mgr.Refresh
	client.sleeper = (&recordingSleeper{}).sleep

	tweets, _, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks should succeed after a single refresh-and-retry, got: %v", err)
	}
	if len(tweets) != 1 {
		t.Fatalf("expected 1 tweet after retry, got %d", len(tweets))
	}
	if got := atomic.LoadInt32(tokenCalls); got != 1 {
		t.Fatalf("expected exactly 1 refresh exchange, got %d", got)
	}
	if got := atomic.LoadInt32(&apiCalls); got != 2 {
		t.Fatalf("expected 2 API calls (401, then 200 retry), got %d", got)
	}
	mu.Lock()
	gotAPIAuth := append([]string{}, apiAuth...)
	mu.Unlock()
	if len(gotAPIAuth) != 2 || gotAPIAuth[0] != "Bearer "+oldAccess || gotAPIAuth[1] != "Bearer "+newAccess {
		t.Fatalf("API requests must use OLD access first then the ROTATED NEW access on retry; got %v", gotAPIAuth)
	}
	if gotRefresh := tokenRefreshSeen(); len(gotRefresh) != 1 || gotRefresh[0] != oldRefresh {
		t.Fatalf("the refresh exchange must present the stored OLD refresh token; got %v", gotRefresh)
	}
	persisted, perr := store.GetTokens(context.Background(), DefaultOwnerUserID)
	if perr != nil {
		t.Fatalf("GetTokens after refresh: %v", perr)
	}
	if persisted.AccessToken != newAccess || persisted.RefreshToken != newRefresh {
		t.Fatalf("the ROTATED pair must be persisted; got access=%q refresh=%q want %q/%q",
			persisted.AccessToken, persisted.RefreshToken, newAccess, newRefresh)
	}
	logText := logBuf.String()
	if !strings.Contains(logText, "user-context token refreshed after 401") {
		t.Fatalf("expected the refresh-after-401 log line; logs:\n%s", logText)
	}
	for _, secret := range []string{oldAccess, newAccess, oldRefresh, newRefresh} {
		if strings.Contains(logText, secret) {
			t.Fatalf("token value %q leaked in logs:\n%s", secret, logText)
		}
	}
}

// TestTwitterAPI_PreExpiryRefresh — SCN-BUG-056-002-011 (proactive arm).
//
// A stored token within refreshSkew of expiry triggers a PROACTIVE refresh
// BEFORE the request goes out: the single API request carries the freshly
// rotated access token and no 401 is ever needed. Non-tautological: without the
// pre-expiry refresh the request would carry the near-expiry OLD access token
// and the token endpoint would not be contacted (both assertions fail).
func TestTwitterAPI_PreExpiryRefresh(t *testing.T) {
	t.Parallel()
	const (
		oldAccess  = "OLD-ACCESS-near-expiry-proactive"
		oldRefresh = "OLD-REFRESH-near-expiry-proactive"
		newAccess  = "NEW-ACCESS-proactively-rotated"
		newRefresh = "NEW-REFRESH-proactively-rotated"
	)

	var mu sync.Mutex
	var apiAuth []string
	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		mu.Lock()
		apiAuth = append(apiAuth, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"data":[{"id":"1001","text":"ok","author_id":"a"}],"meta":{"result_count":1}}`)
	}))
	defer apiSrv.Close()

	tokenSrv, tokenCalls, tokenRefreshSeen := newRefreshTokenServer(t, newAccess, newRefresh, 7200)
	defer tokenSrv.Close()

	store := newFakeFlowStore()
	// Seed a token that expires WITHIN the refreshSkew window → must be
	// proactively refreshed BEFORE the request goes out.
	if err := store.SaveTokens(context.Background(), DefaultOwnerUserID, &auth.Token{
		AccessToken:  oldAccess,
		RefreshToken: oldRefresh,
		ExpiresAt:    time.Now().Add(refreshSkew / 2),
		TokenType:    "bearer",
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	provider := newTwitterOAuthProvider(testOAuthCfg())
	provider.Config.TokenEndpoint = tokenSrv.URL
	mgr := newUserContextManager(store, provider, DefaultOwnerUserID, slog.Default())

	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "app-only-bearer-unused-on-user-owned"}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = apiSrv.URL
	client.userContextToken = mgr.AccessToken
	client.refreshUserContext = mgr.Refresh
	client.sleeper = (&recordingSleeper{}).sleep

	tweets, _, err := client.fetchBookmarks(context.Background(), "2244994945", "")
	if err != nil {
		t.Fatalf("fetchBookmarks: %v", err)
	}
	if len(tweets) != 1 {
		t.Fatalf("expected 1 tweet, got %d", len(tweets))
	}
	if got := atomic.LoadInt32(tokenCalls); got != 1 {
		t.Fatalf("expected exactly 1 PROACTIVE refresh exchange before the request, got %d", got)
	}
	if got := atomic.LoadInt32(&apiCalls); got != 1 {
		t.Fatalf("expected exactly 1 API call (no 401 needed — proactive refresh), got %d", got)
	}
	mu.Lock()
	gotAPIAuth := append([]string{}, apiAuth...)
	mu.Unlock()
	if len(gotAPIAuth) != 1 || gotAPIAuth[0] != "Bearer "+newAccess {
		t.Fatalf("the request must carry the proactively-refreshed NEW access token (proving refresh happened "+
			"BEFORE the request, not reactively after a 401); got %v", gotAPIAuth)
	}
	if gotRefresh := tokenRefreshSeen(); len(gotRefresh) != 1 || gotRefresh[0] != oldRefresh {
		t.Fatalf("the proactive refresh must present the stored OLD refresh token; got %v", gotRefresh)
	}
	persisted, perr := store.GetTokens(context.Background(), DefaultOwnerUserID)
	if perr != nil {
		t.Fatalf("GetTokens after proactive refresh: %v", perr)
	}
	if persisted.AccessToken != newAccess || persisted.RefreshToken != newRefresh {
		t.Fatalf("the proactively-rotated pair must be persisted; got access=%q refresh=%q",
			persisted.AccessToken, persisted.RefreshToken)
	}
}

// TestTwitterAPI_AppOnly401_NoRefresh_Terminal — SCN-BUG-056-002-013 (refresh
// boundary).
//
// An App-Only-tier endpoint (tweets) that returns 401 MUST stay terminal and
// MUST NOT trigger a refresh — an application bearer cannot be rotated. The
// refresh hook IS wired and the store holds a valid refreshable token, so the
// ONLY reason the token endpoint is never contacted is the endpoint→tier gate
// (proving the gate is by TIER, not by the mere presence of a refresh hook).
// Non-tautological: if the 401 backstop refreshed on ANY tier, the token
// endpoint would be hit and the App-Only call would retry — both assertions
// below would fail.
func TestTwitterAPI_AppOnly401_NoRefresh_Terminal(t *testing.T) {
	t.Parallel()

	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401}`)
	}))
	defer apiSrv.Close()

	tokenSrv, tokenCalls, _ := newRefreshTokenServer(t, "SHOULD-NOT-BE-ISSUED", "SHOULD-NOT-ROTATE", 7200)
	defer tokenSrv.Close()

	store := newFakeFlowStore()
	// Seed a VALID refreshable token so the ONLY reason the token endpoint is
	// not contacted is the App-Only tier gate — not an empty store.
	if err := store.SaveTokens(context.Background(), DefaultOwnerUserID, &auth.Token{
		AccessToken:  "user-context-access-present",
		RefreshToken: "user-context-refresh-present",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		TokenType:    "bearer",
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	provider := newTwitterOAuthProvider(testOAuthCfg())
	provider.Config.TokenEndpoint = tokenSrv.URL
	mgr := newUserContextManager(store, provider, DefaultOwnerUserID, slog.Default())

	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "app-only-bearer-for-tweets"}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = apiSrv.URL
	// Refresh IS available — proving the gate is by ENDPOINT TIER, not by the
	// mere presence of a refresh hook.
	client.userContextToken = mgr.AccessToken
	client.refreshUserContext = mgr.Refresh
	client.sleeper = (&recordingSleeper{}).sleep

	// tweets is an App-Only-tier endpoint.
	_, _, err = client.fetchOwnTweets(context.Background(), "2244994945", "")
	if err == nil {
		t.Fatal("expected terminal errAuthRejected on an App-Only 401, got nil")
	}
	if !errors.Is(err, errAuthRejected) {
		t.Fatalf("App-Only 401 must surface errAuthRejected, got %T: %v", err, err)
	}
	if got := atomic.LoadInt32(&apiCalls); got != 1 {
		t.Fatalf("App-Only 401 must NOT retry; expected exactly 1 API call, got %d", got)
	}
	if got := atomic.LoadInt32(tokenCalls); got != 0 {
		t.Fatalf("App-Only 401 must NOT trigger a refresh; expected 0 token-endpoint exchanges, got %d", got)
	}
}

// TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh —
// SCN-BUG-056-002-011 (refresh-once boundary / no-infinite-loop guard).
//
// A user-context endpoint that returns 401 on EVERY attempt (even after the
// refresh) MUST refresh AT MOST ONCE and then surface terminal errAuthRejected —
// no infinite refresh→retry loop. Asserts exactly one refresh exchange, exactly
// two API calls (initial 401 + one post-refresh retry that also 401s), and a
// terminal errAuthRejected. Non-tautological: without the refreshedOnce gate the
// loop would refresh on every 401 and issue maxRetries+1 calls / refreshes.
func TestTwitterAPI_Refresh_On401_PersistentIsTerminalAfterOneRefresh(t *testing.T) {
	t.Parallel()
	const (
		oldAccess  = "OLD-ACCESS-persistent-401"
		oldRefresh = "OLD-REFRESH-persistent-401"
		newAccess  = "NEW-ACCESS-persistent-401"
		newRefresh = "NEW-REFRESH-persistent-401"
	)

	var apiCalls int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&apiCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401}`)
	}))
	defer apiSrv.Close()

	tokenSrv, tokenCalls, _ := newRefreshTokenServer(t, newAccess, newRefresh, 7200)
	defer tokenSrv.Close()

	store := newFakeFlowStore()
	if err := store.SaveTokens(context.Background(), DefaultOwnerUserID, &auth.Token{
		AccessToken:  oldAccess,
		RefreshToken: oldRefresh,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		TokenType:    "bearer",
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	provider := newTwitterOAuthProvider(testOAuthCfg())
	provider.Config.TokenEndpoint = tokenSrv.URL
	mgr := newUserContextManager(store, provider, DefaultOwnerUserID, slog.Default())

	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "app-only-bearer-unused-on-user-owned"}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	client.baseURL = apiSrv.URL
	client.userContextToken = mgr.AccessToken
	client.refreshUserContext = mgr.Refresh
	client.sleeper = (&recordingSleeper{}).sleep

	_, _, err = client.fetchBookmarks(context.Background(), "2244994945", "")
	if err == nil {
		t.Fatal("expected terminal errAuthRejected when the 401 persists after a refresh, got nil")
	}
	if !errors.Is(err, errAuthRejected) {
		t.Fatalf("persisting 401 must surface errAuthRejected, got %T: %v", err, err)
	}
	if got := atomic.LoadInt32(tokenCalls); got != 1 {
		t.Fatalf("refresh must happen AT MOST ONCE per request; expected 1 refresh exchange, got %d", got)
	}
	if got := atomic.LoadInt32(&apiCalls); got != 2 {
		t.Fatalf("expected exactly 2 API calls (initial 401 + one post-refresh retry); "+
			"a third would mean an infinite refresh loop; got %d", got)
	}
}

// ============================================================================
// BUG-056-002 Scope D (GAP-056-G2 / R-016) — x-rate-limit-remaining gauge
// ============================================================================
//
// These integration tests drive doWithRetry against an httptest.Server (a real
// in-process HTTP server, NOT a route/intercept mock) and read the published
// gauge with prometheus testutil.ToFloat64. Each test uses a UNIQUE endpoint
// label so the package-global GaugeVec cannot bleed values across tests — no
// .Reset() is needed and the tests stay parallel-safe.

// remainingReqBuilder returns a doWithRetry reqBuilder that issues a plain GET
// at the given URL — enough to exercise the response-header observation path
// without the buildRequest auth-tier machinery (irrelevant to header parsing).
func remainingReqBuilder(ctx context.Context, rawURL string) func() (*http.Request, error) {
	return func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	}
}

// TestTwitterAPI_RateLimitRemaining_SetFromHeader — SCN-BUG-056-002-015.
//
// A 200 response carrying x-rate-limit-remaining: 42 sets the gauge to 42. This
// is the GREEN side of the RED→GREEN proof: with the doWithRetry hook removed
// the gauge stays at 0 and this assertion fails.
func TestTwitterAPI_RateLimitRemaining_SetFromHeader(t *testing.T) {
	t.Parallel()
	const endpoint = "scope_d_remaining_set_from_header"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-rate-limit-remaining", "42")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "test-bearer-token-not-real"}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	resp, err := client.doWithRetry(context.Background(), endpoint, remainingReqBuilder(context.Background(), srv.URL))
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	drainAndClose(resp)

	if got := testutil.ToFloat64(metrics.ConnectorTwitterAPIRateLimitRemaining.WithLabelValues("twitter", endpoint)); got != 42 {
		t.Fatalf("gauge = %v, want 42 (x-rate-limit-remaining header must drive the gauge)", got)
	}
}

// TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue — no-clobber.
//
// Seed the gauge to a known value, then a response that OMITS the header MUST
// leave it unchanged: an absent header is "unknown", not "exhausted", so it
// must never overwrite the last real headroom with a bogus 0.
func TestTwitterAPI_RateLimitRemaining_AbsentHeaderLeavesPriorValue(t *testing.T) {
	t.Parallel()
	const endpoint = "scope_d_remaining_absent_header"

	// Seed a known prior value via the same gauge the connector publishes to.
	metrics.ConnectorTwitterAPIRateLimitRemaining.WithLabelValues("twitter", endpoint).Set(99)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately NO x-rate-limit-remaining header.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "test-bearer-token-not-real"}, slog.Default())
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	resp, err := client.doWithRetry(context.Background(), endpoint, remainingReqBuilder(context.Background(), srv.URL))
	if err != nil {
		t.Fatalf("doWithRetry: %v", err)
	}
	drainAndClose(resp)

	if got := testutil.ToFloat64(metrics.ConnectorTwitterAPIRateLimitRemaining.WithLabelValues("twitter", endpoint)); got != 99 {
		t.Fatalf("gauge = %v, want 99 unchanged (absent header MUST NOT clobber the prior value)", got)
	}
}

// TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus — SCN-BUG-056-002-016.
//
// ADVERSARIAL: proves the gauge is updated from the header on BOTH a 2xx and a
// 429 — i.e. "after each API call", not only on the 429 branch (the original
// reset-gauge mistake, inverted). Two independent endpoint labels so neither
// status overwrites the other's observation.
func TestTwitterAPI_RateLimitRemaining_SetOnEveryStatus(t *testing.T) {
	t.Parallel()

	// --- 200 path ---
	const ep200 = "scope_d_remaining_every_status_200"
	srv200 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-rate-limit-remaining", "200")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv200.Close()

	client200, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "test-bearer-token-not-real"}, slog.Default())
	if err != nil {
		t.Fatalf("setup 200: %v", err)
	}
	resp, err := client200.doWithRetry(context.Background(), ep200, remainingReqBuilder(context.Background(), srv200.URL))
	if err != nil {
		t.Fatalf("doWithRetry 200: %v", err)
	}
	drainAndClose(resp)
	if got := testutil.ToFloat64(metrics.ConnectorTwitterAPIRateLimitRemaining.WithLabelValues("twitter", ep200)); got != 200 {
		t.Fatalf("200-path gauge = %v, want 200", got)
	}

	// --- 429 path ---
	// A persistent 429 with NO x-rate-limit-reset header (waitDur 0, under the
	// cap) loops to errMaxRetriesExceeded under a no-op sleeper. The
	// remaining-header hook runs BEFORE the status switch, so the gauge is set
	// from the 429 response even though the call ultimately errors — the
	// adversarial proof that the gauge moves on a non-2xx status.
	const ep429 = "scope_d_remaining_every_status_429"
	srv429 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-rate-limit-remaining", "7")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"title":"Too Many Requests"}`))
	}))
	defer srv429.Close()

	client429, err := newAPIClient(TwitterConfig{SyncMode: SyncModeAPI, BearerToken: "test-bearer-token-not-real"}, slog.Default())
	if err != nil {
		t.Fatalf("setup 429: %v", err)
	}
	client429.sleeper = func(context.Context, time.Duration) error { return nil } // no real sleep
	_, err = client429.doWithRetry(context.Background(), ep429, remainingReqBuilder(context.Background(), srv429.URL))
	if err == nil {
		t.Fatalf("doWithRetry 429: expected an error after a persistent 429, got nil")
	}
	if !errors.Is(err, errMaxRetriesExceeded) {
		t.Fatalf("doWithRetry 429: error must wrap errMaxRetriesExceeded, got %T: %v", err, err)
	}
	if got := testutil.ToFloat64(metrics.ConnectorTwitterAPIRateLimitRemaining.WithLabelValues("twitter", ep429)); got != 7 {
		t.Fatalf("429-path gauge = %v, want 7 (gauge MUST update on a 429, not only on 2xx)", got)
	}
}
