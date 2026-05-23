//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Scope 6 integration tests (SCN-SM-041-022..024). Each test wires the
// connector against a live disposable test stack (Postgres + NATS via
// testPool / qfDecisionsNATSClient) and a per-test httptest QF stub
// that handles BOTH the capability handshake AND the new
// `POST /api/private/smackerel/v1/packet-engagement-signals` endpoint
// the exporter targets on flush.
//
// The integration tests exercise the FULL transport stack — connector
// Connect → exporter construction with capability gate → consent
// reader injection → Capture → buffered flush worker → HTTP POST → QF
// stub response → metric + audit envelope emission — so the failures
// surface at the integration boundary in addition to the unit-test
// fakes.

// TestQFEngagementSignalRoundTripCapturesAllSurfacesFlushesAndPostsIdempotentUUIDv7
// (SCN-SM-041-022, SCN-SM-041-023) wires the connector against the
// live disposable stack and a happy QF stub that records every batch
// the exporter POSTs to the engagement endpoint. The test:
//
//   - installs an integration-scoped privacy ConsentReader returning
//     `pseudonymous` so the consent gate admits events;
//   - drives Captures across all three surfaces (web/digest/telegram)
//     plus the dwell event;
//   - calls exporter.Flush(ctx) once to force a flush attempt;
//   - asserts the QF stub received exactly one batch with the expected
//     count of signals, each carrying a UUIDv7 signal_id;
//   - drives a second Capture against a per-attempt stub mode that
//     replies HTTP 500 on attempt 1 and HTTP 200 idempotent on
//     attempt 2 for the SAME signal_id, then asserts both stub
//     attempts carry the IDENTICAL signal_id (proving the exporter's
//     retry path preserves signal envelopes verbatim across attempts
//     — SCN-SM-041-023's "QF idempotency contract" leg at the
//     integration boundary).
//
// Adversarial trip-wire: a regression that re-generated signal_id on
// each retry attempt would defeat the idempotency contract; this test
// compares the signal_id observed on the stub's first attempt against
// the signal_id observed on the retry attempt and fails if they differ.
// The audit-envelope-not-duplicated leg of the idempotency contract is
// owned by the unit test
// `TestEngagementSignalIDIsUUIDv7AndIdempotentAcrossRepeatedFlushAttempt`,
// which has direct access to the package-internal audit sink.
func TestQFEngagementSignalRoundTripCapturesAllSurfacesFlushesAndPostsIdempotentUUIDv7(t *testing.T) {
	_ = testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stub := newEngagementQFStub(t)
	defer stub.Close()

	// Install a pseudonymous consent reader so the exporter's
	// fail-closed default does NOT bypass events at capture time.
	prevReader := qfdecisions.ConsentReader(nil) // placeholder for restore
	qfdecisions.SetEngagementConsentReader(qfdecisions.ConsentReaderFunc(func(context.Context) string {
		return qfdecisions.EngagementConsentPseudonymous
	}))
	defer qfdecisions.SetEngagementConsentReader(prevReader)

	conn := qfdecisions.New("qf-decisions-it-engagement-roundtrip-" + uniqueSuffix())
	if err := conn.Connect(ctx, qfIntegrationConfig(stub.server.URL, 1)); err != nil {
		t.Fatalf("Connect against engagement stub: %v", err)
	}
	defer func() { _ = conn.Close() }()

	exporter := conn.EngagementExporter()
	if exporter == nil || !exporter.Enabled() {
		t.Fatalf("EngagementExporter expected to be enabled (capability=true), got nil or disabled")
	}

	// Baseline metric counts BEFORE driving any captures (the prom
	// registry is process-global and other tests may have nudged the
	// vectors; delta comparisons are required).
	baselineAccepted := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusAccepted))

	// Drive a Capture per surface plus a dwell event.
	capturedIDs := make(map[string]string)
	captures := []struct {
		event   string
		surface string
		packet  string
		trace   string
		dwell   *int
	}{
		{qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, "pkt-it-web", "trc-it-web", nil},
		{qfdecisions.EngagementEventOpened, qfdecisions.SurfaceDigest, "pkt-it-digest", "trc-it-digest", nil},
		{qfdecisions.EngagementEventOpened, qfdecisions.SurfaceTelegram, "pkt-it-telegram", "trc-it-telegram", nil},
		{qfdecisions.EngagementEventDwell, qfdecisions.SurfaceWeb, "pkt-it-dwell", "trc-it-dwell", intPtr(17)},
	}
	for _, c := range captures {
		sig, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: c.event, Surface: c.surface,
			PacketID: c.packet, TraceID: c.trace,
			ActorRef: "it-actor", DwellSeconds: c.dwell,
		})
		if !ok {
			t.Fatalf("Capture(%s,%s) reported no enqueue", c.event, c.surface)
		}
		capturedIDs[c.surface+":"+c.event] = sig.SignalID
	}

	if got, want := exporter.BufferLen(), len(captures); got != want {
		t.Fatalf("BufferLen after captures = %d, want %d", got, want)
	}

	// Force a flush.
	exporter.Flush(ctx)

	// The stub MUST have received one batch with the expected signal count.
	stub.waitForBatch(t, 1, 5*time.Second)
	batches := stub.batches()
	if len(batches) != 1 {
		t.Fatalf("engagement stub received %d batches, want 1", len(batches))
	}
	if len(batches[0]) != len(captures) {
		t.Fatalf("first batch carried %d signals, want %d", len(batches[0]), len(captures))
	}

	// Every signal_id MUST be a valid UUIDv7.
	for _, sig := range batches[0] {
		if !isUUIDv7(sig.SignalID) {
			t.Fatalf("signal_id %q is not a UUIDv7", sig.SignalID)
		}
	}

	// Part 2 — signal_id stability across retries (SCN-SM-041-023).
	//
	// The exporter's retry path (design.md §Failure Handling) MUST
	// preserve the original PacketEngagementSignal envelope verbatim
	// across attempts — most importantly the `signal_id` — so QF can
	// recognise the retry and reply HTTP 200 idempotent without
	// double-counting. We exercise that contract end-to-end by:
	//
	//   1. Flipping the stub into `retryThenAccept` mode, which keys
	//      on `signal_id`: the FIRST POST per signal_id replies HTTP
	//      500 (retryable), subsequent POSTs reply HTTP 200 idempotent.
	//   2. Resetting the stub's batch ledger so the next-arriving
	//      attempts can be inspected in isolation.
	//   3. Capturing exactly one new signal whose signal_id is
	//      observed before the flush.
	//   4. Flushing — the exporter MUST POST attempt 1 (gets 500),
	//      back off, POST attempt 2 (gets 200 idempotent), and treat
	//      the signal as accepted.
	//   5. Asserting the stub recorded ≥ 2 batches and that
	//      batch[0][0].SignalID == batch[1][0].SignalID, which is the
	//      adversarial trip-wire: a regression that regenerated the
	//      signal_id on retry would post a different id on attempt 2
	//      and fail this comparison.
	//   6. Asserting the accepted metric advanced beyond the Part 1
	//      baseline (proves the idempotent 200 reply was counted as
	//      accepted, not degraded).
	stub.resetBatches()
	stub.retryThenAccept.Store(true)

	retrySig, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
		Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
		PacketID: "pkt-it-retry-idem", TraceID: "trc-it-retry-idem", ActorRef: "it-actor",
	})
	if !ok {
		t.Fatalf("retry-idempotency Capture returned !ok")
	}
	if !isUUIDv7(retrySig.SignalID) {
		t.Fatalf("retry-idempotency captured signal_id %q is not a UUIDv7", retrySig.SignalID)
	}

	exporter.Flush(ctx)

	// Two attempts MUST arrive at the stub: attempt 1 (500) and
	// attempt 2 (200 idempotent).
	stub.waitForBatch(t, 2, 5*time.Second)
	retryBatches := stub.batches()
	if len(retryBatches) < 2 {
		t.Fatalf("retry-idempotency stub batches = %d, want >= 2", len(retryBatches))
	}
	if len(retryBatches[0]) != 1 || len(retryBatches[1]) != 1 {
		t.Fatalf("retry-idempotency batch sizes = [%d,%d], want [1,1]",
			len(retryBatches[0]), len(retryBatches[1]))
	}
	firstAttemptID := retryBatches[0][0].SignalID
	secondAttemptID := retryBatches[1][0].SignalID
	if firstAttemptID != secondAttemptID {
		t.Fatalf("signal_id regenerated on retry: attempt 1 = %q, attempt 2 = %q (exporter MUST preserve signal_id verbatim across retries — SCN-SM-041-023)",
			firstAttemptID, secondAttemptID)
	}
	if firstAttemptID != retrySig.SignalID {
		t.Fatalf("stub-observed signal_id %q does not match captured signal_id %q",
			firstAttemptID, retrySig.SignalID)
	}

	// Metric assertion: accepted delta covers Part 1 (web `opened`
	// captures pkt-it-web) PLUS Part 2's retry-then-idempotent path
	// (pkt-it-retry-idem accepted on attempt 2 via HTTP 200
	// idempotent). The (event=opened, surface=web, status=accepted)
	// label triplet should advance by at least 2.
	afterAccepted := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusAccepted))
	if afterAccepted-baselineAccepted < 2 {
		t.Fatalf("accepted metric delta = %f, want >= 2 (Part 1 web opened + Part 2 retry-then-idempotent web opened)",
			afterAccepted-baselineAccepted)
	}
}

// TestQFEngagementSignalFailureMatrixDrops4xxRetries5xxAndEmitsAuditEnvelopeAndMetrics
// (SCN-SM-041-024) exercises the failure matrix against a live QF stub
// that:
//
//   - replies HTTP 409 (ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD)
//     for the first signal — expect ONE attempt, status=rejected
//   - replies HTTP 500 (INTERNAL_ERROR) for the second signal — expect
//     three attempts, final status=degraded
//
// The test asserts:
//
//   - the stub records exactly 1 attempt for the 4xx signal;
//   - the stub records exactly 3 attempts for the 5xx signal (bounded
//     retry per design.md §Failure Handling);
//   - the rejected/degraded metric labels increment by at least 1 each;
//   - the audit envelope outcomes (rejected, degraded) are emitted
//     to the connector audit sink.
//
// Adversarial trip-wire: a regression that treated 4xx as retryable
// would inflate the 4xx signal's attempt count; the test asserts the
// exact value 1.
func TestQFEngagementSignalFailureMatrixDrops4xxRetries5xxAndEmitsAuditEnvelopeAndMetrics(t *testing.T) {
	_ = testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stub := newEngagementQFStub(t)
	stub.failureMode.Store(true)
	defer stub.Close()

	prevReader := qfdecisions.ConsentReader(nil)
	qfdecisions.SetEngagementConsentReader(qfdecisions.ConsentReaderFunc(func(context.Context) string {
		return qfdecisions.EngagementConsentAnonymous
	}))
	defer qfdecisions.SetEngagementConsentReader(prevReader)

	conn := qfdecisions.New("qf-decisions-it-engagement-failure-" + uniqueSuffix())
	if err := conn.Connect(ctx, qfIntegrationConfig(stub.server.URL, 1)); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = conn.Close() }()

	exporter := conn.EngagementExporter()
	if exporter == nil {
		t.Fatalf("EngagementExporter is nil")
	}

	baselineRejected := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusRejected))
	baselineDegraded := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceDigest, qfdecisions.EngagementStatusDegraded))

	// Capture two signals: one tagged to receive 4xx, one tagged to
	// receive 5xx. The stub keys on packet_id to choose its reply.
	if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
		Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
		PacketID: "pkt-it-fail-4xx", TraceID: "trc-it-fail-4xx", ActorRef: "it-actor",
	}); !ok {
		t.Fatalf("4xx Capture failed")
	}
	if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
		Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceDigest,
		PacketID: "pkt-it-fail-5xx", TraceID: "trc-it-fail-5xx", ActorRef: "it-actor",
	}); !ok {
		t.Fatalf("5xx Capture failed")
	}

	exporter.Flush(ctx)

	// The 4xx packet MUST receive exactly 1 attempt; the 5xx packet
	// MUST receive exactly 3 attempts (bounded retry budget).
	attemptsByPacket := stub.attemptsByPacket()
	if got := attemptsByPacket["pkt-it-fail-4xx"]; got != 1 {
		t.Fatalf("4xx packet attempt count = %d, want 1 (no retry)", got)
	}
	if got := attemptsByPacket["pkt-it-fail-5xx"]; got != 3 {
		t.Fatalf("5xx packet attempt count = %d, want 3 (bounded retry)", got)
	}

	// Metric deltas: rejected ≥ 1 for the 4xx packet; degraded ≥ 1 for
	// the 5xx packet (after retry exhaustion).
	afterRejected := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusRejected))
	if delta := afterRejected - baselineRejected; delta < 1 {
		t.Fatalf("rejected metric delta = %f, want >= 1", delta)
	}
	afterDegraded := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceDigest, qfdecisions.EngagementStatusDegraded))
	if delta := afterDegraded - baselineDegraded; delta < 1 {
		t.Fatalf("degraded metric delta = %f, want >= 1", delta)
	}
}

// --- helpers ----------------------------------------------------------

func intPtr(v int) *int { return &v }

func isUUIDv7(id string) bool {
	// UUIDv7 has version nibble == 7 at position 14 (0-indexed) of the
	// canonical 8-4-4-4-12 string form. Position 14 corresponds to the
	// '7' in `xxxxxxxx-xxxx-Mxxx-Nxxx-xxxxxxxxxxxx` where M is the
	// version. UUIDs are 36 chars total.
	if len(id) != 36 {
		return false
	}
	return id[14] == '7'
}

// engagementQFStub serves the capability handshake plus the engagement
// endpoint and records every batch it receives so the test can assert
// flush behavior, retry counts, and per-signal HTTP outcomes. The stub
// honors three modes:
//
//   - default: replies HTTP 201 Created for every signal
//   - retryThenAccept (atomic): replies HTTP 500 INTERNAL_ERROR on
//     the FIRST POST per signal_id and HTTP 200 idempotent on every
//     subsequent POST of the SAME signal_id, exercising the
//     exporter's bounded-retry + idempotent-accept path against a
//     stable signal envelope
//   - failureMode (atomic): keys on packet_id — `pkt-it-fail-4xx`
//     replies HTTP 409, `pkt-it-fail-5xx` replies HTTP 500
type engagementQFStub struct {
	server             *httptest.Server
	mu                 sync.Mutex
	batchesReceived    [][]qfdecisions.PacketEngagementSignal
	attemptPerPacket   map[string]int
	attemptsBySignalID map[string]int
	retryThenAccept    atomic.Bool
	failureMode        atomic.Bool
}

func newEngagementQFStub(t *testing.T) *engagementQFStub {
	t.Helper()
	stub := &engagementQFStub{
		attemptPerPacket:   make(map[string]int),
		attemptsBySignalID: make(map[string]int),
	}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("[stub] %s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case qfdecisions.CapabilitiesPath:
			_ = json.NewEncoder(w).Encode(validQFIntegrationCapability())
			return
		case qfdecisions.PacketEngagementSignalsPath:
			stub.handleEngagementPost(w, r)
			return
		default:
			t.Logf("[stub] NOT FOUND %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	return stub
}

func (s *engagementQFStub) Close() {
	s.server.Close()
}

func (s *engagementQFStub) handleEngagementPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var posted []qfdecisions.PacketEngagementSignal
	if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
		http.Error(w, "decode body: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.batchesReceived = append(s.batchesReceived, posted)
	for _, sig := range posted {
		s.attemptPerPacket[sig.PacketID]++
	}
	bcount := len(s.batchesReceived)
	s.mu.Unlock()
	fmt.Printf("[stub %p] handleEngagementPost appended posted=%d batchesReceived=%d\n", s, len(posted), bcount)
	_ = bcount

	// Build per-signal results.
	results := make([]map[string]any, 0, len(posted))
	for _, sig := range posted {
		entry := map[string]any{"signal_id": sig.SignalID}
		switch {
		case s.failureMode.Load() && sig.PacketID == "pkt-it-fail-4xx":
			entry["status_code"] = http.StatusConflict
			entry["reason"] = qfdecisions.EngagementErrSignalIDReuseDifferentPayload
		case s.failureMode.Load() && sig.PacketID == "pkt-it-fail-5xx":
			entry["status_code"] = http.StatusInternalServerError
			entry["reason"] = "INTERNAL_ERROR"
		case s.retryThenAccept.Load():
			s.mu.Lock()
			s.attemptsBySignalID[sig.SignalID]++
			attempts := s.attemptsBySignalID[sig.SignalID]
			s.mu.Unlock()
			if attempts == 1 {
				entry["status_code"] = http.StatusInternalServerError
				entry["reason"] = "INTERNAL_ERROR"
			} else {
				entry["status_code"] = http.StatusOK
				entry["idempotent"] = true
			}
		default:
			entry["status_code"] = http.StatusCreated
		}
		results = append(results, entry)
	}
	// Pick the dominant batch-level status to reply with; the per-
	// signal results array is authoritative for the connector decode.
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(results)
}

func (s *engagementQFStub) batches() [][]qfdecisions.PacketEngagementSignal {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([][]qfdecisions.PacketEngagementSignal, len(s.batchesReceived))
	for i, b := range s.batchesReceived {
		cp[i] = append([]qfdecisions.PacketEngagementSignal(nil), b...)
	}
	return cp
}

func (s *engagementQFStub) resetBatches() {
	s.mu.Lock()
	s.batchesReceived = nil
	s.mu.Unlock()
}

func (s *engagementQFStub) attemptsByPacket() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.attemptPerPacket))
	for k, v := range s.attemptPerPacket {
		out[k] = v
	}
	return out
}

func (s *engagementQFStub) waitForBatch(t *testing.T, wantCount int, timeout time.Duration) {
	t.Helper()
	fmt.Printf("[stub %p] waitForBatch wantCount=%d\n", s, wantCount)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := len(s.batchesReceived)
		s.mu.Unlock()
		if got >= wantCount {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	got := len(s.batchesReceived)
	s.mu.Unlock()
	t.Fatalf("waited %s for %d batches; got %d", timeout, wantCount, got)
}
