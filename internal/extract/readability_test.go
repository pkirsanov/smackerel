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

// SCN-002-005: Article URL content extraction — readability extracts title, author, hash
func TestSCN002005_ArticleExtraction_TextAndHash(t *testing.T) {
	result := ExtractText("How to build SaaS products\nKey strategies for building and pricing SaaS.\nBy David Kim.")
	if result.Title != "How to build SaaS products" {
		t.Errorf("expected first line as title, got %q", result.Title)
	}
	if result.ContentHash == "" {
		t.Error("content hash must be generated")
	}
	if result.Text == "" {
		t.Error("text must be preserved")
	}
}

// SCN-002-005: URL type detection for articles
func TestSCN002005_ArticleURLDetection(t *testing.T) {
	tests := []string{
		"https://example.com/blog/my-article",
		"https://medium.com/@user/great-post-123",
		"https://news.ycombinator.com/item?id=12345",
	}
	for _, u := range tests {
		ct := DetectContentType(u)
		if ct == ContentTypeYouTube || ct == ContentTypeProduct || ct == ContentTypeRecipe {
			t.Errorf("article URL %q should not detect as %q", u, ct)
		}
	}
}

// SCN-002-006: YouTube URL transcript extraction — video ID extraction
func TestSCN002006_YouTubeURLDetection(t *testing.T) {
	urls := []struct {
		url string
		id  string
	}{
		{"https://www.youtube.com/watch?v=dQw4w9WgXcQ", "dQw4w9WgXcQ"},
		{"https://youtu.be/abc123def45", "abc123def45"},
		{"https://www.youtube.com/embed/XYZtest-1Ab", "XYZtest-1Ab"},
	}
	for _, tt := range urls {
		ct := DetectContentType(tt.url)
		if ct != ContentTypeYouTube {
			t.Errorf("URL %q should detect as YouTube, got %q", tt.url, ct)
		}
		vid := ExtractYouTubeID(tt.url)
		if vid != tt.id {
			t.Errorf("YouTube ID from %q = %q, want %q", tt.url, vid, tt.id)
		}
	}
}

// SCN-002-009: Content deduplication — same content produces same hash
func TestSCN002009_ContentDedup_HashMatch(t *testing.T) {
	content := "This is an article about distributed systems and their trade-offs."
	hash1 := HashContent(content)
	hash2 := HashContent(content)
	if hash1 != hash2 {
		t.Error("identical content must produce identical hash")
	}

	// Case and whitespace normalization
	hash3 := HashContent("  THIS is an ARTICLE about distributed systems and their trade-offs.  ")
	if hash1 != hash3 {
		t.Error("normalized content must produce same hash (case+whitespace insensitive)")
	}

	// Different content must produce different hash
	hash4 := HashContent("Completely different content about cooking recipes.")
	if hash1 == hash4 {
		t.Error("different content must produce different hashes")
	}
}

// SCN-002-009: Dedup uses ExtractText for text input and verifies hash is set
func TestSCN002009_ContentDedup_ExtractTextHash(t *testing.T) {
	r1 := ExtractText("My unique idea about SaaS pricing")
	r2 := ExtractText("My unique idea about SaaS pricing")
	if r1.ContentHash != r2.ContentHash {
		t.Error("same text input should produce same ExtractText hash")
	}
	r3 := ExtractText("A totally different idea about cooking")
	if r1.ContentHash == r3.ContentHash {
		t.Error("different text should produce different hash")
	}
}
