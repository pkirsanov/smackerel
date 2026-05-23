package notification

import (
	"testing"
	"time"
)

func TestDeduperSuppressesRoutineDuplicatesWithoutDeletingHistory(t *testing.T) {
	now := time.Date(2026, 5, 22, 7, 10, 0, 0, time.UTC)
	first := testNormalizedNotification("routine-a", SeverityLow, DomainOps, IntentRoutine)
	first.PayloadHash = "payload-a"
	first.ObservedAt = now.Add(-time.Minute)
	second := first
	second.ID = "routine-b"
	second.RawEventID = "raw-b"
	second.ObservedAt = now

	suppression := NewDeduper(10*time.Minute).Evaluate(second, []NormalizedNotification{first}, "incident-a", now)
	if suppression == nil {
		t.Fatal("expected duplicate routine event suppression")
	}
	if suppression.Kind != SuppressionDedupe || suppression.NotificationID != second.ID || suppression.IncidentID != "incident-a" {
		t.Fatalf("duplicate suppression mismatch: %+v", suppression)
	}
	if !suppression.AuditPreservesRawHistory() {
		t.Fatal("duplicate suppression did not preserve raw history audit flag")
	}
}
