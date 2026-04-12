package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	defer func() {
		if c.health == connector.HealthSyncing {
			c.health = connector.HealthHealthy
		}
	}()

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

// validateFeedURL checks a feed URL for SSRF risks: scheme must be http(s),
// hostname must not resolve to private, loopback, or link-local addresses,
// and known cloud metadata endpoints are blocked.
func validateFeedURL(feedURL string) error {
	u, err := url.Parse(feedURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Scheme allowlist
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme %q not allowed, only http and https", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("empty hostname")
	}

	// Block known cloud metadata hostnames
	blockedHosts := []string{
		"metadata.google.internal",
		"metadata.google",
	}
	for _, blocked := range blockedHosts {
		if strings.EqualFold(host, blocked) {
			return fmt.Errorf("hostname %q is blocked (cloud metadata)", host)
		}
	}

	// Resolve hostname and check all IPs
	ips, err := net.LookupHost(host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %q: %w", host, err)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if isPrivateIP(ip) {
			return fmt.Errorf("resolved IP %s is a private/reserved address", ipStr)
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is private, loopback, link-local,
// or a known cloud metadata address.
func isPrivateIP(ip net.IP) bool {
	// Loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return true
	}
	// Link-local (169.254.0.0/16, fe80::/10)
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// Unspecified (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}

	// RFC1918 private ranges
	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"fc00::/7"}, // IPv6 unique local
	}
	for _, r := range privateRanges {
		_, cidr, err := net.ParseCIDR(r.network)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	// AWS/GCP/Azure metadata IP
	metadataIP := net.ParseIP("169.254.169.254")
	if ip.Equal(metadataIP) {
		return true
	}

	return false
}

// FetchFeed fetches and parses an RSS or Atom feed from a URL.
func FetchFeed(ctx context.Context, feedURL string) ([]FeedItem, error) {
	if err := validateFeedURL(feedURL); err != nil {
		return nil, fmt.Errorf("feed URL rejected: %w", err)
	}

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

	// Limit feed body to 5MB to prevent resource exhaustion
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read feed body: %w", err)
	}

	// Try RSS first
	var rss rssDocument
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		return parseRSSItems(rss.Channel.Items), nil
	}

	// Try Atom
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err == nil && len(atom.Entries) > 0 {
		return parseAtomEntries(atom.Entries), nil
	}

	return nil, fmt.Errorf("could not parse feed at %s (neither RSS nor Atom)", feedURL)
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

// Atom feed types and parser
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string     `xml:"title"`
	Links   []atomLink `xml:"link"`
	Summary string     `xml:"summary"`
	Content string     `xml:"content"`
	Updated string     `xml:"updated"`
	Author  struct {
		Name string `xml:"name"`
	} `xml:"author"`
	ID string `xml:"id"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func parseAtomEntries(entries []atomEntry) []FeedItem {
	var result []FeedItem
	for _, entry := range entries {
		// Find the best link (prefer alternate, fall back to first)
		link := ""
		for _, l := range entry.Links {
			if l.Rel == "alternate" || l.Rel == "" {
				link = l.Href
				break
			}
		}
		if link == "" && len(entry.Links) > 0 {
			link = entry.Links[0].Href
		}

		// Use content or summary
		desc := entry.Summary
		if desc == "" {
			desc = entry.Content
		}

		published, _ := time.Parse(time.RFC3339, entry.Updated)
		if published.IsZero() {
			published = time.Now()
		}

		guid := entry.ID
		if guid == "" {
			guid = link
		}

		result = append(result, FeedItem{
			Title:       entry.Title,
			Link:        link,
			Description: desc,
			Published:   published,
			Author:      entry.Author.Name,
			GUID:        guid,
		})
	}
	return result
}

var _ connector.Connector = (*Connector)(nil)
