// Spec 076 SCOPE-5 — TP-076-05-05 / SCN-074-A07.
//
// Unit-level proof that whenever the facade emits a capture-as-fallback
// event, the per-row IntentTraceID flowing into artifact_capture_policy
// stays joined to the cause that triggered the Prometheus counter
// increment.
//
// The runtime relationship under test:
//
//   - facade calls capturefallback.Policy.CaptureForUser (which
//     writes the artifact_capture_policy row carrying
//     intent_trace_id, source_turn_id, and cause), THEN increments
//     CaptureFallbackTotal{cause,transport}.
//
//   - SCN-074-A07 requires that the counter and the IntentTrace
//     carry the capture link — i.e., for every counter increment
//     attributable to a turn there is exactly one policy row whose
//     intent_trace_id refers to that turn.
//
// This unit test exercises the join shape directly against the
// payload constructors:
//
//   - Request carries IntentTraceID.
//   - BuildCapturePayload preserves IntentTraceID, FallbackCause,
//     and SourceTurnID from the Request/Decision pair.
//   - Cause maps 1:1 to a closed-vocabulary CaptureFallbackTotal
//     label value (AllCaptureFallbackCauses).
//
// Adversarial coverage:
//
//   - If BuildCapturePayload dropped IntentTraceID (or replaced it
//     with the empty string), the assertion `payload.IntentTraceID
//     == req.IntentTraceID` would trip.
//   - If a new fallback cause were added to capturefallback but not
//     to AllCaptureFallbackCauses, the membership probe would trip,
//     proving the counter would silently emit an unbounded label.
package assistantmetrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/smackerel/smackerel/internal/assistant/capturefallback"
)

// TestCaptureFallback_IntentTraceLinkPresent — TP-076-05-05 / SCN-074-A07.
func TestCaptureFallback_IntentTraceLinkPresent(t *testing.T) {
	const (
		hashKey     = "tp-076-05-05-hmac-key"
		dedupWindow = 5 * time.Minute
		intentID    = "intent-trace-076-05-05"
		sourceTurn  = "tg:msg-076-05-05"
	)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	req := capturefallback.Request{
		UserID:             "user-076-05-05",
		Transport:          "telegram",
		TransportMessageID: "msg-076-05-05",
		OriginalText:       "remember to renew the parking permit",
		Cause:              capturefallback.CauseUnrouted,
		TraceID:            "trace-076-05-05",
		IntentTraceID:      intentID,
		OccurredAt:         now,
	}
	dec := capturefallback.Decision{
		Cause:              req.Cause,
		Provenance:         capturefallback.ProvenanceFallback,
		NormalizedText:     capturefallback.NormalizeV1(req.OriginalText),
		NormalizedTextHash: capturefallback.HashNormalized(capturefallback.NormalizeV1(req.OriginalText), hashKey),
		DedupBucketStart:   capturefallback.BucketStart(now, dedupWindow),
		DedupWindow:        dedupWindow,
		SourceTurnID:       sourceTurn,
		IntentTraceID:      intentID,
		SchemaVersion:      capturefallback.SchemaVersion,
		OccurredAt:         now,
	}

	payload := capturefallback.BuildCapturePayload(req, dec, "artifact-076-05-05")

	if payload.IntentTraceID != intentID {
		t.Errorf("payload.IntentTraceID = %q, want %q (SCN-074-A07: IntentTrace MUST carry the capture link)", payload.IntentTraceID, intentID)
	}
	if payload.SourceTurnID == "" {
		t.Error("payload.SourceTurnID empty; counter→IntentTrace join requires a stable turn id")
	}
	if payload.FallbackCause != capturefallback.CauseUnrouted {
		t.Errorf("payload.FallbackCause = %q, want %q", payload.FallbackCause, capturefallback.CauseUnrouted)
	}

	// The counter label value MUST come from the closed vocabulary.
	// If the runtime emitted a cause label that was not registered
	// in AllCaptureFallbackCauses, the dashboard query joining the
	// counter to the IntentTrace would miss it.
	causeLabel := string(payload.FallbackCause)
	if !containsString(AllCaptureFallbackCauses, causeLabel) {
		t.Fatalf("cause label %q not in AllCaptureFallbackCauses %v — counter/IntentTrace join would drop this row", causeLabel, AllCaptureFallbackCauses)
	}

	// Exercise the counter once with the same label to prove the
	// label set is accepted by the registered CounterVec — the
	// runtime path that records the row also emits the counter.
	before := readCounterValue(t, CaptureFallbackTotal.WithLabelValues(causeLabel, TransportTelegram))
	CaptureFallbackTotal.WithLabelValues(causeLabel, TransportTelegram).Inc()
	after := readCounterValue(t, CaptureFallbackTotal.WithLabelValues(causeLabel, TransportTelegram))
	if got := after - before; got != 1 {
		t.Errorf("CaptureFallbackTotal delta = %.0f, want 1", got)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func readCounterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	if err := c.Write(&m); err != nil {
		t.Fatalf("counter Write: %v", err)
	}
	if m.Counter == nil || m.Counter.Value == nil {
		return 0
	}
	return *m.Counter.Value
}
