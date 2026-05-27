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
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
