package rss

import (
	"testing"
	"time"
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
