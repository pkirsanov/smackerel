// BUG-064-002 DEFECT 3a — a completed open_knowledge answer must carry
// the TERMINAL StatusAnswered, not the in-flight StatusThinking (which
// the Telegram adapter renders as a "thinking…" header on the delivered
// answer). This is the facade-side mapping that the prod assistant_turn
// log surfaced as status="thinking" on a termination_reason=final turn.
package assistant

import (
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestTranslateOutcomeToStatus_OpenKnowledgeAnswered_BUG064002(t *testing.T) {
	got := translateOutcomeToStatus(agent.OutcomeOK, "open_knowledge")
	if got == contracts.StatusThinking {
		t.Fatalf("BUG-064-002 DEFECT 3a: completed open_knowledge answer mapped to StatusThinking (renders 'thinking…' header)")
	}
	if got != contracts.StatusAnswered {
		t.Fatalf("open_knowledge OutcomeOK status = %q, want %q (terminal answered)", got, contracts.StatusAnswered)
	}
}

// Adversarial guard: the fix MUST be scoped to open_knowledge. Other
// scenarios' OutcomeOK keep the Thinking-class default (their specific
// terminal tokens are set by their skill adapters); a regression that
// changed the default for ALL scenarios would trip this.
func TestTranslateOutcomeToStatus_OtherScenarioUnchanged_BUG064002(t *testing.T) {
	got := translateOutcomeToStatus(agent.OutcomeOK, "weather_query")
	if got != contracts.StatusThinking {
		t.Fatalf("non-open_knowledge OutcomeOK status = %q, want %q (unchanged default)", got, contracts.StatusThinking)
	}
}
