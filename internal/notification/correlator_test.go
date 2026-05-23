package notification

import (
	"testing"
	"time"
)

func TestCorrelatorGroupsRelatedSevereEventsIntoOneIncident(t *testing.T) {
	now := time.Date(2026, 5, 22, 7, 15, 0, 0, time.UTC)
	first := testNormalizedNotification("incident-first", SeverityHigh, DomainOps, IntentOutage)
	first.Subject = "checkout-api"
	first.Service = "checkout-api"
	incident := Incident{ID: "incident-a", IncidentKey: IncidentKey(first), State: IncidentStateActive, Subject: first.Subject, Service: first.Service, Severity: first.Severity, Domain: first.Domain, Intent: first.Intent, PersistenceCount: 1, LastEventAt: now.Add(-time.Minute), SourceInstanceIDs: []string{first.SourceInstanceID}}
	second := first
	second.ID = "incident-second"
	second.RawEventID = "raw-second"
	second.SourceInstanceID = "queue-source-b"
	second.ObservedAt = now

	result := NewCorrelator().Correlate(second, Classification{Severity: SeverityHigh, Domain: DomainOps, Intent: IntentOutage, Confidence: 0.91}, []Incident{incident}, now)
	if result.Incident.ID != incident.ID {
		t.Fatalf("related event did not join existing incident: %+v", result.Incident)
	}
	if result.Incident.PersistenceCount != 2 {
		t.Fatalf("incident persistence count = %d, want 2", result.Incident.PersistenceCount)
	}
	if result.Correlation.Kind != CorrelationSameService || result.Correlation.Score < 0.8 {
		t.Fatalf("unexpected correlation result: %+v", result.Correlation)
	}
}
