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

// Client wraps the Hospitable Public API.
type Client struct {
	baseURL    string
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
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		backoff:  connector.DefaultBackoff(),
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
func fetchPaginated[T any](c *Client, ctx context.Context, path string, params url.Values) ([]T, error) {
	var all []T

	currentURL := c.baseURL + path + "?" + params.Encode()

	for currentURL != "" {
		body, nextURL, err := c.doGetPaginated(ctx, currentURL)
		if err != nil {
			return all, err
		}

		var page PaginatedResponse[T]
		if err := json.Unmarshal(body, &page); err != nil {
			return all, fmt.Errorf("decode response: %w", err)
		}

		all = append(all, page.Data...)

		if page.NextURL != "" {
			currentURL = page.NextURL
		} else {
			currentURL = nextURL
		}
	}

	return all, nil
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
func (c *Client) doGetPaginated(ctx context.Context, rawURL string) ([]byte, string, error) {
	c.backoff.Reset()

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

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, "", fmt.Errorf("read response: %w", err)
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
			delay, ok := c.backoff.Next()
			if !ok {
				return nil, "", fmt.Errorf("rate limited: max retries exceeded")
			}
			retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
			if retryAfter > delay {
				delay = retryAfter
			}
			slog.Info("hospitable: rate limited, backing off",
				"delay", delay, "retry_after", retryAfter, "attempt", c.backoff.Attempt())
			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			case <-time.After(delay):
				continue
			}

		case resp.StatusCode >= 500:
			delay, ok := c.backoff.Next()
			if !ok {
				return nil, "", fmt.Errorf("server error %d: max retries exceeded", resp.StatusCode)
			}
			slog.Warn("hospitable: server error, retrying",
				"status", resp.StatusCode, "delay", delay, "attempt", c.backoff.Attempt())
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
	// Simple parser for Link: <url>; rel="next"
	for _, part := range splitLinks(header) {
		if len(part) < 2 {
			continue
		}
		urlPart := part[0]
		relPart := part[1]
		if relPart == `rel="next"` || relPart == `rel=next` {
			// Strip angle brackets
			if len(urlPart) > 2 && urlPart[0] == '<' && urlPart[len(urlPart)-1] == '>' {
				return urlPart[1 : len(urlPart)-1]
			}
		}
	}
	return ""
}

// splitLinks parses a Link header into [url, rel] pairs.
func splitLinks(header string) [][]string {
	var result [][]string
	start := 0
	for i := 0; i < len(header); i++ {
		if header[i] == ',' {
			link := parseOneLink(header[start:i])
			if link != nil {
				result = append(result, link)
			}
			start = i + 1
		}
	}
	link := parseOneLink(header[start:])
	if link != nil {
		result = append(result, link)
	}
	return result
}

func parseOneLink(s string) []string {
	parts := splitSemicolon(s)
	if len(parts) < 2 {
		return nil
	}
	return []string{trimSpace(parts[0]), trimSpace(parts[1])}
}

func splitSemicolon(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
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
