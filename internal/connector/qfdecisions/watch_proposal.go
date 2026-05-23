package qfdecisions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WatchProposalPath is the QF Companion Bridge endpoint that accepts
// signed pre-MVP watch-signal proposals. The POSTed JSON body is a
// WatchProposalEnvelope with `signature` and `key_id` populated by the
// Scope 8 keystore. Source of truth for the path: spec 063 §"POST
// /api/private/smackerel/v1/watch-signal-proposals" mirrored in spec
// 041 design.md §"Reserved Schemas → QFWatchSignal pre-MVP behavior".
const WatchProposalPath = "/api/private/smackerel/v1/watch-signal-proposals"

const (
	// WatchProposalSourceSmackerelPropose is the literal `source`
	// field value that MUST appear on every Smackerel-initiated
	// watch proposal. The value identifies the proposal as
	// originating from the Smackerel connector rather than from a
	// QF-internal signal generator. Per spec 041 scopes.md Scope 9
	// SCN-SM-041-031 the field is a literal, never derived from
	// user input.
	WatchProposalSourceSmackerelPropose = "smackerel_propose"

	// WatchProposalRejectionCodeDeferredV1 is the QF rejection code
	// returned for every pre-MVP watch-proposal submission. The
	// connector MUST parse this code without retry and MUST NOT
	// mutate any local watch state. Source: QF spec 063 §"Pre-MVP
	// Watch-Proposal Acceptance" mirrored in spec 041 design.md
	// §"Reserved Schemas → QFWatchSignal pre-MVP behavior".
	WatchProposalRejectionCodeDeferredV1 = "WATCH_PROPOSALS_DEFERRED_TO_V1"

	// WatchProposalStatusRejectedV1Deferred is the value emitted on
	// the `status` label of smackerel_qf_watch_proposal_attempts_total
	// when QF returns the WATCH_PROPOSALS_DEFERRED_TO_V1 rejection.
	// The connector MUST NOT retry and MUST NOT mark the proposal as
	// accepted.
	WatchProposalStatusRejectedV1Deferred = "rejected_v1_deferred"
	// WatchProposalStatusRejectedLocal is the value emitted when the
	// signer aborts BEFORE any HTTP transport — the network never
	// sees the envelope. Used in tandem with the
	// signature-failure reason vocabulary inherited from Scope 8.
	WatchProposalStatusRejectedLocal = "rejected_local"
	// WatchProposalStatusDegraded is the value emitted when transport
	// failure (timeout, network error, malformed QF body) prevents
	// the connector from observing a definitive QF response.
	WatchProposalStatusDegraded = "degraded"

	// WatchProposalSignatureFailureNoActiveKey, WatchProposalSignatureFailureMalformedCanonicalPayload,
	// and WatchProposalSignatureFailureExpiresAtOutsideTolerance are
	// aliased to the Scope 8 vocabulary so the signer-reuse contract
	// is observable from a single source of truth and so any future
	// vocabulary extension in Scope 8 propagates to Scope 9 without
	// a divergent code path. SCN-SM-041-032.
	WatchProposalSignatureFailureNoActiveKey               = CallbackSignatureFailureNoActiveKey
	WatchProposalSignatureFailureMalformedCanonicalPayload = CallbackSignatureFailureMalformedCanonicalPayload
	WatchProposalSignatureFailureExpiresAtOutsideTolerance = CallbackSignatureFailureExpiresAtOutsideTolerance
)

// allowedWatchProposalSources enumerates the pre-MVP allowed `source`
// values. Pre-MVP only `smackerel_propose` is allowed; the connector
// is the sole originator. Any other value is treated as a malformed
// canonical payload and aborted locally with the
// MALFORMED_CANONICAL_PAYLOAD reason.
var allowedWatchProposalSources = map[string]struct{}{
	WatchProposalSourceSmackerelPropose: {},
}

// WatchProposalEnvelope is the signed watch-proposal payload POSTed to
// the QF bridge endpoint. SCN-SM-041-031 / SCN-SM-041-032.
//
// Field set MUST be exactly `{trace_id, source, entity_ref, reason,
// expires_at, signature, key_id}` — no extra fields, no missing
// fields. The signature and key_id are populated by the watch-proposal
// signer (which delegates key selection to the Scope 8 keystore).
type WatchProposalEnvelope struct {
	TraceID   string `json:"trace_id"`
	Source    string `json:"source"`
	EntityRef string `json:"entity_ref"`
	Reason    string `json:"reason"`
	ExpiresAt string `json:"expires_at"`          // RFC3339, UTC
	Signature string `json:"signature,omitempty"` // lower-case hex
	KeyID     string `json:"key_id,omitempty"`
}

// ErrMalformedWatchProposalCanonicalPayload is the sentinel returned
// when canonical payload composition fails. Wrapped by typed errors
// so errors.Is works.
var ErrMalformedWatchProposalCanonicalPayload = errors.New("qf-decisions: malformed canonical watch proposal payload")

// WatchProposalCanonicalPayload composes the canonical payload form
// used as the HMAC-SHA256 input. The form is exactly:
//
//	trace_id|source|entity_ref|reason|expires_at
//
// Pipe-delimited, no whitespace, no trailing pipe. Returns
// ErrMalformedWatchProposalCanonicalPayload (wrapped with a
// descriptive message) if any field is empty, contains an illegal
// character, the source is not in the pre-MVP allowed enum, or
// expires_at is not RFC3339. SCN-SM-041-032.
func WatchProposalCanonicalPayload(env WatchProposalEnvelope) (string, error) {
	fields := []struct {
		name  string
		value string
	}{
		{"trace_id", env.TraceID},
		{"source", env.Source},
		{"entity_ref", env.EntityRef},
		{"reason", env.Reason},
		{"expires_at", env.ExpiresAt},
	}
	for _, f := range fields {
		if f.value == "" {
			return "", fmt.Errorf("%w: field %s is empty", ErrMalformedWatchProposalCanonicalPayload, f.name)
		}
		for _, ch := range illegalCanonicalPayloadCharacters {
			if strings.ContainsRune(f.value, ch) {
				return "", fmt.Errorf("%w: field %s contains illegal character %q", ErrMalformedWatchProposalCanonicalPayload, f.name, ch)
			}
		}
	}
	if _, ok := allowedWatchProposalSources[env.Source]; !ok {
		return "", fmt.Errorf("%w: source %q is not in pre-MVP enum {smackerel_propose}", ErrMalformedWatchProposalCanonicalPayload, env.Source)
	}
	if _, err := time.Parse(time.RFC3339, env.ExpiresAt); err != nil {
		return "", fmt.Errorf("%w: expires_at %q is not RFC3339: %v", ErrMalformedWatchProposalCanonicalPayload, env.ExpiresAt, err)
	}
	return strings.Join([]string{
		env.TraceID,
		env.Source,
		env.EntityRef,
		env.Reason,
		env.ExpiresAt,
	}, "|"), nil
}

// WatchProposalSignatureFailure is the typed error returned by
// WatchProposalSigner.Sign for every local signing failure. The
// `Reason` field carries the documented vocabulary inherited from
// Scope 8: {NO_ACTIVE_KEY, MALFORMED_CANONICAL_PAYLOAD,
// EXPIRES_AT_OUTSIDE_TOLERANCE}. SCN-SM-041-032.
type WatchProposalSignatureFailure struct {
	Reason string
	Cause  error
}

// Error implements the error interface.
func (e *WatchProposalSignatureFailure) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("qf-decisions: watch proposal signature failure %s", e.Reason)
	}
	return fmt.Sprintf("qf-decisions: watch proposal signature failure %s: %v", e.Reason, e.Cause)
}

// Unwrap exposes the underlying cause for errors.Is / errors.As.
func (e *WatchProposalSignatureFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WatchProposalKeystore is the verbatim Scope 8 keystore reuse
// contract. Any keystore implementation supplying SelectActiveKey
// (currently only *CallbackKeystore) satisfies the interface; Scope
// 9 holds an interface reference rather than reimplementing
// keystore selection or rotation logic. SCN-SM-041-032.
//
// The interface is intentionally narrow: it carries only the call
// Scope 9 needs to compose a signature. Any expansion to the
// keystore contract belongs in Scope 8.
type WatchProposalKeystore interface {
	SelectActiveKey(now time.Time) (CallbackSigningKey, error)
}

// Compile-time guard: *CallbackKeystore satisfies the keystore
// interface verbatim. If Scope 8 ever removes or renames
// SelectActiveKey this guard breaks the build before Scope 9 can
// drift out of sync.
var _ WatchProposalKeystore = (*CallbackKeystore)(nil)

// WatchProposalSigner signs WatchProposalEnvelopes using the newest
// `not_before`-valid key in the supplied Scope 8 keystore. The
// signer never sends the envelope over the network — transport is
// invoked by WatchProposalClient AFTER Sign returns successfully.
// Signature failures abort locally and emit the Scope 8
// signature-failure metric + Cross-Product Audit Envelope v1 record
// (the metric vocabulary is reused verbatim); the caller MUST NOT
// retry and MUST NOT POST the envelope. SCN-SM-041-032.
type WatchProposalSigner struct {
	keystore WatchProposalKeystore
	nowFn    func() time.Time
}

// NewWatchProposalSigner constructs a WatchProposalSigner backed by
// the supplied Scope 8 keystore. The keystore reuse is the source of
// truth for "verbatim Scope 8 signer/keystore reuse" required by
// SCN-SM-041-032: the same key selection algorithm is invoked, the
// same `key_id` envelope inclusion is performed, and the same HMAC
// algorithm (HMAC-SHA256, lower-case hex) is used.
//
// If nowFn is nil, the signer defaults to time.Now().UTC(). A nil
// keystore is permitted at construction time; Sign will return the
// NO_ACTIVE_KEY signature-failure record for every attempt.
func NewWatchProposalSigner(keystore WatchProposalKeystore, nowFn func() time.Time) *WatchProposalSigner {
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &WatchProposalSigner{keystore: keystore, nowFn: nowFn}
}

// Sign computes the HMAC-SHA256 signature over the canonical payload
// using the newest `not_before`-valid key and returns the envelope
// with `signature` (lower-case hex) and `key_id` populated. On any
// signature failure, Sign emits the Scope 8 signature-failure metric
// + audit envelope locally and returns a
// *WatchProposalSignatureFailure WITHOUT mutating the envelope. The
// caller MUST NOT POST the envelope when Sign returns an error.
// SCN-SM-041-032.
func (s *WatchProposalSigner) Sign(env WatchProposalEnvelope) (WatchProposalEnvelope, error) {
	now := s.nowFn().UTC()
	canonical, err := WatchProposalCanonicalPayload(env)
	if err != nil {
		return env, s.recordSignatureFailure(env, WatchProposalSignatureFailureMalformedCanonicalPayload, err, now)
	}
	expiresAt, parseErr := time.Parse(time.RFC3339, env.ExpiresAt)
	if parseErr != nil {
		return env, s.recordSignatureFailure(env, WatchProposalSignatureFailureMalformedCanonicalPayload, fmt.Errorf("%w: expires_at re-parse: %v", ErrMalformedWatchProposalCanonicalPayload, parseErr), now)
	}
	if now.Sub(expiresAt) > CallbackExpiryPastTolerance {
		return env, s.recordSignatureFailure(
			env,
			WatchProposalSignatureFailureExpiresAtOutsideTolerance,
			fmt.Errorf("%w: expires_at %s is %s past now %s (tolerance %s)",
				ErrExpiresAtOutsideTolerance,
				env.ExpiresAt,
				now.Sub(expiresAt),
				now.Format(time.RFC3339),
				CallbackExpiryPastTolerance,
			),
			now,
		)
	}
	if s.keystore == nil {
		return env, s.recordSignatureFailure(env, WatchProposalSignatureFailureNoActiveKey, errors.New("qf-decisions: watch proposal keystore is nil"), now)
	}
	key, kerr := s.keystore.SelectActiveKey(now)
	if kerr != nil {
		return env, s.recordSignatureFailure(env, WatchProposalSignatureFailureNoActiveKey, kerr, now)
	}
	mac := hmac.New(sha256.New, []byte(key.Secret))
	mac.Write([]byte(canonical))
	signed := env
	signed.Signature = hex.EncodeToString(mac.Sum(nil))
	signed.KeyID = key.KeyID
	return signed, nil
}

// recordSignatureFailure emits the Scope 8 signature-failure metric +
// Cross-Product Audit Envelope v1 record for the given reason and
// returns the typed WatchProposalSignatureFailure error. The audit
// envelope carries action="watch_proposal" outcome="rejected"
// reason=<documented vocabulary>. No HTTP transport is invoked.
func (s *WatchProposalSigner) recordSignatureFailure(env WatchProposalEnvelope, reason string, cause error, observedAt time.Time) error {
	RecordQFCallbackSignatureFailure(reason)
	EmitWatchProposalAttemptAudit(WatchProposalAttemptAuditInput{
		TraceID:    env.TraceID,
		EntityRef:  env.EntityRef,
		ActorRef:   AuditActorSmackerelConnector,
		Surface:    DefaultConnectorID,
		Status:     "rejected",
		Reason:     reason,
		ObservedAt: observedAt,
	})
	slog.Warn("qf-decisions: watch proposal signature failure",
		slog.String("reason", reason),
		slog.String("trace_id", env.TraceID),
		slog.String("entity_ref", env.EntityRef),
		slog.String("cause", errString(cause)),
	)
	return &WatchProposalSignatureFailure{Reason: reason, Cause: cause}
}

// WatchProposalQFResponse captures the QF bridge's response to a
// signed watch-proposal POST. Populated for every transport that
// reaches the network — populated with zero values when the signer
// aborts locally.
type WatchProposalQFResponse struct {
	HTTPStatus    int    `json:"http_status"`
	Body          string `json:"body"`
	RejectionCode string `json:"rejection_code,omitempty"`
}

// WatchProposalAttemptResult captures the outcome of a single
// WatchProposalClient.Propose invocation.
type WatchProposalAttemptResult struct {
	// Envelope is the envelope as it was prepared (signature and
	// key_id populated only when LocalRejection is nil).
	Envelope WatchProposalEnvelope
	// Status is the metric label value emitted for this attempt.
	Status string
	// QFResponse is the parsed QF response; zero-valued on local
	// signature failure (no network reached).
	QFResponse WatchProposalQFResponse
	// LocalRejection is non-nil when the signer aborted locally.
	LocalRejection *WatchProposalSignatureFailure
}

// watchProposalRejectionBody is the documented QF rejection envelope
// shape; only the `code` field is parsed (the message is preserved
// in the raw response body for diagnostic logging).
type watchProposalRejectionBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WatchProposalClient is the diagnostic watch-proposal client. It is
// connector-internal — NEVER wired into web/digest/Telegram user
// surfaces pre-MVP. Only the connector diagnostic path and the
// Scope 9 integration test call it. SCN-SM-041-031.
type WatchProposalClient struct {
	transport  *Client
	signer     *WatchProposalSigner
	expiriesIn time.Duration
	nowFn      func() time.Time
	traceIDFn  func() (string, error)
}

// NewWatchProposalClient constructs a diagnostic watch-proposal
// client. The Scope 1 QF transport client is reused verbatim (same
// auth, TLS, timeout pipeline). The Scope 8 keystore is reused
// verbatim via the WatchProposalSigner constructor. If nowFn is
// nil the client defaults to time.Now().UTC(). If traceIDFn is nil
// the client generates UUIDv7 trace_ids per proposal so QF can
// correlate against connector audit logs (SCN-SM-041-031).
func NewWatchProposalClient(transport *Client, signer *WatchProposalSigner, expiriesIn time.Duration, nowFn func() time.Time, traceIDFn func() (string, error)) *WatchProposalClient {
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	if traceIDFn == nil {
		traceIDFn = NewWatchProposalTraceID
	}
	if expiriesIn <= 0 {
		expiriesIn = WatchProposalDefaultExpiry
	}
	return &WatchProposalClient{
		transport:  transport,
		signer:     signer,
		expiriesIn: expiriesIn,
		nowFn:      nowFn,
		traceIDFn:  traceIDFn,
	}
}

// WatchProposalDefaultExpiry is the default per-envelope TTL when the
// caller does not supply one explicitly. Mirrors the Scope 8
// callback envelope TTL (5 minutes from issuance).
const WatchProposalDefaultExpiry = 5 * time.Minute

// Signer returns the underlying WatchProposalSigner reference for
// diagnostic display and adversarial-test signer-reuse assertions.
// SCN-SM-041-032.
func (c *WatchProposalClient) Signer() *WatchProposalSigner {
	if c == nil {
		return nil
	}
	return c.signer
}

// Transport returns the underlying *Client transport reference for
// diagnostic display.
func (c *WatchProposalClient) Transport() *Client {
	if c == nil {
		return nil
	}
	return c.transport
}

// Propose generates a trace_id, composes the envelope, signs it,
// POSTs it to the QF watch-proposal endpoint, and parses the QF
// response. Returns a WatchProposalAttemptResult describing the
// outcome. The function NEVER mutates local watch state, NEVER
// retries on `WATCH_PROPOSALS_DEFERRED_TO_V1` rejection, and NEVER
// renders a user-visible "proposal submitted" affordance —
// enforcement is structural (the client exposes no UI hook) and
// behavioral (the function returns the rejection envelope and exits
// immediately). SCN-SM-041-031 / SCN-SM-041-033.
//
// Defense-in-depth: BEFORE any signing or HTTP transport, the
// function invokes EnforceQFActionBoundary with attempted action
// type "watch_proposal" so the Scope 5 safety boundary helper has
// the opportunity to reject the attempt if a future code change
// ever adds `watch_proposal` to the forbidden action vocabulary.
// In MVP `watch_proposal` is NOT forbidden (the diagnostic path is
// the intended pre-MVP behavior), so the boundary helper is a
// no-op gate today; the explicit invocation locks in the pattern.
func (c *WatchProposalClient) Propose(ctx context.Context, entityRef, reason string, expiresAt time.Time) (WatchProposalAttemptResult, error) {
	if c == nil {
		return WatchProposalAttemptResult{Status: WatchProposalStatusDegraded}, errors.New("qf-decisions: watch proposal client is nil")
	}
	if c.signer == nil {
		return WatchProposalAttemptResult{Status: WatchProposalStatusDegraded}, errors.New("qf-decisions: watch proposal signer is nil (signing not configured)")
	}
	if c.transport == nil {
		return WatchProposalAttemptResult{Status: WatchProposalStatusDegraded}, errors.New("qf-decisions: watch proposal transport is nil (transport not configured)")
	}
	now := c.nowFn().UTC()
	expires := expiresAt.UTC()
	if expires.IsZero() {
		expires = now.Add(c.expiriesIn)
	}
	traceID, traceErr := c.traceIDFn()
	if traceErr != nil {
		return WatchProposalAttemptResult{Status: WatchProposalStatusDegraded}, fmt.Errorf("qf-decisions: watch proposal trace_id generation: %w", traceErr)
	}
	env := WatchProposalEnvelope{
		TraceID:   traceID,
		Source:    WatchProposalSourceSmackerelPropose,
		EntityRef: entityRef,
		Reason:    reason,
		ExpiresAt: expires.Format(time.RFC3339),
	}
	// Defense-in-depth safety boundary check. Today "watch_proposal"
	// is not forbidden, so this is a no-op gate; the explicit call
	// records the contract that any future change adding
	// `watch_proposal` to IsForbiddenQFActionType will cause the
	// connector to abort BEFORE any signing or transport reaches QF.
	if _, fired, gateErr := EnforceQFActionBoundary(ActionBoundaryAttempt{
		AttemptedActionType: AuditActionWatchProposalAttempt,
		TraceID:             traceID,
		ActorRef:            AuditActorSmackerelConnector,
		Surface:             DefaultConnectorID,
		Reason:              "watch_proposal_action_rejected",
		ObservedAt:          now,
	}); fired {
		EmitWatchProposalAttemptAudit(WatchProposalAttemptAuditInput{
			TraceID:    traceID,
			EntityRef:  entityRef,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    DefaultConnectorID,
			Status:     "rejected",
			Reason:     "watch_proposal_action_rejected",
			ObservedAt: now,
		})
		RecordQFWatchProposalAttempt(WatchProposalStatusRejectedLocal)
		return WatchProposalAttemptResult{Envelope: env, Status: WatchProposalStatusRejectedLocal}, gateErr
	}

	signed, signErr := c.signer.Sign(env)
	if signErr != nil {
		var failure *WatchProposalSignatureFailure
		if errors.As(signErr, &failure) {
			RecordQFWatchProposalAttempt(WatchProposalStatusRejectedLocal)
			return WatchProposalAttemptResult{
				Envelope:       env,
				Status:         WatchProposalStatusRejectedLocal,
				LocalRejection: failure,
			}, signErr
		}
		return WatchProposalAttemptResult{Envelope: env, Status: WatchProposalStatusDegraded}, signErr
	}

	status, respBody, transportErr := c.transport.doJSON(ctx, http.MethodPost, WatchProposalPath, signed)
	if transportErr != nil {
		RecordQFWatchProposalAttempt(WatchProposalStatusDegraded)
		EmitWatchProposalAttemptAudit(WatchProposalAttemptAuditInput{
			TraceID:    signed.TraceID,
			EntityRef:  signed.EntityRef,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    DefaultConnectorID,
			Status:     "error",
			Reason:     transportErr.Error(),
			ObservedAt: c.nowFn().UTC(),
		})
		return WatchProposalAttemptResult{Envelope: signed, Status: WatchProposalStatusDegraded}, transportErr
	}

	qfResp := WatchProposalQFResponse{HTTPStatus: status, Body: string(respBody)}
	if len(respBody) > 0 {
		var parsed watchProposalRejectionBody
		if jerr := json.Unmarshal(respBody, &parsed); jerr == nil {
			qfResp.RejectionCode = strings.TrimSpace(parsed.Code)
		}
	}
	result := WatchProposalAttemptResult{Envelope: signed, QFResponse: qfResp}

	switch {
	case qfResp.RejectionCode == WatchProposalRejectionCodeDeferredV1:
		// Pre-MVP rejection contract: parse without retry, never
		// mutate local watch state, never render a user surface,
		// emit the metric and audit envelope. SCN-SM-041-033.
		result.Status = WatchProposalStatusRejectedV1Deferred
		RecordQFWatchProposalAttempt(WatchProposalStatusRejectedV1Deferred)
		EmitWatchProposalAttemptAudit(WatchProposalAttemptAuditInput{
			TraceID:    signed.TraceID,
			EntityRef:  signed.EntityRef,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    DefaultConnectorID,
			Status:     "rejected",
			Reason:     WatchProposalRejectionCodeDeferredV1,
			ObservedAt: c.nowFn().UTC(),
		})
		return result, nil
	case status >= 200 && status < 300:
		// Pre-MVP `accepted` is never emitted per scopes.md Scope 9
		// Implementation Plan. Surface as degraded so the dashboard
		// catches any contract drift (QF accepting a proposal would
		// be a contract violation pre-MVP).
		result.Status = WatchProposalStatusDegraded
		RecordQFWatchProposalAttempt(WatchProposalStatusDegraded)
		EmitWatchProposalAttemptAudit(WatchProposalAttemptAuditInput{
			TraceID:    signed.TraceID,
			EntityRef:  signed.EntityRef,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    DefaultConnectorID,
			Status:     "error",
			Reason:     fmt.Sprintf("unexpected_pre_mvp_acceptance http_status=%d", status),
			ObservedAt: c.nowFn().UTC(),
		})
		return result, fmt.Errorf("qf-decisions: watch proposal returned unexpected HTTP %d acceptance pre-MVP", status)
	default:
		result.Status = WatchProposalStatusDegraded
		RecordQFWatchProposalAttempt(WatchProposalStatusDegraded)
		EmitWatchProposalAttemptAudit(WatchProposalAttemptAuditInput{
			TraceID:    signed.TraceID,
			EntityRef:  signed.EntityRef,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    DefaultConnectorID,
			Status:     "error",
			Reason:     fmt.Sprintf("http_status=%d code=%s", status, qfResp.RejectionCode),
			ObservedAt: c.nowFn().UTC(),
		})
		return result, fmt.Errorf("qf-decisions: watch proposal POST returned HTTP %d (code=%s)", status, qfResp.RejectionCode)
	}
}

// NewWatchProposalTraceID returns a fresh UUIDv7 string suitable for
// use as WatchProposalEnvelope.TraceID. The v7 format embeds a 48-bit
// Unix millisecond timestamp so trace_ids are roughly time-sortable
// for audit log scanning. Falls back to UUIDv4 if v7 generation
// fails (extremely unlikely; defense-in-depth). SCN-SM-041-031.
func NewWatchProposalTraceID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		fallback, ferr := uuid.NewRandom()
		if ferr != nil {
			return "", fmt.Errorf("generate watch proposal trace_id: %w", err)
		}
		return fallback.String(), nil
	}
	return id.String(), nil
}
