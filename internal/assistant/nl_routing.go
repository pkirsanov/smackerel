// Spec 076 SCOPE-4a — facade NL routing for /find + /rate.
//
// SCN-066-A02: NL "find me X" (and equivalent phrasings) is routed
// deterministically to the existing internal-retrieval scenario
// (retrieval_qa) at the facade layer, mirroring the behaviour of the
// retired /find slash command. The router's similarity path would
// likely reach the same scenario, but a deterministic facade rule
// makes the routing observable and stable for the spec 066 retirement
// regression.
//
// SCN-066-A03: NL "rate this/that/it" (and equivalent rate-without-
// referenceable-target phrasings) is routed into spec 061's
// disambiguation flow. Because rate-target candidates are not produced
// by the router (there is no rateable-artifact scenario in the v1
// substrate), the facade emits a deterministic DisambiguationPrompt
// seeded from the conversation's recent context turns when present,
// or the save_as_note sentinel choice on its own when no recent
// rateable artifact is in context. Both shapes are valid spec 061
// DisambiguationPrompts — same envelope, same persistence contract
// (PendingDisambig is written by the facade's standard append path),
// same per-transport renderers.
//
// Scope boundary (per scopes.md Scope 4a): facade rule only. No
// changes under internal/annotation/. No router changes. No new
// scenarios. No interactionMap mutation.
package assistant

import "strings"

// NLRoutingHit is the result of LookupNLRouting.
//
// Exactly one of ScenarioID / RateDisambig is set on a true `ok`.
//
//   - ScenarioID non-empty, RateDisambig false: the facade SHOULD
//     route the message to ScenarioID via the explicit-id fast path
//     (same path as a slash-shortcut hit). Used for NL find.
//   - ScenarioID empty,   RateDisambig true : the facade SHOULD
//     emit a deterministic spec 061 DisambiguationPrompt asking the
//     user to identify the rate target. Used for NL rate without a
//     referenceable target.
type NLRoutingHit struct {
	ScenarioID   string
	RateDisambig bool
}

// findPrefixes are the NL prefixes the facade treats as a NL /find
// replacement. Each entry MUST be followed by at least one
// whitespace-delimited non-empty token in the inbound text (the
// retrieval query body); a bare prefix is NOT a NL find.
//
// Matching is case-insensitive against the leading token sequence
// of the trimmed input, scanned in order — the FIRST entry whose
// tokens match wins. Longer prefixes (e.g. "find me") MUST appear
// before their shorter forms (e.g. "find") so the longer phrasing
// is preferred when both could match.
var findPrefixes = []string{
	"find me",
	"find my",
	"search for",
	"look for",
	"find",
	"search",
}

// rateTargetWords are the demonstrative-pronoun target words that,
// when they appear as the SECOND whitespace-delimited token of a
// trimmed "rate <word> ..." input, classify the message as a NL
// /rate replacement with an unresolved target — i.e. the user is
// rating "this/that/it/them/these/those" without naming the
// artifact, so the facade owes them a spec 061 disambiguation
// prompt.
var rateTargetWords = map[string]struct{}{
	"this": {}, "that": {}, "it": {},
	"them": {}, "these": {}, "those": {},
}

// LookupNLRouting inspects the inbound text and, when it matches a
// SCOPE-4a NL routing rule, returns the corresponding NLRoutingHit
// with ok=true. Returns the zero NLRoutingHit and ok=false for any
// other text.
//
// The function is pure and safe for concurrent use. It allocates
// only the lowercased prefix it inspects.
func LookupNLRouting(text string) (NLRoutingHit, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return NLRoutingHit{}, false
	}
	lower := strings.ToLower(trimmed)
	// NL find: a recognized prefix followed by at least one non-empty
	// token. We require the prefix to be a whole-token match (next
	// rune is whitespace) so "findings" does not satisfy "find".
	for _, p := range findPrefixes {
		if len(lower) <= len(p) {
			continue
		}
		if !strings.HasPrefix(lower, p) {
			continue
		}
		next := lower[len(p)]
		if next != ' ' && next != '\t' {
			continue
		}
		tail := strings.TrimSpace(lower[len(p):])
		if tail == "" {
			continue
		}
		return NLRoutingHit{ScenarioID: "retrieval_qa"}, true
	}
	// NL rate: token 0 == "rate", token 1 ∈ rateTargetWords.
	// "rate that 8 out of 10" → match. "rate the burger" → no
	// match (the user named a target). Bare "rate" → no match.
	fields := strings.Fields(lower)
	if len(fields) >= 2 && fields[0] == "rate" {
		if _, ok := rateTargetWords[fields[1]]; ok {
			return NLRoutingHit{RateDisambig: true}, true
		}
	}
	return NLRoutingHit{}, false
}
