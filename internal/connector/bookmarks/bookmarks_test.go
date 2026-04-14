package bookmarks

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseChromeJSON(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bookmarks Bar",
				"children": [
					{"type": "url", "name": "Example", "url": "https://example.com"},
					{
						"type": "folder",
						"name": "Tech",
						"children": [
							{"type": "url", "name": "Go Docs", "url": "https://go.dev"}
						]
					}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(bookmarks))
	}

	if bookmarks[0].Title != "Example" {
		t.Errorf("expected title 'Example', got %q", bookmarks[0].Title)
	}
	if bookmarks[1].Folder != "Bookmarks Bar/Tech" {
		t.Errorf("expected folder 'Bookmarks Bar/Tech', got %q", bookmarks[1].Folder)
	}
}

func TestParseNetscapeHTML(t *testing.T) {
	data := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><H3>Reading</H3>
<DL>
<DT><A HREF="https://example.com/article">Good Article</A>
</DL>
<DT><A HREF="https://go.dev">Go</A>
</DL>`)

	bookmarks, err := ParseNetscapeHTML(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(bookmarks))
	}

	if bookmarks[0].Folder != "Reading" {
		t.Errorf("expected folder 'Reading', got %q", bookmarks[0].Folder)
	}
}

func TestFolderToTopicMapping(t *testing.T) {
	tests := []struct {
		folder   string
		expected string
	}{
		{"Tech/Go", "tech go"},
		{"Reading", "reading"},
		{"", ""},
		{"  Mixed Case  ", "mixed case"},
	}

	for _, tt := range tests {
		got := FolderToTopicMapping(tt.folder)
		if got != tt.expected {
			t.Errorf("FolderToTopicMapping(%q) = %q, want %q", tt.folder, got, tt.expected)
		}
	}
}

func TestToRawArtifacts(t *testing.T) {
	bookmarks := []Bookmark{
		{Title: "Test", URL: "https://example.com", Folder: "Tech"},
	}

	artifacts := ToRawArtifacts(bookmarks)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	if artifacts[0].SourceID != "bookmarks" {
		t.Errorf("expected source_id 'bookmarks', got %q", artifacts[0].SourceID)
	}
}

// T-IMP-01: ToRawArtifacts normalizes SourceRef to canonical URL form.
func TestToRawArtifacts_NormalizedSourceRef(t *testing.T) {
	bookmarks := []Bookmark{
		{Title: "Tracked", URL: "https://Example.COM/page/?utm_source=twitter&id=5"},
		{Title: "Trailing", URL: "https://example.com/path/"},
		{Title: "Clean", URL: "https://example.com/clean"},
	}

	artifacts := ToRawArtifacts(bookmarks)
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}

	// SourceRef should be normalized; URL should remain raw
	if artifacts[0].SourceRef != "https://example.com/page?id=5" {
		t.Errorf("SourceRef = %q, want normalized", artifacts[0].SourceRef)
	}
	if artifacts[0].URL != "https://Example.COM/page/?utm_source=twitter&id=5" {
		t.Errorf("URL should remain raw, got %q", artifacts[0].URL)
	}
	if artifacts[1].SourceRef != "https://example.com/path" {
		t.Errorf("SourceRef = %q, want trailing slash stripped", artifacts[1].SourceRef)
	}
	if artifacts[2].SourceRef != "https://example.com/clean" {
		t.Errorf("SourceRef = %q, want unchanged clean URL", artifacts[2].SourceRef)
	}
}

// T-IMP-02: ParseNetscapeHTML extracts ADD_DATE timestamps.
func TestParseNetscapeHTML_AddDate(t *testing.T) {
	data := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><H3>Reading</H3>
<DL>
<DT><A HREF="https://example.com/article" ADD_DATE="1700000000">Timestamped</A>
<DT><A HREF="https://example.com/nodate">No Date</A>
</DL>
</DL>`)

	bookmarks, err := ParseNetscapeHTML(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(bookmarks))
	}

	// First bookmark should have ADD_DATE parsed
	expected := time.Unix(1700000000, 0)
	if !bookmarks[0].AddedAt.Equal(expected) {
		t.Errorf("AddedAt = %v, want %v", bookmarks[0].AddedAt, expected)
	}

	// Second bookmark should have zero time
	if !bookmarks[1].AddedAt.IsZero() {
		t.Errorf("AddedAt = %v, want zero for bookmark without ADD_DATE", bookmarks[1].AddedAt)
	}
}

func TestToRawArtifacts_Empty(t *testing.T) {
	artifacts := ToRawArtifacts(nil)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for nil, got %d", len(artifacts))
	}
	artifacts = ToRawArtifacts([]Bookmark{})
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for empty slice, got %d", len(artifacts))
	}
}

func TestParseChromeJSON_MalformedJSON(t *testing.T) {
	_, err := ParseChromeJSON([]byte(`{not valid json`))
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseChromeJSON_MissingRoots(t *testing.T) {
	_, err := ParseChromeJSON([]byte(`{"bookmarks": "no roots key"}`))
	if err == nil {
		t.Error("expected error for missing 'roots' key")
	}
}

func TestParseChromeJSON_EmptyRoots(t *testing.T) {
	bookmarks, err := ParseChromeJSON([]byte(`{"roots": {}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks from empty roots, got %d", len(bookmarks))
	}
}

func TestParseNetscapeHTML_Empty(t *testing.T) {
	bookmarks, err := ParseNetscapeHTML([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks from empty input, got %d", len(bookmarks))
	}
}

func TestParseNetscapeHTML_NoLinks(t *testing.T) {
	data := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><H3>Empty Folder</H3>
</DL>`)
	bookmarks, err := ParseNetscapeHTML(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks from folder-only HTML, got %d", len(bookmarks))
	}
}

func TestExtractBookmarks_MaxDepth(t *testing.T) {
	// Build a deeply nested Chrome JSON structure exceeding maxExtractDepth (50)
	inner := map[string]interface{}{
		"type": "url",
		"name": "Deep Bookmark",
		"url":  "https://example.com/deep",
	}
	for i := 0; i < maxExtractDepth+5; i++ {
		inner = map[string]interface{}{
			"type":     "folder",
			"name":     "level",
			"children": []interface{}{inner},
		}
	}

	data := map[string]interface{}{
		"roots": map[string]interface{}{
			"bookmark_bar": inner,
		},
	}

	jsonData, _ := json.Marshal(data)
	bookmarks, err := ParseChromeJSON(jsonData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The bookmark at depth > maxExtractDepth should be unreachable
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks (depth exceeded), got %d", len(bookmarks))
	}
}

func TestFolderToTopicMapping_Backslash(t *testing.T) {
	got := FolderToTopicMapping("Tech\\Go")
	if got != "tech go" {
		t.Errorf("FolderToTopicMapping(\"Tech\\\\Go\") = %q, want \"tech go\"", got)
	}
}

// T-CHAOS-004: Chrome date_added field (microseconds since 1601-01-01) is parsed correctly.
func TestParseChromeJSON_DateAdded(t *testing.T) {
	// 13349961600000000 = 2023-06-01T00:00:00Z in Chrome epoch microseconds.
	// Chrome epoch: microseconds since 1601-01-01T00:00:00Z.
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					{
						"type": "url",
						"name": "Dated Bookmark",
						"url": "https://example.com",
						"date_added": "13349961600000000"
					}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	b := bookmarks[0]
	if b.AddedAt.IsZero() {
		t.Fatal("AddedAt is zero, expected a parsed date")
	}
	// Verify the year is reasonable
	if b.AddedAt.Year() < 2000 || b.AddedAt.Year() > 2100 {
		t.Errorf("AddedAt.Year() = %d, expected between 2000 and 2100", b.AddedAt.Year())
	}
}

// T-CHAOS-004b: Invalid and zero date_added values produce zero time.
func TestParseChromeJSON_DateAddedEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		dateAdded string
		wantZero  bool
	}{
		{"zero value", "0", true},
		{"negative value", "-100", true},
		{"non-numeric", "not-a-number", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dateField := ""
			if tt.dateAdded != "" {
				dateField = `"date_added": "` + tt.dateAdded + `",`
			}
			data := []byte(`{
				"roots": {
					"bookmark_bar": {
						"type": "folder",
						"name": "Bar",
						"children": [
							{
								"type": "url",
								"name": "Test",
								` + dateField + `
								"url": "https://example.com"
							}
						]
					}
				}
			}`)

			bookmarks, err := ParseChromeJSON(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(bookmarks) != 1 {
				t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
			}

			if tt.wantZero && !bookmarks[0].AddedAt.IsZero() {
				t.Errorf("AddedAt = %v, want zero", bookmarks[0].AddedAt)
			}
		})
	}
}

// T-CHAOS-005: Netscape HTML entity-encoded folder and link names are decoded.
func TestParseNetscapeHTML_HTMLEntities(t *testing.T) {
	data := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><H3>Work &amp; Life</H3>
<DL>
<DT><A HREF="https://example.com/article">Bob&#39;s &quot;Best&quot; Article</A>
</DL>
</DL>`)

	bookmarks, err := ParseNetscapeHTML(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	if bookmarks[0].Folder != "Work & Life" {
		t.Errorf("Folder = %q, want %q", bookmarks[0].Folder, "Work & Life")
	}
	if bookmarks[0].Title != "Bob's \"Best\" Article" {
		t.Errorf("Title = %q, want %q", bookmarks[0].Title, `Bob's "Best" Article`)
	}
}

// T-PARSE-001: Chrome JSON with multiple root bars (e.g., bookmark_bar + other).
func TestParseChromeJSON_MultipleRoots(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bookmarks Bar",
				"children": [
					{"type": "url", "name": "A", "url": "https://a.com"}
				]
			},
			"other": {
				"type": "folder",
				"name": "Other Bookmarks",
				"children": [
					{"type": "url", "name": "B", "url": "https://b.com"}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks from 2 roots, got %d", len(bookmarks))
	}
}

// T-PARSE-001b: Chrome JSON nodes with empty URL are skipped.
func TestParseChromeJSON_EmptyURLNode(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					{"type": "url", "name": "No URL", "url": ""},
					{"type": "url", "name": "Has URL", "url": "https://example.com"}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark (empty URL skipped), got %d", len(bookmarks))
	}
	if bookmarks[0].Title != "Has URL" {
		t.Errorf("Title = %q, want %q", bookmarks[0].Title, "Has URL")
	}
}

// T-PARSE-001c: Chrome JSON with non-map children entries are silently skipped.
func TestParseChromeJSON_NonMapChildren(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					"a string child",
					42,
					null,
					{"type": "url", "name": "Valid", "url": "https://example.com"}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark (non-map children skipped), got %d", len(bookmarks))
	}
	if bookmarks[0].Title != "Valid" {
		t.Errorf("Title = %q, want %q", bookmarks[0].Title, "Valid")
	}
}

// T-PARSE-001d: Chrome JSON with non-object entries in roots are silently skipped.
func TestParseChromeJSON_NonObjectRootsEntries(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					{"type": "url", "name": "Valid", "url": "https://example.com"}
				]
			},
			"synced": "not-a-node",
			"checksum": 12345
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark (non-object roots skipped), got %d", len(bookmarks))
	}
}

// T-PARSE-001e: Chrome JSON node without children key is a leaf - no panic.
func TestParseChromeJSON_FolderWithoutChildren(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Empty Folder"
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Errorf("expected 0 bookmarks from childless folder, got %d", len(bookmarks))
	}
}

// T-PARSE-003: Netscape HTML ADD_DATE edge cases.
func TestParseNetscapeHTML_AddDateEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantZero bool
	}{
		{
			name:     "zero timestamp",
			line:     `<DT><A HREF="https://example.com/a" ADD_DATE="0">Zero</A>`,
			wantZero: true,
		},
		{
			name:     "negative timestamp",
			line:     `<DT><A HREF="https://example.com/b" ADD_DATE="-100">Negative</A>`,
			wantZero: true,
		},
		{
			name:     "non-numeric ADD_DATE",
			line:     `<DT><A HREF="https://example.com/c" ADD_DATE="abc">BadDate</A>`,
			wantZero: true,
		},
		{
			name:     "missing ADD_DATE attribute",
			line:     `<DT><A HREF="https://example.com/d">NoDate</A>`,
			wantZero: true,
		},
		{
			name:     "future timestamp",
			line:     `<DT><A HREF="https://example.com/e" ADD_DATE="2000000000">Future</A>`,
			wantZero: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n<DL>\n" + tt.line + "\n</DL>")
			bookmarks, err := ParseNetscapeHTML(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(bookmarks) != 1 {
				t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
			}
			if tt.wantZero && !bookmarks[0].AddedAt.IsZero() {
				t.Errorf("AddedAt = %v, want zero", bookmarks[0].AddedAt)
			}
			if !tt.wantZero && bookmarks[0].AddedAt.IsZero() {
				t.Error("AddedAt is zero, want non-zero")
			}
		})
	}
}

// T-PARSE-004: ToRawArtifacts preserves folder in metadata.
func TestToRawArtifacts_FolderMetadata(t *testing.T) {
	bookmarks := []Bookmark{
		{Title: "A", URL: "https://a.com", Folder: "Dev/Go"},
		{Title: "B", URL: "https://b.com", Folder: ""},
	}

	artifacts := ToRawArtifacts(bookmarks)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	if folder, ok := artifacts[0].Metadata["folder"].(string); !ok || folder != "Dev/Go" {
		t.Errorf("artifacts[0] folder = %v, want %q", artifacts[0].Metadata["folder"], "Dev/Go")
	}
	if folder, ok := artifacts[1].Metadata["folder"].(string); !ok || folder != "" {
		t.Errorf("artifacts[1] folder = %v, want empty string", artifacts[1].Metadata["folder"])
	}
}

// T-PARSE-005: FolderToTopicMapping with multi-level slash paths.
func TestFolderToTopicMapping_MultiLevel(t *testing.T) {
	tests := []struct {
		folder   string
		expected string
	}{
		{"Tech/Go/Libraries", "tech go libraries"},
		{"a/b/c/d", "a b c d"},
		{"single", "single"},
		{"/leading/slash", " leading slash"},
		{"trailing/slash/", "trailing slash "},
	}

	for _, tt := range tests {
		got := FolderToTopicMapping(tt.folder)
		if got != tt.expected {
			t.Errorf("FolderToTopicMapping(%q) = %q, want %q", tt.folder, got, tt.expected)
		}
	}
}

// T-PARSE-002: Netscape HTML with nested folders preserves folder context.
func TestParseNetscapeHTML_NestedFolders(t *testing.T) {
	data := []byte(`<!DOCTYPE NETSCAPE-Bookmark-file-1>
<DL>
<DT><H3>Outer</H3>
<DL>
<DT><H3>Inner</H3>
<DL>
<DT><A HREF="https://example.com/deep">Deep Link</A>
</DL>
</DL>
</DL>`)

	bookmarks, err := ParseNetscapeHTML(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	// The folder should reflect the innermost H3 encountered before the link.
	// Current implementation sets folder to the most recent H3 — "Inner".
	if bookmarks[0].Folder != "Inner" {
		t.Errorf("Folder = %q, want %q", bookmarks[0].Folder, "Inner")
	}
}

// ============================================================================
// CHAOS R24 — Adversarial regression tests for Chrome date_added
// ============================================================================

// T-CHAOS-R24-003: Chrome date_added with adversarial far-future value must
// NOT produce a timestamp. Before the fix, a crafted date_added could push
// AddedAt centuries into the future, disrupting downstream sort/filter.
func TestChaosR24_ChromeDateAddedFarFuture(t *testing.T) {
	// date_added = 99999999999999999 microseconds from Chrome epoch.
	// That's roughly year 5145 — well beyond the year 2100 cap.
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					{
						"type": "url",
						"name": "Future",
						"url": "https://example.com/future",
						"date_added": "99999999999999999"
					}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	// AddedAt must be zero — the far-future value should be rejected.
	if !bookmarks[0].AddedAt.IsZero() {
		t.Errorf("CHAOS R24-003: AddedAt = %v for far-future date_added; "+
			"expected zero — uncapped timestamp leaked through", bookmarks[0].AddedAt)
	}
}

// T-CHAOS-R24-003b: Reasonable Chrome date_added (2024) must still be accepted.
func TestChaosR24_ChromeDateAddedReasonable(t *testing.T) {
	// date_added for 2024-01-15 = 13349811200000000 microseconds from Chrome epoch.
	// (Chrome epoch: 1601-01-01 UTC, offset: 11644473600 seconds)
	// Unix timestamp: (13349811200000000 / 1000000) - 11644473600 = 1705337600
	// ≈ 2024-01-15T16:53:20 UTC
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					{
						"type": "url",
						"name": "Recent",
						"url": "https://example.com/recent",
						"date_added": "13349811200000000"
					}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	if bookmarks[0].AddedAt.IsZero() {
		t.Fatal("CHAOS R24-003b: AddedAt is zero for a reasonable 2024 date_added — valid dates are being rejected")
	}

	year := bookmarks[0].AddedAt.Year()
	if year != 2024 {
		t.Errorf("CHAOS R24-003b: AddedAt year = %d, want 2024", year)
	}
}

// T-CHAOS-R24-003c: Chrome date_added with MaxInt64 must not produce a timestamp.
func TestChaosR24_ChromeDateAddedMaxInt64(t *testing.T) {
	data := []byte(`{
		"roots": {
			"bookmark_bar": {
				"type": "folder",
				"name": "Bar",
				"children": [
					{
						"type": "url",
						"name": "MaxInt",
						"url": "https://example.com/maxint",
						"date_added": "9223372036854775807"
					}
				]
			}
		}
	}`)

	bookmarks, err := ParseChromeJSON(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}

	if !bookmarks[0].AddedAt.IsZero() {
		t.Errorf("CHAOS R24-003c: AddedAt = %v for MaxInt64 date_added; "+
			"expected zero — integer overflow timestamp leaked through", bookmarks[0].AddedAt)
	}
}
