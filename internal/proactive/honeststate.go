package proactive

import "github.com/smackerel/smackerel/internal/intelligence/surfacing"

// HonestState is the closed proactive honest-state vocabulary. Every proactive
// surface renders exactly one of these; each is distinct and visible, and none
// is ever silently substituted for another (Product Principle: honest states).
// A HonestState maps 1:1 onto a spec-106 data-view-state / data-operation-state
// token at the surface layer; this foundation owns the vocabulary, not the DOM.
type HonestState string

const (
	// StatePermitted / StateEscalated are the ONLY card-bearing states.
	StatePermitted HonestState = "permitted"
	StateEscalated HonestState = "escalated"

	// Post-ack terminal states.
	StateActed          HonestState = "acted"
	StateSnoozed        HonestState = "snoozed"
	StateSuppressed     HonestState = "suppressed"
	StateAlreadyHandled HonestState = "already-handled"
	StateExpired        HonestState = "expired"

	// Non-card controller verdicts and read conditions.
	StateDeduped         HonestState = "deduped"
	StateBudgetExhausted HonestState = "budget-exhausted"
	StateQuiet           HonestState = "quiet"
	StateNoCorrelation   HonestState = "no-related-items"
	StateDegraded        HonestState = "degraded"
	StateUnauthorized    HonestState = "unauthorized"

	// StateError is the fail-closed sink for any unknown condition. It is NEVER
	// a normal card.
	StateError HonestState = "error"
)

// HonestStateForVerdict maps a single controller DecisionKind onto the honest
// state a surface renders. permit/escalated are the only card-bearing verdicts;
// every other verdict renders an explicit, non-card honest state; an unknown
// DecisionKind fails closed to StateError, never to a card.
func HonestStateForVerdict(kind surfacing.DecisionKind) HonestState {
	switch kind {
	case surfacing.DecisionPermit:
		return StatePermitted
	case surfacing.DecisionEscalated:
		return StateEscalated
	case surfacing.DecisionDeduped:
		return StateDeduped
	case surfacing.DecisionSuppressed:
		return StateSuppressed
	case surfacing.DecisionDeferredBudgetExhausted:
		return StateBudgetExhausted
	default:
		return StateError
	}
}

// IsCard reports whether a state renders an actionable card. Only permitted and
// escalated do; this is the single truth every renderer consults so no surface
// can draw a card for a non-card verdict.
func (s HonestState) IsCard() bool {
	return s == StatePermitted || s == StateEscalated
}
