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
