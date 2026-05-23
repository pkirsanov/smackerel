package qfdecisions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// Scope 6 unit tests (SCN-SM-041-022..024). Each test exercises the
// Exporter against an in-process fake transport so the failure matrix,
// retry budget, capability gate, consent gate, idempotency contract,
// and overflow drop policy are proven without live-stack dependencies.

// --- Test fixtures -----------------------------------------------------

// fakeTransport is an in-memory EngagementTransport used by the unit
// tests. It captures every batch the Exporter posts, lets the test
// pre-program per-attempt responses (HTTP 201 by default, 4xx, 5xx, or
// transport timeout), and records the call count so retry behavior can
// be asserted by attempt index.
type fakeTransport struct {
	mu        sync.Mutex
	batches   [][]PacketEngagementSignal
	attempts  int
	responder func(attempt int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error)
	signalIDs [][]string
}

func newFakeTransport(responder func(attempt int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error)) *fakeTransport {
	if responder == nil {
		responder = func(_ int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
			results := make([]EngagementSignalResult, 0, len(signals))
			for _, s := range signals {
				results = append(results, EngagementSignalResult{SignalID: s.SignalID, StatusCode: http.StatusCreated})
			}
			return results, nil
		}
	}
	return &fakeTransport{responder: responder}
}

func (f *fakeTransport) PostEngagementSignals(_ context.Context, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
	f.mu.Lock()
	f.attempts++
	attempt := f.attempts
	batchCopy := make([]PacketEngagementSignal, len(signals))
	copy(batchCopy, signals)
	f.batches = append(f.batches, batchCopy)
	ids := make([]string, len(signals))
	for i, s := range signals {
		ids[i] = s.SignalID
	}
	f.signalIDs = append(f.signalIDs, ids)
	responder := f.responder
	f.mu.Unlock()
	return responder(attempt, batchCopy)
}

func (f *fakeTransport) AttemptCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attempts
}

func (f *fakeTransport) Batches() [][]PacketEngagementSignal {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]PacketEngagementSignal, len(f.batches))
	for i, b := range f.batches {
		cp := make([]PacketEngagementSignal, len(b))
		copy(cp, b)
		out[i] = cp
	}
	return out
}

// staticConsent returns a fixed engagement_telemetry preference.
type staticConsent struct {
	value string
}

func (s staticConsent) EngagementTelemetryPreference(context.Context) string {
	return s.value
}

// sequenceUUID hands out pre-generated UUIDv7 strings in order so the
// idempotency-replay test can compare the same signal id across two
// flush attempts. It panics on overflow to surface accidental misuse.
type sequenceUUID struct {
	mu  sync.Mutex
	ids []string
	idx int
}

func newSequenceUUID(t *testing.T, n int) *sequenceUUID {
	t.Helper()
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		id, err := uuid.NewV7()
		if err != nil {
			t.Fatalf("seed uuid v7: %v", err)
		}
		ids[i] = id.String()
	}
	return &sequenceUUID{ids: ids}
}

func (s *sequenceUUID) Next() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx >= len(s.ids) {
		return "", fmt.Errorf("sequenceUUID exhausted at idx=%d (len=%d)", s.idx, len(s.ids))
	}
	id := s.ids[s.idx]
	s.idx++
	return id, nil
}

// metricSample reads the current value of
// smackerel_qf_engagement_signal_attempts_total for the supplied label
// triple. Returns 0 when the label combination has never been observed.
func metricSample(t *testing.T, event, surface, status string) float64 {
	t.Helper()
	return testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(event, surface, status))
}

// fixedNow returns a deterministic clock for engagement_ts assertions.
func fixedNow() time.Time {
	return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
}

// --- Tests ------------------------------------------------------------

// TestEngagementExporterCapturesAllSixEventTypesAcrossWebDigestAndTelegramSurfaces
// (SCN-SM-041-022) drives the 6 × 3 (event × surface) capture matrix
// and asserts:
//
//   - every captured signal carries the documented envelope fields
//     (signal_id, packet_id, trace_id, surface, consent_scope,
//     engagement_event, engagement_ts, actor_ref);
//   - `dwell` events carry a non-nil dwell_seconds; every other event
//     type omits dwell_seconds from the JSON encoding;
//   - the buffer collected 18 signals total (6 events × 3 surfaces)
//     when consent is `anonymous`;
//   - the same matrix collects ZERO signals when capability is false
//     (the exporter is permanently disabled).
//
// Adversarial trip-wire: when the test drives a `dwell` event the
// resulting JSON MUST include `"dwell_seconds":N`; when it drives an
// `opened` event the JSON MUST NOT contain the `dwell_seconds` key.
// A regression that emits a zero dwell on non-dwell events would fail
// the JSON-shape assertion.
func TestEngagementExporterCapturesAllSixEventTypesAcrossWebDigestAndTelegramSurfaces(t *testing.T) {
	transport := newFakeTransport(nil)
	exporter := NewExporter(ExporterOptions{
		Transport:                 transport,
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: true,
		Clock:                     fixedNow,
	})
	if !exporter.Enabled() {
		t.Fatalf("Exporter should be enabled with capability=true")
	}

	events := []string{
		EngagementEventOpened,
		EngagementEventDwell,
		EngagementEventDismissed,
		EngagementEventSnoozed,
		EngagementEventDeepLinked,
		EngagementEventShared,
	}
	surfaces := []string{SurfaceWeb, SurfaceDigest, SurfaceTelegram}

	var captured []PacketEngagementSignal
	for _, event := range events {
		for _, surface := range surfaces {
			req := CaptureRequest{
				Event:    event,
				Surface:  surface,
				PacketID: "pkt-" + event + "-" + surface,
				TraceID:  "trace-" + event + "-" + surface,
				ActorRef: "actor-" + surface,
			}
			if event == EngagementEventDwell {
				d := 17
				req.DwellSeconds = &d
			}
			sig, ok := exporter.Capture(context.Background(), req)
			if !ok {
				t.Fatalf("Capture(%s,%s) reported no enqueue but capability and consent both allow it", event, surface)
			}
			captured = append(captured, sig)
		}
	}

	if got, want := exporter.BufferLen(), len(events)*len(surfaces); got != want {
		t.Fatalf("BufferLen after capture matrix = %d, want %d", got, want)
	}

	// Envelope field assertions (per design.md §Signal Envelope).
	for _, sig := range captured {
		if sig.SignalID == "" {
			t.Errorf("captured signal missing signal_id: %#v", sig)
		}
		if sig.PacketID == "" || sig.TraceID == "" {
			t.Errorf("captured signal missing packet/trace id: %#v", sig)
		}
		if sig.Surface == "" || sig.EngagementEvent == "" {
			t.Errorf("captured signal missing event/surface: %#v", sig)
		}
		if sig.ConsentScope != EngagementConsentAnonymous {
			t.Errorf("captured signal consent_scope = %q, want %q", sig.ConsentScope, EngagementConsentAnonymous)
		}
		if !sig.EngagementTS.Equal(fixedNow()) {
			t.Errorf("captured signal engagement_ts = %s, want %s", sig.EngagementTS, fixedNow())
		}

		encoded, err := json.Marshal(sig)
		if err != nil {
			t.Fatalf("marshal captured signal: %v", err)
		}
		body := string(encoded)
		switch sig.EngagementEvent {
		case EngagementEventDwell:
			if sig.DwellSeconds == nil || *sig.DwellSeconds != 17 {
				t.Errorf("dwell signal missing dwell_seconds=17: %#v", sig)
			}
			if !contains(body, `"dwell_seconds":17`) {
				t.Errorf("dwell JSON missing dwell_seconds key: %s", body)
			}
		default:
			if sig.DwellSeconds != nil {
				t.Errorf("non-dwell event %q has dwell_seconds set: %#v", sig.EngagementEvent, sig)
			}
			if contains(body, `"dwell_seconds"`) {
				t.Errorf("non-dwell JSON leaked dwell_seconds key: %s", body)
			}
		}
	}

	// Re-run the matrix with capability=false and assert ZERO captures.
	disabled := NewExporter(ExporterOptions{
		Transport:                 newFakeTransport(nil),
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: false,
	})
	if disabled.Enabled() {
		t.Fatalf("Exporter MUST be disabled when EngagementSignalSupported=false")
	}
	for _, event := range events {
		for _, surface := range surfaces {
			if _, ok := disabled.Capture(context.Background(), CaptureRequest{
				Event: event, Surface: surface, PacketID: "x", TraceID: "y",
			}); ok {
				t.Fatalf("capability=false should bypass Capture; event=%s surface=%s enqueued", event, surface)
			}
		}
	}
	if got := disabled.BufferLen(); got != 0 {
		t.Fatalf("capability=false buffer length = %d, want 0", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestEngagementExporterHonorsConsentGateAndCapabilityGate
// (SCN-SM-041-022) verifies both gates independently:
//
//   - consent=off bypasses Capture at event-capture time regardless of
//     which surface fires the event;
//   - consent=anonymous/pseudonymous enqueues the event with the
//     canonical `consent_scope` value in the envelope;
//   - capability=false constructs a disabled Exporter whose buffer is
//     never allocated (BufferLen=0 even after Capture attempts).
//
// Adversarial trip-wire: a regression that read the privacy preference
// at FLUSH time instead of CAPTURE time would let an event slip into
// the buffer with consent=anonymous, then drop it at flush. The test
// asserts BufferLen=0 immediately after Capture, so the bypass must
// happen at capture time.
func TestEngagementExporterHonorsConsentGateAndCapabilityGate(t *testing.T) {
	// consent=off — buffer untouched.
	off := NewExporter(ExporterOptions{
		Transport:                 newFakeTransport(nil),
		Consent:                   staticConsent{value: EngagementConsentRawOff},
		EngagementSignalSupported: true,
	})
	for _, surface := range []string{SurfaceWeb, SurfaceDigest, SurfaceTelegram} {
		sig, ok := off.Capture(context.Background(), CaptureRequest{
			Event: EngagementEventOpened, Surface: surface,
			PacketID: "pkt", TraceID: "trace", ActorRef: "actor",
		})
		if ok {
			t.Errorf("consent=off MUST bypass Capture on surface %s, got enqueued sig %#v", surface, sig)
		}
	}
	if got := off.BufferLen(); got != 0 {
		t.Fatalf("consent=off buffer length = %d, want 0 (bypass MUST happen at capture)", got)
	}

	// consent=pseudonymous — canonical envelope value preserved.
	pseud := NewExporter(ExporterOptions{
		Transport:                 newFakeTransport(nil),
		Consent:                   staticConsent{value: EngagementConsentRawPseudonym},
		EngagementSignalSupported: true,
	})
	sig, ok := pseud.Capture(context.Background(), CaptureRequest{
		Event: EngagementEventOpened, Surface: SurfaceWeb,
		PacketID: "pkt", TraceID: "trace", ActorRef: "actor",
	})
	if !ok {
		t.Fatalf("consent=pseudonymous MUST enqueue")
	}
	if sig.ConsentScope != EngagementConsentPseudonymous {
		t.Fatalf("consent_scope = %q, want %q", sig.ConsentScope, EngagementConsentPseudonymous)
	}

	// capability=false — no buffer allocation, no enqueue.
	disabled := NewExporter(ExporterOptions{
		Transport:                 newFakeTransport(nil),
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: false,
	})
	if disabled.Enabled() {
		t.Fatalf("Exporter MUST be disabled when EngagementSignalSupported=false")
	}
	if _, ok := disabled.Capture(context.Background(), CaptureRequest{
		Event: EngagementEventOpened, Surface: SurfaceWeb, PacketID: "x", TraceID: "y",
	}); ok {
		t.Fatalf("capability=false MUST bypass Capture")
	}
	if got := disabled.BufferLen(); got != 0 {
		t.Fatalf("capability=false buffer length = %d, want 0", got)
	}

	// Unknown consent values fall through to off (fail-closed).
	unknown := NewExporter(ExporterOptions{
		Transport:                 newFakeTransport(nil),
		Consent:                   staticConsent{value: "garbled-value"},
		EngagementSignalSupported: true,
	})
	if _, ok := unknown.Capture(context.Background(), CaptureRequest{
		Event: EngagementEventOpened, Surface: SurfaceWeb, PacketID: "x", TraceID: "y",
	}); ok {
		t.Fatalf("unknown consent value MUST fall through to off (fail-closed)")
	}
}

// TestEngagementExporterFlushesOnTenSecondTimerAndOnHundredEventThreshold
// (SCN-SM-041-023) exercises both flush triggers:
//
//   - threshold trigger: enqueue exactly 100 events (the flush
//     threshold) and assert the flush worker drains the buffer to a
//     single attempt.
//   - timer trigger: enqueue 1 event (well below the threshold),
//     advance the flush timer by the configured interval, and assert
//     the flush worker drains the single event.
//
// The test uses a short FlushInterval (50ms) so the timer triggers
// quickly. The 10-second production interval is preserved as the
// design contract; the test exercises the same code path with a
// faster cadence for hermetic execution.
//
// Adversarial trip-wire: a regression that swapped 100 for 10 or 1000
// in the threshold check would leave 100 events in the buffer without
// triggering a flush; the test asserts the buffer empties to zero
// within a bounded waiter.
func TestEngagementExporterFlushesOnTenSecondTimerAndOnHundredEventThreshold(t *testing.T) {
	// Sub-case 1: 100-event threshold trigger fires immediately on the
	// 100th Capture, without waiting for the timer.
	thresholdCh := make(chan struct{}, 1)
	thresholdTransport := newFakeTransport(func(_ int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
		// Notify the test that the flush worker drained a batch; the
		// channel is buffered so the responder never blocks.
		select {
		case thresholdCh <- struct{}{}:
		default:
		}
		results := make([]EngagementSignalResult, 0, len(signals))
		for _, s := range signals {
			results = append(results, EngagementSignalResult{SignalID: s.SignalID, StatusCode: http.StatusCreated})
		}
		return results, nil
	})
	thresholdExporter := NewExporter(ExporterOptions{
		Transport:                 thresholdTransport,
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: true,
		FlushInterval:             10 * time.Second, // far exceeds the test timeout, so only threshold can trigger
		FlushThreshold:            100,
		Capacity:                  1024,
	})
	thresholdExporter.Start(context.Background())
	defer thresholdExporter.Stop(context.Background())
	for i := 0; i < 100; i++ {
		if _, ok := thresholdExporter.Capture(context.Background(), CaptureRequest{
			Event: EngagementEventOpened, Surface: SurfaceWeb,
			PacketID: "pkt-thr-" + strconv.Itoa(i),
			TraceID:  "trc-thr-" + strconv.Itoa(i),
			ActorRef: "actor",
		}); !ok {
			t.Fatalf("threshold-trigger enqueue %d failed", i)
		}
	}
	select {
	case <-thresholdCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("flush worker did NOT drain after 100-event threshold; attempts=%d buffer=%d",
			thresholdTransport.AttemptCount(), thresholdExporter.BufferLen())
	}
	// After threshold flush, the buffer MUST be empty.
	waitFor(t, "threshold buffer drained", 2*time.Second, func() bool {
		return thresholdExporter.BufferLen() == 0
	})

	// Sub-case 2: timer trigger fires after the configured interval
	// even when the buffer holds 1 event (well below threshold).
	timerCh := make(chan int, 8)
	timerTransport := newFakeTransport(func(_ int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
		select {
		case timerCh <- len(signals):
		default:
		}
		results := make([]EngagementSignalResult, 0, len(signals))
		for _, s := range signals {
			results = append(results, EngagementSignalResult{SignalID: s.SignalID, StatusCode: http.StatusCreated})
		}
		return results, nil
	})
	timerExporter := NewExporter(ExporterOptions{
		Transport:                 timerTransport,
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: true,
		FlushInterval:             50 * time.Millisecond,
		FlushThreshold:            100,
	})
	timerExporter.Start(context.Background())
	defer timerExporter.Stop(context.Background())
	if _, ok := timerExporter.Capture(context.Background(), CaptureRequest{
		Event: EngagementEventOpened, Surface: SurfaceDigest,
		PacketID: "pkt-tmr-1", TraceID: "trc-tmr-1", ActorRef: "actor",
	}); !ok {
		t.Fatalf("timer-trigger enqueue failed")
	}
	select {
	case size := <-timerCh:
		if size != 1 {
			t.Fatalf("timer-trigger flush batch size = %d, want 1", size)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("flush worker did NOT drain after timer interval; attempts=%d buffer=%d",
			timerTransport.AttemptCount(), timerExporter.BufferLen())
	}
}

// TestEngagementSignalIDIsUUIDv7AndIdempotentAcrossRepeatedFlushAttempt
// (SCN-SM-041-023) drives:
//
//   - signal_id is a parseable UUIDv7 (version field == 7);
//   - when the QF replies HTTP 200 idempotent on a repeat POST, the
//     Exporter increments the `accepted` metric but does NOT emit a
//     duplicate audit envelope.
//
// Adversarial trip-wire: a regression that used UUIDv4 instead of v7
// would still parse as a UUID; the explicit version check catches it.
// A regression that emitted the audit envelope on the idempotent
// repeat would leave the metric and audit envelope counts unequal.
func TestEngagementSignalIDIsUUIDv7AndIdempotentAcrossRepeatedFlushAttempt(t *testing.T) {
	// Part 1: UUIDv7 version field.
	exporter := NewExporter(ExporterOptions{
		Transport:                 newFakeTransport(nil),
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: true,
		Clock:                     fixedNow,
	})
	sig, ok := exporter.Capture(context.Background(), CaptureRequest{
		Event: EngagementEventOpened, Surface: SurfaceWeb,
		PacketID: "pkt-uuidv7", TraceID: "trc-uuidv7", ActorRef: "actor",
	})
	if !ok {
		t.Fatalf("Capture failed")
	}
	parsed, err := uuid.Parse(sig.SignalID)
	if err != nil {
		t.Fatalf("signal_id is not a valid UUID: %v", err)
	}
	if got := parsed.Version(); got != uuid.Version(7) {
		t.Fatalf("signal_id UUID version = %d, want 7 (UUIDv7)", got)
	}

	// Part 2: idempotent-repeat MUST NOT emit a duplicate audit envelope.
	// We drive two flush attempts against a transport that returns:
	//   attempt 1 → HTTP 201 accepted
	//   attempt 2 → HTTP 200 idempotent (same signal_id and body)
	// then assert audit envelope emission count == 1 (one for the
	// accepted attempt; zero for the idempotent repeat).
	captured := newCapturedAuditSink(t)
	defer captured.restore()

	transport := newFakeTransport(func(attempt int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
		results := make([]EngagementSignalResult, 0, len(signals))
		for _, s := range signals {
			r := EngagementSignalResult{SignalID: s.SignalID}
			if attempt == 1 {
				r.StatusCode = http.StatusCreated
			} else {
				r.StatusCode = http.StatusOK
				r.Idempotent = true
			}
			results = append(results, r)
		}
		return results, nil
	})

	idemExporter := NewExporter(ExporterOptions{
		Transport:                 transport,
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: true,
		Clock:                     fixedNow,
	})

	beforeAccept := metricSample(t, EngagementEventOpened, SurfaceWeb, EngagementStatusAccepted)

	enqueued, ok := idemExporter.Capture(context.Background(), CaptureRequest{
		Event: EngagementEventOpened, Surface: SurfaceWeb,
		PacketID: "pkt-idem", TraceID: "trc-idem", ActorRef: "actor-idem",
	})
	if !ok {
		t.Fatalf("idempotent Capture failed")
	}
	captured.reset()
	idemExporter.Flush(context.Background())
	auditAfterAccept := captured.envelopesForAction(AuditActionEngagementSignalFlush)
	if len(auditAfterAccept) != 1 {
		t.Fatalf("first flush audit count = %d, want 1", len(auditAfterAccept))
	}
	if auditAfterAccept[0].Outcome != AuditOutcomeOK {
		t.Fatalf("first flush outcome = %q, want %q", auditAfterAccept[0].Outcome, AuditOutcomeOK)
	}
	if auditAfterAccept[0].SignalID != enqueued.SignalID {
		t.Fatalf("first flush signal_id = %q, want %q", auditAfterAccept[0].SignalID, enqueued.SignalID)
	}

	// Re-enqueue the SAME signal (same body) and flush. The transport
	// replies HTTP 200 idempotent. The metric MUST increment but the
	// audit envelope MUST NOT emit a duplicate record.
	captured.reset()
	idemExporter.mu.Lock()
	idemExporter.buffer = append(idemExporter.buffer, enqueued)
	idemExporter.mu.Unlock()

	idemExporter.Flush(context.Background())
	auditAfterIdem := captured.envelopesForAction(AuditActionEngagementSignalFlush)
	if len(auditAfterIdem) != 0 {
		t.Fatalf("idempotent repeat MUST NOT emit duplicate audit envelope; got %d entries: %#v",
			len(auditAfterIdem), auditAfterIdem)
	}

	afterIdem := metricSample(t, EngagementEventOpened, SurfaceWeb, EngagementStatusAccepted)
	if afterIdem-beforeAccept < 2 {
		t.Fatalf("accepted metric delta = %f, want >= 2 (one accept + one idempotent confirm)",
			afterIdem-beforeAccept)
	}
}

// TestEngagementExporterDropsFourXXWithoutRetryAndRecordsRejectedMetricAndAuditEnvelope
// (SCN-SM-041-024) drives every documented 4xx reason code through the
// exporter and asserts:
//
//   - the transport receives exactly ONE attempt per batch (no retry);
//   - the per-signal metric increments status="rejected";
//   - the audit envelope outcome=rejected with `reason` carrying the
//     QF error code verbatim;
//   - the signal is dropped (BufferLen=0 after Flush returns).
//
// Adversarial trip-wire: a regression that treated 4xx as retryable
// would leave attempts > 1; the test asserts AttemptCount == 1 after
// Flush completes.
func TestEngagementExporterDropsFourXXWithoutRetryAndRecordsRejectedMetricAndAuditEnvelope(t *testing.T) {
	cases := []struct {
		status int
		reason string
	}{
		{http.StatusConflict, EngagementErrSignalIDReuseDifferentPayload},
		{http.StatusBadRequest, EngagementErrPacketNotFound},
		{http.StatusBadRequest, EngagementErrTraceIDMismatch},
		{http.StatusForbidden, EngagementErrConsentRequired},
		{http.StatusBadRequest, EngagementErrDwellFieldMismatch},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.reason, func(t *testing.T) {
			captured := newCapturedAuditSink(t)
			defer captured.restore()

			transport := newFakeTransport(func(_ int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
				results := make([]EngagementSignalResult, 0, len(signals))
				for _, s := range signals {
					results = append(results, EngagementSignalResult{
						SignalID:   s.SignalID,
						StatusCode: tc.status,
						Reason:     tc.reason,
					})
				}
				return results, nil
			})

			exporter := NewExporter(ExporterOptions{
				Transport:                 transport,
				Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
				EngagementSignalSupported: true,
				Clock:                     fixedNow,
				MaxAttempts:               3,
			})
			beforeRejected := metricSample(t, EngagementEventOpened, SurfaceWeb, EngagementStatusRejected)

			if _, ok := exporter.Capture(context.Background(), CaptureRequest{
				Event: EngagementEventOpened, Surface: SurfaceWeb,
				PacketID: "pkt-4xx", TraceID: "trc-4xx", ActorRef: "actor-4xx",
			}); !ok {
				t.Fatalf("Capture failed")
			}
			captured.reset()
			exporter.Flush(context.Background())

			if got := transport.AttemptCount(); got != 1 {
				t.Fatalf("4xx %s (%d) MUST drop without retry; AttemptCount=%d, want 1",
					tc.reason, tc.status, got)
			}
			if got := exporter.BufferLen(); got != 0 {
				t.Fatalf("buffer length after 4xx drop = %d, want 0", got)
			}
			afterRejected := metricSample(t, EngagementEventOpened, SurfaceWeb, EngagementStatusRejected)
			if delta := afterRejected - beforeRejected; delta < 1 {
				t.Fatalf("rejected metric delta = %f, want >= 1", delta)
			}
			audits := captured.envelopesForAction(AuditActionEngagementSignalFlush)
			if len(audits) != 1 {
				t.Fatalf("audit envelope count = %d, want 1", len(audits))
			}
			if audits[0].Outcome != AuditOutcomeRejected {
				t.Fatalf("audit outcome = %q, want %q", audits[0].Outcome, AuditOutcomeRejected)
			}
			if audits[0].Reason != tc.reason {
				t.Fatalf("audit reason = %q, want %q (QF error code preserved verbatim)",
					audits[0].Reason, tc.reason)
			}
		})
	}
}

// TestEngagementExporterRetriesFiveXXWithBoundedBackoffUpToThreeAttemptsThenDrops
// (SCN-SM-041-024) drives a transport that always replies HTTP 500
// and asserts:
//
//   - the transport receives exactly MaxAttempts attempts;
//   - the final-attempt drop emits status="degraded" metric;
//   - the audit envelope outcome=degraded with reason populated;
//   - transport timeouts (Retryable + no StatusCode) follow the same
//     retry policy.
//
// Adversarial trip-wire: a regression that retried infinitely or
// stopped after 1 attempt would fail the AttemptCount equality check.
func TestEngagementExporterRetriesFiveXXWithBoundedBackoffUpToThreeAttemptsThenDrops(t *testing.T) {
	t.Run("HTTP_500_retries_three_times_then_drops_degraded", func(t *testing.T) {
		captured := newCapturedAuditSink(t)
		defer captured.restore()

		transport := newFakeTransport(func(_ int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
			results := make([]EngagementSignalResult, 0, len(signals))
			for _, s := range signals {
				results = append(results, EngagementSignalResult{
					SignalID:   s.SignalID,
					StatusCode: http.StatusInternalServerError,
					Reason:     "INTERNAL_ERROR",
					Retryable:  true,
				})
			}
			return results, nil
		})

		exporter := NewExporter(ExporterOptions{
			Transport:                 transport,
			Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
			EngagementSignalSupported: true,
			Clock:                     fixedNow,
			MaxAttempts:               3,
			InitialBackoff:            1 * time.Millisecond,
			MaxBackoff:                2 * time.Millisecond,
		})
		beforeDegraded := metricSample(t, EngagementEventOpened, SurfaceWeb, EngagementStatusDegraded)

		if _, ok := exporter.Capture(context.Background(), CaptureRequest{
			Event: EngagementEventOpened, Surface: SurfaceWeb,
			PacketID: "pkt-500", TraceID: "trc-500", ActorRef: "actor-500",
		}); !ok {
			t.Fatalf("Capture failed")
		}
		captured.reset()
		exporter.Flush(context.Background())

		if got := transport.AttemptCount(); got != 3 {
			t.Fatalf("HTTP 500 attempt count = %d, want exactly 3 (bounded retry)", got)
		}
		if got := exporter.BufferLen(); got != 0 {
			t.Fatalf("buffer length after retry exhaustion = %d, want 0", got)
		}
		afterDegraded := metricSample(t, EngagementEventOpened, SurfaceWeb, EngagementStatusDegraded)
		if delta := afterDegraded - beforeDegraded; delta < 1 {
			t.Fatalf("degraded metric delta = %f, want >= 1", delta)
		}
		audits := captured.envelopesForAction(AuditActionEngagementSignalFlush)
		if len(audits) != 1 {
			t.Fatalf("audit envelope count = %d, want 1", len(audits))
		}
		if audits[0].Outcome != EngagementAuditOutcomeDegraded {
			t.Fatalf("audit outcome = %q, want %q", audits[0].Outcome, EngagementAuditOutcomeDegraded)
		}
	})

	t.Run("transport_timeout_retries_three_times_then_drops_degraded", func(t *testing.T) {
		captured := newCapturedAuditSink(t)
		defer captured.restore()

		transport := newFakeTransport(func(_ int, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
			// Simulate a transport timeout: no per-signal HTTP status,
			// every signal is Retryable, and the transport returns an
			// error so the Exporter sees the batch-level network error.
			results := make([]EngagementSignalResult, 0, len(signals))
			for _, s := range signals {
				results = append(results, EngagementSignalResult{
					SignalID:     s.SignalID,
					Reason:       EngagementErrTransportFailed,
					Retryable:    true,
					NetworkError: true,
				})
			}
			return results, errors.New("simulated timeout")
		})

		exporter := NewExporter(ExporterOptions{
			Transport:                 transport,
			Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
			EngagementSignalSupported: true,
			Clock:                     fixedNow,
			MaxAttempts:               3,
			InitialBackoff:            1 * time.Millisecond,
			MaxBackoff:                2 * time.Millisecond,
		})
		if _, ok := exporter.Capture(context.Background(), CaptureRequest{
			Event: EngagementEventOpened, Surface: SurfaceDigest,
			PacketID: "pkt-timeout", TraceID: "trc-timeout", ActorRef: "actor-timeout",
		}); !ok {
			t.Fatalf("Capture failed")
		}
		captured.reset()
		exporter.Flush(context.Background())

		if got := transport.AttemptCount(); got != 3 {
			t.Fatalf("timeout attempt count = %d, want 3", got)
		}
		audits := captured.envelopesForAction(AuditActionEngagementSignalFlush)
		if len(audits) != 1 {
			t.Fatalf("audit envelope count = %d, want 1", len(audits))
		}
		if audits[0].Outcome != EngagementAuditOutcomeDegraded {
			t.Fatalf("audit outcome = %q, want %q", audits[0].Outcome, EngagementAuditOutcomeDegraded)
		}
	})
}

// TestEngagementExporterDropsOldestOnOverflowAndRecordsOverflowDropMetricAndAuditEnvelope
// (SCN-SM-041-024) drives the in-memory ring past its capacity and
// asserts:
//
//   - the oldest entry is dropped (the freshest entry remains);
//   - the overflow drop emits
//     smackerel_qf_engagement_signal_attempts_total{event="overflow_drop", surface, status="dropped"};
//   - an audit envelope with outcome=degraded and
//     reason=ENGAGEMENT_BUFFER_OVERFLOW is written for the dropped signal.
//
// Adversarial trip-wire: a regression that dropped the NEWEST entry
// instead of the OLDEST would leave the original signal at index 0 in
// the buffer; the test asserts the buffer's last entry is the freshest
// signal.
func TestEngagementExporterDropsOldestOnOverflowAndRecordsOverflowDropMetricAndAuditEnvelope(t *testing.T) {
	captured := newCapturedAuditSink(t)
	defer captured.restore()

	// Capacity=3, threshold large enough that we don't trigger a flush
	// before we deliberately overflow. We then enqueue 4 signals so the
	// 4th one triggers a single overflow drop of the oldest entry.
	transport := newFakeTransport(nil)
	exporter := NewExporter(ExporterOptions{
		Transport:                 transport,
		Consent:                   staticConsent{value: EngagementConsentRawAnonymous},
		EngagementSignalSupported: true,
		Clock:                     fixedNow,
		FlushInterval:             10 * time.Second, // never triggers in the test window
		FlushThreshold:            100,
		Capacity:                  3,
	})

	beforeOverflow := metricSample(t, EngagementEventOverflowDrop, SurfaceWeb, EngagementStatusDropped)

	signals := make([]PacketEngagementSignal, 0, 4)
	for i := 0; i < 4; i++ {
		sig, ok := exporter.Capture(context.Background(), CaptureRequest{
			Event:    EngagementEventOpened,
			Surface:  SurfaceWeb,
			PacketID: "pkt-of-" + strconv.Itoa(i),
			TraceID:  "trc-of-" + strconv.Itoa(i),
			ActorRef: "actor-of-" + strconv.Itoa(i),
		})
		if !ok {
			t.Fatalf("Capture %d failed (overflow path)", i)
		}
		signals = append(signals, sig)
	}

	if got := exporter.BufferLen(); got != 3 {
		t.Fatalf("buffer length after overflow = %d, want 3 (capacity)", got)
	}
	// Inspect the buffer directly (test-only) to confirm OLDEST drop:
	// the OLDEST signal (index 0 captured first) should be gone; the
	// freshest signal (last captured) should be present at the tail.
	exporter.mu.Lock()
	bufferSnapshot := make([]PacketEngagementSignal, len(exporter.buffer))
	copy(bufferSnapshot, exporter.buffer)
	exporter.mu.Unlock()
	if bufferSnapshot[0].PacketID == "pkt-of-0" {
		t.Fatalf("OLDEST entry not dropped: buffer[0]=%q", bufferSnapshot[0].PacketID)
	}
	if bufferSnapshot[len(bufferSnapshot)-1].PacketID != "pkt-of-3" {
		t.Fatalf("FRESHEST entry not at tail: buffer[last]=%q, want pkt-of-3",
			bufferSnapshot[len(bufferSnapshot)-1].PacketID)
	}

	afterOverflow := metricSample(t, EngagementEventOverflowDrop, SurfaceWeb, EngagementStatusDropped)
	if delta := afterOverflow - beforeOverflow; delta < 1 {
		t.Fatalf("overflow_drop metric delta = %f, want >= 1", delta)
	}
	// Match audits by signal id (the dropped sig is signals[0]).
	dropAudits := captured.envelopesForActionAndSignal(AuditActionEngagementSignalFlush, signals[0].SignalID)
	if len(dropAudits) != 1 {
		t.Fatalf("overflow audit envelope count for dropped signal = %d, want 1", len(dropAudits))
	}
	if dropAudits[0].Reason != EngagementErrBufferOverflow {
		t.Fatalf("overflow audit reason = %q, want %q", dropAudits[0].Reason, EngagementErrBufferOverflow)
	}
	if dropAudits[0].Outcome != EngagementAuditOutcomeDegraded {
		t.Fatalf("overflow audit outcome = %q, want %q", dropAudits[0].Outcome, EngagementAuditOutcomeDegraded)
	}
}

// --- shared helpers --------------------------------------------------

// waitFor polls `cond` every 10ms up to `timeout`. Fails the test if
// the condition never becomes true.
func waitFor(t *testing.T, name string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waitFor(%s): condition did not become true within %s", name, timeout)
}

// capturedAuditSink intercepts EmitConnectorAuditEnvelope by swapping
// the package default slog logger to a JSON handler that writes into
// an in-test buffer. envelopesForAction decodes the captured JSON
// records and returns the audit envelopes that match the supplied
// action. The restore() method MUST be called on test teardown so the
// previous logger is reinstalled.
type capturedAuditSink struct {
	prev *slog.Logger
	buf  *syncBuffer
}

type capturedRecord struct {
	Action   string `json:"action"`
	Outcome  string `json:"outcome"`
	Reason   string `json:"reason"`
	SignalID string `json:"signal_id"`
	PacketID string `json:"packet_id"`
	TraceID  string `json:"trace_id"`
	Surface  string `json:"surface"`
	Msg      string `json:"msg"`
}

// syncBuffer is a goroutine-safe bytes buffer; slog writes from the
// flush worker goroutine, the test reads from the test goroutine.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func newCapturedAuditSink(t *testing.T) *capturedAuditSink {
	t.Helper()
	prev := slog.Default()
	buf := &syncBuffer{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &capturedAuditSink{prev: prev, buf: buf}
}

func (c *capturedAuditSink) restore() {
	slog.SetDefault(c.prev)
}

func (c *capturedAuditSink) reset() {
	c.buf.Reset()
}

func (c *capturedAuditSink) records() []capturedRecord {
	lines := splitLines(c.buf.String())
	out := make([]capturedRecord, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var rec capturedRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // not a JSON record we care about
		}
		out = append(out, rec)
	}
	return out
}

func (c *capturedAuditSink) envelopesForAction(action string) []capturedRecord {
	out := make([]capturedRecord, 0)
	for _, rec := range c.records() {
		if rec.Action == action {
			out = append(out, rec)
		}
	}
	return out
}

func (c *capturedAuditSink) envelopesForActionAndSignal(action, signalID string) []capturedRecord {
	out := make([]capturedRecord, 0)
	for _, rec := range c.records() {
		if rec.Action == action && rec.SignalID == signalID {
			out = append(out, rec)
		}
	}
	return out
}

func splitLines(s string) []string {
	out := make([]string, 0, 8)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
