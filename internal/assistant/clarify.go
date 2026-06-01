// Spec 068 SCOPE-4 — clarification gate helpers (SCN-068-A05).
//
// When the intent compiler classifies a turn as clarify (or returns
// missing_slots), the facade emits a clarification response and MUST
// NOT call the router. These helpers are kept out of facade.go so the
// transport-neutral predicate and body builder are easy to unit-test.

package assistant

import (
	"fmt"
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/intent"
)

// requiresClarification returns true when the compiled intent should
// short-circuit the router and emit a clarification response instead.
// Triggers: ActionClarify, or a non-empty MissingSlots list. The
// ActionRefuse and ActionCaptureOnly classes are handled elsewhere
// (or fall through to the existing capture/refuse paths).
func requiresClarification(c intent.CompiledIntent) bool {
	if c.ActionClass == intent.ActionClarify {
		return true
	}
	if len(c.MissingSlots) > 0 {
		return true
	}
	return false
}

// buildClarificationBody returns the user-facing clarification text.
// Preference order: compiler-provided clarification_prompt -> a
// deterministic fallback that names the missing slots -> a generic
// "please clarify" string.
func buildClarificationBody(c intent.CompiledIntent) string {
	if c.ClarificationPrompt != nil {
		if s := strings.TrimSpace(*c.ClarificationPrompt); s != "" {
			return s
		}
	}
	if len(c.MissingSlots) > 0 {
		return fmt.Sprintf("please clarify: %s.", strings.Join(c.MissingSlots, ", "))
	}
	return "please clarify your request."
}
