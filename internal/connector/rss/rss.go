package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// FeedItem represents a parsed RSS/Atom feed item.
type FeedItem struct {
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	Description string    `json:"description"`
	Published   time.Time `json:"published"`
	Author      string    `json:"author"`
	GUID        string    `json:"guid"`
}

// Connector implements the RSS/Atom feed connector.
type Connector struct {
	id       string
	feedURLs []string
	health   connector.HealthStatus
}

// New creates a new RSS connector.
func New(id string, feedURLs []string) *Connector {
	return &Connector{id: id, feedURLs: feedURLs, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	if urls, ok := config.SourceConfig["feed_urls"].([]interface{}); ok {
		for _, u := range urls {
			if s, ok := u.(string); ok {
				c.feedURLs = append(c.feedURLs, s)
			}
		}
	}
	c.health = connector.HealthHealthy
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing
	defer func() { c.health = connector.HealthHealthy }()

	var artifacts []connector.RawArtifact
	latestTime := cursor

	for _, feedURL := range c.feedURLs {
		items, err := FetchFeed(ctx, feedURL)
		if err != nil {
			slog.Warn("feed fetch failed", "url", feedURL, "error", err)
			continue
		}

		for _, item := range items {
			ts := item.Published.Format(time.RFC3339)
			if ts <= cursor {
				continue
			}
			if ts > latestTime {
				latestTime = ts
			}

			artifacts = append(artifacts, connector.RawArtifact{
				SourceID:    "rss",
				SourceRef:   item.GUID,
				ContentType: "url",
				Title:       item.Title,
				RawContent:  item.Description,
				URL:         item.Link,
				Metadata: map[string]interface{}{
					"author": item.Author,
					"feed":   feedURL,
				},
				CapturedAt: item.Published,
			})
		}
	}

	return artifacts, latestTime, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus { return c.health }
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	return nil
}

// FetchFeed fetches and parses an RSS or Atom feed from a URL.
func FetchFeed(ctx context.Context, feedURL string) ([]FeedItem, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Smackerel/1.0 (feed reader)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	// Try RSS first
	var rss rssDocument
	decoder := xml.NewDecoder(resp.Body)
	if err := decoder.Decode(&rss); err == nil && len(rss.Channel.Items) > 0 {
		return parseRSSItems(rss.Channel.Items), nil
	}

	return nil, fmt.Errorf("could not parse feed at %s", feedURL)
}

type rssDocument struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Author      string `xml:"author"`
	GUID        string `xml:"guid"`
}

func parseRSSItems(items []rssItem) []FeedItem {
	var result []FeedItem
	for _, item := range items {
		published, _ := time.Parse(time.RFC1123Z, item.PubDate)
		if published.IsZero() {
			published, _ = time.Parse(time.RFC1123, item.PubDate)
		}
		if published.IsZero() {
			published = time.Now()
		}

		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}

		result = append(result, FeedItem{
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			Published:   published,
			Author:      item.Author,
			GUID:        guid,
		})
	}
	return result
}

var _ connector.Connector = (*Connector)(nil)
