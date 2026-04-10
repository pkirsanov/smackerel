package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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
	// Check for local/test videos in source_config (testing path)
	rawVideos, ok := c.config.SourceConfig["videos"]
	if ok {
		return parseVideoItems(rawVideos)
	}

	// Check for OAuth access token (live API path)
	accessToken := getCredential(c.config.Credentials, "access_token")
	if accessToken == "" {
		slog.Debug("YouTube: no source_config videos and no access_token", "id", c.id)
		return nil, nil
	}

	return c.fetchYouTubeVideos(ctx, accessToken, cursor)
}

// fetchYouTubeVideos fetches liked videos and playlist items from YouTube Data API v3.
func (c *Connector) fetchYouTubeVideos(ctx context.Context, token string, cursor string) ([]VideoItem, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	var allVideos []VideoItem

	// Fetch liked videos
	likedVideos, err := c.fetchPlaylistItems(ctx, client, token, "LL", cursor, true)
	if err != nil {
		slog.Warn("youtube: failed to fetch liked videos", "error", err)
	} else {
		allVideos = append(allVideos, likedVideos...)
	}

	// Fetch watch later
	wlVideos, err := c.fetchPlaylistItems(ctx, client, token, "WL", cursor, false)
	if err != nil {
		slog.Warn("youtube: failed to fetch watch later", "error", err)
	} else {
		for i := range wlVideos {
			wlVideos[i].WatchLater = true
		}
		allVideos = append(allVideos, wlVideos...)
	}

	// Fetch custom playlists
	playlists, err := c.fetchUserPlaylists(ctx, client, token)
	if err != nil {
		slog.Warn("youtube: failed to fetch playlists", "error", err)
	} else {
		for _, pl := range playlists {
			plVideos, err := c.fetchPlaylistItems(ctx, client, token, pl.ID, cursor, false)
			if err != nil {
				slog.Warn("youtube: failed to fetch playlist", "playlist_id", pl.ID, "error", err)
				continue
			}
			for i := range plVideos {
				plVideos[i].Playlist = pl.Title
			}
			allVideos = append(allVideos, plVideos...)
		}
	}

	// Deduplicate by video ID (same video may appear in liked + playlist)
	seen := make(map[string]bool)
	var deduped []VideoItem
	for _, v := range allVideos {
		if !seen[v.VideoID] {
			seen[v.VideoID] = true
			deduped = append(deduped, v)
		}
	}

	slog.Info("youtube API fetch complete", "total_videos", len(deduped))
	return deduped, nil
}

type ytPlaylist struct {
	ID    string
	Title string
}

// fetchUserPlaylists lists the user's custom playlists.
func (c *Connector) fetchUserPlaylists(ctx context.Context, client *http.Client, token string) ([]ytPlaylist, error) {
	apiURL := "https://www.googleapis.com/youtube/v3/playlists?part=snippet&mine=true&maxResults=25"

	data, err := youtubeAPICall(ctx, client, apiURL, token)
	if err != nil {
		return nil, err
	}

	items, _ := data["items"].([]interface{})
	var playlists []ytPlaylist
	for _, item := range items {
		im, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		snippet, _ := im["snippet"].(map[string]interface{})
		if snippet == nil {
			continue
		}
		id, _ := im["id"].(string)
		title, _ := snippet["title"].(string)
		if id != "" && title != "" {
			playlists = append(playlists, ytPlaylist{ID: id, Title: title})
		}
	}
	return playlists, nil
}

// fetchPlaylistItems fetches video items from a specific YouTube playlist.
func (c *Connector) fetchPlaylistItems(ctx context.Context, client *http.Client, token string, playlistID string, cursor string, markLiked bool) ([]VideoItem, error) {
	apiURL := fmt.Sprintf("https://www.googleapis.com/youtube/v3/playlistItems?part=snippet,contentDetails&playlistId=%s&maxResults=50",
		playlistID)
	if cursor != "" {
		apiURL += "&pageToken=" + url.QueryEscape(cursor)
	}

	data, err := youtubeAPICall(ctx, client, apiURL, token)
	if err != nil {
		return nil, err
	}

	items, _ := data["items"].([]interface{})
	var videos []VideoItem

	for _, item := range items {
		im, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		snippet, _ := im["snippet"].(map[string]interface{})
		if snippet == nil {
			continue
		}
		contentDetails, _ := im["contentDetails"].(map[string]interface{})

		var videoID string
		if contentDetails != nil {
			videoID, _ = contentDetails["videoId"].(string)
		}
		if videoID == "" {
			// Try resource ID
			resID, _ := snippet["resourceId"].(map[string]interface{})
			if resID != nil {
				videoID, _ = resID["videoId"].(string)
			}
		}
		if videoID == "" {
			continue
		}

		title, _ := snippet["title"].(string)
		channel, _ := snippet["channelTitle"].(string)
		desc, _ := snippet["description"].(string)
		publishedAt, _ := snippet["publishedAt"].(string)

		vid := VideoItem{
			VideoID:     videoID,
			Title:       title,
			Channel:     channel,
			Description: desc,
			Liked:       markLiked,
			Published:   time.Now(),
		}

		if publishedAt != "" {
			if t, err := time.Parse(time.RFC3339, publishedAt); err == nil {
				vid.Published = t
			}
		}

		// Extract categories/tags from snippet if available
		if tags, ok := snippet["tags"].([]interface{}); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					vid.Tags = append(vid.Tags, s)
				}
			}
		}

		videos = append(videos, vid)
	}

	return videos, nil
}

// youtubeAPICall makes an authenticated GET request to the YouTube Data API v3.
func youtubeAPICall(ctx context.Context, client *http.Client, apiURL string, token string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("youtube API: token expired or invalid (401)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("youtube API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Limit response body to 10MB to prevent resource exhaustion
	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10*1024*1024)).Decode(&result); err != nil {
		return nil, fmt.Errorf("youtube API: decode response: %w", err)
	}
	return result, nil
}

func getCredential(creds map[string]string, key string) string {
	if creds == nil {
		return ""
	}
	return creds[key]
}

// parseVideoItems converts interface{} video data into VideoItem structs.
func parseVideoItems(raw interface{}) ([]VideoItem, error) {
	if raw == nil {
		return nil, nil
	}
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
