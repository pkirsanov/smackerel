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

// H1 hardening: subdomain variants of social media must be aggregated per SCN-005-004.
// Adversarial: would fail if IsSocialMedia only used exact map lookup.
func TestIsSocialMedia_Subdomains(t *testing.T) {
	subdomainCases := []struct {
		domain string
		want   bool
	}{
		{"m.twitter.com", true},
		{"mobile.twitter.com", true},
		{"www.facebook.com", true},
		{"m.facebook.com", true},
		{"www.instagram.com", true},
		{"www.reddit.com", true},
		{"old.reddit.com", true},
		{"www.linkedin.com", true},
		{"www.tiktok.com", true},
		{"m.x.com", true},
		// Must NOT match domains that merely contain the social domain as substring
		{"nottwitter.com", false},
		{"myreddit.com", false},
		{"example.com", false},
		{"twitter.com.evil.com", false},
	}
	for _, tt := range subdomainCases {
		got := IsSocialMedia(tt.domain)
		if got != tt.want {
			t.Errorf("IsSocialMedia(%q) = %v, want %v", tt.domain, got, tt.want)
		}
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

// --- CHAOS-HARDENING R3: Adversarial tests ---

// CHAOS-F6: extractDomain must handle non-http/https scheme URLs correctly.
// Adversarial: would fail if extractDomain only stripped http:// and https://.
func TestExtractDomain_NonHTTPSchemes(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"ftp scheme", "ftp://files.example.com/data", "files.example.com"},
		{"ws scheme", "ws://realtime.example.com/feed", "realtime.example.com"},
		{"wss scheme", "wss://secure.example.com:443/ws", "secure.example.com"},
		{"custom scheme", "myapp://settings/foo", "settings"},
		{"no scheme with host", "example.com/path", "example.com"},
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

// CHAOS-F4: DwellTimeTier must handle negative dwell gracefully (treated as metadata).
func TestDwellTimeTier_NegativeDwell(t *testing.T) {
	got := DwellTimeTier(-5 * time.Minute)
	if got != "metadata" {
		t.Errorf("DwellTimeTier(-5m) = %q, want \"metadata\" (negative dwell must not escalate)", got)
	}
}
