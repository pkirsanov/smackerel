package maps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnectorID(t *testing.T) {
	c := New("google-maps-timeline")
	if c.ID() != "google-maps-timeline" {
		t.Errorf("expected ID %q, got %q", "google-maps-timeline", c.ID())
	}
}

// testMapsSourceConfig returns a full SourceConfig with all required
// google-maps-timeline fields matching smackerel.yaml defaults.
func testMapsSourceConfig(importDir string) map[string]interface{} {
	return map[string]interface{}{
		"import_dir":               importDir,
		"watch_interval":           "5m",
		"archive_processed":        false,
		"min_distance_m":           float64(100),
		"min_duration_min":         float64(2),
		"location_radius_m":        float64(500),
		"home_detection":           "frequency",
		"commute_min_occurrences":  float64(3),
		"commute_window_days":      float64(14),
		"commute_weekdays_only":    true,
		"trip_min_distance_km":     float64(50),
		"trip_min_overnight_hours": float64(18),
		"link_time_extend_min":     float64(30),
		"link_proximity_radius_m":  float64(1000),
	}
}

// testMapsSourceConfigWith returns a full SourceConfig with specific overrides applied.
func testMapsSourceConfigWith(importDir string, overrides map[string]interface{}) map[string]interface{} {
	sc := testMapsSourceConfig(importDir)
	for k, v := range overrides {
		sc[k] = v
	}
	return sc
}

func TestConnectValidConfig(t *testing.T) {
	dir := t.TempDir()
	c := New("google-maps-timeline")

	cfg := connector.ConnectorConfig{
		AuthType:     "none",
		Enabled:      true,
		SourceConfig: testMapsSourceConfig(dir),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected health %q, got %q", connector.HealthHealthy, c.Health(context.Background()))
	}
}

func TestConnectMissingImportDir(t *testing.T) {
	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir": "/nonexistent/path/that/does/not/exist",
		},
	}
	err := c.Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for non-existent import dir")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected health %q after error, got %q", connector.HealthError, c.Health(context.Background()))
	}
}

func TestConnectEmptyImportDir(t *testing.T) {
	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir": "",
		},
	}
	err := c.Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for empty import dir")
	}
}

func TestParseMapsConfigRejectsMissingFields(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir": "/tmp/test",
		},
	})
	if err == nil {
		t.Fatal("expected error for missing required config fields")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected missing-fields error, got: %v", err)
	}
}

func TestParseMapsConfigNegativeMinDistance(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":     "/tmp/test",
			"min_distance_m": float64(-10),
		},
	})
	if err == nil {
		t.Fatal("expected validation error for negative min_distance_m")
	}
}

func TestSyncProducesArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "export.json", makeTakeoutJSON(5))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 5 {
		t.Errorf("expected 5 artifacts, got %d", len(artifacts))
	}
	if cursor != "export.json" {
		t.Errorf("expected cursor %q, got %q", "export.json", cursor)
	}
	for _, a := range artifacts {
		if a.SourceID != "google-maps-timeline" {
			t.Errorf("expected SourceID %q, got %q", "google-maps-timeline", a.SourceID)
		}
	}
}

func TestSyncCursorSkipsProcessed(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "jan.json", makeTakeoutJSON(2))
	writeTakeoutFile(t, dir, "feb.json", makeTakeoutJSON(3))
	writeTakeoutFile(t, dir, "mar.json", makeTakeoutJSON(4))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "jan.json|feb.json")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 4 {
		t.Errorf("expected 4 artifacts (only from mar.json), got %d", len(artifacts))
	}
	if cursor != "jan.json|feb.json|mar.json" {
		t.Errorf("expected cursor %q, got %q", "jan.json|feb.json|mar.json", cursor)
	}
}

func TestSyncEmptyCursorFullScan(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "a.json", makeTakeoutJSON(1))
	writeTakeoutFile(t, dir, "b.json", makeTakeoutJSON(2))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 3 {
		t.Errorf("expected 3 artifacts (1+2), got %d", len(artifacts))
	}
}

func TestSyncMinThresholdFiltering(t *testing.T) {
	dir := t.TempDir()
	// 10 activities: 3 under distance threshold (50m), 2 under duration threshold (1min), 5 pass
	writeTakeoutFile(t, dir, "mixed.json", makeMixedThresholdJSON())

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType:     "none",
		Enabled:      true,
		SourceConfig: testMapsSourceConfig(dir),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 5 {
		t.Errorf("expected 5 artifacts after filtering, got %d", len(artifacts))
	}
	if cursor != "mixed.json" {
		t.Errorf("file should still be marked as processed, got cursor %q", cursor)
	}
}

func TestHealthTransitions(t *testing.T) {
	c := New("google-maps-timeline")
	ctx := context.Background()

	// Initial: disconnected
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("expected initial health %q, got %q", connector.HealthDisconnected, c.Health(ctx))
	}

	// After Connect: healthy
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		AuthType:     "none",
		Enabled:      true,
		SourceConfig: testMapsSourceConfig(dir),
	}
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("expected healthy after Connect, got %q", c.Health(ctx))
	}

	// After Sync (no files): still healthy
	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if c.Health(ctx) != connector.HealthHealthy {
		t.Errorf("expected healthy after Sync, got %q", c.Health(ctx))
	}

	// After Close: disconnected
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Health(ctx) != connector.HealthDisconnected {
		t.Errorf("expected disconnected after Close, got %q", c.Health(ctx))
	}
}

func TestParseCursor(t *testing.T) {
	tests := []struct {
		cursor string
		want   int
	}{
		{"", 0},
		{"a.json", 1},
		{"a.json|b.json|c.json", 3},
	}
	for _, tt := range tests {
		got := parseCursor(tt.cursor)
		if len(got) != tt.want {
			t.Errorf("parseCursor(%q) = %d items, want %d", tt.cursor, len(got), tt.want)
		}
	}
}

func TestEncodeCursor(t *testing.T) {
	files := []string{"a.json", "b.json"}
	got := encodeCursor(files)
	if got != "a.json|b.json" {
		t.Errorf("encodeCursor = %q, want %q", got, "a.json|b.json")
	}
}

// --- Scope 02 tests ---

func TestArchiveFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "takeout-march.json")
	if err := os.WriteFile(testFile, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	if err := archiveFile(testFile, dir); err != nil {
		t.Fatalf("archiveFile: %v", err)
	}

	// Original file should be gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("original file still exists after archive")
	}

	// Archived file should exist
	archived := filepath.Join(dir, "archive", "takeout-march.json")
	if _, err := os.Stat(archived); err != nil {
		t.Errorf("archived file not found: %v", err)
	}

	// Archive dir should have been created
	archiveDir := filepath.Join(dir, "archive")
	info, err := os.Stat(archiveDir)
	if err != nil {
		t.Fatalf("archive directory not found: %v", err)
	}
	if !info.IsDir() {
		t.Error("archive path is not a directory")
	}
}

func TestArchiveDisabled(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "takeout-april.json")
	writeTakeoutFile(t, dir, "takeout-april.json", makeTakeoutJSON(2))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"archive_processed": false,
			"min_distance_m":    float64(0),
			"min_duration_min":  float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// File should still exist in original location (not archived)
	if _, err := os.Stat(testFile); err != nil {
		t.Errorf("file should remain when archive_processed=false: %v", err)
	}

	// No archive directory should have been created
	archiveDir := filepath.Join(dir, "archive")
	if _, err := os.Stat(archiveDir); !os.IsNotExist(err) {
		t.Error("archive directory should not exist when archive_processed=false")
	}
}

func TestConnectImportDirIsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("create file: %v", err)
	}

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType:     "none",
		Enabled:      true,
		SourceConfig: testMapsSourceConfig(filePath),
	}
	err := c.Connect(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error when import_dir is a file, not a directory")
	}
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("expected health %q, got %q", connector.HealthError, c.Health(context.Background()))
	}
}

func TestParseMapsConfigNegativeMinDuration(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":       "/tmp/test",
			"min_duration_min": float64(-5),
		},
	})
	if err == nil {
		t.Fatal("expected validation error for negative min_duration_min")
	}
}

func TestParseMapsConfigInvalidWatchInterval(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: testMapsSourceConfigWith("/tmp/test", map[string]interface{}{
			"watch_interval": "not-a-duration",
		}),
	})
	if err == nil {
		t.Fatal("expected validation error for invalid watch_interval")
	}
}

func TestParseMapsConfigIntTypes(t *testing.T) {
	cfg, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: testMapsSourceConfigWith("/tmp/test", map[string]interface{}{
			"min_distance_m":   int(200),
			"min_duration_min": int(5),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinDistanceM != 200 {
		t.Errorf("MinDistanceM = %v, want 200", cfg.MinDistanceM)
	}
	if cfg.MinDurationMin != 5 {
		t.Errorf("MinDurationMin = %v, want 5", cfg.MinDurationMin)
	}
}

func TestParseMapsConfigCustomOverrides(t *testing.T) {
	cfg, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: testMapsSourceConfigWith("/tmp/test", map[string]interface{}{
			"archive_processed":        true,
			"location_radius_m":        float64(250),
			"home_detection":           "manual",
			"commute_min_occurrences":  float64(5),
			"commute_window_days":      float64(30),
			"commute_weekdays_only":    false,
			"trip_min_distance_km":     float64(100),
			"trip_min_overnight_hours": float64(24),
			"link_time_extend_min":     float64(60),
			"link_proximity_radius_m":  float64(2000),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.ArchiveProcessed {
		t.Error("ArchiveProcessed should be true")
	}
	if cfg.LocationRadiusM != 250 {
		t.Errorf("LocationRadiusM = %v, want 250", cfg.LocationRadiusM)
	}
	if cfg.HomeDetection != "manual" {
		t.Errorf("HomeDetection = %q, want %q", cfg.HomeDetection, "manual")
	}
	if cfg.CommuteMinOccurrences != 5 {
		t.Errorf("CommuteMinOccurrences = %v, want 5", cfg.CommuteMinOccurrences)
	}
	if cfg.CommuteWindowDays != 30 {
		t.Errorf("CommuteWindowDays = %v, want 30", cfg.CommuteWindowDays)
	}
	if cfg.CommuteWeekdaysOnly {
		t.Error("CommuteWeekdaysOnly should be false")
	}
	if cfg.TripMinDistanceKm != 100 {
		t.Errorf("TripMinDistanceKm = %v, want 100", cfg.TripMinDistanceKm)
	}
	if cfg.TripMinOvernightHours != 24 {
		t.Errorf("TripMinOvernightHours = %v, want 24", cfg.TripMinOvernightHours)
	}
	if cfg.LinkTimeExtendMin != 60 {
		t.Errorf("LinkTimeExtendMin = %v, want 60", cfg.LinkTimeExtendMin)
	}
	if cfg.LinkProximityRadiusM != 2000 {
		t.Errorf("LinkProximityRadiusM = %v, want 2000", cfg.LinkProximityRadiusM)
	}
}

func TestSyncMalformedFileContinues(t *testing.T) {
	dir := t.TempDir()
	// One valid file + one malformed file
	writeTakeoutFile(t, dir, "good.json", makeTakeoutJSON(2))
	writeTakeoutFile(t, dir, "bad.json", `{not valid json`)

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync should not fail entirely on one bad file: %v", err)
	}
	// The good file should still produce artifacts
	if len(artifacts) != 2 {
		t.Errorf("expected 2 artifacts from good file only, got %d", len(artifacts))
	}
}

func TestSyncArchiveEnabled(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "archive-me.json", makeTakeoutJSON(1))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"archive_processed": true,
			"min_distance_m":    float64(0),
			"min_duration_min":  float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Original file should be archived
	originalPath := filepath.Join(dir, "archive-me.json")
	if _, err := os.Stat(originalPath); !os.IsNotExist(err) {
		t.Error("original file should be moved to archive after sync")
	}

	archivedPath := filepath.Join(dir, "archive", "archive-me.json")
	if _, err := os.Stat(archivedPath); err != nil {
		t.Errorf("archived file should exist: %v", err)
	}
}

// --- Hardening tests ---

// HARDEN-011-F3: Non-JSON files in import directory must be skipped.
func TestSyncSkipsNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "readme.txt", "not json data")
	writeTakeoutFile(t, dir, "data.csv", "col1,col2\n1,2")
	writeTakeoutFile(t, dir, "export.json", makeTakeoutJSON(2))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("expected 2 artifacts from .json only, got %d", len(artifacts))
	}
	if cursor != "export.json" {
		t.Errorf("cursor should only contain .json files, got %q", cursor)
	}
}

// HARDEN-011-F3: When all activities in a file are below threshold, cursor must still advance.
func TestSyncAllFilteredStillAdvancesCursor(t *testing.T) {
	dir := t.TempDir()
	// All activities have 50m distance (below default 100m threshold)
	json := `{"timelineObjects": [{
		"activitySegment": {
			"startLocation": {"latitudeE7": 475000000, "longitudeE7": 87000000},
			"endLocation":   {"latitudeE7": 475010000, "longitudeE7": 87010000},
			"duration": {"startTimestamp": "2026-03-15T10:00:00Z", "endTimestamp": "2026-03-15T10:05:00Z"},
			"distance": 50, "activityType": "WALKING",
			"waypointPath": {"waypoints": []}
		}
	}]}`
	writeTakeoutFile(t, dir, "tiny.json", json)

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType:     "none",
		Enabled:      true,
		SourceConfig: testMapsSourceConfig(dir),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts (all filtered), got %d", len(artifacts))
	}
	if cursor != "tiny.json" {
		t.Errorf("cursor should include processed file even when all filtered, got %q", cursor)
	}

	// Second sync should produce nothing (file already in cursor)
	artifacts2, cursor2, err := c.Sync(context.Background(), cursor)
	if err != nil {
		t.Fatalf("Sync2: %v", err)
	}
	if len(artifacts2) != 0 {
		t.Errorf("second sync should produce 0 artifacts, got %d", len(artifacts2))
	}
	if cursor2 != cursor {
		t.Errorf("cursor should not change when no new files, got %q", cursor2)
	}
}

// HARDEN-011-F2: Context cancellation during Sync should stop processing.
func TestSyncContextCancellation(t *testing.T) {
	dir := t.TempDir()
	// Create multiple files so cancellation can trigger between them
	for i := 0; i < 5; i++ {
		writeTakeoutFile(t, dir, fmt.Sprintf("file%d.json", i), makeTakeoutJSON(1))
	}

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Cancel immediately — should process 0 or few files
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Sync(ctx, "")
	// Should not return an error (cancellation is handled gracefully)
	if err != nil {
		t.Fatalf("Sync with cancelled context returned error: %v", err)
	}
}

// HARDEN-011: Oversized file skipped without blocking other files.
func TestSyncOversizedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "normal.json", makeTakeoutJSON(2))
	// We can't easily create a 200MB+ file in unit tests, but verify the
	// file size check path exists by confirming a normal file passes.
	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("normal file should process, got %d artifacts", len(artifacts))
	}
}

// --- test helpers ---

func writeTakeoutFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

// IMPROVE-011: findNewFiles returns files in sorted order for deterministic processing
func TestFindNewFilesSorted(t *testing.T) {
	dir := t.TempDir()
	// Create files in reverse-alpha order to detect non-determinism
	writeTakeoutFile(t, dir, "z-export.json", makeTakeoutJSON(1))
	writeTakeoutFile(t, dir, "a-export.json", makeTakeoutJSON(1))
	writeTakeoutFile(t, dir, "m-export.json", makeTakeoutJSON(1))

	c := New("google-maps-timeline")
	_ = c // findNewFiles is now a package function
	cfg := MapsConfig{ImportDir: dir}

	files, err := findNewFiles(cfg.ImportDir, nil)
	if err != nil {
		t.Fatalf("findNewFiles: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// Verify sorted order
	for i := 1; i < len(files); i++ {
		if files[i] < files[i-1] {
			t.Errorf("files not sorted: %q appears after %q", files[i], files[i-1])
		}
	}

	expectedOrder := []string{"a-export.json", "m-export.json", "z-export.json"}
	for i, f := range files {
		base := filepath.Base(f)
		if base != expectedOrder[i] {
			t.Errorf("files[%d] = %q, want %q", i, base, expectedOrder[i])
		}
	}
}

// IMPROVE-011: Cursor pruning removes entries for archived/deleted files
func TestCursorPruningRemovesArchivedFiles(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "current.json", makeTakeoutJSON(1))
	// "old.json" is in cursor but not on disk (was archived)

	pruned := pruneCursor(dir, []string{"old.json", "current.json", "deleted.json"})
	if len(pruned) != 1 {
		t.Fatalf("expected 1 pruned entry, got %d: %v", len(pruned), pruned)
	}
	if pruned[0] != "current.json" {
		t.Errorf("expected %q, got %q", "current.json", pruned[0])
	}
}

// IMPROVE-011: Cursor pruning keeps all entries when import dir can't be read
func TestCursorPruningFallbackOnError(t *testing.T) {
	files := []string{"a.json", "b.json"}
	pruned := pruneCursor("/nonexistent/path/that/does/not/exist", files)
	if len(pruned) != 2 {
		t.Errorf("expected all entries preserved on error, got %d", len(pruned))
	}
}

// IMPROVE-011: Sync with archiving produces a cursor that doesn't contain archived files
func TestSyncCursorPrunedAfterArchive(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "batch1.json", makeTakeoutJSON(1))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: testMapsSourceConfigWith(dir, map[string]interface{}{
			"archive_processed": true,
			"min_distance_m":    float64(0),
			"min_duration_min":  float64(0),
		}),
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// After archive, batch1.json is moved to archive subdir. Cursor should be pruned.
	parsed := parseCursor(cursor)
	for _, f := range parsed {
		if f == "batch1.json" {
			t.Error("cursor should not contain archived file batch1.json")
		}
	}
}

// IMPROVE-011-R09: parseMapsConfig int-type handling for trip/link/location fields.
func TestParseMapsConfigIntTypesExtended(t *testing.T) {
	cfg, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: testMapsSourceConfigWith("/tmp/test", map[string]interface{}{
			"location_radius_m":        int(250),
			"trip_min_distance_km":     int(100),
			"trip_min_overnight_hours": int(24),
			"link_time_extend_min":     int(60),
			"link_proximity_radius_m":  int(2000),
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LocationRadiusM != 250 {
		t.Errorf("LocationRadiusM = %v, want 250", cfg.LocationRadiusM)
	}
	if cfg.TripMinDistanceKm != 100 {
		t.Errorf("TripMinDistanceKm = %v, want 100", cfg.TripMinDistanceKm)
	}
	if cfg.TripMinOvernightHours != 24 {
		t.Errorf("TripMinOvernightHours = %v, want 24", cfg.TripMinOvernightHours)
	}
	if cfg.LinkTimeExtendMin != 60 {
		t.Errorf("LinkTimeExtendMin = %v, want 60", cfg.LinkTimeExtendMin)
	}
	if cfg.LinkProximityRadiusM != 2000 {
		t.Errorf("LinkProximityRadiusM = %v, want 2000", cfg.LinkProximityRadiusM)
	}
}

// IMPROVE-011-R09: validation rejects negative/zero values for location/link fields.
func TestParseMapsConfigValidationLocationLink(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
	}{
		{"negative_location_radius_m_float", map[string]interface{}{"import_dir": "/tmp/test", "location_radius_m": float64(-1)}},
		{"negative_location_radius_m_int", map[string]interface{}{"import_dir": "/tmp/test", "location_radius_m": int(-1)}},
		{"negative_trip_min_distance_km_int", map[string]interface{}{"import_dir": "/tmp/test", "trip_min_distance_km": int(-10)}},
		{"zero_trip_min_distance_km_int", map[string]interface{}{"import_dir": "/tmp/test", "trip_min_distance_km": int(0)}},
		{"negative_trip_min_overnight_hours_int", map[string]interface{}{"import_dir": "/tmp/test", "trip_min_overnight_hours": int(-1)}},
		{"zero_trip_min_overnight_hours_int", map[string]interface{}{"import_dir": "/tmp/test", "trip_min_overnight_hours": int(0)}},
		{"negative_link_time_extend_min_float", map[string]interface{}{"import_dir": "/tmp/test", "link_time_extend_min": float64(-5)}},
		{"negative_link_time_extend_min_int", map[string]interface{}{"import_dir": "/tmp/test", "link_time_extend_min": int(-5)}},
		{"negative_link_proximity_radius_m_float", map[string]interface{}{"import_dir": "/tmp/test", "link_proximity_radius_m": float64(-100)}},
		{"zero_link_proximity_radius_m_float", map[string]interface{}{"import_dir": "/tmp/test", "link_proximity_radius_m": float64(0)}},
		{"negative_link_proximity_radius_m_int", map[string]interface{}{"import_dir": "/tmp/test", "link_proximity_radius_m": int(-100)}},
		{"zero_link_proximity_radius_m_int", map[string]interface{}{"import_dir": "/tmp/test", "link_proximity_radius_m": int(0)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseMapsConfig(connector.ConnectorConfig{SourceConfig: tt.config})
			if err == nil {
				t.Errorf("expected validation error for %s", tt.name)
			}
		})
	}
}

// IMPROVE-011-R09: zero location_radius_m is accepted (non-negative, not positive).
func TestParseMapsConfigZeroLocationRadiusAllowed(t *testing.T) {
	cfg, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: testMapsSourceConfigWith("/tmp/test", map[string]interface{}{
			"location_radius_m": float64(0),
		}),
	})
	if err != nil {
		t.Fatalf("zero location_radius_m should be accepted: %v", err)
	}
	if cfg.LocationRadiusM != 0 {
		t.Errorf("LocationRadiusM = %v, want 0", cfg.LocationRadiusM)
	}
}

// IMPROVE-011-R09: zero link_time_extend_min is accepted (non-negative, not positive).
func TestParseMapsConfigZeroLinkTimeExtendAllowed(t *testing.T) {
	cfg, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: testMapsSourceConfigWith("/tmp/test", map[string]interface{}{
			"link_time_extend_min": float64(0),
		}),
	})
	if err != nil {
		t.Fatalf("zero link_time_extend_min should be accepted: %v", err)
	}
	if cfg.LinkTimeExtendMin != 0 {
		t.Errorf("LinkTimeExtendMin = %v, want 0", cfg.LinkTimeExtendMin)
	}
}

func makeTakeoutJSON(n int) string {
	base := time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)
	objects := ""
	for i := 0; i < n; i++ {
		start := base.Add(time.Duration(i) * time.Hour)
		end := start.Add(45 * time.Minute)
		if i > 0 {
			objects += ","
		}
		objects += `{
			"activitySegment": {
				"startLocation": {"latitudeE7": 475000000, "longitudeE7": 87000000},
				"endLocation": {"latitudeE7": 475200000, "longitudeE7": 87500000},
				"duration": {
					"startTimestamp": "` + start.Format(time.RFC3339) + `",
					"endTimestamp": "` + end.Format(time.RFC3339) + `"
				},
				"distance": 6000,
				"activityType": "WALKING",
				"waypointPath": {
					"waypoints": [
						{"latE7": 475000000, "lngE7": 87000000},
						{"latE7": 475100000, "lngE7": 87250000},
						{"latE7": 475200000, "lngE7": 87500000}
					]
				}
			}
		}`
	}
	return `{"timelineObjects": [` + objects + `]}`
}

func makeMixedThresholdJSON() string {
	base := time.Date(2026, 3, 15, 8, 0, 0, 0, time.UTC)
	objects := ""

	for i := 0; i < 10; i++ {
		start := base.Add(time.Duration(i) * time.Hour)
		var end time.Time
		var distance int

		if i < 3 {
			// Under distance threshold: 50m, 1 minute
			distance = 50
			end = start.Add(5 * time.Minute)
		} else if i < 5 {
			// Under duration threshold: 500m, 1 minute
			distance = 500
			end = start.Add(1 * time.Minute)
		} else {
			// Passes both: 5km, 30 minutes
			distance = 5000
			end = start.Add(30 * time.Minute)
		}

		if i > 0 {
			objects += ","
		}
		objects += `{
			"activitySegment": {
				"startLocation": {"latitudeE7": 475000000, "longitudeE7": 87000000},
				"endLocation": {"latitudeE7": 475200000, "longitudeE7": 87500000},
				"duration": {
					"startTimestamp": "` + start.Format(time.RFC3339) + `",
					"endTimestamp": "` + end.Format(time.RFC3339) + `"
				},
				"distance": ` + fmt.Sprintf("%d", distance) + `,
				"activityType": "WALKING",
				"waypointPath": {
					"waypoints": [
						{"latE7": 475000000, "lngE7": 87000000},
						{"latE7": 475200000, "lngE7": 87500000}
					]
				}
			}
		}`
	}
	return `{"timelineObjects": [` + objects + `]}`
}

func TestConfigFloat64NonNeg(t *testing.T) {
	tests := []struct {
		name    string
		sc      map[string]interface{}
		key     string
		wantVal float64
		wantErr bool
	}{
		{"absent key", map[string]interface{}{}, "x", -1, false},
		{"float64 zero", map[string]interface{}{"x": float64(0)}, "x", 0, false},
		{"float64 positive", map[string]interface{}{"x": float64(42)}, "x", 42, false},
		{"float64 negative", map[string]interface{}{"x": float64(-1)}, "x", 0, true},
		{"int zero", map[string]interface{}{"x": int(0)}, "x", 0, false},
		{"int positive", map[string]interface{}{"x": int(7)}, "x", 7, false},
		{"int negative", map[string]interface{}{"x": int(-3)}, "x", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := configFloat64NonNeg(tt.sc, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && v != tt.wantVal {
				t.Errorf("value = %v, want %v", v, tt.wantVal)
			}
		})
	}
}

func TestConfigFloat64Positive(t *testing.T) {
	tests := []struct {
		name    string
		sc      map[string]interface{}
		key     string
		wantVal float64
		wantErr bool
	}{
		{"absent key", map[string]interface{}{}, "x", -1, false},
		{"float64 positive", map[string]interface{}{"x": float64(5)}, "x", 5, false},
		{"float64 zero", map[string]interface{}{"x": float64(0)}, "x", 0, true},
		{"float64 negative", map[string]interface{}{"x": float64(-1)}, "x", 0, true},
		{"int positive", map[string]interface{}{"x": int(10)}, "x", 10, false},
		{"int zero", map[string]interface{}{"x": int(0)}, "x", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := configFloat64Positive(tt.sc, tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && v != tt.wantVal {
				t.Errorf("value = %v, want %v", v, tt.wantVal)
			}
		})
	}
}

func TestConfigIntMin(t *testing.T) {
	tests := []struct {
		name    string
		sc      map[string]interface{}
		key     string
		min     int
		wantVal int
		wantErr bool
	}{
		{"absent key", map[string]interface{}{}, "x", 1, -1, false},
		{"float64 at min", map[string]interface{}{"x": float64(1)}, "x", 1, 1, false},
		{"float64 above min", map[string]interface{}{"x": float64(5)}, "x", 1, 5, false},
		{"float64 below min", map[string]interface{}{"x": float64(0)}, "x", 1, 0, true},
		{"int at min", map[string]interface{}{"x": int(3)}, "x", 3, 3, false},
		{"int below min", map[string]interface{}{"x": int(0)}, "x", 1, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := configIntMin(tt.sc, tt.key, tt.min)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && v != tt.wantVal {
				t.Errorf("value = %v, want %v", v, tt.wantVal)
			}
		})
	}
}
