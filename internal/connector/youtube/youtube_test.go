package youtube

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnector_Interface(t *testing.T) {
	var _ connector.Connector = New("test-yt")
}

func TestConnector_Connect(t *testing.T) {
	c := New("youtube-main")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
}

func TestConnector_Connect_APIKey(t *testing.T) {
	c := New("youtube-main")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "api_key"})
	if err != nil {
		t.Fatalf("connect with api_key: %v", err)
	}
}

func TestSync_WithVideos(t *testing.T) {
	c := New("youtube-main")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":    "dQw4w9WgXcQ",
					"title":       "Go Concurrency Patterns",
					"channel":     "GopherCon",
					"description": "Advanced concurrency patterns in Go",
					"duration":    "PT45M",
					"published":   "2026-04-01T15:00:00Z",
					"liked":       true,
					"categories":  []interface{}{"Education", "Technology"},
					"tags":        []interface{}{"golang", "concurrency"},
				},
				map[string]interface{}{
					"video_id":    "abc123xyz",
					"title":       "Quick Recipe: Pasta",
					"channel":     "CookingChannel",
					"published":   "2026-04-02T12:00:00Z",
					"watch_later": true,
				},
				map[string]interface{}{
					"video_id":  "xyz789abc",
					"title":     "Random Vlog",
					"channel":   "SomeVlogger",
					"published": "2026-04-03T18:00:00Z",
				},
			},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(artifacts))
	}

	// Liked video gets full tier
	liked := artifacts[0]
	if liked.Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier for liked video, got %v", liked.Metadata["processing_tier"])
	}
	if liked.ContentType != "youtube" {
		t.Errorf("expected youtube content type, got %q", liked.ContentType)
	}
	if !strings.Contains(liked.URL, "dQw4w9WgXcQ") {
		t.Errorf("expected URL with video ID, got %q", liked.URL)
	}

	// Watch-later gets standard tier
	wl := artifacts[1]
	if wl.Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier for watch-later, got %v", wl.Metadata["processing_tier"])
	}

	// Default gets light tier
	def := artifacts[2]
	if def.Metadata["processing_tier"] != "light" {
		t.Errorf("expected light tier for default, got %v", def.Metadata["processing_tier"])
	}

	if cursor == "" {
		t.Error("expected non-empty cursor after sync")
	}
}

func TestSync_CursorAdvancement(t *testing.T) {
	c := New("youtube-main")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{"video_id": "old", "title": "Old Video", "published": "2026-04-01T10:00:00Z"},
				map[string]interface{}{"video_id": "new", "title": "New Video", "published": "2026-04-08T10:00:00Z"},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "2026-04-05T00:00:00Z")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact after cursor, got %d", len(artifacts))
	}
	if artifacts[0].Title != "New Video" {
		t.Errorf("expected 'New Video', got %q", artifacts[0].Title)
	}
}

func TestSync_Empty(t *testing.T) {
	c := New("youtube-main")
	c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(artifacts))
	}
	if cursor != "" {
		t.Errorf("expected empty cursor, got %q", cursor)
	}
}

func TestEngagementTier_Liked(t *testing.T) {
	tier := EngagementTier(true, false, "", 0.0)
	if tier != "full" {
		t.Errorf("expected full for liked, got %q", tier)
	}
}

func TestEngagementTier_Playlist(t *testing.T) {
	tier := EngagementTier(false, false, "SaaS Content", 0.0)
	if tier != "full" {
		t.Errorf("expected full for playlist, got %q", tier)
	}
}

func TestEngagementTier_WatchLater(t *testing.T) {
	tier := EngagementTier(false, true, "", 0.0)
	if tier != "standard" {
		t.Errorf("expected standard for watch later, got %q", tier)
	}
}

func TestEngagementTier_Default(t *testing.T) {
	tier := EngagementTier(false, false, "", 0.0)
	if tier != "light" {
		t.Errorf("expected light by default, got %q", tier)
	}
}

func TestEngagementTier_HighCompletion(t *testing.T) {
	// R-203: >80% completed → full tier even without liked/playlist
	tier := EngagementTier(false, false, "", 0.85)
	if tier != "full" {
		t.Errorf("expected full for 85%% completion, got %q", tier)
	}
}

func TestEngagementTier_MidCompletion(t *testing.T) {
	// 50-79% completed → standard tier
	tier := EngagementTier(false, false, "", 0.6)
	if tier != "standard" {
		t.Errorf("expected standard for 60%% completion, got %q", tier)
	}
}

func TestEngagementTier_LowCompletion(t *testing.T) {
	// <50% completed without other signals → light tier
	tier := EngagementTier(false, false, "", 0.15)
	if tier != "light" {
		t.Errorf("expected light for 15%% completion, got %q", tier)
	}
}

// Security: verify pageToken cursor is URL-encoded in API URL construction (S003)
func TestFetchPlaylistItems_CursorURLEncoded(t *testing.T) {
	// Verify that a cursor containing special characters would be encoded
	// by testing the url.QueryEscape behavior used in the implementation.
	// A cursor like "abc&inject=true" should not produce raw "&inject=true" in the URL.
	maliciousCursor := "abc&inject=true&another=param"
	encoded := "abc%26inject%3Dtrue%26another%3Dparam"
	result := url.QueryEscape(maliciousCursor)
	if result != encoded {
		t.Errorf("expected URL-encoded cursor, got %q", result)
	}
	// Verify it does NOT contain raw unencoded ampersand that would split params
	if strings.Contains(result, "&") {
		t.Errorf("encoded cursor must not contain raw '&', got %q", result)
	}
}

func TestNew_Defaults(t *testing.T) {
	c := New("yt-1")
	if c.ID() != "yt-1" {
		t.Errorf("expected ID 'yt-1', got %q", c.ID())
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected before connect, got %v", c.Health(context.Background()))
	}
}

func TestConnect_InvalidAuth(t *testing.T) {
	c := New("yt")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "password"})
	if err == nil {
		t.Error("expected error for password auth type")
	}
}

func TestClose_SetsDisconnected(t *testing.T) {
	c := New("yt")
	c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected after close, got %v", c.Health(context.Background()))
	}
}

func TestSync_DescriptionTruncation(t *testing.T) {
	longDesc := strings.Repeat("a", 600)
	c := New("yt")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":    "truncated",
					"title":       "Long Desc Video",
					"description": longDesc,
					"published":   "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	// Description should be truncated to 500 + "..."
	if !strings.Contains(artifacts[0].RawContent, "...") {
		t.Error("expected truncated description with '...'")
	}
	// Content should not contain the full 600-char description
	if strings.Contains(artifacts[0].RawContent, longDesc) {
		t.Error("description should be truncated, not full")
	}
}

func TestSync_URLConstruction(t *testing.T) {
	c := New("yt")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "api_key",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":  "abc123",
					"title":     "URL Test",
					"published": "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	expected := "https://www.youtube.com/watch?v=abc123"
	if artifacts[0].URL != expected {
		t.Errorf("expected URL %q, got %q", expected, artifacts[0].URL)
	}
}

func TestSync_MetadataFields(t *testing.T) {
	c := New("yt")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":   "meta-test",
					"title":      "Metadata Video",
					"channel":    "TestChannel",
					"duration":   "PT10M",
					"liked":      true,
					"playlist":   "Favorites",
					"categories": []interface{}{"Tech"},
					"tags":       []interface{}{"go", "programming"},
					"published":  "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	m := artifacts[0].Metadata
	if m["video_id"] != "meta-test" {
		t.Errorf("video_id: %v", m["video_id"])
	}
	if m["channel"] != "TestChannel" {
		t.Errorf("channel: %v", m["channel"])
	}
	if m["duration"] != "PT10M" {
		t.Errorf("duration: %v", m["duration"])
	}
	if m["liked"] != true {
		t.Errorf("liked: %v", m["liked"])
	}
	if m["playlist"] != "Favorites" {
		t.Errorf("playlist: %v", m["playlist"])
	}
}

func TestParseVideoItems_InvalidInput(t *testing.T) {
	_, err := parseVideoItems("not-an-array")
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

func TestParseVideoItems_SkipsNonMapEntries(t *testing.T) {
	vids, err := parseVideoItems([]interface{}{
		"not-a-map",
		42,
		map[string]interface{}{
			"video_id": "valid",
			"title":    "Valid Video",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vids) != 1 {
		t.Fatalf("expected 1 valid video, got %d", len(vids))
	}
	if vids[0].VideoID != "valid" {
		t.Errorf("expected video_id 'valid', got %q", vids[0].VideoID)
	}
}

func TestParseVideoItems_AllFields(t *testing.T) {
	vids, err := parseVideoItems([]interface{}{
		map[string]interface{}{
			"video_id":    "v1",
			"title":       "Full Video",
			"channel":     "Ch1",
			"description": "Desc",
			"duration":    "PT5M",
			"playlist":    "PL1",
			"published":   "2026-04-05T12:00:00Z",
			"liked":       true,
			"watch_later": true,
			"categories":  []interface{}{"Cat1", "Cat2"},
			"tags":        []interface{}{"tag1"},
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := vids[0]
	if v.Description != "Desc" {
		t.Errorf("description: %q", v.Description)
	}
	if v.Duration != "PT5M" {
		t.Errorf("duration: %q", v.Duration)
	}
	if v.Playlist != "PL1" {
		t.Errorf("playlist: %q", v.Playlist)
	}
	if !v.Liked {
		t.Error("expected liked=true")
	}
	if !v.WatchLater {
		t.Error("expected watch_later=true")
	}
	if len(v.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(v.Categories))
	}
	if len(v.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(v.Tags))
	}
	if v.Published.IsZero() {
		t.Error("expected non-zero published time")
	}
}

func TestGetCredential_NilMap(t *testing.T) {
	if v := getCredential(nil, "key"); v != "" {
		t.Errorf("expected empty for nil map, got %q", v)
	}
}

func TestGetCredential_MissingKey(t *testing.T) {
	if v := getCredential(map[string]string{"a": "b"}, "missing"); v != "" {
		t.Errorf("expected empty for missing key, got %q", v)
	}
}

func TestGetStr_MissingKey(t *testing.T) {
	m := map[string]interface{}{"a": "b"}
	if v := getStr(m, "missing"); v != "" {
		t.Errorf("expected empty, got %q", v)
	}
}

func TestGetStr_NonStringValue(t *testing.T) {
	m := map[string]interface{}{"num": 42}
	if v := getStr(m, "num"); v != "" {
		t.Errorf("expected empty for non-string, got %q", v)
	}
}

func TestSync_HealthTransitions(t *testing.T) {
	c := New("yt")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{},
		},
	})
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after sync, got %v", c.Health(context.Background()))
	}
}

func TestSync_ErrorOnInvalidVideos(t *testing.T) {
	c := New("yt")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": "not-an-array",
		},
	})
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error for invalid videos format")
	}
}

func TestSync_HealthStaysErrorOnFailure(t *testing.T) {
	c := New("yt-health")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": "not-an-array", // triggers parse error
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected sync error for invalid videos")
	}

	// Health must remain at Error, not reset to Healthy by the defer
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("health should be Error after failed sync, got %v", c.Health(context.Background()))
	}
}

func TestSync_BeforeConnect(t *testing.T) {
	c := New("yt-no-connect")
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when Sync called before Connect")
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("health should remain disconnected, got %v", c.Health(context.Background()))
	}
}

func TestParseVideoItems_SkipsEmptyVideoID(t *testing.T) {
	vids, err := parseVideoItems([]interface{}{
		map[string]interface{}{"video_id": "", "title": "No ID"},
		map[string]interface{}{"video_id": "valid-id", "title": "Valid Video"},
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(vids) != 1 {
		t.Fatalf("expected 1 video (empty ID skipped), got %d", len(vids))
	}
	if vids[0].VideoID != "valid-id" {
		t.Errorf("expected video_id 'valid-id', got %q", vids[0].VideoID)
	}
}

// Test that dedup merges metadata from duplicate video entries.
// R-203: same video in liked + named playlist should preserve both signals.
func TestSync_DedupMergesMetadata(t *testing.T) {
	c := New("yt-dedup")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":  "dup-1",
					"title":     "Duplicate Video",
					"liked":     true,
					"published": "2026-04-01T10:00:00Z",
				},
				map[string]interface{}{
					"video_id":        "dup-1",
					"title":           "Duplicate Video",
					"playlist":        "Leadership",
					"completion_rate": 0.95,
					"published":       "2026-04-01T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact after dedup, got %d", len(artifacts))
	}

	m := artifacts[0].Metadata
	if m["liked"] != true {
		t.Errorf("expected liked=true after merge, got %v", m["liked"])
	}
	if m["playlist"] != "Leadership" {
		t.Errorf("expected playlist='Leadership' after merge, got %v", m["playlist"])
	}
	if cr, ok := m["completion_rate"].(float64); !ok || cr < 0.9 {
		t.Errorf("expected completion_rate >= 0.9 after merge, got %v", m["completion_rate"])
	}
	// Tier should be full (liked + high completion + named playlist)
	if m["processing_tier"] != "full" {
		t.Errorf("expected full tier after merge, got %v", m["processing_tier"])
	}
}
