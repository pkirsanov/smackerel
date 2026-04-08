package browser

import (
	"testing"
	"time"
)

func TestDwellTimeTier(t *testing.T) {
	tests := []struct {
		dwell    time.Duration
		expected string
	}{
		{6 * time.Minute, "full"},
		{3 * time.Minute, "standard"},
		{45 * time.Second, "light"},
		{10 * time.Second, "metadata"},
	}

	for _, tt := range tests {
		got := DwellTimeTier(tt.dwell)
		if got != tt.expected {
			t.Errorf("DwellTimeTier(%v) = %q, want %q", tt.dwell, got, tt.expected)
		}
	}
}

func TestIsSocialMedia(t *testing.T) {
	if !IsSocialMedia("twitter.com") {
		t.Error("twitter.com should be social media")
	}
	if IsSocialMedia("example.com") {
		t.Error("example.com should not be social media")
	}
}

func TestShouldSkip(t *testing.T) {
	if !ShouldSkip("chrome://settings", nil) {
		t.Error("chrome:// should be skipped")
	}
	if !ShouldSkip("localhost:3000/test", nil) {
		t.Error("localhost should be skipped")
	}
	if ShouldSkip("https://example.com", nil) {
		t.Error("example.com should not be skipped")
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/page", "example.com"},
		{"http://test.org:8080/path", "test.org"},
		{"https://sub.domain.com/", "sub.domain.com"},
	}

	for _, tt := range tests {
		got := extractDomain(tt.url)
		if got != tt.expected {
			t.Errorf("extractDomain(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}

func TestChromeTimeToGo(t *testing.T) {
	// A known Chrome timestamp for 2024-01-01 00:00:00 UTC
	// ChromeTime = UnixMicro + 11644473600000000
	expectedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	chromeTime := expectedTime.UnixMicro() + 11644473600000000
	got := chromeTimeToGo(chromeTime)

	if !got.Equal(expectedTime) {
		t.Errorf("chromeTimeToGo() = %v, want %v", got, expectedTime)
	}
}

func TestOptInRequired(t *testing.T) {
	// Browser connector must not process URLs when consent is absent.
	// ShouldSkip must block internal/sensitive URLs even with no custom skip list.
	internalURLs := []string{
		"chrome://settings",
		"chrome-extension://abc/options.html",
		"about:blank",
		"file:///home/user/secret.html",
		"localhost:3000/dashboard",
	}
	for _, u := range internalURLs {
		if !ShouldSkip(u, nil) {
			t.Errorf("ShouldSkip(%q, nil) = false, want true (privacy-sensitive URL)", u)
		}
	}

	// User-provided skip list must be respected as an opt-out mechanism
	customSkip := []string{"private.corp.com"}
	if !ShouldSkip("private.corp.com/page", customSkip) {
		t.Error("custom skip domain should be blocked")
	}
}

func TestPerSourceDeletion(t *testing.T) {
	// ToRawArtifacts must tag each artifact with source_id="browser"
	// so deletion by source can isolate browser data from other connectors.
	entries := []HistoryEntry{
		{URL: "https://a.com", Title: "A", VisitTime: time.Now(), Domain: "a.com"},
		{URL: "https://b.com", Title: "B", VisitTime: time.Now(), Domain: "b.com"},
	}

	artifacts := ToRawArtifacts(entries)
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
	for i, a := range artifacts {
		if a.SourceID != "browser" {
			t.Errorf("artifact[%d].SourceID = %q, want \"browser\"", i, a.SourceID)
		}
		if a.SourceRef != entries[i].URL {
			t.Errorf("artifact[%d].SourceRef = %q, want %q", i, a.SourceRef, entries[i].URL)
		}
	}
}
