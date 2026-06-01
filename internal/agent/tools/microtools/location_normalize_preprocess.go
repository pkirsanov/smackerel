// Spec 065 SCOPE-2 — colloquial-input preprocessing for
// location_normalize.
//
// The downstream geocoders (open-meteo today; others tomorrow) accept
// "City", "City, Region", "City, Country", and postal codes but they
// do NOT understand the colloquial forms users actually type:
//   "palm springs ca"   — trailing 2-letter US state abbreviation
//   "sf"                — single common city nickname
//   "nyc"               — single common city nickname
//
// NormalizeQuery folds these into geocoder-friendly input BEFORE we
// hit the provider. This logic used to live in the weather scenario's
// system prompt (~ a third of the prompt bytes). SCOPE-2 moves it
// into the tool so any scenario calling location_normalize gets the
// same canonical normalization without duplicating prompt text.
//
// The tables are intentionally small and explicit:
//
//   - US state abbreviations: only the 50 states plus DC.
//   - Common city nicknames: only the small set documented in the
//     weather prompt today ("sf", "nyc", "la", "dc", "philly", "chi").
//
// Anything else passes through unchanged. There is no fuzzy matching,
// no edit-distance, and no silent rewrite. If we don't recognize the
// input we hand it to the geocoder as the user typed it.

package microtools

import "strings"

// usStateAbbrev maps 2-letter US state abbreviations (lowercase) to
// the full state name expected by the open-meteo geocoder.
var usStateAbbrev = map[string]string{
	"al": "Alabama", "ak": "Alaska", "az": "Arizona", "ar": "Arkansas",
	"ca": "California", "co": "Colorado", "ct": "Connecticut", "de": "Delaware",
	"fl": "Florida", "ga": "Georgia", "hi": "Hawaii", "id": "Idaho",
	"il": "Illinois", "in": "Indiana", "ia": "Iowa", "ks": "Kansas",
	"ky": "Kentucky", "la": "Louisiana", "me": "Maine", "md": "Maryland",
	"ma": "Massachusetts", "mi": "Michigan", "mn": "Minnesota", "ms": "Mississippi",
	"mo": "Missouri", "mt": "Montana", "ne": "Nebraska", "nv": "Nevada",
	"nh": "New Hampshire", "nj": "New Jersey", "nm": "New Mexico", "ny": "New York",
	"nc": "North Carolina", "nd": "North Dakota", "oh": "Ohio", "ok": "Oklahoma",
	"or": "Oregon", "pa": "Pennsylvania", "ri": "Rhode Island", "sc": "South Carolina",
	"sd": "South Dakota", "tn": "Tennessee", "tx": "Texas", "ut": "Utah",
	"vt": "Vermont", "va": "Virginia", "wa": "Washington", "wv": "West Virginia",
	"wi": "Wisconsin", "wy": "Wyoming", "dc": "District of Columbia",
}

// cityNicknames maps single-token colloquial city nicknames
// (lowercase) to the geocoder-friendly canonical form.
var cityNicknames = map[string]string{
	"sf":     "San Francisco, California",
	"nyc":    "New York",
	"la":     "Los Angeles, California",
	"philly": "Philadelphia, Pennsylvania",
	"chi":    "Chicago, Illinois",
}

// NormalizeQuery applies the two preprocessing rules in order:
//
//  1. If input matches a single-token city nickname (case-insensitive,
//     ignoring outer whitespace) → return the canonical form.
//  2. If input ends in a 2-letter US state abbreviation (preceded by
//     whitespace) → expand the abbreviation to the full state name
//     and re-join with ", ". E.g. "palm springs ca" → "Palm Springs,
//     California".
//
// Anything else is returned with outer whitespace trimmed.
//
// NormalizeQuery is pure and deterministic; callers may use the
// output as a cache key.
func NormalizeQuery(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if expanded, ok := cityNicknames[lower]; ok {
		return expanded
	}
	// Trailing 2-letter US state abbreviation. Split on whitespace
	// to avoid matching mid-word "ca" sequences (e.g. "Cairo").
	fields := strings.Fields(trimmed)
	if len(fields) >= 2 {
		lastLower := strings.ToLower(fields[len(fields)-1])
		if state, ok := usStateAbbrev[lastLower]; ok {
			city := strings.Join(fields[:len(fields)-1], " ")
			return titleCaseWords(city) + ", " + state
		}
	}
	return trimmed
}

// titleCaseWords title-cases ASCII word boundaries without depending
// on the deprecated strings.Title. We only handle ASCII here because
// city names that need preserved non-ASCII casing (e.g. "Reykjavík")
// are not in the abbreviation table — they pass through untouched.
func titleCaseWords(s string) string {
	if s == "" {
		return s
	}
	parts := strings.Fields(s)
	for i, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(strings.ToLower(p))
		runes[0] = upperASCII(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func upperASCII(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - ('a' - 'A')
	}
	return r
}
