package policy

import "strings"

// EvaluateAttribution checks that every provider attribution map carries the
// label and link required for honest source disclosure (BS-024). When an
// attribution is missing either the label or the link, the guard returns a
// `withhold` decision so the caller does NOT render a candidate that would
// hide its source.
func EvaluateAttribution(attributions []map[string]any) Decision {
	if len(attributions) == 0 {
		return Decision{Kind: "attribution", Outcome: "withhold", Reason: "no-attribution"}
	}
	for _, attribution := range attributions {
		label, _ := attribution["label"].(string)
		if strings.TrimSpace(label) == "" {
			return Decision{Kind: "attribution", Outcome: "withhold", Reason: "missing-attribution-label"}
		}
		urlValue, _ := attribution["url"].(string)
		if strings.TrimSpace(urlValue) == "" {
			return Decision{Kind: "attribution", Outcome: "withhold", Reason: "missing-attribution-url"}
		}
	}
	return Decision{Kind: "attribution", Outcome: "allow", Reason: "attribution-complete"}
}
