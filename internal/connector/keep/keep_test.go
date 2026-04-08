package keep

import (
	"context"
	"testing"

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
