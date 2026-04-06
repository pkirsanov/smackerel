package bookmarks

import (
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
