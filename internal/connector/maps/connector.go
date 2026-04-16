package maps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
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

	// Snapshot config under RLock to prevent data races with concurrent Connect().
	c.mu.RLock()
	cfg := c.config
	c.mu.RUnlock()

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

	newFiles, err := findNewFiles(cfg.ImportDir, processedFiles)
	if err != nil {
		c.mu.Lock()
		c.lastSyncCount = 0
		c.lastSyncErrors = 1
		c.lastTrailCount = 0
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
		artifactCapReached := false
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

			// Enforce cross-file artifact cap to prevent unbounded memory growth.
			if len(allArtifacts) >= maxActivities {
				slog.Warn("cross-file artifact cap reached, stopping activity processing",
					"cap", maxActivities, "file", filename, "activity_index", i)
				artifactCapReached = true
				break
			}

			if activity.DistanceKm*1000 < cfg.MinDistanceM {
				continue
			}
			if activity.DurationMin < cfg.MinDurationMin {
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
		// If context was cancelled mid-file or the artifact cap was reached,
		// the file must be re-processed on the next sync to avoid permanently
		// losing unprocessed activities.
		if fileCancelled {
			break
		}
		if artifactCapReached {
			slog.Warn("artifact cap reached, halting sync cycle",
				"total_artifacts", len(allArtifacts), "cap", maxActivities)
			break
		}

		processedThisCycle = append(processedThisCycle, filename)

		if cfg.ArchiveProcessed {
			if err := archiveFile(file, cfg.ImportDir); err != nil {
				slog.Warn("failed to archive processed file", "file", file, "error", err)
			}
		}
	}

	allProcessed := append(processedFiles, processedThisCycle...)
	pruned := pruneCursor(cfg.ImportDir, allProcessed)
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
// Each step continues on failure — errors are logged and aggregated.
func (c *Connector) PostSync(ctx context.Context, activities []TakeoutActivity) ([]connector.RawArtifact, error) {
	c.mu.RLock()
	pool := c.pool
	cfg := c.config
	c.mu.RUnlock()

	if pool == nil {
		return nil, nil // no DB pool = skip pattern detection
	}

	pd := NewPatternDetector(pool, cfg)
	var allArtifacts []connector.RawArtifact
	var commuteCount, tripCount int
	var errs []error

	// R18-S3: Fetch clusters once and reuse for both commute and trip detection.
	clusters, err := pd.queryRecentClusters(ctx)
	if err != nil {
		slog.Warn("cluster query failed, skipping commute+trip detection", "error", err)
		errs = append(errs, fmt.Errorf("query recent clusters: %w", err))
	} else {
		commutePatterns := classifyCommutes(clusters, cfg)
		commuteArtifacts := normalizeCommutePatterns(commutePatterns)
		allArtifacts = append(allArtifacts, commuteArtifacts...)
		commuteCount = len(commuteArtifacts)

		home, err := pd.InferHome(ctx)
		if err != nil {
			slog.Warn("home inference failed", "error", err)
			errs = append(errs, fmt.Errorf("infer home: %w", err))
		} else if home != nil {
			trips := classifyTrips(clusters, *home, cfg)
			tripArtifacts := normalizeTripEvents(trips)
			allArtifacts = append(allArtifacts, tripArtifacts...)
			tripCount = len(tripArtifacts)
		} else {
			slog.Info("no home location inferred, skipping trip detection")
		}
	}

	linkedCount, err := pd.LinkTemporalSpatial(ctx, activities)
	if err != nil {
		slog.Warn("temporal-spatial linking failed", "error", err)
		errs = append(errs, fmt.Errorf("temporal-spatial linking: %w", err))
	}

	slog.Info("post-sync patterns complete",
		"commute_patterns", commuteCount,
		"trip_events", tripCount,
		"links_created", linkedCount)
	return allArtifacts, errors.Join(errs...)
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
// sourceRef is the dedup hash already computed by NormalizeActivity and doubles as the cluster primary key.
func InsertLocationCluster(ctx context.Context, pool *pgxpool.Pool, activity TakeoutActivity, sourceRef string) error {
	startLat, startLng, endLat, endLng := activityGridCoords(activity)

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
		sourceRef, sourceRef,
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
func findNewFiles(importDir string, processedFiles []string) ([]string, error) {
	processed := make(map[string]bool, len(processedFiles))
	for _, f := range processedFiles {
		processed[f] = true
	}

	entries, err := os.ReadDir(importDir)
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

		// Skip files with pipe character — the cursor uses "|" as delimiter
		// and a pipe in the filename would corrupt cursor encoding.
		if strings.Contains(name, "|") {
			slog.Warn("skipping file with pipe character in name (incompatible with cursor encoding)",
				"file", name)
			continue
		}

		if processed[name] {
			continue
		}

		newFiles = append(newFiles, filepath.Join(importDir, name))
	}

	sort.Strings(newFiles)
	return newFiles, nil
}

// archiveFile moves a processed file to the archive/ subdirectory.
// If a file with the same name already exists in the archive, a numeric
// suffix is appended to prevent silent data loss from overwriting.
// importDir is passed explicitly to avoid reading c.config without lock.
func archiveFile(filePath string, importDir string) error {
	archiveDir := filepath.Join(importDir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}
	baseName := filepath.Base(filePath)
	dest := filepath.Join(archiveDir, baseName)

	// If target already exists, add a numeric suffix to avoid overwriting.
	const maxArchiveCollisions = 1000
	if _, err := os.Stat(dest); err == nil {
		ext := filepath.Ext(baseName)
		prefix := strings.TrimSuffix(baseName, ext)
		for i := 1; i <= maxArchiveCollisions; i++ {
			candidate := filepath.Join(archiveDir, fmt.Sprintf("%s_%d%s", prefix, i, ext))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				dest = candidate
				break
			}
			if i == maxArchiveCollisions {
				return fmt.Errorf("archive collision limit exceeded (%d) for %s", maxArchiveCollisions, baseName)
			}
		}
		slog.Info("archive collision detected, using alternate name",
			"original", baseName, "dest", filepath.Base(dest))
	}

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
func pruneCursor(importDir string, files []string) []string {
	if len(files) == 0 {
		return files
	}

	// Build a set of files currently present in the import directory.
	entries, err := os.ReadDir(importDir)
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
// SST: All config values must be provided via smackerel.yaml → env → SourceConfig.
// No hardcoded Go-side fallback defaults; missing required fields fail loud.
func parseMapsConfig(config connector.ConnectorConfig) (MapsConfig, error) {
	var cfg MapsConfig
	sc := config.SourceConfig
	var missing []string

	// Import directory (required — fail immediately)
	if importDir, ok := sc["import_dir"].(string); ok && importDir != "" {
		cfg.ImportDir = importDir
	} else {
		return MapsConfig{}, fmt.Errorf("import directory is required")
	}

	// Watch interval (required)
	if wi, ok := sc["watch_interval"].(string); ok && wi != "" {
		d, err := time.ParseDuration(wi)
		if err != nil {
			return MapsConfig{}, fmt.Errorf("invalid watch_interval %q: %w", wi, err)
		}
		cfg.WatchInterval = d
	} else {
		missing = append(missing, "watch_interval")
	}

	// Archive processed (optional boolean, zero-value false is safe)
	if ap, ok := sc["archive_processed"].(bool); ok {
		cfg.ArchiveProcessed = ap
	}

	// Min distance (required)
	if v, err := configFloat64NonNeg(sc, "min_distance_m"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.MinDistanceM = v
	} else {
		missing = append(missing, "min_distance_m")
	}

	// Min duration (required)
	if v, err := configFloat64NonNeg(sc, "min_duration_min"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.MinDurationMin = v
	} else {
		missing = append(missing, "min_duration_min")
	}

	// Clustering config (required)
	if v, err := configFloat64NonNeg(sc, "location_radius_m"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.LocationRadiusM = v
	} else {
		missing = append(missing, "location_radius_m")
	}
	if hd, ok := sc["home_detection"].(string); ok && hd != "" {
		cfg.HomeDetection = hd
	} else {
		missing = append(missing, "home_detection")
	}

	// Commute config (required)
	if v, err := configIntMin(sc, "commute_min_occurrences", 1); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.CommuteMinOccurrences = v
	} else {
		missing = append(missing, "commute_min_occurrences")
	}
	if v, err := configIntMin(sc, "commute_window_days", 1); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.CommuteWindowDays = v
	} else {
		missing = append(missing, "commute_window_days")
	}
	// Commute weekdays-only (optional boolean, zero-value false is safe)
	if cwo, ok := sc["commute_weekdays_only"].(bool); ok {
		cfg.CommuteWeekdaysOnly = cwo
	}

	// Trip config (required)
	if v, err := configFloat64Positive(sc, "trip_min_distance_km"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.TripMinDistanceKm = v
	} else {
		missing = append(missing, "trip_min_distance_km")
	}
	if v, err := configFloat64Positive(sc, "trip_min_overnight_hours"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.TripMinOvernightHours = v
	} else {
		missing = append(missing, "trip_min_overnight_hours")
	}

	// Link config (required)
	if v, err := configFloat64NonNeg(sc, "link_time_extend_min"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.LinkTimeExtendMin = v
	} else {
		missing = append(missing, "link_time_extend_min")
	}
	if v, err := configFloat64Positive(sc, "link_proximity_radius_m"); err != nil {
		return MapsConfig{}, err
	} else if v >= 0 {
		cfg.LinkProximityRadiusM = v
	} else {
		missing = append(missing, "link_proximity_radius_m")
	}

	// SST: fail-loud if any required config values were not provided in smackerel.yaml
	if len(missing) > 0 {
		return MapsConfig{}, fmt.Errorf("required google-maps-timeline config values missing: %s", strings.Join(missing, ", "))
	}

	return cfg, nil
}

// configFloat64NonNeg extracts a non-negative float64 from a source config map.
// Returns (-1, nil) if the key is absent, (value, nil) on success, or an error on invalid input.
func configFloat64NonNeg(sc map[string]interface{}, key string) (float64, error) {
	val, ok := sc[key]
	if !ok {
		return -1, nil
	}
	switch v := val.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("%s must be a finite number, got %v", key, v)
		}
		if v < 0 {
			return 0, fmt.Errorf("%s must be non-negative, got %v", key, v)
		}
		return v, nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("%s must be non-negative, got %v", key, v)
		}
		return float64(v), nil
	case string:
		return 0, fmt.Errorf("%s has unsupported type string (got %q); use a numeric value", key, v)
	default:
		return -1, nil
	}
}

// configFloat64Positive extracts a positive (>0) float64 from a source config map.
// Returns (-1, nil) if the key is absent, (value, nil) on success, or an error on invalid input.
func configFloat64Positive(sc map[string]interface{}, key string) (float64, error) {
	val, ok := sc[key]
	if !ok {
		return -1, nil
	}
	switch v := val.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("%s must be a finite number, got %v", key, v)
		}
		if v <= 0 {
			return 0, fmt.Errorf("%s must be positive, got %v", key, v)
		}
		return v, nil
	case int:
		if v <= 0 {
			return 0, fmt.Errorf("%s must be positive, got %v", key, v)
		}
		return float64(v), nil
	case string:
		return 0, fmt.Errorf("%s has unsupported type string (got %q); use a numeric value", key, v)
	default:
		return -1, nil
	}
}

// configIntMin extracts an int >= min from a source config map.
// Returns (-1, nil) if the key is absent, (value, nil) on success, or an error on invalid input.
func configIntMin(sc map[string]interface{}, key string, min int) (int, error) {
	val, ok := sc[key]
	if !ok {
		return -1, nil
	}
	switch v := val.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, fmt.Errorf("%s must be a finite number, got %v", key, v)
		}
		if v > 1e9 || v < -1e9 {
			return 0, fmt.Errorf("%s value out of safe integer range, got %v", key, v)
		}
		if int(v) < min {
			return 0, fmt.Errorf("%s must be >= %d, got %v", key, min, v)
		}
		return int(v), nil
	case int:
		if v < min {
			return 0, fmt.Errorf("%s must be >= %d, got %v", key, min, v)
		}
		return v, nil
	case string:
		return 0, fmt.Errorf("%s has unsupported type string (got %q); use a numeric value", key, v)
	default:
		return -1, nil
	}
}
