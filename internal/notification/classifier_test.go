package notification

import "testing"

func TestClassifierStoresSeverityDomainIntentWithRationale(t *testing.T) {
	notification := testNormalizedNotification("classify-a", SeverityUnknown, DomainUnknown, IntentUnknown)
	notification.Title = "checkout outage"
	notification.Body = "checkout-api is down for five minutes"
	notification.SourceSeverity = "critical"
	notification.Tags = map[string][]string{"source": {"outage"}, "handler": {"checkout"}}

	classification, err := NewClassifier("rules-v1").Classify(notification, ClassificationContext{KnownServices: []string{"checkout-api"}})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if classification.Severity != SeverityCritical || classification.Domain != DomainOps || classification.Intent != IntentOutage {
		t.Fatalf("classification labels mismatch: %+v", classification)
	}
	if classification.Confidence <= 0.7 {
		t.Fatalf("classification confidence too low for strong evidence: %f", classification.Confidence)
	}
	if classification.Rationale == "" || classification.ClassifierVersion != "rules-v1" {
		t.Fatalf("classification missing rationale/version: %+v", classification)
	}
}
