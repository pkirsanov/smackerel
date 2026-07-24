package proactive

import (
	"encoding/json"
	"fmt"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// ProactiveCardModel is the immutable projection of exactly ONE permit/escalated
// verdict from the single spec-078 surfacing controller. It carries the title,
// a Producer-derived provenance line, the honest-state token, the opaque
// NudgeRef, and the fixed three-action set. It exists for no other verdict.
//
// The content_key is deliberately UNEXPORTED so it can never be marshalled onto
// a wire: a channel renderer serializes Ref (opaque) and never the content_key
// (FR-107-028). Internal callers read it via ContentKey().
type ProactiveCardModel struct {
	Title      string        `json:"title"`
	Provenance string        `json:"provenance"`
	State      HonestState   `json:"state"`
	Ref        NudgeRef      `json:"ref"`
	Actions    []NudgeAction `json:"actions"`
	Urgent     bool          `json:"urgent"`

	// contentKey is unexported: it is the controller's dedupe/ack identity and
	// MUST NOT reach any transport wire, data-* hook, or telemetry. json.Marshal
	// omits it structurally.
	contentKey string
}

// cardActions is the fixed three-action set every card renders, in order.
func cardActions() []NudgeAction {
	return []NudgeAction{ActionAct, ActionSnooze, ActionDismiss}
}

// ContentKey returns the controller dedupe/ack identity for internal wiring
// only. Callers MUST NOT place the return value on any wire.
func (c ProactiveCardModel) ContentKey() string { return c.contentKey }

// WireCallback returns the on-wire callback string for one action:
// a:n:<ref>:<a|s|d>. It carries ONLY the opaque ref — never the content_key —
// so it is the single sanctioned way a renderer produces a nudge wire payload.
func (c ProactiveCardModel) WireCallback(action NudgeAction) (string, bool) {
	return EncodeNudgeCallback(c.Ref, action)
}

// ProjectCard builds a ProactiveCardModel for a controller decision.
//
// It returns (card, true) ONLY for DecisionPermit / DecisionEscalated. For
// deduped / suppressed / deferred-budget-exhausted / any unknown verdict it
// returns (zero, false): those inform the budget meter and honest state via
// HonestStateForVerdict and never produce a card on any channel. An escalated
// card is marked Urgent with an "URGENT ESCALATION" provenance line.
//
// ref MUST be an opaque NudgeRef minted for this content_key; the caller mints
// it only after a card-bearing verdict, so no ref is ever created for a
// non-card verdict.
func ProjectCard(dec surfacing.SurfacingDecision, cand surfacing.SurfacingCandidate, ref NudgeRef, title string) (ProactiveCardModel, bool) {
	state := HonestStateForVerdict(dec.Kind)
	if !state.IsCard() {
		return ProactiveCardModel{}, false
	}
	urgent := dec.Kind == surfacing.DecisionEscalated
	return ProactiveCardModel{
		Title:      title,
		Provenance: provenanceLine(cand.Producer, urgent),
		State:      state,
		Ref:        ref,
		Actions:    cardActions(),
		Urgent:     urgent,
		contentKey: cand.ContentKey,
	}, true
}

// provenanceLine renders the Producer-derived provenance a card shows. An
// escalated card is explicitly marked as an urgent escalation on every channel
// (FR-107-010 / SCN-107-009).
func provenanceLine(p surfacing.Producer, urgent bool) string {
	base := producerLabel(p)
	if urgent {
		return "URGENT ESCALATION — " + base
	}
	return base
}

// producerLabel maps a bounded Producer enum value to a stable, human-legible
// provenance label. An unknown producer degrades to a neutral label rather than
// leaking the raw enum or fabricating a source.
func producerLabel(p surfacing.Producer) string {
	switch p {
	case surfacing.ProducerAlerts:
		return "From your alerts"
	case surfacing.ProducerDigest:
		return "From your digest"
	case surfacing.ProducerResurfacing:
		return "Resurfaced for you"
	case surfacing.ProducerWeeklySynthesis:
		return "From your weekly synthesis"
	case surfacing.ProducerMonthlyReport:
		return "From your monthly report"
	case surfacing.ProducerPreMeetingBriefs:
		return "From a pre-meeting brief"
	case surfacing.ProducerFrequentLookups:
		return "From your frequent lookups"
	case surfacing.ProducerNotification:
		return "From a notification"
	default:
		return "From your knowledge base"
	}
}

// MarshalJSON is defined to make the anti-leak contract explicit and testable:
// the serialized form carries the opaque ref and the honest state but NEVER the
// content_key. It mirrors the default field set (contentKey is unexported and
// already omitted) and exists so a regression that promotes contentKey to an
// exported/serialized field fails this method's test.
func (c ProactiveCardModel) MarshalJSON() ([]byte, error) {
	type wire struct {
		Title      string      `json:"title"`
		Provenance string      `json:"provenance"`
		State      HonestState `json:"state"`
		Ref        NudgeRef    `json:"ref"`
		Actions    []string    `json:"actions"`
		Urgent     bool        `json:"urgent"`
	}
	actions := make([]string, 0, len(c.Actions))
	for _, a := range c.Actions {
		actions = append(actions, a.String())
	}
	return json.Marshal(wire{
		Title:      c.Title,
		Provenance: c.Provenance,
		State:      c.State,
		Ref:        c.Ref,
		Actions:    actions,
		Urgent:     c.Urgent,
	})
}

// String renders a compact debug form that, like the wire form, never includes
// the content_key.
func (c ProactiveCardModel) String() string {
	return fmt.Sprintf("ProactiveCardModel{state=%s ref=%s urgent=%t}", c.State, c.Ref, c.Urgent)
}
