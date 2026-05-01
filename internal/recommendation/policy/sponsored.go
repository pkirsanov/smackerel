package policy

import "strings"

// SponsoredKinds enumerates provider sponsored_state values that mark a
// candidate as commercially promoted in any way.
var SponsoredKinds = map[string]struct{}{
	"sponsored": {},
	"promoted":  {},
	"affiliate": {},
	"paid":      {},
}

// SponsoredOptions captures the runtime opt-in surface for sponsored ranking.
// PromotionsEnabled is the SST switch (config.Policy.SponsoredPromotionsEnabled).
// QueryOptIn / WatchOptIn flip true when the request explicitly asked for
// commercial promotions (e.g. an `allow_sponsored=true` flag on a query or a
// watch consent flag).
type SponsoredOptions struct {
	PromotionsEnabled bool
	QueryOptIn        bool
	WatchOptIn        bool
}

// IsSponsored reports whether sponsoredState marks a commercially-promoted
// candidate. Empty / "none" / "organic" all return false.
func IsSponsored(sponsoredState string) bool {
	value := strings.ToLower(strings.TrimSpace(sponsoredState))
	if value == "" || value == "none" || value == "organic" {
		return false
	}
	_, ok := SponsoredKinds[value]
	return ok
}

// SponsoredBoostAllowed reports whether ranking may apply a positive boost
// to a sponsored candidate. The default is false: sponsored cannot improve
// rank unless BOTH the SST switch AND an explicit per-call opt-in are true.
func SponsoredBoostAllowed(opts SponsoredOptions) bool {
	if !opts.PromotionsEnabled {
		return false
	}
	return opts.QueryOptIn || opts.WatchOptIn
}

// EvaluateSponsored returns the policy decisions for a candidate's sponsored
// state. The decisions are append-only and never mutate ranking on their own;
// they record the labeling and, when the candidate is sponsored, the
// `boost_blocked` reason that audits why no rank boost was applied.
func EvaluateSponsored(sponsoredState string, opts SponsoredOptions) []Decision {
	if !IsSponsored(sponsoredState) {
		return []Decision{{Kind: "sponsored", Outcome: "allow", Reason: "organic"}}
	}
	decisions := []Decision{{
		Kind:    "sponsored",
		Outcome: "label",
		Reason:  strings.ToLower(strings.TrimSpace(sponsoredState)),
	}}
	if !SponsoredBoostAllowed(opts) {
		decisions = append(decisions, Decision{
			Kind:    "sponsored_boost",
			Outcome: "deny",
			Reason:  "no-explicit-opt-in",
		})
	} else {
		decisions = append(decisions, Decision{
			Kind:    "sponsored_boost",
			Outcome: "allow",
			Reason:  "explicit-opt-in",
		})
	}
	return decisions
}
