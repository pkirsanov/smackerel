package connector

import (
	"context"
	"time"
)

// HealthStatus represents the health state of a connector.
type HealthStatus string

const (
	HealthHealthy      HealthStatus = "healthy"
	HealthSyncing      HealthStatus = "syncing"
	HealthError        HealthStatus = "error"
	HealthDisconnected HealthStatus = "disconnected"
)

// RawArtifact is the raw data produced by a connector sync.
type RawArtifact struct {
	SourceID    string                 `json:"source_id"`
	SourceRef   string                 `json:"source_ref"`
	ContentType string                 `json:"content_type"`
	Title       string                 `json:"title"`
	RawContent  string                 `json:"raw_content"`
	URL         string                 `json:"url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CapturedAt  time.Time              `json:"captured_at"`
}

// Connector defines the interface that all source connectors must implement.
type Connector interface {
	// ID returns the unique identifier for this connector instance.
	ID() string

	// Connect initializes the connector with the given configuration.
	Connect(ctx context.Context, config ConnectorConfig) error

	// Sync fetches new items since the last cursor position.
	// Returns the fetched items, a new cursor, and any error.
	Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)

	// Health returns the current health status of the connector.
	Health(ctx context.Context) HealthStatus

	// Close shuts down the connector and releases resources.
	Close() error
}

// ConnectorConfig holds configuration for a connector instance.
type ConnectorConfig struct {
	// Auth configuration
	AuthType    string            `json:"auth_type"`   // oauth2, api_key, token, none
	Credentials map[string]string `json:"credentials"` // type-specific credentials

	// Schedule configuration
	SyncSchedule string `json:"sync_schedule"` // cron expression
	Enabled      bool   `json:"enabled"`

	// Processing configuration
	ProcessingTier string                 `json:"processing_tier"` // full, standard, light, metadata
	Qualifiers     map[string]interface{} `json:"qualifiers"`

	// Source-specific configuration
	SourceConfig map[string]interface{} `json:"source_config"`
}
