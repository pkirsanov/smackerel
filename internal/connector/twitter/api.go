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
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/metrics"
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
	logger      *slog.Logger // sleeper / now are injected for tests. Nil values use the production
	// defaults (defaultSleeper, time.Now).
	sleeper sleeperFunc
	now     nowFunc
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
	resp, err := c.doWithRetry(ctx, "users_me", func() (*http.Request, error) {
		return c.buildRequest(ctx, http.MethodGet, "/users/me", nil)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out usersMeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("twitter api client: decode /users/me: %w", err)
	}
	return &out, nil
}

// ============================================================================

// apiEndpoint enumerates the four per-user endpoints we paginate. Values are
// also used as cursor-map keys, so changing them is a forward-incompatible
// schema change to persisted state.
type apiEndpoint string

const (
	endpointBookmarks apiEndpoint = "bookmarks"
	endpointLikes     apiEndpoint = "liked_tweets"
	endpointOwnTweets apiEndpoint = "tweets"
	endpointMentions  apiEndpoint = "mentions"
)

// apiTweet is the minimal v2 tweet shape we persist. Additional fields
// (entities, attachments, public_metrics) land in later scopes as needed.
type apiTweet struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	AuthorID string `json:"author_id,omitempty"`
}

// tweetsResponse mirrors the documented v2 envelope for the four endpoints.
// `meta.next_token` is absent on the final page, signalling pagination
// termination.
type tweetsResponse struct {
	Data []apiTweet `json:"data"`
	Meta struct {
		ResultCount int    `json:"result_count"`
		NextToken   string `json:"next_token,omitempty"`
	} `json:"meta"`
}

// apiCursor is the JSON-encoded per-endpoint pagination state we marshal into
// the single cursor string the connector framework persists per source. The
// connector framework treats cursors opaquely; we re-serialize on every Sync.
//
// Forward-compatibility: unknown JSON keys are preserved by golang/json's
// default behavior of dropping them on Unmarshal, so adding new endpoints in
// later scopes is safe.
type apiCursor struct {
	// PerEndpoint maps endpoint name to the next_token returned by the last
	// non-empty page. An empty string means "start from the beginning"; an
	// absent map entry is equivalent to an empty string.
	PerEndpoint map[apiEndpoint]string `json:"per_endpoint,omitempty"`
}

// loadCursor parses the connector framework's opaque cursor string into our
// per-endpoint map. An empty or whitespace-only cursor returns a zero-value
// cursor (i.e. start from the beginning for every endpoint), which is the
// correct first-run behavior.
//
// loadCursor never returns a partial cursor on parse failure; it returns the
// zero value plus the error, so the caller chooses between fail-loud (abort
// the sync) and fail-fresh (restart pagination). For spec 056 the dispatcher
// (scope 04) will choose fail-loud to avoid silently re-ingesting tweets.
func loadCursor(raw string) (apiCursor, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return apiCursor{PerEndpoint: map[apiEndpoint]string{}}, nil
	}
	var c apiCursor
	if err := json.Unmarshal([]byte(trimmed), &c); err != nil {
		return apiCursor{PerEndpoint: map[apiEndpoint]string{}},
			fmt.Errorf("twitter api client: parse cursor: %w", err)
	}
	if c.PerEndpoint == nil {
		c.PerEndpoint = map[apiEndpoint]string{}
	}
	return c, nil
}

// saveCursor serializes the per-endpoint cursor map back to the opaque string
// the connector framework persists. Returns the canonical JSON representation
// (no surrounding whitespace).
func saveCursor(c apiCursor) (string, error) {
	if c.PerEndpoint == nil {
		c.PerEndpoint = map[apiEndpoint]string{}
	}
	buf, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("twitter api client: serialize cursor: %w", err)
	}
	return string(buf), nil
}

// maxPagesPerEndpoint bounds the pagination loop per endpoint per sync tick.
// Prevents an unbounded loop if the API ever returns a non-terminating
// next_token chain. Set to 100 to give 10,000 tweets per endpoint per tick
// at the default 100-results-per-request limit, which exceeds any realistic
// hourly delta.
const maxPagesPerEndpoint = 100

// fetchEndpointPaginated drives the pagination loop for a single per-user
// endpoint. Starts from the provided pagination_token (empty = first page),
// follows next_token across pages, terminates when next_token is absent OR
// data is empty OR maxPagesPerEndpoint is hit (logs a warning when the bound
// is hit).
//
// Returns the union of all returned tweets across pages, plus the last
// non-empty page's next_token (or empty string if the last page had no
// next_token). The caller persists the returned cursor.
//
// All requests carry the authenticated GET request built by buildRequest.
// All response bodies are read under a 1 MiB limit and closed via defer.
func (c *apiClient) fetchEndpointPaginated(ctx context.Context, endpoint apiEndpoint, userID, startToken string) ([]apiTweet, string, error) {
	if c == nil {
		return nil, "", errors.New("twitter api client: fetchEndpointPaginated called on nil client")
	}
	if userID == "" {
		return nil, "", errors.New("twitter api client: userID is required for paginated endpoints")
	}
	path := endpointPath(endpoint, userID)
	if path == "" {
		return nil, "", fmt.Errorf("twitter api client: unknown endpoint %q", string(endpoint))
	}

	tweets := []apiTweet{}
	cursor := startToken
	lastNonEmptyToken := ""
	for page := 0; page < maxPagesPerEndpoint; page++ {
		query := url.Values{}
		if cursor != "" {
			query.Set("pagination_token", cursor)
		}
		resp, err := c.doWithRetry(ctx, string(endpoint), func() (*http.Request, error) {
			return c.buildRequest(ctx, http.MethodGet, path, query)
		})
		if err != nil {
			return tweets, lastNonEmptyToken, fmt.Errorf("twitter api client: %s page %d: %w", endpoint, page, err)
		}
		body, decErr := decodeTweetsResponse(resp)
		// decodeTweetsResponse closes resp.Body.
		if decErr != nil {
			return tweets, lastNonEmptyToken, fmt.Errorf("twitter api client: %s page %d: %w", endpoint, page, decErr)
		}
		tweets = append(tweets, body.Data...)
		if body.Meta.NextToken == "" {
			return tweets, lastNonEmptyToken, nil
		}
		lastNonEmptyToken = body.Meta.NextToken
		cursor = body.Meta.NextToken
	}
	c.logger.Warn("pagination cap hit",
		slog.String("endpoint", string(endpoint)),
		slog.Int("pages", maxPagesPerEndpoint),
		slog.Int("tweets_so_far", len(tweets)),
	)
	return tweets, lastNonEmptyToken, nil
}

// decodeTweetsResponse handles status check, body-size limit, JSON decode,
// and body close for a single paginated request. Bounded body size protects
// against memory exhaustion from a misbehaving server (CWE-770).
func decodeTweetsResponse(resp *http.Response) (*tweetsResponse, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Scope 03 will replace this with structured rate-limit / fast-fail
		// handling; for scope 02 we surface the status as a plain error and
		// expose the body excerpt for operator debugging.
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(excerpt))
	}
	var out tweetsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &out, nil
}

// endpointPath returns the Twitter v2 path for a given per-user endpoint, or
// the empty string for an unknown endpoint.
func endpointPath(endpoint apiEndpoint, userID string) string {
	switch endpoint {
	case endpointBookmarks:
		return "/users/" + userID + "/bookmarks"
	case endpointLikes:
		return "/users/" + userID + "/liked_tweets"
	case endpointOwnTweets:
		return "/users/" + userID + "/tweets"
	case endpointMentions:
		return "/users/" + userID + "/mentions"
	default:
		return ""
	}
}

// fetchBookmarks, fetchLikes, fetchOwnTweets, fetchMentions are typed
// per-endpoint convenience wrappers. They all delegate to
// fetchEndpointPaginated to keep behavior identical across endpoints; the
// per-method shape matches the spec 056 scope 02 implementation plan.
func (c *apiClient) fetchBookmarks(ctx context.Context, userID, startToken string) ([]apiTweet, string, error) {
	return c.fetchEndpointPaginated(ctx, endpointBookmarks, userID, startToken)
}
func (c *apiClient) fetchLikes(ctx context.Context, userID, startToken string) ([]apiTweet, string, error) {
	return c.fetchEndpointPaginated(ctx, endpointLikes, userID, startToken)
}
func (c *apiClient) fetchOwnTweets(ctx context.Context, userID, startToken string) ([]apiTweet, string, error) {
	return c.fetchEndpointPaginated(ctx, endpointOwnTweets, userID, startToken)
}
func (c *apiClient) fetchMentions(ctx context.Context, userID, startToken string) ([]apiTweet, string, error) {
	return c.fetchEndpointPaginated(ctx, endpointMentions, userID, startToken)
}

// ============================================================================
// Spec 056 Scope 03 — Rate-Limit & Error Handling
// ============================================================================

// maxRetries bounds the number of automatic retries per HTTP request across
// all retryable failure classes (rate-limit, 5xx, transport). After this many
// retries, the last error is returned to the caller.
const maxRetries = 3

// rateLimitMaxWait bounds the maximum honored x-rate-limit-reset wait. If a
// server returns a reset header farther in the future than this, the request
// fails rather than blocking the sync round indefinitely. The bound is a
// defensive cap; under normal Twitter API operation reset windows are seconds
// to minutes, never hours.
const rateLimitMaxWait = 30 * time.Minute

// errAuthRejected and errMaxRetriesExceeded are sentinel errors returned by
// doWithRetry. Tests use errors.Is to assert intent.
var (
	errAuthRejected       = errors.New("twitter api client: authentication rejected (401/403); no retry")
	errMaxRetriesExceeded = errors.New("twitter api client: max retries exceeded")
)

// sleeperFunc is a context-aware sleep abstraction so tests can verify the
// wait duration without paying the wall clock cost. Production uses the real
// time package; tests substitute a recorder that captures requested durations
// and returns immediately.
type sleeperFunc func(ctx context.Context, d time.Duration) error

// defaultSleeper is the production implementation: time.NewTimer with context
// cancellation. Returns context error if the deadline fires before the timer.
func defaultSleeper(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// nowFunc abstracts time.Now() so tests can pin the clock without exposing a
// full clock interface. Production uses time.Now.
type nowFunc func() time.Time

// doWithRetry wraps c.httpClient.Do with rate-limit, 5xx-backoff, and
// 401/403-fast-fail semantics. The endpoint parameter is the apiEndpoint label
// used by metrics; pass an empty string for non-paginated calls (e.g.
// /users/me) and the metric will use "users_me".
//
// The reqBuilder closure is invoked once per attempt so the caller can issue a
// fresh request (with a fresh body / fresh context) per retry. This is the
// pattern recommended by net/http for retry — re-using a single *http.Request
// across attempts is unsafe.
//
// Behavior matrix:
//   - 2xx: return resp, no retry
//   - 4xx (401, 403): return errAuthRejected wrapped with status, no retry
//   - 4xx (other than 401/403/429): return error with status, no retry
//   - 429: parse x-rate-limit-reset, sleep until that time (capped at
//     rateLimitMaxWait), retry. Increments TwitterAPIRetries{reason=rate_limit}.
//     Sets TwitterAPIRateLimitReset gauge to observed seconds-until-reset.
//   - 5xx: exponential backoff (1s, 2s, 4s), retry. Increments
//     TwitterAPIRetries{reason=server_error}.
//   - transport error: exponential backoff, retry. Increments
//     TwitterAPIRetries{reason=transport}.
//   - max retries exhausted: return errMaxRetriesExceeded wrapped with last err.
//
// On every attempt, the response body is either returned to the caller (on
// success) or fully drained-and-closed (on retry decision) so the underlying
// connection can be reused. Callers MUST close the body of any returned
// successful response.
//
// Anti-fabrication: this function never logs the Authorization header. The
// only request data it logs is method, path, and status. Tests verify with
// the bearer-token-never-in-logs adversarial assertion (TestTwitterAPI_BearerTokenNeverAppearsInLogs).
func (c *apiClient) doWithRetry(ctx context.Context, endpoint string, reqBuilder func() (*http.Request, error)) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("twitter api client: doWithRetry called on nil client")
	}
	if endpoint == "" {
		endpoint = "users_me"
	}
	sleeper := c.sleeper
	if sleeper == nil {
		sleeper = defaultSleeper
	}
	now := c.now
	if now == nil {
		now = time.Now
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := reqBuilder()
		if err != nil {
			return nil, fmt.Errorf("twitter api client: build request: %w", err)
		}
		resp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			c.observeRequest(endpoint, "error")
			c.observeRetry(endpoint, "transport")
			c.logger.Warn("transport error",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", attempt),
				slog.String("err", doErr.Error()),
			)
			lastErr = doErr
			if attempt == maxRetries {
				return nil, fmt.Errorf("%w: %v", errMaxRetriesExceeded, doErr)
			}
			if err := sleeper(ctx, backoffDuration(attempt)); err != nil {
				return nil, err
			}
			continue
		}

		statusLabel := strconv.Itoa(resp.StatusCode)
		c.observeRequest(endpoint, statusLabel)

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return resp, nil
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			drainAndClose(resp)
			c.logger.Warn("authentication rejected",
				slog.String("endpoint", endpoint),
				slog.Int("status", resp.StatusCode),
			)
			return nil, fmt.Errorf("%w: status=%d", errAuthRejected, resp.StatusCode)
		case resp.StatusCode == http.StatusTooManyRequests:
			waitDur := parseRateLimitReset(resp.Header.Get("x-rate-limit-reset"), now())
			drainAndClose(resp)
			c.observeRetry(endpoint, "rate_limit")
			c.observeRateLimitReset(endpoint, waitDur)
			if waitDur > rateLimitMaxWait {
				return nil, fmt.Errorf("twitter api client: %s: rate-limit reset %s exceeds cap %s; aborting",
					endpoint, waitDur, rateLimitMaxWait)
			}
			c.logger.Warn("rate limit hit; sleeping until reset",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", attempt),
				slog.Duration("wait", waitDur),
			)
			if attempt == maxRetries {
				return nil, fmt.Errorf("%w: rate limit persisted", errMaxRetriesExceeded)
			}
			if err := sleeper(ctx, waitDur); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode >= 500:
			drainAndClose(resp)
			c.observeRetry(endpoint, "server_error")
			backoff := backoffDuration(attempt)
			c.logger.Warn("server error; backing off",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", attempt),
				slog.Int("status", resp.StatusCode),
				slog.Duration("backoff", backoff),
			)
			lastErr = fmt.Errorf("twitter api client: %s: server error status=%d", endpoint, resp.StatusCode)
			if attempt == maxRetries {
				return nil, fmt.Errorf("%w: %v", errMaxRetriesExceeded, lastErr)
			}
			if err := sleeper(ctx, backoff); err != nil {
				return nil, err
			}
			continue
		default:
			// Other 4xx: not retryable, surface to caller without exposing body.
			excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			drainAndClose(resp)
			return nil, fmt.Errorf("twitter api client: %s: non-retryable status=%d body=%s",
				endpoint, resp.StatusCode, string(excerpt))
		}
	}
	return nil, fmt.Errorf("%w: %v", errMaxRetriesExceeded, lastErr)
}

// drainAndClose reads and discards any remaining body bytes so the underlying
// connection can be reused, then closes the body. Bounded by 4 KiB to avoid
// blocking on a server that keeps streaming bytes.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

// parseRateLimitReset converts an x-rate-limit-reset header value (Unix epoch
// seconds per Twitter API v2 docs) into a duration relative to now. Returns
// zero for unparseable or past values (defensive: if the header is malformed
// we don't sleep at all rather than sleeping forever).
func parseRateLimitReset(headerVal string, now time.Time) time.Duration {
	if headerVal == "" {
		return 0
	}
	epoch, err := strconv.ParseInt(strings.TrimSpace(headerVal), 10, 64)
	if err != nil {
		return 0
	}
	reset := time.Unix(epoch, 0)
	if reset.Before(now) {
		return 0
	}
	return reset.Sub(now)
}

// backoffDuration returns the exponential backoff interval for the given
// retry attempt number (0-indexed). 1s, 2s, 4s, ... capped at 30s.
func backoffDuration(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := time.Duration(1<<attempt) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// observeRequest, observeRetry, observeRateLimitReset increment the spec 056
// Prometheus metrics. They use the connector ID label "twitter" (matching the
// existing connector.Connector ID convention). All three are no-ops when
// metrics registration is bypassed (e.g. unit tests run without the init()
// pulling in prometheus side effects — which is the default Go behavior).
func (c *apiClient) observeRequest(endpoint, status string) {
	metrics.ConnectorTwitterAPIRequests.WithLabelValues("twitter", endpoint, status).Inc()
}

func (c *apiClient) observeRetry(endpoint, reason string) {
	metrics.ConnectorTwitterAPIRetries.WithLabelValues("twitter", endpoint, reason).Inc()
}

func (c *apiClient) observeRateLimitReset(endpoint string, wait time.Duration) {
	metrics.ConnectorTwitterAPIRateLimitReset.WithLabelValues("twitter", endpoint).Set(wait.Seconds())
}
