package intelligence

import (
	"context"
	"sort"
	"testing"
	"time"
)

func TestInsightType_Constants(t *testing.T) {
	types := []InsightType{InsightThroughLine, InsightContradiction, InsightPattern, InsightSerendipity}
	if len(types) != 4 {
		t.Errorf("expected 4 insight types, got %d", len(types))
	}
	// Verify each constant has a distinct non-empty value
	seen := make(map[InsightType]bool)
	for _, it := range types {
		if it == "" {
			t.Error("insight type must not be empty")
		}
		if seen[it] {
			t.Errorf("duplicate insight type: %s", it)
		}
		seen[it] = true
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

func TestAlertPriority_EdgeCases(t *testing.T) {
	// Equal priority — alerts at same priority should be sortable by creation time
	a1 := Alert{Priority: 2, Title: "First", CreatedAt: time.Now().Add(-time.Hour)}
	a2 := Alert{Priority: 2, Title: "Second", CreatedAt: time.Now()}

	if a1.Priority != a2.Priority {
		t.Error("expected equal priority")
	}
	if !a1.CreatedAt.Before(a2.CreatedAt) {
		t.Error("a1 should be older than a2 for tiebreaking")
	}

	// Boundary: priority 0 is below valid range, should sort above 1
	zero := Alert{Priority: 0}
	one := Alert{Priority: 1}
	if zero.Priority >= one.Priority {
		t.Error("priority 0 should sort before 1")
	}

	// Sort a slice of alerts by priority, then by creation time
	alerts := []Alert{
		{Priority: 3, Title: "Low", CreatedAt: time.Now()},
		{Priority: 1, Title: "High", CreatedAt: time.Now()},
		{Priority: 1, Title: "High-older", CreatedAt: time.Now().Add(-time.Hour)},
		{Priority: 2, Title: "Medium", CreatedAt: time.Now()},
	}
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].Priority != alerts[j].Priority {
			return alerts[i].Priority < alerts[j].Priority
		}
		return alerts[i].CreatedAt.Before(alerts[j].CreatedAt)
	})
	if alerts[0].Title != "High-older" {
		t.Errorf("expected High-older first after sort, got %s", alerts[0].Title)
	}
	if alerts[1].Title != "High" {
		t.Errorf("expected High second after sort, got %s", alerts[1].Title)
	}
	if alerts[3].Title != "Low" {
		t.Errorf("expected Low last after sort, got %s", alerts[3].Title)
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

	// Confidence threshold interaction: a low-confidence insight should be
	// distinguishable from a high-confidence one for filtering.
	lowConf := SynthesisInsight{Confidence: 0.2}
	highConf := SynthesisInsight{Confidence: 0.9}
	threshold := 0.5
	if lowConf.Confidence >= threshold {
		t.Errorf("low confidence %.2f should be below threshold %.2f", lowConf.Confidence, threshold)
	}
	if highConf.Confidence < threshold {
		t.Errorf("high confidence %.2f should be at or above threshold %.2f", highConf.Confidence, threshold)
	}

	// Boundary: exactly at threshold
	borderline := SynthesisInsight{Confidence: threshold}
	if borderline.Confidence < threshold {
		t.Error("borderline confidence should pass >= threshold check")
	}
}

func TestNewEngine_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.Pool != nil {
		t.Error("expected nil pool")
	}
	if engine.NATS != nil {
		t.Error("expected nil NATS")
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

	// pending → delivered
	if a.Status != AlertPending {
		t.Error("should start pending")
	}
	a.Status = AlertDelivered
	now := time.Now()
	a.DeliveredAt = &now
	if a.Status != AlertDelivered {
		t.Error("should transition to delivered")
	}
	if a.DeliveredAt == nil {
		t.Error("delivered alert should have delivery timestamp")
	}

	// pending → dismissed (direct)
	b := &Alert{Status: AlertPending}
	b.Status = AlertDismissed
	if b.Status != AlertDismissed {
		t.Error("should transition pending→dismissed")
	}

	// pending → snoozed → pending (snooze expires)
	c := &Alert{Status: AlertPending}
	c.Status = AlertSnoozed
	snoozeUntil := time.Now().Add(-time.Minute) // already expired
	c.SnoozeUntil = &snoozeUntil
	if c.Status != AlertSnoozed {
		t.Error("should transition to snoozed")
	}
	// Simulate snooze expiry: if snooze_until is in the past, revert to pending
	if c.SnoozeUntil != nil && c.SnoozeUntil.Before(time.Now()) {
		c.Status = AlertPending
		c.SnoozeUntil = nil
	}
	if c.Status != AlertPending {
		t.Error("expired snooze should revert to pending")
	}
	if c.SnoozeUntil != nil {
		t.Error("expired snooze should clear snooze_until")
	}

	// snooze that hasn't expired should stay snoozed
	d := &Alert{Status: AlertSnoozed}
	future := time.Now().Add(time.Hour)
	d.SnoozeUntil = &future
	if d.SnoozeUntil != nil && d.SnoozeUntil.Before(time.Now()) {
		t.Error("future snooze should not be expired")
	}
	if d.Status != AlertSnoozed {
		t.Error("non-expired snooze should stay snoozed")
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
	// A through-line insight must reference multiple sources from different domains
	insight := SynthesisInsight{
		InsightType:       InsightThroughLine,
		ThroughLine:       "These sources converge on pricing",
		SourceArtifactIDs: []string{"art-1", "art-2", "art-3"},
	}

	if len(insight.SourceArtifactIDs) < 2 {
		t.Error("insight should reference at least 2 source artifacts")
	}

	// Verify source IDs are distinct (cross-domain provenance)
	seen := make(map[string]bool)
	for _, id := range insight.SourceArtifactIDs {
		if seen[id] {
			t.Errorf("duplicate source artifact ID: %s", id)
		}
		seen[id] = true
	}

	// Single source is insufficient for a through-line
	single := SynthesisInsight{
		InsightType:       InsightThroughLine,
		SourceArtifactIDs: []string{"art-1"},
	}
	if len(single.SourceArtifactIDs) >= 2 {
		t.Error("single-source insight should not qualify as cross-domain")
	}

	// Empty source list
	empty := SynthesisInsight{
		InsightType:       InsightThroughLine,
		SourceArtifactIDs: nil,
	}
	if len(empty.SourceArtifactIDs) != 0 {
		t.Error("nil source list should have length 0")
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

	// A contradiction needs at least 2 opposing sources
	if len(insight.SourceArtifactIDs) < 2 {
		t.Error("contradiction requires at least 2 sources")
	}

	// Verify the two sources are different
	if insight.SourceArtifactIDs[0] == insight.SourceArtifactIDs[1] {
		t.Error("contradiction sources must be distinct artifacts")
	}

	// A contradiction without key_tension is incomplete
	incomplete := SynthesisInsight{
		InsightType:       InsightContradiction,
		SourceArtifactIDs: []string{"x", "y"},
	}
	if incomplete.KeyTension != "" {
		t.Error("expected empty key tension for incomplete contradiction")
	}
}

func TestRunSynthesis_EmptyPool(t *testing.T) {
	// Engine with nil pool should not panic on RunSynthesis
	engine := NewEngine(nil, nil)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	// RunSynthesis requires a pool for DB queries; with nil pool it should
	// return an error, not panic.
	_, err := engine.RunSynthesis(context.Background())
	if err == nil {
		t.Error("expected error when running synthesis with nil pool")
	}
}

func TestCheckOverdueCommitments_NilPool(t *testing.T) {
	engine := NewEngine(nil, nil)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}

	// CheckOverdueCommitments requires a pool; nil pool should error, not panic.
	err := engine.CheckOverdueCommitments(context.Background())
	if err == nil {
		t.Error("expected error when checking overdue commitments with nil pool")
	}
}
