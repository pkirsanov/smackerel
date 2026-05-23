package notification

import (
	"testing"
	"time"
)

func TestIncidentStateMachineRecordsTransitionsAndRefusesInvalidMoves(t *testing.T) {
	now := time.Date(2026, 5, 22, 7, 20, 0, 0, time.UTC)
	incident := Incident{ID: "incident-state-a", State: IncidentStateObserving}
	transition, updated, err := NewIncidentStateMachine().Transition(incident, IncidentStateActive, TransitionCause{ActorKind: ActorSystem, ActorRef: "handler", Rationale: "persistence threshold crossed", At: now})
	if err != nil {
		t.Fatalf("valid transition: %v", err)
	}
	if updated.State != IncidentStateActive || transition.FromState == nil || *transition.FromState != IncidentStateObserving || transition.ToState != IncidentStateActive {
		t.Fatalf("transition mismatch: transition=%+v updated=%+v", transition, updated)
	}

	refusal, _, err := NewIncidentStateMachine().Transition(updated, IncidentStateObserving, TransitionCause{ActorKind: ActorSystem, ActorRef: "handler", Rationale: "invalid rewind", At: now.Add(time.Second)})
	if err == nil {
		t.Fatal("expected invalid transition refusal")
	}
	if !refusal.Refused || refusal.ToState != IncidentStateObserving {
		t.Fatalf("invalid transition refusal was not recorded: %+v", refusal)
	}
}
