package youtube

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the YouTube connector via YouTube Data API v3.
type Connector struct {
	id     string
	config connector.ConnectorConfig
	health connector.HealthStatus
}

// New creates a new YouTube connector.
func New(id string) *Connector {
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	c.config = config
	if config.AuthType != "oauth2" && config.AuthType != "api_key" {
		return fmt.Errorf("YouTube connector requires oauth2 or api_key auth")
	}
	c.health = connector.HealthHealthy
	slog.Info("YouTube connector connected", "id", c.id)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing
	defer func() { c.health = connector.HealthHealthy }()

	// Real implementation would:
	// 1. Fetch liked/watch-later/playlist videos via YouTube Data API v3
	// 2. Apply engagement-based tier assignment (liked=full, watch-later=standard)
	// 3. Fetch transcripts via Python sidecar (youtube-transcript-api)
	// 4. Tag topics from video categories and metadata

	slog.Info("YouTube sync cycle", "id", c.id, "cursor", cursor)
	return nil, cursor, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus { return c.health }
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	return nil
}

// EngagementTier assigns processing tier based on YouTube engagement signals.
func EngagementTier(liked bool, watchLater bool, playlistName string) string {
	if liked || playlistName != "" {
		return "full"
	}
	if watchLater {
		return "standard"
	}
	return "light"
}

var _ connector.Connector = (*Connector)(nil)
