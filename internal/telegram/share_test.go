package telegram

import (
	"testing"
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
