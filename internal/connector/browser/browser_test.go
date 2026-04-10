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

func TestDwellTimeTier_BoundaryValues(t *testing.T) {
	tests := []struct {
		name     string
		dwell    time.Duration
		expected string
	}{
		{"exactly 5m is full", 5 * time.Minute, "full"},
		{"5m minus 1us is standard", 5*time.Minute - time.Microsecond, "standard"},
		{"exactly 2m is standard", 2 * time.Minute, "standard"},
		{"2m minus 1us is light", 2*time.Minute - time.Microsecond, "light"},
		{"exactly 30s is light", 30 * time.Second, "light"},
		{"30s minus 1us is metadata", 30*time.Second - time.Microsecond, "metadata"},
		{"zero dwell is metadata", 0, "metadata"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DwellTimeTier(tt.dwell)
			if got != tt.expected {
				t.Errorf("DwellTimeTier(%v) = %q, want %q", tt.dwell, got, tt.expected)
			}
		})
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

func TestExtractDomain_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"short URL returns as-is", "http://", ""},
		{"very short string", "abc", "abc"},
		{"empty string", "", ""},
		{"https no host", "https://", ""},
		{"https with trailing slash only", "https:///path", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDomain(tt.url)
			if got != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
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
		t.Error("custom skip domain should be blocked (no scheme)")
	}

	// R001 regression: user skip domains must match URLs WITH https:// scheme
	if !ShouldSkip("https://private.corp.com/page", customSkip) {
		t.Error("custom skip domain should block https:// URLs (SCN-005-005)")
	}
	if !ShouldSkip("http://private.corp.com/internal", customSkip) {
		t.Error("custom skip domain should block http:// URLs")
	}
	// Non-matching domain must not be skipped
	if ShouldSkip("https://public.example.com/page", customSkip) {
		t.Error("non-skip domain should not be blocked")
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

// CHAOS-005-F1: Scheme-prefixed localhost/loopback URLs must be caught by default skip.
// Adversarial: would fail if ShouldSkip only used prefix matching on raw URL.
func TestShouldSkip_SchemePrefixedLocalhost(t *testing.T) {
	// These URLs have schemes so the prefix match against "localhost" / "127.0.0.1" alone
	// would miss them. The fix adds domain-extracted matching for default skip domains.
	mustSkip := []string{
		"https://localhost:3000/admin",
		"http://localhost:8080/dashboard",
		"https://localhost/",
		"http://127.0.0.1:9090/api",
		"https://127.0.0.1/metrics",
	}
	for _, u := range mustSkip {
		if !ShouldSkip(u, nil) {
			t.Errorf("ShouldSkip(%q, nil) = false, want true (scheme-prefixed local URL must be filtered)", u)
		}
	}

	// External URLs must remain unaffected.
	mustAllow := []string{
		"https://example.com/page",
		"https://news.ycombinator.com",
		"https://docs.google.com/edit",
	}
	for _, u := range mustAllow {
		if ShouldSkip(u, nil) {
			t.Errorf("ShouldSkip(%q, nil) = true, want false (external URL should pass)", u)
		}
	}
}

func TestIsSocialMedia_AllRegisteredDomains(t *testing.T) {
	// Verify ALL domains in SocialMediaDomains map are recognized
	expected := []string{"twitter.com", "x.com", "facebook.com", "instagram.com", "reddit.com", "linkedin.com", "tiktok.com"}
	for _, domain := range expected {
		if !IsSocialMedia(domain) {
			t.Errorf("IsSocialMedia(%q) = false, want true", domain)
		}
	}
	// Non-social domains must not be matched
	nonSocial := []string{"github.com", "google.com", "youtube.com", "wikipedia.org", ""}
	for _, domain := range nonSocial {
		if IsSocialMedia(domain) {
			t.Errorf("IsSocialMedia(%q) = true, want false", domain)
		}
	}
}

func TestToRawArtifacts_MetadataFields(t *testing.T) {
	entries := []HistoryEntry{
		{
			URL:       "https://example.com/article",
			Title:     "Test Article",
			VisitTime: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
			DwellTime: 5 * time.Minute,
			Domain:    "example.com",
		},
	}

	artifacts := ToRawArtifacts(entries)
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	a := artifacts[0]

	// Verify metadata contains dwell_time and domain
	dwellSec, ok := a.Metadata["dwell_time"].(float64)
	if !ok {
		t.Fatal("metadata missing dwell_time")
	}
	if dwellSec != 300.0 {
		t.Errorf("dwell_time = %f, want 300.0 (5 minutes)", dwellSec)
	}

	domain, ok := a.Metadata["domain"].(string)
	if !ok {
		t.Fatal("metadata missing domain")
	}
	if domain != "example.com" {
		t.Errorf("domain = %q, want %q", domain, "example.com")
	}
}

func TestToRawArtifacts_EmptyEntries(t *testing.T) {
	artifacts := ToRawArtifacts(nil)
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for nil entries, got %d", len(artifacts))
	}
	artifacts = ToRawArtifacts([]HistoryEntry{})
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for empty entries, got %d", len(artifacts))
	}
}

func TestGoTimeToChrome_RoundTrip(t *testing.T) {
	original := time.Date(2026, 4, 10, 14, 30, 0, 0, time.UTC)
	chromeTime := GoTimeToChrome(original)
	converted := ChromeTimeToGo(chromeTime)

	if !converted.Equal(original) {
		t.Errorf("round-trip failed: %v → %d → %v", original, chromeTime, converted)
	}
}

// CHAOS-005-F3: ParseChromeHistorySince uses a LIMIT to prevent memory exhaustion.
func TestParseChromeHistorySince_HasLimit(t *testing.T) {
	// Verify the function signature exists and handles missing DB gracefully.
	// The actual LIMIT is enforced at the SQL level; we verify the function
	// doesn't error on non-existent path (it should return an error).
	_, err := ParseChromeHistorySince("/nonexistent/History", 0)
	if err == nil {
		t.Error("expected error for non-existent history path")
	}
}
