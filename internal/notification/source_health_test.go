package notification

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSourceHealthStatesRedactErrorsAndTrackRetryCounts(t *testing.T) {
	now := time.Date(2026, 5, 22, 6, 10, 0, 0, time.UTC)
	lastEvent := now.Add(-2 * time.Minute)
	lastCheck := now.Add(-1 * time.Minute)
	store := &recordingHealthStore{}
	service := NewHealthService(store)

	reports := []SourceHealthReport{
		{SourceType: "stream_fixture", SourceInstanceID: "connected-a", SourceForm: SourceFormStream, State: SourceHealthConnected, LastEventAt: &lastEvent, LastSuccessfulCheckAt: &lastCheck, ObservedAt: now},
		{SourceType: "webhook_fixture", SourceInstanceID: "invalid-a", SourceForm: SourceFormWebhook, State: SourceHealthDisconnected, RetryCount: 1, LastErrorKind: "invalid_credentials", LastErrorRedacted: "token secret-token-123 rejected", ObservedAt: now},
		{SourceType: "polling_fixture", SourceInstanceID: "transient-a", SourceForm: SourceFormPolling, State: SourceHealthDegraded, LastEventAt: &lastEvent, RetryCount: 4, LastErrorKind: "transient_failure", LastErrorRedacted: "upstream says password=hunter2", ObservedAt: now},
	}
	for _, report := range reports {
		if err := service.Report(context.Background(), report); err != nil {
			t.Fatalf("record health for %s: %v", report.SourceInstanceID, err)
		}
	}
	if len(store.reports) != 3 {
		t.Fatalf("recorded reports = %d, want 3", len(store.reports))
	}
	connected := store.byInstance("connected-a")
	if connected.State != SourceHealthConnected || connected.LastEventAt == nil || connected.LastSuccessfulCheckAt == nil {
		t.Fatalf("connected health did not preserve timestamps: %+v", connected)
	}
	invalid := store.byInstance("invalid-a")
	if invalid.State != SourceHealthDisconnected || invalid.RetryCount != 1 || invalid.LastErrorKind != "invalid_credentials" {
		t.Fatalf("invalid credentials health mismatch: %+v", invalid)
	}
	if strings.Contains(invalid.LastErrorRedacted, "secret-token") || invalid.LastErrorRedacted != "source authentication failed" {
		t.Fatalf("invalid credentials error was not redacted: %q", invalid.LastErrorRedacted)
	}
	transient := store.byInstance("transient-a")
	if transient.State != SourceHealthDegraded || transient.RetryCount != 4 || transient.LastErrorKind != "transient_failure" {
		t.Fatalf("transient health mismatch: %+v", transient)
	}
	if strings.Contains(transient.LastErrorRedacted, "hunter2") || transient.LastErrorRedacted != "transient source check failed" {
		t.Fatalf("transient error was not redacted: %q", transient.LastErrorRedacted)
	}
}

type recordingHealthStore struct {
	reports []SourceHealthReport
}

func (s *recordingHealthStore) RecordSourceHealth(ctx context.Context, report SourceHealthReport) error {
	s.reports = append(s.reports, report)
	return nil
}

func (s *recordingHealthStore) byInstance(instanceID string) SourceHealthReport {
	for _, report := range s.reports {
		if report.SourceInstanceID == instanceID {
			return report
		}
	}
	return SourceHealthReport{}
}
