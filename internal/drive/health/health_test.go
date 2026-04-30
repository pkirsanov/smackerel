package health

import (
	"errors"
	"testing"

	"github.com/smackerel/smackerel/internal/drive"
)

func TestProviderErrorsTransitionHealthAndPreserveRetryableWork(t *testing.T) {
	tracker := NewTracker(Policy{DegradedAfter: 1, FailingAfter: 3})
	connectionID := "conn-health-unit"
	providerError := errors.New("fixture provider returned 429 rate limit")

	first := tracker.RecordProviderError(connectionID, "scan", providerError)
	if first.Status != drive.HealthDegraded {
		t.Fatalf("first error status = %s, want degraded", first.Status)
	}
	if first.RetryableWorkCount != 1 {
		t.Fatalf("first RetryableWorkCount = %d, want 1", first.RetryableWorkCount)
	}
	if len(tracker.RetryableWork(connectionID)) != 1 {
		t.Fatalf("retryable work not preserved after first error")
	}

	second := tracker.RecordProviderError(connectionID, "monitor", providerError)
	if second.Status != drive.HealthDegraded {
		t.Fatalf("second error status = %s, want degraded before failing threshold", second.Status)
	}
	third := tracker.RecordProviderError(connectionID, "retrieve", providerError)
	if third.Status != drive.HealthFailing {
		t.Fatalf("third error status = %s, want failing", third.Status)
	}
	if third.RetryableWorkCount != 3 {
		t.Fatalf("third RetryableWorkCount = %d, want 3 queued attempts", third.RetryableWorkCount)
	}

	work := tracker.RetryableWork(connectionID)
	if gotTypes := []string{work[0].WorkType, work[1].WorkType, work[2].WorkType}; gotTypes[0] != "scan" || gotTypes[1] != "monitor" || gotTypes[2] != "retrieve" {
		t.Fatalf("retryable work types = %v, want [scan monitor retrieve]", gotTypes)
	}
	if work[0].LastError == "" || work[1].LastError == "" || work[2].LastError == "" {
		t.Fatalf("retryable work must preserve provider error text: %+v", work)
	}

	recovered := tracker.RecordProviderSuccess(connectionID)
	if recovered.Status != drive.HealthHealthy {
		t.Fatalf("success status = %s, want healthy", recovered.Status)
	}
	if len(tracker.RetryableWork(connectionID)) != 3 {
		t.Fatalf("successful health probe must not silently drop queued work")
	}
}
