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

// --- DiffLabels edge cases ---

func TestDiffLabelsNilInputs(t *testing.T) {
	added, removed := DiffLabels(nil, nil)
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("nil/nil: added=%v, removed=%v — both should be empty", added, removed)
	}
}

func TestDiffLabelsEmptySlices(t *testing.T) {
	added, removed := DiffLabels([]string{}, []string{})
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("empty/empty: added=%v, removed=%v — both should be empty", added, removed)
	}
}

func TestDiffLabelsIdenticalSets(t *testing.T) {
	added, removed := DiffLabels([]string{"A", "B", "C"}, []string{"A", "B", "C"})
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("identical sets: added=%v, removed=%v — both should be empty", added, removed)
	}
}

func TestDiffLabelsAllNew(t *testing.T) {
	added, removed := DiffLabels([]string{"X", "Y"}, []string{})
	if len(added) != 2 {
		t.Errorf("all new: added=%v, want [X Y]", added)
	}
	if len(removed) != 0 {
		t.Errorf("all new: removed=%v, want empty", removed)
	}
}

func TestDiffLabelsAllRemoved(t *testing.T) {
	added, removed := DiffLabels([]string{}, []string{"X", "Y"})
	if len(added) != 0 {
		t.Errorf("all removed: added=%v, want empty", added)
	}
	if len(removed) != 2 {
		t.Errorf("all removed: removed=%v, want [X Y]", removed)
	}
}

func TestDiffLabelsDuplicates(t *testing.T) {
	// DiffLabels uses set-based comparison — duplicates in input are deduplicated via map
	added, removed := DiffLabels([]string{"A", "A", "B"}, []string{"A", "C", "C"})
	// added: B (A is in previous, B is not)
	// removed: C (C is in previous, not in current)
	hasB := false
	for _, a := range added {
		if a == "B" {
			hasB = true
		}
	}
	if !hasB {
		t.Errorf("added should contain B: %v", added)
	}
	hasC := false
	for _, r := range removed {
		if r == "C" {
			hasC = true
		}
	}
	if !hasC {
		t.Errorf("removed should contain C: %v", removed)
	}
}

// --- topicIDFromName edge cases ---

func TestTopicIDFromNameEmpty(t *testing.T) {
	id := topicIDFromName("")
	if id != "topic-" {
		t.Errorf("topicIDFromName(\"\") = %q, want \"topic-\"", id)
	}
}

func TestTopicIDFromNameSpecialChars(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Machine Learning", "topic-machine-learning"},
		{"  padded  ", "topic-padded"},
		{"UPPER CASE", "topic-upper-case"},
		{"single", "topic-single"},
	}
	for _, tt := range tests {
		got := topicIDFromName(tt.name)
		if got != tt.want {
			t.Errorf("topicIDFromName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// --- MapLabels: multiple labels in one call ---

func TestMapLabelsMultiple(t *testing.T) {
	tm := NewTopicMapper()
	labels := []string{"Recipes", "ML", "Unknown Topic", "Travel"}
	topics := []string{"Recipes", "Travel", "Machine Learning"}
	matches := tm.MapLabels(labels, topics)
	if len(matches) != 4 {
		t.Fatalf("matches = %d, want 4", len(matches))
	}

	expectedTypes := map[string]string{
		"Recipes":       "exact",
		"ML":            "abbreviation",
		"Unknown Topic": "created",
		"Travel":        "exact",
	}
	for _, m := range matches {
		if expected, ok := expectedTypes[m.LabelName]; ok {
			if m.MatchType != expected {
				t.Errorf("label %q: type = %q, want %q", m.LabelName, m.MatchType, expected)
			}
		}
	}
}

// --- fuzzyMatch: empty topics list ---

func TestFuzzyMatchEmptyTopics(t *testing.T) {
	tm := NewTopicMapper()
	best, sim := tm.fuzzyMatch("anything", []string{})
	if best != "" {
		t.Errorf("fuzzyMatch with empty topics: best = %q, want empty", best)
	}
	if sim != 0 {
		t.Errorf("fuzzyMatch with empty topics: similarity = %f, want 0", sim)
	}
}

// --- trigramSimilarity: both empty ---

func TestTrigramSimilarityBothEmpty(t *testing.T) {
	a := make(map[string]bool)
	b := make(map[string]bool)
	sim := trigramSimilarity(a, b)
	if sim != 0 {
		t.Errorf("trigramSimilarity(empty, empty) = %f, want 0", sim)
	}
}

func TestTrigramSimilarityIdentical(t *testing.T) {
	a := trigrams("hello")
	sim := trigramSimilarity(a, a)
	if sim != 1.0 {
		t.Errorf("trigramSimilarity(identical) = %f, want 1.0", sim)
	}
}
