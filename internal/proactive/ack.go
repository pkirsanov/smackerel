package proactive

// Acknowledger is the process-wide acknowledgement sink the nudge ack path
// calls. It is satisfied by *surfacing.InMemoryAck (the single shared ack
// registry wired in cmd/core/main.go as sharedAck), so act/snooze/dismiss from
// ANY channel feed the one SuppressionWindow the controller already consults.
// Keeping it an interface avoids an import cycle and lets tests assert the call.
type Acknowledger interface {
	Acknowledge(contentKey string)
}

// AckOutcome is the honest render produced by resolving and acting on a nudge.
// Exactly one State is returned; Acknowledged is true only when this call made
// the single Acknowledge(content_key) call.
type AckOutcome struct {
	State        HonestState
	ContentKey   string
	Principal    string
	Action       NudgeAction
	Acknowledged bool
}

// NudgeAck is the single acknowledge path for every channel. act/snooze/dismiss
// resolve their opaque ref through the NudgeRegistry and call exactly one
// Acknowledge(content_key) on the process-wide ack sink, so acting once
// suppresses the same content_key on EVERY channel within
// suppression_window_hours (FR-107-003/007, SCN-107-004).
type NudgeAck struct {
	registry *NudgeRegistry
	ack      Acknowledger
}

// NewNudgeAck wires the registry and the process-wide ack sink. ack MAY be nil
// (e.g. a pre-wiring path); a nil sink degrades to resolving-only and never
// panics — but then no suppression is recorded, matching the controller's own
// nil-AckLookup contract.
func NewNudgeAck(registry *NudgeRegistry, ack Acknowledger) *NudgeAck {
	return &NudgeAck{registry: registry, ack: ack}
}

// Handle resolves ref, and on the first valid consume calls Acknowledge exactly
// once, returning the honest terminal render:
//
//   - live ref, first tap   -> Acknowledge(content_key); State acted/snoozed/suppressed
//   - already-handled ref    -> no ack; State already-handled (idempotent)
//   - unknown/expired ref     -> no ack; State expired
//
// act, snooze, and dismiss all acknowledge the same content_key; the difference
// is only the intent/label (design.md OQ6). There is no second store and no
// second budget — suppression is the controller's, keyed by content_key.
func (n *NudgeAck) Handle(ref NudgeRef, action NudgeAction) AckOutcome {
	resolved, status := n.registry.Consume(ref)
	switch status {
	case ResolveOK:
		if n.ack != nil {
			n.ack.Acknowledge(resolved.ContentKey)
		}
		return AckOutcome{
			State:        stateForAction(action),
			ContentKey:   resolved.ContentKey,
			Principal:    resolved.Principal,
			Action:       action,
			Acknowledged: n.ack != nil,
		}
	case ResolveAlreadyHandled:
		return AckOutcome{State: StateAlreadyHandled, Action: action}
	default: // ResolveExpired
		return AckOutcome{State: StateExpired, Action: action}
	}
}

// stateForAction maps an action to its post-ack honest terminal state. An
// unknown action fails closed to StateError so a malformed decode never renders
// as a successful ack.
func stateForAction(action NudgeAction) HonestState {
	switch action {
	case ActionAct:
		return StateActed
	case ActionSnooze:
		return StateSnoozed
	case ActionDismiss:
		return StateSuppressed
	default:
		return StateError
	}
}
