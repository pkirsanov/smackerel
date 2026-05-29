// Spec 061 SCOPE-04 — slash-command shortcut map.
//
// Per design §3.4 the capability layer pre-checks every inbound text
// message for an exact slash-command prefix and, on hit, builds the
// IntentEnvelope with explicit ScenarioID set so agent.Router.Route
// takes the explicit-id fast path (BS-002) — no embedding call, no
// borderline post-processor, deterministic dispatch.
//
// The shortcut set is intentionally TINY and v1-frozen: the v1 user-
// observable surface is "/ask | /weather | /remind | /reset" (design
// §3.4 + UX §14.A.3). /reset is a capability-level action (drops
// pending confirm/disambig state) and does NOT name a scenario id;
// LookupShortcut signals it via a dedicated return value.
//
// Matching rules (design §3.4):
//   - Case-SENSITIVE (slashes are universally lowercase per UX guide).
//   - The shortcut MUST be the FIRST whitespace-delimited token.
//     Trailing text after the shortcut is preserved as the natural-
//     language tail and passed through unchanged in the envelope's
//     RawInput; callers MAY trim it before forwarding.
//   - Surrounding whitespace on the text input is trimmed before
//     extraction (so a leading newline does not defeat the prefix).

package assistant

import "strings"

// ResetActionID is the sentinel return value for LookupShortcut when
// the user issued "/reset". It is NOT a scenario id (no Spec 037
// scenario by that name); the facade interprets it as the capability-
// level reset action (delete the conversation row for the user/transport).
const ResetActionID = "__capability_reset__"

// SlashShortcuts maps the four v1 slash commands to their target.
// Three of them name an agent.Scenario id; "/reset" maps to the
// sentinel ResetActionID consumed by the facade as a capability
// action.
//
// The map is exported so tests and adapters can enumerate it (e.g.
// the Telegram adapter renders /reset in its slash menu).
var SlashShortcuts = map[string]string{
	"/ask":     "retrieval_qa",
	"/weather": "weather_query",
	"/remind":  "notification_schedule",
	"/reset":   ResetActionID,
}

// LookupShortcut extracts the first whitespace-delimited token from
// the (trimmed) text and, if it matches a v1 slash command, returns
// the mapped target plus an isReset flag.
//
// Returns:
//   - scenarioID : the Spec 037 scenario id for /ask, /weather, /remind;
//     empty for /reset (the sentinel is internal to this
//     package and not useful to the router).
//   - isReset    : true iff the shortcut was "/reset".
//   - ok         : true iff the first token matched any entry in
//     SlashShortcuts; false for any non-shortcut input
//     (including the empty string).
//
// The function is pure (no allocations beyond the strings.Fields slice
// for the input split) and safe for concurrent use.
func LookupShortcut(text string) (scenarioID string, isReset bool, ok bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false, false
	}
	first := trimmed
	if i := indexAnyWhitespace(trimmed); i >= 0 {
		first = trimmed[:i]
	}
	target, hit := SlashShortcuts[first]
	if !hit {
		return "", false, false
	}
	if target == ResetActionID {
		return "", true, true
	}
	return target, false, true
}

// indexAnyWhitespace returns the byte index of the first whitespace
// rune in s, or -1 if none is present. We avoid strings.Fields here
// because we want to preserve the exact first token (no allocations
// for the tail).
func indexAnyWhitespace(s string) int {
	for i, r := range s {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			return i
		}
	}
	return -1
}

// StripShortcutPrefix returns the natural-language tail after a v1
// slash-command shortcut, or the trimmed input verbatim if no v1
// shortcut is present.
//
// Spec 061 Round-55 Defect-3 fix: the capability-layer dispatch must
// hand the executor a clean query string for the structured_context
// payload that satisfies each scenario's input_schema. The raw
// msg.Text still carries the slash prefix; this helper strips it.
//
// Examples:
//   - "/ask what is the weather" → "what is the weather"
//   - "/weather"                 → ""           (bare shortcut, no body)
//   - "  /remind   tomorrow   "  → "tomorrow"   (whitespace normalized)
//   - "hello there"              → "hello there"
//   - "/help anything"           → "/help anything" (not in v1 set)
//   - ""                         → ""
//
// The function is pure and follows the same case-sensitive, first-token
// matching contract as LookupShortcut. Both the input and the returned
// body are TrimSpace-normalized so downstream consumers (LLM prompts,
// schema-validated structured_context) see clean text. Safe for
// concurrent use.
func StripShortcutPrefix(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	first := trimmed
	i := indexAnyWhitespace(trimmed)
	if i >= 0 {
		first = trimmed[:i]
	}
	if _, ok := SlashShortcuts[first]; !ok {
		return trimmed
	}
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(trimmed[i:])
}
