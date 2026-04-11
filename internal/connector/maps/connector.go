package maps

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
)

// Compile-time interface check.
var _ connector.Connector = (*Connector)(nil)

// MapsConfig holds parsed Maps-specific configuration.
type MapsConfig struct {
	ImportDir        string
	WatchInterval    time.Duration
	ArchiveProcessed bool
	MinDistanceM     float64
	MinDurationMin   float64

	// Cluster/commute/trip/link config (used by future scopes)
	LocationRadiusM       float64
	HomeDetection         string
	CommuteMinOccurrences int
	CommuteWindowDays     int
	CommuteWeekdaysOnly   bool
	TripMinDistanceKm     float64
	TripMinOvernightHours float64
	LinkTimeExtendMin     float64
	LinkProximityRadiusM  float64
}

// Connector implements the Google Maps Timeline connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config MapsConfig
	pool   *pgxpool.Pool

	// Sync metadata for health reporting
	lastSyncTime   time.Time
	lastSyncCount  int
	lastSyncErrors int
	lastTrailCount int
}

// New creates a new Google Maps Timeline connector.
func New(id string) *Connector {
	return &Connector{
		id:     id,
		health: connector.HealthDisconnected,
	}
}

// setHealth sets the connector's health status under lock.
func (c *Connector) setHealth(status connector.HealthStatus) {
	c.mu.Lock()
	c.health = status
	c.mu.Unlock()
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	mapsCfg, err := parseMapsConfig(config)
	if err != nil {
		c.setHealth(connector.HealthError)
		return err
	}

	// Validate import directory exists and is readable.
	info, err := os.Stat(mapsCfg.ImportDir)
	if os.IsNotExist(err) {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("import directory does not exist: %s", mapsCfg.ImportDir)
	}
	if err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("import directory stat error: %w", err)
	}
	if !info.IsDir() {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("import directory is not a directory: %s", mapsCfg.ImportDir)
	}

	// Resolve symlinks to get canonical path, preventing the import directory
	// from being a symlink that could be retargeted between Connect and Sync.
	resolved, err := filepath.EvalSymlinks(mapsCfg.ImportDir)
	if err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("resolve import directory path: %w", err)
	}
	mapsCfg.ImportDir = resolved

	c.mu.Lock()
	c.config = mapsCfg
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	slog.Info("google maps timeline connector connected",
		"import_dir", mapsCfg.ImportDir,
		"archive_processed", mapsCfg.ArchiveProcessed,
		"min_distance_m", mapsCfg.MinDistanceM,
		"min_duration_min", mapsCfg.MinDurationMin,
	)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.setHealth(connector.HealthSyncing)

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

	processedFiles := parseCursor(cursor)

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

	const largeFileSizeBytes = 50 * 1024 * 1024 // 50MB warning threshold
	const maxFileSizeBytes = 200 * 1024 * 1024  // 200MB hard limit

	var allArtifacts []connector.RawArtifact
	var processedThisCycle []string
	syncErrors := 0
	trailCount := 0

	for _, file := range newFiles {
		// Check for context cancellation between files.
		if err := ctx.Err(); err != nil {
			slog.Warn("sync cancelled", "processed_so_far", len(processedThisCycle), "error", err)
			break
		}

		// Enforce file size limits.
		if info, statErr := os.Stat(file); statErr == nil {
			if info.Size() > maxFileSizeBytes {
				slog.Warn("skipping oversized takeout file",
					"file", filepath.Base(file),
					"size_mb", info.Size()/(1024*1024),
					"limit_mb", maxFileSizeBytes/(1024*1024))
				syncErrors++
				continue
			}
			if info.Size() > largeFileSizeBytes {
				slog.Warn("large takeout file detected",
					"file", filepath.Base(file),
					"size_mb", info.Size()/(1024*1024))
			}
		}

		data, err := os.ReadFile(file)
		if err != nil {
			slog.Warn("failed to read takeout file", "file", file, "error", err)
			syncErrors++
			continue
		}

		activities, err := ParseTakeoutJSON(data)
		if err != nil {
			slog.Warn("failed to parse takeout file", "file", file, "error", err)
			syncErrors++
			continue
		}

		filename := filepath.Base(file)
		fileCancelled := false
		for i, activity := range activities {
			// Check for context cancellation periodically within large files.
			if i > 0 && i%500 == 0 {
				if err := ctx.Err(); err != nil {
					slog.Warn("sync cancelled during activity processing",
						"file", filename, "processed", i, "total", len(activities), "error", err)
					fileCancelled = true
					break
				}
			}

			if activity.DistanceKm*1000 < c.config.MinDistanceM {
				continue
			}
			if activity.DurationMin < c.config.MinDurationMin {
				continue
			}

			artifact := NormalizeActivity(activity, filename)
			allArtifacts = append(allArtifacts, artifact)

			if IsTrailQualified(activity) {
				trailCount++
			}

			// Insert location cluster row for pattern detection.
			c.mu.RLock()
			pool := c.pool
			c.mu.RUnlock()
			if pool != nil {
				if err := InsertLocationCluster(ctx, pool, activity, artifact.SourceRef); err != nil {
					slog.Warn("failed to insert location cluster", "error", err)
				}
			}
		}

		// Only mark the file as processed if all activities were processed.
		// If context was cancelled mid-file, the file must be re-processed
		// on the next sync to avoid permanently losing unprocessed activities.
		if fileCancelled {
			break
		}

		processedThisCycle = append(processedThisCycle, filename)

		if c.config.ArchiveProcessed {
			if err := c.archiveFile(file); err != nil {
				slog.Warn("failed to archive processed file", "file", file, "error", err)
			}
		}
	}

	allProcessed := append(processedFiles, processedThisCycle...)
	pruned := c.pruneCursor(allProcessed)
	newCursor := encodeCursor(pruned)

	c.mu.Lock()
	c.lastSyncCount = len(allArtifacts)
	c.lastSyncErrors = syncErrors
	c.lastTrailCount = trailCount
	c.mu.Unlock()

	slog.Info("google maps timeline sync complete",
		"new_files", len(newFiles),
		"artifacts", len(allArtifacts),
		"trail_qualified", trailCount,
		"errors", syncErrors,
	)

	return allArtifacts, newCursor, nil
}

// PostSync runs pattern detection and temporal-spatial linking after a sync cycle.
// Returns any generated pattern/trip artifacts for pipeline publishing.
// Each step continues on failure — errors are logged but do not block subsequent steps.
func (c *Connector) PostSync(ctx context.Context, activities []TakeoutActivity) ([]connector.RawArtifact, error) {
	c.mu.RLock()
	pool := c.pool
	c.mu.RUnlock()

	if pool == nil {
		return nil, nil // no DB pool = skip pattern detection
	}

	pd := NewPatternDetector(pool, c.config)
	var allArtifacts []connector.RawArtifact

	commuteArtifacts, err := pd.DetectCommutes(ctx)
	if err != nil {
		slog.Warn("commute detection failed", "error", err)
	} else {
		allArtifacts = append(allArtifacts, commuteArtifacts...)
	}

	tripArtifacts, err := pd.DetectTrips(ctx)
	if err != nil {
		slog.Warn("trip detection failed", "error", err)
	} else {
		allArtifacts = append(allArtifacts, tripArtifacts...)
	}

	linkedCount, err := pd.LinkTemporalSpatial(ctx, activities)
	if err != nil {
		slog.Warn("temporal-spatial linking failed", "error", err)
	}

	slog.Info("post-sync patterns complete",
		"commute_patterns", len(commuteArtifacts),
		"trip_events", len(tripArtifacts),
		"links_created", linkedCount)
	return allArtifacts, nil
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
	slog.Info("google maps timeline connector closed")
	return nil
}

// SetPool sets the database connection pool for location_clusters insertion.
func (c *Connector) SetPool(pool *pgxpool.Pool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pool = pool
}

// InsertLocationCluster inserts a location cluster row for pattern detection.
func InsertLocationCluster(ctx context.Context, pool *pgxpool.Pool, activity TakeoutActivity, sourceRef string) error {
	startLat, startLng, endLat, endLng := activityGridCoords(activity)

	id := computeDedupHash(activity)
	dayOfWeek := int(activity.StartTime.Weekday())
	departureHour := activity.StartTime.Hour()
	activityDate := activity.StartTime.Format("2006-01-02")

	_, err := pool.Exec(ctx, `
		INSERT INTO location_clusters (
			id, source_ref,
			start_cluster_lat, start_cluster_lng,
			end_cluster_lat, end_cluster_lng,
			activity_type, activity_date,
			day_of_week, departure_hour,
			distance_km, duration_min
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO NOTHING`,
		id, sourceRef,
		startLat, startLng,
		endLat, endLng,
		string(activity.Type), activityDate,
		dayOfWeek, departureHour,
		activity.DistanceKm, activity.DurationMin,
	)
	if err != nil {
		return fmt.Errorf("insert location cluster: %w", err)
	}
	return nil
}

// findNewFiles scans the import directory for .json files not in the processed set.
func (c *Connector) findNewFiles(processedFiles []string) ([]string, error) {
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
			continue
		}

		// Skip symlinks to prevent reading files outside the import directory.
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}

		name := entry.Name()
		if strings.ToLower(filepath.Ext(name)) != ".json" {
			continue
		}

		if processed[name] {
			continue
		}

		newFiles = append(newFiles, filepath.Join(c.config.ImportDir, name))
	}

	sort.Strings(newFiles)
	return newFiles, nil
}

// archiveFile moves a processed file to the archive/ subdirectory.
func (c *Connector) archiveFile(filePath string) error {
	archiveDir := filepath.Join(c.config.ImportDir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}
	dest := filepath.Join(archiveDir, filepath.Base(filePath))
	if err := os.Rename(filePath, dest); err != nil {
		return fmt.Errorf("move file to archive: %w", err)
	}
	slog.Debug("archived processed file", "from", filePath, "to", dest)
	return nil
}

// parseCursor splits a pipe-delimited cursor into a list of processed filenames.
func parseCursor(cursor string) []string {
	if cursor == "" {
		return nil
	}
	return strings.Split(cursor, "|")
}

// encodeCursor joins processed filenames into a pipe-delimited cursor string.
func encodeCursor(files []string) string {
	return strings.Join(files, "|")
}

// pruneCursor removes cursor entries for files that no longer exist in the import directory.
// This prevents the cursor from growing unboundedly after files are archived or deleted.
func (c *Connector) pruneCursor(files []string) []string {
	if len(files) == 0 {
		return files
	}

	// Build a set of files currently present in the import directory.
	entries, err := os.ReadDir(c.config.ImportDir)
	if err != nil {
		// On read error, keep all entries to avoid losing track of processed files.
		return files
	}
	present := make(map[string]bool, len(entries))
	for _, e := range entries {
		present[e.Name()] = true
	}

	pruned := make([]string, 0, len(files))
	for _, f := range files {
		if present[f] {
			pruned = append(pruned, f)
		}
	}
	return pruned
}

// parseMapsConfig extracts Maps-specific fields from ConnectorConfig.SourceConfig.
func parseMapsConfig(config connector.ConnectorConfig) (MapsConfig, error) {
	cfg := MapsConfig{
		WatchInterval:         5 * time.Minute,
		ArchiveProcessed:      false,
		MinDistanceM:          100,
		MinDurationMin:        2,
		LocationRadiusM:       500,
		HomeDetection:         "frequency",
		CommuteMinOccurrences: 3,
		CommuteWindowDays:     14,
		CommuteWeekdaysOnly:   true,
		TripMinDistanceKm:     50,
		TripMinOvernightHours: 18,
		LinkTimeExtendMin:     30,
		LinkProximityRadiusM:  1000,
	}

	sc := config.SourceConfig

	// Import directory (required)
	if importDir, ok := sc["import_dir"].(string); ok && importDir != "" {
		cfg.ImportDir = importDir
	} else {
		return MapsConfig{}, fmt.Errorf("import directory is required")
	}

	// Watch interval
	if wi, ok := sc["watch_interval"].(string); ok && wi != "" {
		d, err := time.ParseDuration(wi)
		if err != nil {
			return MapsConfig{}, fmt.Errorf("invalid watch_interval %q: %w", wi, err)
		}
		cfg.WatchInterval = d
	}

	// Archive processed
	if ap, ok := sc["archive_processed"].(bool); ok {
		cfg.ArchiveProcessed = ap
	}

	// Min distance
	if md, ok := sc["min_distance_m"]; ok {
		switch v := md.(type) {
		case float64:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("min_distance_m must be non-negative, got %v", v)
			}
			cfg.MinDistanceM = v
		case int:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("min_distance_m must be non-negative, got %v", v)
			}
			cfg.MinDistanceM = float64(v)
		}
	}

	// Min duration
	if md, ok := sc["min_duration_min"]; ok {
		switch v := md.(type) {
		case float64:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("min_duration_min must be non-negative, got %v", v)
			}
			cfg.MinDurationMin = v
		case int:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("min_duration_min must be non-negative, got %v", v)
			}
			cfg.MinDurationMin = float64(v)
		}
	}

	// Clustering config
	if lr, ok := sc["location_radius_m"]; ok {
		switch v := lr.(type) {
		case float64:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("location_radius_m must be non-negative, got %v", v)
			}
			cfg.LocationRadiusM = v
		case int:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("location_radius_m must be non-negative, got %v", v)
			}
			cfg.LocationRadiusM = float64(v)
		}
	}
	if hd, ok := sc["home_detection"].(string); ok && hd != "" {
		cfg.HomeDetection = hd
	}

	// Commute config
	if cmo, ok := sc["commute_min_occurrences"]; ok {
		switch v := cmo.(type) {
		case float64:
			if v < 1 {
				return MapsConfig{}, fmt.Errorf("commute_min_occurrences must be >= 1, got %v", v)
			}
			cfg.CommuteMinOccurrences = int(v)
		case int:
			if v < 1 {
				return MapsConfig{}, fmt.Errorf("commute_min_occurrences must be >= 1, got %v", v)
			}
			cfg.CommuteMinOccurrences = v
		}
	}
	if cwd, ok := sc["commute_window_days"]; ok {
		switch v := cwd.(type) {
		case float64:
			if v < 1 {
				return MapsConfig{}, fmt.Errorf("commute_window_days must be >= 1, got %v", v)
			}
			cfg.CommuteWindowDays = int(v)
		case int:
			if v < 1 {
				return MapsConfig{}, fmt.Errorf("commute_window_days must be >= 1, got %v", v)
			}
			cfg.CommuteWindowDays = v
		}
	}
	if cwo, ok := sc["commute_weekdays_only"].(bool); ok {
		cfg.CommuteWeekdaysOnly = cwo
	}

	// Trip config
	if tmd, ok := sc["trip_min_distance_km"]; ok {
		switch v := tmd.(type) {
		case float64:
			if v <= 0 {
				return MapsConfig{}, fmt.Errorf("trip_min_distance_km must be positive, got %v", v)
			}
			cfg.TripMinDistanceKm = v
		case int:
			if v <= 0 {
				return MapsConfig{}, fmt.Errorf("trip_min_distance_km must be positive, got %v", v)
			}
			cfg.TripMinDistanceKm = float64(v)
		}
	}
	if tmo, ok := sc["trip_min_overnight_hours"]; ok {
		switch v := tmo.(type) {
		case float64:
			if v <= 0 {
				return MapsConfig{}, fmt.Errorf("trip_min_overnight_hours must be positive, got %v", v)
			}
			cfg.TripMinOvernightHours = v
		case int:
			if v <= 0 {
				return MapsConfig{}, fmt.Errorf("trip_min_overnight_hours must be positive, got %v", v)
			}
			cfg.TripMinOvernightHours = float64(v)
		}
	}

	// Link config
	if lte, ok := sc["link_time_extend_min"]; ok {
		switch v := lte.(type) {
		case float64:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("link_time_extend_min must be non-negative, got %v", v)
			}
			cfg.LinkTimeExtendMin = v
		case int:
			if v < 0 {
				return MapsConfig{}, fmt.Errorf("link_time_extend_min must be non-negative, got %v", v)
			}
			cfg.LinkTimeExtendMin = float64(v)
		}
	}
	if lpr, ok := sc["link_proximity_radius_m"]; ok {
		switch v := lpr.(type) {
		case float64:
			if v <= 0 {
				return MapsConfig{}, fmt.Errorf("link_proximity_radius_m must be positive, got %v", v)
			}
			cfg.LinkProximityRadiusM = v
		case int:
			if v <= 0 {
				return MapsConfig{}, fmt.Errorf("link_proximity_radius_m must be positive, got %v", v)
			}
			cfg.LinkProximityRadiusM = float64(v)
		}
	}

	return cfg, nil
}
