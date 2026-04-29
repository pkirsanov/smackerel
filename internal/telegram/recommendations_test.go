package telegram

import (
	"strings"
	"testing"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

func TestFormatRecommendationCardIncludesCompactActions(t *testing.T) {
	card := FormatRecommendationCard(recstore.RenderedRequest{Recommendations: []recstore.RenderedRecommendation{{
		ID:    "rec-test",
		Title: "Tonkotsu Workshop",
		Rank:  1,
		ProviderBadges: []recstore.ProviderBadge{{
			ProviderID: "fixture_google_places",
			Label:      "Fixture Google Places",
		}},
		Rationale: []string{"Personal graph signal ART-123 supports this pick"},
	}}})

	for _, want := range []string{"[Open]", "[Why?]", "[Liked]", "[Not interested]", "Tonkotsu Workshop"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card missing %q: %s", want, card)
		}
	}
}
