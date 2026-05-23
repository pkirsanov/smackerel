package qfdecisions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/metrics"
)

// SCN-SM-041-028 — Canonical payload composition is pipe-delimited,
// no whitespace, no trailing pipe, exactly 7 fields in the documented
// order: callback_id|trace_id|packet_id|action|nonce|expires_at|surface.
func TestCallbackCanonicalPayloadCompositionIsPipeDelimitedWithoutWhitespaceOrTrailingPipe(t *testing.T) {
	env := CallbackEnvelope{
		CallbackID: "cb-001",
		TraceID:    "tr-001",
		PacketID:   "pk-001",
		Action:     CallbackActionNoop,
		Nonce:      "no-001",
		ExpiresAt:  "2026-05-22T12:00:00Z",
		Surface:    SurfaceTelegram,
	}
	got, err := CallbackCanonicalPayload(env)
	if err != nil {
		t.Fatalf("CallbackCanonicalPayload: %v", err)
	}
	want := "cb-001|tr-001|pk-001|noop|no-001|2026-05-22T12:00:00Z|telegram"
	if got != want {
		t.Fatalf("canonical payload\n  got:  %q\n  want: %q", got, want)
	}
	if strings.HasSuffix(got, "|") {
		t.Fatalf("canonical payload ends with pipe: %q", got)
	}
	if strings.ContainsAny(got, " \t\r\n") {
		t.Fatalf("canonical payload contains whitespace: %q", got)
	}
	if strings.Count(got, "|") != 6 {
		t.Fatalf("canonical payload pipe count: want 6, got %d (%q)", strings.Count(got, "|"), got)
	}
}

// SCN-SM-041-028 — HMAC-SHA256 signature is lower-case hex and matches
// a known vector computed against a known secret + known canonical payload.
// Adversarial: if ANY field of the canonical payload is mutated, the
// signature differs.
func TestCallbackHMACSHA256SignatureIsLowerCaseHexAndMatchesKnownVector(t *testing.T) {
	raw := `[{"key_id":"k-known","secret":"sek-known","not_before":"2026-01-01T00:00:00Z"}]`
	keystore, err := LoadCallbackKeystoreFromJSON(raw)
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
	}
	signer := NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:00:00Z")
	})
	env := CallbackEnvelope{
		CallbackID: "cb-known",
		TraceID:    "tr-known",
		PacketID:   "pk-known",
		Action:     CallbackActionNoop,
		Nonce:      "no-known",
		ExpiresAt:  "2026-05-22T12:00:30Z",
		Surface:    SurfaceTelegram,
	}
	signed, err := signer.Sign(env)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	canonical, _ := CallbackCanonicalPayload(env)
	mac := hmac.New(sha256.New, []byte("sek-known"))
	mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))
	if signed.Signature != want {
		t.Fatalf("signature mismatch\n  got:  %q\n  want: %q", signed.Signature, want)
	}
	if signed.Signature != strings.ToLower(signed.Signature) {
		t.Fatalf("signature is not lower-case hex: %q", signed.Signature)
	}
	if signed.KeyID != "k-known" {
		t.Fatalf("signed KeyID: want k-known, got %q", signed.KeyID)
	}
	// Adversarial: mutate the action and prove the signature differs.
	mutated := env
	mutated.Action = CallbackActionOpen
	mutatedSigned, err := signer.Sign(mutated)
	if err != nil {
		t.Fatalf("Sign mutated: %v", err)
	}
	if mutatedSigned.Signature == signed.Signature {
		t.Fatalf("mutating action did not change signature (%q == %q)", mutatedSigned.Signature, signed.Signature)
	}
	// Adversarial: mutate the packet_id and prove the signature differs.
	mutated = env
	mutated.PacketID = "pk-other"
	mutatedSigned, err = signer.Sign(mutated)
	if err != nil {
		t.Fatalf("Sign mutated: %v", err)
	}
	if mutatedSigned.Signature == signed.Signature {
		t.Fatalf("mutating packet_id did not change signature (%q == %q)", mutatedSigned.Signature, signed.Signature)
	}
}

// SCN-SM-041-029 — Pre-MVP signing path executes end-to-end with
// signature and key_id in the request body; QF CALLBACK_DEFERRED_TO_V1
// rejection is parsed without retry.
func TestCallbackPreMVPParsesCallbackDeferredToV1RejectionWithoutRetryOrLocalActionAcceptance(t *testing.T) {
	resetCallbackMetrics(t)
	var (
		reqCount  int
		reqBodies []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != CallbackPath {
			http.NotFound(w, r)
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		reqCount++
		reqBodies = append(reqBodies, string(body))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"CALLBACK_DEFERRED_TO_V1","message":"pre-MVP: bridge does not accept callbacks"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "bridge-test-token", 1, 100)
	keystore := keystoreForTest(t)
	signer := NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:00:00Z")
	})
	env := CallbackEnvelope{
		CallbackID: "cb-pre-mvp-001",
		TraceID:    "tr-pre-mvp-001",
		PacketID:   "pk-pre-mvp-001",
		Action:     CallbackActionNoop,
		Nonce:      "no-pre-mvp-001",
		ExpiresAt:  "2026-05-22T12:00:30Z",
		Surface:    SurfaceTelegram,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := PostCallback(ctx, client, signer, env)
	if err != nil {
		t.Fatalf("PostCallback: %v", err)
	}
	if result.Status != CallbackStatusRejectedV1Deferred {
		t.Fatalf("result.Status: want %q, got %q", CallbackStatusRejectedV1Deferred, result.Status)
	}
	if result.QFResponse.RejectionCode != CallbackRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode: want %q, got %q", CallbackRejectionCodeDeferredV1, result.QFResponse.RejectionCode)
	}
	if result.QFResponse.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("HTTPStatus: want 503, got %d", result.QFResponse.HTTPStatus)
	}
	if reqCount != 1 {
		t.Fatalf("server saw %d requests, want 1 (no retry)", reqCount)
	}
	// Confirm the request body carries both signature and key_id.
	var posted CallbackEnvelope
	if jerr := json.Unmarshal([]byte(reqBodies[0]), &posted); jerr != nil {
		t.Fatalf("unmarshal posted body: %v", jerr)
	}
	if posted.Signature == "" {
		t.Fatal("posted body Signature is empty")
	}
	if posted.KeyID != "k-test" {
		t.Fatalf("posted body KeyID: want k-test, got %q", posted.KeyID)
	}
	// Confirm attempt counter incremented under rejected_v1_deferred.
	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(CallbackActionNoop, CallbackStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFCallbackAttemptsTotal{noop,rejected_v1_deferred}: want 1, got %v", got)
	}
	// No signature-failure increment.
	if got := testutil.CollectAndCount(metrics.QFCallbackSignatureFailuresTotal); got != 0 {
		t.Fatalf("QFCallbackSignatureFailuresTotal collected count: want 0, got %d", got)
	}
}

// SCN-SM-041-029 — Attempts metric + audit envelope shape proof.
func TestCallbackEmitsAttemptsMetricAndAuditEnvelopeForRejectedV1DeferredStatus(t *testing.T) {
	resetCallbackMetrics(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"CALLBACK_DEFERRED_TO_V1"}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "tok", 1, 100)
	keystore := keystoreForTest(t)
	signer := NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:00:00Z")
	})
	env := CallbackEnvelope{
		CallbackID: "cb-aud-001",
		TraceID:    "tr-aud-001",
		PacketID:   "pk-aud-001",
		Action:     CallbackActionOpen,
		Nonce:      "no-aud-001",
		ExpiresAt:  "2026-05-22T12:01:00Z",
		Surface:    SurfaceWeb,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := PostCallback(ctx, client, signer, env); err != nil {
		t.Fatalf("PostCallback: %v", err)
	}
	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(CallbackActionOpen, CallbackStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFCallbackAttemptsTotal{open,rejected_v1_deferred}: want 1, got %v", got)
	}
	// Build the same audit envelope to prove the shape contract.
	envelope := EmitCallbackAttemptAudit(CallbackAttemptAuditInput{
		TraceID:    env.TraceID,
		PacketID:   env.PacketID,
		ActorRef:   AuditActorSmackerelConnector,
		Surface:    env.Surface,
		Action:     env.Action,
		Status:     "rejected",
		Reason:     CallbackRejectionCodeDeferredV1,
		ObservedAt: mustParse(t, "2026-05-22T12:00:00Z"),
	})
	if envelope.Action != AuditActionCallbackAttempt {
		t.Fatalf("audit envelope Action: want %q, got %q", AuditActionCallbackAttempt, envelope.Action)
	}
	if envelope.Outcome != AuditOutcomeRejected {
		t.Fatalf("audit envelope Outcome: want %q, got %q", AuditOutcomeRejected, envelope.Outcome)
	}
	if envelope.Reason != CallbackRejectionCodeDeferredV1 {
		t.Fatalf("audit envelope Reason: want %q, got %q", CallbackRejectionCodeDeferredV1, envelope.Reason)
	}
	if envelope.AuditEnvelopeVersion != AuditEnvelopeVersionV1 {
		t.Fatalf("audit envelope version: want %q, got %q", AuditEnvelopeVersionV1, envelope.AuditEnvelopeVersion)
	}
}

// SCN-SM-041-030 — Signature failure NO_ACTIVE_KEY: keystore with all
// future not_before. Sign aborts locally; no network is touched.
func TestCallbackSignatureFailureNoActiveKeyAbortsLocallyAndEmitsFailureMetricAndAuditEnvelope(t *testing.T) {
	resetCallbackMetrics(t)
	raw := `[{"key_id":"k-future","secret":"x","not_before":"2099-01-01T00:00:00Z"}]`
	keystore, err := LoadCallbackKeystoreFromJSON(raw)
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
	}
	// Use a tracking transport to prove no HTTP request was made.
	var requested bool
	tracker := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		w.WriteHeader(http.StatusOK)
	})
	server := httptest.NewServer(tracker)
	defer server.Close()
	client := NewClient(server.URL, "tok", 1, 100)
	signer := NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:00:00Z")
	})
	env := validEnvelope("2026-05-22T12:00:30Z")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := PostCallback(ctx, client, signer, env)
	if err == nil {
		t.Fatal("PostCallback: want signature-failure error, got nil")
	}
	var failure *CallbackSignatureFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error type: want *CallbackSignatureFailure, got %T (%v)", err, err)
	}
	if failure.Reason != CallbackSignatureFailureNoActiveKey {
		t.Fatalf("failure.Reason: want %q, got %q", CallbackSignatureFailureNoActiveKey, failure.Reason)
	}
	if result.Status != CallbackStatusRejectedLocal {
		t.Fatalf("result.Status: want %q, got %q", CallbackStatusRejectedLocal, result.Status)
	}
	if requested {
		t.Fatal("HTTP transport was invoked despite NO_ACTIVE_KEY signature failure")
	}
	if got := testutil.ToFloat64(metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(CallbackSignatureFailureNoActiveKey)); got != 1 {
		t.Fatalf("QFCallbackSignatureFailuresTotal{NO_ACTIVE_KEY}: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(CallbackActionNoop, CallbackStatusRejectedLocal)); got != 1 {
		t.Fatalf("QFCallbackAttemptsTotal{noop,rejected_local}: want 1, got %v", got)
	}
}

// SCN-SM-041-030 — Signature failure MALFORMED_CANONICAL_PAYLOAD: empty
// trace_id, pipe character in field, illegal action enum.
func TestCallbackSignatureFailureMalformedCanonicalPayloadAbortsLocallyAndRecordsReason(t *testing.T) {
	cases := []struct {
		name string
		env  CallbackEnvelope
	}{
		{
			name: "empty trace_id",
			env: CallbackEnvelope{
				CallbackID: "cb-1", PacketID: "pk-1", Action: CallbackActionNoop,
				Nonce: "no-1", ExpiresAt: "2026-05-22T12:00:30Z", Surface: SurfaceTelegram,
			},
		},
		{
			name: "pipe in nonce",
			env: CallbackEnvelope{
				CallbackID: "cb-1", TraceID: "tr-1", PacketID: "pk-1", Action: CallbackActionNoop,
				Nonce: "no|injected", ExpiresAt: "2026-05-22T12:00:30Z", Surface: SurfaceTelegram,
			},
		},
		{
			name: "newline in callback_id",
			env: CallbackEnvelope{
				CallbackID: "cb\n1", TraceID: "tr-1", PacketID: "pk-1", Action: CallbackActionNoop,
				Nonce: "no-1", ExpiresAt: "2026-05-22T12:00:30Z", Surface: SurfaceTelegram,
			},
		},
		{
			name: "forbidden action",
			env: CallbackEnvelope{
				CallbackID: "cb-1", TraceID: "tr-1", PacketID: "pk-1",
				Action: "approval", // not in pre-MVP enum
				Nonce:  "no-1", ExpiresAt: "2026-05-22T12:00:30Z", Surface: SurfaceTelegram,
			},
		},
		{
			name: "unknown surface",
			env: CallbackEnvelope{
				CallbackID: "cb-1", TraceID: "tr-1", PacketID: "pk-1", Action: CallbackActionNoop,
				Nonce: "no-1", ExpiresAt: "2026-05-22T12:00:30Z", Surface: "carrier-pigeon",
			},
		},
		{
			name: "non-RFC3339 expires_at",
			env: CallbackEnvelope{
				CallbackID: "cb-1", TraceID: "tr-1", PacketID: "pk-1", Action: CallbackActionNoop,
				Nonce: "no-1", ExpiresAt: "yesterday", Surface: SurfaceTelegram,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCallbackMetrics(t)
			keystore := keystoreForTest(t)
			signer := NewCallbackSigner(keystore, func() time.Time {
				return mustParse(t, "2026-05-22T12:00:00Z")
			})
			var requested bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requested = true
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			client := NewClient(server.URL, "tok", 1, 100)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := PostCallback(ctx, client, signer, tc.env)
			if err == nil {
				t.Fatal("PostCallback: want MALFORMED_CANONICAL_PAYLOAD failure, got nil")
			}
			var failure *CallbackSignatureFailure
			if !errors.As(err, &failure) {
				t.Fatalf("error type: want *CallbackSignatureFailure, got %T (%v)", err, err)
			}
			if failure.Reason != CallbackSignatureFailureMalformedCanonicalPayload {
				t.Fatalf("failure.Reason: want %q, got %q", CallbackSignatureFailureMalformedCanonicalPayload, failure.Reason)
			}
			if requested {
				t.Fatal("HTTP transport was invoked despite MALFORMED_CANONICAL_PAYLOAD signature failure")
			}
			if got := testutil.ToFloat64(metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(CallbackSignatureFailureMalformedCanonicalPayload)); got != 1 {
				t.Fatalf("QFCallbackSignatureFailuresTotal{MALFORMED_CANONICAL_PAYLOAD}: want 1, got %v", got)
			}
		})
	}
}

// SCN-SM-041-030 — Signature failure EXPIRES_AT_OUTSIDE_TOLERANCE: now
// is more than 60 seconds past expires_at.
func TestCallbackSignatureFailureExpiresAtOutsideToleranceAbortsLocallyAndRecordsReason(t *testing.T) {
	resetCallbackMetrics(t)
	keystore := keystoreForTest(t)
	signer := NewCallbackSigner(keystore, func() time.Time {
		// 5 minutes past expires_at — well outside 60s tolerance.
		return mustParse(t, "2026-05-22T12:05:01Z")
	})
	env := validEnvelope("2026-05-22T12:00:00Z")
	var requested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := NewClient(server.URL, "tok", 1, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := PostCallback(ctx, client, signer, env)
	if err == nil {
		t.Fatal("PostCallback: want EXPIRES_AT_OUTSIDE_TOLERANCE failure, got nil")
	}
	var failure *CallbackSignatureFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error type: want *CallbackSignatureFailure, got %T (%v)", err, err)
	}
	if failure.Reason != CallbackSignatureFailureExpiresAtOutsideTolerance {
		t.Fatalf("failure.Reason: want %q, got %q", CallbackSignatureFailureExpiresAtOutsideTolerance, failure.Reason)
	}
	if requested {
		t.Fatal("HTTP transport was invoked despite EXPIRES_AT_OUTSIDE_TOLERANCE signature failure")
	}
	if got := testutil.ToFloat64(metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(CallbackSignatureFailureExpiresAtOutsideTolerance)); got != 1 {
		t.Fatalf("QFCallbackSignatureFailuresTotal{EXPIRES_AT_OUTSIDE_TOLERANCE}: want 1, got %v", got)
	}
	// Boundary: exactly 60 seconds past expires_at MUST succeed (tolerance is inclusive).
	signer = NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:01:00Z")
	})
	if _, err := signer.Sign(env); err != nil {
		t.Fatalf("Sign at exactly 60s past expires_at: want success, got %v", err)
	}
	// Boundary: 61 seconds past expires_at MUST fail.
	signer = NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:01:01Z")
	})
	if _, err := signer.Sign(env); err == nil {
		t.Fatal("Sign at 61s past expires_at: want EXPIRES_AT_OUTSIDE_TOLERANCE failure, got nil")
	}
}

// PP10 — even on HTTP 2xx, the connector emits no local action acceptance.
// Synthetic 2xx path exercises the status="ok" branch.
func TestCallbackHTTP2xxBranchDoesNotAcceptAnyLocalAction(t *testing.T) {
	resetCallbackMetrics(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "tok", 1, 100)
	keystore := keystoreForTest(t)
	signer := NewCallbackSigner(keystore, func() time.Time {
		return mustParse(t, "2026-05-22T12:00:00Z")
	})
	env := validEnvelope("2026-05-22T12:00:30Z")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := PostCallback(ctx, client, signer, env)
	if err != nil {
		t.Fatalf("PostCallback: %v", err)
	}
	if result.Status != CallbackStatusOK {
		t.Fatalf("result.Status: want %q, got %q", CallbackStatusOK, result.Status)
	}
	// Per PP10: no persisted state, no QF-side mutation initiated.
	// This is enforced by the absence of any side effect in PostCallback
	// itself; the test asserts the result shape only and that the
	// attempt counter incremented under status=ok.
	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(CallbackActionNoop, CallbackStatusOK)); got != 1 {
		t.Fatalf("QFCallbackAttemptsTotal{noop,ok}: want 1, got %v", got)
	}
}

// Helper: reset the callback metrics between tests so increments are
// independently observable.
func resetCallbackMetrics(t *testing.T) {
	t.Helper()
	metrics.QFCallbackAttemptsTotal.Reset()
	metrics.QFCallbackSignatureFailuresTotal.Reset()
}

// Helper: build a single-key keystore for tests. The test secret is a
// stable random-looking string so signature comparisons are deterministic
// across runs.
func keystoreForTest(t *testing.T) *CallbackKeystore {
	t.Helper()
	raw := `[{"key_id":"k-test","secret":"sek-test-2026","not_before":"2026-01-01T00:00:00Z"}]`
	keystore, err := LoadCallbackKeystoreFromJSON(raw)
	if err != nil {
		t.Fatalf("keystoreForTest: %v", err)
	}
	return keystore
}

// Compile-time guard: keep prometheus + testutil imports referenced
// even if test bodies are temporarily commented out.
var _ = prometheus.Collector(metrics.QFCallbackAttemptsTotal)
var _ = testutil.ToFloat64
