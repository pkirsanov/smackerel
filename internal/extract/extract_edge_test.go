package extract

import (
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- validateURLSafety edge cases ---

func TestValidateURLSafety_DataScheme(t *testing.T) {
	u, _ := url.Parse("data:text/html,<h1>hi</h1>")
	err := validateURLSafety(u)
	if err == nil {
		t.Error("data: scheme should be blocked")
	}
}

func TestValidateURLSafety_JavascriptScheme(t *testing.T) {
	u, _ := url.Parse("javascript:alert(1)")
	err := validateURLSafety(u)
	if err == nil {
		t.Error("javascript: scheme should be blocked")
	}
}

func TestValidateURLSafety_FileScheme(t *testing.T) {
	u, _ := url.Parse("file:///etc/passwd")
	err := validateURLSafety(u)
	if err == nil {
		t.Error("file: scheme should be blocked")
	}
}

func TestValidateURLSafety_InternalSuffix(t *testing.T) {
	// Any hostname ending in ".internal" must be blocked
	u, _ := url.Parse("http://secret-service.internal/api")
	err := validateURLSafety(u)
	if err == nil {
		t.Error("*.internal hostname should be blocked")
	}
}

func TestValidateURLSafety_AzureMetadataIP(t *testing.T) {
	// 169.254.170.2 is an Azure metadata endpoint
	u, _ := url.Parse("http://169.254.170.2/metadata")
	err := validateURLSafety(u)
	if err == nil {
		t.Error("Azure metadata IP 169.254.170.2 should be blocked")
	}
}

func TestValidateURLSafety_HTTPSAllowed(t *testing.T) {
	u, _ := url.Parse("https://news.ycombinator.com/item?id=12345")
	err := validateURLSafety(u)
	if err != nil {
		t.Errorf("public HTTPS URL should be allowed, got: %v", err)
	}
}

func TestValidateURLSafety_HTTPAllowed(t *testing.T) {
	u, _ := url.Parse("http://example.com/article")
	err := validateURLSafety(u)
	if err != nil {
		t.Errorf("public HTTP URL should be allowed, got: %v", err)
	}
}

// --- ExtractText edge cases ---

func TestExtractText_OnlyWhitespaceInput(t *testing.T) {
	result := ExtractText("   \t\n  ")
	if result.ContentHash == "" {
		t.Error("whitespace-only input should still produce a hash")
	}
	// Title should be the first "line" which is "   \t"
	if result.ContentType != ContentTypeGeneric {
		t.Errorf("expected generic content type, got %q", result.ContentType)
	}
}

func TestExtractText_UTF8TitleTruncation(t *testing.T) {
	// Build a title that's exactly 99 bytes of ASCII + a 3-byte UTF-8 char
	// If truncation at byte 100 splits the rune, the title may be invalid UTF-8
	prefix := strings.Repeat("a", 99)
	// "é" is 2 bytes in UTF-8 (0xC3 0xA9)
	singleLine := prefix + "é" + " extra"
	result := ExtractText(singleLine)
	if !utf8.ValidString(result.Title) {
		t.Errorf("title should be valid UTF-8 after truncation, got %q", result.Title)
	}
	if len(result.Title) > 100 {
		t.Errorf("title should be at most 100 bytes, got %d", len(result.Title))
	}
}

func TestExtractText_BinaryGarbage(t *testing.T) {
	garbage := string([]byte{0xFF, 0xFE, 0x00, 0x01, 0x80, 0x90})
	result := ExtractText(garbage)
	if result.ContentHash == "" {
		t.Error("binary garbage should still produce a content hash")
	}
	if result.ContentType != ContentTypeGeneric {
		t.Errorf("expected generic type for binary input, got %q", result.ContentType)
	}
}

func TestExtractText_ControlCharacters(t *testing.T) {
	text := "Title here\x01\x02\x03\nBody with control chars\x07"
	result := ExtractText(text)
	if result.Title != "Title here\x01\x02\x03" {
		t.Errorf("title should preserve control chars up to first newline, got %q", result.Title)
	}
}

func TestExtractText_ExactlyHundredByteTitle(t *testing.T) {
	singleLine := strings.Repeat("x", 100)
	result := ExtractText(singleLine)
	if len(result.Title) != 100 {
		t.Errorf("exactly 100-byte title should not be truncated, got %d bytes", len(result.Title))
	}
}

func TestExtractText_HundredAndOneByteTitle(t *testing.T) {
	singleLine := strings.Repeat("x", 101)
	result := ExtractText(singleLine)
	if len(result.Title) != 100 {
		t.Errorf("101-byte single line title should be truncated to 100, got %d", len(result.Title))
	}
}

// --- DetectContentType edge cases ---

func TestDetectContentType_GIFExtension(t *testing.T) {
	if DetectContentType("https://example.com/animation.gif") != ContentTypeImage {
		t.Error("should detect .gif as image")
	}
}

func TestDetectContentType_BMPExtension(t *testing.T) {
	if DetectContentType("https://example.com/bitmap.bmp") != ContentTypeImage {
		t.Error("should detect .bmp as image")
	}
}

func TestDetectContentType_TIFFExtension(t *testing.T) {
	if DetectContentType("https://example.com/scan.tiff") != ContentTypeImage {
		t.Error("should detect .tiff as image")
	}
}

func TestDetectContentType_HEICExtension(t *testing.T) {
	if DetectContentType("https://example.com/photo.heic") != ContentTypeImage {
		t.Error("should detect .heic as image")
	}
}

func TestDetectContentType_SVGExtension(t *testing.T) {
	if DetectContentType("https://example.com/icon.svg") != ContentTypeImage {
		t.Error("should detect .svg as image")
	}
}

func TestDetectContentType_YouTubeWithPlaylist(t *testing.T) {
	ct := DetectContentType("https://www.youtube.com/watch?v=dQw4w9WgXcQ&list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf")
	if ct != ContentTypeYouTube {
		t.Errorf("YouTube URL with playlist param should still be YouTube, got %q", ct)
	}
}

func TestDetectContentType_RecipeInPath(t *testing.T) {
	ct := DetectContentType("https://myfoodsite.com/recipe/chocolate-cake")
	if ct != ContentTypeRecipe {
		t.Errorf("URL with 'recipe' in path should detect as recipe, got %q", ct)
	}
}

func TestDetectContentType_CookingInDomain(t *testing.T) {
	ct := DetectContentType("https://cooking.nytimes.com/recipes/12345")
	if ct != ContentTypeRecipe {
		t.Errorf("cooking domain should detect as recipe, got %q", ct)
	}
}

func TestDetectContentType_ImagePrecedesProduct(t *testing.T) {
	// An image extension on a product domain should be classified as image
	ct := DetectContentType("https://www.amazon.com/images/product.jpg")
	if ct != ContentTypeImage {
		t.Errorf("image extension should take precedence, got %q", ct)
	}
}

// --- HashContent edge cases ---

func TestHashContent_VeryLongWhitespace(t *testing.T) {
	// Lots of whitespace should normalize to empty
	ws := strings.Repeat(" ", 10000)
	h1 := HashContent(ws)
	h2 := HashContent("")
	if h1 != h2 {
		t.Error("long whitespace should hash to same as empty after normalization")
	}
}

func TestHashContent_TabAndNewlineNormalization(t *testing.T) {
	// Tabs/newlines in the middle are NOT trimmed by TrimSpace (only leading/trailing)
	h1 := HashContent("hello\tworld")
	h2 := HashContent("hello\tworld")
	if h1 != h2 {
		t.Error("identical whitespace-in-middle content must hash identically")
	}
	h3 := HashContent("hello world")
	if h1 == h3 {
		t.Error("tab vs space in middle should produce different hashes")
	}
}
