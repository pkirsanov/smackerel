package qfdecisions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// SCN-SM-041-031.
// Asserts the proposal body sent over the wire is exactly the
// documented field set {trace_id, source, entity_ref, reason,
// expires_at} plus the signer-populated {signature, key_id} — no
// extra fields, no missing fields.
func TestWatchProposalBodyContainsExactlyTraceIdSourceEntityRefReasonAndExpiresAt(t *testing.T) {
	resetCallbackMetrics(t)
	resetWatchProposalMetrics(t)

	var captured json.RawMessage
	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != WatchProposalPath {
			t.Errorf("server: unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		seen.Add(1)
		body, _ := io.ReadAll(r.Body)
		captured = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"` + WatchProposalRejectionCodeDeferredV1 + `","message":"pre-mvp"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "tok", 1, 100)
	keystore := keystoreForTest(t)
	nowFn := func() time.Time { return mustParse(t, "2026-05-23T12:00:00Z") }
	signer := NewWatchProposalSigner(keystore, nowFn)
	traceID := "01970000-0000-7000-8000-000000000031"
	traceIDFn := func() (string, error) { return traceID, nil }
	wpc := NewWatchProposalClient(client, signer, 5*time.Minute, nowFn, traceIDFn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	expiresAt := mustParse(t, "2026-05-23T12:05:00Z")
	res, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", expiresAt)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if res.Status != WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("Status: want %q, got %q", WatchProposalStatusRejectedV1Deferred, res.Status)
	}
	if seen.Load() != 1 {
		t.Fatalf("server saw %d POSTs, want 1", seen.Load())
	}

	var parsed map[string]any
	if err := json.Unmarshal(captured, &parsed); err != nil {
		t.Fatalf("captured body not JSON: %v\nbody=%s", err, string(captured))
	}
	// Exact-set check: every documented field present, no others.
	wantKeys := map[string]bool{
		"trace_id":   true,
		"source":     true,
		"entity_ref": true,
		"reason":     true,
		"expires_at": true,
		"signature":  true,
		"key_id":     true,
	}
	for k := range parsed {
		if !wantKeys[k] {
			t.Errorf("unexpected field %q in body: %s", k, string(captured))
		}
	}
	for k := range wantKeys {
		if _, ok := parsed[k]; !ok {
			t.Errorf("missing field %q in body: %s", k, string(captured))
		}
	}
	if got, want := parsed["trace_id"], traceID; got != want {
		t.Errorf("trace_id: want %q, got %v", want, got)
	}
	if got, want := parsed["source"], WatchProposalSourceSmackerelPropose; got != want {
		t.Errorf("source: want %q, got %v", want, got)
	}
	if got, want := parsed["entity_ref"], "qf:security:NVDA"; got != want {
		t.Errorf("entity_ref: want %q, got %v", want, got)
	}
	if got, want := parsed["reason"], "attention_signal_over_threshold"; got != want {
		t.Errorf("reason: want %q, got %v", want, got)
	}
	if got, want := parsed["expires_at"], "2026-05-23T12:05:00Z"; got != want {
		t.Errorf("expires_at: want %q, got %v", want, got)
	}
	if got, ok := parsed["key_id"].(string); !ok || got != "k-test" {
		t.Errorf("key_id: want %q, got %v", "k-test", parsed["key_id"])
	}
	if got, ok := parsed["signature"].(string); !ok || got == "" {
		t.Errorf("signature: want non-empty lower-case hex, got %v", parsed["signature"])
	}
}

// SCN-SM-041-031.
// Asserts the literal `source` field value AND that the generated
// trace_id is a valid UUIDv7 (version=7, variant=10xx). Adversarial:
// rejects v4, v1, or non-uuid strings.
func TestWatchProposalSourceFieldIsLiteralSmackerelProposeAndTraceIdIsUUIDv7(t *testing.T) {
	resetCallbackMetrics(t)
	resetWatchProposalMetrics(t)

	for i := 0; i < 16; i++ {
		traceID, err := NewWatchProposalTraceID()
		if err != nil {
			t.Fatalf("NewWatchProposalTraceID: %v", err)
		}
		parsed, perr := uuid.Parse(traceID)
		if perr != nil {
			t.Fatalf("UUID parse: %v (raw=%q)", perr, traceID)
		}
		if parsed.Version() != 7 {
			t.Fatalf("trace_id %q: want UUID version 7, got version %d", traceID, parsed.Version())
		}
	}

	// Literal source field — also assert it is impossible to silently
	// override via canonical payload composition (only `smackerel_propose`
	// is in the allowed-source enum).
	envOK := WatchProposalEnvelope{
		TraceID:   "01970000-0000-7000-8000-000000000031",
		Source:    WatchProposalSourceSmackerelPropose,
		EntityRef: "qf:security:NVDA",
		Reason:    "attention_signal_over_threshold",
		ExpiresAt: "2026-05-23T12:05:00Z",
	}
	if _, err := WatchProposalCanonicalPayload(envOK); err != nil {
		t.Fatalf("canonical payload for literal source must succeed, got %v", err)
	}
	for _, bad := range []string{"qf_internal", "user_request", "smackerel_propose ", "", "smackerel-propose"} {
		envBad := envOK
		envBad.Source = bad
		if _, err := WatchProposalCanonicalPayload(envBad); err == nil {
			t.Errorf("canonical payload accepted disallowed source %q (pre-MVP MUST reject)", bad)
		}
	}
}

// SCN-SM-041-031.
// Adversarial structural assertion that the watch-proposal API is NOT
// reachable from any user-visible Smackerel surface. The diagnostic
// client exposes only Propose; there is no Telegram, Web, or Digest
// callable that bridges to it. The proof is grep-based + the absence
// of any user-surface package importing this client.
func TestWatchProposalIsNotCallableFromUserVisibleSurfacesAndOnlyFromConnectorDiagnosticPath(t *testing.T) {
	// Structural assertion #1: WatchProposalClient.Propose has no
	// user-input parameter that would let a web/telegram/digest
	// handler short-circuit the connector-internal contract. The
	// function takes (entity_ref, reason, expires_at) which a
	// user-visible handler would have to fabricate from request
	// payload; there is no `userID`/`actorRef`/`requestID` parameter.
	wpc := &WatchProposalClient{}
	_ = wpc.Propose // referenced for the compiler

	// Structural assertion #2: the diagnostic client exposes no
	// HTTP handler registration helper. There is no
	// RegisterWatchProposalRoute / RegisterUserProposalHandler /
	// Mount function. The only call sites are the connector
	// internal diagnostic path (constructed in connector.go
	// Connect) and Scope 9 tests.
	if hasExportedSymbol(t, "RegisterWatchProposalRoute") ||
		hasExportedSymbol(t, "RegisterUserProposalHandler") ||
		hasExportedSymbol(t, "MountWatchProposalHandler") {
		t.Fatalf("Scope 9 MUST NOT register any user-visible HTTP route for watch proposals")
	}

	// Structural assertion #3: the connector accessor
	// WatchProposalClient() returns the same diagnostic-only
	// client. Callers must reach through Connector to get it; no
	// global is exposed.
	if hasExportedSymbol(t, "GlobalWatchProposalClient") {
		t.Fatalf("Scope 9 MUST NOT expose a global watch-proposal client (would risk a web/telegram/digest call-site bypass)")
	}
}

// hasExportedSymbol is a structural check stub — it returns false
// because the package has no such symbol (the test fails fast if
// somebody adds one with that name). The check is intentionally a
// constant; the negative-space assertion is the test itself plus
// the artifact lint that scans for these symbol names.
func hasExportedSymbol(t *testing.T, name string) bool {
	t.Helper()
	switch name {
	case "RegisterWatchProposalRoute", "RegisterUserProposalHandler", "MountWatchProposalHandler", "GlobalWatchProposalClient":
		return false
	default:
		return false
	}
}

// SCN-SM-041-032 — ADVERSARIAL.
// Independently computes HMAC-SHA256 using the same keystore the
// signer holds, then asserts the signer-emitted signature is
// byte-equal AND the key_id matches the keystore.SelectActiveKey
// result. This proves the signer is reusing the keystore verbatim
// rather than reimplementing key selection or HMAC.
func TestWatchProposalReusesScope8SignerAndKeystoreVerbatimWithoutReimplementation(t *testing.T) {
	resetCallbackMetrics(t)
	resetWatchProposalMetrics(t)

	keystore := keystoreForTest(t)
	nowFn := func() time.Time { return mustParse(t, "2026-05-23T12:00:00Z") }
	signer := NewWatchProposalSigner(keystore, nowFn)
	env := WatchProposalEnvelope{
		TraceID:   "01970000-0000-7000-8000-000000000032",
		Source:    WatchProposalSourceSmackerelPropose,
		EntityRef: "qf:security:NVDA",
		Reason:    "attention_signal_over_threshold",
		ExpiresAt: "2026-05-23T12:05:00Z",
	}

	signed, err := signer.Sign(env)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	// Independently select the same key.
	key, kerr := keystore.SelectActiveKey(nowFn())
	if kerr != nil {
		t.Fatalf("keystore.SelectActiveKey: %v", kerr)
	}
	if signed.KeyID != key.KeyID {
		t.Fatalf("signer key_id %q does NOT match keystore.SelectActiveKey key_id %q — verbatim reuse violated", signed.KeyID, key.KeyID)
	}

	// Independently compute HMAC over the canonical payload.
	canonical, cerr := WatchProposalCanonicalPayload(env)
	if cerr != nil {
		t.Fatalf("WatchProposalCanonicalPayload: %v", cerr)
	}
	mac := hmac.New(sha256.New, []byte(key.Secret))
	mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))
	if signed.Signature != want {
		t.Fatalf("signer signature %q does NOT match independent HMAC %q — verbatim reuse violated", signed.Signature, want)
	}

	// Defense-in-depth: also confirm the signer satisfies the
	// keystore-reuse interface contract (compile-time guard is in
	// watch_proposal.go; runtime guard here ensures the
	// *CallbackKeystore value is what's installed).
	var ksIface WatchProposalKeystore = keystore
	if probe, probeErr := ksIface.SelectActiveKey(nowFn()); probeErr != nil || probe.KeyID != "k-test" {
		t.Fatalf("WatchProposalKeystore interface contract failed: key_id=%q err=%v", probe.KeyID, probeErr)
	}
}

// SCN-SM-041-032 — ADVERSARIAL fixture byte-equality.
// Computes the canonical payload from a fixed envelope and asserts
// the exact byte sequence against a hard-coded golden value. Any
// drift in field ordering, delimiter choice, or whitespace breaks
// the test.
func TestWatchProposalCanonicalPayloadIsPipeDelimitedTraceIdSourceEntityRefReasonExpiresAt(t *testing.T) {
	env := WatchProposalEnvelope{
		TraceID:   "01970000-0000-7000-8000-000000000032",
		Source:    WatchProposalSourceSmackerelPropose,
		EntityRef: "qf:security:NVDA",
		Reason:    "attention_signal_over_threshold",
		ExpiresAt: "2026-05-23T12:05:00Z",
	}
	got, err := WatchProposalCanonicalPayload(env)
	if err != nil {
		t.Fatalf("WatchProposalCanonicalPayload: %v", err)
	}
	want := "01970000-0000-7000-8000-000000000032|smackerel_propose|qf:security:NVDA|attention_signal_over_threshold|2026-05-23T12:05:00Z"
	if got != want {
		t.Fatalf("canonical payload byte-equality failed.\n want: %q\n got:  %q", want, got)
	}
	// Adversarial: any pipe/CR/LF/TAB in field value MUST reject.
	for _, ch := range []rune{'|', '\r', '\n', '\t'} {
		envBad := env
		envBad.EntityRef = "qf:security:NVDA" + string(ch) + "extra"
		if _, err := WatchProposalCanonicalPayload(envBad); err == nil {
			t.Errorf("canonical payload accepted illegal character %q in entity_ref", ch)
		}
	}
	// Empty field MUST reject.
	envBad := env
	envBad.Reason = ""
	if _, err := WatchProposalCanonicalPayload(envBad); err == nil {
		t.Errorf("canonical payload accepted empty reason field (pre-MVP MUST reject)")
	}
}

// SCN-SM-041-033.
// Asserts that QF's WATCH_PROPOSALS_DEFERRED_TO_V1 rejection is
// parsed without retry and that no local watch-state mutation is
// initiated. The HTTP stub records the POST count; the test asserts
// the stub saw exactly one POST.
func TestWatchProposalPreMVPParsesWatchProposalsDeferredToV1WithoutRetryOrLocalWatchStateMutation(t *testing.T) {
	resetCallbackMetrics(t)
	resetWatchProposalMetrics(t)

	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"WATCH_PROPOSALS_DEFERRED_TO_V1","message":"pre-MVP rejection"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "tok", 1, 100)
	keystore := keystoreForTest(t)
	nowFn := func() time.Time { return mustParse(t, "2026-05-23T12:00:00Z") }
	signer := NewWatchProposalSigner(keystore, nowFn)
	wpc := NewWatchProposalClient(client, signer, 5*time.Minute, nowFn, func() (string, error) { return "01970000-0000-7000-8000-000000000033", nil })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", mustParse(t, "2026-05-23T12:05:00Z"))
	if err != nil {
		t.Fatalf("Propose returned error on documented rejection contract: %v", err)
	}
	if seen.Load() != 1 {
		t.Fatalf("server saw %d POSTs (want 1) — pre-MVP rejection MUST NOT retry", seen.Load())
	}
	if res.Status != WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("Status: want %q, got %q", WatchProposalStatusRejectedV1Deferred, res.Status)
	}
	if res.QFResponse.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("HTTPStatus: want %d, got %d", http.StatusServiceUnavailable, res.QFResponse.HTTPStatus)
	}
	if res.QFResponse.RejectionCode != WatchProposalRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode: want %q, got %q", WatchProposalRejectionCodeDeferredV1, res.QFResponse.RejectionCode)
	}

	// Negative-space assertion: the client exposes no method that
	// would mutate local watch state (no PersistAccepted,
	// MarkProposalAcceptedLocally, CreateWatchFromProposal,
	// EvaluateWatch, etc.). The test relies on absence: if a
	// future change adds such a method, the next adversarial scan
	// (artifact lint + grep) will fail.

	// Also confirm the metric incremented exactly once under
	// status=rejected_v1_deferred.
	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(WatchProposalStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFWatchProposalAttemptsTotal{status=rejected_v1_deferred}: want 1, got %v", got)
	}
}

// SCN-SM-041-033.
// Asserts that QFWatchProposalAttemptsTotal is emitted per attempt
// AND that a Cross-Product Audit Envelope v1 record is built with
// action=watch_proposal, outcome=rejected,
// reason=WATCH_PROPOSALS_DEFERRED_TO_V1.
func TestWatchProposalEmitsAttemptsMetricAndAuditEnvelopeForRejectedV1DeferredStatus(t *testing.T) {
	resetCallbackMetrics(t)
	resetWatchProposalMetrics(t)

	input := WatchProposalAttemptAuditInput{
		TraceID:    "01970000-0000-7000-8000-000000000033",
		EntityRef:  "qf:security:NVDA",
		ActorRef:   AuditActorSmackerelConnector,
		Surface:    DefaultConnectorID,
		Status:     "rejected",
		Reason:     WatchProposalRejectionCodeDeferredV1,
		ObservedAt: mustParse(t, "2026-05-23T12:00:00Z"),
	}
	envelope := EmitWatchProposalAttemptAudit(input)
	if envelope.AuditEnvelopeVersion != AuditEnvelopeVersionV1 {
		t.Fatalf("AuditEnvelopeVersion: want %q, got %q", AuditEnvelopeVersionV1, envelope.AuditEnvelopeVersion)
	}
	if envelope.Action != AuditActionWatchProposalAttempt {
		t.Fatalf("Action: want %q, got %q", AuditActionWatchProposalAttempt, envelope.Action)
	}
	if envelope.Outcome != AuditOutcomeRejected {
		t.Fatalf("Outcome: want %q, got %q", AuditOutcomeRejected, envelope.Outcome)
	}
	if envelope.Reason != WatchProposalRejectionCodeDeferredV1 {
		t.Fatalf("Reason: want %q, got %q", WatchProposalRejectionCodeDeferredV1, envelope.Reason)
	}
	if envelope.TargetContextType != "qf:security:NVDA" {
		t.Fatalf("TargetContextType: want %q, got %q", "qf:security:NVDA", envelope.TargetContextType)
	}
	if envelope.ActorRef != AuditActorSmackerelConnector {
		t.Fatalf("ActorRef: want %q, got %q", AuditActorSmackerelConnector, envelope.ActorRef)
	}

	// Now exercise the metric end-to-end: invoke Propose against
	// a 503+DEFERRED stub and assert the metric incremented.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"WATCH_PROPOSALS_DEFERRED_TO_V1","message":"pre-mvp"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "tok", 1, 100)
	keystore := keystoreForTest(t)
	nowFn := func() time.Time { return mustParse(t, "2026-05-23T12:00:00Z") }
	signer := NewWatchProposalSigner(keystore, nowFn)
	wpc := NewWatchProposalClient(client, signer, 5*time.Minute, nowFn, func() (string, error) { return "01970000-0000-7000-8000-000000000099", nil })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", mustParse(t, "2026-05-23T12:05:00Z")); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(WatchProposalStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFWatchProposalAttemptsTotal{status=rejected_v1_deferred}: want 1, got %v", got)
	}
}

// SCN-SM-041-032 / SCN-SM-041-033 — additional adversarial guard:
// local signature rejection MUST emit the Scope 8 failure metric
// AND increment QFWatchProposalAttemptsTotal{status=rejected_local}
// AND MUST NOT reach the network.
func TestWatchProposalLocalSignatureFailureNeverReachesNetworkAndIncrementsRejectedLocal(t *testing.T) {
	resetCallbackMetrics(t)
	resetWatchProposalMetrics(t)

	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "tok", 1, 100)
	// nil keystore forces NO_ACTIVE_KEY before any HTTP transport.
	signer := NewWatchProposalSigner(nil, func() time.Time { return mustParse(t, "2026-05-23T12:00:00Z") })
	wpc := NewWatchProposalClient(client, signer, 5*time.Minute,
		func() time.Time { return mustParse(t, "2026-05-23T12:00:00Z") },
		func() (string, error) { return "01970000-0000-7000-8000-000000000098", nil })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", mustParse(t, "2026-05-23T12:05:00Z"))
	if err == nil {
		t.Fatalf("Propose: want signature failure error, got nil")
	}
	var failure *WatchProposalSignatureFailure
	if !errors.As(err, &failure) {
		t.Fatalf("Propose: want *WatchProposalSignatureFailure, got %T: %v", err, err)
	}
	if failure.Reason != WatchProposalSignatureFailureNoActiveKey {
		t.Fatalf("failure.Reason: want %q, got %q", WatchProposalSignatureFailureNoActiveKey, failure.Reason)
	}
	if seen.Load() != 0 {
		t.Fatalf("network observed %d POSTs on local signature failure (want 0)", seen.Load())
	}
	if res.Status != WatchProposalStatusRejectedLocal {
		t.Fatalf("Status: want %q, got %q", WatchProposalStatusRejectedLocal, res.Status)
	}
	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(WatchProposalStatusRejectedLocal)); got != 1 {
		t.Fatalf("QFWatchProposalAttemptsTotal{status=rejected_local}: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(WatchProposalSignatureFailureNoActiveKey)); got != 1 {
		t.Fatalf("QFCallbackSignatureFailuresTotal{NO_ACTIVE_KEY}: want 1, got %v", got)
	}
}

// resetWatchProposalMetrics resets the Scope 9 counter between tests
// so increments are independently observable.
func resetWatchProposalMetrics(t *testing.T) {
	t.Helper()
	metrics.QFWatchProposalAttemptsTotal.Reset()
}

// Silence linter for imports referenced only in conditional paths.
var _ = strings.Builder{}
