package telegram

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/stringutil"
)

func TestExtractAllURLs_SingleURL(t *testing.T) {
	urls := extractAllURLs("Check out https://example.com/article please")
	if len(urls) != 1 || urls[0] != "https://example.com/article" {
		t.Errorf("expected 1 URL, got %v", urls)
	}
}

func TestExtractAllURLs_MultipleURLs(t *testing.T) {
	text := "https://example.com and also https://other.com/page"
	urls := extractAllURLs(text)
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
}

func TestExtractAllURLs_DuplicateURLs(t *testing.T) {
	text := "https://example.com and https://example.com again"
	urls := extractAllURLs(text)
	if len(urls) != 1 {
		t.Errorf("expected 1 deduplicated URL, got %d: %v", len(urls), urls)
	}
}

func TestExtractAllURLs_NoURLs(t *testing.T) {
	urls := extractAllURLs("just plain text without links")
	if len(urls) != 0 {
		t.Errorf("expected 0 URLs, got %v", urls)
	}
}

func TestExtractAllURLs_TrailingPunctuation(t *testing.T) {
	urls := extractAllURLs("Visit https://example.com. More info at https://other.com!")
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com" {
		t.Errorf("expected trailing period stripped, got %s", urls[0])
	}
	if urls[1] != "https://other.com" {
		t.Errorf("expected trailing exclamation stripped, got %s", urls[1])
	}
}

func TestExtractContext_URLRemoved(t *testing.T) {
	text := "Check this out https://example.com very interesting"
	ctx := extractContext(text, []string{"https://example.com"})
	if ctx != "Check this out very interesting" {
		t.Errorf("unexpected context: %q", ctx)
	}
}

func TestExtractContext_MultipleURLsRemoved(t *testing.T) {
	text := "https://a.com and https://b.com both good"
	ctx := extractContext(text, []string{"https://a.com", "https://b.com"})
	if ctx != "and both good" {
		t.Errorf("unexpected context: %q", ctx)
	}
}

func TestExtractContext_EmptyAfterRemoval(t *testing.T) {
	text := "https://example.com"
	ctx := extractContext(text, []string{"https://example.com"})
	if ctx != "" {
		t.Errorf("expected empty context, got %q", ctx)
	}
}

func TestSCN008001_ShareSheetURLWithContext(t *testing.T) {
	// SC-TSC01: Share-sheet URL + context preserved
	urls := extractAllURLs("Great article about Go concurrency https://blog.example.com/go-concurrency")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	ctx := extractContext("Great article about Go concurrency https://blog.example.com/go-concurrency", urls)
	if ctx != "Great article about Go concurrency" {
		t.Errorf("expected context preserved, got %q", ctx)
	}
}

func TestSCN008002_MultipleURLsFromShareSheet(t *testing.T) {
	// SC-TSC02: Multiple URLs captured individually
	text := "Check these: https://a.example.com/page and https://b.example.com/other"
	urls := extractAllURLs(text)
	if len(urls) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(urls))
	}
}

func TestSCN008003_BareURLBackwardCompat(t *testing.T) {
	// SC-TSC03: Bare URL without context — backward compatible
	urls := extractAllURLs("https://example.com/article")
	ctx := extractContext("https://example.com/article", urls)
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	if ctx != "" {
		t.Errorf("expected empty context for bare URL, got %q", ctx)
	}
}

// --- Chaos-hardening tests ---

func TestChaos_TruncateUTF8_MultiByteAtBoundary(t *testing.T) {
	// 3-byte UTF-8 char (Chinese): "你" = 0xE4 0xBD 0xA0
	// Build a string where a multi-byte char straddles the byte boundary
	prefix := strings.Repeat("a", 4094) // 4094 ASCII bytes
	text := prefix + "你好"               // 4094 + 3 + 3 = 4100 bytes

	result := stringutil.TruncateUTF8(text, 4096)
	if !utf8.ValidString(result) {
		t.Errorf("TruncateUTF8 produced invalid UTF-8: %q", result)
	}
	if len(result) > 4096 {
		t.Errorf("result exceeds maxBytes: got %d", len(result))
	}
	// The 3-byte char at byte 4094 would end at 4097, so it should be excluded
	if len(result) != 4094 {
		t.Errorf("expected 4094 bytes (dropping split rune), got %d", len(result))
	}
}

func TestChaos_TruncateUTF8_ExactBoundary(t *testing.T) {
	// String that is exactly maxShareTextLen — no truncation needed
	text := strings.Repeat("x", maxShareTextLen)
	result := stringutil.TruncateUTF8(text, maxShareTextLen)
	if result != text {
		t.Error("exact-length string should not be modified")
	}
}

func TestChaos_TruncateUTF8_ShortString(t *testing.T) {
	text := "short"
	result := stringutil.TruncateUTF8(text, maxShareTextLen)
	if result != text {
		t.Errorf("short string modified: %q", result)
	}
}

func TestChaos_TruncateUTF8_EmojiAtBoundary(t *testing.T) {
	// 4-byte UTF-8 emoji: "😀" = 0xF0 0x9F 0x98 0x80
	prefix := strings.Repeat("a", 4093) // 4093 ASCII bytes
	text := prefix + "😀"                // 4093 + 4 = 4097 bytes

	result := stringutil.TruncateUTF8(text, 4096)
	if !utf8.ValidString(result) {
		t.Errorf("TruncateUTF8 produced invalid UTF-8 with emoji")
	}
	// Emoji can't fit, so we get just the prefix
	if len(result) != 4093 {
		t.Errorf("expected 4093 bytes (dropping emoji), got %d", len(result))
	}
}

func TestChaos_TruncateUTF8_AllMultiByte(t *testing.T) {
	// String of 2-byte UTF-8 chars (Cyrillic Д = 0xD0 0x94)
	text := strings.Repeat("Д", 3000) // 6000 bytes
	result := stringutil.TruncateUTF8(text, 4096)
	if !utf8.ValidString(result) {
		t.Errorf("TruncateUTF8 produced invalid UTF-8 for Cyrillic")
	}
	// 4096 / 2 = 2048 chars = 4096 bytes exactly
	if len(result) != 4096 {
		t.Errorf("expected 4096 bytes, got %d", len(result))
	}
}

func TestChaos_ExtractAllURLs_ParenthesizedURL(t *testing.T) {
	// Common in markdown/email: (https://example.com)
	urls := extractAllURLs("Check this (https://example.com/article) for details")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL from parenthesized, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/article" {
		t.Errorf("expected clean URL, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_AngleBracketURL(t *testing.T) {
	// Common in email: <https://example.com>
	urls := extractAllURLs("Link: <https://example.com/page>")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL from angle brackets, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/page" {
		t.Errorf("expected clean URL, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_SquareBracketURL(t *testing.T) {
	urls := extractAllURLs("See [https://example.com/doc]")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL from square brackets, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com/doc" {
		t.Errorf("expected clean URL, got %q", urls[0])
	}
}

func TestChaos_ExtractAllURLs_EmptyString(t *testing.T) {
	urls := extractAllURLs("")
	if len(urls) != 0 {
		t.Errorf("empty string should return no URLs, got %v", urls)
	}
}

func TestChaos_ExtractAllURLs_OnlyWhitespace(t *testing.T) {
	urls := extractAllURLs("   \t\n  ")
	if len(urls) != 0 {
		t.Errorf("whitespace-only should return no URLs, got %v", urls)
	}
}

func TestChaos_ExtractAllURLs_UnicodeAroundURL(t *testing.T) {
	// URL preceded/followed by Unicode text
	urls := extractAllURLs("日本語 https://example.com/ページ 中文")
	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d: %v", len(urls), urls)
	}
}

func TestChaos_ExtractContext_OnlyURLs(t *testing.T) {
	text := "https://a.com https://b.com"
	urls := extractAllURLs(text)
	ctx := extractContext(text, urls)
	if ctx != "" {
		t.Errorf("expected empty context when only URLs, got %q", ctx)
	}
}
