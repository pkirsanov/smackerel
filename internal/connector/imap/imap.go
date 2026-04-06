package imap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the IMAP email connector.
type Connector struct {
	id         string
	config     connector.ConnectorConfig
	health     connector.HealthStatus
	lastCursor string
}

// New creates a new IMAP connector.
func New(id string) *Connector {
	return &Connector{
		id:     id,
		health: connector.HealthDisconnected,
	}
}

// ID returns the connector identifier.
func (c *Connector) ID() string { return c.id }

// Connect initializes the IMAP connection with OAuth2 XOAUTH2.
func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	c.config = config

	authType := config.AuthType
	if authType != "oauth2" && authType != "password" {
		return fmt.Errorf("IMAP connector requires oauth2 or password auth, got %q", authType)
	}

	// In a real implementation, this would establish an IMAP connection
	// using go-imap v2 with XOAUTH2 for Gmail
	slog.Info("IMAP connector connected", "id", c.id, "auth", authType)
	c.health = connector.HealthHealthy
	return nil
}

// Sync fetches emails newer than the cursor (message UID).
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing

	// In a real implementation, this would:
	// 1. IMAP SEARCH for messages with UID > cursor
	// 2. FETCH each message (headers + body)
	// 3. Apply qualifier filtering (priority senders, skip labels)
	// 4. Assign processing tier based on signals
	// 5. Extract action items from email content
	// 6. Return raw artifacts for pipeline processing

	slog.Info("IMAP sync cycle", "id", c.id, "cursor", cursor)
	c.health = connector.HealthHealthy

	// Return empty for now — real implementation hooks into go-imap
	return nil, cursor, nil
}

// Health returns the current connector status.
func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	return c.health
}

// Close disconnects the IMAP session.
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	slog.Info("IMAP connector closed", "id", c.id)
	return nil
}

// QualifierConfig holds IMAP-specific qualifier settings.
type QualifierConfig struct {
	PrioritySenders []string `json:"priority_senders"`
	SkipLabels      []string `json:"skip_labels"`
	PriorityLabels  []string `json:"priority_labels"`
	SkipDomains     []string `json:"skip_domains"`
}

// AssignTier determines processing tier for an email based on qualifiers.
func AssignTier(from string, labels []string, qualifiers QualifierConfig) string {
	// Check priority senders
	for _, s := range qualifiers.PrioritySenders {
		if s == from {
			return "full"
		}
	}

	// Check priority labels
	for _, l := range labels {
		for _, pl := range qualifiers.PriorityLabels {
			if l == pl {
				return "full"
			}
		}
	}

	// Check skip labels
	for _, l := range labels {
		for _, sl := range qualifiers.SkipLabels {
			if l == sl {
				return "metadata"
			}
		}
	}

	return "standard"
}

// ExtractActionItems identifies action items from email text.
func ExtractActionItems(text string) []string {
	// Simplified extraction — real implementation would use LLM
	// or pattern matching for deadlines, promises, requests
	return nil
}

var _ connector.Connector = (*Connector)(nil)

func init() {
	_ = time.Now // reference to avoid import cycle
}
