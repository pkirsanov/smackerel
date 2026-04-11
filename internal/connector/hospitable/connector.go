package hospitable

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Ensure Connector implements the connector.Connector interface.
var _ connector.Connector = (*Connector)(nil)

// HospitableConfig holds parsed connector-specific configuration.
type HospitableConfig struct {
	AccessToken         string
	BaseURL             string
	SyncSchedule        string
	InitialLookbackDays int
	PageSize            int
	SyncProperties      bool
	SyncReservations    bool
	SyncMessages        bool
	SyncReviews         bool
	TierMessages        string
	TierReviews         string
	TierReservations    string
	TierProperties      string
}

// Connector implements the Hospitable connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config HospitableConfig
	client *Client

	// Sync metadata for health reporting
	lastSyncTime   time.Time
	lastSyncCounts map[string]int
	lastSyncErrors int

	// Property name cache for enriching reservation/review titles
	propertyNames map[string]string
}

// New creates a new Hospitable connector.
func New(id string) *Connector {
	return &Connector{
		id:             id,
		health:         connector.HealthDisconnected,
		lastSyncCounts: make(map[string]int),
		propertyNames:  make(map[string]string),
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseHospitableConfig(config)
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("parse hospitable config: %w", err)
	}

	client := NewClient(cfg.BaseURL, cfg.AccessToken, cfg.PageSize)

	if err := client.Validate(ctx); err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("validate hospitable token: %w", err)
	}

	c.mu.Lock()
	c.config = cfg
	c.client = client
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	slog.Info("Hospitable connector connected", "id", c.id)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	if c.client == nil {
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("hospitable connector not connected")
	}
	c.health = connector.HealthSyncing
	c.lastSyncErrors = 0
	c.lastSyncCounts = make(map[string]int)
	c.mu.Unlock()

	syncCursor := parseCursor(cursor, c.config.InitialLookbackDays)
	var allArtifacts []connector.RawArtifact
	var syncErrors int

	// Load persisted property name cache from cursor (R-018)
	if len(syncCursor.PropertyNames) > 0 {
		c.mu.Lock()
		for id, name := range syncCursor.PropertyNames {
			c.propertyNames[id] = name
		}
		c.mu.Unlock()
	}

	// 1. Sync properties (needed first for name cache)
	if c.config.SyncProperties {
		props, err := c.client.ListProperties(ctx, syncCursor.Properties)
		if err != nil {
			slog.Error("hospitable: property sync failed", "error", err)
			syncErrors++
		} else {
			for _, p := range props {
				c.mu.Lock()
				c.propertyNames[p.ID] = p.Name
				c.mu.Unlock()
				allArtifacts = append(allArtifacts, NormalizeProperty(p, c.config))
			}
			syncCursor.Properties = time.Now().UTC()
			c.mu.Lock()
			c.lastSyncCounts["properties"] = len(props)
			c.mu.Unlock()
		}
	}

	// 2. Sync reservations
	var reservationIDs []string
	if c.config.SyncReservations {
		reservations, err := c.client.ListReservations(ctx, syncCursor.Reservations)
		if err != nil {
			slog.Error("hospitable: reservation sync failed", "error", err)
			syncErrors++
		} else {
			for _, r := range reservations {
				c.mu.RLock()
				propName := c.propertyNames[r.PropertyID]
				c.mu.RUnlock()
				allArtifacts = append(allArtifacts, NormalizeReservation(r, propName, c.config))
				reservationIDs = append(reservationIDs, r.ID)
			}
			syncCursor.Reservations = time.Now().UTC()
			c.mu.Lock()
			c.lastSyncCounts["reservations"] = len(reservations)
			c.mu.Unlock()
		}
	}

	// 2b. Fetch active reservations for message sync coverage (R-016)
	if c.config.SyncMessages {
		activeRes, err := c.client.ListActiveReservations(ctx, time.Now().UTC().AddDate(0, 0, -7))
		if err != nil {
			slog.Warn("hospitable: active reservation fetch failed", "error", err)
		} else {
			seen := make(map[string]bool, len(reservationIDs))
			for _, id := range reservationIDs {
				seen[id] = true
			}
			for _, r := range activeRes {
				if !seen[r.ID] {
					reservationIDs = append(reservationIDs, r.ID)
					seen[r.ID] = true
				}
			}
		}
	}

	// 3. Sync messages per reservation (R-021: isolated cursor advancement)
	if c.config.SyncMessages {
		var msgAnyFailed bool
		var msgCount int
		for _, resID := range reservationIDs {
			messages, err := c.client.ListMessages(ctx, resID, syncCursor.Messages)
			if err != nil {
				slog.Warn("hospitable: message sync failed for reservation",
					"reservation_id", resID, "error", err)
				msgAnyFailed = true
				syncErrors++
				continue
			}
			for _, m := range messages {
				allArtifacts = append(allArtifacts, NormalizeMessage(m, resID, c.config))
				msgCount++
			}
		}
		if !msgAnyFailed {
			syncCursor.Messages = time.Now().UTC()
		}
		c.mu.Lock()
		c.lastSyncCounts["messages"] = msgCount
		c.mu.Unlock()
	}

	// 4. Sync reviews
	if c.config.SyncReviews {
		reviews, err := c.client.ListReviews(ctx, syncCursor.Reviews)
		if err != nil {
			slog.Error("hospitable: review sync failed", "error", err)
			syncErrors++
		} else {
			for _, r := range reviews {
				c.mu.RLock()
				propName := c.propertyNames[r.PropertyID]
				c.mu.RUnlock()
				allArtifacts = append(allArtifacts, NormalizeReview(r, propName, c.config))
			}
			syncCursor.Reviews = time.Now().UTC()
			c.mu.Lock()
			c.lastSyncCounts["reviews"] = len(reviews)
			c.mu.Unlock()
		}
	}

	// Persist property name cache in cursor (R-018)
	c.mu.RLock()
	syncCursor.PropertyNames = make(map[string]string, len(c.propertyNames))
	for id, name := range c.propertyNames {
		syncCursor.PropertyNames[id] = name
	}
	c.mu.RUnlock()

	// Store active reservation IDs in cursor (R-016)
	syncCursor.ActiveReservationIDs = reservationIDs

	newCursor := encodeCursor(syncCursor)

	c.mu.Lock()
	c.lastSyncTime = time.Now()
	c.lastSyncErrors = syncErrors
	if syncErrors > 0 && len(allArtifacts) == 0 {
		c.health = connector.HealthError
	} else {
		c.health = connector.HealthHealthy
	}
	logProperties := c.lastSyncCounts["properties"]
	logReservations := c.lastSyncCounts["reservations"]
	logMessages := c.lastSyncCounts["messages"]
	logReviews := c.lastSyncCounts["reviews"]
	c.mu.Unlock()

	slog.Info("Hospitable sync complete",
		"id", c.id,
		"artifacts", len(allArtifacts),
		"errors", syncErrors,
		"properties", logProperties,
		"reservations", logReservations,
		"messages", logMessages,
		"reviews", logReviews,
	)

	return allArtifacts, newCursor, nil
}

func (c *Connector) Health(_ context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = connector.HealthDisconnected
	c.client = nil
	slog.Info("Hospitable connector closed", "id", c.id)
	return nil
}

// parseHospitableConfig extracts configuration from ConnectorConfig.
func parseHospitableConfig(config connector.ConnectorConfig) (HospitableConfig, error) {
	cfg := HospitableConfig{
		BaseURL:             "https://api.hospitable.com",
		InitialLookbackDays: 90,
		PageSize:            100,
		SyncProperties:      true,
		SyncReservations:    true,
		SyncMessages:        true,
		SyncReviews:         true,
		TierMessages:        "full",
		TierReviews:         "full",
		TierReservations:    "standard",
		TierProperties:      "light",
	}

	// Extract access token
	if token, ok := config.Credentials["access_token"]; ok && token != "" {
		cfg.AccessToken = token
	} else {
		return cfg, fmt.Errorf("access_token is required")
	}

	sc := config.SourceConfig

	if v, ok := sc["base_url"].(string); ok && v != "" {
		parsed, err := url.Parse(v)
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return cfg, fmt.Errorf("base_url must be a valid HTTP(S) URL: %q", v)
		}
		cfg.BaseURL = v
	}
	if v, ok := sc["sync_schedule"].(string); ok && v != "" {
		cfg.SyncSchedule = v
	}
	if v, ok := sc["initial_lookback_days"].(float64); ok {
		if int(v) < 0 {
			return cfg, fmt.Errorf("initial_lookback_days must not be negative")
		}
		cfg.InitialLookbackDays = int(v)
	}
	if v, ok := sc["page_size"].(float64); ok && v > 0 {
		cfg.PageSize = int(v)
	}
	if v, ok := sc["sync_properties"].(bool); ok {
		cfg.SyncProperties = v
	}
	if v, ok := sc["sync_reservations"].(bool); ok {
		cfg.SyncReservations = v
	}
	if v, ok := sc["sync_messages"].(bool); ok {
		cfg.SyncMessages = v
	}
	if v, ok := sc["sync_reviews"].(bool); ok {
		cfg.SyncReviews = v
	}
	if v, ok := sc["processing_tier_messages"].(string); ok && v != "" {
		cfg.TierMessages = v
	}
	if v, ok := sc["processing_tier_reviews"].(string); ok && v != "" {
		cfg.TierReviews = v
	}
	if v, ok := sc["processing_tier_reservations"].(string); ok && v != "" {
		cfg.TierReservations = v
	}
	if v, ok := sc["processing_tier_properties"].(string); ok && v != "" {
		cfg.TierProperties = v
	}

	return cfg, nil
}

// parseCursor decodes a JSON cursor or returns a zero-value cursor with lookback applied.
func parseCursor(raw string, lookbackDays int) SyncCursor {
	if raw == "" {
		since := time.Now().UTC().AddDate(0, 0, -lookbackDays)
		return SyncCursor{
			Properties:   time.Time{}, // fetch all properties on first sync
			Reservations: since,
			Messages:     since,
			Reviews:      since,
		}
	}

	var cursor SyncCursor
	if err := json.Unmarshal([]byte(raw), &cursor); err != nil {
		slog.Warn("hospitable: invalid cursor, using lookback", "error", err)
		since := time.Now().UTC().AddDate(0, 0, -lookbackDays)
		return SyncCursor{
			Reservations: since,
			Messages:     since,
			Reviews:      since,
		}
	}
	return cursor
}

// encodeCursor serializes the cursor to JSON.
func encodeCursor(cursor SyncCursor) string {
	data, err := json.Marshal(cursor)
	if err != nil {
		slog.Error("hospitable: failed to encode cursor", "error", err)
		return ""
	}
	return string(data)
}
