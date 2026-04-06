package caldav

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the CalDAV calendar connector.
type Connector struct {
	id     string
	config connector.ConnectorConfig
	health connector.HealthStatus
}

// New creates a new CalDAV connector.
func New(id string) *Connector {
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	c.config = config
	if config.AuthType != "oauth2" {
		return fmt.Errorf("CalDAV connector requires oauth2 auth")
	}
	c.health = connector.HealthHealthy
	slog.Info("CalDAV connector connected", "id", c.id)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing
	defer func() { c.health = connector.HealthHealthy }()

	// Real implementation would:
	// 1. Fetch events from CalDAV endpoint since cursor (sync token)
	// 2. Extract attendees, link to People entities
	// 3. Assemble pre-meeting context (related artifacts for attendees)
	// 4. Create event artifacts with attendee metadata

	slog.Info("CalDAV sync cycle", "id", c.id, "cursor", cursor)
	return nil, cursor, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus { return c.health }
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	return nil
}

var _ connector.Connector = (*Connector)(nil)
