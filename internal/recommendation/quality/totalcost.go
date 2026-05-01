package quality

// TotalCostFacts is the bounded subset of canonical_fact fields the total-cost
// guard inspects. All optional fields default to their zero value when the
// provider has not surfaced them.
type TotalCostFacts struct {
	HeadlinePrice     float64
	HeadlinePriceSet  bool
	ShippingCost      float64
	ShippingKnown     bool
	ReturnPolicy      string
	ReturnPolicyKnown bool
	TaxesIncluded     bool
	TotalCost         float64
	TotalCostKnown    bool
	CheapestClaimed   bool
}

// TotalCostFactsFromMap reads a candidate canonical_fact map and produces a
// strongly-typed TotalCostFacts value. Missing keys produce known=false.
func TotalCostFactsFromMap(canonicalFact map[string]any) TotalCostFacts {
	facts := TotalCostFacts{}
	if canonicalFact == nil {
		return facts
	}
	if value, ok := numericFromAny(canonicalFact["headline_price"]); ok {
		facts.HeadlinePrice = value
		facts.HeadlinePriceSet = true
	}
	if value, ok := numericFromAny(canonicalFact["shipping_cost"]); ok {
		facts.ShippingCost = value
		facts.ShippingKnown = true
	} else if known, set := canonicalFact["shipping_known"].(bool); set {
		facts.ShippingKnown = known
	}
	if value, ok := canonicalFact["return_policy"].(string); ok && value != "" {
		facts.ReturnPolicy = value
		facts.ReturnPolicyKnown = true
	} else if known, set := canonicalFact["return_policy_known"].(bool); set {
		facts.ReturnPolicyKnown = known
	}
	if included, set := canonicalFact["taxes_included"].(bool); set {
		facts.TaxesIncluded = included
	}
	if value, ok := numericFromAny(canonicalFact["total_cost"]); ok {
		facts.TotalCost = value
		facts.TotalCostKnown = true
	}
	if claimed, set := canonicalFact["cheapest_claimed"].(bool); set {
		facts.CheapestClaimed = claimed
	}
	return facts
}

// EvaluateTotalCost returns the quality decisions that disclose unknown or
// unfavorable total-cost facts (SCN-039-044 / BS-031). When the candidate's
// total cost cannot be proven less-than-or-equal to the headline price, the
// guard emits a `block_label_cheapest` decision so the renderer must NOT call
// it cheapest. The guard is purely descriptive: it never withholds the
// candidate, it only labels the unknowns.
func EvaluateTotalCost(facts TotalCostFacts) []map[string]any {
	decisions := []map[string]any{}
	if !facts.ShippingKnown {
		decisions = append(decisions, map[string]any{
			"kind":    "total_cost_transparency",
			"outcome": "disclose_unknown",
			"reason":  "shipping-cost-unknown",
		})
	}
	if !facts.ReturnPolicyKnown {
		decisions = append(decisions, map[string]any{
			"kind":    "total_cost_transparency",
			"outcome": "disclose_unknown",
			"reason":  "return-policy-unknown",
		})
	}
	if !facts.TaxesIncluded {
		decisions = append(decisions, map[string]any{
			"kind":    "total_cost_transparency",
			"outcome": "disclose_unknown",
			"reason":  "taxes-not-included",
		})
	}
	if facts.CheapestClaimed && !cheapestSupported(facts) {
		decisions = append(decisions, map[string]any{
			"kind":    "total_cost_transparency",
			"outcome": "block_label_cheapest",
			"reason":  "total-cost-not-supported",
		})
	}
	return decisions
}

// cheapestSupported reports whether the candidate's known total cost is
// low enough to support a "cheapest" label.
func cheapestSupported(facts TotalCostFacts) bool {
	if !facts.TotalCostKnown {
		return false
	}
	if !facts.HeadlinePriceSet {
		return false
	}
	if !facts.ShippingKnown {
		return false
	}
	if !facts.TaxesIncluded {
		return false
	}
	return facts.TotalCost <= facts.HeadlinePrice+facts.ShippingCost
}

func numericFromAny(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	default:
		return 0, false
	}
}
