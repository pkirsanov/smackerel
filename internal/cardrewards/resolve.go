package cardrewards

import (
	"sort"
	"strings"
)

// Candidate is one ranked card-resolution match. Score is in [0,1]; higher is a
// better match. MatchType records WHY it matched (exact alias, exact name,
// substring, token overlap) for transparency (Principle 8).
type Candidate struct {
	CardID    string  `json:"card_id"`
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
}

// Match-type labels and the score each tier yields. Exact matches dominate;
// substring beats token overlap; token overlap is proportional to the Jaccard
// similarity of the word sets.
const (
	matchExactAlias   = "exact_alias"
	matchExactName    = "exact_name"
	matchSubstring    = "substring"
	matchTokenOverlap = "token_overlap"

	scoreExact     = 1.0
	scoreSubstring = 0.85

	// resolveMinScore is the floor below which a token-overlap match is
	// considered noise and dropped. A single shared token between two
	// multi-word names still clears this (B03 ambiguity), while unrelated
	// cards score 0 and are excluded.
	resolveMinScore = 0.15
)

// ResolveCard matches free-text user input against the catalog and returns
// candidates ranked best-first. It replaces CCManager's card_resolver.py
// (SequenceMatcher fuzzy matching) with deterministic, dependency-free
// alias/name/substring/token resolution — no new dependency, reproducible
// output, and explicit MatchType for each candidate.
//
//   - Exact alias or exact name match  → score 1.0 (a single dominant candidate).
//   - Substring containment (either direction) → score 0.85.
//   - Word-token Jaccard overlap → proportional score (drives B03 ambiguity).
//
// Each card contributes at most one candidate (its best-scoring match across
// name + aliases). Candidates scoring below resolveMinScore are dropped. Ties
// are broken by card id for stable ordering.
func ResolveCard(input string, catalog []CatalogCard) []Candidate {
	needle := normalizeResolve(input)
	if needle == "" {
		return nil
	}
	needleTokens := tokenSet(needle)

	var candidates []Candidate
	for _, card := range catalog {
		best := scoreCard(needle, needleTokens, card)
		if best.Score < resolveMinScore {
			continue
		}
		best.CardID = card.ID
		best.Name = card.Name
		candidates = append(candidates, best)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].CardID < candidates[j].CardID
	})
	return candidates
}

// scoreCard returns the best (highest-scoring) match between the needle and a
// single card's name + aliases.
func scoreCard(needle string, needleTokens map[string]struct{}, card CatalogCard) Candidate {
	best := Candidate{Score: 0}

	consider := func(score float64, matchType string) {
		if score > best.Score {
			best.Score = score
			best.MatchType = matchType
		}
	}

	// Exact + substring + token overlap against the display name.
	name := normalizeResolve(card.Name)
	if name == needle {
		consider(scoreExact, matchExactName)
	} else if containsEither(name, needle) {
		consider(scoreSubstring, matchSubstring)
	}
	consider(jaccard(needleTokens, tokenSet(name)), matchTokenOverlap)

	// Same against every alias. Exact alias is the strongest signal.
	for _, alias := range card.Aliases {
		a := normalizeResolve(alias)
		if a == "" {
			continue
		}
		if a == needle {
			consider(scoreExact, matchExactAlias)
			continue
		}
		if containsEither(a, needle) {
			consider(scoreSubstring, matchSubstring)
		}
		consider(jaccard(needleTokens, tokenSet(a)), matchTokenOverlap)
	}
	return best
}

// normalizeResolve lowercases, trims, and collapses internal whitespace so
// "  Citi   Custom Cash " and "citi custom cash" compare equal.
func normalizeResolve(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// containsEither reports whether a contains b or b contains a (both already
// normalized and non-empty).
func containsEither(a, b string) bool {
	return strings.Contains(a, b) || strings.Contains(b, a)
}

// tokenSet splits a normalized string into a set of word tokens.
func tokenSet(s string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, tok := range strings.Fields(s) {
		set[tok] = struct{}{}
	}
	return set
}

// jaccard returns |A∩B| / |A∪B| for two token sets (0 when either is empty).
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for tok := range a {
		if _, ok := b[tok]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
