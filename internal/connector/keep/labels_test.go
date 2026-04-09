package keep

import (
	"testing"
)

func TestExactLabelMatch(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"Recipes"}, []string{"Recipes", "Travel"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "exact" {
		t.Errorf("match type = %q, want exact", matches[0].MatchType)
	}
	if matches[0].TopicName != "Recipes" {
		t.Errorf("topic name = %q, want Recipes", matches[0].TopicName)
	}
}

func TestExactMatchCaseInsensitive(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"recipes"}, []string{"Recipes"})
	if len(matches) != 1 || matches[0].MatchType != "exact" {
		t.Errorf("expected case-insensitive exact match")
	}
}

func TestAbbreviationMatch(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"ML"}, []string{"Machine Learning", "Data Science"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "abbreviation" {
		t.Errorf("match type = %q, want abbreviation", matches[0].MatchType)
	}
	if matches[0].TopicName != "Machine Learning" {
		t.Errorf("topic name = %q, want Machine Learning", matches[0].TopicName)
	}
}

func TestAbbreviationBidirectional(t *testing.T) {
	tm := NewTopicMapper()
	// Label is full name, topic stored as abbreviation
	matches := tm.MapLabels([]string{"Machine Learning"}, []string{"ML", "Data Science"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "abbreviation" {
		t.Errorf("match type = %q, want abbreviation", matches[0].MatchType)
	}
}

func TestFuzzyMatch(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"Machine Learn"}, []string{"Machine Learning"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "fuzzy" {
		t.Errorf("match type = %q, want fuzzy", matches[0].MatchType)
	}
}

func TestFuzzyMatchBelowThreshold(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"xyz"}, []string{"Machine Learning"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "created" {
		t.Errorf("match type = %q, want created (no fuzzy match)", matches[0].MatchType)
	}
}

func TestCreateNewTopic(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"Birdwatching"}, []string{})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	if matches[0].MatchType != "created" {
		t.Errorf("match type = %q, want created", matches[0].MatchType)
	}
	if matches[0].TopicName != "Birdwatching" {
		t.Errorf("topic name = %q, want Birdwatching", matches[0].TopicName)
	}
}

func TestEmptyLabelSkipped(t *testing.T) {
	tm := NewTopicMapper()
	matches := tm.MapLabels([]string{"", "  ", "Valid"}, []string{})
	if len(matches) != 1 {
		t.Errorf("matches = %d, want 1 (empty labels skipped)", len(matches))
	}
}

func TestDiffLabels(t *testing.T) {
	added, removed := DiffLabels(
		[]string{"Work", "Travel", "New"},
		[]string{"Work", "Travel", "Old"},
	)
	if len(added) != 1 || added[0] != "New" {
		t.Errorf("added = %v, want [New]", added)
	}
	if len(removed) != 1 || removed[0] != "Old" {
		t.Errorf("removed = %v, want [Old]", removed)
	}
}

func TestTopicEdgeIdempotent(t *testing.T) {
	// Verify topicIDFromName produces consistent IDs
	id1 := topicIDFromName("Machine Learning")
	id2 := topicIDFromName("Machine Learning")
	if id1 != id2 {
		t.Errorf("topic IDs not consistent: %q vs %q", id1, id2)
	}
}

func TestUnicodeFuzzyMatch(t *testing.T) {
	tm := NewTopicMapper()

	// Accented characters — "café" should fuzzy-match "cafe" reasonably
	matches := tm.MapLabels([]string{"café"}, []string{"cafe latte"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	// Should produce a match (fuzzy or created), not panic
	if matches[0].LabelName != "café" {
		t.Errorf("label = %q, want café", matches[0].LabelName)
	}

	// CJK characters should not panic and produce valid trigrams
	matches = tm.MapLabels([]string{"机器学习"}, []string{"Machine Learning"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}

	// Emoji label should not panic
	matches = tm.MapLabels([]string{"🚀 Ideas"}, []string{"Rocket Ideas"})
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
}

func TestTrigramUnicodeSafety(t *testing.T) {
	// Verify trigrams function produces valid strings for multibyte input
	result := trigrams("café")
	if len(result) == 0 {
		t.Error("trigrams should produce entries for Unicode input")
	}
	// Each trigram should be exactly 3 runes
	for tri := range result {
		runes := []rune(tri)
		if len(runes) != 3 {
			t.Errorf("trigram %q has %d runes, want 3", tri, len(runes))
		}
	}
}
