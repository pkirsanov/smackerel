// Spec 095 SCOPE-08 — synthesis/digest pool-eligibility predicate + aggressive
// decay routing (Idea 2).
//
// Low-evergreen, high-churn artifacts are excluded from the §10 synthesis and
// §12 digest candidate pools (R12) and routed to aggressive decay (an EARLIER
// input to the EXISTING lifecycle, not a fork — Principle 3). They remain FULLY
// searchable/retrievable (R13, Principle 9) — exclusion is pool-eligibility
// only, never hiding. The §10 synthesis and §12 digest builders consult this
// predicate INLINE: Increment 3 (PKT-095-C) wired the additive SQL
// PoolExclusionSQLPredicate into buildSynthesisClusterQuery
// (internal/intelligence/synthesis.go) and buildOvernightArtifactsQuery
// (internal/digest/generator.go), gated on the off-by-default SST switches — no
// separate packet was routed.
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R12, R13, SCN-095-B02/B03/B04
//   - specs/095-retrieval-strategy-routing/design.md §6
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-08
package evergreen

// PoolPolicy carries the SST pool-exclusion switches (config evergreen.pools.*).
type PoolPolicy struct {
	SynthesisExcludesLowEvergreen bool
	DigestExcludesLowEvergreen    bool
}

// IncludeInSynthesisPool reports whether the artifact (given its evergreen
// signal) is eligible for the §10 synthesis candidate pool. Low-evergreen items
// are excluded when the SST switch is on (R12); evergreen items are always
// included.
func IncludeInSynthesisPool(sig EvergreenSignal, p PoolPolicy) bool {
	if p.SynthesisExcludesLowEvergreen && !sig.Evergreen {
		return false
	}
	return true
}

// IncludeInDigestPool reports whether the artifact is eligible for the §12
// digest candidate pool. Low-evergreen items are excluded when the SST switch
// is on (R12).
func IncludeInDigestPool(sig EvergreenSignal, p PoolPolicy) bool {
	if p.DigestExcludesLowEvergreen && !sig.Evergreen {
		return false
	}
	return true
}

// Searchable ALWAYS returns true (R13, Principle 9): an ephemeral artifact
// remains fully searchable and retrievable; it is only de-prioritized from the
// permanent synthesis/digest surfaces, never hidden or deleted.
func Searchable(_ EvergreenSignal) bool { return true }

// AggressiveDecay reports whether the artifact should be routed to aggressive
// decay — an EARLIER input to the EXISTING momentum/cooling lifecycle (§11.1),
// NOT a parallel mechanism (Principle 3). Low-evergreen items decay faster.
func AggressiveDecay(sig EvergreenSignal) bool { return !sig.Evergreen }

// PoolExclusionTrace returns the §14.A `pool_excluded` observability token when
// the artifact is excluded from the named pool, or "" when it is included.
// Trace/audit only (Principle 8) — never surfaced to the user as "we hid this".
func PoolExclusionTrace(pool string, sig EvergreenSignal, p PoolPolicy) string {
	var excluded bool
	switch pool {
	case "synthesis":
		excluded = !IncludeInSynthesisPool(sig, p)
	case "digest":
		excluded = !IncludeInDigestPool(sig, p)
	}
	if excluded {
		return "pool_excluded pool=" + pool + " id=" + sig.ArtifactID + " reason=low_evergreen"
	}
	return ""
}

// FilterSynthesisPool returns only the signals eligible for the synthesis pool
// (helper for the builder adapter).
func FilterSynthesisPool(sigs []EvergreenSignal, p PoolPolicy) []EvergreenSignal {
	out := make([]EvergreenSignal, 0, len(sigs))
	for _, s := range sigs {
		if IncludeInSynthesisPool(s, p) {
			out = append(out, s)
		}
	}
	return out
}
