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

// CallbackPath is the QF Companion Bridge endpoint that accepts signed
// pre-MVP callback envelopes. The POSTed JSON body is a
// CallbackEnvelope with `signature` and `key_id` populated. Source of
// truth for path: spec 063 §"POST /api/private/smackerel/v1/callback".
const CallbackPath = "/api/private/smackerel/v1/callback"

const (
	// CallbackActionNoop is the no-op callback action — used as a
	// keep-alive / acknowledgement signal that does not request any
	// downstream behavior. Allowed because it does not mutate any QF
	// state.
	CallbackActionNoop = "noop"
	// CallbackActionOpen is the deep-link-open callback action — used
	// to record that the user opened a packet's deep link. Pre-MVP
	// the connector emits this as a passive observation only; QF
	// rejects the envelope with CALLBACK_DEFERRED_TO_V1 and Smackerel
	// MUST NOT treat the rejection as a failure.
	CallbackActionOpen = "open"

	// CallbackStatusRejectedV1Deferred is the value emitted on the
	// `status` label of smackerel_qf_callback_attempts_total when QF
	// returns the CALLBACK_DEFERRED_TO_V1 rejection. The connector
	// MUST NOT retry and MUST NOT mark the callback as accepted.
	CallbackStatusRejectedV1Deferred = "rejected_v1_deferred"
	// CallbackStatusRejectedLocal is the value emitted when the
	// signer aborts BEFORE any HTTP transport — the network never
	// sees the envelope. Used in tandem with the signature-failure
	// metric reason vocabulary.
	CallbackStatusRejectedLocal = "rejected_local"
	// CallbackStatusOK is the value emitted when QF returns a 2xx
	// response. Pre-MVP this code path is not exercised because QF
	// always returns the CALLBACK_DEFERRED_TO_V1 rejection; included
	// for v1 forward-compat and unit-test coverage of the status
	// label dispatcher.
	CallbackStatusOK = "ok"
	// CallbackStatusError is the value emitted when transport failure
	// (timeout, network error, malformed QF body) prevents the
	// connector from observing a definitive QF response.
	CallbackStatusError = "error"

	// CallbackSignatureFailureNoActiveKey is the documented metric
	// `reason` label and audit envelope `reason` value when no key
	// in the keystore has not_before <= now.
	CallbackSignatureFailureNoActiveKey = "NO_ACTIVE_KEY"
	// CallbackSignatureFailureMalformedCanonicalPayload is recorded
	// when canonical payload composition fails — any required field
	// is empty, contains an illegal character (pipe, CR/LF, tab),
	// the action is not in the pre-MVP {noop, open} enum, the
	// surface is not in the {telegram, web} enum, or expires_at is
	// not RFC3339.
	CallbackSignatureFailureMalformedCanonicalPayload = "MALFORMED_CANONICAL_PAYLOAD"
	// CallbackSignatureFailureExpiresAtOutsideTolerance is recorded
	// when the envelope expires_at is more than
	// CallbackExpiryPastTolerance in the past relative to "now" at
	// sign time. Future-dated expires_at is accepted (the signer is
	// not the freshness gate — QF enforces the upper bound).
	CallbackSignatureFailureExpiresAtOutsideTolerance = "EXPIRES_AT_OUTSIDE_TOLERANCE"

	// CallbackExpiryPastTolerance is the maximum allowed staleness
	// of envelope.expires_at relative to "now" at sign time. The
	// signer rejects any envelope whose expires_at is older than
	// (now - CallbackExpiryPastTolerance) and emits the
	// EXPIRES_AT_OUTSIDE_TOLERANCE signature-failure record. Spec
	// 041 Scope 8 / SCN-SM-041-030: 60 seconds.
	CallbackExpiryPastTolerance = 60 * time.Second

	// CallbackRejectionCodeDeferredV1 is the QF rejection code
	// returned for every pre-MVP callback submission. The connector
	// MUST parse this code without retry and MUST NOT mark the
	// callback as accepted. Source: QF spec 063 §"Pre-MVP Callback
	// Acceptance".
	CallbackRejectionCodeDeferredV1 = "CALLBACK_DEFERRED_TO_V1"
)

// allowedCallbackActions enumerates the pre-MVP allowed `action`
// values. Any other value is treated as a malformed canonical payload
// and aborted locally with the MALFORMED_CANONICAL_PAYLOAD reason.
var allowedCallbackActions = map[string]struct{}{
	CallbackActionNoop: {},
	CallbackActionOpen: {},
}

// allowedCallbackSurfaces enumerates the pre-MVP allowed `surface`
// values. Any other value is treated as a malformed canonical payload.
var allowedCallbackSurfaces = map[string]struct{}{
	SurfaceTelegram: {},
	SurfaceWeb:      {},
}

// illegalCanonicalPayloadCharacters lists the runes that MUST NOT
// appear in any canonical-payload field. Pipe is the field delimiter;
// CR/LF/tab corrupt the structured form when consumed by line-oriented
// log readers and HMAC verifiers.
var illegalCanonicalPayloadCharacters = []rune{'|', '\r', '\n', '\t'}

// CallbackEnvelope is the signed callback payload POSTed to the QF
// bridge endpoint. SCN-SM-041-028 / SCN-SM-041-029.
type CallbackEnvelope struct {
	CallbackID string `json:"callback_id"`
	TraceID    string `json:"trace_id"`
	PacketID   string `json:"packet_id"`
	Action     string `json:"action"`
	Nonce      string `json:"nonce"`
	ExpiresAt  string `json:"expires_at"` // RFC3339, UTC
	Surface    string `json:"surface"`
	Signature  string `json:"signature,omitempty"` // lower-case hex
	KeyID      string `json:"key_id,omitempty"`
}

// ErrMalformedCanonicalPayload is the sentinel returned when canonical
// payload composition fails. Wrapped by typed errors so errors.Is works.
var ErrMalformedCanonicalPayload = errors.New("qf-decisions: malformed canonical callback payload")

// ErrExpiresAtOutsideTolerance is the sentinel returned when the
// envelope expires_at is more than CallbackExpiryPastTolerance in the
// past relative to "now" at sign time.
var ErrExpiresAtOutsideTolerance = errors.New("qf-decisions: callback expires_at outside past-tolerance window")

// CallbackCanonicalPayload composes the canonical payload form used as
// the HMAC-SHA256 input. The form is exactly:
//
//	callback_id|trace_id|packet_id|action|nonce|expires_at|surface
//
// Pipe-delimited, no whitespace, no trailing pipe. Returns
// ErrMalformedCanonicalPayload (wrapped with a descriptive message) if
// any field is empty, contains an illegal character, the action is not
// in the pre-MVP allowed enum, the surface is not in the pre-MVP
// allowed enum, or expires_at is not RFC3339. SCN-SM-041-028.
func CallbackCanonicalPayload(env CallbackEnvelope) (string, error) {
	fields := []struct {
		name  string
		value string
	}{
		{"callback_id", env.CallbackID},
		{"trace_id", env.TraceID},
		{"packet_id", env.PacketID},
		{"action", env.Action},
		{"nonce", env.Nonce},
		{"expires_at", env.ExpiresAt},
		{"surface", env.Surface},
	}
	for _, f := range fields {
		if f.value == "" {
			return "", fmt.Errorf("%w: field %s is empty", ErrMalformedCanonicalPayload, f.name)
		}
		for _, ch := range illegalCanonicalPayloadCharacters {
			if strings.ContainsRune(f.value, ch) {
				return "", fmt.Errorf("%w: field %s contains illegal character %q", ErrMalformedCanonicalPayload, f.name, ch)
			}
		}
	}
	if _, ok := allowedCallbackActions[env.Action]; !ok {
		return "", fmt.Errorf("%w: action %q is not in pre-MVP enum {noop, open}", ErrMalformedCanonicalPayload, env.Action)
	}
	if _, ok := allowedCallbackSurfaces[env.Surface]; !ok {
		return "", fmt.Errorf("%w: surface %q is not in allowed enum {telegram, web}", ErrMalformedCanonicalPayload, env.Surface)
	}
	if _, err := time.Parse(time.RFC3339, env.ExpiresAt); err != nil {
		return "", fmt.Errorf("%w: expires_at %q is not RFC3339: %v", ErrMalformedCanonicalPayload, env.ExpiresAt, err)
	}
	return strings.Join([]string{
		env.CallbackID,
		env.TraceID,
		env.PacketID,
		env.Action,
		env.Nonce,
		env.ExpiresAt,
		env.Surface,
	}, "|"), nil
}

// CallbackSignatureFailure is the typed error returned by Sign for
// every local signing failure. The `Reason` field carries the
// documented vocabulary
// {NO_ACTIVE_KEY, MALFORMED_CANONICAL_PAYLOAD, EXPIRES_AT_OUTSIDE_TOLERANCE}.
type CallbackSignatureFailure struct {
	Reason string
	Cause  error
}

// Error implements the error interface.
func (e *CallbackSignatureFailure) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return fmt.Sprintf("qf-decisions: callback signature failure %s", e.Reason)
	}
	return fmt.Sprintf("qf-decisions: callback signature failure %s: %v", e.Reason, e.Cause)
}

// Unwrap exposes the underlying cause for errors.Is / errors.As.
func (e *CallbackSignatureFailure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// CallbackSigner signs CallbackEnvelopes using the newest
// `not_before`-valid key in the supplied keystore. The signer never
// sends the envelope over the network — Transport is invoked by the
// caller AFTER Sign returns successfully. Signature failures abort
// locally and emit the signature-failure metric + audit envelope; the
// caller MUST NOT retry and MUST NOT POST the envelope. SCN-SM-041-028 /
// SCN-SM-041-030.
type CallbackSigner struct {
	keystore *CallbackKeystore
	nowFn    func() time.Time
}

// NewCallbackSigner constructs a CallbackSigner backed by the supplied
// keystore. If nowFn is nil, the signer defaults to time.Now().UTC().
// A nil keystore is permitted at construction time; Sign will return
// the NO_ACTIVE_KEY signature-failure record for every attempt.
func NewCallbackSigner(keystore *CallbackKeystore, nowFn func() time.Time) *CallbackSigner {
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	return &CallbackSigner{keystore: keystore, nowFn: nowFn}
}

// KeyIDs returns the list of key_ids in the underlying keystore for
// diagnostic display. Returns nil for a nil signer.
func (s *CallbackSigner) KeyIDs() []string {
	if s == nil {
		return nil
	}
	return s.keystore.KeyIDs()
}

// Sign computes the HMAC-SHA256 signature over the canonical payload
// using the newest `not_before`-valid key and returns the envelope
// with `signature` (lower-case hex) and `key_id` populated. On any
// signature failure, Sign emits the signature-failure metric + audit
// envelope locally and returns a *CallbackSignatureFailure WITHOUT
// mutating the envelope. The caller MUST NOT POST the envelope when
// Sign returns an error. SCN-SM-041-028 / SCN-SM-041-030.
func (s *CallbackSigner) Sign(env CallbackEnvelope) (CallbackEnvelope, error) {
	now := s.nowFn().UTC()
	// Step 1: canonical payload composition. Validates structural shape.
	canonical, err := CallbackCanonicalPayload(env)
	if err != nil {
		return env, s.recordSignatureFailure(env, CallbackSignatureFailureMalformedCanonicalPayload, err, now)
	}
	// Step 2: expires_at past-tolerance check (60 seconds).
	expiresAt, parseErr := time.Parse(time.RFC3339, env.ExpiresAt)
	if parseErr != nil {
		// Defensive: CallbackCanonicalPayload already validated
		// RFC3339, so this branch is unreachable in practice.
		// Surface as MALFORMED for vocabulary consistency.
		return env, s.recordSignatureFailure(env, CallbackSignatureFailureMalformedCanonicalPayload, fmt.Errorf("%w: expires_at re-parse: %v", ErrMalformedCanonicalPayload, parseErr), now)
	}
	if now.Sub(expiresAt) > CallbackExpiryPastTolerance {
		return env, s.recordSignatureFailure(
			env,
			CallbackSignatureFailureExpiresAtOutsideTolerance,
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
	// Step 3: select active key.
	key, kerr := s.keystore.SelectActiveKey(now)
	if kerr != nil {
		return env, s.recordSignatureFailure(env, CallbackSignatureFailureNoActiveKey, kerr, now)
	}
	// Step 4: HMAC-SHA256, lower-case hex.
	mac := hmac.New(sha256.New, []byte(key.Secret))
	mac.Write([]byte(canonical))
	signed := env
	signed.Signature = hex.EncodeToString(mac.Sum(nil))
	signed.KeyID = key.KeyID
	return signed, nil
}

// recordSignatureFailure emits the signature-failure metric +
// Cross-Product Audit Envelope v1 record for the given reason and
// returns the typed CallbackSignatureFailure error. The audit envelope
// carries action="callback_attempt" outcome="rejected"
// reason=<documented vocabulary>. No HTTP transport is invoked.
func (s *CallbackSigner) recordSignatureFailure(env CallbackEnvelope, reason string, cause error, observedAt time.Time) error {
	RecordQFCallbackSignatureFailure(reason)
	EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
		TraceID:    env.TraceID,
		PacketID:   env.PacketID,
		ActorRef:   AuditActorSmackerelConnector,
		Surface:    env.Surface,
		Action:     env.Action,
		Status:     "rejected",
		Reason:     reason,
		ObservedAt: observedAt,
	})
	slog.Warn("qf-decisions: callback signature failure",
		slog.String("reason", reason),
		slog.String("trace_id", env.TraceID),
		slog.String("packet_id", env.PacketID),
		slog.String("action", env.Action),
		slog.String("surface", env.Surface),
		slog.String("cause", errString(cause)),
	)
	return &CallbackSignatureFailure{Reason: reason, Cause: cause}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// CallbackAttemptResult captures the outcome of a single PostCallback
// invocation.
type CallbackAttemptResult struct {
	// Envelope is the envelope as it was prepared (signature and
	// key_id populated only when LocalRejection is nil).
	Envelope CallbackEnvelope
	// Status is the documented label value emitted to
	// smackerel_qf_callback_attempts_total for this attempt.
	Status string
	// LocalRejection is set to the signature-failure error when the
	// signer aborted locally. When set, no network transport was
	// attempted (no HTTP request was made).
	LocalRejection *CallbackSignatureFailure
	// QFResponse describes the QF-side outcome when LocalRejection
	// is nil. RejectionCode is set when QF returned a structured
	// rejection (e.g., "CALLBACK_DEFERRED_TO_V1"). HTTPStatus is
	// the raw response status.
	QFResponse CallbackQFResponse
}

// CallbackQFResponse describes the QF-side outcome of a callback POST.
type CallbackQFResponse struct {
	HTTPStatus    int
	RejectionCode string
	Body          string
}

// callbackRejectionBody mirrors the JSON shape QF returns on rejection:
//
//	{"code":"CALLBACK_DEFERRED_TO_V1","message":"...","retry_after_seconds":0}
type callbackRejectionBody struct {
	Code              string `json:"code"`
	Message           string `json:"message,omitempty"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

// PostCallback signs the envelope and POSTs it through the QF client
// transport. Returns a CallbackAttemptResult describing the outcome.
//
// Behavioural guarantees (SCN-SM-041-029, SCN-SM-041-030):
//   - The signer is invoked first. If it returns a
//     *CallbackSignatureFailure, PostCallback returns immediately
//     WITHOUT invoking any HTTP transport. The signer has already
//     incremented smackerel_qf_callback_signature_failures_total and
//     emitted the audit envelope (action=callback_attempt
//     outcome=rejected reason=<vocabulary>). PostCallback additionally
//     increments smackerel_qf_callback_attempts_total with
//     status="rejected_local" so dashboards see one attempt per try.
//     No retry.
//   - When signing succeeds and QF returns CALLBACK_DEFERRED_TO_V1,
//     PostCallback records the attempts metric with
//     status="rejected_v1_deferred" and the audit envelope
//     (action=callback_attempt outcome=rejected
//     reason=CALLBACK_DEFERRED_TO_V1). The Go-level return is
//     (result, nil) because this is the documented pre-MVP outcome,
//     not a connector failure. No retry. No local action acceptance.
//   - When signing succeeds and QF returns 2xx, PostCallback records
//     status="ok". Per PP10, the connector does NOT persist any
//     local action acceptance even on 2xx — the callback is an
//     emission, not a state mutation.
//   - When signing succeeds but transport fails (timeout, network
//     error), PostCallback records status="error" and returns the
//     transport error. No retry — retry is the caller's policy.
func PostCallback(ctx context.Context, client *Client, signer *CallbackSigner, env CallbackEnvelope) (CallbackAttemptResult, error) {
	if signer == nil {
		return CallbackAttemptResult{Envelope: env, Status: CallbackStatusError}, errors.New("qf-decisions: callback signer is nil (signing not configured)")
	}
	signed, signErr := signer.Sign(env)
	if signErr != nil {
		var failure *CallbackSignatureFailure
		if errors.As(signErr, &failure) {
			RecordQFCallbackAttempt(env.Action, CallbackStatusRejectedLocal)
			return CallbackAttemptResult{
				Envelope:       env,
				Status:         CallbackStatusRejectedLocal,
				LocalRejection: failure,
			}, signErr
		}
		return CallbackAttemptResult{Envelope: env, Status: CallbackStatusError}, signErr
	}
	if client == nil {
		return CallbackAttemptResult{Envelope: signed, Status: CallbackStatusError}, errors.New("qf-decisions: callback client is nil (transport not configured)")
	}

	status, respBody, transportErr := client.doJSON(ctx, http.MethodPost, CallbackPath, signed)
	if transportErr != nil {
		RecordQFCallbackAttempt(env.Action, CallbackStatusError)
		EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
			TraceID:    env.TraceID,
			PacketID:   env.PacketID,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    env.Surface,
			Action:     env.Action,
			Status:     "error",
			Reason:     transportErr.Error(),
			ObservedAt: signer.nowFn().UTC(),
		})
		return CallbackAttemptResult{Envelope: signed, Status: CallbackStatusError}, transportErr
	}

	qfResp := CallbackQFResponse{HTTPStatus: status, Body: string(respBody)}
	if len(respBody) > 0 {
		var parsed callbackRejectionBody
		if jerr := json.Unmarshal(respBody, &parsed); jerr == nil {
			qfResp.RejectionCode = strings.TrimSpace(parsed.Code)
		}
	}
	result := CallbackAttemptResult{Envelope: signed, QFResponse: qfResp}

	switch {
	case qfResp.RejectionCode == CallbackRejectionCodeDeferredV1:
		result.Status = CallbackStatusRejectedV1Deferred
		RecordQFCallbackAttempt(env.Action, CallbackStatusRejectedV1Deferred)
		EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
			TraceID:    env.TraceID,
			PacketID:   env.PacketID,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    env.Surface,
			Action:     env.Action,
			Status:     "rejected",
			Reason:     CallbackRejectionCodeDeferredV1,
			ObservedAt: signer.nowFn().UTC(),
		})
		return result, nil
	case status >= 200 && status < 300:
		result.Status = CallbackStatusOK
		RecordQFCallbackAttempt(env.Action, CallbackStatusOK)
		EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
			TraceID:    env.TraceID,
			PacketID:   env.PacketID,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    env.Surface,
			Action:     env.Action,
			Status:     "ok",
			Reason:     "callback_acknowledged",
			ObservedAt: signer.nowFn().UTC(),
		})
		// PP10 guarantee: even on HTTP 2xx, the connector does NOT
		// persist any local action acceptance. The callback is an
		// emission, not a state mutation.
		return result, nil
	default:
		result.Status = CallbackStatusError
		RecordQFCallbackAttempt(env.Action, CallbackStatusError)
		EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
			TraceID:    env.TraceID,
			PacketID:   env.PacketID,
			ActorRef:   AuditActorSmackerelConnector,
			Surface:    env.Surface,
			Action:     env.Action,
			Status:     "error",
			Reason:     fmt.Sprintf("http_status=%d code=%s", status, qfResp.RejectionCode),
			ObservedAt: signer.nowFn().UTC(),
		})
		return result, fmt.Errorf("qf-decisions: callback POST returned HTTP %d (code=%s)", status, qfResp.RejectionCode)
	}
}

// NewCallbackID returns a fresh UUIDv7 string suitable for use as
// CallbackEnvelope.CallbackID. The v7 format embeds a 48-bit Unix
// millisecond timestamp so envelope IDs are roughly time-sortable for
// audit log scanning. Falls back to UUIDv4 if v7 generation fails
// (extremely unlikely; defense-in-depth).
func NewCallbackID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		fallback, ferr := uuid.NewRandom()
		if ferr != nil {
			return "", fmt.Errorf("generate callback id: %w", err)
		}
		return fallback.String(), nil
	}
	return id.String(), nil
}

// NewCallbackNonce returns a fresh UUIDv4 string suitable for use as
// CallbackEnvelope.Nonce. The nonce MUST be unique per envelope; the
// QF-side replay defense relies on it.
func NewCallbackNonce() (string, error) {
	n, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate callback nonce: %w", err)
	}
	return n.String(), nil
}
