package extract

import (
	"testing"
)

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected ContentType
	}{
		{"youtube watch", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", ContentTypeYouTube},
		{"youtube short", "https://youtu.be/dQw4w9WgXcQ", ContentTypeYouTube},
		{"youtube embed", "https://www.youtube.com/embed/dQw4w9WgXcQ", ContentTypeYouTube},
		{"amazon product", "https://www.amazon.com/dp/B08N5WRWNW", ContentTypeProduct},
		{"recipe site", "https://www.allrecipes.com/recipe/123", ContentTypeRecipe},
		{"generic article", "https://example.com/some-article", ContentTypeArticle},
		{"empty url", "", ContentTypeGeneric},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectContentType(tt.url)
			if got != tt.expected {
				t.Errorf("DetectContentType(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestExtractYouTubeID(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := ExtractYouTubeID(tt.url)
			if got != tt.expected {
				t.Errorf("ExtractYouTubeID(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

func TestHashContent(t *testing.T) {
	hash1 := HashContent("hello world")
	hash2 := HashContent("  Hello World  ")
	hash3 := HashContent("different content")

	if hash1 != hash2 {
		t.Errorf("normalized content should produce same hash")
	}
	if hash1 == hash3 {
		t.Errorf("different content should produce different hash")
	}
	if len(hash1) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars, got %d", len(hash1))
	}
}

func TestExtractText(t *testing.T) {
	result := ExtractText("My idea\nSome details about it")
	if result.Title != "My idea" {
		t.Errorf("expected title 'My idea', got %q", result.Title)
	}
	if result.ContentHash == "" {
		t.Error("expected non-empty content hash")
	}
	if result.ContentType != ContentTypeGeneric {
		t.Errorf("expected generic content type, got %q", result.ContentType)
	}
}

func TestExtractText_LongTitle(t *testing.T) {
	longText := "x" + string(make([]byte, 200))
	result := ExtractText(longText)
	if len(result.Title) > 100 {
		t.Errorf("title should be capped at 100 chars, got %d", len(result.Title))
	}
}

func TestExtractText_SingleLine(t *testing.T) {
	result := ExtractText("Just a note")
	if result.Title != "Just a note" {
		t.Errorf("expected full text as title, got %q", result.Title)
	}
}

func TestExtractText_EmptyString(t *testing.T) {
	result := ExtractText("")
	if result.ContentHash == "" {
		t.Error("should produce hash even for empty text")
	}
}

func TestExtractText_MultilineTitle(t *testing.T) {
	result := ExtractText("Title line\nSecond line\nThird line")
	if result.Title != "Title line" {
		t.Errorf("expected first line as title, got %q", result.Title)
	}
}

func TestHashContent_Deterministic(t *testing.T) {
	h1 := HashContent("test content")
	h2 := HashContent("test content")
	if h1 != h2 {
		t.Error("same content should produce same hash")
	}
}

func TestHashContent_CaseInsensitive(t *testing.T) {
	h1 := HashContent("Hello World")
	h2 := HashContent("hello world")
	if h1 != h2 {
		t.Error("hash should be case-insensitive")
	}
}

func TestHashContent_TrimsWhitespace(t *testing.T) {
	h1 := HashContent("hello")
	h2 := HashContent("  hello  ")
	if h1 != h2 {
		t.Error("hash should trim whitespace")
	}
}

func TestDetectContentType_ProductURLs(t *testing.T) {
	tests := []struct {
		url      string
		expected ContentType
	}{
		{"https://www.amazon.com/dp/B08N5WRWNW/ref=something", ContentTypeProduct},
		{"https://www.amazon.com/some-product/dp/B123456", ContentTypeProduct},
	}
	for _, tt := range tests {
		got := DetectContentType(tt.url)
		if got != tt.expected {
			t.Errorf("DetectContentType(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestExtractYouTubeID_VariousFormats(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=60", "dQw4w9WgXcQ"},
		{"https://youtu.be/dQw4w9WgXcQ?si=abc", "dQw4w9WgXcQ"},
		{"https://www.youtube.com/embed/dQw4w9WgXcQ?autoplay=1", "dQw4w9WgXcQ"},
		{"https://youtube.com/watch?v=abc-_def123", "abc-_def123"},
	}
	for _, tt := range tests {
		got := ExtractYouTubeID(tt.url)
		if got != tt.expected {
			t.Errorf("ExtractYouTubeID(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestResult_Fields(t *testing.T) {
	r := &Result{
		ContentType: ContentTypeArticle,
		Title:       "Test",
		Author:      "Author",
		Date:        "2026-04-06",
		Text:        "Some text",
		ContentHash: "abc123",
		SourceURL:   "https://example.com",
	}
	if r.ContentType != ContentTypeArticle {
		t.Errorf("unexpected content type: %q", r.ContentType)
	}
	if r.VideoID != "" {
		t.Error("non-youtube result should have empty VideoID")
	}
}

func TestContentType_Constants(t *testing.T) {
	types := []ContentType{
		ContentTypeArticle, ContentTypeYouTube, ContentTypeProduct,
		ContentTypeRecipe, ContentTypeGeneric,
	}
	seen := make(map[ContentType]bool)
	for _, ct := range types {
		if ct == "" {
			t.Error("content type should not be empty")
		}
		if seen[ct] {
			t.Errorf("duplicate content type: %q", ct)
		}
		seen[ct] = true
	}
}
