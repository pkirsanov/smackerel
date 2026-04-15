package hospitable

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// maxPaginationPages is the upper bound on pagination requests to prevent
// infinite loops from a misbehaving or malicious API server.
const maxPaginationPages = 1000

// maxRetryAfterCap is the upper bound on Retry-After delays accepted from the
// server. A malicious or misconfigured server cannot force arbitrarily long
// sleeps beyond this cap.
const maxRetryAfterCap = 60 * time.Second

// Client wraps the Hospitable Public API.
type Client struct {
	baseURL    string
	baseOrigin string // scheme+host for same-origin pagination checks
	token      string
	httpClient *http.Client
	backoff    *connector.Backoff
	pageSize   int
}

// NewClient creates a new Hospitable API client.
func NewClient(baseURL, token string, pageSize int) *Client {
	if pageSize <= 0 {
		pageSize = 100
	}
	// Extract scheme+host for same-origin pagination validation (SSRF defence).
	origin := ""
	if parsed, err := url.Parse(baseURL); err == nil {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	return &Client{
		baseURL:    baseURL,
		baseOrigin: origin,
		token:      token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		backoff: &connector.Backoff{
			BaseDelay:  1 * time.Second,
			MaxDelay:   16 * time.Second,
			MaxRetries: 3, // R-009: max 3 retries per request
		},
		pageSize: pageSize,
	}
}

// SetBackoff replaces the default backoff policy (used for testing).
func (c *Client) SetBackoff(b *connector.Backoff) {
	c.backoff = b
}

// Validate tests the API token by fetching properties with a small page size.
func (c *Client) Validate(ctx context.Context) error {
	_, err := c.doGet(ctx, "/properties", url.Values{"per_page": {"1"}})
	if err != nil {
		return fmt.Errorf("validate token: %w", err)
	}
	return nil
}

// ListProperties fetches all properties, optionally filtered by updated_since.
func (c *Client) ListProperties(ctx context.Context, since time.Time) ([]Property, error) {
	params := url.Values{"per_page": {strconv.Itoa(c.pageSize)}}
	if !since.IsZero() {
		params.Set("updated_since", since.Format(time.RFC3339))
	}
	return fetchPaginated[Property](c, ctx, "/properties", params)
}

// ListReservations fetches reservations, optionally filtered by updated_since.
func (c *Client) ListReservations(ctx context.Context, since time.Time) ([]Reservation, error) {
	params := url.Values{"per_page": {strconv.Itoa(c.pageSize)}}
	if !since.IsZero() {
		params.Set("updated_since", since.Format(time.RFC3339))
	}
	return fetchPaginated[Reservation](c, ctx, "/reservations", params)
}

// ListMessages fetches messages for a specific reservation, optionally filtered by since.
func (c *Client) ListMessages(ctx context.Context, reservationID string, since time.Time) ([]Message, error) {
	params := url.Values{"per_page": {strconv.Itoa(c.pageSize)}}
	if !since.IsZero() {
		params.Set("since", since.Format(time.RFC3339))
	}
	path := fmt.Sprintf("/reservations/%s/messages", url.PathEscape(reservationID))
	return fetchPaginated[Message](c, ctx, path, params)
}

// ListActiveReservations fetches reservations with check-out on or after the cutoff date,
// independent of the incremental updated_since cursor (R-016).
func (c *Client) ListActiveReservations(ctx context.Context, cutoff time.Time) ([]Reservation, error) {
	params := url.Values{"per_page": {strconv.Itoa(c.pageSize)}}
	params.Set("checkout_after", cutoff.Format("2006-01-02"))
	return fetchPaginated[Reservation](c, ctx, "/reservations", params)
}

// ListReviews fetches reviews, optionally filtered by updated_since.
func (c *Client) ListReviews(ctx context.Context, since time.Time) ([]Review, error) {
	params := url.Values{"per_page": {strconv.Itoa(c.pageSize)}}
	if !since.IsZero() {
		params.Set("updated_since", since.Format(time.RFC3339))
	}
	return fetchPaginated[Review](c, ctx, "/reviews", params)
}

// fetchPaginated is a generic helper that follows pagination links.
// It enforces same-origin validation on pagination URLs (SSRF defence)
// and caps the number of pages to prevent infinite loops.
func fetchPaginated[T any](c *Client, ctx context.Context, path string, params url.Values) ([]T, error) {
	var all []T

	currentURL := c.baseURL + path + "?" + params.Encode()

	for page := 0; currentURL != "" && page < maxPaginationPages; page++ {
		// IMP-012-001: Short-circuit pagination on context cancellation to avoid
		// unnecessary request setup/teardown for remaining pages.
		if err := ctx.Err(); err != nil {
			return all, err
		}
		body, nextURL, err := c.doGetPaginated(ctx, currentURL)
		if err != nil {
			return all, err
		}

		var resp PaginatedResponse[T]
		if err := json.Unmarshal(body, &resp); err != nil {
			return all, fmt.Errorf("decode response: %w", err)
		}

		all = append(all, resp.Data...)

		// IMP-012-SQS-003: Break early when the server returns an empty data page
		// to avoid chasing pagination links that yield no results. This prevents
		// wasted network calls against servers that return empty pages with a
		// "next" link indefinitely.
		if len(resp.Data) == 0 {
			break
		}

		candidateURL := resp.NextURL
		if candidateURL == "" {
			candidateURL = nextURL
		}

		// Validate pagination URL stays on the same origin to prevent SSRF.
		if candidateURL != "" && !c.isSameOrigin(candidateURL) {
			slog.Warn("hospitable: pagination URL rejected (different origin)",
				"url", candidateURL, "expected_origin", c.baseOrigin)
			break
		}
		currentURL = candidateURL
	}

	return all, nil
}

// isSameOrigin checks that a URL shares the same scheme+host as the client's base URL.
func (c *Client) isSameOrigin(rawURL string) bool {
	if c.baseOrigin == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	candidate := parsed.Scheme + "://" + parsed.Host
	return strings.EqualFold(candidate, c.baseOrigin)
}

// doGet makes an authenticated GET request and returns the response body.
func (c *Client) doGet(ctx context.Context, path string, params url.Values) ([]byte, error) {
	fullURL := c.baseURL + path
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}
	body, _, err := c.doGetPaginated(ctx, fullURL)
	return body, err
}

// doGetPaginated makes an authenticated GET request, returns body and next link.
// Each call creates its own backoff state to be safe for concurrent use.
func (c *Client) doGetPaginated(ctx context.Context, rawURL string) ([]byte, string, error) {
	backoff := &connector.Backoff{
		BaseDelay:  c.backoff.BaseDelay,
		MaxDelay:   c.backoff.MaxDelay,
		MaxRetries: c.backoff.MaxRetries,
	}

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("http request: %w", err)
		}

		// Limit response body to 10 MiB to prevent OOM from malicious/compromised API.
		const maxResponseSize = 10 << 20
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
		resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("read response: %w", err)
		}
		if int64(len(body)) > maxResponseSize {
			return nil, "", fmt.Errorf("response body exceeds %d bytes limit", maxResponseSize)
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			// Extract Link header for pagination if present
			nextURL := parseLinkNext(resp.Header.Get("Link"))
			return body, nextURL, nil

		case resp.StatusCode == http.StatusUnauthorized:
			return nil, "", fmt.Errorf("unauthorized: invalid or expired access token")

		case resp.StatusCode == http.StatusForbidden:
			return nil, "", fmt.Errorf("forbidden: insufficient permissions")

		case resp.StatusCode == http.StatusTooManyRequests:
			delay, ok := backoff.Next()
			if !ok {
				return nil, "", fmt.Errorf("rate limited: max retries exceeded")
			}
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			// Cap Retry-After to prevent malicious servers from forcing
			// arbitrarily long sleeps.
			if retryAfter > maxRetryAfterCap {
				retryAfter = maxRetryAfterCap
			}
			if retryAfter > delay {
				delay = retryAfter
			}
			slog.Info("hospitable: rate limited, backing off",
				"delay", delay, "retry_after", retryAfter, "attempt", backoff.Attempt())
			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			case <-time.After(delay):
				continue
			}

		case resp.StatusCode >= 500:
			delay, ok := backoff.Next()
			if !ok {
				return nil, "", fmt.Errorf("server error %d: max retries exceeded", resp.StatusCode)
			}
			slog.Warn("hospitable: server error, retrying",
				"status", resp.StatusCode, "delay", delay, "attempt", backoff.Attempt())
			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			case <-time.After(delay):
				continue
			}

		default:
			return nil, "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}
	}
}

// parseLinkNext extracts the "next" URL from a Link header.
// Format: <https://api.hospitable.com/...?page=2>; rel="next"
func parseLinkNext(header string) string {
	if header == "" {
		return ""
	}
	for _, entry := range strings.Split(header, ",") {
		parts := strings.SplitN(entry, ";", 2)
		if len(parts) < 2 {
			continue
		}
		urlPart := strings.TrimSpace(parts[0])
		relPart := strings.TrimSpace(parts[1])
		if relPart == `rel="next"` || relPart == `rel=next` {
			if len(urlPart) > 2 && urlPart[0] == '<' && urlPart[len(urlPart)-1] == '>' {
				return urlPart[1 : len(urlPart)-1]
			}
		}
	}
	return ""
}

// parseRetryAfter parses a Retry-After header value per RFC 7231 §7.1.3.
// Supports integer seconds ("120") and HTTP-date ("Wed, 09 Apr 2026 12:00:00 GMT").
// Returns 0 if the header is absent or unparseable.
func parseRetryAfter(headerVal string, now time.Time) time.Duration {
	headerVal = strings.TrimSpace(headerVal)
	if headerVal == "" {
		return 0
	}
	// Try integer seconds first
	if seconds, err := strconv.Atoi(headerVal); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	// Try HTTP-date
	if t, err := http.ParseTime(headerVal); err == nil {
		d := t.Sub(now)
		if d > 0 {
			return d
		}
	}
	return 0
}
