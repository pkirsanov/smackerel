package quality

import (
	"sort"
	"strings"
)

// ChainKeyOf returns a stable chain identifier for grouping near-duplicate
// candidates from the same brand or chain. Resolution order:
//  1. canonicalFact["chain_id"] (string) — providers may set this directly.
//  2. canonicalFact["chain_name"] (string) — normalized to lower-case slug.
//  3. The first whitespace-separated token of the title — used as a fallback
//     so providers that have not yet wired chain_id still get reasonable
//     grouping for things like "Starbucks Mission" / "Starbucks Castro".
//
// Returns the empty string when no chain identifier can be inferred. An empty
// chain key MUST NOT cause a candidate to be grouped with other empty-key
// candidates — they are unique by definition.
func ChainKeyOf(canonicalFact map[string]any, title string) string {
	if canonicalFact != nil {
		if value, ok := canonicalFact["chain_id"].(string); ok {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				return value
			}
		}
		if value, ok := canonicalFact["chain_name"].(string); ok {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				return slug(value)
			}
		}
	}
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ""
	}
	first := strings.Fields(trimmed)[0]
	return slug(first)
}

// Variant captures a near-duplicate candidate that was grouped under another
// candidate by the diversity guard.
type Variant struct {
	CandidateLocalID string
	CanonicalKey     string
	Title            string
}

// DiversityResult summarises the diversity grouping outcome.
type DiversityResult struct {
	// KeptOrder lists the candidate local IDs that survived grouping, in the
	// same relative order as the input (highest-ranked first).
	KeptOrder []string
	// VariantsByParent maps each kept candidate's local ID to the list of
	// near-duplicate variants that were grouped under it.
	VariantsByParent map[string][]Variant
	// ParentByVariant maps each grouped variant local ID back to the kept
	// parent candidate's local ID. Useful for persisting withheld rows that
	// reference their parent.
	ParentByVariant map[string]string
}

// CandidateForDiversity is the diversity-guard input shape. Callers wrap
// their richer candidate type in this struct to keep the guard provider-free.
type CandidateForDiversity struct {
	LocalID      string
	CanonicalKey string
	Title        string
	ChainKey     string
}

// GroupNearDuplicates collapses near-duplicate same-chain candidates so the
// top-K list contains at most one branch per chain. When more than one
// candidate shares a non-empty chain key, the highest-ranked candidate is
// kept and the rest are recorded as variants on the parent.
//
// Empty chain keys are NEVER grouped — a candidate without an inferrable
// chain identifier is always unique.
func GroupNearDuplicates(rankedCandidates []CandidateForDiversity) DiversityResult {
	result := DiversityResult{
		VariantsByParent: map[string][]Variant{},
		ParentByVariant:  map[string]string{},
	}
	parentForChain := map[string]string{}
	for _, candidate := range rankedCandidates {
		chain := candidate.ChainKey
		if chain == "" {
			result.KeptOrder = append(result.KeptOrder, candidate.LocalID)
			continue
		}
		if parent, seen := parentForChain[chain]; seen {
			result.VariantsByParent[parent] = append(result.VariantsByParent[parent], Variant{
				CandidateLocalID: candidate.LocalID,
				CanonicalKey:     candidate.CanonicalKey,
				Title:            candidate.Title,
			})
			result.ParentByVariant[candidate.LocalID] = parent
			continue
		}
		parentForChain[chain] = candidate.LocalID
		result.KeptOrder = append(result.KeptOrder, candidate.LocalID)
	}
	for parent := range result.VariantsByParent {
		sort.SliceStable(result.VariantsByParent[parent], func(i, j int) bool {
			return result.VariantsByParent[parent][i].Title < result.VariantsByParent[parent][j].Title
		})
	}
	return result
}

// VariantsDecision returns a quality decision row encoding the variant group
// for one parent candidate. The decision is stable regardless of map iteration
// order, so persisted JSON is reproducible.
func VariantsDecision(variants []Variant) map[string]any {
	keys := make([]string, 0, len(variants))
	titles := make([]string, 0, len(variants))
	for _, variant := range variants {
		keys = append(keys, variant.CanonicalKey)
		titles = append(titles, variant.Title)
	}
	return map[string]any{
		"kind":           "diversity",
		"outcome":        "variants_grouped",
		"reason":         "same-chain",
		"variant_count":  len(variants),
		"variant_keys":   keys,
		"variant_titles": titles,
	}
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "-", "'", "", "&", "and", "/", "-").Replace(value)
	return value
}
