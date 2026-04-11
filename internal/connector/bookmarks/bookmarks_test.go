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
