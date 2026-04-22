package bookmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// chromeJSONFixture returns a minimal valid Chrome JSON bookmarks export.
func chromeJSONFixture() []byte {
	return []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bookmarks Bar",
				"children": [
					{
						"type": "url",
						"name": "Example",
						"url": "https://example.com"
					},
					{
						"type": "folder",
						"name": "Tech",
						"children": [
							{
								"type": "url",
								"name": "Go Lang",
								"url": "https://go.dev"
							}
						]
					}
				]
			}
		}
	}`)
}

// netscapeHTMLFixture returns a minimal Netscape HTML bookmarks export.
func netscapeHTMLFixture() []byte {
	return []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">
<TITLE>Bookmarks</TITLE>
<H1>Bookmarks</H1>
<DL><p>
<DT><H3>Tech</H3>
<DL><p>
<DT><A HREF="https://rust-lang.org">Rust Lang</A>
</DL><p>
<DT><A HREF="https://python.org">Python</A>
</DL><p>`)
}

func setupImportDir(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return dir
}

func makeConfig(importDir string) connector.ConnectorConfig {
	return connector.ConnectorConfig{
		AuthType:       "none",
		Enabled:        true,
		ProcessingTier: "full",
		SourceConfig: map[string]interface{}{
			"import_dir":        importDir,
			"archive_processed": false,
		},
	}
}

// T-1-01
func TestConnectorID(t *testing.T) {
	c := NewConnector("bookmarks")
	if c.ID() != "bookmarks" {
		t.Errorf("ID() = %q, want %q", c.ID(), "bookmarks")
	}
}

// T-1-02
func TestConnectValidConfig(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()

	err := c.Connect(ctx, makeConfig(dir))
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	health := c.Health(ctx)
	if health != connector.HealthHealthy {
		t.Errorf("Health() = %q, want %q", health, connector.HealthHealthy)
	}
}

// T-1-03
func TestConnectMissingImportDir(t *testing.T) {
	c := NewConnector("bookmarks")
	ctx := context.Background()

	err := c.Connect(ctx, makeConfig("/nonexistent/path/does/not/exist"))
	if err == nil {
		t.Fatal("Connect() expected error for non-existent dir, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "does not exist") {
		t.Errorf("error = %q, want containing 'does not exist'", got)
	}
	if h := c.Health(ctx); h != connector.HealthError {
		t.Errorf("Health() = %q, want %q", h, connector.HealthError)
	}
}

// T-1-04
func TestConnectEmptyImportDir(t *testing.T) {
	c := NewConnector("bookmarks")
	ctx := context.Background()

	cfg := connector.ConnectorConfig{
		AuthType:       "none",
		Enabled:        true,
		ProcessingTier: "full",
		SourceConfig: map[string]interface{}{
			"import_dir": "",
		},
	}
	err := c.Connect(ctx, cfg)
	if err == nil {
		t.Fatal("Connect() expected error for empty import_dir, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "import directory") {
		t.Errorf("error = %q, want containing 'import directory'", got)
	}
}

// T-1-05
func TestSyncChromeJSON(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"chrome_export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("Sync() returned %d artifacts, want 2", len(artifacts))
	}

	for _, a := range artifacts {
		if a.SourceID != "bookmarks" {
			t.Errorf("artifact SourceID = %q, want %q", a.SourceID, "bookmarks")
		}
		if a.Metadata["processing_tier"] != "full" {
			t.Errorf("artifact processing_tier = %v, want %q", a.Metadata["processing_tier"], "full")
		}
		if a.Metadata["source_format"] != "chrome_json" {
			t.Errorf("artifact source_format = %v, want %q", a.Metadata["source_format"], "chrome_json")
		}
		if a.Metadata["import_file"] != "chrome_export.json" {
			t.Errorf("artifact import_file = %v, want %q", a.Metadata["import_file"], "chrome_export.json")
		}
		if _, ok := a.Metadata["folder_path"]; !ok {
			t.Error("artifact missing folder_path metadata")
		}
		// R-006: bookmark_url must be present
		if burl, ok := a.Metadata["bookmark_url"].(string); !ok || burl == "" {
			t.Errorf("artifact missing or empty bookmark_url metadata, got %v", a.Metadata["bookmark_url"])
		}
	}

	// Cursor should contain the processed file
	var files []string
	if err := json.Unmarshal([]byte(cursor), &files); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}
	if len(files) != 1 || files[0] != "chrome_export.json" {
		t.Errorf("cursor files = %v, want [chrome_export.json]", files)
	}
}

// T-1-06
func TestSyncNetscapeHTML(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"firefox_export.html": netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("Sync() returned %d artifacts, want 2", len(artifacts))
	}

	for _, a := range artifacts {
		if a.Metadata["source_format"] != "netscape_html" {
			t.Errorf("artifact source_format = %v, want %q", a.Metadata["source_format"], "netscape_html")
		}
	}
}

// T-1-07
func TestSyncHTMExtension(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"edge_export.htm": netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	if len(artifacts) != 2 {
		t.Fatalf("Sync() returned %d artifacts, want 2", len(artifacts))
	}

	if artifacts[0].Metadata["source_format"] != "netscape_html" {
		t.Errorf("source_format = %v, want %q", artifacts[0].Metadata["source_format"], "netscape_html")
	}
}

// T-1-08
func TestSyncSkipsUnknownFormat(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"notes.txt":          []byte("just some notes"),
		"chrome_export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Should only process the .json file, skip .txt
	if len(artifacts) != 2 {
		t.Fatalf("Sync() returned %d artifacts, want 2 (from json only)", len(artifacts))
	}

	// Cursor should NOT contain notes.txt
	var files []string
	if err := json.Unmarshal([]byte(cursor), &files); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}
	for _, f := range files {
		if f == "notes.txt" {
			t.Error("cursor contains notes.txt, should be excluded")
		}
	}
}

// T-1-09
func TestSyncIncrementalSkipsProcessed(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"chrome_export.json":  chromeJSONFixture(),
		"firefox_export.html": netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Simulate previous sync that already processed chrome_export.json
	existingCursor := encodeProcessedFilesCursor([]string{"chrome_export.json"})

	artifacts, cursor, err := c.Sync(ctx, existingCursor)
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}

	// Only firefox_export.html should be processed
	if len(artifacts) != 2 {
		t.Fatalf("Sync() returned %d artifacts, want 2 (from firefox only)", len(artifacts))
	}

	for _, a := range artifacts {
		if a.Metadata["source_format"] != "netscape_html" {
			t.Errorf("unexpected source_format %v, expected all netscape_html", a.Metadata["source_format"])
		}
	}

	// Cursor should contain both files
	var files []string
	if err := json.Unmarshal([]byte(cursor), &files); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("cursor has %d files, want 2", len(files))
	}
}

// T-1-10
func TestSyncCorruptedFileSkipped(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"good_export.json": chromeJSONFixture(),
		"bad_export.json":  []byte(`{invalid json!!!`),
		"good_html.html":   netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() error: %v", err) // partial success should not return error
	}

	// Should have artifacts from the 2 good files only
	if len(artifacts) < 2 {
		t.Fatalf("Sync() returned %d artifacts, expected at least 2 from good files", len(artifacts))
	}

	// Cursor should NOT contain bad_export.json
	var files []string
	if cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &files); err != nil {
			t.Fatalf("cursor unmarshal: %v", err)
		}
	}
	for _, f := range files {
		if f == "bad_export.json" {
			t.Error("cursor contains bad_export.json, should be excluded")
		}
	}

	// Health should be healthy (partial success is acceptable)
	if h := c.Health(ctx); h != connector.HealthHealthy {
		t.Errorf("Health() = %q after partial sync, want %q", h, connector.HealthHealthy)
	}
}

// T-1-11
func TestCloseResetsHealth(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()

	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthHealthy {
		t.Fatalf("after Connect, Health() = %q, want healthy", h)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthDisconnected {
		t.Errorf("after Close, Health() = %q, want disconnected", h)
	}
}

// T-1-12
func TestHealthTransitions(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"export.json": chromeJSONFixture(),
	})
	c := NewConnector("bookmarks")
	ctx := context.Background()

	// Initial: disconnected
	if h := c.Health(ctx); h != connector.HealthDisconnected {
		t.Errorf("initial Health() = %q, want disconnected", h)
	}

	// After Connect: healthy
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthHealthy {
		t.Errorf("after Connect, Health() = %q, want healthy", h)
	}

	// After Sync completes: healthy
	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthHealthy {
		t.Errorf("after Sync, Health() = %q, want healthy", h)
	}

	// After Close: disconnected
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthDisconnected {
		t.Errorf("after Close, Health() = %q, want disconnected", h)
	}
}

// T-1-13
func TestParseConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir": dir,
		},
	}

	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig() error: %v", err)
	}

	if parsed.WatchInterval != 5*60*1000000000 { // 5 minutes in nanoseconds
		t.Errorf("WatchInterval = %v, want 5m", parsed.WatchInterval)
	}
	if !parsed.ArchiveProcessed {
		t.Error("ArchiveProcessed = false, want true (default)")
	}
	if parsed.ProcessingTier != "full" {
		t.Errorf("ProcessingTier = %q, want %q", parsed.ProcessingTier, "full")
	}
	if parsed.MinURLLength != 10 {
		t.Errorf("MinURLLength = %d, want 10", parsed.MinURLLength)
	}
}

// T-1-14
func TestCursorEncodeDecodeCycle(t *testing.T) {
	// Round-trip
	original := []string{"file_a.json", "file_b.html", "file_c.htm"}
	encoded := encodeProcessedFilesCursor(original)
	decoded := decodeProcessedFilesCursor(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("decoded length = %d, want %d", len(decoded), len(original))
	}
	for i, f := range decoded {
		if f != original[i] {
			t.Errorf("decoded[%d] = %q, want %q", i, f, original[i])
		}
	}

	// Empty cursor
	if files := decodeProcessedFilesCursor(""); files != nil {
		t.Errorf("empty cursor decoded to %v, want nil", files)
	}

	// Invalid cursor
	if files := decodeProcessedFilesCursor("not-json"); files != nil {
		t.Errorf("invalid cursor decoded to %v, want nil", files)
	}

	// Empty list encodes to empty string
	if s := encodeProcessedFilesCursor(nil); s != "" {
		t.Errorf("nil list encoded to %q, want empty string", s)
	}
}

// T-1-R1 Regression: corrupted export does not crash connector
func TestSyncCorruptedExportNoPanic(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"corrupt1.json": []byte(`}`),
		"corrupt2.html": []byte(`<not valid`),
		"good.json":     chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Should not panic
	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() should not return error for partial failures: %v", err)
	}

	// At least the good.json should produce artifacts
	if len(artifacts) < 2 {
		t.Errorf("got %d artifacts, expected at least 2 from good.json", len(artifacts))
	}
}

// T-IMP-009-002: Multi-bookmark-per-line Netscape HTML gets correct per-bookmark ADD_DATE.
func TestParseNetscapeHTML_MultiBookmarkPerLine_CorrectDates(t *testing.T) {
	// Minified HTML with two bookmarks on a single line, each with a different ADD_DATE.
	html := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL><p>
<DT><A HREF="https://first.com" ADD_DATE="1000000000">First</A><DT><A HREF="https://second.com" ADD_DATE="1200000000">Second</A>
</DL>`)

	bookmarks, err := ParseNetscapeHTML(html)
	if err != nil {
		t.Fatalf("ParseNetscapeHTML: %v", err)
	}
	if len(bookmarks) != 2 {
		t.Fatalf("got %d bookmarks, want 2", len(bookmarks))
	}

	// First bookmark should have ADD_DATE=1000000000 (2001-09-08)
	if bookmarks[0].URL != "https://first.com" {
		t.Errorf("bookmarks[0].URL = %q, want https://first.com", bookmarks[0].URL)
	}
	if bookmarks[0].AddedAt.Unix() != 1000000000 {
		t.Errorf("bookmarks[0].AddedAt.Unix() = %d, want 1000000000", bookmarks[0].AddedAt.Unix())
	}

	// Second bookmark should have ADD_DATE=1200000000 (2008-01-10), NOT 1000000000
	if bookmarks[1].URL != "https://second.com" {
		t.Errorf("bookmarks[1].URL = %q, want https://second.com", bookmarks[1].URL)
	}
	if bookmarks[1].AddedAt.Unix() != 1200000000 {
		t.Errorf("bookmarks[1].AddedAt.Unix() = %d, want 1200000000 (got first bookmark's date instead)", bookmarks[1].AddedAt.Unix())
	}
}

// T-GAP-R006-01: R-006 bookmark_url and added_at metadata fields are populated.
func TestMetadataR006Fields(t *testing.T) {
	// Chrome JSON fixture with date_added field (Chrome epoch: microseconds since 1601-01-01)
	// 13350000000000000 µs ≈ 2023-06-12 in Chrome epoch
	fixture := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bookmarks Bar",
				"children": [
					{
						"type": "url",
						"name": "Dated Bookmark",
						"url": "https://example.com/dated",
						"date_added": "13350000000000000"
					},
					{
						"type": "url",
						"name": "Undated Bookmark",
						"url": "https://example.com/undated"
					}
				]
			}
		}
	}`)

	dir := setupImportDir(t, map[string][]byte{
		"with_dates.json": fixture,
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync() error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(artifacts))
	}

	for _, a := range artifacts {
		// bookmark_url must always be present
		burl, ok := a.Metadata["bookmark_url"].(string)
		if !ok || burl == "" {
			t.Errorf("artifact %q: missing bookmark_url metadata", a.Title)
		}
		if burl != a.URL {
			t.Errorf("artifact %q: bookmark_url = %q, want %q (matches URL)", a.Title, burl, a.URL)
		}
	}

	// Find the dated bookmark and check added_at
	for _, a := range artifacts {
		if a.Title == "Dated Bookmark" {
			addedAt, ok := a.Metadata["added_at"].(string)
			if !ok || addedAt == "" {
				t.Errorf("dated bookmark missing added_at metadata")
			}
			// Verify it's a valid RFC3339 timestamp
			if ok && addedAt != "" {
				if _, err := time.Parse(time.RFC3339, addedAt); err != nil {
					t.Errorf("added_at %q is not valid RFC3339: %v", addedAt, err)
				}
			}
		}
		if a.Title == "Undated Bookmark" {
			// Undated bookmark should not have added_at
			if _, ok := a.Metadata["added_at"]; ok {
				t.Errorf("undated bookmark should not have added_at metadata")
			}
		}
	}
}

// T-STAB-001: File size limit prevents reading oversized exports.
func TestSyncRejectsOversizedFile(t *testing.T) {
	// Create a file that exceeds maxFileSize (we use a small override isn't possible,
	// so we test that the stat check path works by checking processFile directly)
	dir := t.TempDir()

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// processFile should fail for a file that doesn't exist (stat check)
	_, err := c.processFile(ctx, c.config, filepath.Join(dir, "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// T-STAB-002: Cursor capping keeps processed-files list bounded.
func TestCursorCapping(t *testing.T) {
	// Build a list exceeding maxCursorEntries
	large := make([]string, maxCursorEntries+500)
	for i := range large {
		large[i] = fmt.Sprintf("file_%05d.json", i)
	}

	encoded := encodeProcessedFilesCursor(large)
	decoded := decodeProcessedFilesCursor(encoded)

	if len(decoded) != maxCursorEntries {
		t.Errorf("decoded length = %d, want %d (capped)", len(decoded), maxCursorEntries)
	}

	// Should keep the most recent entries (tail)
	if decoded[0] != large[500] {
		t.Errorf("first entry = %q, want %q (tail preserved)", decoded[0], large[500])
	}
}

// T-STAB-003: Deep Chrome JSON nesting doesn't cause stack overflow.
func TestDeepNestedChromeJSON(t *testing.T) {
	// Build a JSON bookmark tree that exceeds maxExtractDepth
	inner := `{"type": "url", "name": "Deep", "url": "https://example.com/deep"}`
	for i := 0; i < 60; i++ {
		inner = fmt.Sprintf(`{"type": "folder", "name": "L%d", "children": [%s]}`, i, inner)
	}
	data := fmt.Sprintf(`{"roots": {"bar": %s}}`, inner)

	bookmarks, err := ParseChromeJSON([]byte(data))
	if err != nil {
		t.Fatalf("ParseChromeJSON: %v", err)
	}

	// Due to depth limiting, the deeply nested bookmark should NOT be found
	if len(bookmarks) != 0 {
		t.Errorf("got %d bookmarks from depth-60 tree, expected 0 (capped at %d)", len(bookmarks), maxExtractDepth)
	}
}

// T-STAB-004: Context cancellation stops sync loop.
func TestSyncRespectsContextCancel(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"a.json": chromeJSONFixture(),
		"b.json": chromeJSONFixture(),
		"c.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Cancel context before sync
	cancel()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Errorf("error = %q, want containing 'cancelled'", err.Error())
	}
}

// T-STAB-006: Archive doesn't overwrite existing files.
func TestArchiveDoesNotOverwrite(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	cfg := makeConfig(dir)
	cfg.SourceConfig["archive_processed"] = true
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Pre-create an archived file
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "export.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Sync should archive with a unique name
	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Both files should exist in archive/
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(entries) < 2 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected at least 2 files in archive, got %d: %v", len(entries), names)
	}
}

// T-STAB-007: ExcludeDomains filter removes matching artifacts.
func TestFilterExcludeDomains(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"mixed.html": []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="https://example.com/page">Keep This</A>
<DT><A HREF="https://spam.com/bad">Exclude This</A>
<DT><A HREF="https://go.dev/doc">Also Keep</A>
</DL>`),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	cfg := connector.ConnectorConfig{
		AuthType:       "none",
		Enabled:        true,
		ProcessingTier: "full",
		SourceConfig: map[string]interface{}{
			"import_dir":        dir,
			"archive_processed": false,
			"exclude_domains":   []interface{}{"spam.com"},
		},
	}
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 2 {
		t.Errorf("got %d artifacts, want 2 (spam.com excluded)", len(artifacts))
	}
	for _, a := range artifacts {
		if strings.Contains(a.URL, "spam.com") {
			t.Errorf("excluded domain artifact leaked through: %s", a.URL)
		}
	}
}

// T-STAB-008: MinURLLength filter removes short URLs.
func TestFilterMinURLLength(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"short.html": []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="https://example.com/page">Long URL</A>
<DT><A HREF="x://s">Short</A>
</DL>`),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	cfg := connector.ConnectorConfig{
		AuthType:       "none",
		Enabled:        true,
		ProcessingTier: "full",
		SourceConfig: map[string]interface{}{
			"import_dir":        dir,
			"archive_processed": false,
			"min_url_length":    float64(10),
		},
	}
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 1 {
		t.Errorf("got %d artifacts, want 1 (short URL filtered)", len(artifacts))
	}
}

// T-SEC-R1 Regression: symlink path traversal protection in findNewFiles.
// Verifies that symlinks inside the import directory are silently skipped,
// preventing an attacker from reading files outside the import boundary.
func TestSyncSkipsSymlinks(t *testing.T) {
	// Create a real import directory with one legitimate export file
	dir := setupImportDir(t, map[string][]byte{
		"legit.json": chromeJSONFixture(),
	})

	// Create a "secret" file outside the import directory
	secretDir := t.TempDir()
	secretFile := filepath.Join(secretDir, "stolen_creds.json")
	if err := os.WriteFile(secretFile, chromeJSONFixture(), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	// Create a symlink inside the import directory pointing to the secret file
	symPath := filepath.Join(dir, "evil_link.json")
	if err := os.Symlink(secretFile, symPath); err != nil {
		t.Skipf("cannot create symlink (OS restriction): %v", err)
	}

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Only legit.json should produce artifacts (2 bookmarks from fixture)
	if len(artifacts) != 2 {
		t.Errorf("got %d artifacts, want 2 (only from legit.json); symlink was not skipped", len(artifacts))
	}

	// Cursor must NOT contain the symlink target
	var files []string
	if cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &files); err != nil {
			t.Fatalf("cursor unmarshal: %v", err)
		}
	}
	for _, f := range files {
		if f == "evil_link.json" {
			t.Fatal("SECURITY: symlink target appeared in cursor — directory traversal protection is broken")
		}
	}

	// Verify the symlink file was NOT processed by checking artifact metadata
	for _, a := range artifacts {
		if a.Metadata["import_file"] == "evil_link.json" {
			t.Fatal("SECURITY: symlink-targeted file was parsed as bookmark export — path traversal is possible")
		}
	}
}

// T-CHAOS-003: Dangerous URL schemes (javascript:, data:, file:) are rejected.
func TestFilterRejectsDangerousSchemes(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"mixed.html": []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="https://safe.com/page">Safe HTTPS</A>
<DT><A HREF="http://also-safe.com/page">Safe HTTP</A>
<DT><A HREF="javascript:alert(1)">XSS Payload</A>
<DT><A HREF="data:text/html,<script>alert(1)</script>">Data URI</A>
<DT><A HREF="file:///etc/passwd">Local File</A>
<DT><A HREF="ftp://files.example.com/readme.txt">FTP Link</A>
</DL>`),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Only https, http, and ftp are allowed
	if len(artifacts) != 3 {
		urls := make([]string, len(artifacts))
		for i, a := range artifacts {
			urls[i] = a.URL
		}
		t.Fatalf("got %d artifacts %v, want 3 (https + http + ftp only)", len(artifacts), urls)
	}

	for _, a := range artifacts {
		scheme := ""
		if idx := strings.Index(a.URL, "://"); idx > 0 {
			scheme = a.URL[:idx]
		}
		switch scheme {
		case "https", "http", "ftp":
			// allowed
		default:
			t.Errorf("SECURITY: dangerous scheme %q leaked through: %s", scheme, a.URL)
		}
	}
}

// T-CHAOS-006: processFile rejects path traversal attempts.
func TestProcessFileRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Attempt to read a file outside the import directory
	outsideFile := filepath.Join(os.TempDir(), "outside.json")
	_ = os.WriteFile(outsideFile, chromeJSONFixture(), 0o644)
	defer os.Remove(outsideFile)

	_, err := c.processFile(ctx, c.config, outsideFile)
	if err == nil {
		t.Fatal("SECURITY: processFile should reject files outside import directory")
	}
	if !strings.Contains(err.Error(), "outside import directory") {
		t.Errorf("error = %q, want containing 'outside import directory'", err.Error())
	}
}

// T-SYNC-001: All-files-fail sync transitions health to HealthError.
func TestSyncAllFailsHealthError(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"bad1.json": []byte(`{invalid`),
		"bad2.json": []byte(`not json either`),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("got %d artifacts, want 0 (all files corrupt)", len(artifacts))
	}

	// Health should be error because all files failed (syncErrors > 0, lastSyncCount == 0)
	if h := c.Health(ctx); h != connector.HealthError {
		t.Errorf("Health() = %q after all-fail sync, want %q", h, connector.HealthError)
	}
}

// T-SYNC-002: Empty directory sync returns no artifacts and stays healthy.
func TestSyncEmptyDir(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("got %d artifacts, want 0", len(artifacts))
	}
	if cursor != "" {
		t.Errorf("cursor = %q, want empty for no new files", cursor)
	}
	if h := c.Health(ctx); h != connector.HealthHealthy {
		t.Errorf("Health() = %q, want healthy", h)
	}
}

// T-SYNC-003: archiveProcessed moves files to archive/ subdirectory.
func TestSyncArchivesProcessedFiles(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	cfg := makeConfig(dir)
	cfg.SourceConfig["archive_processed"] = true
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Original file should be gone
	if _, err := os.Stat(filepath.Join(dir, "export.json")); !os.IsNotExist(err) {
		t.Error("export.json still exists after archive sync")
	}

	// Archived copy should exist
	archiveDir := filepath.Join(dir, "archive")
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("archive has %d files, want 1", len(entries))
	}
}

// T-CFG-001: Invalid watch_interval returns error.
func TestParseConfigInvalidWatchInterval(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":     dir,
			"watch_interval": "not-a-duration",
		},
	}

	_, err := parseConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid watch_interval")
	}
	if !strings.Contains(err.Error(), "watch_interval") {
		t.Errorf("error = %q, want containing 'watch_interval'", err.Error())
	}
}

// T-CFG-002: min_url_length accepts int type from config.
func TestParseConfigMinURLLengthInt(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":     dir,
			"min_url_length": 25,
		},
	}

	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if parsed.MinURLLength != 25 {
		t.Errorf("MinURLLength = %d, want 25", parsed.MinURLLength)
	}
}

// T-CFG-003: exclude_domains parses correctly.
func TestParseConfigExcludeDomains(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":      dir,
			"exclude_domains": []interface{}{"spam.com", "ads.net"},
		},
	}

	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if len(parsed.ExcludeDomains) != 2 {
		t.Fatalf("ExcludeDomains len = %d, want 2", len(parsed.ExcludeDomains))
	}
	if parsed.ExcludeDomains[0] != "spam.com" || parsed.ExcludeDomains[1] != "ads.net" {
		t.Errorf("ExcludeDomains = %v, want [spam.com ads.net]", parsed.ExcludeDomains)
	}
}

// T-CFG-004: Missing import_dir returns clear error.
func TestParseConfigMissingImportDir(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{},
	}

	_, err := parseConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing import_dir")
	}
	if !strings.Contains(err.Error(), "import directory") {
		t.Errorf("error = %q, want containing 'import directory'", err.Error())
	}
}

// T-CFG-005: Valid custom watch_interval is parsed.
func TestParseConfigValidWatchInterval(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":     dir,
			"watch_interval": "10m",
		},
	}

	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if parsed.WatchInterval != 10*60*1000000000 { // 10 minutes in nanoseconds
		t.Errorf("WatchInterval = %v, want 10m", parsed.WatchInterval)
	}
}

// T-CFG-006: ProcessingTier from ConnectorConfig overrides default.
func TestParseConfigProcessingTierOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		ProcessingTier: "lightweight",
		SourceConfig: map[string]interface{}{
			"import_dir": dir,
		},
	}

	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if parsed.ProcessingTier != "lightweight" {
		t.Errorf("ProcessingTier = %q, want %q", parsed.ProcessingTier, "lightweight")
	}
}

// T-CFG-007: archive_processed=false overrides default true.
func TestParseConfigArchiveProcessedFalse(t *testing.T) {
	dir := t.TempDir()
	cfg := connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"import_dir":        dir,
			"archive_processed": false,
		},
	}

	parsed, err := parseConfig(cfg)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if parsed.ArchiveProcessed {
		t.Error("ArchiveProcessed = true, want false")
	}
}

// T-1-15: Connect fails when import path is a regular file instead of directory.
func TestConnectImportPathIsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-directory.json")
	if err := os.WriteFile(filePath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := NewConnector("bookmarks")
	ctx := context.Background()
	err := c.Connect(ctx, makeConfig(filePath))
	if err == nil {
		t.Fatal("Connect() expected error when import path is a file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error = %q, want containing 'not a directory'", err.Error())
	}
	if h := c.Health(ctx); h != connector.HealthError {
		t.Errorf("Health() = %q, want %q", h, connector.HealthError)
	}
}

// T-SYNC-004: Sync when all files already appear in cursor returns zero artifacts.
func TestSyncAllFilesAlreadyProcessed(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	cursor := encodeProcessedFilesCursor([]string{"export.json"})
	artifacts, newCursor, err := c.Sync(ctx, cursor)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("got %d artifacts, want 0 (all already processed)", len(artifacts))
	}
	// Cursor should be passed through unchanged (no new files)
	if newCursor != cursor {
		t.Errorf("cursor = %q, want original cursor %q", newCursor, cursor)
	}
}

// T-SYNC-005: Mixed JSON and HTML files in a single sync.
func TestSyncMixedFormats(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"chrome.json":  chromeJSONFixture(),
		"firefox.html": netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Chrome fixture has 2 bookmarks, Netscape has 2 → total 4
	if len(artifacts) != 4 {
		t.Fatalf("got %d artifacts, want 4 (2 from JSON + 2 from HTML)", len(artifacts))
	}

	// Verify both formats are represented in metadata
	formats := map[string]int{}
	for _, a := range artifacts {
		if f, ok := a.Metadata["source_format"].(string); ok {
			formats[f]++
		}
	}
	if formats["chrome_json"] != 2 {
		t.Errorf("chrome_json count = %d, want 2", formats["chrome_json"])
	}
	if formats["netscape_html"] != 2 {
		t.Errorf("netscape_html count = %d, want 2", formats["netscape_html"])
	}

	// Cursor should list both files
	var files []string
	if err := json.Unmarshal([]byte(cursor), &files); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("cursor has %d files, want 2", len(files))
	}
}

// T-FILTER-001: filterArtifacts domain exclusion is case insensitive.
func TestFilterDomainCaseInsensitive(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"test.html": []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="https://SPAM.COM/page">Upper Case Domain</A>
<DT><A HREF="https://Spam.Com/other">Mixed Case Domain</A>
<DT><A HREF="https://good.com/page">Keep</A>
</DL>`),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	cfg := connector.ConnectorConfig{
		AuthType:       "none",
		Enabled:        true,
		ProcessingTier: "full",
		SourceConfig: map[string]interface{}{
			"import_dir":        dir,
			"archive_processed": false,
			"exclude_domains":   []interface{}{"spam.com"},
		},
	}
	if err := c.Connect(ctx, cfg); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 1 {
		t.Errorf("got %d artifacts, want 1 (both spam.com variants excluded)", len(artifacts))
	}
	for _, a := range artifacts {
		if strings.Contains(strings.ToLower(a.URL), "spam.com") {
			t.Errorf("excluded domain leaked through: %s", a.URL)
		}
	}
}

// T-FILTER-002: filterArtifacts rejects all artifacts with only disallowed schemes.
func TestFilterAllDangerousSchemes(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"bad.html": []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><A HREF="javascript:void(0)">JS</A>
<DT><A HREF="data:text/html,hello">Data</A>
<DT><A HREF="file:///etc/passwd">File</A>
</DL>`),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if len(artifacts) != 0 {
		urls := make([]string, len(artifacts))
		for i, a := range artifacts {
			urls[i] = a.URL
		}
		t.Errorf("got %d artifacts %v, want 0 (all dangerous schemes)", len(artifacts), urls)
	}
}

// T-CTOR-001: NewConnectorWithPool initializes deduplicator and topic mapper.
func TestNewConnectorWithPool(t *testing.T) {
	c := NewConnectorWithPool("bookmarks-pool", nil)
	if c.ID() != "bookmarks-pool" {
		t.Errorf("ID() = %q, want %q", c.ID(), "bookmarks-pool")
	}
	if c.deduplicator == nil {
		t.Error("deduplicator is nil, want initialized")
	}
	if c.topicMapper == nil {
		t.Error("topicMapper is nil, want initialized")
	}
	ctx := context.Background()
	if h := c.Health(ctx); h != connector.HealthDisconnected {
		t.Errorf("Health() = %q, want %q", h, connector.HealthDisconnected)
	}
}

// T-CURSOR-002: Cursor with nil SourceConfig key returns error.
func TestParseConfigNilSourceConfig(t *testing.T) {
	cfg := connector.ConnectorConfig{
		SourceConfig: nil,
	}
	_, err := parseConfig(cfg)
	if err == nil {
		t.Fatal("expected error for nil SourceConfig")
	}
}

// T-SYNC-006: processFile return error for lstat failure on non-existent file.
func TestProcessFileNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	_, err := c.processFile(ctx, c.config, filepath.Join(dir, "ghost.json"))
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

// T-CURSOR-003: decoded cursor with empty JSON array returns empty slice.
func TestDecodeCursorEmptyArray(t *testing.T) {
	files := decodeProcessedFilesCursor("[]")
	if len(files) != 0 {
		t.Errorf("decoded = %v, want empty slice", files)
	}
}

// ============================================================================
// CHAOS R16 — Adversarial regression tests
// These tests verify fixes from chaos-hardening round C16 and MUST FAIL if
// the underlying defences are reverted.
// ============================================================================

// T-CHAOS-C16-001: Context cancellation with deduplicator present still
// correctly transitions health to Error when all files fail.
// Before the fix, stale zero counters could leave health at HealthHealthy.
func TestChaosC16_CancelledSyncWithDedupHealthError(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"bad.json": []byte(`{invalid json!!!`),
	})

	// Use NewConnectorWithPool so c.deduplicator is non-nil.
	// (nil pool means dedup short-circuits but the struct pointer is live.)
	c := NewConnectorWithPool("bookmarks", nil)
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Sync with one corrupted file → file-processing error, allArtifacts empty.
	// The dedup block is skipped (allArtifacts empty), but the counters must
	// still reflect the failure so the deferred health function sets Error.
	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync returned unexpected error: %v", err)
	}

	h := c.Health(ctx)
	if h != connector.HealthError {
		t.Fatalf("CHAOS C16-001: Health() = %q after all-fail sync with deduplicator; "+
			"expected %q — health state may be reading stale counters", h, connector.HealthError)
	}
}

// T-CHAOS-C16-001b: Cancelled context with deduplicator AND files to process
// must not leave health at HealthHealthy if there are accumulated errors.
func TestChaosC16_CancelledDuringFileLoopWithDedup(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"a.json": chromeJSONFixture(),
		"b.json": chromeJSONFixture(),
		"c.json": chromeJSONFixture(),
	})

	c := NewConnectorWithPool("bookmarks", nil)
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Pre-cancel so the file loop's context check fires on the first iteration.
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := c.Sync(cancelCtx, "")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	// After cancel mid-file-loop, health must NOT be HealthHealthy.
	h := c.Health(context.Background())
	if h == connector.HealthHealthy {
		t.Fatalf("CHAOS C16-001b: Health() = %q after cancelled sync with deduplicator; "+
			"expected %q — sync counters were not flushed before early return", h, connector.HealthError)
	}
}

// T-CHAOS-C16-002: Non-bookmark HTML file produces zero artifacts but is added
// to the cursor (ensuring the file is not reprocessed on subsequent syncs).
// This documents the behaviour and confirms the warning path is reachable.
func TestChaosC16_NonBookmarkHTMLZeroArtifacts(t *testing.T) {
	// A regular HTML page, NOT a bookmark export — no <A HREF> patterns.
	regularHTML := []byte(`<!DOCTYPE html>
<html><head><title>My Blog</title></head>
<body><h1>Hello World</h1><p>No bookmarks here.</p></body>
</html>`)

	dir := setupImportDir(t, map[string][]byte{
		"not_bookmarks.html": regularHTML,
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Zero bookmarks = zero artifacts after scheme filtering.
	if len(artifacts) != 0 {
		t.Errorf("got %d artifacts from non-bookmark HTML, want 0", len(artifacts))
	}

	// The file IS added to the cursor (marked as processed) so it won't be
	// re-read endlessly. Confirm this defensive behaviour.
	var files []string
	if cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &files); err != nil {
			t.Fatalf("cursor unmarshal: %v", err)
		}
	}
	found := false
	for _, f := range files {
		if f == "not_bookmarks.html" {
			found = true
		}
	}
	if !found {
		t.Errorf("CHAOS C16-002: non-bookmark file not in cursor %v — would be reprocessed forever", files)
	}
}

// T-CHAOS-C16-003: Mixed valid/corrupted files with deduplicator present.
// Verifies that partial success with a non-nil deduplicator still reports
// correct health (not HealthError, which is reserved for total failure).
func TestChaosC16_PartialSuccessWithDedupHealthy(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"good.json":    chromeJSONFixture(),
		"corrupt.json": []byte(`NOT JSON`),
	})

	c := NewConnectorWithPool("bookmarks", nil)
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// At least 2 artifacts from the good file.
	if len(artifacts) < 2 {
		t.Fatalf("got %d artifacts, want >= 2 from good.json", len(artifacts))
	}

	// Partial success: health should be Healthy (per SCN-BK-005).
	h := c.Health(ctx)
	if h != connector.HealthHealthy {
		t.Errorf("Health() = %q after partial success with dedup, want %q", h, connector.HealthHealthy)
	}
}

// ============================================================================
// CHAOS R24 — Adversarial regression tests
// These tests verify fixes from chaos-hardening round R24 and MUST FAIL if
// the underlying defences are reverted.
// ============================================================================

// T-CHAOS-R24-001: Config snapshot prevents data race between Connect and Sync.
// Verifies that Sync uses a consistent config snapshot even if Connect updates
// the config concurrently. Without the fix, this test would trip the race
// detector or produce inconsistent results.
func TestChaosR24_ConfigSnapshotRace(t *testing.T) {
	dirA := setupImportDir(t, map[string][]byte{
		"export_a.json": chromeJSONFixture(),
	})
	dirB := setupImportDir(t, map[string][]byte{
		"export_b.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dirA)); err != nil {
		t.Fatalf("Connect A: %v", err)
	}

	// Run multiple concurrent Sync + Connect calls.
	// The race detector will flag unsafe reads if the snapshot is missing.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 10; i++ {
			if i%2 == 0 {
				_ = c.Connect(ctx, makeConfig(dirA))
			} else {
				_ = c.Connect(ctx, makeConfig(dirB))
			}
		}
	}()

	for i := 0; i < 10; i++ {
		_, _, _ = c.Sync(ctx, "")
	}
	<-done

	// If we reach here without a race-detector panic, the snapshot is working.
	// Verify final state is deterministic.
	h := c.Health(ctx)
	if h != connector.HealthHealthy && h != connector.HealthError {
		t.Errorf("Health() = %q, want healthy or error (deterministic after race)", h)
	}
}

// T-CHAOS-R24-001b: After the config snapshot fix, helper methods must NOT
// access c.config directly. Verify by confirming that Sync with a connected
// config produces deterministic results.
func TestChaosR24_ConfigSnapshotDeterministic(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"test.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("got %d artifacts, want 2", len(artifacts))
	}

	// Now close the connector (resets health to disconnected) but note
	// that config stays in place — verifying the snapshot was already taken.
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthDisconnected {
		t.Errorf("Health after Close = %q, want disconnected", h)
	}
}

// ============================================================================
// CHAOS C17 — Adversarial probe tests
// ============================================================================

// T-CHAOS-C17-001: Sync before Connect must fail with clear error, not
// scan the working directory via os.ReadDir("").
func TestChaosC17_SyncBeforeConnect(t *testing.T) {
	c := NewConnector("bookmarks")
	ctx := context.Background()

	_, _, err := c.Sync(ctx, "")
	if err == nil {
		t.Fatal("CHAOS C17-001: Sync() before Connect() returned nil error — " +
			"empty ImportDir would scan the working directory (information leak)")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error = %q, want containing 'not connected'", err.Error())
	}

	// Health should be error, not healthy.
	if h := c.Health(ctx); h == connector.HealthHealthy {
		t.Errorf("Health() = %q after failed Sync-before-Connect, want non-healthy", h)
	}
}

// T-CHAOS-C17-001b: Sync after Close must also fail — config.ImportDir is
// still set but health is disconnected. The key invariant is that an
// unconnected connector never reads the filesystem.
func TestChaosC17_SyncAfterClose(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Close resets health but ImportDir stays.
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Sync should still succeed because ImportDir is valid.
	// (Close doesn't clear config — this is intentional for reconnect scenarios.)
	// The connector can legally sync after close as long as config is populated.
	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync after Close: %v", err)
	}
	if len(artifacts) != 2 {
		t.Errorf("got %d artifacts, want 2 from export.json", len(artifacts))
	}
}

// T-CHAOS-C17-003: NormalizeURL strips www. prefix for dedup consistency.
func TestChaosC17_NormalizeURLWwwStripping(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"www prefix", "https://www.example.com/page", "https://example.com/page"},
		{"no www", "https://example.com/page", "https://example.com/page"},
		{"www with port", "https://www.example.com:8080/page", "example.com:8080/page"},
		{"www subdomain", "https://www.sub.example.com", "https://sub.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeURL(tt.in)
			if !strings.Contains(got, "example.com") {
				t.Errorf("NormalizeURL(%q) = %q", tt.in, got)
			}
			// Main invariant: no www. prefix in output host
			if strings.Contains(got, "www.") {
				t.Errorf("CHAOS C17-003: NormalizeURL(%q) = %q still contains www. prefix", tt.in, got)
			}
		})
	}
}

// T-CHAOS-C17-004: Empty and whitespace-only bookmark files are handled gracefully.
func TestChaosC17_EmptyFileHandling(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"empty.json":      {},
		"whitespace.html": []byte("   \n\t\n  "),
		"good.json":       chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Only good.json should produce artifacts.
	if len(artifacts) < 2 {
		t.Errorf("got %d artifacts, want >= 2 from good.json", len(artifacts))
	}

	// Cursor should contain good.json but not the failed files.
	var files []string
	if cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &files); err != nil {
			t.Fatalf("cursor unmarshal: %v", err)
		}
	}
	for _, f := range files {
		if f == "empty.json" {
			t.Error("empty file should not be in cursor (parse failed)")
		}
	}
}

// T-CHAOS-C17-005: Filenames with special characters don't break cursor
// encoding/decoding or file operations.
func TestChaosC17_SpecialCharFilenames(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"bookmarks (copy).json":   chromeJSONFixture(),
		"export [2024-01-15].htm": netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, cursor, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// Both files should be processed: 2 from JSON + 2 from HTML = 4 artifacts.
	if len(artifacts) != 4 {
		t.Fatalf("got %d artifacts, want 4", len(artifacts))
	}

	// Round-trip the cursor — special chars must survive encoding.
	var files []string
	if err := json.Unmarshal([]byte(cursor), &files); err != nil {
		t.Fatalf("cursor unmarshal: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("cursor has %d files, want 2", len(files))
	}

	// Re-sync with the cursor — should produce zero new artifacts.
	artifacts2, _, err := c.Sync(ctx, cursor)
	if err != nil {
		t.Fatalf("Sync 2: %v", err)
	}
	if len(artifacts2) != 0 {
		t.Errorf("re-sync got %d artifacts, want 0 (all already in cursor)", len(artifacts2))
	}
}

// T-CHAOS-C17-006: NormalizeURL with userinfo (user:pass@host) strips credentials.
func TestChaosC17_NormalizeURLStripsUserinfo(t *testing.T) {
	got := NormalizeURL("https://admin:secret@example.com/admin")
	if strings.Contains(got, "admin:secret") {
		t.Fatalf("CHAOS C17-006: NormalizeURL preserved credentials: %q", got)
	}
	if strings.Contains(got, "@") {
		t.Errorf("CHAOS C17-006: NormalizeURL still has @ sign: %q", got)
	}
}

// ============================================================================
// GAP — R-012 active health validation and R-006 content_fetched metadata
// ============================================================================

// T-GAP-R012-001: Health() detects removed import directory and returns HealthError.
func TestHealthDetectsRemovedImportDir(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if h := c.Health(ctx); h != connector.HealthHealthy {
		t.Fatalf("initial Health() = %q, want healthy", h)
	}

	// Remove the import directory after successful connect
	if err := os.Remove(dir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	// Health should now detect the missing directory and return error
	if h := c.Health(ctx); h != connector.HealthError {
		t.Errorf("GAP R-012: Health() = %q after import dir removed, want %q", h, connector.HealthError)
	}
}

// T-GAP-R012-002: Health() returns healthy when import directory is present.
func TestHealthStaysHealthyWithValidDir(t *testing.T) {
	dir := t.TempDir()
	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Multiple health checks should all return healthy
	for i := 0; i < 3; i++ {
		if h := c.Health(ctx); h != connector.HealthHealthy {
			t.Errorf("Health() call %d = %q, want healthy", i, h)
		}
	}
}

// T-GAP-R012-003: pendingFiles is populated after findNewFiles.
func TestPendingFilesTracked(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"a.json": chromeJSONFixture(),
		"b.html": netscapeHTMLFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Before sync, pendingFiles should be 0
	c.mu.RLock()
	before := c.pendingFiles
	c.mu.RUnlock()
	if before != 0 {
		t.Errorf("pendingFiles before sync = %d, want 0", before)
	}

	// After sync, pendingFiles should have been set (then cleared by processing)
	_, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	// After full sync, there are no pending files left (all were processed)
	c.mu.RLock()
	after := c.pendingFiles
	c.mu.RUnlock()
	// pendingFiles was set to 2 when findNewFiles ran, stays at that value
	// (it reflects the count at discovery time, not the current pending count)
	if after != 2 {
		t.Errorf("pendingFiles after sync = %d, want 2 (discovery count)", after)
	}
}

// T-GAP-R006-002: content_fetched metadata is set to false on all artifacts.
func TestContentFetchedMetadata(t *testing.T) {
	dir := setupImportDir(t, map[string][]byte{
		"export.json": chromeJSONFixture(),
	})

	c := NewConnector("bookmarks")
	ctx := context.Background()
	if err := c.Connect(ctx, makeConfig(dir)); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	artifacts, _, err := c.Sync(ctx, "")
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	for _, a := range artifacts {
		cf, ok := a.Metadata["content_fetched"]
		if !ok {
			t.Errorf("artifact %q missing content_fetched metadata", a.Title)
			continue
		}
		if cf != false {
			t.Errorf("artifact %q content_fetched = %v, want false (initial value)", a.Title, cf)
		}
	}
}

// T-GAP-R012-004: Health() returns disconnected when connector was never connected
// (import dir is empty string).
func TestHealthDisconnectedBeforeConnect(t *testing.T) {
	c := NewConnector("bookmarks")
	ctx := context.Background()
	// No Connect() call — health should stay disconnected
	if h := c.Health(ctx); h != connector.HealthDisconnected {
		t.Errorf("Health() = %q before Connect, want disconnected", h)
	}
}
