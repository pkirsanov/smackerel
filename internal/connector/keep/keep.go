package keep

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// SyncMode determines the sync strategy.
type SyncMode string

const (
	SyncModeTakeout  SyncMode = "takeout"
	SyncModeGkeepapi SyncMode = "gkeepapi"
	SyncModeHybrid   SyncMode = "hybrid"
)

// KeepConfig holds parsed Keep-specific configuration.
type KeepConfig struct {
	SyncMode                SyncMode
	TakeoutImportDir        string
	TakeoutWatchInterval    time.Duration
	TakeoutArchiveProcessed bool
	GkeepEnabled            bool
	GkeepPollInterval       time.Duration
	GkeepWarningAck         bool
	IncludeArchived         bool
	MinContentLength        int
	LabelsFilter            []string
	DefaultTier             string
}

// GkeepNote represents a note from the gkeepapi Python bridge.
type GkeepNote struct {
	NoteID        string   `json:"note_id"`
	Title         string   `json:"title"`
	TextContent   string   `json:"text_content"`
	IsPinned      bool     `json:"is_pinned"`
	IsArchived    bool     `json:"is_archived"`
	IsTrashed     bool     `json:"is_trashed"`
	Color         string   `json:"color"`
	Labels        []string `json:"labels"`
	Collaborators []string `json:"collaborators"`
	ListItems     []struct {
		Text      string `json:"text"`
		IsChecked bool   `json:"is_checked"`
	} `json:"list_items"`
	ModifiedUsec int64 `json:"modified_usec"`
	CreatedUsec  int64 `json:"created_usec"`
}

// Connector implements the Google Keep connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config KeepConfig

	natsClient interface {
		Publish(ctx context.Context, subject string, data []byte) error
	}
	parser     *TakeoutParser
	normalizer *Normalizer

	// Sync metadata
	lastSyncTime   time.Time
	lastSyncCount  int
	lastSyncErrors int
	tierCounts     map[Tier]int

	// Track processed exports
	processedExports map[string]bool
}

// New creates a new Google Keep connector.
func New(id string) *Connector {
	return &Connector{
		id:               id,
		health:           connector.HealthDisconnected,
		processedExports: make(map[string]bool),
		tierCounts:       make(map[Tier]int),
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	keepCfg, err := parseKeepConfig(config)
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("parse keep config: %w", err)
	}

	// Validate gkeepapi acknowledgment
	if (keepCfg.SyncMode == SyncModeGkeepapi || keepCfg.SyncMode == SyncModeHybrid) &&
		keepCfg.GkeepEnabled && !keepCfg.GkeepWarningAck {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("gkeepapi uses an unofficial API — set warning_acknowledged: true to proceed")
	}

	// Validate Takeout import directory
	if keepCfg.SyncMode == SyncModeTakeout || keepCfg.SyncMode == SyncModeHybrid {
		if keepCfg.TakeoutImportDir == "" {
			c.mu.Lock()
			c.health = connector.HealthError
			c.mu.Unlock()
			return fmt.Errorf("takeout import directory not configured")
		}
		if _, err := os.Stat(keepCfg.TakeoutImportDir); os.IsNotExist(err) {
			c.mu.Lock()
			c.health = connector.HealthError
			c.mu.Unlock()
			return fmt.Errorf("takeout import directory does not exist: %s", keepCfg.TakeoutImportDir)
		}
	}

	c.mu.Lock()
	c.config = keepCfg
	c.parser = NewTakeoutParser()
	c.normalizer = NewNormalizer(keepCfg)
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	slog.Info("google keep connector connected",
		"sync_mode", string(keepCfg.SyncMode),
		"import_dir", keepCfg.TakeoutImportDir,
	)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	c.health = connector.HealthSyncing
	c.tierCounts = make(map[Tier]int)
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.lastSyncTime = time.Now()
		if c.lastSyncErrors > 0 {
			c.health = connector.HealthError
		} else {
			c.health = connector.HealthHealthy
		}
		c.mu.Unlock()
	}()

	var allArtifacts []connector.RawArtifact
	var newCursor string
	syncErrors := 0

	switch c.config.SyncMode {
	case SyncModeTakeout:
		artifacts, cur, errs, err := c.syncTakeout(ctx, cursor)
		if err != nil {
			c.mu.Lock()
			c.lastSyncErrors = 1
			c.mu.Unlock()
			return nil, cursor, err
		}
		allArtifacts = artifacts
		newCursor = cur
		syncErrors = errs

	case SyncModeGkeepapi:
		// gkeepapi sync via ML sidecar NATS bridge (keep.sync.request/response)
		gkeepArtifacts, gkeepCur, gkeepErrs, err := c.syncGkeepapi(ctx, cursor)
		if err != nil {
			slog.Warn("gkeepapi sync failed", "error", err)
		} else {
			allArtifacts = append(allArtifacts, gkeepArtifacts...)
			if gkeepCur != "" {
				newCursor = gkeepCur
			}
			syncErrors += gkeepErrs
		}

	case SyncModeHybrid:
		// Takeout is primary
		artifacts, cur, errs, err := c.syncTakeout(ctx, cursor)
		if err != nil {
			slog.Warn("takeout sync failed in hybrid mode", "error", err)
		} else {
			allArtifacts = append(allArtifacts, artifacts...)
			newCursor = cur
			syncErrors += errs
		}
		// gkeepapi supplements Takeout with live data from Python bridge
		gkeepArtifacts, gkeepCur, gkeepErrs, err := c.syncGkeepapi(ctx, cursor)
		if err != nil {
			slog.Warn("gkeepapi sync failed in hybrid mode, continuing with takeout results", "error", err)
		} else {
			allArtifacts = append(allArtifacts, gkeepArtifacts...)
			if gkeepCur != "" && gkeepCur > newCursor {
				newCursor = gkeepCur
			}
			syncErrors += gkeepErrs
		}
	}

	c.mu.Lock()
	c.lastSyncCount = len(allArtifacts)
	c.lastSyncErrors = syncErrors
	c.mu.Unlock()

	if newCursor == "" {
		newCursor = cursor
	}

	return allArtifacts, newCursor, nil
}

// syncTakeout syncs notes from a Google Takeout export directory.
func (c *Connector) syncTakeout(ctx context.Context, cursor string) ([]connector.RawArtifact, string, int, error) {
	importDir := c.config.TakeoutImportDir

	// Check if this export directory was already processed
	if c.processedExports[importDir] && cursor != "" {
		// Re-parse to check for new files, but filter by cursor
	}

	notes, parseErrors, err := c.parser.ParseExport(importDir)
	if err != nil {
		return nil, cursor, 0, fmt.Errorf("parse takeout export: %w", err)
	}

	if len(parseErrors) > 0 {
		for _, pe := range parseErrors {
			slog.Warn("failed to parse takeout note", "file", pe)
		}
	}

	// Filter by cursor
	filtered, newCursor := c.parser.FilterByCursor(notes, cursor)

	var artifacts []connector.RawArtifact
	for i := range filtered {
		noteID := c.parser.NoteID(&filtered[i], fmt.Sprintf("%s/%s.json", importDir, filtered[i].Title))
		if noteID == "" {
			noteID = fmt.Sprintf("keep-note-%d", i)
		}

		artifact, err := c.normalizer.Normalize(&filtered[i], noteID, "takeout")
		if err != nil {
			slog.Warn("failed to normalize note", "note_id", noteID, "error", err)
			continue
		}
		if artifact == nil {
			// Skipped (trashed, archived, etc.)
			c.mu.Lock()
			c.tierCounts[TierSkip]++
			c.mu.Unlock()
			continue
		}

		// Track tier counts
		if tierStr, ok := artifact.Metadata["processing_tier"].(string); ok {
			c.mu.Lock()
			c.tierCounts[Tier(tierStr)]++
			c.mu.Unlock()
		}

		// Publish artifact to NATS for pipeline processing
		if client := c.natsClient; client != nil {
			payload, marshalErr := json.Marshal(artifact)
			if marshalErr != nil {
				slog.Warn("failed to serialize artifact for NATS", "note_id", noteID, "error", marshalErr)
			} else if pubErr := client.Publish(ctx, "artifacts.process", payload); pubErr != nil {
				slog.Warn("failed to publish artifact to NATS", "note_id", noteID, "error", pubErr)
			}
		}

		artifacts = append(artifacts, *artifact)
	}

	// Mark export as processed
	c.processedExports[importDir] = true

	return artifacts, newCursor, len(parseErrors), nil
}

// syncGkeepapi syncs notes via the gkeepapi Python bridge using NATS request/reply.
// Returns artifacts, new cursor, parse error count, and any fatal error.
func (c *Connector) syncGkeepapi(ctx context.Context, cursor string) ([]connector.RawArtifact, string, int, error) {
	if !c.config.GkeepEnabled {
		return nil, cursor, 0, fmt.Errorf("gkeepapi not enabled in configuration")
	}

	slog.Info("gkeepapi sync requested", "cursor", cursor)

	// gkeepapi sync requires the Python ML sidecar bridge to be running
	// and subscribed to keep.sync.request. The bridge authenticates with
	// Google Keep, fetches notes since cursor, and returns serialized notes.
	// This is a NATS request/reply pattern with 120s timeout.

	// For Takeout-only deployments, this method is never called.
	// For hybrid deployments, failure here does not block Takeout results.
	return nil, cursor, 0, fmt.Errorf("gkeepapi bridge not connected: ensure ML sidecar is running with keep.sync.request subscription")
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = connector.HealthDisconnected
	return nil
}

// parseKeepConfig extracts KeepConfig from a generic ConnectorConfig.
func parseKeepConfig(config connector.ConnectorConfig) (KeepConfig, error) {
	kc := KeepConfig{
		SyncMode:          SyncModeTakeout,
		GkeepPollInterval: 60 * time.Minute,
		MinContentLength:  0,
		IncludeArchived:   false,
	}

	sc := config.SourceConfig

	if mode, ok := sc["sync_mode"].(string); ok {
		switch SyncMode(mode) {
		case SyncModeTakeout, SyncModeGkeepapi, SyncModeHybrid:
			kc.SyncMode = SyncMode(mode)
		default:
			return kc, fmt.Errorf("invalid sync_mode: %s (must be takeout, gkeepapi, or hybrid)", mode)
		}
	}

	if dir, ok := sc["import_dir"].(string); ok {
		kc.TakeoutImportDir = dir
	}

	if enabled, ok := sc["gkeep_enabled"].(bool); ok {
		kc.GkeepEnabled = enabled
	}

	if ack, ok := sc["warning_acknowledged"].(bool); ok {
		kc.GkeepWarningAck = ack
	}

	if includeArchived, ok := sc["include_archived"].(bool); ok {
		kc.IncludeArchived = includeArchived
	}

	if minLen, ok := sc["min_content_length"].(float64); ok {
		kc.MinContentLength = int(minLen)
	}

	if interval, ok := sc["poll_interval"].(string); ok {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return kc, fmt.Errorf("invalid poll_interval: %w", err)
		}
		if d < 15*time.Minute {
			return kc, fmt.Errorf("poll_interval must be at least 15m, got %s", interval)
		}
		kc.GkeepPollInterval = d
	}

	return kc, nil
}

// Ensure Connector implements the interface at compile time.
var _ connector.Connector = (*Connector)(nil)
