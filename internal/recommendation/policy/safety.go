package policy

// SafetyRecallKey is the boolean key on restricted_flags that providers set
// when a candidate carries a recall, hazard, or other safety advisory.
const SafetyRecallKey = "recall"

// SafetyAdvisoryKey is an additional safety boolean (e.g. health, hazard).
const SafetyAdvisoryKey = "safety_advisory"

// EvaluateSafety returns a `withhold` decision with reason
// "withheld:safety-policy" whenever any safety flag is set on the candidate.
// This guard runs after restricted-category but before delivery, so a recalled
// product never ships an ordinary deal alert (SCN-039-042 / BS-026).
func EvaluateSafety(restrictedFlags map[string]any) Decision {
	if boolFlag(restrictedFlags, SafetyRecallKey) {
		return Decision{Kind: "safety", Outcome: "withhold", Reason: "withheld:safety-policy"}
	}
	if boolFlag(restrictedFlags, SafetyAdvisoryKey) {
		return Decision{Kind: "safety", Outcome: "withhold", Reason: "withheld:safety-policy"}
	}
	return Decision{Kind: "safety", Outcome: "allow", Reason: "no-safety-advisory"}
}

func boolFlag(flags map[string]any, key string) bool {
	value, ok := flags[key]
	if !ok {
		return false
	}
	if typed, ok := value.(bool); ok {
		return typed
	}
	return false
}
