package digest

import (
	"testing"
)

func TestJoinStrings(t *testing.T) {
	result := joinStrings([]string{"a", "b", "c"}, ", ")
	if result != "a, b, c" {
		t.Errorf("expected 'a, b, c', got %q", result)
	}
}

func TestJoinStrings_Empty(t *testing.T) {
	result := joinStrings(nil, ", ")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestSplitWords(t *testing.T) {
	words := splitWords("hello world  test")
	if len(words) != 3 {
		t.Errorf("expected 3 words, got %d", len(words))
	}
}

func TestSplitWords_Empty(t *testing.T) {
	words := splitWords("")
	if len(words) != 0 {
		t.Errorf("expected 0 words, got %d", len(words))
	}
}

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

func TestJoinStrings_SingleItem(t *testing.T) {
	result := joinStrings([]string{"only"}, ", ")
	if result != "only" {
		t.Errorf("expected 'only', got %q", result)
	}
}

func TestJoinStrings_Newlines(t *testing.T) {
	result := joinStrings([]string{"line1", "line2"}, "\n")
	if result != "line1\nline2" {
		t.Errorf("expected lines joined by newline, got %q", result)
	}
}

func TestSplitWords_TabsAndNewlines(t *testing.T) {
	words := splitWords("hello\tworld\ntest")
	if len(words) != 3 {
		t.Errorf("expected 3 words, got %d: %v", len(words), words)
	}
}

func TestSplitWords_LeadingTrailingSpaces(t *testing.T) {
	words := splitWords("  hello  ")
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
		DigestDate: "2026-04-06",
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
	g := NewGenerator(nil, nil)
	if g == nil {
		t.Fatal("expected non-nil generator")
	}
	if g.Pool != nil {
		t.Error("expected nil pool")
	}
}
