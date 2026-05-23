package qfdecisions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Scope 6 (SCN-SM-041-022..024) — Packet Engagement Signal Exporter.
//
// The exporter captures per-packet engagement events from the three
// rendering surfaces (web, digest, telegram), buffers them in an
// in-memory ring, and flushes them to QF's
// `POST /api/private/smackerel/v1/packet-engagement-signals` endpoint
// (PacketEngagementSignalsPath) on either a 10-second timer or a
// 100-event threshold (whichever fires first). The exporter is write-only:
// captured signals NEVER feed back into local rendering, ranking, digest
// priority, recommendation surfaces, or trust metadata. The buffer is
// purely in-memory and is NOT persisted across process restarts; signal
// loss on crash is acceptable for calibration-grade telemetry.
//
// Per the Smackerel Product Principle 10 boundary (Smackerel observes QF
// without directing it), engagement signals are one-way Smackerel→QF
// observability; the corresponding return path (which would influence
// QF mandate, watch list, trade approval, or execution) is forbidden.

// Engagement event constants. The `event` label vocabulary on
// smackerel_qf_engagement_signal_attempts_total admits exactly these
// values plus the overflow-drop sentinel.
const (
	EngagementEventOpened       = "opened"
	EngagementEventDwell        = "dwell"
	EngagementEventDismissed    = "dismissed"
	EngagementEventSnoozed      = "snoozed"
	EngagementEventDeepLinked   = "deep_linked"
	EngagementEventShared       = "shared"
	EngagementEventOverflowDrop = "overflow_drop"
)

// Engagement status constants (metric `status` label vocabulary).
const (
	EngagementStatusAccepted = "accepted"
	EngagementStatusRejected = "rejected"
	EngagementStatusDegraded = "degraded"
	EngagementStatusDropped  = "dropped"
)

// Engagement audit-envelope outcome constants. The `outcome` field on
// Cross-Product Audit Envelope v1 records emitted by the engagement
// exporter is one of the canonical AuditOutcome* constants for the
// happy/4xx paths, and `degraded` for the 5xx-exhausted and overflow
// paths (per design.md §Failure Handling).
const EngagementAuditOutcomeDegraded = "degraded"

// Engagement consent scope constants. The user's privacy-settings
// `engagement_telemetry` preference is one of these values; `off`
// bypasses the buffer at event-capture time.
const (
	EngagementConsentOff           = "off"
	EngagementConsentAnonymous     = "engagement_telemetry_anonymous"
	EngagementConsentPseudonymous  = "engagement_telemetry_pseudonymous"
	EngagementConsentRawAnonymous  = "anonymous"
	EngagementConsentRawPseudonym  = "pseudonymous"
	EngagementConsentRawOff        = "off"
	engagementConsentInputAnonRaw  = "anonymous"
	engagementConsentInputPseudRaw = "pseudonymous"
	engagementConsentInputOffRaw   = "off"
)

// QF-side engagement signal error codes (design.md §Failure Handling).
// These are the `reason` strings populated on 4xx responses; they are
// surfaced in the Cross-Product Audit Envelope v1 `reason` field for
// the corresponding rejected outcome.
const (
	EngagementErrSignalIDReuseDifferentPayload = "ENGAGEMENT_SIGNAL_ID_REUSE_WITH_DIFFERENT_PAYLOAD"
	EngagementErrPacketNotFound                = "ENGAGEMENT_PACKET_NOT_FOUND"
	EngagementErrTraceIDMismatch               = "ENGAGEMENT_TRACE_ID_MISMATCH"
	EngagementErrConsentRequired               = "ENGAGEMENT_CONSENT_REQUIRED"
	EngagementErrDwellFieldMismatch            = "ENGAGEMENT_DWELL_FIELD_MISMATCH"
	EngagementErrBufferOverflow                = "ENGAGEMENT_BUFFER_OVERFLOW"
	EngagementErrTransportFailed               = "ENGAGEMENT_TRANSPORT_FAILED"
)

// Engagement flush policy constants. These are fixed per the QF bridge
// contract (design.md §Packet Engagement Signal Exporter) and are NOT
// runtime-configurable; values that would change semantics are owned
// by the bridge contract, not by Smackerel deployments.
const (
	engagementFlushInterval    = 10 * time.Second
	engagementFlushThreshold   = 100
	engagementBufferCapacity   = 1024
	engagementMaxRetryAttempts = 3
	engagementInitialBackoff   = 100 * time.Millisecond
	engagementMaxBackoff       = 2 * time.Second
	engagementTransportTimeout = 5 * time.Second
)

// PacketEngagementSignal is the per-event envelope POSTed to QF on
// flush. The shape is the source of truth for SCN-SM-041-023 and is
// mirrored by design.md §Signal Envelope. DwellSeconds is omitted from
// the JSON encoding when the event is not `dwell` so QF can validate
// the dwell-required field without seeing a spurious zero on
// opened/dismissed/snoozed/deep_linked/shared events.
type PacketEngagementSignal struct {
	SignalID        string    `json:"signal_id"`
	PacketID        string    `json:"packet_id"`
	TraceID         string    `json:"trace_id"`
	EngagementEvent string    `json:"engagement_event"`
	EngagementTS    time.Time `json:"engagement_ts"`
	Surface         string    `json:"surface"`
	ConsentScope    string    `json:"consent_scope"`
	ActorRef        string    `json:"actor_ref"`
	// DwellSeconds is required ONLY for `dwell` events; omitted on
	// every other event type per design.md §Signal Envelope.
	DwellSeconds *int `json:"dwell_seconds,omitempty"`
}

// ConsentReader returns the user's `engagement_telemetry` privacy
// preference at event-capture time. Implementations MUST be safe for
// concurrent calls. The returned value SHOULD be one of:
//
//   - `off` — engagement signals are bypassed entirely at capture time
//   - `anonymous` / `engagement_telemetry_anonymous`
//   - `pseudonymous` / `engagement_telemetry_pseudonymous`
//
// Unknown values are treated as `off` (fail-closed) so a misconfigured
// privacy reader cannot leak engagement data without explicit consent.
type ConsentReader interface {
	EngagementTelemetryPreference(ctx context.Context) string
}

// ConsentReaderFunc adapts a function to the ConsentReader interface
// so callers can pass a plain function without a struct.
type ConsentReaderFunc func(ctx context.Context) string

func (f ConsentReaderFunc) EngagementTelemetryPreference(ctx context.Context) string {
	if f == nil {
		return EngagementConsentOff
	}
	return f(ctx)
}

// EngagementTransport posts a batch of engagement signals to QF and
// returns the per-signal HTTP outcomes. Implementations MUST not retry
// internally; the Exporter owns the bounded retry policy.
type EngagementTransport interface {
	PostEngagementSignals(ctx context.Context, signals []PacketEngagementSignal) ([]EngagementSignalResult, error)
}

// EngagementSignalResult is the per-signal outcome of a flush POST. The
// Exporter consumes these results to update metrics + audit envelopes
// and to drive the retry/drop policy.
type EngagementSignalResult struct {
	SignalID     string
	StatusCode   int
	Reason       string
	Message      string
	Idempotent   bool // true when QF replied with HTTP 200 idempotent-repeat
	Retryable    bool // true when QF replied with 5xx or a transport timeout
	NetworkError bool // true when the transport failed before QF responded
}

// engagementClientTransport adapts the Scope 1 QF *Client to the
// EngagementTransport interface so the exporter reuses the connector's
// auth, TLS, and timeout configuration.
type engagementClientTransport struct {
	client *Client
}

func newEngagementClientTransport(client *Client) *engagementClientTransport {
	return &engagementClientTransport{client: client}
}

// PostEngagementSignals POSTs the supplied batch to QF and returns
// per-signal results. The batch is sent as a single JSON array; QF
// MUST reply with a JSON array of per-signal results carrying the
// `signal_id` echoed back so the Exporter can match results to inputs.
// On network failure every signal is reported as a transport timeout
// so the retry policy treats the batch uniformly.
func (t *engagementClientTransport) PostEngagementSignals(ctx context.Context, signals []PacketEngagementSignal) ([]EngagementSignalResult, error) {
	if t == nil || t.client == nil {
		return nil, errors.New("qf-decisions: engagement transport has no client")
	}
	if len(signals) == 0 {
		return nil, nil
	}
	status, body, err := t.client.doJSON(ctx, http.MethodPost, PacketEngagementSignalsPath, signals)
	if err != nil {
		// Network or context error: treat every signal as a transport
		// failure so the Exporter's retry policy can decide whether
		// to back off and retry the batch or drop it.
		results := make([]EngagementSignalResult, len(signals))
		for i, sig := range signals {
			results[i] = EngagementSignalResult{
				SignalID:     sig.SignalID,
				Reason:       EngagementErrTransportFailed,
				Message:      err.Error(),
				Retryable:    true,
				NetworkError: true,
			}
		}
		return results, err
	}
	return decodeEngagementResults(signals, status, body), nil
}

// decodeEngagementResults parses QF's per-signal flush response. When
// QF replies with a structured JSON array of results the per-signal
// outcomes are read verbatim; when QF replies with a bare status (no
// body or a non-array body) every signal in the batch is assigned the
// same outcome derived from the HTTP status code.
func decodeEngagementResults(signals []PacketEngagementSignal, status int, body []byte) []EngagementSignalResult {
	results := make([]EngagementSignalResult, 0, len(signals))
	// Try to decode the structured response shape first.
	var perSignal []struct {
		SignalID   string `json:"signal_id"`
		Status     int    `json:"status,omitempty"`
		StatusCode int    `json:"status_code,omitempty"`
		Reason     string `json:"reason,omitempty"`
		Code       string `json:"code,omitempty"`
		Message    string `json:"message,omitempty"`
		Idempotent bool   `json:"idempotent,omitempty"`
	}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &perSignal); err == nil && len(perSignal) > 0 {
			index := make(map[string]int, len(perSignal))
			for i, entry := range perSignal {
				index[entry.SignalID] = i
			}
			for _, sig := range signals {
				idx, ok := index[sig.SignalID]
				if !ok {
					results = append(results, resultFromStatus(sig, status, "", ""))
					continue
				}
				entry := perSignal[idx]
				code := entry.StatusCode
				if code == 0 {
					code = entry.Status
				}
				if code == 0 {
					code = status
				}
				reason := entry.Reason
				if reason == "" {
					reason = entry.Code
				}
				results = append(results, EngagementSignalResult{
					SignalID:   sig.SignalID,
					StatusCode: code,
					Reason:     reason,
					Message:    entry.Message,
					Idempotent: entry.Idempotent || code == http.StatusOK,
					Retryable:  code >= http.StatusInternalServerError,
				})
			}
			return results
		}
	}
	// Fallback: every signal inherits the batch-level status. Try to
	// pull a top-level error code from a non-array body so the audit
	// envelope still carries a meaningful `reason` for 4xx replies.
	reason := ""
	if status >= http.StatusBadRequest {
		var topErr struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &topErr)
		reason = topErr.Code
	}
	for _, sig := range signals {
		results = append(results, resultFromStatus(sig, status, reason, ""))
	}
	return results
}

func resultFromStatus(sig PacketEngagementSignal, status int, reason, message string) EngagementSignalResult {
	return EngagementSignalResult{
		SignalID:   sig.SignalID,
		StatusCode: status,
		Reason:     reason,
		Message:    message,
		Idempotent: status == http.StatusOK,
		Retryable:  status >= http.StatusInternalServerError,
	}
}

// Exporter is the Scope 6 packet engagement signal exporter. It is
// constructed by the connector when the QF capability response reports
// `engagement_signal_supported=true` and is shut down (drained) when
// the connector closes. When the capability flag is false the
// Exporter is constructed in a disabled state — Capture is a no-op,
// the buffer is never allocated, and no flush worker runs — so the
// "capability gate" is enforced at construction time per scopes.md
// SCN-SM-041-022.
type Exporter struct {
	transport      EngagementTransport
	consent        ConsentReader
	capable        bool
	clock          func() time.Time
	uuidFactory    func() (string, error)
	flushInterval  time.Duration
	flushThresh    int
	capacity       int
	maxAttempts    int
	initialBackoff time.Duration
	maxBackoff     time.Duration

	mu       sync.Mutex
	buffer   []PacketEngagementSignal
	closed   bool
	flushCh  chan struct{}
	doneCh   chan struct{}
	stopCh   chan struct{}
	workerWG sync.WaitGroup
}

// ExporterOptions configures a Scope 6 engagement exporter.
type ExporterOptions struct {
	Transport ConfiguredEngagementTransport
	Consent   ConsentReader
	// EngagementSignalSupported mirrors the persisted QF capability
	// field; when false the exporter is constructed in a disabled
	// state and Capture is a no-op forever.
	EngagementSignalSupported bool
	// Clock returns the current time; defaults to time.Now.UTC.
	Clock func() time.Time
	// UUIDFactory returns a fresh UUIDv7 string; defaults to
	// google/uuid NewV7. Provided for tests that need deterministic
	// signal IDs.
	UUIDFactory func() (string, error)
	// FlushInterval overrides the default 10-second timer. Tests that
	// exercise the timer-trigger path should set this to a short
	// duration so the test does not have to sleep for 10s.
	FlushInterval time.Duration
	// FlushThreshold overrides the default 100-event threshold.
	FlushThreshold int
	// Capacity overrides the default in-memory ring capacity.
	Capacity int
	// MaxAttempts overrides the default 3-attempt retry cap.
	MaxAttempts int
	// InitialBackoff overrides the default 100ms first retry delay.
	InitialBackoff time.Duration
	// MaxBackoff caps the exponential backoff at this value.
	MaxBackoff time.Duration
}

// ConfiguredEngagementTransport is a typed alias so the exported
// ExporterOptions API distinguishes test-provided transports from the
// connector-provided *Client transport. Both implement
// EngagementTransport so the Exporter treats them uniformly.
type ConfiguredEngagementTransport interface {
	EngagementTransport
}

// NewExporter constructs an Exporter from the supplied options. When
// EngagementSignalSupported is false the returned Exporter is in a
// permanently-disabled state — Capture is a no-op, no flush worker is
// started, and Flush returns immediately. When true the Exporter is
// ready to accept Capture calls; the flush worker must be started
// explicitly with Start.
func NewExporter(opts ExporterOptions) *Exporter {
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	uuidFactory := opts.UUIDFactory
	if uuidFactory == nil {
		uuidFactory = defaultUUIDv7
	}
	consent := opts.Consent
	if consent == nil {
		consent = ConsentReaderFunc(func(context.Context) string { return EngagementConsentOff })
	}
	interval := opts.FlushInterval
	if interval <= 0 {
		interval = engagementFlushInterval
	}
	threshold := opts.FlushThreshold
	if threshold <= 0 {
		threshold = engagementFlushThreshold
	}
	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = engagementBufferCapacity
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = engagementMaxRetryAttempts
	}
	initialBackoff := opts.InitialBackoff
	if initialBackoff <= 0 {
		initialBackoff = engagementInitialBackoff
	}
	maxBackoff := opts.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = engagementMaxBackoff
	}
	exporter := &Exporter{
		transport:      opts.Transport,
		consent:        consent,
		capable:        opts.EngagementSignalSupported,
		clock:          clock,
		uuidFactory:    uuidFactory,
		flushInterval:  interval,
		flushThresh:    threshold,
		capacity:       capacity,
		maxAttempts:    maxAttempts,
		initialBackoff: initialBackoff,
		maxBackoff:     maxBackoff,
		flushCh:        make(chan struct{}, 1),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}
	if exporter.capable {
		exporter.buffer = make([]PacketEngagementSignal, 0, capacity)
	}
	return exporter
}

// NewExporterFromClient is a convenience constructor used by the
// connector when wiring the Scope 1 *Client transport. The capability
// flag MUST come from the persisted QF capability response so the
// "capability gate at construction time" invariant is honored.
func NewExporterFromClient(client *Client, consent ConsentReader, capable bool) *Exporter {
	return NewExporter(ExporterOptions{
		Transport:                 newEngagementClientTransport(client),
		Consent:                   consent,
		EngagementSignalSupported: capable,
	})
}

func defaultUUIDv7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate engagement signal UUIDv7: %w", err)
	}
	return id.String(), nil
}

// Enabled reports whether the Exporter will accept Capture calls. An
// Exporter constructed with EngagementSignalSupported=false is
// permanently disabled and Enabled returns false forever.
func (e *Exporter) Enabled() bool {
	if e == nil {
		return false
	}
	return e.capable
}

// CaptureRequest carries the per-event context the surface render
// hooks pass to Capture. The Exporter generates the `signal_id` and
// stamps `engagement_ts` so callers cannot drift those fields from the
// idempotency contract.
type CaptureRequest struct {
	Event        string
	Surface      string
	PacketID     string
	TraceID      string
	ActorRef     string
	DwellSeconds *int
}

// Capture enqueues an engagement event for transport to QF. Capture is
// safe to call on a nil receiver — that case is a no-op, which makes
// it safe for render call sites to fetch the global exporter and call
// Capture unconditionally even when the connector has not been
// initialised. Capture returns the buffered signal (for test
// observability) and a boolean reporting whether the event was
// enqueued. The capability gate and consent gate are enforced before
// the buffer is touched.
func (e *Exporter) Capture(ctx context.Context, req CaptureRequest) (PacketEngagementSignal, bool) {
	if e == nil || !e.capable {
		return PacketEngagementSignal{}, false
	}
	consent := normalizeEngagementConsent(e.consent.EngagementTelemetryPreference(ctx))
	if consent == EngagementConsentOff {
		return PacketEngagementSignal{}, false
	}
	event := strings.TrimSpace(req.Event)
	surface := strings.TrimSpace(req.Surface)
	if event == "" || surface == "" {
		return PacketEngagementSignal{}, false
	}
	id, err := e.uuidFactory()
	if err != nil {
		slog.Warn("qf-decisions: engagement signal id generation failed",
			slog.String("event", event), slog.String("surface", surface), slog.String("error", err.Error()))
		return PacketEngagementSignal{}, false
	}
	sig := PacketEngagementSignal{
		SignalID:        id,
		PacketID:        strings.TrimSpace(req.PacketID),
		TraceID:         strings.TrimSpace(req.TraceID),
		EngagementEvent: event,
		EngagementTS:    e.clock(),
		Surface:         surface,
		ConsentScope:    consent,
		ActorRef:        strings.TrimSpace(req.ActorRef),
		DwellSeconds:    req.DwellSeconds,
	}
	if !e.enqueue(sig) {
		// enqueue handled overflow accounting; the signal is dropped
		// but Capture still returns the would-have-been-buffered
		// envelope so callers can correlate logs.
		return sig, false
	}
	return sig, true
}

// enqueue appends `sig` to the in-memory ring. When the ring is at
// capacity the oldest entry is dropped, an overflow metric +
// audit-envelope record are emitted, and the new signal takes the
// dropped slot so the freshest events are preserved. enqueue also
// triggers the flush worker when the threshold is reached.
func (e *Exporter) enqueue(sig PacketEngagementSignal) bool {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return false
	}
	if len(e.buffer) >= e.capacity {
		dropped := e.buffer[0]
		e.buffer = append(e.buffer[:0], e.buffer[1:]...)
		e.buffer = append(e.buffer, sig)
		bufferLen := len(e.buffer)
		e.mu.Unlock()
		recordEngagementOverflowDrop(dropped)
		if bufferLen >= e.flushThresh {
			e.signalFlush()
		}
		return true
	}
	e.buffer = append(e.buffer, sig)
	bufferLen := len(e.buffer)
	e.mu.Unlock()
	if bufferLen >= e.flushThresh {
		e.signalFlush()
	}
	return true
}

// recordEngagementOverflowDrop emits the metric + audit envelope for an
// overflow-dropped signal. The dropped signal's surface and trace/packet
// context are preserved so operators can correlate the drop with the
// originating engagement event.
func recordEngagementOverflowDrop(dropped PacketEngagementSignal) {
	RecordQFEngagementSignalAttempt(EngagementEventOverflowDrop, dropped.Surface, EngagementStatusDropped)
	_ = EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
		SignalID:   dropped.SignalID,
		TraceID:    dropped.TraceID,
		PacketID:   dropped.PacketID,
		ActorRef:   dropped.ActorRef,
		Surface:    dropped.Surface,
		Event:      EngagementEventOverflowDrop,
		Status:     EngagementAuditOutcomeDegraded,
		Reason:     EngagementErrBufferOverflow,
		ObservedAt: dropped.EngagementTS,
	})
}

// normalizeEngagementConsent maps the user-facing privacy preference
// vocabulary to the canonical envelope `consent_scope` value. Unknown
// values fall through to `off` (fail-closed) so a misconfigured
// privacy reader cannot leak engagement data without explicit consent.
func normalizeEngagementConsent(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case engagementConsentInputAnonRaw, EngagementConsentAnonymous:
		return EngagementConsentAnonymous
	case engagementConsentInputPseudRaw, EngagementConsentPseudonymous:
		return EngagementConsentPseudonymous
	case engagementConsentInputOffRaw, "":
		return EngagementConsentOff
	default:
		return EngagementConsentOff
	}
}

// Start launches the flush worker goroutine. Calling Start on a
// disabled Exporter is a no-op. Calling Start more than once panics.
func (e *Exporter) Start(ctx context.Context) {
	if e == nil || !e.capable {
		return
	}
	e.workerWG.Add(1)
	go e.run(ctx)
}

// Stop signals the flush worker to drain the buffer once more and
// then exit. Stop blocks until the worker has returned. It is safe to
// call Stop on a disabled Exporter (no-op) and to call it multiple
// times (subsequent calls return immediately).
func (e *Exporter) Stop(ctx context.Context) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()
	if e.capable {
		close(e.stopCh)
		e.workerWG.Wait()
	}
}

// signalFlush wakes the flush worker without blocking. A coalescing
// channel of capacity 1 ensures that bursts of Capture calls do not
// pile up wake-up signals.
func (e *Exporter) signalFlush() {
	select {
	case e.flushCh <- struct{}{}:
	default:
	}
}

func (e *Exporter) run(ctx context.Context) {
	defer e.workerWG.Done()
	ticker := time.NewTicker(e.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			e.drain(context.Background())
			return
		case <-e.stopCh:
			e.drain(context.Background())
			return
		case <-ticker.C:
			e.flushOnce(ctx)
		case <-e.flushCh:
			e.flushOnce(ctx)
		}
	}
}

// drain is the Stop-time companion to flushOnce: it issues a final
// flush attempt with a fresh detached context so pending signals reach
// QF even if the parent context has already been cancelled.
func (e *Exporter) drain(parent context.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), engagementTransportTimeout)
	defer cancel()
	e.flushOnce(ctx)
	// Silence unused-parameter lint when parent context is cancelled
	// before drain runs; the drain context above is intentional.
	_ = parent
}

// Flush drains the in-memory buffer through one transport attempt
// (subject to retries). It is exposed for tests that need to force a
// flush without waiting for the timer or threshold trigger.
func (e *Exporter) Flush(ctx context.Context) {
	if e == nil || !e.capable {
		return
	}
	e.flushOnce(ctx)
}

// flushOnce takes a snapshot of the buffer (under the mutex), then
// attempts to POST the snapshot through the transport with bounded
// retries. Per-signal outcomes drive metric + audit-envelope emission.
// Signals that exhaust the retry budget (5xx/timeouts) are emitted as
// `degraded` and dropped — per design.md §Failure Handling, buffered
// entries are NOT persisted across process restarts.
func (e *Exporter) flushOnce(ctx context.Context) {
	e.mu.Lock()
	if len(e.buffer) == 0 {
		e.mu.Unlock()
		return
	}
	batch := make([]PacketEngagementSignal, len(e.buffer))
	copy(batch, e.buffer)
	e.buffer = e.buffer[:0]
	e.mu.Unlock()
	if e.transport == nil {
		// No transport configured (test fixture or pre-Connect
		// state): drop everything and surface a degraded audit so the
		// operator can correlate the drop with the missing transport.
		for _, sig := range batch {
			RecordQFEngagementSignalAttempt(sig.EngagementEvent, sig.Surface, EngagementStatusDegraded)
			_ = EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
				SignalID:   sig.SignalID,
				TraceID:    sig.TraceID,
				PacketID:   sig.PacketID,
				ActorRef:   sig.ActorRef,
				Surface:    sig.Surface,
				Event:      sig.EngagementEvent,
				Status:     EngagementAuditOutcomeDegraded,
				Reason:     EngagementErrTransportFailed,
				ObservedAt: sig.EngagementTS,
			})
		}
		return
	}
	remaining := batch
	backoff := e.initialBackoff
	for attempt := 1; attempt <= e.maxAttempts && len(remaining) > 0; attempt++ {
		results, _ := e.transport.PostEngagementSignals(ctx, remaining)
		// Index results by signal id so retry/exhaustion logic can
		// distinguish the per-signal outcomes when QF returns a
		// mixed-result batch.
		resultByID := make(map[string]EngagementSignalResult, len(results))
		for _, r := range results {
			resultByID[r.SignalID] = r
		}
		next := make([]PacketEngagementSignal, 0, len(remaining))
		for _, sig := range remaining {
			res, ok := resultByID[sig.SignalID]
			if !ok {
				// QF did not echo this signal id — treat as transport
				// failure so the retry policy applies. This is the
				// fallback-status path used when the QF response is a
				// bare HTTP status without a structured body.
				res = EngagementSignalResult{
					SignalID:   sig.SignalID,
					StatusCode: 0,
					Reason:     EngagementErrTransportFailed,
					Retryable:  true,
				}
			}
			if e.handleResult(sig, res, attempt) {
				continue // signal terminally handled (accepted or hard-dropped)
			}
			// Retryable failure: queue for the next attempt unless we
			// have exhausted the attempt budget below.
			next = append(next, sig)
		}
		remaining = next
		if len(remaining) == 0 {
			return
		}
		if attempt == e.maxAttempts {
			break
		}
		// Wait for the backoff window OR a context cancellation. The
		// inner select uses a goto fall-through pattern (via the
		// `cancelled` flag) so a parent-context cancellation breaks
		// the OUTER retry loop, not just the wait select.
		cancelled := false
		select {
		case <-ctx.Done():
			cancelled = true
		case <-time.After(backoff):
		}
		if cancelled {
			break
		}
		backoff *= 2
		if backoff > e.maxBackoff {
			backoff = e.maxBackoff
		}
	}
	// Any signals still remaining after the retry budget is exhausted
	// are emitted as `degraded` and dropped (no persistence).
	for _, sig := range remaining {
		RecordQFEngagementSignalAttempt(sig.EngagementEvent, sig.Surface, EngagementStatusDegraded)
		_ = EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
			SignalID:   sig.SignalID,
			TraceID:    sig.TraceID,
			PacketID:   sig.PacketID,
			ActorRef:   sig.ActorRef,
			Surface:    sig.Surface,
			Event:      sig.EngagementEvent,
			Status:     EngagementAuditOutcomeDegraded,
			Reason:     EngagementErrTransportFailed,
			ObservedAt: sig.EngagementTS,
		})
	}
}

// handleResult applies the per-signal outcome from a flush attempt.
// Returns true when the signal is terminally handled (accepted on
// HTTP 201, idempotent no-op on HTTP 200, or hard-dropped on 4xx).
// Returns false when the signal MUST be retried by the caller. The
// `attempt` argument is reserved for callers that need to attach the
// retry index to per-attempt telemetry; it is unused by the default
// per-signal accounting.
func (e *Exporter) handleResult(sig PacketEngagementSignal, res EngagementSignalResult, attempt int) bool {
	_ = attempt
	switch {
	case res.StatusCode == http.StatusCreated:
		RecordQFEngagementSignalAttempt(sig.EngagementEvent, sig.Surface, EngagementStatusAccepted)
		_ = EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
			SignalID:   sig.SignalID,
			TraceID:    sig.TraceID,
			PacketID:   sig.PacketID,
			ActorRef:   sig.ActorRef,
			Surface:    sig.Surface,
			Event:      sig.EngagementEvent,
			Status:     EngagementStatusAccepted,
			ObservedAt: sig.EngagementTS,
		})
		return true
	case res.StatusCode == http.StatusOK && res.Idempotent:
		// HTTP 200 idempotent-repeat MUST NOT emit a duplicate audit
		// envelope per SCN-SM-041-023. The metric still increments
		// with status="accepted" because the signal was accepted by
		// QF on the original POST and the retry simply confirmed
		// receipt.
		RecordQFEngagementSignalAttempt(sig.EngagementEvent, sig.Surface, EngagementStatusAccepted)
		return true
	case res.StatusCode >= http.StatusBadRequest && res.StatusCode < http.StatusInternalServerError:
		// 4xx (including 409 reuse-with-different-payload): drop
		// without retry. Per SCN-SM-041-024 the audit envelope
		// outcome is `rejected` and `reason` carries the QF error
		// code so downstream consumers can correlate the drop.
		RecordQFEngagementSignalAttempt(sig.EngagementEvent, sig.Surface, EngagementStatusRejected)
		_ = EmitEngagementSignalFlushAudit(EngagementSignalAuditInput{
			SignalID:   sig.SignalID,
			TraceID:    sig.TraceID,
			PacketID:   sig.PacketID,
			ActorRef:   sig.ActorRef,
			Surface:    sig.Surface,
			Event:      sig.EngagementEvent,
			Status:     EngagementStatusRejected,
			Reason:     fallbackReason(res.Reason, EngagementErrTransportFailed),
			ObservedAt: sig.EngagementTS,
		})
		return true
	default:
		// 5xx, transport timeout, or unknown response: retry until
		// the attempt budget is exhausted (caller's `attempt` counter
		// drives the bounded backoff schedule).
		return false
	}
}

func fallbackReason(reason, fallback string) string {
	if strings.TrimSpace(reason) == "" {
		return fallback
	}
	return reason
}

// BufferLen reports the current in-memory buffer length. Exposed for
// tests that need to assert overflow/threshold/flush behavior.
func (e *Exporter) BufferLen() int {
	if e == nil || !e.capable {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.buffer)
}

// Package-level global pointer to the active engagement exporter. The
// connector sets/clears this in Connect/Close so the three rendering
// surfaces (web, digest, telegram) can call CaptureEngagementOpened
// without depending on connector-specific wiring.
var (
	globalExporterMu sync.RWMutex
	globalExporter   *Exporter
)

// SetGlobalEngagementExporter installs `e` as the active exporter that
// CaptureEngagementOpened consults. Passing nil resets the global to
// the disabled state. The connector calls this on Connect/Close.
func SetGlobalEngagementExporter(e *Exporter) {
	globalExporterMu.Lock()
	globalExporter = e
	globalExporterMu.Unlock()
}

// GlobalEngagementExporter returns the currently-active exporter or
// nil when no exporter has been installed. Safe for concurrent use.
func GlobalEngagementExporter() *Exporter {
	globalExporterMu.RLock()
	defer globalExporterMu.RUnlock()
	return globalExporter
}

// CaptureEngagementOpened is the convenience entry point used by the
// three render surfaces. It fetches the global exporter (nil-safe)
// and enqueues an `opened` event for the supplied packet/trace/surface
// combination. Callers MUST NOT call this when no surface render has
// occurred — the event signals user-visible delivery of the packet.
func CaptureEngagementOpened(ctx context.Context, surface, packetID, traceID, actorRef string) {
	exporter := GlobalEngagementExporter()
	if exporter == nil {
		return
	}
	_, _ = exporter.Capture(ctx, CaptureRequest{
		Event:    EngagementEventOpened,
		Surface:  surface,
		PacketID: packetID,
		TraceID:  traceID,
		ActorRef: actorRef,
	})
}
