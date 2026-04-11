package bookmarks

import (
	"encoding/json"
	"testing"
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
