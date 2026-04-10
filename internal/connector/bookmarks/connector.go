package bookmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
)

const (
	// maxFileSize is the largest bookmark export file we will read (50 MiB).
	maxFileSize = 50 << 20

	// maxCursorEntries caps the processed-files cursor list to prevent unbounded growth.
	maxCursorEntries = 1000
)

// Compile-time interface check.
var _ connector.Connector = (*BookmarksConnector)(nil)

// Config holds parsed bookmarks-specific configuration.
type Config struct {
	ImportDir        string
	WatchInterval    time.Duration
	ArchiveProcessed bool
	ProcessingTier   string
	MinURLLength     int
	ExcludeDomains   []string
}

// BookmarksConnector implements the connector.Connector interface for browser bookmark exports.
type BookmarksConnector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config Config

	// Optional DB pool for dedup and topic mapping (nil if not available)
	pool         *pgxpool.Pool
	deduplicator *URLDeduplicator
	topicMapper  *TopicMapper

	// Sync metadata for health reporting
	lastSyncTime   time.Time
	lastSyncCount  int
	lastSyncErrors int
}

// NewConnector creates a new Bookmarks connector.
func NewConnector(id string) *BookmarksConnector {
	return &BookmarksConnector{
		id:     id,
		health: connector.HealthDisconnected,
	}
}

// NewConnectorWithPool creates a new Bookmarks connector with DB pool for dedup and topic mapping.
func NewConnectorWithPool(id string, pool *pgxpool.Pool) *BookmarksConnector {
	return &BookmarksConnector{
		id:           id,
		health:       connector.HealthDisconnected,
		pool:         pool,
		deduplicator: NewURLDeduplicator(pool),
		topicMapper:  NewTopicMapper(pool),
	}
}

func (c *BookmarksConnector) ID() string { return c.id }

func (c *BookmarksConnector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseConfig(config)
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return err
	}

	// Validate import directory exists and is readable
	info, err := os.Stat(cfg.ImportDir)
	if os.IsNotExist(err) {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("import directory does not exist: %s", cfg.ImportDir)
	}
	if err != nil {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("import directory stat error: %w", err)
	}
	if !info.IsDir() {
		c.mu.Lock()
		c.health = connector.HealthError
		c.mu.Unlock()
		return fmt.Errorf("import directory is not a directory: %s", cfg.ImportDir)
	}

	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	slog.Info("bookmarks connector connected",
		"import_dir", cfg.ImportDir,
		"archive_processed", cfg.ArchiveProcessed,
		"processing_tier", cfg.ProcessingTier,
	)
	return nil
}

func (c *BookmarksConnector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	c.health = connector.HealthSyncing
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.lastSyncTime = time.Now()
		if c.lastSyncErrors > 0 && c.lastSyncCount == 0 {
			c.health = connector.HealthError
		} else {
			c.health = connector.HealthHealthy
		}
		c.mu.Unlock()
	}()

	// Decode cursor: JSON list of processed file names
	processedFiles := decodeProcessedFilesCursor(cursor)

	// Scan import directory for new files
	newFiles, err := c.findNewFiles(processedFiles)
	if err != nil {
		c.mu.Lock()
		c.lastSyncErrors = 1
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("scan import directory: %w", err)
	}

	if len(newFiles) == 0 {
		return nil, cursor, nil
	}

	var allArtifacts []connector.RawArtifact
	syncErrors := 0

	for _, file := range newFiles {
		// F-STAB-004: respect context cancellation between files
		if err := ctx.Err(); err != nil {
			c.mu.Lock()
			c.lastSyncErrors = syncErrors + 1
			c.mu.Unlock()
			return allArtifacts, encodeProcessedFilesCursor(processedFiles), fmt.Errorf("sync cancelled: %w", err)
		}

		artifacts, err := c.processFile(ctx, file)
		if err != nil {
			slog.Warn("failed to process bookmark export file",
				"file", file,
				"error", err,
			)
			syncErrors++
			continue
		}

		allArtifacts = append(allArtifacts, artifacts...)
		processedFiles = append(processedFiles, filepath.Base(file))

		// Archive processed file if configured
		if c.config.ArchiveProcessed {
			if err := c.archiveFile(file); err != nil {
				slog.Warn("failed to archive processed file",
					"file", file,
					"error", err,
				)
			}
		}
	}

	// Encode updated cursor
	newCursor := encodeProcessedFilesCursor(processedFiles)

	// Deduplicate by normalized URL against existing artifacts
	dedupCount := 0
	if c.deduplicator != nil && len(allArtifacts) > 0 {
		if err := ctx.Err(); err != nil {
			return allArtifacts, encodeProcessedFilesCursor(processedFiles), fmt.Errorf("sync cancelled before dedup: %w", err)
		}
		var err error
		allArtifacts, dedupCount, err = c.deduplicator.FilterNew(ctx, allArtifacts)
		if err != nil {
			slog.Warn("dedup filtering failed, proceeding with all artifacts", "error", err)
		}
	}

	// Map folder paths to topics and create edges
	if c.topicMapper != nil {
		for _, a := range allArtifacts {
			folder, _ := a.Metadata["folder_path"].(string)
			if folder == "" {
				folder, _ = a.Metadata["folder"].(string)
			}
			if folder == "" {
				continue
			}

			matches, err := c.topicMapper.MapFolder(ctx, folder)
			if err != nil {
				slog.Warn("topic mapping failed", "folder", folder, "error", err)
				continue
			}

			// Create BELONGS_TO edges to the most specific (last) topic
			if len(matches) > 0 {
				leafTopic := matches[len(matches)-1]
				if err := c.topicMapper.CreateTopicEdge(ctx, a.SourceRef, leafTopic.TopicID); err != nil {
					slog.Warn("create topic edge failed", "artifact", a.SourceRef, "topic", leafTopic.TopicID, "error", err)
				}
				// Update momentum for all matched topics
				for _, m := range matches {
					if err := c.topicMapper.UpdateTopicMomentum(ctx, m.TopicID); err != nil {
						slog.Warn("update topic momentum failed", "topic", m.TopicID, "error", err)
					}
				}
			}
		}
	}

	c.mu.Lock()
	c.lastSyncCount = len(allArtifacts)
	c.lastSyncErrors = syncErrors
	c.mu.Unlock()

	slog.Info("bookmarks sync complete",
		"new_files", len(newFiles),
		"artifacts", len(allArtifacts),
		"duplicates_skipped", dedupCount,
		"errors", syncErrors,
	)

	return allArtifacts, newCursor, nil
}

func (c *BookmarksConnector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *BookmarksConnector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health = connector.HealthDisconnected
	slog.Info("bookmarks connector closed")
	return nil
}

// findNewFiles scans the import directory for bookmark export files not yet processed.
func (c *BookmarksConnector) findNewFiles(processedFiles []string) ([]string, error) {
	processed := make(map[string]bool, len(processedFiles))
	for _, f := range processedFiles {
		processed[f] = true
	}

	entries, err := os.ReadDir(c.config.ImportDir)
	if err != nil {
		return nil, fmt.Errorf("read import directory: %w", err)
	}

	var newFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue // skip directories (including archive/)
		}

		// Skip symlinks to prevent reading files outside the import directory.
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))

		// Only process known bookmark export formats
		if ext != ".json" && ext != ".html" && ext != ".htm" {
			continue
		}

		// Skip already-processed files
		if processed[name] {
			continue
		}

		newFiles = append(newFiles, filepath.Join(c.config.ImportDir, name))
	}

	return newFiles, nil
}

// processFile reads and parses a bookmark export file, returning RawArtifacts.
func (c *BookmarksConnector) processFile(ctx context.Context, filePath string) ([]connector.RawArtifact, error) {
	// F-STAB-001: check file size before reading to prevent memory pressure
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file %s: %w", filePath, err)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("file %s exceeds max size (%d > %d bytes)", filepath.Base(filePath), info.Size(), maxFileSize)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", filePath, err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	var bookmarks []Bookmark

	switch ext {
	case ".json":
		bookmarks, err = ParseChromeJSON(data)
		if err != nil {
			return nil, fmt.Errorf("parse Chrome JSON %s: %w", filePath, err)
		}
	case ".html", ".htm":
		bookmarks, err = ParseNetscapeHTML(data)
		if err != nil {
			return nil, fmt.Errorf("parse Netscape HTML %s: %w", filePath, err)
		}
	default:
		slog.Warn("unknown bookmark export format, skipping", "file", filePath, "ext", ext)
		return nil, fmt.Errorf("unknown format: %s", ext)
	}

	// Convert to RawArtifacts
	artifacts := ToRawArtifacts(bookmarks)

	// F-STAB-007/008: apply domain exclusion and minimum URL length filters
	artifacts = c.filterArtifacts(artifacts)

	// Enrich metadata
	fileName := filepath.Base(filePath)
	sourceFormat := "chrome_json"
	if ext == ".html" || ext == ".htm" {
		sourceFormat = "netscape_html"
	}

	for i := range artifacts {
		if artifacts[i].Metadata == nil {
			artifacts[i].Metadata = make(map[string]interface{})
		}
		artifacts[i].Metadata["source_format"] = sourceFormat
		artifacts[i].Metadata["import_file"] = fileName
		artifacts[i].Metadata["processing_tier"] = c.config.ProcessingTier
		// folder_path is already set by ToRawArtifacts via the "folder" key
		if folder, ok := artifacts[i].Metadata["folder"]; ok {
			artifacts[i].Metadata["folder_path"] = folder
		}
	}

	slog.Info("processed bookmark export",
		"file", fileName,
		"format", sourceFormat,
		"bookmarks", len(bookmarks),
		"artifacts", len(artifacts),
	)

	return artifacts, nil
}

// archiveFile moves a processed file to the archive/ subdirectory.
func (c *BookmarksConnector) archiveFile(filePath string) error {
	archiveDir := filepath.Join(c.config.ImportDir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	baseName := filepath.Base(filePath)
	dest := filepath.Join(archiveDir, baseName)

	// F-STAB-006: avoid overwriting previously archived files
	if _, err := os.Stat(dest); err == nil {
		ext := filepath.Ext(baseName)
		name := strings.TrimSuffix(baseName, ext)
		dest = filepath.Join(archiveDir, fmt.Sprintf("%s_%d%s", name, time.Now().UnixMilli(), ext))
	}

	if err := os.Rename(filePath, dest); err != nil {
		return fmt.Errorf("move file to archive: %w", err)
	}

	slog.Debug("archived processed file", "from", filePath, "to", dest)
	return nil
}

// parseConfig extracts bookmarks-specific config from ConnectorConfig.
func parseConfig(config connector.ConnectorConfig) (Config, error) {
	cfg := Config{
		WatchInterval:    5 * time.Minute,
		ArchiveProcessed: true,
		ProcessingTier:   "full",
		MinURLLength:     10,
	}

	if config.ProcessingTier != "" {
		cfg.ProcessingTier = config.ProcessingTier
	}

	sc := config.SourceConfig

	// Import directory (required)
	if importDir, ok := sc["import_dir"].(string); ok && importDir != "" {
		cfg.ImportDir = importDir
	} else {
		return Config{}, fmt.Errorf("import directory is required")
	}

	// Watch interval
	if wi, ok := sc["watch_interval"].(string); ok && wi != "" {
		d, err := time.ParseDuration(wi)
		if err != nil {
			return Config{}, fmt.Errorf("invalid watch_interval %q: %w", wi, err)
		}
		cfg.WatchInterval = d
	}

	// Archive processed
	if ap, ok := sc["archive_processed"].(bool); ok {
		cfg.ArchiveProcessed = ap
	}

	// Min URL length
	if mul, ok := sc["min_url_length"]; ok {
		switch v := mul.(type) {
		case float64:
			cfg.MinURLLength = int(v)
		case int:
			cfg.MinURLLength = v
		}
	}

	// Exclude domains
	if ed, ok := sc["exclude_domains"].([]interface{}); ok {
		for _, d := range ed {
			if s, ok := d.(string); ok {
				cfg.ExcludeDomains = append(cfg.ExcludeDomains, s)
			}
		}
	}

	return cfg, nil
}

// decodeProcessedFilesCursor decodes a JSON-encoded list of processed file names.
func decodeProcessedFilesCursor(cursor string) []string {
	if cursor == "" {
		return nil
	}
	var files []string
	if err := json.Unmarshal([]byte(cursor), &files); err != nil {
		slog.Warn("failed to decode bookmarks cursor, starting fresh", "error", err)
		return nil
	}
	return files
}

// encodeProcessedFilesCursor encodes a list of processed file names as JSON.
func encodeProcessedFilesCursor(files []string) string {
	if len(files) == 0 {
		return ""
	}
	// F-STAB-002: cap cursor list to prevent unbounded growth
	if len(files) > maxCursorEntries {
		files = files[len(files)-maxCursorEntries:]
	}
	data, err := json.Marshal(files)
	if err != nil {
		slog.Error("failed to encode bookmarks cursor", "error", err)
		return ""
	}
	return string(data)
}

// filterArtifacts applies ExcludeDomains and MinURLLength filters.
func (c *BookmarksConnector) filterArtifacts(artifacts []connector.RawArtifact) []connector.RawArtifact {
	if len(c.config.ExcludeDomains) == 0 && c.config.MinURLLength <= 0 {
		return artifacts
	}

	excludeSet := make(map[string]bool, len(c.config.ExcludeDomains))
	for _, d := range c.config.ExcludeDomains {
		excludeSet[strings.ToLower(d)] = true
	}

	filtered := make([]connector.RawArtifact, 0, len(artifacts))
	for _, a := range artifacts {
		if c.config.MinURLLength > 0 && len(a.URL) < c.config.MinURLLength {
			continue
		}
		if len(excludeSet) > 0 {
			u, err := url.Parse(a.URL)
			if err == nil && excludeSet[strings.ToLower(u.Hostname())] {
				continue
			}
		}
		filtered = append(filtered, a)
	}
	return filtered
}
