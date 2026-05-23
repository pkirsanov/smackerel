package notification

import "testing"

func TestClassifierDoesNotBranchOnSourceSpecificFields(t *testing.T) {
	left := testNormalizedNotification("source-a", SeverityMedium, DomainOps, IntentInvestigate)
	left.SourceType = "webhook_fixture"
	left.SourceSpecificRef = map[string]any{"vendor_priority": "urgent"}
	right := left
	right.ID = "source-b"
	right.SourceType = "queue_fixture"
	right.SourceInstanceID = "queue-a"
	right.SourceSpecificRef = map[string]any{"queue_priority": "low"}

	classifier := NewClassifier("rules-v1")
	leftClassification, err := classifier.Classify(left, ClassificationContext{KnownServices: []string{left.Service}})
	if err != nil {
		t.Fatalf("classify left: %v", err)
	}
	rightClassification, err := classifier.Classify(right, ClassificationContext{KnownServices: []string{right.Service}})
	if err != nil {
		t.Fatalf("classify right: %v", err)
	}
	if leftClassification.Severity != rightClassification.Severity || leftClassification.Domain != rightClassification.Domain || leftClassification.Intent != rightClassification.Intent {
		t.Fatalf("equivalent normalized semantics classified differently: left=%+v right=%+v", leftClassification, rightClassification)
	}
}
