package notification

import "testing"

func TestDecisionEngineChoosesExactlyOnePrimaryDecision(t *testing.T) {
	policy := DecisionPolicy{PersistenceThreshold: 2, EscalationSeverity: SeverityHigh, LowConfidenceThreshold: 0.55, AutonomousActions: map[string]ActionPolicy{"restart-cache": {Risk: RiskLow, ActionClass: ActionClassLowRisk, Destructive: false}}, OutputChannels: []string{"dashboard"}, MaxRetries: 2}
	engine, err := NewDecisionEngine(policy)
	if err != nil {
		t.Fatalf("decision engine: %v", err)
	}
	incident := Incident{ID: "incident-decision-a", State: IncidentStateActive, Severity: SeverityHigh, PersistenceCount: 3, RiskLevel: RiskMedium}
	classification := Classification{Severity: SeverityHigh, Domain: DomainOps, Intent: IntentOutage, Confidence: 0.91}
	decision := engine.Decide(testNormalizedNotification("decision-a", SeverityHigh, DomainOps, IntentOutage), classification, incident, nil, nil)
	if decision.Type == "" {
		t.Fatal("decision type is empty")
	}
	if len(decision.Alternatives) != 0 {
		t.Fatalf("decision engine emitted multiple primary alternatives: %+v", decision)
	}
	if decision.Type != DecisionUserEscalation {
		t.Fatalf("persistent high severity incident should escalate, got %+v", decision)
	}
}

func TestRoutineEventsStaySilentWithRecordOnlyDecision(t *testing.T) {
	engine, err := NewDecisionEngine(DecisionPolicy{PersistenceThreshold: 2, EscalationSeverity: SeverityHigh, LowConfidenceThreshold: 0.55, OutputChannels: []string{"dashboard"}, MaxRetries: 2})
	if err != nil {
		t.Fatalf("decision engine: %v", err)
	}
	incident := Incident{ID: "incident-routine-a", State: IncidentStateObserving, Severity: SeverityLow, PersistenceCount: 1, RiskLevel: RiskLow}
	classification := Classification{Severity: SeverityLow, Domain: DomainOps, Intent: IntentRoutine, Confidence: 0.8}
	decision := engine.Decide(testNormalizedNotification("routine-decision-a", SeverityLow, DomainOps, IntentRoutine), classification, incident, nil, nil)
	if decision.Type != DecisionRecordOnly && decision.Type != DecisionNoAction {
		t.Fatalf("routine event should stay silent, got %+v", decision)
	}
	if decision.RequiresOutput || decision.RequiresAction {
		t.Fatalf("routine decision produced side effects: %+v", decision)
	}
}
