package guesthost

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Ensure Connector implements the connector.Connector interface.
var _ connector.Connector = (*Connector)(nil)

// Connector implements the GuestHost connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config connector.ConnectorConfig
	client *Client
}

// New creates a new GuestHost connector.
func New() *Connector {
	return &Connector{
		id:     "guesthost",
		health: connector.HealthDisconnected,
	}
}

// setHealth updates the connector health status under lock.
func (c *Connector) setHealth(status connector.HealthStatus) {
	c.mu.Lock()
	c.health = status
	c.mu.Unlock()
}

// ID returns the unique identifier for this connector.
func (c *Connector) ID() string { return c.id }

// Connect initializes the connector with the given configuration.
func (c *Connector) Connect(ctx context.Context, cfg connector.ConnectorConfig) error {
	baseURL, err := extractString(cfg.SourceConfig, "base_url")
	if err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("guesthost connect: %w", err)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("guesthost connect: base_url must be a valid http(s) URL: %s", baseURL)
	}

	// IMP-013-001: Strip trailing slashes to prevent double-slash in API paths.
	baseURL = strings.TrimRight(baseURL, "/")

	apiKey, err := extractString(cfg.SourceConfig, "api_key")
	if err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("guesthost connect: %w", err)
	}

	client := NewClient(baseURL, apiKey)

	if err := client.Validate(ctx); err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("guesthost validate: %w", err)
	}

	c.mu.Lock()
	c.config = cfg
	c.client = client
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	slog.Info("GuestHost connector connected", "id", c.id)
	return nil
}

// Sync fetches new activity events since the last cursor position.
func (c *Connector) Sync(ctx context.Context, cursor string) (arts []connector.RawArtifact, newCur string, retErr error) {
	// R27-H23-01: Recover from panics so health is not stuck on "syncing".
	defer func() {
		if r := recover(); r != nil {
			c.setHealth(connector.HealthError)
			retErr = fmt.Errorf("guesthost sync panic: %v", r)
			slog.Error("guesthost: panic recovered in Sync", "panic", r)
		}
	}()
	// If Sync returns an error, ensure health transitions from syncing to error.
	defer func() {
		if retErr != nil {
			c.setHealth(connector.HealthError)
		}
	}()

	c.mu.Lock()
	if c.client == nil {
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("guesthost connector not connected")
	}
	client := c.client // snapshot before releasing lock
	// Snapshot config values under lock to avoid data race (SEC-013-001)
	var types string
	if et, ok := c.config.SourceConfig["event_types"]; ok {
		switch v := et.(type) {
		case string:
			types = v
		case []interface{}:
			// YAML list → join as CSV (H-013-002: don't silently ignore)
			parts := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					parts = append(parts, s)
				}
			}
			types = strings.Join(parts, ",")
		default:
			slog.Warn("guesthost: event_types has unexpected type, fetching all types",
				"type", fmt.Sprintf("%T", et))
		}
	}
	c.health = connector.HealthSyncing
	c.mu.Unlock()

	// Parse cursor as RFC3339 timestamp; empty means first sync
	since := cursor
	if since != "" {
		if _, err := time.Parse(time.RFC3339, since); err != nil {
			c.setHealth(connector.HealthError) // H-013-003: don't leave stale HealthSyncing
			return nil, cursor, fmt.Errorf("invalid cursor timestamp: %w", err)
		}
	}

	resp, err := client.FetchActivity(ctx, since, types, 100)
	if err != nil {
		c.setHealth(connector.HealthError)
		return nil, cursor, fmt.Errorf("guesthost sync: %w", err)
	}

	var artifacts []connector.RawArtifact
	var newCursor string
	var latestTime time.Time
	var normalizeErrors int

	for _, event := range resp.Events {
		artifact, err := NormalizeEvent(event)
		if err != nil {
			normalizeErrors++
			slog.Warn("guesthost: skipping event normalization",
				"event_id", event.ID, "type", event.Type, "error", err)
			continue
		}
		artifacts = append(artifacts, artifact)
		// Track the latest event timestamp as the new cursor using proper time comparison
		eventTime, parseErr := time.Parse(time.RFC3339, event.Timestamp)
		if parseErr == nil && eventTime.After(latestTime) {
			latestTime = eventTime
			newCursor = event.Timestamp
		}
	}

	// Circuit-breaker: if all events failed normalization and the API returned events,
	// advance the cursor to the latest event timestamp to prevent infinite retry loops.
	if len(artifacts) == 0 && len(resp.Events) > 0 && normalizeErrors == len(resp.Events) {
		for _, event := range resp.Events {
			eventTime, parseErr := time.Parse(time.RFC3339, event.Timestamp)
			if parseErr == nil && eventTime.After(latestTime) {
				latestTime = eventTime
				newCursor = event.Timestamp
			}
		}
		slog.Warn("guesthost: all events failed normalization, advancing cursor to prevent loop",
			"events", len(resp.Events), "cursor", newCursor)
	}

	if newCursor == "" {
		newCursor = cursor
	}

	// CHAOS-013-002: Only restore HealthHealthy if the connector has not been
	// closed while this Sync was in-flight. Without this guard, Sync's deferred
	// setHealth overwrites Close's HealthDisconnected, making a closed connector
	// appear healthy.
	c.mu.RLock()
	closed := c.client == nil
	c.mu.RUnlock()
	if !closed {
		c.setHealth(connector.HealthHealthy)
	}

	slog.Info("GuestHost sync complete", "id", c.id, "events", len(artifacts), "cursor", newCursor)
	return artifacts, newCursor, nil
}

// Health returns the current health status of the connector.
func (c *Connector) Health(_ context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

// Close shuts down the connector and releases resources.
func (c *Connector) Close() error {
	c.mu.Lock()
	c.client = nil
	c.health = connector.HealthDisconnected
	c.mu.Unlock()
	slog.Info("GuestHost connector closed", "id", c.id)
	return nil
}

// extractString extracts a required string value from a config map.
func extractString(m map[string]interface{}, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", fmt.Errorf("missing required config key %q", key)
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("config key %q must be a non-empty string", key)
	}
	return s, nil
}
