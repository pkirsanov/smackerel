package proactive

import (
	"testing"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

func TestHonestStateForVerdict_AllKinds(t *testing.T) {
	cases := []struct {
		kind surfacing.DecisionKind
		want HonestState
	}{
		{surfacing.DecisionPermit, StatePermitted},
		{surfacing.DecisionEscalated, StateEscalated},
		{surfacing.DecisionDeduped, StateDeduped},
		{surfacing.DecisionSuppressed, StateSuppressed},
		{surfacing.DecisionDeferredBudgetExhausted, StateBudgetExhausted},
	}
	for _, tc := range cases {
		if got := HonestStateForVerdict(tc.kind); got != tc.want {
			t.Errorf("HonestStateForVerdict(%q) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// TestHonestStateForVerdict_UnknownFailsClosed proves an unknown verdict maps to
// StateError (fail-closed), never to a card-bearing state.
func TestHonestStateForVerdict_UnknownFailsClosed(t *testing.T) {
	got := HonestStateForVerdict(surfacing.DecisionKind("future-verdict-we-do-not-know"))
	if got != StateError {
		t.Fatalf("unknown verdict = %q, want %q (fail-closed)", got, StateError)
	}
	if got.IsCard() {
		t.Fatalf("StateError.IsCard() = true, want false")
	}
}

func TestHonestState_IsCard(t *testing.T) {
	cardStates := map[HonestState]bool{
		StatePermitted:       true,
		StateEscalated:       true,
		StateActed:           false,
		StateSnoozed:         false,
		StateSuppressed:      false,
		StateAlreadyHandled:  false,
		StateExpired:         false,
		StateDeduped:         false,
		StateBudgetExhausted: false,
		StateQuiet:           false,
		StateNoCorrelation:   false,
		StateDegraded:        false,
		StateUnauthorized:    false,
		StateError:           false,
	}
	for s, want := range cardStates {
		if got := s.IsCard(); got != want {
			t.Errorf("%q.IsCard() = %t, want %t", s, got, want)
		}
	}
}
