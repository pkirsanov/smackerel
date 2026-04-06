package youtube
package youtube

import (
	"context"
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
