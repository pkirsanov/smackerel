package caldav

import (
	"context"
	"testing"

	"github.com/smackerel/smackerel/internal/connector"
)

func TestConnector_Interface(t *testing.T) {
	var _ connector.Connector = New("test-caldav")
}

func TestConnector_Connect(t *testing.T) {
	c := New("google-calendar")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Error("expected healthy")
	}
}

func TestConnector_RequiresOAuth2(t *testing.T) {
	c := New("test")
	err := c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "api_key"})
	if err == nil {
		t.Error("expected error for non-oauth2 auth")
	}
}

func TestSync_WithEvents(t *testing.T) {
	c := New("google-cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":         "evt-001",
					"summary":     "Team Standup",
					"description": "Daily standup meeting",
					"start":       "2026-04-08T09:00:00Z",
					"end":         "2026-04-08T09:30:00Z",
					"organizer":   "lead@example.com",
					"attendees":   []interface{}{"alice@example.com", "bob@example.com"},
					"recurring":   false,
					"updated":     "2026-04-07T20:00:00Z",
				},
				map[string]interface{}{
					"uid":       "evt-002",
					"summary":   "Lunch Break",
					"start":     "2026-04-08T12:00:00Z",
					"end":       "2026-04-08T13:00:00Z",
					"recurring": true,
					"updated":   "2026-04-07T21:00:00Z",
				},
			},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}

	// Team standup with attendees should get full tier
	standup := artifacts[0]
	if standup.Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier for meeting with attendees, got %v", standup.Metadata["processing_tier"])
	}
	if standup.Metadata["attendee_count"] != 2 {
		t.Errorf("expected 2 attendees, got %v", standup.Metadata["attendee_count"])
	}

	// Recurring lunch gets light tier
	lunch := artifacts[1]
	if lunch.Metadata["processing_tier"] != "light" {
		t.Errorf("expected light tier for recurring event, got %v", lunch.Metadata["processing_tier"])
	}

	if cursor == "" {
		t.Error("expected non-empty cursor after sync")
	}
}

func TestSync_SkipsCancelled(t *testing.T) {
	c := New("google-cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "evt-cancel",
					"summary": "Cancelled Meeting",
					"status":  "cancelled",
					"start":   "2026-04-08T14:00:00Z",
					"updated": "2026-04-07T22:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("expected 0 artifacts for cancelled event, got %d", len(artifacts))
	}
}

func TestSync_CursorAdvancement(t *testing.T) {
	c := New("google-cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{"uid": "old", "summary": "Old", "start": "2026-04-01T10:00:00Z", "updated": "2026-04-01T10:00:00Z"},
				map[string]interface{}{"uid": "new", "summary": "New", "start": "2026-04-08T10:00:00Z", "updated": "2026-04-08T10:00:00Z"},
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
	if artifacts[0].Title != "New" {
		t.Errorf("expected 'New', got %q", artifacts[0].Title)
	}
}

func TestSync_Empty(t *testing.T) {
	c := New("google-cal")
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
