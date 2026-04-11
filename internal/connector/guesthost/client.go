package guesthost

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// maxPaginationPages caps pagination requests to prevent infinite loops.
const maxPaginationPages = 1000

// maxResponseSize limits each response body to 10 MiB (OOM defence).
const maxResponseSize = 10 << 20

// Client wraps the GuestHost REST API.
type Client struct {
	baseURL    string
	baseOrigin string // scheme+host for same-origin pagination checks
	apiKey     string
	httpClient *http.Client
	backoff    *connector.Backoff
}

// NewClient creates a new GuestHost API client.
func NewClient(baseURL, apiKey string) *Client {
	origin := ""
	if parsed, err := url.Parse(baseURL); err == nil {
		origin = parsed.Scheme + "://" + parsed.Host
	}
	return &Client{
		baseURL:    baseURL,
		baseOrigin: origin,
		apiKey:     apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		backoff: &connector.Backoff{
			BaseDelay:  1 * time.Second,
			MaxDelay:   16 * time.Second,
			MaxRetries: 3,
		},
	}
}

// SetBackoff replaces the default backoff policy (used for testing).
func (c *Client) SetBackoff(b *connector.Backoff) {
	c.backoff = b
}

// Validate tests the API key by hitting the GuestHost health endpoint.
// Returns nil on 200, or a descriptive error for 401/403/other failures.
func (c *Client) Validate(ctx context.Context) error {
	body, err := c.doGet(ctx, c.baseURL+"/health")
	if err != nil {
		return fmt.Errorf("validate api key: %w", err)
	}
	_ = body
	return nil
}

// FetchActivity retrieves activity events from GuestHost.
// It handles cursor-based pagination, accumulating all pages into a single slice.
//
// Parameters:
//   - since: RFC3339 timestamp for incremental sync; empty string for first sync
//   - types: comma-separated event types to filter; empty string for all types
//   - limit: max events per page request
func (c *Client) FetchActivity(ctx context.Context, since string, types string, limit int) (*ActivityFeedResponse, error) {
	params := url.Values{}
	if since != "" {
		params.Set("since", since)
	}
	if types != "" {
		params.Set("types", types)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	fullURL := c.baseURL + "/api/v1/activity"
	if len(params) > 0 {
		fullURL += "?" + params.Encode()
	}

	var allEvents []ActivityEvent

	for page := 0; page < maxPaginationPages; page++ {
		body, err := c.doGet(ctx, fullURL)
		if err != nil {
			return nil, fmt.Errorf("fetch activity page %d: %w", page, err)
		}

		var resp ActivityFeedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode activity response: %w", err)
		}

		allEvents = append(allEvents, resp.Events...)

		if !resp.HasMore || resp.Cursor == "" {
			break
		}

		// Build next page URL with the returned cursor
		nextParams := url.Values{}
		if since != "" {
			nextParams.Set("since", since)
		}
		if types != "" {
			nextParams.Set("types", types)
		}
		if limit > 0 {
			nextParams.Set("limit", strconv.Itoa(limit))
		}
		nextParams.Set("cursor", resp.Cursor)
		fullURL = c.baseURL + "/api/v1/activity?" + nextParams.Encode()
	}

	return &ActivityFeedResponse{
		Events:  allEvents,
		HasMore: false,
	}, nil
}

// doGet makes an authenticated GET request with retry on 429 and 5xx.
func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	backoff := &connector.Backoff{
		BaseDelay:  c.backoff.BaseDelay,
		MaxDelay:   c.backoff.MaxDelay,
		MaxRetries: c.backoff.MaxRetries,
	}

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("http request: %w", err)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		if int64(len(body)) > maxResponseSize {
			return nil, fmt.Errorf("response body exceeds %d bytes limit", maxResponseSize)
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil

		case resp.StatusCode == http.StatusUnauthorized:
			return nil, fmt.Errorf("unauthorized: invalid or expired api key")

		case resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("forbidden: insufficient permissions")

		case resp.StatusCode == http.StatusTooManyRequests:
			delay, ok := backoff.Next()
			if !ok {
				return nil, fmt.Errorf("rate limited: max retries exceeded")
			}
			slog.Info("guesthost: rate limited, backing off",
				"delay", delay, "attempt", backoff.Attempt())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}

		case resp.StatusCode >= 500:
			delay, ok := backoff.Next()
			if !ok {
				return nil, fmt.Errorf("server error %d: max retries exceeded", resp.StatusCode)
			}
			slog.Warn("guesthost: server error, retrying",
				"status", resp.StatusCode, "delay", delay, "attempt", backoff.Attempt())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
				continue
			}

		default:
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}
	}
}
