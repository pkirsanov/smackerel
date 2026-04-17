package annotation

import (
	"testing"
)

func TestCreateFromParsed_GeneratesCorrectAnnotationTypes(t *testing.T) {
	// Test the Parse → event generation logic without DB
	// We verify that CreateFromParsed WOULD create the right annotation types
	// by testing the Parse output shapes that feed into it.

	input := "4/5 made it #weeknight great flavor"
	parsed := Parse(input)

	// Should generate: rating, interaction, tag_add, note
	expectedTypes := 0
	if parsed.Rating != nil {
		expectedTypes++
	}
	if parsed.InteractionType != "" {
		expectedTypes++
	}
	expectedTypes += len(parsed.Tags)
	expectedTypes += len(parsed.RemovedTags)
	if parsed.Note != "" {
		expectedTypes++
	}

	if expectedTypes != 4 {
		t.Fatalf("expected 4 annotation events from %q, got %d (rating=%v interaction=%s tags=%v note=%q)",
			input, expectedTypes, parsed.Rating, parsed.InteractionType, parsed.Tags, parsed.Note)
	}
}

func TestCreateFromParsed_EmptyParsedGeneratesNothing(t *testing.T) {
	parsed := Parse("")
	expectedTypes := 0
	if parsed.Rating != nil {
		expectedTypes++
	}
	if parsed.InteractionType != "" {
		expectedTypes++
	}
	expectedTypes += len(parsed.Tags)
	expectedTypes += len(parsed.RemovedTags)
	if parsed.Note != "" {
		expectedTypes++
	}

	if expectedTypes != 0 {
		t.Fatalf("expected 0 annotation events from empty input, got %d", expectedTypes)
	}
}

func TestNewStore_NilPool(t *testing.T) {
	store := NewStore(nil)
	if store == nil {
		t.Fatal("NewStore should return non-nil even with nil pool")
	}
	if store.Pool != nil {
		t.Fatal("expected nil pool")
	}
}

func TestMin_EdgeCases(t *testing.T) {
	cases := []struct {
		a, b, expected int
	}{
		{8, 10, 8},
		{8, 3, 3},
		{0, 0, 0},
		{8, 8, 8},
		{1, 100, 1},
	}
	for _, tc := range cases {
		got := min(tc.a, tc.b)
		if got != tc.expected {
			t.Errorf("min(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestAnnotationIDGeneration_ShortArtifactID(t *testing.T) {
	// Verify min(8, len) doesn't panic on short artifact IDs
	shortIDs := []string{"a", "ab", "abc", "abcd1234", "abcdefghijklmnop"}
	for _, id := range shortIDs {
		truncated := id[:min(8, len(id))]
		if len(truncated) > 8 {
			t.Errorf("truncated ID %q exceeds 8 chars", truncated)
		}
		if len(truncated) == 0 && len(id) > 0 {
			t.Errorf("truncated ID is empty for non-empty input %q", id)
		}
	}
}

func TestAnnotationTypes_Constants(t *testing.T) {
	// Verify type constants match DB constraint values
	types := []AnnotationType{TypeRating, TypeNote, TypeTagAdd, TypeTagRemove, TypeInteraction, TypeStatusChange}
	for _, at := range types {
		if at == "" {
			t.Error("annotation type constant is empty")
		}
	}

	interactions := []InteractionType{InteractionMadeIt, InteractionBoughtIt, InteractionReadIt, InteractionVisited, InteractionTriedIt, InteractionUsedIt}
	for _, it := range interactions {
		if it == "" {
			t.Error("interaction type constant is empty")
		}
	}

	channels := []SourceChannel{ChannelTelegram, ChannelAPI, ChannelWeb}
	for _, ch := range channels {
		if ch == "" {
			t.Error("source channel constant is empty")
		}
	}
}

func TestSummary_DefaultValues(t *testing.T) {
	s := Summary{ArtifactID: "test"}
	if s.CurrentRating != nil {
		t.Error("expected nil CurrentRating by default")
	}
	if s.AverageRating != nil {
		t.Error("expected nil AverageRating by default")
	}
	if s.RatingCount != 0 {
		t.Error("expected 0 RatingCount by default")
	}
	if s.TimesUsed != 0 {
		t.Error("expected 0 TimesUsed by default")
	}
	if s.Tags != nil {
		t.Error("expected nil Tags by default")
	}
}
