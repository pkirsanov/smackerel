package proactive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// candidate is a small helper for building a surfacing candidate in tests.
func candidate(key string, producer surfacing.Producer, channel surfacing.Channel) surfacing.SurfacingCandidate {
	return surfacing.SurfacingCandidate{
		Producer:   producer,
		Channel:    channel,
		ContentKey: key,
	}
}

// TestProjectCard_PermitProducesCard covers the SCN-107-004/-008/-009 invariant
// that a card exists ONLY for a permit verdict (permit branch).
func TestProjectCard_PermitProducesCard(t *testing.T) {
	dec := surfacing.SurfacingDecision{Kind: surfacing.DecisionPermit, Reason: "within_budget"}
	cand := candidate("artifact-42", surfacing.ProducerAlerts, surfacing.ChannelTelegram)

	card, ok := ProjectCard(dec, cand, NudgeRef("REF-1"), "Renewal due")
	if !ok {
		t.Fatalf("ProjectCard(permit) ok = false, want true")
	}
	if card.State != StatePermitted {
		t.Errorf("State = %q, want %q", card.State, StatePermitted)
	}
	if card.Urgent {
		t.Errorf("permit card marked Urgent, want non-urgent")
	}
	if card.Ref != NudgeRef("REF-1") {
		t.Errorf("Ref = %q, want REF-1", card.Ref)
	}
	if card.ContentKey() != "artifact-42" {
		t.Errorf("ContentKey() = %q, want artifact-42", card.ContentKey())
	}
	wantActions := []NudgeAction{ActionAct, ActionSnooze, ActionDismiss}
	if len(card.Actions) != len(wantActions) {
		t.Fatalf("Actions = %v, want %v", card.Actions, wantActions)
	}
	for i, a := range wantActions {
		if card.Actions[i] != a {
			t.Errorf("Actions[%d] = %v, want %v", i, card.Actions[i], a)
		}
	}
	if card.Provenance == "" {
		t.Errorf("Provenance is empty; a card must carry a producer-derived provenance line")
	}
}

// TestProjectCard_EscalatedIsUrgentWithProvenance covers SCN-107-009 at the unit
// level: an escalated verdict projects an urgent card whose provenance marks the
// urgent escalation.
func TestProjectCard_EscalatedIsUrgentWithProvenance(t *testing.T) {
	dec := surfacing.SurfacingDecision{Kind: surfacing.DecisionEscalated, Reason: "urgent_escalation"}
	cand := candidate("artifact-urgent", surfacing.ProducerAlerts, surfacing.ChannelWebPush)

	card, ok := ProjectCard(dec, cand, NudgeRef("REF-U"), "Flight in 2h")
	if !ok {
		t.Fatalf("ProjectCard(escalated) ok = false, want true")
	}
	if card.State != StateEscalated {
		t.Errorf("State = %q, want %q", card.State, StateEscalated)
	}
	if !card.Urgent {
		t.Errorf("escalated card Urgent = false, want true")
	}
	if !strings.Contains(card.Provenance, "URGENT ESCALATION") {
		t.Errorf("Provenance = %q, want it to mark URGENT ESCALATION", card.Provenance)
	}
}

// TestProjectCard_NonCardVerdictsProduceNoCard covers SCN-107-008 and the
// foundation invariant that deduped/suppressed/deferred-budget-exhausted and any
// unknown verdict never produce a card.
func TestProjectCard_NonCardVerdictsProduceNoCard(t *testing.T) {
	cand := candidate("artifact-x", surfacing.ProducerDigest, surfacing.ChannelTelegram)
	for _, kind := range []surfacing.DecisionKind{
		surfacing.DecisionDeduped,
		surfacing.DecisionSuppressed,
		surfacing.DecisionDeferredBudgetExhausted,
		surfacing.DecisionKind("some-unknown-future-verdict"),
	} {
		dec := surfacing.SurfacingDecision{Kind: kind}
		card, ok := ProjectCard(dec, cand, NudgeRef("REF-N"), "should not render")
		if ok {
			t.Errorf("ProjectCard(%q) ok = true, want false", kind)
		}
		if !reflect.DeepEqual(card, ProactiveCardModel{}) {
			t.Errorf("ProjectCard(%q) returned a non-zero card: %+v", kind, card)
		}
	}
}

// TestProducerLabel_AllBoundedProducers proves every bounded Producer maps to a
// non-empty, stable label and an unknown producer degrades safely.
func TestProducerLabel_AllBoundedProducers(t *testing.T) {
	producers := []surfacing.Producer{
		surfacing.ProducerAlerts, surfacing.ProducerDigest, surfacing.ProducerResurfacing,
		surfacing.ProducerWeeklySynthesis, surfacing.ProducerMonthlyReport,
		surfacing.ProducerPreMeetingBriefs, surfacing.ProducerFrequentLookups,
		surfacing.ProducerNotification,
	}
	for _, p := range producers {
		if got := producerLabel(p); got == "" {
			t.Errorf("producerLabel(%q) is empty", p)
		}
	}
	if got := producerLabel(surfacing.Producer("mystery")); got == "" {
		t.Errorf("producerLabel(unknown) is empty; want a neutral degraded label")
	}
}

// TestProactiveCardModel_MarshalOmitsContentKey is the card half of the
// anti-leak boundary (FR-107-028): the serialized card carries the opaque ref
// and honest state but NEVER the content_key.
func TestProactiveCardModel_MarshalOmitsContentKey(t *testing.T) {
	secret := "artifact-super-secret-key"
	card, ok := ProjectCard(
		surfacing.SurfacingDecision{Kind: surfacing.DecisionPermit},
		candidate(secret, surfacing.ProducerAlerts, surfacing.ChannelTelegram),
		NudgeRef("REF-MARSHAL"),
		"Title",
	)
	if !ok {
		t.Fatalf("ProjectCard ok = false")
	}
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("serialized card leaks content_key %q: %s", secret, raw)
	}
	if !strings.Contains(string(raw), "REF-MARSHAL") {
		t.Errorf("serialized card missing opaque ref: %s", raw)
	}
}
