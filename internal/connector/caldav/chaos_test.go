package caldav

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
)

// --- Chaos: Event parsing edge cases ---

func TestChaos_ParseEvents_AllFieldsEmpty(t *testing.T) {
	c := New("chaos-cal")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{}, // completely empty event
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Empty event defaults: status="confirmed", updated=time.Now()
	// Since status is "confirmed" (not "cancelled"), it should produce an artifact
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact for empty event, got %d", len(artifacts))
	}
	// Empty UID → empty SourceRef
	if artifacts[0].SourceRef != "" {
		t.Errorf("expected empty SourceRef, got %q", artifacts[0].SourceRef)
	}
	// Standard tier (no attendees, not recurring)
	if artifacts[0].Metadata["processing_tier"] != "standard" {
		t.Errorf("expected standard tier, got %v", artifacts[0].Metadata["processing_tier"])
	}
}

func TestChaos_ParseEvents_WrongTypes(t *testing.T) {
	c := New("chaos-types")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":       12345,      // int instead of string
					"summary":   true,       // bool instead of string
					"start":     12345,      // int instead of string
					"end":       nil,        // nil
					"recurring": "yes",      // string instead of bool
					"attendees": "not-list", // string instead of array
				},
			},
		},
	})

	// Should not panic
	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync should handle wrong types: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d", len(artifacts))
	}
}

func TestChaos_ParseEvents_NonArrayInput(t *testing.T) {
	c := New("chaos-nonarray")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": "not-an-array",
		},
	})

	_, _, err := c.Sync(context.Background(), "")
	if err == nil {
		t.Error("expected error when events is not an array")
	}
}

func TestChaos_ParseEvents_MixedValidInvalid(t *testing.T) {
	c := New("chaos-mixed")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				"not-a-map",
				42,
				nil,
				map[string]interface{}{
					"uid":     "valid-evt",
					"summary": "Valid Event",
					"start":   "2026-04-10T10:00:00Z",
					"updated": "2026-04-09T10:00:00Z",
				},
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
}

// --- Chaos: Status edge cases ---

func TestChaos_Sync_TentativeStatus(t *testing.T) {
	// Tentative events should NOT be skipped (only cancelled is skipped)
	c := New("chaos-tentative")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "tentative-1",
					"summary": "Maybe Meeting",
					"status":  "tentative",
					"start":   "2026-04-10T14:00:00Z",
					"updated": "2026-04-09T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("tentative events should NOT be skipped, got %d artifacts", len(artifacts))
	}
	if artifacts[0].Metadata["status"] != "tentative" {
		t.Errorf("expected tentative status in metadata, got %v", artifacts[0].Metadata["status"])
	}
}

func TestChaos_Sync_UnknownStatus(t *testing.T) {
	c := New("chaos-status")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "weird-status",
					"summary": "Weird Status Event",
					"status":  "some-unknown-status",
					"start":   "2026-04-10T10:00:00Z",
					"updated": "2026-04-09T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Unknown status should be treated like confirmed (not skipped)
	if len(artifacts) != 1 {
		t.Errorf("unknown status events should not be skipped, got %d", len(artifacts))
	}
}

// --- Chaos: Date/time edge cases ---

func TestChaos_Sync_EndBeforeStart(t *testing.T) {
	c := New("chaos-endbeforestart")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "backwards",
					"summary": "Time-reversed Event",
					"start":   "2026-04-10T14:00:00Z",
					"end":     "2026-04-10T10:00:00Z", // end before start
					"updated": "2026-04-09T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Should still produce an artifact (calendar data issue, not our fault)
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact even with end before start")
	}
}

func TestChaos_Sync_ZeroTimeStart(t *testing.T) {
	c := New("chaos-zerostart")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "zero-start",
					"summary": "Zero Start Event",
					// start not provided → zero time
					"updated": "2026-04-09T10:00:00Z",
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
	// CapturedAt is evt.Start → zero time
	if !artifacts[0].CapturedAt.IsZero() {
		t.Logf("CapturedAt for missing start: %v (may be zero time)", artifacts[0].CapturedAt)
	}
}

func TestChaos_Sync_AllDayEvent(t *testing.T) {
	// All-day events have date-only format: "2026-04-10" instead of RFC3339
	// The local parseCalendarEvents only accepts RFC3339 format
	c := New("chaos-allday")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "allday-1",
					"summary": "All Day Event",
					"start":   "2026-04-10", // date-only format
					"end":     "2026-04-11",
					"updated": "2026-04-09T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact for all-day event")
	}
	// Start time will be zero because "2026-04-10" doesn't parse as RFC3339
	// This is a known gap: local test path doesn't support date-only events
	// (The Google Calendar API path handles this properly)
	if artifacts[0].CapturedAt.IsZero() {
		t.Log("CHAOS-FINDING: date-only events (all-day) get zero CapturedAt in test/local path — only RFC3339 is supported")
	}
}

// --- Chaos: Very large attendee lists ---

func TestChaos_Sync_LargeAttendeeList(t *testing.T) {
	attendees := make([]interface{}, 500)
	for i := range attendees {
		attendees[i] = strings.Repeat("attendee", 5) + "@example.com"
	}

	c := New("chaos-bigmtg")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":       "big-meeting",
					"summary":   "All-hands Meeting",
					"start":     "2026-04-10T14:00:00Z",
					"updated":   "2026-04-09T10:00:00Z",
					"attendees": attendees,
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
	// Content includes "Attendees: " with all 500 attendees joined
	content := artifacts[0].RawContent
	if !strings.Contains(content, "Attendees:") {
		t.Error("expected attendees in content")
	}
	if artifacts[0].Metadata["attendee_count"] != 500 {
		t.Errorf("expected 500 attendees, got %v", artifacts[0].Metadata["attendee_count"])
	}
	// 500 attendees → full tier
	if artifacts[0].Metadata["processing_tier"] != "full" {
		t.Errorf("expected full tier for meeting with attendees, got %v", artifacts[0].Metadata["processing_tier"])
	}
}

// --- Chaos: Tier assignment edge ---

func TestChaos_Tier_RecurringWithAttendees(t *testing.T) {
	// Recurring events get light tier, events with attendees get full
	// Current logic: recurring check happens after attendee check, so recurring wins
	c := New("chaos-tier")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":       "recurring-mtg",
					"summary":   "Weekly 1:1",
					"start":     "2026-04-10T14:00:00Z",
					"updated":   "2026-04-09T10:00:00Z",
					"attendees": []interface{}{"manager@example.com"},
					"recurring": true,
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Recurring overrides attendee presence → light tier
	// This is debatable: a recurring 1:1 with your manager probably deserves higher tier
	tier := artifacts[0].Metadata["processing_tier"]
	if tier != "light" {
		t.Errorf("current logic: recurring override → light tier, but got %v", tier)
	}
}

// --- Chaos: Unicode content ---

func TestChaos_Sync_UnicodeEventFields(t *testing.T) {
	c := New("chaos-unicode")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":         "uni-evt",
					"summary":     "🎯 日本語ミーティング — café",
					"description": "Описание встречи with émojis 🌍 and αβγδε",
					"location":    "Büro München — 会議室 B",
					"organizer":   "müller@example.de",
					"attendees":   []interface{}{"tanaka@example.jp", "café@example.fr"},
					"start":       "2026-04-10T14:00:00Z",
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
		t.Fatal("expected 1 artifact")
	}
	if !utf8.ValidString(artifacts[0].Title) {
		t.Error("title should be valid UTF-8")
	}
	if !utf8.ValidString(artifacts[0].RawContent) {
		t.Error("content should be valid UTF-8")
	}
}

// --- Chaos: Concurrent Sync ---

func TestChaos_ConcurrentCalDAVSync(t *testing.T) {
	c := New("chaos-concurrent")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "evt-1",
					"summary": "Test",
					"start":   "2026-04-10T10:00:00Z",
					"updated": "2026-04-09T10:00:00Z",
				},
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

// --- Chaos: Cursor with updated time edge ---

func TestChaos_Sync_CursorExactMatchUpdated(t *testing.T) {
	// When cursor exactly matches event updated time, event should be skipped
	c := New("chaos-cursor-exact")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "exact",
					"summary": "Exact Match",
					"start":   "2026-04-10T10:00:00Z",
					"updated": "2026-04-09T10:00:00Z",
				},
			},
		},
	})

	artifacts, _, err := c.Sync(context.Background(), "2026-04-09T10:00:00Z")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	// Cursor condition: cursorTime <= cursor → "2026-04-09T10:00:00Z" <= "2026-04-09T10:00:00Z" → true → skip
	if len(artifacts) != 0 {
		t.Errorf("exact cursor match should skip event, got %d artifacts", len(artifacts))
	}
}

func TestChaos_Sync_DefaultUpdatedForMissingField(t *testing.T) {
	// When updated is missing, parseCalendarEvents sets Updated = time.Now()
	// But sort is by Updated, so this should still work
	c := New("chaos-no-updated")
	c.Connect(context.Background(), connector.ConnectorConfig{
		AuthType: "oauth2",
		SourceConfig: map[string]interface{}{
			"events": []interface{}{
				map[string]interface{}{
					"uid":     "no-updated",
					"summary": "Missing Updated",
					"start":   "2026-04-10T10:00:00Z",
				},
			},
		},
	})

	artifacts, cursor, err := c.Sync(context.Background(), "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatal("expected 1 artifact")
	}
	// Cursor should be set to a recent timestamp (from time.Now() default)
	if cursor == "" {
		t.Error("expected non-empty cursor")
	}
	parsed, err := time.Parse(time.RFC3339, cursor)
	if err != nil {
		t.Fatalf("cursor should be parseable RFC3339: %v", err)
	}
	if time.Since(parsed) > time.Minute {
		t.Errorf("cursor should be recent (from time.Now default), got %v", parsed)
	}
}
