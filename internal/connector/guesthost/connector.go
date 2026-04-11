package guesthost

import (
	"context"
	"fmt"
	"log/slog"
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

// ID returns the unique identifier for this connector.
func (c *Connector) ID() string { return c.id }

// Connect initializes the connector with the given configuration.
func (c *Connector) Connect(ctx context.Context, cfg connector.ConnectorConfig) error {
	baseURL, err := extractString(cfg.SourceConfig, "base_url")
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("guesthost connect: %w", err)
	}

	apiKey, err := extractString(cfg.SourceConfig, "api_key")
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("guesthost connect: %w", err)
	}

	client := NewClient(baseURL, apiKey)

	if err := client.Validate(ctx); err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
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
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	if c.client == nil {
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("guesthost connector not connected")
	}
	c.health = connector.HealthSyncing
	c.mu.Unlock()

	// Parse cursor as RFC3339 timestamp; empty means first sync
	since := cursor
	if since != "" {
		if _, err := time.Parse(time.RFC3339, since); err != nil {
			return nil, cursor, fmt.Errorf("invalid cursor timestamp: %w", err)
		}
	}

	// Build event_types CSV from config
	var types string
	if et, ok := c.config.SourceConfig["event_types"]; ok {
		if s, ok := et.(string); ok {
			types = s
		}
	}

	resp, err := c.client.FetchActivity(ctx, since, types, 100)
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("guesthost sync: %w", err)
	}

	var artifacts []connector.RawArtifact
	var newCursor string

	for _, event := range resp.Events {
		artifact, err := NormalizeEvent(event)
		if err != nil {
			slog.Warn("guesthost: skipping event normalization",
				"event_id", event.ID, "type", event.Type, "error", err)
			continue
		}
		artifacts = append(artifacts, artifact)
		// Track the latest event timestamp as the new cursor
		if event.Timestamp > newCursor {
			newCursor = event.Timestamp
		}
	}

	if newCursor == "" {
		newCursor = cursor
	}

	c.mu.Lock()
	c.health = connector.HealthHealthy
	c.mu.Unlock()

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
