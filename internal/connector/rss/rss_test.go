package rss

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestParseRSSItems(t *testing.T) {
	items := []rssItem{
		{
			Title:       "Test Article",
			Link:        "https://example.com/article",
			Description: "A test article",
			PubDate:     "Mon, 06 Apr 2026 10:00:00 +0000",
			Author:      "Author",
			GUID:        "guid-1",
		},
	}

	result := parseRSSItems(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}

	if result[0].Title != "Test Article" {
		t.Errorf("expected title 'Test Article', got %q", result[0].Title)
	}
	if result[0].GUID != "guid-1" {
		t.Errorf("expected GUID 'guid-1', got %q", result[0].GUID)
	}
}

func TestParseRSSItems_FallbackGUID(t *testing.T) {
	items := []rssItem{
		{Title: "No GUID", Link: "https://example.com/page"},
	}

	result := parseRSSItems(items)
	if result[0].GUID != "https://example.com/page" {
		t.Errorf("expected GUID to fall back to link, got %q", result[0].GUID)
	}
}

func TestConnector_Interface(t *testing.T) {
	var _ interface{ ID() string } = New("test-rss", nil)
}

func TestNew(t *testing.T) {
	c := New("feed-1", []string{"https://example.com/rss"})
	if c.ID() != "feed-1" {
		t.Errorf("expected ID 'feed-1', got %q", c.ID())
	}
	if len(c.feedURLs) != 1 {
		t.Errorf("expected 1 feed URL, got %d", len(c.feedURLs))
	}
}

func TestFeedItem_Fields(t *testing.T) {
	item := FeedItem{
		Title:     "Test",
		Link:      "https://example.com",
		Published: time.Now(),
		Author:    "Author",
		GUID:      "guid-1",
	}

	if item.Title != "Test" {
		t.Errorf("unexpected title: %q", item.Title)
	}
}

// Security: SSRF validation tests for feed URL validation (S001)
func TestValidateFeedURL_AllowsHTTPAndHTTPS(t *testing.T) {
	// These should not be rejected by scheme check (DNS may fail, that's OK)
	for _, u := range []string{
		"https://feeds.example.com/rss",
		"http://feeds.example.com/rss",
	} {
		err := validateFeedURL(u)
		// DNS failures are acceptable; only check for scheme/host-level rejections
		if err != nil && !isExpectedDNSError(err) {
			t.Errorf("validateFeedURL(%q) unexpected error: %v", u, err)
		}
	}
}

func TestValidateFeedURL_BlocksNonHTTPSchemes(t *testing.T) {
	blocked := []string{
		"file:///etc/passwd",
		"ftp://internal.server/data",
		"gopher://evil.host/ssrf",
		"javascript:alert(1)",
		"data:text/xml,<rss/>",
	}
	for _, u := range blocked {
		err := validateFeedURL(u)
		if err == nil {
			t.Errorf("validateFeedURL(%q) should have been rejected", u)
		}
	}
}

func TestValidateFeedURL_BlocksLocalhostAndPrivateIPs(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/rss",
		"http://localhost/rss",
		"http://[::1]/rss",
		"http://0.0.0.0/rss",
	}
	for _, u := range blocked {
		err := validateFeedURL(u)
		if err == nil {
			t.Errorf("validateFeedURL(%q) should have been rejected (SSRF)", u)
		}
	}
}

func TestValidateFeedURL_BlocksMetadataEndpoints(t *testing.T) {
	blocked := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
	}
	for _, u := range blocked {
		err := validateFeedURL(u)
		if err == nil {
			t.Errorf("validateFeedURL(%q) should have been rejected (cloud metadata)", u)
		}
	}
}

func TestValidateFeedURL_BlocksEmptyAndInvalidURLs(t *testing.T) {
	blocked := []string{
		"",
		"not-a-url",
		"://missing-scheme",
	}
	for _, u := range blocked {
		err := validateFeedURL(u)
		if err == nil {
			t.Errorf("validateFeedURL(%q) should have been rejected", u)
		}
	}
}

func isExpectedDNSError(err error) bool {
	s := err.Error()
	return testing.Short() ||
		contains(s, "DNS") || contains(s, "dns") ||
		contains(s, "no such host") || contains(s, "lookup") ||
		contains(s, "resolution failed")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestConnect_LoadsFeedURLs(t *testing.T) {
	c := New("rss-1", nil)
	err := c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"feed_urls": []interface{}{
				"https://example.com/rss",
				"https://blog.example.com/feed",
			},
		},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if len(c.feedURLs) != 2 {
		t.Errorf("expected 2 feed URLs, got %d", len(c.feedURLs))
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy, got %v", c.Health(context.Background()))
	}
}

func TestConnect_PreservesConstructorURLs(t *testing.T) {
	c := New("rss-1", []string{"https://existing.com/rss"})
	c.Connect(context.Background(), connector.ConnectorConfig{
		SourceConfig: map[string]interface{}{
			"feed_urls": []interface{}{"https://new.com/rss"},
		},
	})
	if len(c.feedURLs) != 2 {
		t.Errorf("expected 2 feed URLs (existing + new), got %d", len(c.feedURLs))
	}
}

func TestClose_SetsDisconnected(t *testing.T) {
	c := New("rss-1", nil)
	c.Connect(context.Background(), connector.ConnectorConfig{})
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected after close, got %v", c.Health(context.Background()))
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New("feed-x", nil)
	if c.ID() != "feed-x" {
		t.Errorf("expected ID 'feed-x', got %q", c.ID())
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected before connect, got %v", c.Health(context.Background()))
	}
}

func TestParseRSSItems_RFC1123Date(t *testing.T) {
	items := []rssItem{
		{
			Title:   "RFC1123",
			Link:    "https://example.com/rfc1123",
			PubDate: "Mon, 06 Apr 2026 10:00:00 GMT",
			GUID:    "g1",
		},
	}
	result := parseRSSItems(items)
	if result[0].Published.IsZero() {
		t.Error("expected parsed RFC1123 date, got zero time")
	}
}

func TestParseRSSItems_InvalidDate(t *testing.T) {
	items := []rssItem{
		{Title: "Bad Date", PubDate: "not-a-date", GUID: "g2"},
	}
	result := parseRSSItems(items)
	// Should fall back to time.Now()
	if result[0].Published.IsZero() {
		t.Error("expected fallback to time.Now(), got zero time")
	}
	if time.Since(result[0].Published) > time.Minute {
		t.Error("expected recent fallback time")
	}
}

func TestParseAtomEntries_Basic(t *testing.T) {
	entries := []atomEntry{
		{
			Title: "Atom Post",
			Links: []atomLink{
				{Href: "https://example.com/atom", Rel: "alternate"},
			},
			Summary: "Atom summary",
			Updated: "2026-04-05T10:00:00Z",
			ID:      "atom-1",
		},
	}
	entries[0].Author.Name = "AtomAuthor"

	result := parseAtomEntries(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0].Title != "Atom Post" {
		t.Errorf("title: %q", result[0].Title)
	}
	if result[0].Link != "https://example.com/atom" {
		t.Errorf("link: %q", result[0].Link)
	}
	if result[0].Author != "AtomAuthor" {
		t.Errorf("author: %q", result[0].Author)
	}
	if result[0].GUID != "atom-1" {
		t.Errorf("GUID: %q", result[0].GUID)
	}
}

func TestParseAtomEntries_FallbackContent(t *testing.T) {
	entries := []atomEntry{
		{
			Title:   "Content Only",
			Content: "Full content here",
			ID:      "atom-2",
		},
	}
	result := parseAtomEntries(entries)
	if result[0].Description != "Full content here" {
		t.Errorf("expected content as description, got %q", result[0].Description)
	}
}

func TestParseAtomEntries_FallbackLinkAndGUID(t *testing.T) {
	entries := []atomEntry{
		{
			Title: "No Alt Link",
			Links: []atomLink{
				{Href: "https://example.com/first", Rel: "self"},
			},
		},
	}
	result := parseAtomEntries(entries)
	// Should fall back to first link
	if result[0].Link != "https://example.com/first" {
		t.Errorf("expected fallback to first link, got %q", result[0].Link)
	}
	// GUID should fall back to link
	if result[0].GUID != "https://example.com/first" {
		t.Errorf("expected GUID fallback to link, got %q", result[0].GUID)
	}
}

func TestParseAtomEntries_InvalidDate(t *testing.T) {
	entries := []atomEntry{
		{Title: "Bad Date", Updated: "not-a-date", ID: "a3"},
	}
	result := parseAtomEntries(entries)
	if result[0].Published.IsZero() {
		t.Error("expected fallback time.Now() for invalid date")
	}
}

func TestIsPrivateIP_Loopback(t *testing.T) {
	if !isPrivateIP(net.ParseIP("127.0.0.1")) {
		t.Error("127.0.0.1 should be private")
	}
	if !isPrivateIP(net.ParseIP("::1")) {
		t.Error("::1 should be private")
	}
}

func TestIsPrivateIP_RFC1918(t *testing.T) {
	private := []string{"10.0.0.1", "172.16.0.1", "192.168.1.1"}
	for _, ip := range private {
		if !isPrivateIP(net.ParseIP(ip)) {
			t.Errorf("%s should be private", ip)
		}
	}
}

func TestIsPrivateIP_Public(t *testing.T) {
	public := []string{"8.8.8.8", "1.1.1.1", "203.0.113.1"}
	for _, ip := range public {
		if isPrivateIP(net.ParseIP(ip)) {
			t.Errorf("%s should NOT be private", ip)
		}
	}
}

func TestIsPrivateIP_MetadataIP(t *testing.T) {
	if !isPrivateIP(net.ParseIP("169.254.169.254")) {
		t.Error("169.254.169.254 (metadata) should be private")
	}
}

func TestIsPrivateIP_LinkLocal(t *testing.T) {
	if !isPrivateIP(net.ParseIP("169.254.0.1")) {
		t.Error("169.254.0.1 should be link-local/private")
	}
}

func TestIsPrivateIP_Unspecified(t *testing.T) {
	if !isPrivateIP(net.ParseIP("0.0.0.0")) {
		t.Error("0.0.0.0 should be private")
	}
}
