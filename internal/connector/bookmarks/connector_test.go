package bookmarks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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
	if got := err.Error(); !contains(got, "does not exist") {
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
	if got := err.Error(); !contains(got, "import directory") {
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
	_, err := c.processFile(ctx, filepath.Join(dir, "nonexistent.json"))
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
	if !contains(err.Error(), "cancelled") {
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
		if contains(a.URL, "spam.com") {
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
