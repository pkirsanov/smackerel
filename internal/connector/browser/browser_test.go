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
