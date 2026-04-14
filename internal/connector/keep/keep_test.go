package keep

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnectorID(t *testing.T) {
	c := New("google-keep")
	if c.ID() != "google-keep" {
		t.Errorf("ID() = %q, want %q", c.ID(), "google-keep")
	}
}

func TestConnectValidTakeoutConfig(t *testing.T) {
	dir := t.TempDir()
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.Health(context.Background()) != "healthy" {
		t.Errorf("health = %q, want healthy", c.Health(context.Background()))
	}
}

func TestConnectMissingImportDir(t *testing.T) {
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig("/nonexistent/path/that/does/not/exist", "takeout", false, false))
	if err == nil {
		t.Fatal("expected error for missing import dir")
	}
}

func TestConnectGkeepWithoutAck(t *testing.T) {
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig(t.TempDir(), "gkeepapi", true, false))
	if err == nil {
		t.Fatal("expected error for gkeepapi without ack")
	}
	if c.Health(context.Background()) != "error" {
		t.Errorf("health = %q, want error", c.Health(context.Background()))
	}
}

func TestParseKeepConfigValidation(t *testing.T) {
	// Invalid sync_mode
	_, err := parseKeepConfig(testConnectorConfig("", "invalid_mode", false, false))
	if err == nil {
		t.Error("expected error for invalid sync_mode")
	}

	// Poll interval too short
	cfg := testConnectorConfig("", "takeout", false, false)
	cfg.SourceConfig["poll_interval"] = "5m"
	_, err = parseKeepConfig(cfg)
	if err == nil {
		t.Error("expected error for poll_interval < 15m")
	}
}

func TestSyncTakeoutProducesArtifacts(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		writeTestJSON(t, dir, "note-"+string(rune('a'+i))+".json", `{
			"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
			"textContent": "This is note content that is long enough to pass filters easily",
			"title": "Test Note", "userEditedTimestampUsec": 1712000000000000,
			"createdTimestampUsec": 1711900000000000,
			"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
		}`)
	}

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 10 {
		t.Errorf("artifacts = %d, want 10", len(artifacts))
	}
	if cursor == "" {
		t.Error("cursor should not be empty")
	}
	for _, a := range artifacts {
		if a.SourceID != "google-keep" {
			t.Errorf("SourceID = %q, want google-keep", a.SourceID)
		}
	}
}

func TestSyncAdvancesCursor(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Content here for cursor test with enough length",
		"title": "Test", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if cursor == "" {
		t.Error("cursor should not be empty after sync")
	}
}

func TestSyncSkipsTrashedNotes(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "active.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Active note with enough content to pass", "title": "Active",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)
	writeTestJSON(t, dir, "trashed.json", `{
		"color": "DEFAULT", "isTrashed": true, "isPinned": false, "isArchived": false,
		"textContent": "Trashed note", "title": "Trashed",
		"userEditedTimestampUsec": 1712000000000000, "createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("artifacts = %d, want 1 (trashed should be skipped)", len(artifacts))
	}
}

func TestHealthTransitions(t *testing.T) {
	ctx := context.Background()
	c := New("google-keep")

	// Initial: disconnected
	if c.Health(ctx) != "disconnected" {
		t.Errorf("initial health = %q, want disconnected", c.Health(ctx))
	}

	// After Connect: healthy
	dir := t.TempDir()
	if err := c.Connect(ctx, testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.Health(ctx) != "healthy" {
		t.Errorf("after connect health = %q, want healthy", c.Health(ctx))
	}

	// After Close: disconnected
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Health(ctx) != "disconnected" {
		t.Errorf("after close health = %q, want disconnected", c.Health(ctx))
	}
}

func TestCloseResetsHealth(t *testing.T) {
	c := New("google-keep")
	dir := t.TempDir()
	_ = c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false))
	_ = c.Close()
	if c.Health(context.Background()) != "disconnected" {
		t.Errorf("health = %q, want disconnected", c.Health(context.Background()))
	}
}

func TestKeepExportTracking(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Content long enough to not be filtered out by min content",
		"title": "Test", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	_ = c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false))

	// First sync
	artifacts1, cursor1, _ := c.Sync(context.Background(), "")
	if len(artifacts1) != 1 {
		t.Fatalf("first sync artifacts = %d, want 1", len(artifacts1))
	}

	// Second sync with same cursor — should return 0 (cursor filters them out)
	artifacts2, _, _ := c.Sync(context.Background(), cursor1)
	if len(artifacts2) != 0 {
		t.Errorf("second sync artifacts = %d, want 0", len(artifacts2))
	}
}

func TestCorruptedCursorFallback(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Content that has enough length for the minimum content check",
		"title": "Test", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	_ = c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false))

	// Corrupted cursor — should trigger full re-sync
	artifacts, _, err := c.Sync(context.Background(), "not-a-valid-timestamp")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("artifacts = %d, want 1 (full resync on corrupted cursor)", len(artifacts))
	}
}

// connectorConfig creates a ConnectorConfig for testing.
func testConnectorConfig(importDir, mode string, gkeepEnabled, gkeepAck bool) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":            mode,
			"import_dir":           importDir,
			"gkeep_enabled":        gkeepEnabled,
			"warning_acknowledged": gkeepAck,
		},
	}
}

func TestSyncSameTitleDifferentFiles(t *testing.T) {
	dir := t.TempDir()
	// Two notes with identical titles but different filenames
	writeTestJSON(t, dir, "meeting-2026-04-01.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "First meeting notes with enough content for min length",
		"title": "Meeting Notes", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)
	writeTestJSON(t, dir, "meeting-2026-04-03.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Second meeting notes with enough content for min length",
		"title": "Meeting Notes", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %d, want 2", len(artifacts))
	}

	// NoteIDs must be unique despite same title
	if artifacts[0].SourceRef == artifacts[1].SourceRef {
		t.Errorf("NoteIDs collide: both are %q — filenames should produce unique IDs", artifacts[0].SourceRef)
	}
}

func TestParseKeepConfigAllFields(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":            "hybrid",
			"import_dir":           "/tmp/keep",
			"gkeep_enabled":        true,
			"warning_acknowledged": true,
			"include_archived":     true,
			"min_content_length":   float64(10),
			"poll_interval":        "30m",
			"watch_interval":       "10m",
			"archive_processed":    true,
			"default_tier":         "full",
			"labels_filter":        []interface{}{"Work", "Personal"},
		},
	}

	kc, err := parseKeepConfig(cfg)
	if err != nil {
		t.Fatalf("parseKeepConfig: %v", err)
	}
	if kc.SyncMode != SyncModeHybrid {
		t.Errorf("SyncMode = %q, want hybrid", kc.SyncMode)
	}
	if kc.TakeoutImportDir != "/tmp/keep" {
		t.Errorf("TakeoutImportDir = %q, want /tmp/keep", kc.TakeoutImportDir)
	}
	if !kc.GkeepEnabled {
		t.Error("GkeepEnabled should be true")
	}
	if !kc.GkeepWarningAck {
		t.Error("GkeepWarningAck should be true")
	}
	if !kc.IncludeArchived {
		t.Error("IncludeArchived should be true")
	}
	if kc.MinContentLength != 10 {
		t.Errorf("MinContentLength = %d, want 10", kc.MinContentLength)
	}
	if kc.GkeepPollInterval != 30*time.Minute {
		t.Errorf("GkeepPollInterval = %v, want 30m", kc.GkeepPollInterval)
	}
	if kc.TakeoutWatchInterval != 10*time.Minute {
		t.Errorf("TakeoutWatchInterval = %v, want 10m", kc.TakeoutWatchInterval)
	}
	if !kc.TakeoutArchiveProcessed {
		t.Error("TakeoutArchiveProcessed should be true")
	}
	if kc.DefaultTier != "full" {
		t.Errorf("DefaultTier = %q, want full", kc.DefaultTier)
	}
	if len(kc.LabelsFilter) != 2 || kc.LabelsFilter[0] != "Work" || kc.LabelsFilter[1] != "Personal" {
		t.Errorf("LabelsFilter = %v, want [Work Personal]", kc.LabelsFilter)
	}
}

// --- Config parsing edge cases ---

func TestParseKeepConfigEmptySourceConfig(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	}
	kc, err := parseKeepConfig(cfg)
	if err != nil {
		t.Fatalf("parseKeepConfig with empty config: %v", err)
	}
	if kc.SyncMode != SyncModeTakeout {
		t.Errorf("default SyncMode = %q, want takeout", kc.SyncMode)
	}
	if kc.GkeepPollInterval != 60*time.Minute {
		t.Errorf("default GkeepPollInterval = %v, want 60m", kc.GkeepPollInterval)
	}
	if kc.MinContentLength != 0 {
		t.Errorf("default MinContentLength = %d, want 0", kc.MinContentLength)
	}
	if kc.IncludeArchived != false {
		t.Error("default IncludeArchived should be false")
	}
}

func TestParseKeepConfigInvalidWatchInterval(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"watch_interval": "not-a-duration",
		},
	}
	_, err := parseKeepConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid watch_interval")
	}
}

func TestParseKeepConfigInvalidPollIntervalFormat(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"poll_interval": "abc",
		},
	}
	_, err := parseKeepConfig(cfg)
	if err == nil {
		t.Error("expected error for unparseable poll_interval")
	}
}

func TestParseKeepConfigTypeMismatchesUseDefaults(t *testing.T) {
	// When type assertions fail, the fields should keep defaults (no panic)
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"gkeep_enabled":      "not-a-bool",   // string, not bool
			"include_archived":   42,             // int, not bool
			"min_content_length": "not-a-number", // string, not float64
		},
	}
	kc, err := parseKeepConfig(cfg)
	if err != nil {
		t.Fatalf("type mismatches should not cause error: %v", err)
	}
	// All mismatched fields should stay at defaults
	if kc.GkeepEnabled != false {
		t.Errorf("GkeepEnabled = %v, want false (default on type mismatch)", kc.GkeepEnabled)
	}
	if kc.IncludeArchived != false {
		t.Errorf("IncludeArchived = %v, want false (default on type mismatch)", kc.IncludeArchived)
	}
	if kc.MinContentLength != 0 {
		t.Errorf("MinContentLength = %d, want 0 (default on type mismatch)", kc.MinContentLength)
	}
}

// --- Connect edge cases ---

func TestConnectEmptyImportDirString(t *testing.T) {
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig("", "takeout", false, false))
	if err == nil {
		t.Fatal("expected error for empty import dir string in takeout mode")
	}
	if c.Health(context.Background()) != "error" {
		t.Errorf("health = %q, want error", c.Health(context.Background()))
	}
}

func TestConnectGkeepWithAck(t *testing.T) {
	c := New("google-keep")
	// gkeepapi mode with warning acknowledged — Connect should succeed
	// (no import dir needed since gkeepapi doesn't use it)
	err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true))
	if err != nil {
		t.Fatalf("Connect gkeepapi with ack: %v", err)
	}
	if c.Health(context.Background()) != "healthy" {
		t.Errorf("health = %q, want healthy", c.Health(context.Background()))
	}
}

func TestConnectHybridModeMissingDir(t *testing.T) {
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig("", "hybrid", true, true))
	if err == nil {
		t.Fatal("expected error for hybrid mode with empty import dir")
	}
}

func TestConnectHybridModeValid(t *testing.T) {
	dir := t.TempDir()
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig(dir, "hybrid", true, true))
	if err != nil {
		t.Fatalf("Connect hybrid: %v", err)
	}
	if c.Health(context.Background()) != "healthy" {
		t.Errorf("health = %q, want healthy", c.Health(context.Background()))
	}
}

// --- Sync mode and health transition tests ---

func TestSyncGkeepModeReportsError(t *testing.T) {
	// gkeepapi mode always fails (bridge stub) — health should reflect that
	c := New("google-keep")
	err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync should not return fatal error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("artifacts = %d, want 0 (gkeepapi bridge not connected)", len(artifacts))
	}
	// Health should transition to degraded on first failure (graduated escalation)
	if c.Health(context.Background()) != "degraded" {
		t.Errorf("health = %q, want degraded (gkeepapi failed — first failure)", c.Health(context.Background()))
	}
}

func TestSyncHybridModeTakeoutSucceedsGkeepFails(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "hybrid-note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Hybrid mode note with enough content for filters to pass",
		"title": "Hybrid Test", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":            "hybrid",
			"import_dir":           dir,
			"gkeep_enabled":        true,
			"warning_acknowledged": true,
		},
	}

	c := New("google-keep")
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Hybrid mode: takeout succeeds, gkeepapi fails gracefully
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("artifacts = %d, want 1 (from takeout)", len(artifacts))
	}
}

func TestHealthErrorToCloseTransition(t *testing.T) {
	ctx := context.Background()
	c := New("google-keep")

	// Force error state via bad config
	_ = c.Connect(ctx, testConnectorConfig("", "invalid_mode", false, false))
	// connect returns error, health set to error from parse failure
	if c.Health(ctx) != "error" {
		t.Errorf("after bad connect: health = %q, want error", c.Health(ctx))
	}

	// Close from error state → disconnected
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.Health(ctx) != "disconnected" {
		t.Errorf("after close from error: health = %q, want disconnected", c.Health(ctx))
	}
}

func TestCloseIdempotent(t *testing.T) {
	c := New("google-keep")
	// Close on a never-connected connector
	if err := c.Close(); err != nil {
		t.Fatalf("Close on disconnected: %v", err)
	}
	if c.Health(context.Background()) != "disconnected" {
		t.Errorf("health = %q, want disconnected", c.Health(context.Background()))
	}
	// Close again
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestSyncSetsHealthToSyncing(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "sync-health.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Note to verify syncing health transition during sync",
		"title": "Sync Health", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// After sync completes, health should be healthy (no errors)
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if c.Health(context.Background()) != "healthy" {
		t.Errorf("after sync health = %q, want healthy", c.Health(context.Background()))
	}
}

func TestCursorPrecisionPreventsDuplicates(t *testing.T) {
	dir := t.TempDir()
	// Timestamp with sub-second precision: 1712000000123456 usec
	writeTestJSON(t, dir, "precise-note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Note with sub-second timestamp to test cursor precision",
		"title": "Precision Test", "userEditedTimestampUsec": 1712000000123456,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts1, cursor1, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync 1: %v", err)
	}
	if len(artifacts1) != 1 {
		t.Fatalf("first sync: artifacts = %d, want 1", len(artifacts1))
	}

	// Second sync with cursor from first — sub-second precision must prevent re-processing
	artifacts2, _, err := c.Sync(context.Background(), cursor1)
	if err != nil {
		t.Fatalf("Sync 2: %v", err)
	}
	if len(artifacts2) != 0 {
		t.Errorf("second sync: artifacts = %d, want 0 (cursor precision should prevent re-processing)", len(artifacts2))
	}
}

func TestHealthGraduatedEscalation(t *testing.T) {
	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// First failure → degraded
	c.Sync(context.Background(), "")
	if c.Health(context.Background()) != "degraded" {
		t.Errorf("after 1 failure: health = %q, want degraded", c.Health(context.Background()))
	}

	// Failures 2-4 → still degraded
	for i := 2; i <= 4; i++ {
		c.Sync(context.Background(), "")
		if c.Health(context.Background()) != "degraded" {
			t.Errorf("after %d failures: health = %q, want degraded", i, c.Health(context.Background()))
		}
	}

	// Failure 5 → failing
	c.Sync(context.Background(), "")
	if c.Health(context.Background()) != "failing" {
		t.Errorf("after 5 failures: health = %q, want failing", c.Health(context.Background()))
	}

	// Failures 6-9 → still failing
	for i := 6; i <= 9; i++ {
		c.Sync(context.Background(), "")
		if c.Health(context.Background()) != "failing" {
			t.Errorf("after %d failures: health = %q, want failing", i, c.Health(context.Background()))
		}
	}

	// Failure 10 → error
	c.Sync(context.Background(), "")
	if c.Health(context.Background()) != "error" {
		t.Errorf("after 10 failures: health = %q, want error", c.Health(context.Background()))
	}
}

func TestHealthRecoveryAfterFailures(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "recovery-note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Note for health recovery test with enough content",
		"title": "Recovery", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	c := New("google-keep")

	// Start in gkeepapi mode to induce failures
	if err := c.Connect(context.Background(), testConnectorConfig("", "gkeepapi", true, true)); err != nil {
		t.Fatalf("Connect gkeepapi: %v", err)
	}

	// Build up 6 consecutive failures → failing state
	for i := 0; i < 6; i++ {
		c.Sync(context.Background(), "")
	}
	if c.Health(context.Background()) != "failing" {
		t.Fatalf("after 6 failures: health = %q, want failing", c.Health(context.Background()))
	}

	// Reconnect with working takeout config
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect takeout: %v", err)
	}

	// Successful sync should recover to healthy and reset consecutive error counter
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if c.Health(context.Background()) != "healthy" {
		t.Errorf("after recovery sync: health = %q, want healthy", c.Health(context.Background()))
	}
}

func TestHealthDegradedOnPartialFailure(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "valid.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Valid note content that is long enough to pass filters easily",
		"title": "Valid", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)
	writeTestJSON(t, dir, "corrupt.json", `{invalid json that cannot be parsed}`)

	c := New("google-keep")
	if err := c.Connect(context.Background(), testConnectorConfig(dir, "takeout", false, false)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("artifacts = %d, want 1", len(artifacts))
	}

	// Partial failure: 1 artifact produced, 1 parse error → degraded (not error)
	if c.Health(context.Background()) != "degraded" {
		t.Errorf("health = %q, want degraded (partial failure)", c.Health(context.Background()))
	}
}

func TestHybridModeCountsErrorsFromBothSources(t *testing.T) {
	dir := t.TempDir()
	writeTestJSON(t, dir, "note.json", `{
		"color": "DEFAULT", "isTrashed": false, "isPinned": false, "isArchived": false,
		"textContent": "Hybrid mode note for error counting verification test",
		"title": "Hybrid Error Count", "userEditedTimestampUsec": 1712000000000000,
		"createdTimestampUsec": 1711900000000000,
		"labels": [], "annotations": [], "attachments": [], "listContent": [], "sharees": []
	}`)

	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"sync_mode":            "hybrid",
			"import_dir":           dir,
			"gkeep_enabled":        true,
			"warning_acknowledged": true,
		},
	}

	c := New("google-keep")
	if err := c.Connect(context.Background(), cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Takeout succeeds (1 artifact), gkeep fails → partial success → degraded
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Errorf("artifacts = %d, want 1", len(artifacts))
	}
	if c.Health(context.Background()) != "degraded" {
		t.Errorf("health = %q, want degraded (hybrid with gkeep failure)", c.Health(context.Background()))
	}
}
