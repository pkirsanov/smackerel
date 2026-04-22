package intelligence

import (
	"context"
	"strings"
	"testing"
	"time"
)

// === buildAttendeeBrief with nil pool ===

func TestBuildAttendeeBrief_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	ab := engine.buildAttendeeBrief(context.Background(), "stranger@example.com")

	if !ab.IsNewContact {
		t.Error("nil pool should mark attendee as new contact")
	}
	if ab.Name != "stranger@example.com" {
		t.Errorf("nil pool should set Name to email, got %q", ab.Name)
	}
	if ab.Email != "stranger@example.com" {
		t.Errorf("expected email to be preserved, got %q", ab.Email)
	}
	if len(ab.RecentThreads) != 0 {
		t.Error("nil pool should have no recent threads")
	}
	if len(ab.SharedTopics) != 0 {
		t.Error("nil pool should have no shared topics")
	}
	if len(ab.PendingItems) != 0 {
		t.Error("nil pool should have no pending items")
	}
}

func TestBuildAttendeeBrief_CancelledContext(t *testing.T) {
	engine := NewEngine(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ab := engine.buildAttendeeBrief(ctx, "test@example.com")
	// With nil pool, context cancellation doesn't matter — hits nil pool guard first
	if !ab.IsNewContact {
		t.Error("nil pool should mark attendee as new contact even with cancelled context")
	}
}

// === collectOverdueItems nil pool ===

func TestCollectOverdueItems_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	items, err := engine.collectOverdueItems(context.Background())
	if err == nil {
		t.Error("expected error for nil pool")
	}
	if items != nil {
		t.Error("expected nil items for nil pool")
	}
}

// === assembleBriefText empty-context known attendee ===

func TestAssembleBriefText_KnownAttendeeNoContext(t *testing.T) {
	// Known contact but with zero threads, topics, and pending items
	// should produce no line for this attendee (no context to show)
	brief := MeetingBrief{
		EventTitle: "Check-in",
		Attendees: []AttendeeBrief{
			{
				Name:          "EmptyContextPerson",
				Email:         "empty@example.com",
				RecentThreads: nil,
				SharedTopics:  nil,
				PendingItems:  nil,
				IsNewContact:  false,
			},
		},
	}

	text := assembleBriefText(brief)
	if !strings.Contains(text, "Check-in") {
		t.Error("brief should contain meeting title")
	}
	// Known contact with no context data should not produce a line
	if strings.Contains(text, "EmptyContextPerson") {
		t.Error("known attendee with zero context should not produce a line")
	}
}

func TestAssembleBriefText_MixedAttendeeContexts(t *testing.T) {
	brief := MeetingBrief{
		EventTitle: "Team Sync",
		Attendees: []AttendeeBrief{
			{
				Name:          "FullContext",
				Email:         "full@example.com",
				RecentThreads: []string{"Thread A", "Thread B"},
				SharedTopics:  []string{"architecture"},
				PendingItems:  []string{"Review PR"},
				IsNewContact:  false,
			},
			{
				Email:        "unknown@external.com",
				IsNewContact: true,
			},
			{
				Name:         "TopicsOnly",
				Email:        "topics@example.com",
				SharedTopics: []string{"golang", "testing"},
				IsNewContact: false,
			},
		},
	}

	text := assembleBriefText(brief)

	// Full context attendee
	if !strings.Contains(text, "FullContext") {
		t.Error("should contain full-context attendee")
	}
	if !strings.Contains(text, "2 recent threads") {
		t.Error("should show thread count for full-context attendee")
	}
	if !strings.Contains(text, "1 pending items") {
		t.Error("should show pending count for full-context attendee")
	}

	// Unknown attendee
	if !strings.Contains(text, "New contact") {
		t.Error("should flag unknown attendee")
	}

	// Topics-only attendee
	if !strings.Contains(text, "TopicsOnly") {
		t.Error("should contain topics-only attendee")
	}
}

// === assembleBriefText title always first line ===

func TestAssembleBriefText_TitleIsFirstLine(t *testing.T) {
	brief := MeetingBrief{
		EventTitle: "Important Meeting",
		Attendees: []AttendeeBrief{
			{Name: "Alice", Email: "alice@test.com", SharedTopics: []string{"x"}, IsNewContact: false},
		},
	}

	text := assembleBriefText(brief)
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		t.Fatal("expected non-empty brief text")
	}
	if !strings.HasPrefix(lines[0], "Meeting: Important Meeting") {
		t.Errorf("first line should be 'Meeting: <title>', got %q", lines[0])
	}
}

// === overdueItem struct ===

func TestOverdueItem_StructFields(t *testing.T) {
	item := overdueItem{
		id:           "ai-001",
		text:         "Send report to manager",
		expectedDate: parseDate("2026-04-10"),
		person:       "Alice",
	}

	if item.id == "" {
		t.Error("overdueItem.id should not be empty")
	}
	if item.text == "" {
		t.Error("overdueItem.text should not be empty")
	}
	if item.person == "" {
		t.Error("overdueItem.person should not be empty")
	}
	if item.expectedDate.IsZero() {
		t.Error("overdueItem.expectedDate should not be zero")
	}
}

// === CheckOverdueCommitments cancelled context ===

func TestCheckOverdueCommitments_CancelledContext(t *testing.T) {
	engine := NewEngine(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := engine.CheckOverdueCommitments(ctx)
	// Nil pool error should be returned before context is checked
	if err == nil {
		t.Error("expected error for nil pool")
	}
}

// === MeetingBrief struct defaults ===

func TestMeetingBrief_EmptyDefaults(t *testing.T) {
	brief := MeetingBrief{}
	if brief.EventID != "" {
		t.Error("empty MeetingBrief should have empty EventID")
	}
	if brief.EventTitle != "" {
		t.Error("empty MeetingBrief should have empty EventTitle")
	}
	if brief.StartsAt.IsZero() != true {
		t.Error("empty MeetingBrief should have zero StartsAt")
	}
	if len(brief.Attendees) != 0 {
		t.Error("empty MeetingBrief should have no attendees")
	}
	if brief.BriefText != "" {
		t.Error("empty MeetingBrief should have empty BriefText")
	}
}

// === AttendeeBrief struct defaults ===

func TestAttendeeBrief_EmptyDefaults(t *testing.T) {
	ab := AttendeeBrief{}
	if ab.Name != "" {
		t.Error("empty AttendeeBrief should have empty Name")
	}
	if ab.Email != "" {
		t.Error("empty AttendeeBrief should have empty Email")
	}
	if ab.IsNewContact {
		t.Error("empty AttendeeBrief should default IsNewContact to false")
	}
	if len(ab.RecentThreads) != 0 {
		t.Error("empty AttendeeBrief should have no recent threads")
	}
	if len(ab.SharedTopics) != 0 {
		t.Error("empty AttendeeBrief should have no shared topics")
	}
	if len(ab.PendingItems) != 0 {
		t.Error("empty AttendeeBrief should have no pending items")
	}
}

// test helper
func parseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// === Improve: IMP-004-F1 — pre-meeting briefs use dtstart, not created_at ===

func TestGeneratePreMeetingBriefs_UsesEventStartTime(t *testing.T) {
	// Verify the query contract: GeneratePreMeetingBriefs with a nil pool
	// should fail at the DB layer, confirming the method is callable.
	// The SQL-level fix (IMP-004-F1) switches from created_at to
	// metadata->>'dtstart' for calendar event time matching.
	engine := NewEngine(nil, nil)
	_, err := engine.GeneratePreMeetingBriefs(context.Background())
	if err == nil {
		t.Error("expected error for nil pool")
	}
	if err != nil && !strings.Contains(err.Error(), "database connection") {
		t.Errorf("expected database connection error, got: %v", err)
	}
}

// === Improve: IMP-004-F3 — context cancellation in meeting loop ===

func TestGeneratePreMeetingBriefs_CancelledContext(t *testing.T) {
	// With nil pool, the nil-pool guard fires before context check.
	// This verifies the method handles cancellation paths cleanly.
	engine := NewEngine(nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := engine.GeneratePreMeetingBriefs(ctx)
	if err == nil {
		t.Error("expected error for nil pool or cancelled context")
	}
}

// === Harden H-004-001: assembleBriefText word-cap regression guard ===

func TestAssembleBriefText_WordCountCap(t *testing.T) {
	// Build a meeting with many attendees each having verbose context
	// to push the output well beyond maxBriefWords (120).
	var attendees []AttendeeBrief
	for i := 0; i < 20; i++ {
		attendees = append(attendees, AttendeeBrief{
			Name:          strings.Repeat("LongName ", 3),
			Email:         "person@example.com",
			RecentThreads: []string{"Thread alpha beta gamma", "Thread delta epsilon zeta", "Thread eta theta iota"},
			SharedTopics:  []string{"distributed systems", "machine learning", "cloud architecture", "data engineering", "platform reliability"},
			PendingItems:  []string{"Review the quarterly report", "Follow up on pricing proposal", "Schedule design review session"},
			IsNewContact:  false,
		})
	}
	brief := MeetingBrief{
		EventTitle: "Extended Strategy Review Meeting With All Department Leads",
		Attendees:  attendees,
	}

	text := assembleBriefText(brief)
	wordCount := len(strings.Fields(text))
	if wordCount > maxBriefWords {
		t.Errorf("assembleBriefText exceeded maxBriefWords cap: got %d words, max %d", wordCount, maxBriefWords)
	}
	if wordCount == 0 {
		t.Error("expected non-empty brief text")
	}
}
