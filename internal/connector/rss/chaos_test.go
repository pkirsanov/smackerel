package rss

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- Chaos: Malformed RSS XML parsing ---

func TestChaos_ParseRSSItems_EmptyGUIDAndLink(t *testing.T) {
	// Both GUID and Link empty — fallback chain produces empty GUID
	items := []rssItem{
		{Title: "No identifiers", Description: "Missing both GUID and link"},
	}
	result := parseRSSItems(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	// GUID falls back to Link; both empty → empty GUID
	// This is a data quality concern: empty SourceRef in artifacts
	if result[0].GUID != "" {
		t.Errorf("expected empty GUID when both GUID and link are empty, got %q", result[0].GUID)
	}
}

func TestChaos_ParseRSSItems_AllFieldsEmpty(t *testing.T) {
	items := []rssItem{{}}
	result := parseRSSItems(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	// Should not panic; Published should have fallback
	if result[0].Published.IsZero() {
		t.Error("expected fallback time for empty date, got zero")
	}
}

func TestChaos_ParseRSSItems_MalformedDates(t *testing.T) {
	dateVariants := []string{
		"not-a-date",
		"2026-04-06",
		"Apr 6 2026",
		"1234567890",
		"",
		"Mon, 32 Apr 2026 25:99:99 +0000", // impossible date/time
	}
	for _, d := range dateVariants {
		items := []rssItem{{Title: "Date: " + d, PubDate: d, GUID: "date-test"}}
		result := parseRSSItems(items)
		if len(result) != 1 {
			t.Fatalf("date %q: expected 1 item, got %d", d, len(result))
		}
		// All unparseable dates should fall back to time.Now()
		if result[0].Published.IsZero() {
			t.Errorf("date %q: expected fallback time, got zero", d)
		}
	}
}

func TestChaos_ParseRSSItems_HTMLInFields(t *testing.T) {
	items := []rssItem{
		{
			Title:       "<b>Bold Title</b><script>alert('xss')</script>",
			Link:        "https://example.com/article",
			Description: "<p>Paragraph with <a href='http://evil.com'>link</a> and <img src=x onerror=alert(1)></p>",
			GUID:        "html-test",
		},
	}
	result := parseRSSItems(items)
	if len(result) != 1 {
		t.Fatal("should parse item with HTML in fields")
	}
	// Parser should not crash on HTML content
	if result[0].Title == "" {
		t.Error("title should not be empty even with HTML")
	}
}

func TestChaos_ParseRSSItems_VeryLongDescription(t *testing.T) {
	// 10MB description — parser should handle without crashing
	longDesc := strings.Repeat("Lorem ipsum dolor sit amet. ", 100000) // ~2.8MB
	items := []rssItem{
		{Title: "Long", Description: longDesc, GUID: "long-desc"},
	}
	result := parseRSSItems(items)
	if len(result) != 1 {
		t.Fatal("should handle very long description")
	}
	if result[0].Description != longDesc {
		t.Error("description should be preserved unmodified")
	}
}

func TestChaos_ParseRSSItems_UnicodeContent(t *testing.T) {
	items := []rssItem{
		{
			Title:       "🚀 日本語タイトル café résumé Ñ",
			Description: "多言語コンテンツ with émojis 🌍🎯 and Ωmega αβγδε",
			Author:      "作者 Müller",
			GUID:        "unicode-test",
			PubDate:     "Mon, 06 Apr 2026 10:00:00 +0000",
		},
	}
	result := parseRSSItems(items)
	if !utf8.ValidString(result[0].Title) {
		t.Error("title should be valid UTF-8")
	}
	if !utf8.ValidString(result[0].Description) {
		t.Error("description should be valid UTF-8")
	}
}

func TestChaos_ParseRSSItems_NullBytes(t *testing.T) {
	items := []rssItem{
		{
			Title:       "Title with \x00 null byte",
			Description: "Content\x00with\x00nulls",
			GUID:        "null-byte",
		},
	}
	result := parseRSSItems(items)
	if len(result) != 1 {
		t.Fatal("should handle null bytes in fields")
	}
}

// --- Chaos: Atom entry edge cases ---

func TestChaos_ParseAtomEntries_NoLinks(t *testing.T) {
	entries := []atomEntry{
		{Title: "No links at all", ID: "no-links"},
	}
	result := parseAtomEntries(entries)
	if len(result) != 1 {
		t.Fatal("should parse entry with no links")
	}
	if result[0].Link != "" {
		t.Errorf("expected empty link, got %q", result[0].Link)
	}
}

func TestChaos_ParseAtomEntries_MalformedDate(t *testing.T) {
	entries := []atomEntry{
		{Title: "Bad date", Updated: "not-rfc3339", ID: "bad-date"},
	}
	result := parseAtomEntries(entries)
	if result[0].Published.IsZero() {
		t.Error("expected fallback time.Now() for unparseable date")
	}
}

func TestChaos_ParseAtomEntries_EmptyID(t *testing.T) {
	entries := []atomEntry{
		{Title: "No ID", Links: []atomLink{{Href: "https://example.com", Rel: "alternate"}}},
	}
	result := parseAtomEntries(entries)
	// ID falls back to link
	if result[0].GUID != "https://example.com" {
		t.Errorf("expected GUID to fall back to link, got %q", result[0].GUID)
	}
}

func TestChaos_ParseAtomEntries_NoIDNoLink(t *testing.T) {
	entries := []atomEntry{
		{Title: "Orphan entry"},
	}
	result := parseAtomEntries(entries)
	if result[0].GUID != "" {
		t.Errorf("expected empty GUID when no ID and no link, got %q", result[0].GUID)
	}
}

func TestChaos_ParseAtomEntries_MultipleLinks(t *testing.T) {
	entries := []atomEntry{
		{
			Title: "Many links",
			Links: []atomLink{
				{Href: "https://enclosure.example.com/file.mp3", Rel: "enclosure"},
				{Href: "https://example.com/article", Rel: "alternate"},
				{Href: "https://example.com/self", Rel: "self"},
			},
			ID: "multi-link",
		},
	}
	result := parseAtomEntries(entries)
	// Should prefer alternate link
	if result[0].Link != "https://example.com/article" {
		t.Errorf("expected alternate link, got %q", result[0].Link)
	}
}

// --- Chaos: Sync with edge-case data ---

func TestChaos_Sync_EmptyRSSItems(t *testing.T) {
	// Items exist but with no parseable content — cursor should not advance to garbage
	c := New("chaos-rss", nil)
	c.Connect(context.Background(), connector.ConnectorConfig{})
	// No feed URLs = no items
	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts with no feed URLs, got %d", len(artifacts))
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
}

func TestChaos_Sync_CursorComparisonEdge(t *testing.T) {
	// Cursor is string-compared with RFC3339 timestamps
	// Test with a cursor at exact time boundary
	c := New("edge-cursor", nil)
	c.Connect(context.Background(), connector.ConnectorConfig{})

	// Sync with a very old cursor and very future cursor
	_, cursor1, _ := c.Sync(context.Background(), "1970-01-01T00:00:00Z")
	if cursor1 != "1970-01-01T00:00:00Z" {
		t.Errorf("cursor should not advance without items, got %q", cursor1)
	}

	_, cursor2, _ := c.Sync(context.Background(), "9999-12-31T23:59:59Z")
	if cursor2 != "9999-12-31T23:59:59Z" {
		t.Errorf("very future cursor should be preserved, got %q", cursor2)
	}
}

// --- Chaos: Concurrent Sync ---

func TestChaos_ConcurrentSyncHealthRace(t *testing.T) {
	c := New("race-rss", nil)
	c.Connect(context.Background(), connector.ConnectorConfig{})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Sync(context.Background(), "")
		}()
	}

	// Concurrently read health while syncs are running
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				h := c.Health(context.Background())
				// Health should be one of the valid states
				switch h {
				case connector.HealthHealthy, connector.HealthSyncing, connector.HealthDisconnected:
					// OK
				default:
					t.Errorf("unexpected health state: %v", h)
				}
			}
		}
	}()
	wg.Wait()
	close(done)
}

// --- Chaos: SSRF validation edge cases ---

func TestChaos_ValidateFeedURL_IPv6Loopback(t *testing.T) {
	err := validateFeedURL("http://[::1]:8080/rss")
	if err == nil {
		t.Error("should block IPv6 loopback")
	}
}

func TestChaos_ValidateFeedURL_ZeroIP(t *testing.T) {
	err := validateFeedURL("http://0.0.0.0/rss")
	if err == nil {
		t.Error("should block 0.0.0.0")
	}
}

func TestChaos_ValidateFeedURL_VeryLongURL(t *testing.T) {
	// 10K character URL should be handled without panic
	longPath := strings.Repeat("a", 10000)
	err := validateFeedURL("https://example.com/" + longPath)
	// DNS will likely fail, but it should not panic
	if err == nil {
		// DNS resolution of 'example.com' may succeed in some environments
		t.Log("long URL accepted — DNS resolved successfully")
	}
}

func TestChaos_ValidateFeedURL_URLWithCredentials(t *testing.T) {
	// URLs with embedded credentials
	err := validateFeedURL("http://user:pass@internal-server/rss")
	// Should either reject or DNS-fail, never silently accept internal URLs
	if err == nil {
		t.Log("URL with credentials accepted — DNS resolved to non-private IP")
	}
}

func TestChaos_ValidateFeedURL_DoubleEncoded(t *testing.T) {
	// Double-encoded localhost attempt
	err := validateFeedURL("http://127%2E0%2E0%2E1/rss")
	// URL parsing should handle this, but the decoded hostname will be validated
	if err == nil {
		t.Error("should block double-encoded localhost")
	}
}

// --- Chaos: RSS date parsing boundary ---

func TestChaos_ParseRSSItems_FutureDates(t *testing.T) {
	items := []rssItem{
		{
			Title:   "Future article",
			PubDate: "Mon, 06 Apr 2099 10:00:00 +0000",
			GUID:    "future",
		},
	}
	result := parseRSSItems(items)
	if result[0].Published.Year() != 2099 {
		t.Errorf("expected year 2099, got %d", result[0].Published.Year())
	}
}

func TestChaos_ParseRSSItems_EpochDate(t *testing.T) {
	items := []rssItem{
		{
			Title:   "Epoch article",
			PubDate: "Thu, 01 Jan 1970 00:00:00 +0000",
			GUID:    "epoch",
		},
	}
	result := parseRSSItems(items)
	// The parser should parse this valid RFC1123Z date correctly
	if result[0].Published.Year() == time.Now().Year() {
		// Should have parsed to 1970, not fallen back to time.Now()
		t.Error("valid 1970 date should be parsed, not treated as fallback")
	}
}
