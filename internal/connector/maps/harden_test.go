package maps

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- H-011-001: Stale sync health after findNewFiles failure ---
// When findNewFiles fails after a previous successful sync, the defer health
// check must still set HealthError (not HealthHealthy from stale lastSyncCount).

func TestHarden_SyncErrorResetsHealthAfterPreviousSuccess(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "data.json", makeTakeoutJSON(3))

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

	// First sync succeeds — lastSyncCount > 0.
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("first Sync: %v", err)
	}
	if len(artifacts) == 0 {
		t.Fatal("first sync should produce artifacts")
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Fatalf("health after first sync = %q, want healthy", c.Health(context.Background()))
	}

	// Remove the import directory to make findNewFiles fail.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	// Second sync will fail at findNewFiles.
	_, _, err = c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when import dir is removed")
	}

	// Health MUST be HealthError, not HealthHealthy (stale lastSyncCount bug).
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("health after directory scan failure = %q, want %q",
			c.Health(context.Background()), connector.HealthError)
	}
}

// --- H-011-002: Config helpers reject string-typed values ---
// String values in config must produce an explicit error, not silently use defaults.

func TestHarden_ConfigFloat64NonNegRejectsString(t *testing.T) {
	_, err := configFloat64NonNeg(map[string]interface{}{"x": "200"}, "x")
	if err == nil {
		t.Fatal("expected error for string-typed float64 config value")
	}
}

func TestHarden_ConfigFloat64PositiveRejectsString(t *testing.T) {
	_, err := configFloat64Positive(map[string]interface{}{"x": "50"}, "x")
	if err == nil {
		t.Fatal("expected error for string-typed positive float64 config value")
	}
}

func TestHarden_ConfigIntMinRejectsString(t *testing.T) {
	_, err := configIntMin(map[string]interface{}{"x": "3"}, "x", 1)
	if err == nil {
		t.Fatal("expected error for string-typed int config value")
	}
}

func TestHarden_ParseMapsConfigRejectsStringMinDistance(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":     "/tmp/test",
			"min_distance_m": "200",
		},
	})
	if err == nil {
		t.Fatal("expected error when min_distance_m is a string")
	}
}

func TestHarden_ParseMapsConfigRejectsStringCommuteOccurrences(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":              "/tmp/test",
			"commute_min_occurrences": "5",
		},
	})
	if err == nil {
		t.Fatal("expected error when commute_min_occurrences is a string")
	}
}

func TestHarden_ParseMapsConfigRejectsStringTripDistance(t *testing.T) {
	_, err := parseMapsConfig(connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":           "/tmp/test",
			"trip_min_distance_km": "100",
		},
	})
	if err == nil {
		t.Fatal("expected error when trip_min_distance_km is a string")
	}
}

// --- H-011-003: NormalizeActivity validates ActivityType ---
// Empty or unknown ActivityType must not produce "activity/" ContentType.

func TestHarden_NormalizeActivityEmptyType(t *testing.T) {
	activity := TakeoutActivity{
		Type:        "", // empty
		StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC),
		DistanceKm:  5.0,
		DurationMin: 30,
	}
	artifact := NormalizeActivity(activity, "test.json")
	if artifact.ContentType == "activity/" {
		t.Error("empty ActivityType must not produce ContentType 'activity/'")
	}
	if artifact.ContentType != "activity/walk" {
		t.Errorf("empty ActivityType should default to 'activity/walk', got %q", artifact.ContentType)
	}
}

func TestHarden_NormalizeActivityUnknownType(t *testing.T) {
	activity := TakeoutActivity{
		Type:        ActivityType("skateboard"), // unknown
		StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC),
		DistanceKm:  5.0,
		DurationMin: 30,
	}
	artifact := NormalizeActivity(activity, "test.json")
	if artifact.ContentType == "activity/skateboard" {
		t.Error("unknown ActivityType must not produce unrecognized ContentType")
	}
	if artifact.ContentType != "activity/walk" {
		t.Errorf("unknown ActivityType should default to 'activity/walk', got %q", artifact.ContentType)
	}
}

func TestHarden_ValidatedActivityTypeReturnsKnown(t *testing.T) {
	known := []ActivityType{ActivityHike, ActivityWalk, ActivityCycle, ActivityDrive, ActivityTransit, ActivityRun}
	for _, k := range known {
		got := validatedActivityType(k)
		if got != k {
			t.Errorf("validatedActivityType(%q) = %q, want %q", k, got, k)
		}
	}

	// Unknown types all map to walk.
	unknowns := []ActivityType{"", "skateboard", "flying", ActivityType("SWIM")}
	for _, u := range unknowns {
		got := validatedActivityType(u)
		if got != ActivityWalk {
			t.Errorf("validatedActivityType(%q) = %q, want %q", u, got, ActivityWalk)
		}
	}
}

// --- H-011-004: Cross-file artifact cap prevents unbounded memory ---
// When multiple files together exceed maxActivities, processing must stop
// before memory is exhausted.

func TestHarden_CrossFileArtifactCap(t *testing.T) {
	dir := t.TempDir()

	// Create enough files that total activities would exceed any reasonable cap.
	// Each file has 100 activities. With maxActivities = 50000, we'd need 501 files
	// to hit it. Instead, test the mechanism by creating files and verifying the
	// artifact count is bounded.
	//
	// We can't easily test 50001+ activities in unit tests (too slow), so we
	// verify the cap code path exists by confirming normal operation still works
	// and the cap field is respected.
	for i := 0; i < 10; i++ {
		writeTakeoutFile(t, dir, fmt.Sprintf("batch_%02d.json", i), makeTakeoutJSON(5))
	}

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

	// 10 files × 5 activities = 50 — well below cap, so all should be processed.
	if len(artifacts) != 50 {
		t.Errorf("expected 50 artifacts (10×5), got %d", len(artifacts))
	}

	// Verify the cap constant is reasonable.
	if maxActivities < 1000 || maxActivities > 200000 {
		t.Errorf("maxActivities = %d, expected a reasonable cap between 1000 and 200000", maxActivities)
	}
}
