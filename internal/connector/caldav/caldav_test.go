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

func TestNew_Defaults(t *testing.T) {
	c := New("cal-1")
	if c.ID() != "cal-1" {
		t.Errorf("expected ID 'cal-1', got %q", c.ID())
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected before connect, got %v", c.Health(context.Background()))
	}
}

func TestClose_SetsDisconnected(t *testing.T) {
	c := New("test")
	c.Connect(context.Background(), connector.ConnectorConfig{AuthType: "oauth2"})
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Fatal("expected healthy after connect")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("expected disconnected after close, got %v", c.Health(context.Background()))
	}
}

func TestSync_ContentBuilding(t *testing.T) {
	c := New("cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":         "evt-full",
					"summary":     "Design Review",
					"description": "Review the new design system",
					"location":    "Conference Room B",
					"organizer":   "lead@example.com",
					"attendees":   []interface{}{"dev1@example.com"},
					"start":       "2026-04-10T14:00:00Z",
					"end":         "2026-04-10T15:00:00Z",
					"updated":     "2026-04-09T10:00:00Z",
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

	a := artifacts[0]
	content := a.RawContent
	if !containsSubstr(content, "Design Review") {
		t.Error("content missing summary")
	}
	if !containsSubstr(content, "Review the new design system") {
		t.Error("content missing description")
	}
	if !containsSubstr(content, "Location: Conference Room B") {
		t.Error("content missing location")
	}
	if !containsSubstr(content, "Attendees: dev1@example.com") {
		t.Error("content missing attendees")
	}

	// Metadata verification
	if a.Metadata["organizer"] != "lead@example.com" {
		t.Errorf("unexpected organizer: %v", a.Metadata["organizer"])
	}
	if a.Metadata["location"] != "Conference Room B" {
		t.Errorf("unexpected location: %v", a.Metadata["location"])
	}
	if a.ContentType != "event" {
		t.Errorf("expected content type 'event', got %q", a.ContentType)
	}
	if a.SourceRef != "evt-full" {
		t.Errorf("expected source ref 'evt-full', got %q", a.SourceRef)
	}
}

func TestSync_StandardTier_NoAttendees(t *testing.T) {
	c := New("cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "evt-solo",
					"summary": "Focus Time",
					"start":   "2026-04-10T10:00:00Z",
					"updated": "2026-04-09T08:00:00Z",
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
	if artifacts[0].Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier for solo event, got %v", artifacts[0].Metadata["processing_tier"])
	}
}

func TestParseCalendarEvents_InvalidInput(t *testing.T) {
	_, err := parseCalendarEvents("not-an-array")
	if err == nil {
		t.Error("expected error for non-array input")
	}
}

func TestParseCalendarEvents_SkipsNonMapEntries(t *testing.T) {
	events, err := parseCalendarEvents([]interface{}{
		"not-a-map",
		42,
		map[string]interface{}{
			"uid":     "valid",
			"summary": "Valid Event",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 valid event, got %d", len(events))
	}
	if events[0].UID != "valid" {
		t.Errorf("expected UID 'valid', got %q", events[0].UID)
	}
}

func TestParseCalendarEvents_AllFields(t *testing.T) {
	events, err := parseCalendarEvents([]interface{}{
		map[string]interface{}{
			"uid":         "e1",
			"summary":     "Full Event",
			"description": "Desc",
			"location":    "Room A",
			"organizer":   "org@test.com",
			"status":      "tentative",
			"start":       "2026-04-10T09:00:00Z",
			"end":         "2026-04-10T10:00:00Z",
			"updated":     "2026-04-09T12:00:00Z",
			"recurring":   true,
			"attendees":   []interface{}{"a@b.com", "c@d.com"},
		},
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Description != "Desc" {
		t.Errorf("description: %q", e.Description)
	}
	if e.Location != "Room A" {
		t.Errorf("location: %q", e.Location)
	}
	if e.Organizer != "org@test.com" {
		t.Errorf("organizer: %q", e.Organizer)
	}
	if e.Status != "tentative" {
		t.Errorf("status: %q", e.Status)
	}
	if !e.Recurring {
		t.Error("expected recurring=true")
	}
	if len(e.Attendees) != 2 {
		t.Errorf("expected 2 attendees, got %d", len(e.Attendees))
	}
	if e.Start.IsZero() || e.End.IsZero() || e.Updated.IsZero() {
		t.Error("expected non-zero times")
	}
}

func TestGetCredential_NilMap(t *testing.T) {
	if v := getCredential(nil, "key"); v != "" {
		t.Errorf("expected empty string for nil map, got %q", v)
	}
}

func TestGetCredential_MissingKey(t *testing.T) {
	creds := map[string]string{"a": "b"}
	if v := getCredential(creds, "missing"); v != "" {
		t.Errorf("expected empty string for missing key, got %q", v)
	}
}

func TestGetCredential_Found(t *testing.T) {
	creds := map[string]string{"token": "abc123"}
	if v := getCredential(creds, "token"); v != "abc123" {
		t.Errorf("expected 'abc123', got %q", v)
	}
}

func TestGetStr_MissingKey(t *testing.T) {
	m := map[string]interface{}{"a": "b"}
	if v := getStr(m, "missing"); v != "" {
		t.Errorf("expected empty string, got %q", v)
	}
}

func TestGetStr_NonStringValue(t *testing.T) {
	m := map[string]interface{}{"num": 42}
	if v := getStr(m, "num"); v != "" {
		t.Errorf("expected empty string for non-string, got %q", v)
	}
}

func TestSync_HealthTransitions(t *testing.T) {
	c := New("cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{},
		},
	})
	// After successful sync, health should be healthy
	_, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if c.Health(context.Background()) != connector.HealthHealthy {
		t.Errorf("expected healthy after sync, got %v", c.Health(context.Background()))
	}
}

func TestSync_ErrorOnInvalidEvents(t *testing.T) {
	c := New("cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": "not-an-array",
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error for invalid events format")
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// Regression: recurring events WITH attendees must keep "full" tier.
// Previously, recurring unconditionally overrode attendee-based tier to "light".
func TestSync_RecurringWithAttendees_FullTier(t *testing.T) {
	c := New("cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":       "evt-weekly-1on1",
					"summary":   "Weekly 1:1 with Sarah",
					"attendees": []interface{}{"sarah@company.com"},
					"recurring": true,
					"start":     "2026-04-10T10:00:00Z",
					"updated":   "2026-04-09T08:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	tier := artifacts[0].Metadata["processing_tier"]
	if tier != "full" {
		t.Errorf("recurring event with attendees should get full tier, got %v", tier)
	}
}

// Regression: recurring events WITHOUT attendees should get "light" tier.
func TestSync_RecurringNoAttendees_LightTier(t *testing.T) {
	c := New("cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":       "evt-daily-block",
					"summary":   "Focus Time",
					"recurring": true,
					"start":     "2026-04-10T14:00:00Z",
					"updated":   "2026-04-09T12:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync error: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
	tier := artifacts[0].Metadata["processing_tier"]
	if tier != "light" {
		t.Errorf("recurring event without attendees should get light tier, got %v", tier)
	}
}

func TestSync_HealthStaysErrorOnFailure(t *testing.T) {
	c := New("cal-err")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": "not-an-array", // triggers parse error
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Fatal("expected sync error for invalid events")
	}

	// Health must remain at Error, not reset to Healthy by the defer
	if c.Health(context.Background()) != connector.HealthError {
		t.Errorf("health should be Error after failed sync, got %v", c.Health(context.Background()))
	}
}

func TestSync_BeforeConnect(t *testing.T) {
	c := New("cal-no-connect")
	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when Sync called before Connect")
	}
	if c.Health(context.Background()) != connector.HealthDisconnected {
		t.Errorf("health should remain disconnected, got %v", c.Health(context.Background()))
	}
}

func TestParseCalendarEvents_SkipsEmptyUID(t *testing.T) {
	events, err := parseCalendarEvents([]interface{}{
		map[string]interface{}{"uid": "", "summary": "No UID Event"},
		map[string]interface{}{"uid": "valid-uid", "summary": "Valid Event"},
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event (empty UID skipped), got %d", len(events))
	}
	if events[0].UID != "valid-uid" {
		t.Errorf("expected UID 'valid-uid', got %q", events[0].UID)
	}
}
