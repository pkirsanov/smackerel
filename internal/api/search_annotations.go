package api

import (
	"regexp"
	"strings"
)

// AnnotationIntent represents detected annotation-related intent in a search query.
type AnnotationIntent struct {
	MinRating      int
	HasInteraction bool
	Tag            string
	Cleaned        string
}

var (
	topRatedRe       = regexp.MustCompile(`(?i)\b(my\s+)?(top\s+rated|best\s+rated|highest\s+rated|best)\b`)
	interactionRe    = regexp.MustCompile(`(?i)\b(things?\s+I'?v?e?\s*(made|cooked|tried|bought|read|visited|used)|my\s+(made|cooked|tried|used)\s+\w+)\b`)
	tagIntentRe      = regexp.MustCompile(`#([\w-]+)`)
	whitespaceNormRe = regexp.MustCompile(`\s+`)
)

// parseAnnotationIntent detects annotation-related intent in a search query.
// Returns nil if no annotation intent is detected (plain query).
func parseAnnotationIntent(query string) *AnnotationIntent {
	var intent AnnotationIntent
	remaining := query
	found := false

	// Check for "top rated" / "best" patterns → min_rating=4
	if topRatedRe.MatchString(remaining) {
		intent.MinRating = 4
		remaining = topRatedRe.ReplaceAllString(remaining, "")
		found = true
	}

	// Check for interaction phrases → has_interaction=true
	if interactionRe.MatchString(remaining) {
		intent.HasInteraction = true
		remaining = interactionRe.ReplaceAllString(remaining, "")
		found = true
	}

	// Check for hashtag in query → tag filter
	if m := tagIntentRe.FindStringSubmatch(remaining); len(m) >= 2 {
		intent.Tag = strings.ToLower(m[1])
		remaining = tagIntentRe.ReplaceAllString(remaining, "")
		found = true
	}

	if !found {
		return nil
	}

	// Clean up remaining text
	intent.Cleaned = strings.TrimSpace(whitespaceNormRe.ReplaceAllString(remaining, " "))
	return &intent
}

// applyAnnotationBoost adjusts a similarity score based on annotation data.
// Rating boost: max 0.05 (for rating 5), scaled linearly from 0 for rating 1.
// Usage boost: max 0.03 (capped at 10 uses).
// Total max boost: 0.08.
func applyAnnotationBoost(similarity float64, rating *int, timesUsed *int) float64 {
	var boost float64

	// Rating boost: (rating - 1) / 4 * 0.05 → 0.0 for rating 1, 0.05 for rating 5
	if rating != nil && *rating >= 1 && *rating <= 5 {
		boost += float64(*rating-1) / 4.0 * 0.05
	}

	// Usage boost: min(times_used, 10) / 10 * 0.03 → capped at 0.03
	if timesUsed != nil && *timesUsed > 0 {
		uses := *timesUsed
		if uses > 10 {
			uses = 10
		}
		boost += float64(uses) / 10.0 * 0.03
	}

	// Cap total boost at 0.08
	if boost > 0.08 {
		boost = 0.08
	}

	return similarity + boost
}
