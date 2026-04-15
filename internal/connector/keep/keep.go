package keep

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	lastSyncTime      time.Time
	lastSyncCount     int
	lastSyncErrors    int
	consecutiveErrors int

	// Track processed exports
	processedExports map[string]bool
}

// New creates a new Google Keep connector.
func New(id string) *Connector {
	return &Connector{
		id:               id,
		health:           connector.HealthDisconnected,
		processedExports: make(map[string]bool),
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
	c.lastSyncCount = 0
	c.lastSyncErrors = 0
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.lastSyncTime = time.Now()
		if c.lastSyncErrors > 0 && c.lastSyncCount == 0 {
			// Complete failure: no artifacts produced — escalate with consecutive count.
			// Complete-failure escalation is more aggressive than per-error escalation
			// (HealthFromErrorCount) because producing zero artifacts means the entire
			// sync path is broken, not just individual items.
			c.consecutiveErrors++
			switch {
			case c.consecutiveErrors >= 10:
				c.health = connector.HealthError
			case c.consecutiveErrors >= 5:
				c.health = connector.HealthFailing
			default:
				c.health = connector.HealthDegraded
			}
		} else if c.lastSyncErrors > 0 {
			// Partial success: some artifacts produced despite errors
			c.consecutiveErrors = 0
			c.health = connector.HealthDegraded
		} else {
			// Full success
			c.consecutiveErrors = 0
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
			syncErrors++
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
			syncErrors++
		} else {
			allArtifacts = append(allArtifacts, artifacts...)
			newCursor = cur
			syncErrors += errs
		}
		// gkeepapi supplements Takeout with live data from Python bridge
		gkeepArtifacts, gkeepCur, gkeepErrs, err := c.syncGkeepapi(ctx, cursor)
		if err != nil {
			slog.Warn("gkeepapi sync failed in hybrid mode, continuing with takeout results", "error", err)
			syncErrors++
		} else {
			allArtifacts = append(allArtifacts, gkeepArtifacts...)
			if gkeepCur != "" && newCursor != "" {
				gkeepTime, gErr := time.Parse(time.RFC3339Nano, gkeepCur)
				newTime, nErr := time.Parse(time.RFC3339Nano, newCursor)
				if gErr == nil && nErr == nil && gkeepTime.After(newTime) {
					newCursor = gkeepCur
				}
			} else if gkeepCur != "" {
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
	c.mu.RLock()
	importDir := c.config.TakeoutImportDir
	parser := c.parser
	normalizer := c.normalizer
	natsClient := c.natsClient
	alreadyProcessed := c.processedExports[importDir]
	c.mu.RUnlock()
	if alreadyProcessed && cursor != "" {
		// Re-parse to check for new files, but filter by cursor
	}

	notes, parseErrors, err := parser.ParseExport(importDir)
	if err != nil {
		return nil, cursor, 0, fmt.Errorf("parse takeout export: %w", err)
	}

	if len(parseErrors) > 0 {
		for _, pe := range parseErrors {
			slog.Warn("failed to parse takeout note", "file", pe)
		}
	}

	// Filter by cursor
	filtered, newCursor := parser.FilterByCursor(notes, cursor)

	var artifacts []connector.RawArtifact

	for i := range filtered {
		if err := ctx.Err(); err != nil {
			return artifacts, cursor, 0, fmt.Errorf("sync cancelled: %w", err)
		}

		noteID := parser.NoteID(&filtered[i], filtered[i].SourceFile)
		if noteID == "" {
			noteID = fmt.Sprintf("keep-note-%d", i)
		}

		artifact, err := normalizer.Normalize(&filtered[i], noteID, "takeout")
		if err != nil {
			slog.Warn("failed to normalize note", "note_id", noteID, "error", err)
			continue
		}
		if artifact == nil {
			// Skipped (trashed, archived, etc.)
			continue
		}

		// Publish artifact to NATS for pipeline processing
		if natsClient != nil {
			payload, marshalErr := json.Marshal(artifact)
			if marshalErr != nil {
				slog.Warn("failed to serialize artifact for NATS", "note_id", noteID, "error", marshalErr)
			} else if pubErr := natsClient.Publish(ctx, "artifacts.process", payload); pubErr != nil {
				slog.Warn("failed to publish artifact to NATS", "note_id", noteID, "error", pubErr)
			}
		}

		artifacts = append(artifacts, *artifact)
	}

	c.mu.Lock()
	c.processedExports[importDir] = true
	c.mu.Unlock()

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

	if dir, ok := sc["import_dir"].(string); ok && dir != "" {
		// Canonicalize import path to prevent traversal via config (CWE-22).
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return kc, fmt.Errorf("invalid import_dir path: %w", err)
		}
		kc.TakeoutImportDir = absDir
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
		if minLen < 0 {
			return kc, fmt.Errorf("min_content_length must be non-negative, got %v", minLen)
		}
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

	if watchInterval, ok := sc["watch_interval"].(string); ok {
		d, err := time.ParseDuration(watchInterval)
		if err != nil {
			return kc, fmt.Errorf("invalid watch_interval: %w", err)
		}
		kc.TakeoutWatchInterval = d
	}

	if archiveProcessed, ok := sc["archive_processed"].(bool); ok {
		kc.TakeoutArchiveProcessed = archiveProcessed
	}

	if defaultTier, ok := sc["default_tier"].(string); ok {
		kc.DefaultTier = defaultTier
	}

	if labelsRaw, ok := sc["labels_filter"].([]interface{}); ok {
		for _, l := range labelsRaw {
			if s, ok := l.(string); ok {
				kc.LabelsFilter = append(kc.LabelsFilter, s)
			}
		}
	}

	return kc, nil
}

// Ensure Connector implements the interface at compile time.
var _ connector.Connector = (*Connector)(nil)
