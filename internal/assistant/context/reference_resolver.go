// Spec 061 SCOPE-04 — reference resolver.
//
// Per design §6.4, the capability layer resolves user reference
// expressions ("that one", "open 2") against the most recent
// ContextTurn's SourceIDs BEFORE building the IntentEnvelope. The
// resolver is intentionally narrow:
//
//   - "that" / "that one" / "it"           → newest turn, first source.
//   - "open N" / "show N" / numeric digits → newest turn, Nth source
//                                            (1-indexed).
//   - unresolvable                         → ResolveOutcomeMissing
//                                            (caller short-circuits
//                                             with ErrSlotMissing).
//   - no reference detected                → ResolveOutcomeNone
//                                            (caller proceeds to
//                                             router as normal).
//
// REGEX is intentionally absent here per the spec 037 / spec 061 no-
// regex discipline. Reference detection is a small token-based
// classifier. The detection logic is not exhaustive — its purpose is
// to catch the obvious phrasings that real users type, not to parse
// natural language.

package assistantctx

import (
	"strconv"
	"strings"
)

// ResolveOutcome discriminates the four possible reference-resolution
// states.
type ResolveOutcome string

const (
	// ResolveOutcomeNone — the text contains no recognised reference;
	// the caller proceeds to normal routing.
	ResolveOutcomeNone ResolveOutcome = "none"

	// ResolveOutcomeResolved — a reference was detected and the
	// resolver returned the matching source ID.
	ResolveOutcomeResolved ResolveOutcome = "resolved"

	// ResolveOutcomeMissing — a reference was detected but could not
	// be resolved (out-of-range numeric, no prior context, etc.);
	// the caller emits Status=StatusUnavailable + ErrorCause=ErrSlotMissing.
	ResolveOutcomeMissing ResolveOutcome = "missing"
)

// ReferenceResult is the structured return value of ResolveReference.
// SourceID is non-empty iff Outcome == ResolveOutcomeResolved.
// AvailableSources is the count of sources on the most recent turn
// (or 0 if no prior turn exists); the caller uses it to build the
// human-readable "last result has N sources." message.
type ReferenceResult struct {
	Outcome          ResolveOutcome
	SourceID         string
	AvailableSources int
}

// ResolveReference inspects userText for a reference expression and,
// if one is present, attempts to resolve it against the supplied
// WorkingContext (which MUST contain the most recent turn, if any).
//
// Returns:
//   - Outcome == ResolveOutcomeNone     when userText has no reference.
//   - Outcome == ResolveOutcomeResolved when a reference was found
//     and SourceID is set.
//   - Outcome == ResolveOutcomeMissing  when a reference was found
//     but could not be resolved
//     (no prior turn, no sources,
//     or numeric out-of-range).
func ResolveReference(userText string, wc WorkingContext) ReferenceResult {
	idx, ok := detectReference(userText)
	if !ok {
		return ReferenceResult{Outcome: ResolveOutcomeNone}
	}
	if len(wc.Turns) == 0 {
		return ReferenceResult{Outcome: ResolveOutcomeMissing}
	}
	latest := wc.Turns[len(wc.Turns)-1]
	if len(latest.SourceIDs) == 0 {
		return ReferenceResult{Outcome: ResolveOutcomeMissing}
	}
	// 1-indexed numeric out-of-range
	if idx < 1 || idx > len(latest.SourceIDs) {
		return ReferenceResult{
			Outcome:          ResolveOutcomeMissing,
			AvailableSources: len(latest.SourceIDs),
		}
	}
	return ReferenceResult{
		Outcome:          ResolveOutcomeResolved,
		SourceID:         latest.SourceIDs[idx-1],
		AvailableSources: len(latest.SourceIDs),
	}
}

// detectReference inspects (lowercased, whitespace-tokenised) userText
// for a v1 reference phrase and returns the 1-indexed position the
// reference resolves to.
//
// Detection rules (intentionally narrow):
//   - "that" / "that one" / "it"           → index 1 (default to first source).
//   - "open N" / "show N" / "show me N"   → index N (must parse as int).
//   - bare integer at the START of the message (e.g. "2") → index N.
//
// Anything else returns (0, false).
func detectReference(userText string) (int, bool) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(userText)))
	if len(tokens) == 0 {
		return 0, false
	}

	first := tokens[0]

	// "that" / "that one" / "it" → index 1
	switch first {
	case "that":
		return 1, true
	case "it":
		// only treat "it" as a reference when it is the WHOLE message
		// or the user explicitly chained another reference token
		// (e.g. "it please"). One-word "it" is the common case.
		if len(tokens) == 1 {
			return 1, true
		}
		return 0, false
	}

	// "open N" / "show N" / "show me N"
	switch first {
	case "open", "show":
		// look for the first numeric token after the verb
		for _, t := range tokens[1:] {
			if n, err := strconv.Atoi(t); err == nil {
				return n, true
			}
		}
		// "open" / "show" without a numeric tail is NOT a reference —
		// it could be unrelated free-form text (e.g. "open the door").
		return 0, false
	}

	// Bare leading integer → index N
	if n, err := strconv.Atoi(first); err == nil {
		return n, true
	}

	return 0, false
}
