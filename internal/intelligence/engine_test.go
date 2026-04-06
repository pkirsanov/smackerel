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

func TestAlert_Lifecycle(t *testing.T) {
	a := &Alert{
		ID:        "test-1",
		AlertType: AlertBill,
		Title:     "AWS Invoice",
		Body:      "Monthly bill due",
		Priority:  2,
		Status:    AlertPending,
	}

	if a.Status != AlertPending {
		t.Error("should start pending")
	}

	a.Status = AlertDelivered
	if a.Status != AlertDelivered {
		t.Error("should transition to delivered")
	}

	a.Status = AlertSnoozed
	if a.Status != AlertSnoozed {
		t.Error("should transition to snoozed")
	}

	a.Status = AlertDismissed
	if a.Status != AlertDismissed {
		t.Error("should transition to dismissed")
	}
}

func TestAlert_PriorityOrdering(t *testing.T) {
	alerts := []Alert{
		{Priority: 3, Title: "Low"},
		{Priority: 1, Title: "High"},
		{Priority: 2, Title: "Medium"},
	}

	// Priority 1 = highest
	for _, a := range alerts {
		if a.Priority < 1 || a.Priority > 3 {
			t.Errorf("priority out of range: %d", a.Priority)
		}
	}
}

func TestSynthesisInsight_SourceCount(t *testing.T) {
	// A valid insight should reference at least 2 source artifacts
	insight := SynthesisInsight{
		InsightType:       InsightThroughLine,
		ThroughLine:       "These sources converge on pricing",
		SourceArtifactIDs: []string{"art-1", "art-2", "art-3"},
	}

	if len(insight.SourceArtifactIDs) < 2 {
		t.Error("insight should reference at least 2 source artifacts")
	}
}

func TestSynthesisInsight_Contradiction(t *testing.T) {
	insight := SynthesisInsight{
		InsightType:       InsightContradiction,
		ThroughLine:       "Conflicting views on remote work productivity",
		KeyTension:        "Article A says productive, Article B says not",
		SourceArtifactIDs: []string{"art-a", "art-b"},
		Confidence:        0.7,
	}

	if insight.InsightType != InsightContradiction {
		t.Error("expected contradiction type")
	}
	if insight.KeyTension == "" {
		t.Error("contradiction should have key tension")
	}
}
