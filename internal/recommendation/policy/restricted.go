package policy

import "strings"

// RestrictedFlagsCategoryKey is the canonical key under which providers
// surface the restricted-category label for a candidate.
const RestrictedFlagsCategoryKey = "restricted_category"

// EvaluateRestricted inspects the candidate's restricted_flags map against
// the user's restricted-category list and returns a decision. When the
// candidate's category is restricted, the returned decision is a `withhold`
// outcome whose reason includes the offending category — that lets the
// renderer surface a category-level "withheld" reason without leaking the
// candidate detail. The empty list is allowed: an empty list means no
// categories are restricted, which is itself a valid SST state.
func EvaluateRestricted(restrictedFlags map[string]any, restrictedCategories []string) Decision {
	categoryRaw := restrictedFlags[RestrictedFlagsCategoryKey]
	category, _ := categoryRaw.(string)
	category = strings.ToLower(strings.TrimSpace(category))
	if category == "" {
		return Decision{Kind: "restricted_category", Outcome: "allow", Reason: "no-restricted-category"}
	}
	for _, restricted := range restrictedCategories {
		if strings.EqualFold(strings.TrimSpace(restricted), category) {
			return Decision{
				Kind:    "restricted_category",
				Outcome: "withhold",
				Reason:  "restricted:" + category,
			}
		}
	}
	return Decision{Kind: "restricted_category", Outcome: "allow", Reason: "category-allowed"}
}
