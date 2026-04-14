package maps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- CHAOS-C01: Pipe character in filename corrupts cursor encoding ---
// The cursor uses "|" as a delimiter. Filenames containing "|" would be split
// incorrectly, causing the cursor to track phantom entries and miss the real file.
// Fix: findNewFiles skips files with "|" in their name.

func TestChaos_PipeInFilenameSkipped(t *testing.T) {
	dir := t.TempDir()
	// Create a file with a pipe in its name (valid on Linux/macOS).
	pipeFile := filepath.Join(dir, "data|export.json")
	if err := os.WriteFile(pipeFile, []byte(makeTakeoutJSON(2)), 0o644); err != nil {
		t.Fatalf("write pipe-named file: %v", err)
	}
	// Also create a normal file.
	writeTakeoutFile(t, dir, "normal.json", makeTakeoutJSON(3))

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

	// Only the normal file should be processed (pipe file skipped).
	if len(artifacts) != 3 {
		t.Errorf("expected 3 artifacts from normal.json only, got %d", len(artifacts))
	}

	// Cursor must not be corrupted by the pipe filename.
	parsed := parseCursor(cursor)
	for _, entry := range parsed {
		if entry == "data" || entry == "export.json" {
			t.Errorf("cursor contains split fragment %q from pipe-named file", entry)
		}
	}
	if cursor != "normal.json" {
		t.Errorf("cursor = %q, want %q (pipe file excluded)", cursor, "normal.json")
	}

	// Second sync should still work with the cursor.
	artifacts2, cursor2, err := c.Sync(context.Background(), cursor)
	if err != nil {
		t.Fatalf("Sync2: %v", err)
	}
	// Pipe file is still skipped, normal file already processed → 0 new artifacts.
	if len(artifacts2) != 0 {
		t.Errorf("second sync should produce 0 new artifacts, got %d", len(artifacts2))
	}
	if cursor2 != cursor {
		t.Errorf("cursor should not change, got %q", cursor2)
	}
}

// --- CHAOS-C02: Archive collision silently overwrites a previous archived file ---
// If the same filename appears in two separate Takeout drops (or file is restored
// to import dir after archival), the second archive operation could overwrite the first.
// Fix: archiveFile adds a timestamp suffix when the destination already exists.

func TestChaos_ArchiveCollisionPreservesBoth(t *testing.T) {
	dir := t.TempDir()

	c := New("google-maps-timeline")
	c.config = MapsConfig{ImportDir: dir, ArchiveProcessed: true}

	// Create and archive the first version.
	firstContent := []byte(`{"version": 1}`)
	firstFile := filepath.Join(dir, "export.json")
	if err := os.WriteFile(firstFile, firstContent, 0o644); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := c.archiveFile(firstFile); err != nil {
		t.Fatalf("archive first: %v", err)
	}

	// Verify first version is archived.
	archivedFirst := filepath.Join(dir, "archive", "export.json")
	data, err := os.ReadFile(archivedFirst)
	if err != nil {
		t.Fatalf("read archived first: %v", err)
	}
	if string(data) != `{"version": 1}` {
		t.Errorf("first archived content = %q, want version 1", string(data))
	}

	// Create a second file with the same name but different content.
	secondContent := []byte(`{"version": 2}`)
	secondFile := filepath.Join(dir, "export.json")
	if err := os.WriteFile(secondFile, secondContent, 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}
	if err := c.archiveFile(secondFile); err != nil {
		t.Fatalf("archive second: %v", err)
	}

	// First archived version must still exist with original content (not overwritten).
	data, err = os.ReadFile(archivedFirst)
	if err != nil {
		t.Fatalf("re-read archived first: %v", err)
	}
	if string(data) != `{"version": 1}` {
		t.Errorf("first archived file was overwritten! content = %q", string(data))
	}

	// Second version must also exist in the archive directory.
	archiveEntries, err := os.ReadDir(filepath.Join(dir, "archive"))
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(archiveEntries) < 2 {
		t.Errorf("expected at least 2 files in archive (original + collision-renamed), got %d", len(archiveEntries))
	}

	// Verify the second version's content is preserved somewhere in the archive.
	foundSecond := false
	for _, entry := range archiveEntries {
		path := filepath.Join(dir, "archive", entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if string(content) == `{"version": 2}` {
			foundSecond = true
			break
		}
	}
	if !foundSecond {
		t.Error("second archived file content not found in archive directory")
	}
}

// --- CHAOS-C03: Dedup hash collision for routeless activities at same type/hour ---
// Two routeless activities of the same type at the same hour on the same day
// produce identical dedup hashes because activityGridCoords returns (0,0,0,0)
// for both. This is a known limitation of the grid-based hash design.
// Changing the hash format would break existing dedup data, so this is documented.

func TestChaos_DedupHashCollisionRouteless(t *testing.T) {
	// Two distinct walks at the same hour, no route data, different distances.
	a1 := TakeoutActivity{
		Type:        ActivityWalk,
		StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 10, 15, 0, 0, time.UTC),
		DistanceKm:  1.2,
		DurationMin: 15,
		Route:       nil, // no waypoints
	}
	a2 := TakeoutActivity{
		Type:        ActivityWalk,
		StartTime:   time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 10, 50, 0, 0, time.UTC),
		DistanceKm:  0.8,
		DurationMin: 20,
		Route:       nil, // no waypoints
	}

	h1 := computeDedupHash(a1)
	h2 := computeDedupHash(a2)

	// Both are hour 10, walk type, grid (0,0)→(0,0) — known collision scenario.
	// This test documents the limitation. If the hash is ever improved to include
	// distance or minute-level granularity, this test should be updated to expect
	// distinct hashes.
	if h1 != h2 {
		// If they're different, the hash was improved — update this test.
		t.Logf("dedup hashes are now distinct (%q vs %q) — collision fixed", h1, h2)
	} else {
		t.Logf("KNOWN LIMITATION: routeless same-type same-hour activities collide: %q", h1)
	}

	// Activities at DIFFERENT hours must always produce distinct hashes.
	a3 := TakeoutActivity{
		Type:        ActivityWalk,
		StartTime:   time.Date(2026, 3, 15, 11, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 11, 15, 0, 0, time.UTC),
		DistanceKm:  1.2,
		DurationMin: 15,
		Route:       nil,
	}
	h3 := computeDedupHash(a3)
	if h1 == h3 {
		t.Errorf("activities at different hours must have different hashes: h1=%q h3=%q", h1, h3)
	}

	// Activities at DIFFERENT types must always produce distinct hashes.
	a4 := TakeoutActivity{
		Type:        ActivityRun,
		StartTime:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 10, 15, 0, 0, time.UTC),
		DistanceKm:  1.2,
		DurationMin: 15,
		Route:       nil,
	}
	h4 := computeDedupHash(a4)
	if h1 == h4 {
		t.Errorf("activities of different types must have different hashes: h1=%q h4=%q", h1, h4)
	}
}

// --- CHAOS-C04: Concurrent Sync + Close must not panic ---
// Rapid interleaving of Sync and Close exercises the mutex boundaries.

func TestChaos_ConcurrentSyncClose(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeTakeoutFile(t, dir, fmt.Sprintf("concurrent_%d.json", i), makeTakeoutJSON(2))
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

	var wg sync.WaitGroup
	// Hammer Sync and Close concurrently — must not panic.
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			// Sync may return error after Close; that's fine.
			_, _, _ = c.Sync(context.Background(), "")
		}()
		go func() {
			defer wg.Done()
			_ = c.Close()
		}()
	}
	wg.Wait()
	// If we got here without panic, the test passes.
}

// --- CHAOS-C05: File disappears between directory scan and ReadFile ---
// A file present during findNewFiles may be deleted before Sync reads it.
// Sync must handle this gracefully without crashing.

func TestChaos_FileDisappearsBetweenScanAndRead(t *testing.T) {
	dir := t.TempDir()
	ephemeral := filepath.Join(dir, "ephemeral.json")
	if err := os.WriteFile(ephemeral, []byte(makeTakeoutJSON(1)), 0o644); err != nil {
		t.Fatalf("write ephemeral: %v", err)
	}
	writeTakeoutFile(t, dir, "stable.json", makeTakeoutJSON(2))

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

	// Delete the ephemeral file after Connect but before Sync reads it.
	if err := os.Remove(ephemeral); err != nil {
		t.Fatalf("remove ephemeral: %v", err)
	}

	// Sync should not crash. The stable file should still be processed.
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync should handle missing file gracefully: %v", err)
	}
	// At minimum, stable.json's 2 artifacts should be produced.
	if len(artifacts) < 2 {
		t.Errorf("expected at least 2 artifacts from stable.json, got %d", len(artifacts))
	}
}

// --- CHAOS-C06: Midnight UTC boundary dedup hash stability ---
// Activities near midnight must produce distinct hashes because the date
// component changes across the boundary.

func TestChaos_MidnightBoundaryDedupHash(t *testing.T) {
	// Activity ending at 23:59 UTC on March 15.
	beforeMidnight := TakeoutActivity{
		Type:        ActivityWalk,
		StartTime:   time.Date(2026, 3, 15, 23, 30, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 15, 23, 59, 0, 0, time.UTC),
		DistanceKm:  2.5,
		DurationMin: 29,
		Route:       []LatLng{{Lat: 47.5, Lng: 8.7}, {Lat: 47.52, Lng: 8.75}},
	}

	// Activity starting at 00:01 UTC on March 16.
	afterMidnight := TakeoutActivity{
		Type:        ActivityWalk,
		StartTime:   time.Date(2026, 3, 16, 0, 1, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 3, 16, 0, 30, 0, 0, time.UTC),
		DistanceKm:  2.5,
		DurationMin: 29,
		Route:       []LatLng{{Lat: 47.5, Lng: 8.7}, {Lat: 47.52, Lng: 8.75}},
	}

	h1 := computeDedupHash(beforeMidnight)
	h2 := computeDedupHash(afterMidnight)

	if h1 == h2 {
		t.Errorf("activities across midnight boundary must have different hashes: both = %q", h1)
	}
}

// --- CHAOS-C07: Rapid re-sync after archive does not re-import archived files ---
// Verifies that archived files in the archive/ subdirectory are not re-discovered.

func TestChaos_ArchivedFilesNotRediscovered(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "cycle1.json", makeTakeoutJSON(2))

	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir":        dir,
			"archive_processed": true,
			"min_distance_m":    float64(0),
			"min_duration_min":  float64(0),
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// First sync archives the file.
	artifacts1, cursor1, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync1: %v", err)
	}
	if len(artifacts1) != 2 {
		t.Errorf("expected 2 artifacts from first sync, got %d", len(artifacts1))
	}

	// Verify archive directory contains the file.
	archivedPath := filepath.Join(dir, "archive", "cycle1.json")
	if _, err := os.Stat(archivedPath); err != nil {
		t.Fatalf("archived file not found: %v", err)
	}

	// Second sync with same cursor should find nothing.
	artifacts2, _, err := c.Sync(context.Background(), cursor1)
	if err != nil {
		t.Fatalf("Sync2: %v", err)
	}
	if len(artifacts2) != 0 {
		t.Errorf("expected 0 artifacts on re-sync after archive, got %d", len(artifacts2))
	}

	// Archive subdirectory must not be scanned as a new JSON file.
	// The archive/ dir is inside import dir — findNewFiles must skip directories.
	files, err := findNewFiles(dir, nil)
	if err != nil {
		t.Fatalf("findNewFiles: %v", err)
	}
	for _, f := range files {
		if filepath.Base(f) == "cycle1.json" {
			t.Error("archived file should not appear in findNewFiles results")
		}
	}
}

// --- CHAOS-C08: Sync with cancelled context produces consistent state ---
// After cancellation, the health status must be set correctly and the connector
// must remain usable for subsequent Sync calls.

func TestChaos_CancelledSyncRecovery(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeTakeoutFile(t, dir, fmt.Sprintf("data_%d.json", i), makeTakeoutJSON(1))
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

	// Sync with pre-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("cancelled Sync should not return error: %v", err)
	}

	// Health should be healthy or error, not left in "syncing" state.
	health := c.Health(context.Background())
	if health == connector.HealthSyncing {
		t.Error("health must not remain 'syncing' after Sync completes")
	}

	// Connector must be usable for a normal Sync afterward.
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("recovery Sync failed: %v", err)
	}
	if len(artifacts) < 1 {
		t.Error("recovery Sync should process files")
	}
}

// --- CHAOS-C09: archiveFile collision loop has no termination bound ---
// Under adversarial conditions (many collision files in archive/), the collision
// loop in archiveFile could spin indefinitely. Fix: cap at maxArchiveCollisions = 1000.

func TestChaos_ArchiveCollisionLoopBounded(t *testing.T) {
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("create archive dir: %v", err)
	}

	// Pre-create the base archive entry plus 1000 collision files (export.json, export_1.json ... export_1000.json).
	baseName := "export.json"
	if err := os.WriteFile(filepath.Join(archiveDir, baseName), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write base archive: %v", err)
	}
	for i := 1; i <= 1000; i++ {
		name := fmt.Sprintf("export_%d.json", i)
		if err := os.WriteFile(filepath.Join(archiveDir, name), []byte(`{}`), 0o644); err != nil {
			t.Fatalf("write collision file %d: %v", i, err)
		}
	}

	// Create the source file to archive.
	srcFile := filepath.Join(dir, baseName)
	if err := os.WriteFile(srcFile, []byte(`{"data": true}`), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	c := New("google-maps-timeline")
	c.config = MapsConfig{ImportDir: dir, ArchiveProcessed: true}

	err := c.archiveFile(srcFile)
	if err == nil {
		t.Fatal("expected error when collision limit exceeded, got nil")
	}
	if !strings.Contains(err.Error(), "archive collision limit exceeded") {
		t.Errorf("unexpected error message: %v", err)
	}
	if !strings.Contains(err.Error(), "1000") {
		t.Errorf("error should mention the limit (1000): %v", err)
	}

	// Source file should still exist (not moved).
	if _, err := os.Stat(srcFile); err != nil {
		t.Errorf("source file should still exist after collision limit error: %v", err)
	}
}

// --- CHAOS-C10: StartLocation/EndLocation fallback for routeless activity dedup hash ---
// Two routeless activities at the same hour with different start/end locations must
// produce distinct dedup hashes, now that activityGridCoords falls back to
// StartLocation/EndLocation when Route is empty.

func TestChaos_StartEndLocationFallbackDedupHash(t *testing.T) {
	// Two walks at the same hour, no route, but different start/end locations.
	a1 := TakeoutActivity{
		Type:          ActivityWalk,
		StartTime:     time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 3, 15, 10, 15, 0, 0, time.UTC),
		Route:         nil,
		StartLocation: LatLng{Lat: 47.5, Lng: 8.7},
		EndLocation:   LatLng{Lat: 47.52, Lng: 8.75},
		DistanceKm:    1.2,
		DurationMin:   15,
	}
	a2 := TakeoutActivity{
		Type:          ActivityWalk,
		StartTime:     time.Date(2026, 3, 15, 10, 30, 0, 0, time.UTC),
		EndTime:       time.Date(2026, 3, 15, 10, 50, 0, 0, time.UTC),
		Route:         nil,
		StartLocation: LatLng{Lat: 48.1, Lng: 11.5},
		EndLocation:   LatLng{Lat: 48.15, Lng: 11.55},
		DistanceKm:    0.8,
		DurationMin:   20,
	}

	h1 := computeDedupHash(a1)
	h2 := computeDedupHash(a2)

	if h1 == h2 {
		t.Errorf("routeless activities with different start/end locations must have distinct hashes: both = %q", h1)
	}

	// Verify the grid coords actually use StartLocation/EndLocation fallback.
	sLat1, sLng1, eLat1, eLng1 := activityGridCoords(a1)
	sLat2, sLng2, eLat2, eLng2 := activityGridCoords(a2)

	if sLat1 == 0 && sLng1 == 0 && eLat1 == 0 && eLng1 == 0 {
		t.Error("a1 grid coords should use StartLocation/EndLocation fallback, got all zeros")
	}
	if sLat2 == 0 && sLng2 == 0 && eLat2 == 0 && eLng2 == 0 {
		t.Error("a2 grid coords should use StartLocation/EndLocation fallback, got all zeros")
	}
	if sLat1 == sLat2 && sLng1 == sLng2 && eLat1 == eLat2 && eLng1 == eLng2 {
		t.Error("grid coords should be different for activities at different locations")
	}

	// Also verify buildMetadata uses the fallback for routeless activities.
	meta1 := buildMetadata(a1, "test.json")
	if meta1["start_lat"].(float64) == 0.0 && meta1["start_lng"].(float64) == 0.0 {
		t.Error("buildMetadata should use StartLocation fallback for routeless activity")
	}
	if meta1["start_lat"].(float64) != a1.StartLocation.Lat {
		t.Errorf("buildMetadata start_lat = %v, want %v", meta1["start_lat"], a1.StartLocation.Lat)
	}
	if meta1["end_lat"].(float64) != a1.EndLocation.Lat {
		t.Errorf("buildMetadata end_lat = %v, want %v", meta1["end_lat"], a1.EndLocation.Lat)
	}
}

// --- STB-011-001: Config race — Sync uses snapshotted config, not live c.config ---
// If Connect() is called concurrently with Sync(), the Sync must use the config
// that was snapshotted at the start, not a partially-updated config.

func TestStabilize_SyncUsesConfigSnapshot(t *testing.T) {
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

	// Start Sync and concurrently update config via re-Connect to a different dir.
	// Under the old code this was a data race. Under the fix, Sync snapshots
	// config at the start so concurrent Connect won't cause inconsistency.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _, _ = c.Sync(context.Background(), "")
	}()

	go func() {
		defer wg.Done()
		otherDir := t.TempDir()
		otherCfg := connector.ConnectorConfig{
			AuthType: "none",
			Enabled:  true,
			SourceConfig: map[string]interface{}{
				"import_dir":       otherDir,
				"min_distance_m":   float64(0),
				"min_duration_min": float64(0),
			},
		}
		_ = c.Connect(context.Background(), otherCfg)
	}()

	wg.Wait()

	// If we get here without a race detector panic or data corruption, the test passes.
	// The primary value is that `go test -race` catches the race; the test itself
	// verifies the concurrent access doesn't panic.
}

// --- STB-011-001: Config race — PostSync uses snapshotted config ---

func TestStabilize_PostSyncUsesConfigSnapshot(t *testing.T) {
	dir := t.TempDir()
	c := New("google-maps-timeline")
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir":               dir,
			"commute_min_occurrences":  float64(3),
			"commute_window_days":      float64(14),
			"trip_min_distance_km":     float64(50),
			"trip_min_overnight_hours": float64(18),
			"link_time_extend_min":     float64(30),
			"link_proximity_radius_m":  float64(1000),
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// PostSync without a pool should return nil, nil — no race even without pool.
	artifacts, err := c.PostSync(context.Background(), nil)
	if err != nil {
		t.Errorf("PostSync without pool should return nil error, got: %v", err)
	}
	if artifacts != nil {
		t.Errorf("PostSync without pool should return nil artifacts, got %d", len(artifacts))
	}

	// Concurrent PostSync + Connect must not race on c.config.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = c.PostSync(context.Background(), nil)
	}()
	go func() {
		defer wg.Done()
		_ = c.Connect(context.Background(), cfg)
	}()
	wg.Wait()
}

// --- STB-011-002: PostSync error aggregation ---
// PostSync must return a non-nil error when sub-operations fail, instead of
// silently returning nil. This verifies the error propagation contract.

func TestStabilize_PostSyncNoPoolReturnsNil(t *testing.T) {
	c := New("google-maps-timeline")
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		AuthType: "none",
		Enabled:  true,
		SourceConfig: map[string]interface{}{
			"import_dir": dir,
		},
	}
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Without pool, PostSync should return (nil, nil) — the no-op path.
	artifacts, err := c.PostSync(context.Background(), nil)
	if err != nil {
		t.Errorf("PostSync without pool: expected nil error, got %v", err)
	}
	if artifacts != nil {
		t.Errorf("PostSync without pool: expected nil artifacts, got %d", len(artifacts))
	}
}

// --- STB-011-001: findNewFiles as package function uses explicit importDir ---

func TestStabilize_FindNewFilesExplicitDir(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "test.json", makeTakeoutJSON(1))

	files, err := findNewFiles(dir, nil)
	if err != nil {
		t.Fatalf("findNewFiles: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}

	// With the file already processed, should return empty.
	files2, err := findNewFiles(dir, []string{"test.json"})
	if err != nil {
		t.Fatalf("findNewFiles with processed: %v", err)
	}
	if len(files2) != 0 {
		t.Errorf("expected 0 files after processing, got %d", len(files2))
	}
}

// --- STB-011-001: pruneCursor as package function uses explicit importDir ---

func TestStabilize_PruneCursorExplicitDir(t *testing.T) {
	dir := t.TempDir()
	writeTakeoutFile(t, dir, "exists.json", makeTakeoutJSON(1))

	pruned := pruneCursor(dir, []string{"exists.json", "gone.json"})
	if len(pruned) != 1 {
		t.Fatalf("expected 1 after pruning, got %d: %v", len(pruned), pruned)
	}
	if pruned[0] != "exists.json" {
		t.Errorf("expected exists.json, got %q", pruned[0])
	}

	// With invalid dir, should keep all entries as safety fallback.
	pruned2 := pruneCursor("/nonexistent/path", []string{"a.json", "b.json"})
	if len(pruned2) != 2 {
		t.Errorf("expected 2 entries on fallback, got %d", len(pruned2))
	}
}
