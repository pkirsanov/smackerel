package notification

import "testing"

func TestEnricherRecordsBoundedReferencesAndMissingContext(t *testing.T) {
	notification := testNormalizedNotification("enrich-a", SeverityMedium, DomainOps, IntentInvestigate)
	incident := Incident{ID: "incident-enrich-a", Subject: notification.Subject, Service: notification.Service}
	enrichments := NewEnricher().Enrich(notification, incident, EnrichmentContext{ArtifactRefs: []string{"artifact-a"}, KnownServices: []string{notification.Service}})
	if len(enrichments) == 0 {
		t.Fatal("expected bounded enrichment references")
	}
	if enrichments[0].RefType != EnrichmentArtifact || enrichments[0].RefID != "artifact-a" {
		t.Fatalf("unexpected enrichment reference: %+v", enrichments[0])
	}

	missing := NewEnricher().Enrich(notification, incident, EnrichmentContext{})
	foundMissing := false
	for _, enrichment := range missing {
		if enrichment.MissingContextReason != "context_unavailable" {
			continue
		}
		foundMissing = true
	}
	if !foundMissing {
		t.Fatalf("missing context was not recorded explicitly: %+v", missing)
	}
}
