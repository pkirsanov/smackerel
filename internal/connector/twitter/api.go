// Package twitter — Twitter API v2 client foundation (spec 056 scope 01).
//
// This file owns the HTTP plumbing for the SyncModeAPI and SyncModeHybrid sync
// modes. It deliberately covers ONLY the foundational request/response shape:
//   - an apiClient struct with explicit timeout, unexported bearer token,
//     base URL, slog logger
//   - a constructor that fails loud when the bearer token is empty in a mode
//     that requires API access (per spec 056 R-004)
//   - a request builder that always attaches Authorization + User-Agent and
//     refuses any HTTP method other than GET (per spec 056 SCN-056-009)
//
// Pagination, rate-limit handling, hybrid dispatch, and live-gated tests land
// in scopes 02, 03, 04, and 05 respectively.
package twitter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiBaseURL is the Twitter API v2 base URL. Endpoint paths are appended.
const apiBaseURL = "https://api.twitter.com/2"

// apiClientTimeout is the per-request HTTP timeout. Conservative default;
// individual long-poll endpoints (none used here) would override.
const apiClientTimeout = 30 * time.Second

// userAgentValue identifies the connector to the Twitter API for support /
// abuse reporting. Includes the spec ID for traceability.
const userAgentValue = "smackerel-twitter-connector/1.0 (+spec-056)"

// ErrAPIBearerTokenRequired is returned by newAPIClient when the configured
// sync mode requires the API but the bearer token is empty. Per spec 056 R-004
// and the smackerel-no-defaults policy: fail loud at startup, never fall back.
var ErrAPIBearerTokenRequired = errors.New(
	"twitter connector: bearer_token is required for sync_mode=api or sync_mode=hybrid; " +
		"set connectors.twitter.bearer_token in config/smackerel.yaml")

// ErrAPIMethodNotAllowed is returned by the request builder for any HTTP method
// other than GET. The Twitter v2 read endpoints we consume are GET-only; any
// non-GET attempt is a programming error worth surfacing immediately.
var ErrAPIMethodNotAllowed = errors.New(
	"twitter api client: only GET requests are allowed by this client")

// apiClient is the package-private HTTP client for Twitter API v2 calls.
// The bearer token field is unexported and never read by anything outside this
// file. Log records constructed by methods on this struct MUST NOT include the
// token; the spec 056 scope 03 log-scan assertion enforces this contract.
type apiClient struct {
	httpClient  *http.Client
	bearerToken string //nolint:unused // consumed by buildRequest below
	baseURL     string
	userAgent   string
	logger      *slog.Logger
}

// newAPIClient constructs the API client for a given Twitter connector config.
// Returns (ErrAPIBearerTokenRequired) when the mode requires API access AND the
// bearer token is empty. Returns (nil, nil) when the mode does not require API
// access (i.e. SyncModeArchive) — callers MUST nil-check the returned client
// before invoking any method.
//
// The logger argument is required; passing nil panics rather than silently
// constructing a no-op logger. This mirrors the smackerel-no-defaults policy.
func newAPIClient(cfg TwitterConfig, logger *slog.Logger) (*apiClient, error) {
	if logger == nil {
		panic("twitter.newAPIClient: logger is required (nil passed)")
	}
	if cfg.SyncMode == SyncModeArchive {
		// API client is not needed in archive-only mode. Callers MUST NOT use
		// the returned nil client; this is enforced by the dispatcher landing
		// in scope 04.
		return nil, nil
	}
	if cfg.SyncMode != SyncModeAPI && cfg.SyncMode != SyncModeHybrid {
		return nil, fmt.Errorf("twitter api client: unsupported sync_mode %q", string(cfg.SyncMode))
	}
	if cfg.BearerToken == "" {
		return nil, ErrAPIBearerTokenRequired
	}
	return &apiClient{
		httpClient:  &http.Client{Timeout: apiClientTimeout},
		bearerToken: cfg.BearerToken,
		baseURL:     apiBaseURL,
		userAgent:   userAgentValue,
		logger:      logger.With(slog.String("component", "twitter.api")),
	}, nil
}

// buildRequest constructs an authenticated HTTP request for the given API
// path. Path is appended to baseURL; if path is absolute (starts with "http"),
// it is used directly so tests can point at httptest.Server. Returns
// ErrAPIMethodNotAllowed for any method other than GET.
//
// Always sets:
//   - Authorization: Bearer <token>
//   - User-Agent: smackerel-twitter-connector/...
//   - Accept: application/json
//
// The token is read from the unexported field; callers MUST NOT pass the token
// in path or query, and MUST NOT log the returned request without first
// scrubbing the Authorization header.
func (c *apiClient) buildRequest(ctx context.Context, method, path string, query url.Values) (*http.Request, error) {
	if method != http.MethodGet {
		return nil, fmt.Errorf("%w: method=%q path=%q", ErrAPIMethodNotAllowed, method, path)
	}
	if c == nil {
		return nil, errors.New("twitter api client: buildRequest called on nil client")
	}

	fullURL := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		fullURL = c.baseURL + path
	}
	if len(query) > 0 {
		if strings.Contains(fullURL, "?") {
			fullURL = fullURL + "&" + query.Encode()
		} else {
			fullURL = fullURL + "?" + query.Encode()
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter api client: build request for %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// usersMeResponse is the minimal shape of GET /2/users/me. Wider response
// fields land in later scopes as needed.
type usersMeResponse struct {
	Data struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"data"`
}

// fetchUsersMe issues GET /2/users/me and returns the authenticated user's ID
// and username. Used by scope 02 as the seed for per-user endpoint paths
// (/2/users/:id/bookmarks, /2/users/:id/liked_tweets, /2/users/:id/tweets,
// /2/users/:id/mentions).
//
// Scope 01 ships this method as the smallest end-to-end exercise of the
// request builder + JSON decoding path. Scope 03 will wrap it with rate-limit
// + retry semantics; until then, the caller MUST NOT rely on it under load.
func (c *apiClient) fetchUsersMe(ctx context.Context) (*usersMeResponse, error) {
	if c == nil {
		return nil, errors.New("twitter api client: fetchUsersMe called on nil client")
	}
	req, err := c.buildRequest(ctx, http.MethodGet, "/users/me", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter api client: GET /users/me: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Scope 03 will replace this with structured rate-limit / fast-fail
		// handling; the foundation just surfaces the raw status with the body.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("twitter api client: GET /users/me: status=%d body=%s",
			resp.StatusCode, string(body))
	}
	var out usersMeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("twitter api client: decode /users/me: %w", err)
	}
	return &out, nil
}
