package intelligence

import (
	"testing"
	"time"
)

func TestInsightType_Constants(t *testing.T) {
	types := []InsightType{InsightThroughLine, InsightContradiction, InsightPattern, InsightSerendipity}
	if len(types) != 4 {
		t.Errorf("expected 4 insight types, got %d", len(types))
	}
}

func TestAlertType_Constants(t *testing.T) {
	types := []AlertType{AlertBill, AlertReturnWindow, AlertTripPrep, AlertRelationship, AlertCommitmentOverdue, AlertMeetingBrief}
	if len(types) != 6 {
		t.Errorf("expected 6 alert types, got %d", len(types))
	}
}

func TestAlertStatus_Lifecycle(t *testing.T) {
	alert := &Alert{
		AlertType: AlertBill,
		Title:     "Test Alert",
		Body:      "Test body",
		Priority:  2,
		Status:    AlertPending,
		CreatedAt: time.Now(),
	}

	if alert.Status != AlertPending {
		t.Errorf("expected pending, got %s", alert.Status)
	}

	alert.Status = AlertDelivered
	if alert.Status != AlertDelivered {
		t.Errorf("expected delivered, got %s", alert.Status)
	}

	alert.Status = AlertDismissed
	if alert.Status != AlertDismissed {
		t.Errorf("expected dismissed, got %s", alert.Status)
	}
}

func TestAlertPriority(t *testing.T) {
	high := &Alert{Priority: 1}
	medium := &Alert{Priority: 2}
	low := &Alert{Priority: 3}

	if high.Priority >= medium.Priority {
		t.Error("high priority should have lower number")
	}
	if medium.Priority >= low.Priority {
		t.Error("medium priority should have lower number than low")
	}
}

func TestSynthesisInsight_Fields(t *testing.T) {
	insight := SynthesisInsight{
		ID:                "test-1",
		InsightType:       InsightThroughLine,
		ThroughLine:       "All three sources converge on pricing strategy",
		SourceArtifactIDs: []string{"art-1", "art-2", "art-3"},
		Confidence:        0.85,
	}

	if len(insight.SourceArtifactIDs) != 3 {
		t.Errorf("expected 3 source artifacts, got %d", len(insight.SourceArtifactIDs))
	}
	if insight.Confidence < 0 || insight.Confidence > 1 {
		t.Errorf("confidence should be 0-1, got %f", insight.Confidence)
	}
}

func TestNewEngine_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}
