package contracts

// RefusalCause is the closed-vocabulary discriminator used by the
// provenance gate to pick a cause-specific canonical refusal body.
// Added via PKT-061-A from spec 064 to extend the original single
// CanonicalRefusalBody into a taxonomy that distinguishes
// open-knowledge-agent refusal causes (budget exhaustion, tool
// unavailability, fabricated-source rejection, internal-only
// restriction, ambiguous query) from the default "no sourced
// answer" case which remains the fallback for any uncategorised
// refusal.
type RefusalCause string

const (
	// RefusalDefault is the original spec 061 canonical refusal —
	// used when the assembler did not classify the cause OR when a
	// scenario requires provenance but produced an unsourced body
	// without a more specific cause.
	RefusalDefault RefusalCause = "default"
	// RefusalBudgetExhausted — the open-knowledge agent hit one of
	// its bounded budgets (iterations / tokens / USD) before it
	// could ground an answer. Maps to spec 064 TerminationCapIterations,
	// TerminationCapTokens, TerminationCapUSD.
	RefusalBudgetExhausted RefusalCause = "budget_exhausted"
	// RefusalToolUnavailable — a tool required to ground the answer
	// returned a hard error or was disabled.
	RefusalToolUnavailable RefusalCause = "tool_unavailable"
	// RefusalFabricatedSourceBlocked — the cite-back verifier
	// rejected the planner's citations because they were not
	// present in the tool trace. Maps to spec 064
	// TerminationFabricatedSource.
	RefusalFabricatedSourceBlocked RefusalCause = "fabricated_source_blocked"
	// RefusalInternalOnlyRestricted — the user (or policy) has
	// restricted the agent to internal-knowledge-graph lookups
	// only, but the query required outbound retrieval.
	RefusalInternalOnlyRestricted RefusalCause = "internal_only_restricted"
	// RefusalAmbiguousNotClarified — the planner could not pick a
	// search target and the disambiguation budget was exhausted.
	RefusalAmbiguousNotClarified RefusalCause = "ambiguous_not_clarified"
)

// AllRefusalCauses is the exhaustive closed-vocabulary list,
// including RefusalDefault so closed-vocabulary tests can iterate
// every case.
var AllRefusalCauses = []RefusalCause{
	RefusalDefault,
	RefusalBudgetExhausted,
	RefusalToolUnavailable,
	RefusalFabricatedSourceBlocked,
	RefusalInternalOnlyRestricted,
	RefusalAmbiguousNotClarified,
}

// Canonical refusal body strings. Each is short per P7 (Small,
// Frequent, Actionable Output) and ends with the capture-as-fallback
// tail per P9 (Design For Restart, Not Perfection). The default
// body is preserved verbatim from the original spec 061
// CanonicalRefusalBody constant for backward compatibility.
const (
	canonicalRefusalDefault                 = "I don't have a sourced answer for that."
	canonicalRefusalBudgetExhausted         = "I couldn't complete that within the answer budget — saved as an idea."
	canonicalRefusalToolUnavailable         = "A tool I needed isn't available right now — saved as an idea."
	canonicalRefusalFabricatedSourceBlocked = "I couldn't verify the sources I would have cited — saved as an idea."
	canonicalRefusalInternalOnlyRestricted  = "That requires looking outside your knowledge graph, which is disabled — saved as an idea."
	canonicalRefusalAmbiguousNotClarified   = "I couldn't decide what to look up — saved as an idea."
)

// CanonicalRefusalBodyFor returns the canonical user-facing refusal
// text for the given cause. An unrecognised cause falls back to the
// default body so the contract is total (never returns "").
func CanonicalRefusalBodyFor(cause RefusalCause) string {
	switch cause {
	case RefusalBudgetExhausted:
		return canonicalRefusalBudgetExhausted
	case RefusalToolUnavailable:
		return canonicalRefusalToolUnavailable
	case RefusalFabricatedSourceBlocked:
		return canonicalRefusalFabricatedSourceBlocked
	case RefusalInternalOnlyRestricted:
		return canonicalRefusalInternalOnlyRestricted
	case RefusalAmbiguousNotClarified:
		return canonicalRefusalAmbiguousNotClarified
	default:
		return canonicalRefusalDefault
	}
}
