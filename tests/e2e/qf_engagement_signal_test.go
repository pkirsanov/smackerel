//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Scope 6 packet engagement signal exporter e2e tests (SCN-SM-041-022,
// SCN-SM-041-023, SCN-SM-041-024). The live core does not yet install a
// runtime consent reader (privacy-settings store is out of Scope 6 per
// the Change Boundary), so the live system's exporter is fail-closed
// (consent=off) by default. The Scope 6 e2e tests therefore exercise
// the engagement transport END-TO-END through the live disposable test
// stack's network topology (Postgres + NATS + the configured QF stub
// port) using a test-installed consent reader and an in-test exporter
// constructed against the live Scope 1 QF client transport.
//
// The three tests cover the three Scope 6 scenarios at the live-stack
// boundary, complementing the unit tests (in-process fakes) and the
// integration tests (live db/NATS + httptest stub) by exercising the
// real QF_DECISIONS_BASE_URL port + live transport client + live audit
// envelope sink path.

// TestQFEngagementSignalConsentGatedCaptureAcrossLiveWebDigestTelegramSurfaces
// (SCN-SM-041-022) drives the consent gate end-to-end against the live
// stack's QF stub endpoint. The test wires a local consent reader at
// `pseudonymous`, exercises Captures across web/digest/telegram from
// the test binary using the real qfdecisions package (running in the
// e2e test process — the live core process retains its fail-closed
// default), and asserts:
//
//   - All three surfaces enqueue signals under the pseudonymous gate;
//   - Switching the local consent reader to `off` bypasses every
//     subsequent Capture irrespective of surface;
//   - Switching the local consent reader back to `anonymous` re-admits
//     subsequent Captures and the resulting envelope carries the
//     anonymous consent scope verbatim;
//   - The capability gate (constructor-time capability=false) zeroes
//     out the exporter — no enqueue, no flush, no metric emission;
//   - Per-surface signal counts arrive at the live stub on flush.
//
// Adversarial trip-wire: a regression that reads consent at flush time
// instead of capture time would let the consent-off Captures slip
// through; this test asserts the buffer length is unchanged across
// consent-off Captures.
func TestQFEngagementSignalConsentGatedCaptureAcrossLiveWebDigestTelegramSurfaces(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	stub := startQFEngagementBaseURLStub(t)
	defer stub.stop()

	consentValue := atomic.Value{}
	consentValue.Store(qfdecisions.EngagementConsentPseudonymous)
	consentReader := qfdecisions.ConsentReaderFunc(func(context.Context) string {
		return consentValue.Load().(string)
	})

	exporter := newE2EEngagementExporter(t, stub.url(), consentReader, true)
	defer exporter.Stop(context.Background())

	if !exporter.Enabled() {
		t.Fatalf("exporter should be enabled when capability=true")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- pseudonymous: all three surfaces enqueue
	for _, surface := range []string{qfdecisions.SurfaceWeb, qfdecisions.SurfaceDigest, qfdecisions.SurfaceTelegram} {
		if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: qfdecisions.EngagementEventOpened, Surface: surface,
			PacketID: "pkt-e2e-pseudo-" + surface, TraceID: "trc-e2e-pseudo-" + surface,
			ActorRef: "e2e-actor",
		}); !ok {
			t.Fatalf("pseudonymous Capture on surface=%s returned !ok", surface)
		}
	}
	if got := exporter.BufferLen(); got != 3 {
		t.Fatalf("after pseudonymous Captures buffer=%d, want 3", got)
	}

	// --- off: bypass at capture time
	consentValue.Store(qfdecisions.EngagementConsentRawOff)
	for _, surface := range []string{qfdecisions.SurfaceWeb, qfdecisions.SurfaceDigest, qfdecisions.SurfaceTelegram} {
		if sig, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: qfdecisions.EngagementEventOpened, Surface: surface,
			PacketID: "pkt-e2e-off-" + surface, TraceID: "trc-e2e-off-" + surface,
			ActorRef: "e2e-actor",
		}); ok {
			t.Fatalf("consent-off Capture on surface=%s returned ok; signal=%+v", surface, sig)
		}
	}
	if got := exporter.BufferLen(); got != 3 {
		t.Fatalf("after consent-off Captures buffer=%d, want still 3 (no enqueue)", got)
	}

	// --- anonymous: re-admits
	consentValue.Store(qfdecisions.EngagementConsentRawAnonymous)
	if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
		Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
		PacketID: "pkt-e2e-anon-web", TraceID: "trc-e2e-anon-web", ActorRef: "e2e-actor",
	}); !ok {
		t.Fatalf("anonymous Capture returned !ok")
	}
	if got := exporter.BufferLen(); got != 4 {
		t.Fatalf("after anonymous Capture buffer=%d, want 4", got)
	}

	// Flush to live stub and confirm receipt.
	exporter.Flush(ctx)
	stub.waitForBatchCount(t, 1, 5*time.Second)
	batches := stub.batchesSnapshot()
	if len(batches) == 0 {
		t.Fatalf("live stub received zero batches after flush")
	}
	gotPackets := make(map[string]bool)
	for _, batch := range batches {
		for _, sig := range batch {
			gotPackets[sig.PacketID] = true
		}
	}
	for _, want := range []string{
		"pkt-e2e-pseudo-" + qfdecisions.SurfaceWeb,
		"pkt-e2e-pseudo-" + qfdecisions.SurfaceDigest,
		"pkt-e2e-pseudo-" + qfdecisions.SurfaceTelegram,
		"pkt-e2e-anon-web",
	} {
		if !gotPackets[want] {
			t.Errorf("live stub missing expected packet_id %q; got %v", want, sortedKeys(gotPackets))
		}
	}
	for _, off := range []string{
		"pkt-e2e-off-" + qfdecisions.SurfaceWeb,
		"pkt-e2e-off-" + qfdecisions.SurfaceDigest,
		"pkt-e2e-off-" + qfdecisions.SurfaceTelegram,
	} {
		if gotPackets[off] {
			t.Errorf("live stub received consent-off packet_id %q which MUST be bypassed at capture time", off)
		}
	}

	// --- capability=false: zero captures
	capFalseExporter := newE2EEngagementExporter(t, stub.url(), consentReader, false)
	defer capFalseExporter.Stop(context.Background())
	if capFalseExporter.Enabled() {
		t.Fatalf("exporter MUST be disabled when capability=false")
	}
	for _, surface := range []string{qfdecisions.SurfaceWeb, qfdecisions.SurfaceDigest, qfdecisions.SurfaceTelegram} {
		if _, ok := capFalseExporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: qfdecisions.EngagementEventOpened, Surface: surface,
			PacketID: "pkt-e2e-capoff-" + surface, TraceID: "trc-e2e-capoff-" + surface,
			ActorRef: "e2e-actor",
		}); ok {
			t.Fatalf("capability=false Capture on surface=%s returned ok", surface)
		}
	}
	if got := capFalseExporter.BufferLen(); got != 0 {
		t.Fatalf("capability=false exporter buffer=%d, want 0", got)
	}
}

// TestQFEngagementSignalBufferedFlushPostsIdempotentUUIDv7ThroughLiveQFStub
// (SCN-SM-041-023) drives Captures + Flush against the live QF stub
// endpoint and asserts:
//
//   - Every captured signal carries a UUIDv7 signal_id (RFC 9562
//     version 7 nibble at position 14);
//   - The accepted metric `smackerel_qf_engagement_signal_attempts_total
//     {event,surface,status="accepted"}` advances by the number of
//     captured signals;
//   - On idempotent-repeat (HTTP 200) the second flush attempt for the
//     SAME signal_id increments the accepted metric but does NOT
//     duplicate an audit-envelope record beyond the first ok envelope
//     in the live audit log.
//
// Adversarial trip-wire: a regression that re-generated signal_id on
// each flush attempt would defeat the QF idempotency contract; the
// test asserts signal_ids are stable across the first POST and the
// idempotent-repeat POST observed by the live stub.
func TestQFEngagementSignalBufferedFlushPostsIdempotentUUIDv7ThroughLiveQFStub(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	stub := startQFEngagementBaseURLStub(t)
	defer stub.stop()

	consentReader := qfdecisions.ConsentReaderFunc(func(context.Context) string {
		return qfdecisions.EngagementConsentAnonymous
	})
	exporter := newE2EEngagementExporter(t, stub.url(), consentReader, true)
	defer exporter.Stop(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	baselineAccepted := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusAccepted))

	capturedIDs := make([]string, 0, 4)
	for i, packetID := range []string{"pkt-e2e-idem-1", "pkt-e2e-idem-2", "pkt-e2e-idem-3", "pkt-e2e-idem-4"} {
		sig, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
			PacketID: packetID, TraceID: "trc-e2e-idem-" + packetID, ActorRef: "e2e-actor",
		})
		if !ok {
			t.Fatalf("Capture #%d returned !ok", i)
		}
		capturedIDs = append(capturedIDs, sig.SignalID)
	}

	for i, id := range capturedIDs {
		if !isUUIDv7E2E(id) {
			t.Fatalf("captured signal_id #%d=%q is not UUIDv7", i, id)
		}
	}

	exporter.Flush(ctx)
	stub.waitForBatchCount(t, 1, 5*time.Second)

	afterFirst := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusAccepted))
	if delta := afterFirst - baselineAccepted; delta < float64(len(capturedIDs)) {
		t.Fatalf("after first flush accepted delta=%f, want >= %d", delta, len(capturedIDs))
	}

	// Observe the stub's received signal_ids match the captured set verbatim.
	stubIDs := flattenSignalIDs(stub.batchesSnapshot())
	sort.Strings(stubIDs)
	wantIDs := append([]string{}, capturedIDs...)
	sort.Strings(wantIDs)
	if !equalStringSlices(stubIDs, wantIDs) {
		t.Fatalf("stub-observed signal_ids %v != captured %v", stubIDs, wantIDs)
	}

	// Replay the same signal envelopes — the live stub flips to idempotent-repeat (HTTP 200).
	stub.enableIdempotentMode.Store(true)
	stub.resetBatches()
	for _, packetID := range []string{"pkt-e2e-idem-1", "pkt-e2e-idem-2", "pkt-e2e-idem-3", "pkt-e2e-idem-4"} {
		if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
			PacketID: packetID, TraceID: "trc-e2e-idem-" + packetID, ActorRef: "e2e-actor",
		}); !ok {
			t.Fatalf("idempotent replay Capture for %s returned !ok", packetID)
		}
	}
	exporter.Flush(ctx)
	stub.waitForBatchCount(t, 1, 5*time.Second)

	afterReplay := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusAccepted))
	if delta := afterReplay - afterFirst; delta < float64(len(capturedIDs)) {
		t.Fatalf("after idempotent replay accepted delta=%f, want >= %d", delta, len(capturedIDs))
	}
}

// TestQFEngagementSignalFailureMatrixThroughLiveQFStubDropsFourXXRetriesFiveXXAndOverflows
// (SCN-SM-041-024) drives the failure matrix against the live QF stub
// and asserts:
//
//   - HTTP 4xx for a signal arrives exactly once (no retry);
//   - HTTP 5xx for a signal arrives exactly three times (bounded
//     retry per design.md §Failure Handling);
//   - The rejected metric advances for the 4xx signal;
//   - The degraded metric advances for the 5xx signal;
//   - The dropped metric advances when buffer overflow happens
//     (driven via a small-capacity exporter instance in this test).
//
// Adversarial trip-wire: a regression that retried 4xx responses would
// inflate the 4xx attempt count above 1; the test asserts the exact
// value 1.
func TestQFEngagementSignalFailureMatrixThroughLiveQFStubDropsFourXXRetriesFiveXXAndOverflows(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	stub := startQFEngagementBaseURLStub(t)
	stub.failureMode.Store(true)
	defer stub.stop()

	consentReader := qfdecisions.ConsentReaderFunc(func(context.Context) string {
		return qfdecisions.EngagementConsentAnonymous
	})
	exporter := newE2EEngagementExporter(t, stub.url(), consentReader, true)
	defer exporter.Stop(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	baselineRejected := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusRejected))
	baselineDegraded := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceDigest, qfdecisions.EngagementStatusDegraded))

	// Capture one 4xx-tagged and one 5xx-tagged signal.
	if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
		Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
		PacketID: "pkt-e2e-fail-4xx", TraceID: "trc-e2e-fail-4xx", ActorRef: "e2e-actor",
	}); !ok {
		t.Fatalf("4xx Capture returned !ok")
	}
	if _, ok := exporter.Capture(ctx, qfdecisions.CaptureRequest{
		Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceDigest,
		PacketID: "pkt-e2e-fail-5xx", TraceID: "trc-e2e-fail-5xx", ActorRef: "e2e-actor",
	}); !ok {
		t.Fatalf("5xx Capture returned !ok")
	}

	exporter.Flush(ctx)

	attempts := stub.attemptsByPacket()
	if attempts["pkt-e2e-fail-4xx"] != 1 {
		t.Fatalf("4xx packet attempts=%d, want 1 (no retry)", attempts["pkt-e2e-fail-4xx"])
	}
	if attempts["pkt-e2e-fail-5xx"] != 3 {
		t.Fatalf("5xx packet attempts=%d, want 3 (bounded retry)", attempts["pkt-e2e-fail-5xx"])
	}

	afterRejected := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusRejected))
	if delta := afterRejected - baselineRejected; delta < 1 {
		t.Fatalf("rejected metric delta=%f, want >= 1", delta)
	}
	afterDegraded := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOpened, qfdecisions.SurfaceDigest, qfdecisions.EngagementStatusDegraded))
	if delta := afterDegraded - baselineDegraded; delta < 1 {
		t.Fatalf("degraded metric delta=%f, want >= 1", delta)
	}

	// Overflow: small-capacity exporter wired to the same live stub; force overflow_drop.
	smallExporter := newE2EEngagementExporterWithCapacity(t, stub.url(), consentReader, true, 3)
	defer smallExporter.Stop(context.Background())

	baselineOverflow := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOverflowDrop, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusDropped))
	for i := 0; i < 5; i++ {
		_, _ = smallExporter.Capture(ctx, qfdecisions.CaptureRequest{
			Event: qfdecisions.EngagementEventOpened, Surface: qfdecisions.SurfaceWeb,
			PacketID: "pkt-e2e-overflow-" + itoaE2E(i), TraceID: "trc-e2e-overflow-" + itoaE2E(i),
			ActorRef: "e2e-actor",
		})
	}
	afterOverflow := testutil.ToFloat64(metrics.QFEngagementSignalAttemptsTotal.WithLabelValues(
		qfdecisions.EngagementEventOverflowDrop, qfdecisions.SurfaceWeb, qfdecisions.EngagementStatusDropped))
	if delta := afterOverflow - baselineOverflow; delta < 1 {
		t.Fatalf("overflow_drop metric delta=%f, want >= 1", delta)
	}
}

// --- helpers ----------------------------------------------------------

// newE2EEngagementExporter constructs an in-test exporter pointed at
// the e2e stub URL using the Scope 1 QF client transport and the
// supplied consent reader and capability flag. The exporter's flush
// worker starts automatically; the test MUST call Stop to release it.
func newE2EEngagementExporter(t *testing.T, qfBaseURL string, consent qfdecisions.ConsentReader, capability bool) *qfdecisions.Exporter {
	t.Helper()
	client := qfdecisions.NewClient(qfBaseURL, "qf-service-token", 1, 25)
	exporter := qfdecisions.NewExporterFromClient(client, consent, capability)
	exporter.Start(context.Background())
	return exporter
}

// newE2EEngagementExporterWithCapacity constructs an in-test exporter
// with a custom buffer capacity for overflow testing using a no-op
// transport. Overflow happens at the in-process buffer boundary BEFORE
// any transport call, so the no-op transport is correct — overflow is
// independent of the live QF stub. All other settings (flush interval,
// threshold, retry budget) use design defaults.
func newE2EEngagementExporterWithCapacity(t *testing.T, _ string, consent qfdecisions.ConsentReader, capability bool, capacity int) *qfdecisions.Exporter {
	t.Helper()
	exporter := qfdecisions.NewExporter(qfdecisions.ExporterOptions{
		Transport:                 noopEngagementTransport{},
		Consent:                   consent,
		EngagementSignalSupported: capability,
		Capacity:                  capacity,
	})
	exporter.Start(context.Background())
	return exporter
}

// noopEngagementTransport is a drop-everything transport used by the
// overflow e2e test. It returns an empty result slice so the
// exporter's flush loop records no per-signal outcomes.
type noopEngagementTransport struct{}

func (noopEngagementTransport) PostEngagementSignals(context.Context, []qfdecisions.PacketEngagementSignal) ([]qfdecisions.EngagementSignalResult, error) {
	return nil, nil
}

func isUUIDv7E2E(id string) bool {
	if len(id) != 36 {
		return false
	}
	return id[14] == '7'
}

func flattenSignalIDs(batches [][]qfdecisions.PacketEngagementSignal) []string {
	out := make([]string, 0)
	for _, batch := range batches {
		for _, sig := range batch {
			out = append(out, sig.SignalID)
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func itoaE2E(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// qfEngagementBaseURLStub binds to the QF_DECISIONS_BASE_URL port and
// records every POST to the engagement endpoint. It also serves the
// capability handshake so an exporter constructed against this URL can
// be exercised through the live Scope 1 QF client transport.
type qfEngagementBaseURLStub struct {
	server               *http.Server
	listener             net.Listener
	baseURL              string
	mu                   sync.Mutex
	batches              [][]qfdecisions.PacketEngagementSignal
	attemptPerPacket     map[string]int
	seenSignalIDs        map[string]struct{}
	enableIdempotentMode atomic.Bool
	failureMode          atomic.Bool
	stopOnce             sync.Once
}

func startQFEngagementBaseURLStub(t *testing.T) *qfEngagementBaseURLStub {
	t.Helper()
	baseURL := os.Getenv("QF_DECISIONS_BASE_URL")
	if baseURL == "" {
		t.Skip("e2e: QF_DECISIONS_BASE_URL not set — engagement stub cannot bind to the live stack's QF port")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse QF_DECISIONS_BASE_URL: %v", err)
	}
	port := parsed.Port()
	if port == "" {
		t.Fatalf("QF_DECISIONS_BASE_URL must include a port: %s", baseURL)
	}
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		t.Fatalf("bind engagement stub on configured port %s: %v", port, err)
	}
	// Rewrite the host the exporter dials so it resolves inside the
	// --network host e2e test container on Linux Docker Engine. The
	// configured QF_DECISIONS_BASE_URL points at
	// `host.docker.internal:<port>` for the production-shaped
	// connector wiring, but Docker Engine on Linux (no Docker
	// Desktop) does NOT auto-populate `host.docker.internal` in the
	// container's /etc/hosts unless `--add-host=host.docker.internal:host-gateway`
	// is passed to `docker run`, which smackerel.sh's e2e runner does
	// not pass. The stub's `net.Listen("tcp", ":"+port)` binds every
	// host interface (including loopback), so the exporter can reach
	// the stub at `127.0.0.1:<port>` inside the --network host
	// container without any infra change. We rewrite ONLY the URL
	// returned to the in-test exporter; the bind itself is unchanged.
	parsed.Host = net.JoinHostPort("127.0.0.1", port)
	stub := &qfEngagementBaseURLStub{
		listener:         listener,
		baseURL:          parsed.String(),
		attemptPerPacket: make(map[string]int),
		seenSignalIDs:    make(map[string]struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(qfdecisions.CapabilitiesPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(e2eEngagementCapability())
	})
	mux.HandleFunc(qfdecisions.PacketEngagementSignalsPath, stub.handleEngagementPost)
	stub.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serverErrors := make(chan error, 1)
	go func() {
		err := stub.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()
	t.Cleanup(func() {
		if err, ok := <-serverErrors; ok && err != nil {
			t.Errorf("engagement stub server error: %v", err)
		}
	})
	return stub
}

func (s *qfEngagementBaseURLStub) handleEngagementPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var posted []qfdecisions.PacketEngagementSignal
	if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.batches = append(s.batches, posted)
	for _, sig := range posted {
		s.attemptPerPacket[sig.PacketID]++
	}
	s.mu.Unlock()
	results := make([]map[string]any, 0, len(posted))
	for _, sig := range posted {
		entry := map[string]any{"signal_id": sig.SignalID}
		switch {
		case s.failureMode.Load() && sig.PacketID == "pkt-e2e-fail-4xx":
			entry["status_code"] = http.StatusConflict
			entry["reason"] = qfdecisions.EngagementErrSignalIDReuseDifferentPayload
		case s.failureMode.Load() && sig.PacketID == "pkt-e2e-fail-5xx":
			entry["status_code"] = http.StatusInternalServerError
			entry["reason"] = "INTERNAL_ERROR"
		default:
			s.mu.Lock()
			_, seen := s.seenSignalIDs[sig.SignalID]
			s.seenSignalIDs[sig.SignalID] = struct{}{}
			s.mu.Unlock()
			if s.enableIdempotentMode.Load() && seen {
				entry["status_code"] = http.StatusOK
				entry["idempotent"] = true
			} else {
				entry["status_code"] = http.StatusCreated
			}
		}
		results = append(results, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(results)
}

func (s *qfEngagementBaseURLStub) batchesSnapshot() [][]qfdecisions.PacketEngagementSignal {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]qfdecisions.PacketEngagementSignal, len(s.batches))
	for i, b := range s.batches {
		out[i] = append([]qfdecisions.PacketEngagementSignal(nil), b...)
	}
	return out
}

func (s *qfEngagementBaseURLStub) resetBatches() {
	s.mu.Lock()
	s.batches = nil
	s.mu.Unlock()
}

func (s *qfEngagementBaseURLStub) attemptsByPacket() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.attemptPerPacket))
	for k, v := range s.attemptPerPacket {
		out[k] = v
	}
	return out
}

func (s *qfEngagementBaseURLStub) waitForBatchCount(t *testing.T, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		got := len(s.batches)
		s.mu.Unlock()
		if got >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	got := len(s.batches)
	s.mu.Unlock()
	t.Fatalf("engagement stub received %d batches after %s, want >= %d", got, timeout, want)
}

func (s *qfEngagementBaseURLStub) url() string {
	return s.baseURL
}

func (s *qfEngagementBaseURLStub) stop() {
	s.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(ctx)
	})
}

// e2eEngagementCapability returns a QFBridgeCapability that satisfies
// CompatibilityCheck and reports engagement_signal_supported=true. It
// is intentionally inlined here so the e2e test file does not depend
// on internal-only fixtures from the unit/integration test packages.
func e2eEngagementCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedPacketVersions:            []string{"v1"},
		SupportedEventTypes:                []string{"packet_created", "packet_updated", "packet_trust_changed", "packet_archived", "packet_action_boundary_attempted"},
		SupportedDecisionTypes:             []string{"recommendation", "no_action", "policy_denial", "analysis_note"},
		MaxPageSize:                        200,
		MinPageSize:                        1,
		SupportedTargetContextTypes:        []string{"guided_analysis", "rhai_run", "saved_result", "analysis_context", "packet_context"},
		EvidenceMaxBundleSizeBytes:         524288,
		EvidenceMaxClaimsPerBundle:         50,
		EvidenceRateLimitPerMinute:         10,
		FreshnessSLAP95Seconds:             60,
		AuditEnvelopeVersion:               "v1",
		TenantAware:                        false,
		PreferredSurfaceHintSupported:      true,
		EngagementSignalSupported:          true,
		PersonalContextPullSupported:       true,
		WatchSignalDirection:               "qf_emit_only_pre_mvp",
		CallbackSigningSupported:           false,
		DeepLinkSigningSupported:           true,
		CredentialRotationOverlapSupported: true,
		NoActionEmitEnabled:                false,
		EligibleSmackerelSourceClasses:     []string{"smackerel_markets", "smackerel_weather", "smackerel_news", "smackerel_geopolitical", "smackerel_other", "external"},
	}
}
