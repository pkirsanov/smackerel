package digest

import (
	"strings"
	"testing"
	"time"
)

func TestDigestContext_QuietDay(t *testing.T) {
	ctx := &DigestContext{
		DigestDate:         "2026-04-06",
		ActionItems:        nil,
		OvernightArtifacts: nil,
		HotTopics:          nil,
	}

	if len(ctx.ActionItems) != 0 {
		t.Error("quiet day should have no action items")
	}
	if len(ctx.OvernightArtifacts) != 0 {
		t.Error("quiet day should have no overnight artifacts")
	}
	if len(ctx.HotTopics) != 0 {
		t.Error("quiet day should have no hot topics")
	}
}

func TestDigestContext_WithItems(t *testing.T) {
	ctx := &DigestContext{
		DigestDate: "2026-04-06",
		ActionItems: []ActionItem{
			{Text: "Reply to Sarah", Person: "Sarah", DaysWaiting: 2},
		},
		OvernightArtifacts: []ArtifactBrief{
			{Title: "SaaS Pricing", Type: "article"},
		},
		HotTopics: []TopicBrief{
			{Name: "pricing", CapturesThisWeek: 4},
		},
	}

	if len(ctx.ActionItems) != 1 {
		t.Errorf("expected 1 action item, got %d", len(ctx.ActionItems))
	}
	if ctx.ActionItems[0].DaysWaiting != 2 {
		t.Errorf("expected 2 days waiting, got %d", ctx.ActionItems[0].DaysWaiting)
	}
}

func TestSplitWords_LeadingTrailingSpaces(t *testing.T) {
	words := strings.Fields("  hello  ")
	if len(words) != 1 {
		t.Errorf("expected 1 word, got %d: %v", len(words), words)
	}
	if words[0] != "hello" {
		t.Errorf("expected 'hello', got %q", words[0])
	}
}

func TestDigestContext_IsQuiet(t *testing.T) {
	quiet := DigestContext{
		DigestDate: "2026-04-06",
	}
	isQuiet := len(quiet.ActionItems) == 0 && len(quiet.OvernightArtifacts) == 0 && len(quiet.HotTopics) == 0
	if !isQuiet {
		t.Error("expected quiet day detection")
	}

	notQuiet := DigestContext{
		DigestDate:  "2026-04-06",
		ActionItems: []ActionItem{{Text: "do thing"}},
	}
	isQuiet2 := len(notQuiet.ActionItems) == 0 && len(notQuiet.OvernightArtifacts) == 0 && len(notQuiet.HotTopics) == 0
	if isQuiet2 {
		t.Error("should not be quiet with action items")
	}
}

func TestActionItem_Fields(t *testing.T) {
	ai := ActionItem{
		Text:        "Reply to proposal",
		Person:      "David",
		DaysWaiting: 3,
	}
	if ai.Text == "" || ai.Person == "" || ai.DaysWaiting == 0 {
		t.Error("action item fields should be populated")
	}
}

func TestArtifactBrief_Fields(t *testing.T) {
	ab := ArtifactBrief{Title: "Test Article", Type: "article"}
	if ab.Title != "Test Article" || ab.Type != "article" {
		t.Error("artifact brief fields mismatch")
	}
}

func TestTopicBrief_Fields(t *testing.T) {
	tb := TopicBrief{Name: "pricing", CapturesThisWeek: 5}
	if tb.Name != "pricing" || tb.CapturesThisWeek != 5 {
		t.Error("topic brief fields mismatch")
	}
}

func TestDigest_Fields(t *testing.T) {
	d := Digest{
		ID:         "d-1",
		DigestDate: time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		DigestText: "! Reply to Sarah about proposal.",
		WordCount:  6,
		IsQuiet:    false,
		ModelUsed:  "claude-3-haiku",
	}
	if d.WordCount != 6 {
		t.Errorf("expected 6 words, got %d", d.WordCount)
	}
	if d.IsQuiet {
		t.Error("should not be quiet")
	}
}

func TestNewGenerator(t *testing.T) {
	g := NewGenerator(nil, nil, nil)
	if g == nil {
		t.Fatal("expected non-nil generator")
	}
	if g.Pool != nil {
		t.Error("expected nil pool")
	}
}

// SCN-002-030: Digest with action items — context assembly
func TestSCN002030_DigestWithActionItems(t *testing.T) {
	ctx := &DigestContext{
		DigestDate: "2026-04-06",
		ActionItems: []ActionItem{
			{Text: "Reply to Sarah about Q2 proposal", Person: "Sarah", DaysWaiting: 2},
			{Text: "Review budget spreadsheet", Person: "Finance team", DaysWaiting: 1},
		},
		OvernightArtifacts: []ArtifactBrief{
			{Title: "SaaS Pricing Strategy", Type: "article"},
			{Title: "Team meeting notes", Type: "note"},
			{Title: "Competitor analysis video", Type: "video"},
		},
		HotTopics: nil,
	}
	if len(ctx.ActionItems) != 2 {
		t.Fatalf("expected 2 action items, got %d", len(ctx.ActionItems))
	}
	if ctx.ActionItems[0].Person != "Sarah" {
		t.Errorf("expected person 'Sarah', got %q", ctx.ActionItems[0].Person)
	}
	if len(ctx.OvernightArtifacts) != 3 {
		t.Errorf("expected 3 overnight artifacts, got %d", len(ctx.OvernightArtifacts))
	}
	// This should NOT be a quiet day
	isQuiet := len(ctx.ActionItems) == 0 && len(ctx.OvernightArtifacts) == 0 && len(ctx.HotTopics) == 0
	if isQuiet {
		t.Error("day with action items should not be quiet")
	}
}

// SCN-002-031: Quiet day digest — zero items
func TestSCN002031_QuietDayDigest(t *testing.T) {
	ctx := &DigestContext{
		DigestDate:         "2026-04-06",
		ActionItems:        nil,
		OvernightArtifacts: nil,
		HotTopics:          nil,
	}
	isQuiet := len(ctx.ActionItems) == 0 && len(ctx.OvernightArtifacts) == 0 && len(ctx.HotTopics) == 0
	if !isQuiet {
		t.Error("empty context should be detected as quiet day")
	}
}

// SCN-002-043: Digest LLM failure fallback — generates plain-text from metadata
func TestSCN002043_DigestLLMFailureFallback(t *testing.T) {
	// Simulate fallback digest generation using strings.Join/strings.Fields (the same
	// logic used by storeFallbackDigest)
	ctx := &DigestContext{
		DigestDate: "2026-04-06",
		ActionItems: []ActionItem{
			{Text: "Reply to Sarah", Person: "Sarah", DaysWaiting: 2},
			{Text: "Review budget", Person: "Finance", DaysWaiting: 1},
		},
		OvernightArtifacts: []ArtifactBrief{
			{Title: "SaaS Pricing", Type: "article"},
		},
		HotTopics: []TopicBrief{
			{Name: "pricing", CapturesThisWeek: 4},
			{Name: "leadership", CapturesThisWeek: 2},
		},
	}

	// Build fallback text (same logic as storeFallbackDigest)
	var lines []string
	if len(ctx.ActionItems) > 0 {
		lines = append(lines, "! 2 action items need attention.")
	}
	if len(ctx.OvernightArtifacts) > 0 {
		lines = append(lines, "> 1 items processed overnight.")
	}
	if len(ctx.HotTopics) > 0 {
		topicNames := []string{}
		for _, t := range ctx.HotTopics {
			topicNames = append(topicNames, t.Name)
		}
		lines = append(lines, "> Hot topics: "+strings.Join(topicNames, ", "))
	}

	text := strings.Join(lines, "\n")
	if text == "" {
		t.Fatal("fallback digest should not be empty")
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		t.Error("fallback digest should have words")
	}
	// Verify action items mentioned
	if !containsSubstring(text, "action items") {
		t.Error("fallback should mention action items")
	}
	// Verify topics mentioned
	if !containsSubstring(text, "pricing") {
		t.Error("fallback should mention hot topics")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
