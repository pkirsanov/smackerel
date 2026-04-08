package youtube

import (
	"context"
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
	tier := EngagementTier(true, false, "")
	if tier != "full" {
		t.Errorf("expected full for liked, got %q", tier)
	}
}

func TestEngagementTier_Playlist(t *testing.T) {
	tier := EngagementTier(false, false, "SaaS Content")
	if tier != "full" {
		t.Errorf("expected full for playlist, got %q", tier)
	}
}

func TestEngagementTier_WatchLater(t *testing.T) {
	tier := EngagementTier(false, true, "")
	if tier != "standard" {
		t.Errorf("expected standard for watch later, got %q", tier)
	}
}

func TestEngagementTier_Default(t *testing.T) {
	tier := EngagementTier(false, false, "")
	if tier != "light" {
		t.Errorf("expected light by default, got %q", tier)
	}
}
