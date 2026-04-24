package annotation

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedAnnotation is the output of parsing a freeform annotation string.
type ParsedAnnotation struct {
	Rating          *int
	InteractionType InteractionType
	Tags            []string
	RemovedTags     []string
	Note            string
}

var (
	ratingRe       = regexp.MustCompile(`\b([1-5])\s*/\s*5\b`)
	tagRe          = regexp.MustCompile(`#([\w-]+)`)
	removeTagRe    = regexp.MustCompile(`#remove-([\w-]+)`)
	whitespaceRe   = regexp.MustCompile(`\s+`)
	interactionMap = map[string]InteractionType{
		"made it":   InteractionMadeIt,
		"made_it":   InteractionMadeIt,
		"madeit":    InteractionMadeIt,
		"cooked it": InteractionMadeIt,
		"bought it": InteractionBoughtIt,
		"bought_it": InteractionBoughtIt,
		"purchased": InteractionBoughtIt,
		"read it":   InteractionReadIt,
		"read_it":   InteractionReadIt,
		"visited":   InteractionVisited,
		"tried it":  InteractionTriedIt,
		"tried_it":  InteractionTriedIt,
		"used it":   InteractionUsedIt,
		"used_it":   InteractionUsedIt,
	}
)

// InteractionPhrases returns the canonical human-readable interaction phrases
// recognized by the parser (e.g., "made it", "bought it"). Use this instead of
// maintaining separate phrase lists for split-point detection.
//
// The returned order is deterministic so callers (e.g. splitRateArgs) get the
// same split-point regardless of Go's randomized map iteration. The list is
// the canonical phrase per InteractionType, in a stable, hand-picked order.
func InteractionPhrases() []string {
	return []string{
		"made it",
		"bought it",
		"read it",
		"visited",
		"tried it",
		"used it",
		"purchased",
	}
}

// Parse extracts structured annotation components from a freeform string.
// Example: "4/5 made it #weeknight needs more garlic"
// → rating:4, interaction:made_it, tags:[weeknight], note:"needs more garlic"
func Parse(input string) ParsedAnnotation {
	var result ParsedAnnotation
	remaining := input

	// Extract rating (e.g., "4/5")
	if m := ratingRe.FindStringSubmatch(remaining); len(m) >= 2 {
		r, _ := strconv.Atoi(m[1])
		result.Rating = &r
		remaining = ratingRe.ReplaceAllString(remaining, "")
	}

	// Extract removed tags (e.g., "#remove-quick") — must be before regular tags
	for _, m := range removeTagRe.FindAllStringSubmatch(remaining, -1) {
		if len(m) >= 2 {
			result.RemovedTags = append(result.RemovedTags, strings.ToLower(m[1]))
		}
	}
	remaining = removeTagRe.ReplaceAllString(remaining, "")

	// Extract tags (e.g., "#weeknight")
	for _, m := range tagRe.FindAllStringSubmatch(remaining, -1) {
		if len(m) >= 2 {
			result.Tags = append(result.Tags, strings.ToLower(m[1]))
		}
	}
	remaining = tagRe.ReplaceAllString(remaining, "")

	// Extract interaction type
	lower := strings.ToLower(remaining)
	for phrase, itype := range interactionMap {
		if strings.Contains(lower, phrase) {
			result.InteractionType = itype
			// Remove the matched phrase from remaining
			idx := strings.Index(lower, phrase)
			remaining = remaining[:idx] + remaining[idx+len(phrase):]
			lower = strings.ToLower(remaining)
			break
		}
	}

	// Remaining text is the note
	note := strings.TrimSpace(remaining)
	// Clean up extra spaces
	note = whitespaceRe.ReplaceAllString(note, " ")
	if note != "" {
		result.Note = note
	}

	return result
}
