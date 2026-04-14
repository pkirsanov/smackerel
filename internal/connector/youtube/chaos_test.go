package youtube

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- Chaos: Video parsing edge cases ---

func TestChaos_ParseVideoItems_AllFieldsEmpty(t *testing.T) {
	c := New("chaos-yt")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{}, // completely empty video
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Empty video_id → skipped by empty-VideoID guard
	if len(artifacts) != 0 {
		t.Fatalf("expected 0 artifacts for empty-VideoID video, got %d", len(artifacts))
	}
}

func TestChaos_ParseVideoItems_WrongTypes(t *testing.T) {
	c := New("chaos-types")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":    12345,      // int instead of string
					"title":       true,       // bool instead of string
					"channel":     []int{1},   // array instead of string
					"description": 42,         // int instead of string
					"published":   12345,      // int instead of string
					"liked":       "yes",      // string instead of bool
					"categories":  "not-list", // string instead of array
				},
			},
		},
	})

	// Should not panic
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync should handle wrong types: %v", err)
	}
	// video_id is int 12345, getStr returns "" → skipped by empty-VideoID guard
	if len(artifacts) != 0 {
		t.Fatalf("expected 0 artifacts for invalid-VideoID video, got %d", len(artifacts))
	}
}

func TestChaos_ParseVideoItems_NonArrayInput(t *testing.T) {
	c := New("chaos-nonarray")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": "not-an-array",
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when videos is not an array")
	}
}

func TestChaos_ParseVideoItems_MixedValidInvalid(t *testing.T) {
	c := New("chaos-mixed")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				"not-a-map",
				42,
				nil,
				map[string]interface{}{"video_id": "valid123", "title": "Valid", "published": "2026-04-08T10:00:00Z"},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact from mixed input, got %d", len(artifacts))
	}
	if artifacts[0].SourceRef != "valid123" {
		t.Errorf("expected valid video, got %q", artifacts[0].SourceRef)
	}
}

// --- Chaos: Invalid published dates ---

func TestChaos_Sync_InvalidPublishedDate(t *testing.T) {
	c := New("chaos-date")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":  "bad-date",
					"title":     "Bad Date",
					"published": "not-a-date",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact")
	}
	// Published should fallback to time.Now()
	if artifacts[0].CapturedAt.Year() < 2020 {
		t.Errorf("expected recent CapturedAt, got %v", artifacts[0].CapturedAt)
	}
}

func TestChaos_Sync_FuturePublishedDate(t *testing.T) {
	c := New("chaos-future")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":  "future-1",
					"title":     "Scheduled Video",
					"published": "2099-12-31T23:59:59Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact")
	}
	if artifacts[0].CapturedAt.Year() != 2099 {
		t.Errorf("expected year 2099, got %d", artifacts[0].CapturedAt.Year())
	}
}

// --- Chaos: Unicode in video fields ---

func TestChaos_Sync_UnicodeVideoFields(t *testing.T) {
	c := New("chaos-unicode")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":    "uni-123",
					"title":       "🎬 日本語ビデオ — café tutorial",
					"channel":     "Müller's Channel",
					"description": "Описание видео 🌍 with αβγδε and 中文描述",
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
		t.Fatal("expected 1 artifact")
	}
	if !utf8.ValidString(artifacts[0].Title) {
		t.Error("title should be valid UTF-8")
	}
	if !utf8.ValidString(artifacts[0].RawContent) {
		t.Error("content should be valid UTF-8")
	}
}

// --- Chaos: EngagementTier edge cases ---

func TestChaos_EngagementTier_BothLikedAndPlaylist(t *testing.T) {
	tier := EngagementTier(true, true, "My Playlist")
	if tier != "full" {
		t.Errorf("both liked and playlist should give full, got %q", tier)
	}
}

func TestChaos_EngagementTier_AllFlagsTrue(t *testing.T) {
	tier := EngagementTier(true, true, "Playlist")
	if tier != "full" {
		t.Errorf("all flags true should give full, got %q", tier)
	}
}

func TestChaos_EngagementTier_EmptyPlaylistName(t *testing.T) {
	tier := EngagementTier(false, false, "")
	if tier != "light" {
		t.Errorf("empty playlist should give light, got %q", tier)
	}
}

func TestChaos_EngagementTier_WhitespacePlaylist(t *testing.T) {
	// Playlist name is whitespace-only — still non-empty string
	tier := EngagementTier(false, false, "   ")
	if tier != "full" {
		t.Errorf("whitespace playlist still non-empty → full, got %q", tier)
	}
}

// --- Chaos: Description truncation boundary ---

func TestChaos_Sync_DescriptionExactly500(t *testing.T) {
	desc500 := strings.Repeat("a", 500)
	c := New("chaos-exact")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":    "exact500",
					"title":       "Exact 500",
					"description": desc500,
					"published":   "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Exactly 500 chars should NOT be truncated (truncation is for > 500)
	if strings.Contains(artifacts[0].RawContent, "...") {
		t.Error("exactly 500-char description should not be truncated")
	}
}

func TestChaos_Sync_Description501(t *testing.T) {
	desc501 := strings.Repeat("b", 501)
	c := New("chaos-501")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{
					"video_id":    "over500",
					"title":       "Over 500",
					"description": desc501,
					"published":   "2026-04-08T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !strings.HasSuffix(artifacts[0].RawContent, "...") {
		t.Error("501-char description should be truncated with '...'")
	}
}

// --- Chaos: Concurrent Sync ---

func TestChaos_ConcurrentYouTubeSync(t *testing.T) {
	c := New("chaos-concurrent")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"videos": []interface{}{
				map[string]interface{}{"video_id": "v1", "title": "Test", "published": "2026-04-08T10:00:00Z"},
			},
		},
	})

	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, errs[idx] = c.Sync(context.Background(), "")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: sync error: %v", i, err)
		}
	}
}

// --- Chaos: URL construction ---

func TestChaos_Sync_VideoURLConstruction(t *testing.T) {
	specialIDs := []string{
		"abc-_def123",  // dashes + underscores (valid)
		"AAAAAAAAAAA",  // all uppercase
		"12345678901",  // all digits
		"a&b=c<d>e\"f", // injection chars — should be preserved in URL as-is per design
	}

	for _, id := range specialIDs {
		c := New("chaos-url-" + id)
		c.Connect(context.Background(), connector.ConnectorConfig{
			AuthType: "oauth2",
			SourceConfig: map[string]interface{}{
				"videos": []interface{}{
					map[string]interface{}{"video_id": id, "title": "Test", "published": "2026-04-08T10:00:00Z"},
				},
			},
		})

		artifacts, _, err := c.Sync(context.Background(), "")
		if err != nil {
			t.Errorf("sync with id %q: %v", id, err)
			continue
		}
		if len(artifacts) != 1 {
			t.Errorf("id %q: expected 1 artifact, got %d", id, len(artifacts))
			continue
		}
		expectedURL := "https://www.youtube.com/watch?v=" + id
		if artifacts[0].URL != expectedURL {
			t.Errorf("id %q: expected URL %q, got %q", id, expectedURL, artifacts[0].URL)
		}
	}
}
