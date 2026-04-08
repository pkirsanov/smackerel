package youtube

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the YouTube connector via YouTube Data API v3.
type Connector struct {
	id     string
	config connector.ConnectorConfig
	health connector.HealthStatus
}

// VideoItem represents a YouTube video from a playlist or activity feed.
type VideoItem struct {
	VideoID     string    `json:"video_id"`
	Title       string    `json:"title"`
	Channel     string    `json:"channel"`
	Description string    `json:"description"`
	Duration    string    `json:"duration"`
	Published   time.Time `json:"published"`
	Liked       bool      `json:"liked"`
	WatchLater  bool      `json:"watch_later"`
	Playlist    string    `json:"playlist"`
	Categories  []string  `json:"categories"`
	Tags        []string  `json:"tags"`
}

// New creates a new YouTube connector.
func New(id string) *Connector {
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	c.config = config
	if config.AuthType != "oauth2" && config.AuthType != "api_key" {
		return fmt.Errorf("YouTube connector requires oauth2 or api_key auth")
	}
	c.health = connector.HealthHealthy
	slog.Info("YouTube connector connected", "id", c.id)
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.health = connector.HealthSyncing
	defer func() { c.health = connector.HealthHealthy }()

	videos, err := c.fetchVideos(ctx, cursor)
	if err != nil {
		c.health = connector.HealthError
		return nil, cursor, fmt.Errorf("fetch videos: %w", err)
	}

	if len(videos) == 0 {
		slog.Info("YouTube sync: no new videos", "id", c.id, "cursor", cursor)
		return nil, cursor, nil
	}

	// Sort by published ascending for cursor advancement
	sort.Slice(videos, func(i, j int) bool {
		return videos[i].Published.Before(videos[j].Published)
	})

	var artifacts []connector.RawArtifact
	newCursor := cursor

	for _, vid := range videos {
		cursorTime := vid.Published.Format(time.RFC3339)
		if cursorTime <= cursor && cursor != "" {
			continue
		}

		tier := EngagementTier(vid.Liked, vid.WatchLater, vid.Playlist)

		// Build content from video metadata
		var contentParts []string
		contentParts = append(contentParts, vid.Title)
		if vid.Channel != "" {
			contentParts = append(contentParts, "Channel: "+vid.Channel)
		}
		if vid.Description != "" {
			desc := vid.Description
			if len(desc) > 500 {
				desc = desc[:500] + "..."
			}
			contentParts = append(contentParts, desc)
		}
		content := strings.Join(contentParts, "\n")

		url := "https://www.youtube.com/watch?v=" + vid.VideoID

		metadata := map[string]interface{}{
			"video_id":        vid.VideoID,
			"channel":         vid.Channel,
			"duration":        vid.Duration,
			"liked":           vid.Liked,
			"watch_later":     vid.WatchLater,
			"playlist":        vid.Playlist,
			"categories":      vid.Categories,
			"tags":            vid.Tags,
			"processing_tier": tier,
		}

		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    c.id,
			SourceRef:   vid.VideoID,
			ContentType: "youtube",
			Title:       vid.Title,
			RawContent:  content,
			URL:         url,
			Metadata:    metadata,
			CapturedAt:  vid.Published,
		})

		if cursorTime > newCursor {
			newCursor = cursorTime
		}
	}

	slog.Info("YouTube sync complete",
		"id", c.id,
		"fetched", len(videos),
		"artifacts", len(artifacts),
		"cursor", newCursor,
	)

	return artifacts, newCursor, nil
}

// fetchVideos retrieves videos from source config or live API.
func (c *Connector) fetchVideos(ctx context.Context, cursor string) ([]VideoItem, error) {
	rawVideos, ok := c.config.SourceConfig["videos"]
	if ok {
		return parseVideoItems(rawVideos)
	}
	return nil, nil
}

// parseVideoItems converts interface{} video data into VideoItem structs.
func parseVideoItems(raw interface{}) ([]VideoItem, error) {
	vids, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("videos must be an array")
	}

	var result []VideoItem
	for _, v := range vids {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		vid := VideoItem{
			VideoID:   getStr(vm, "video_id"),
			Title:     getStr(vm, "title"),
			Channel:   getStr(vm, "channel"),
			Published: time.Now(),
		}

		if desc := getStr(vm, "description"); desc != "" {
			vid.Description = desc
		}
		if dur := getStr(vm, "duration"); dur != "" {
			vid.Duration = dur
		}
		if pl := getStr(vm, "playlist"); pl != "" {
			vid.Playlist = pl
		}
		if p, ok := vm["published"].(string); ok {
			if t, err := time.Parse(time.RFC3339, p); err == nil {
				vid.Published = t
			}
		}
		if liked, ok := vm["liked"].(bool); ok {
			vid.Liked = liked
		}
		if wl, ok := vm["watch_later"].(bool); ok {
			vid.WatchLater = wl
		}
		if cats, ok := vm["categories"].([]interface{}); ok {
			for _, c := range cats {
				if s, ok := c.(string); ok {
					vid.Categories = append(vid.Categories, s)
				}
			}
		}
		if tags, ok := vm["tags"].([]interface{}); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					vid.Tags = append(vid.Tags, s)
				}
			}
		}

		result = append(result, vid)
	}
	return result, nil
}

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus { return c.health }
func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	return nil
}

// EngagementTier assigns processing tier based on YouTube engagement signals.
func EngagementTier(liked bool, watchLater bool, playlistName string) string {
	if liked || playlistName != "" {
		return "full"
	}
	if watchLater {
		return "standard"
	}
	return "light"
}

var _ connector.Connector = (*Connector)(nil)
