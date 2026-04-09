package maps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestConnectValidConfig(t *testing.T) {
	dir := t.TempDir()
	c := New("google-maps-timeline")

	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir": dir,
		},
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

func TestParseMapsConfigDefaults(t *testing.T) {
	cfg, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir": "/tmp/test",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MinDistanceM != 100 {
		t.Errorf("expected default MinDistanceM 100, got %v", cfg.MinDistanceM)
	}
	if cfg.MinDurationMin != 2 {
		t.Errorf("expected default MinDurationMin 2, got %v", cfg.MinDurationMin)
	}
	if cfg.DefaultTier != "standard" {
		t.Errorf("expected default tier %q, got %q", "standard", cfg.DefaultTier)
	}
	if cfg.CommuteMinOccurrences != 3 {
		t.Errorf("expected default CommuteMinOccurrences 3, got %v", cfg.CommuteMinOccurrences)
	}
	if cfg.TripMinDistanceKm != 50 {
		t.Errorf("expected default TripMinDistanceKm 50, got %v", cfg.TripMinDistanceKm)
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
		SourceConfig: map[string]interface{}{
			"import_dir":       dir,
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		},
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
		SourceConfig: map[string]interface{}{
			"import_dir":       dir,
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		},
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
		SourceConfig: map[string]interface{}{
			"import_dir":       dir,
			"min_distance_m":   float64(0),
			"min_duration_min": float64(0),
		},
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
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir":       dir,
			"min_distance_m":   float64(100),
			"min_duration_min": float64(2),
		},
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
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir": dir,
		},
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

// --- test helpers ---

func writeTakeoutFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
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
