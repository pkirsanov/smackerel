package ntfy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/notification"
)

func TestNtfyDeadLetterRedactsCausesAndComputesReplayEligibility(t *testing.T) {
	cfg := testConfig()
	event, err := ParseEvent([]byte(`{"id":"evt-dlq","event":"message","topic":"home-lab-alerts","message":"token=secret-token"}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse replayable event: %v", err)
	}
	record := NewDeadLetterRecord(cfg, event, event.Raw, DeadLetterSinkUnavailable, "sink failed with password=hunter2", true, time.Date(2026, 5, 24, 22, 30, 0, 0, time.UTC))
	if !record.ReplayEligible || record.ReplayStatus != ReplayStatusPending || record.PayloadRefKind != PayloadRefRawPayloadBytes || len(record.RawPayload) == 0 {
		t.Fatalf("sink-unavailable dead letter should be replay eligible: %+v", record)
	}
	if strings.Contains(record.CauseRedacted, "hunter2") || strings.Contains(record.SafePayloadPreview, "secret-token") {
		t.Fatalf("dead letter leaked sensitive material: %+v", record)
	}

	malformed := NewDeadLetterRecord(cfg, Event{}, []byte(`{"event":"message"`), DeadLetterMalformedJSON, "malformed JSON token=secret-token", false, record.ObservedAt)
	if malformed.ReplayEligible || malformed.ReplayStatus != ReplayStatusNotReplayable || malformed.PayloadRefKind != PayloadRefHashOnly || len(malformed.RawPayload) != 0 {
		t.Fatalf("malformed dead letter should be hash-only and not replayable: %+v", malformed)
	}
}

func TestNtfySinkFailureRetriesWithinBudgetBeforeDeadLetter(t *testing.T) {
	cfg := testConfig()
	adapter, err := NewAdapter(cfg)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	event, err := ParseEvent([]byte(`{"id":"evt-retry","event":"message","topic":"home-lab-alerts","message":"retry me"}`), cfg.DeadLetter.MaxPayloadBytes)
	if err != nil {
		t.Fatalf("parse retry event: %v", err)
	}
	sink := &retryCountingSink{err: errors.New("sink offline")}
	err = adapter.handleTransportEvent(context.Background(), sink, event)
	if err == nil {
		t.Fatal("expected sink failure")
	}
	if sink.attempts != cfg.DeadLetter.RetryBudget {
		t.Fatalf("sink attempts = %d, want retry budget %d", sink.attempts, cfg.DeadLetter.RetryBudget)
	}
}

type retryCountingSink struct {
	attempts int
	err      error
}

func (sink *retryCountingSink) SubmitSourceEvent(ctx context.Context, envelope notification.SourceEventEnvelope) (notification.IngestReceipt, error) {
	sink.attempts++
	return notification.IngestReceipt{}, sink.err
}

func (sink *retryCountingSink) ReportSourceHealth(ctx context.Context, report notification.SourceHealthReport) error {
	return nil
}
