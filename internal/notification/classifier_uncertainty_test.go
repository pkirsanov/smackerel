package notification

import "testing"

func TestClassifierRecordsUncertaintyWhenEvidenceIsMissing(t *testing.T) {
	notification := testNormalizedNotification("uncertain-a", SeverityUnknown, DomainUnknown, IntentUnknown)
	notification.Title = "attention needed"
	notification.Body = "something changed"
	notification.Subject = "unknown-service"

	classification, err := NewClassifier("rules-v1").Classify(notification, ClassificationContext{})
	if err != nil {
		t.Fatalf("classify uncertain notification: %v", err)
	}
	if classification.Confidence >= 0.7 {
		t.Fatalf("missing evidence produced inflated confidence: %+v", classification)
	}
	if len(classification.Uncertainty) == 0 {
		t.Fatalf("missing evidence did not record uncertainty: %+v", classification)
	}
	if _, ok := classification.Signals["fabricated_service"]; ok {
		t.Fatalf("classifier fabricated a service signal: %+v", classification.Signals)
	}
}
