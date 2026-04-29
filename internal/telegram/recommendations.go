package telegram

import (
	"fmt"
	"strings"

	recstore "github.com/smackerel/smackerel/internal/recommendation/store"
)

// FormatRecommendationCard renders a compact reactive recommendation card.
func FormatRecommendationCard(request recstore.RenderedRequest) string {
	if len(request.Recommendations) == 0 {
		return MarkerUncertain + "No recommendation is ready."
	}
	var lines []string
	lines = append(lines, MarkerHeading+"Recommendations")
	limit := len(request.Recommendations)
	if limit > 3 {
		limit = 3
	}
	for _, rec := range request.Recommendations[:limit] {
		badges := make([]string, 0, len(rec.ProviderBadges))
		for _, badge := range rec.ProviderBadges {
			badges = append(badges, badge.Label)
		}
		if len(badges) == 0 {
			badges = append(badges, "source")
		}
		line := fmt.Sprintf("%s%d. %s (%s)", MarkerListItem, rec.Rank, rec.Title, strings.Join(badges, ", "))
		if len(rec.Rationale) > 0 {
			line += " - " + rec.Rationale[0]
		}
		lines = append(lines, line)
	}
	lines = append(lines, MarkerAction+"[Open] [Why?] [Liked] [Not interested]")
	return strings.Join(lines, "\n")
}
