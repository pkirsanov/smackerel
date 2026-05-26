//go:build e2e

package e2e

// SCN-SM-041-028 / SCN-SM-041-029 / SCN-SM-041-030 — Scope 8 e2e coverage.
//
// This file drives the QF Companion connector's signed callback path through
// the LIVE Smackerel test stack (Postgres + NATS + core API), proving:
//
//   1. The SST-managed env var QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON is
//      loaded by Connector.Connect, an active key is selected, and the
//      CallbackSigner() / Client() accessors expose the wired pair to render
//      surfaces. (SCN-SM-041-028)
//   2. The signed envelope POSTed to the QF stub carries lower-case-hex
//      HMAC-SHA256, a key_id matching the active key, and a canonical
//      pipe-delimited payload composed of (callback_id, trace_id, packet_id,
//      action, nonce, expires_at, surface) with no whitespace and no trailing
//      pipe. (SCN-SM-041-028)
//   3. The pre-MVP rejection envelope { "code":"CALLBACK_DEFERRED_TO_V1" }
//      surfaces as Status == rejected_v1_deferred with zero local action
//      acceptance and zero retry. (SCN-SM-041-029, PP10)
//   4. All three signature-failure reasons (NO_ACTIVE_KEY,
//      MALFORMED_CANONICAL_PAYLOAD, EXPIRES_AT_OUTSIDE_TOLERANCE) short-circuit
//      before any network send, record the documented reason metric, and emit
//      a cross-product audit envelope. (SCN-SM-041-030)
//
// The live stack health gate enforces that we exercise the real connector
// lifecycle (Connect → CallbackSigner accessor → render.EmitSignedCallback →
// Close) instead of a unit-style shortcut. The QF backend itself is stubbed
// via httptest because a live QF service is not deployed in the test stack;
// the stub serves /capabilities and the callback endpoint deterministically.

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/telegram/render"
)

// keystoreJSONForE2E returns a single-key JSON keystore whose not_before is
// in the past, so the keystore probe in Connector.Connect succeeds.
func keystoreJSONForE2E(t *testing.T, keyID, secret string, notBefore time.Time) string {
	t.Helper()
	type entry struct {
		KeyID     string `json:"key_id"`
		Secret    string `json:"secret"`
		NotBefore string `json:"not_before"`
	}
	payload := []entry{{
		KeyID:     keyID,
		Secret:    secret,
		NotBefore: notBefore.UTC().Format(time.RFC3339),
	}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal keystore json: %v", err)
	}
	return string(raw)
}

// e2eCallbackStub is a deterministic QF stub serving /capabilities and the
// callback endpoint with configurable response status + body.
type e2eCallbackStub struct {
	server *httptest.Server
	hits   atomic.Int64

	mu     sync.Mutex
	status int
	body   string
	bodies [][]byte

	capability qfdecisions.QFBridgeCapability
}

func newE2ECallbackStub(t *testing.T, capability qfdecisions.QFBridgeCapability) *e2eCallbackStub {
	t.Helper()
	s := &e2eCallbackStub{
		status:     http.StatusUnprocessableEntity,
		body:       `{"code":"CALLBACK_DEFERRED_TO_V1","message":"pre-MVP: callbacks deferred until v1"}`,
		capability: capability,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(qfdecisions.CapabilitiesPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.capability)
	})
	mux.HandleFunc(qfdecisions.CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		status := s.status
		responseBody := s.body
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	})
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *e2eCallbackStub) URL() string { return s.server.URL }

func (s *e2eCallbackStub) Hits() int64 { return s.hits.Load() }

func (s *e2eCallbackStub) Bodies() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.bodies))
	copy(out, s.bodies)
	return out
}

func (s *e2eCallbackStub) SetResponse(status int, body string) {
	s.mu.Lock()
	s.status = status
	s.body = body
	s.mu.Unlock()
}

// e2eDefaultCapability returns a CompatibilityCheck-passing capability with
// CallbackSigningSupported=false (pre-MVP) so the connector wires the signer
// without depending on a v1-capable QF backend.
func e2eDefaultCapability() qfdecisions.QFBridgeCapability {
	return qfdecisions.QFBridgeCapability{
		SupportedPacketVersions:        []string{"v1"},
		SupportedEventTypes:            []string{"packet_created", "packet_updated", "packet_trust_changed", "packet_archived", "packet_action_boundary_attempted"},
		SupportedDecisionTypes:         []string{"recommendation", "policy_denial", "analysis_note"},
		MaxPageSize:                    100,
		MinPageSize:                    1,
		SupportedTargetContextTypes:    []string{"packet_context"},
		EvidenceMaxBundleSizeBytes:     1048576,
		EvidenceMaxClaimsPerBundle:     50,
		EvidenceRateLimitPerMinute:     60,
		FreshnessSLAP95Seconds:         60,
		AuditEnvelopeVersion:           "v1",
		WatchSignalDirection:           "qf_to_smackerel",
		EligibleSmackerelSourceClasses: []string{"watch"},
		CallbackSigningSupported:       false,
	}
}

// connectQFWithKeystore brings up a real Connector against the supplied QF stub
// with the provided env-var keystore. Cleanup closes the connector.
func connectQFWithKeystore(t *testing.T, ctx context.Context, stubURL, keystoreJSON, sourceID string) *qfdecisions.Connector {
	t.Helper()
	t.Setenv(qfdecisions.CallbackSigningKeysEnvVar, keystoreJSON)
	c := qfdecisions.New(sourceID)
	if err := c.Connect(ctx, connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-bridge-e2e-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       stubURL,
			"packet_version": 1,
			"page_size":      25,
		},
	}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

// resetCallbackMetricsE2E zeroes the two callback CounterVecs at the start of
// every test so per-test assertions are independent.
func resetCallbackMetricsE2E(t *testing.T) {
	t.Helper()
	metrics.QFCallbackAttemptsTotal.Reset()
	metrics.QFCallbackSignatureFailuresTotal.Reset()
}

// TestQFCallbackSigningWiringThroughLiveSurfaceComposesCanonicalPayloadAndKeyIdEnvelope
// proves SCN-SM-041-028 end-to-end through the live stack: the SST env var
// loads, the connector wires the signer, the signed envelope reaches the QF
// stub with the documented canonical payload + key_id + lower-case-hex
// HMAC-SHA256.
func TestQFCallbackSigningWiringThroughLiveSurfaceComposesCanonicalPayloadAndKeyIdEnvelope(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)
	resetCallbackMetricsE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secret := "e2e-callback-secret-bytes-do-not-leak"
	keystoreJSON := keystoreJSONForE2E(t, "k-e2e-wire", secret, notBefore)

	stub := newE2ECallbackStub(t, e2eDefaultCapability())
	conn := connectQFWithKeystore(t, ctx, stub.URL(), keystoreJSON, "qf-decisions-e2e-callback-wire")

	signer := conn.CallbackSigner()
	if signer == nil {
		t.Fatalf("CallbackSigner accessor returned nil after Connect — signer not wired")
	}
	client := conn.Client()
	if client == nil {
		t.Fatalf("Client accessor returned nil after Connect — transport not wired")
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	env := qfdecisions.CallbackEnvelope{
		CallbackID: "cb-e2e-wire-001",
		TraceID:    "tr-e2e-wire-001",
		PacketID:   "pk-e2e-wire-001",
		Action:     qfdecisions.CallbackActionNoop,
		Nonce:      "nonce-e2e-wire-001",
		ExpiresAt:  expiresAt,
		Surface:    "telegram",
	}

	result, err := render.EmitSignedCallback(ctx, client, signer, env)
	if err != nil {
		t.Fatalf("EmitSignedCallback (pre-MVP rejection should be Go-nil error): %v", err)
	}
	if result.Status != qfdecisions.CallbackStatusRejectedV1Deferred {
		t.Fatalf("Status = %q, want %q", result.Status, qfdecisions.CallbackStatusRejectedV1Deferred)
	}
	if result.QFResponse.RejectionCode != qfdecisions.CallbackRejectionCodeDeferredV1 {
		t.Fatalf("QFResponse rejection code = %+v, want %q", result.QFResponse, qfdecisions.CallbackRejectionCodeDeferredV1)
	}
	if got := stub.Hits(); got != 1 {
		t.Fatalf("QF stub hits = %d, want 1 (Scope 8 must POST exactly once and not retry)", got)
	}

	// Verify the POSTed envelope: parse and confirm signature is lower-case
	// hex, key_id matches active key, and canonical payload composition
	// matches the spec.
	bodies := stub.Bodies()
	if len(bodies) != 1 {
		t.Fatalf("recorded bodies = %d, want 1", len(bodies))
	}
	var postedEnv qfdecisions.CallbackEnvelope
	if err := json.Unmarshal(bodies[0], &postedEnv); err != nil {
		t.Fatalf("unmarshal posted envelope: %v\nbody: %s", err, string(bodies[0]))
	}
	if postedEnv.KeyID != "k-e2e-wire" {
		t.Fatalf("posted KeyID = %q, want %q", postedEnv.KeyID, "k-e2e-wire")
	}
	if len(postedEnv.Signature) != 64 {
		t.Fatalf("posted Signature length = %d, want 64 (HMAC-SHA256 lower-case hex)", len(postedEnv.Signature))
	}
	if strings.ToLower(postedEnv.Signature) != postedEnv.Signature {
		t.Fatalf("posted Signature = %q must be lower-case hex", postedEnv.Signature)
	}
	if _, err := hex.DecodeString(postedEnv.Signature); err != nil {
		t.Fatalf("posted Signature is not valid hex: %v", err)
	}

	// Canonical payload reconstruction must match the documented form
	// callback_id|trace_id|packet_id|action|nonce|expires_at|surface with
	// no whitespace and no trailing pipe.
	canonical, err := qfdecisions.CallbackCanonicalPayload(env)
	if err != nil {
		t.Fatalf("CallbackCanonicalPayload(env) returned error: %v", err)
	}
	if strings.HasSuffix(canonical, "|") {
		t.Fatalf("canonical payload must not end with trailing pipe: %q", canonical)
	}
	if strings.ContainsAny(canonical, " \t\r\n") {
		t.Fatalf("canonical payload must not contain whitespace: %q", canonical)
	}
	wantCanonical := strings.Join([]string{
		env.CallbackID, env.TraceID, env.PacketID, env.Action,
		env.Nonce, env.ExpiresAt, env.Surface,
	}, "|")
	if canonical != wantCanonical {
		t.Fatalf("canonical payload mismatch:\n got: %q\nwant: %q", canonical, wantCanonical)
	}

	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues("noop", "rejected_v1_deferred")); got != 1 {
		t.Fatalf("qf_callback_attempts_total{action=noop,status=rejected_v1_deferred} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.QFCallbackSignatureFailuresTotal); got != 0 {
		t.Fatalf("qf_callback_signature_failures_total series count = %d, want 0 (this is the success path)", got)
	}
}

// TestQFCallbackPreMVPDeferralRejectionThroughLiveSurfaceNoLocalActionAcceptance
// proves SCN-SM-041-029 + PP10 explicitly: the pre-MVP CALLBACK_DEFERRED_TO_V1
// response is parsed deterministically AND no local action is accepted as a
// side effect. PP10 is NON-NEGOTIABLE: Smackerel never executes a QF action.
func TestQFCallbackPreMVPDeferralRejectionThroughLiveSurfaceNoLocalActionAcceptance(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)
	resetCallbackMetricsE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	keystoreJSON := keystoreJSONForE2E(t, "k-e2e-premvp", "premvp-secret", notBefore)

	stub := newE2ECallbackStub(t, e2eDefaultCapability())
	conn := connectQFWithKeystore(t, ctx, stub.URL(), keystoreJSON, "qf-decisions-e2e-callback-premvp")

	signer := conn.CallbackSigner()
	client := conn.Client()
	if signer == nil || client == nil {
		t.Fatalf("signer/client not wired: signer=%v client=%v", signer, client)
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
	env := qfdecisions.CallbackEnvelope{
		CallbackID: "cb-e2e-premvp-001",
		TraceID:    "tr-e2e-premvp-001",
		PacketID:   "pk-e2e-premvp-001",
		Action:     qfdecisions.CallbackActionOpen,
		Nonce:      "nonce-e2e-premvp-001",
		ExpiresAt:  expiresAt,
		Surface:    "telegram",
	}

	result, err := render.EmitSignedCallback(ctx, client, signer, env)
	if err != nil {
		t.Fatalf("EmitSignedCallback (pre-MVP rejection must be Go-nil error): %v", err)
	}
	if result.Status != qfdecisions.CallbackStatusRejectedV1Deferred {
		t.Fatalf("Status = %q, want %q (PP10: pre-MVP rejection must surface deterministically)",
			result.Status, qfdecisions.CallbackStatusRejectedV1Deferred)
	}
	if result.QFResponse.RejectionCode != qfdecisions.CallbackRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode = %q, want %q", result.QFResponse.RejectionCode, qfdecisions.CallbackRejectionCodeDeferredV1)
	}
	if result.QFResponse.HTTPStatus != http.StatusUnprocessableEntity {
		t.Fatalf("HTTPStatus = %d, want %d", result.QFResponse.HTTPStatus, http.StatusUnprocessableEntity)
	}
	// PP10: no LocalRejection — this was a parsed QF rejection, not a local
	// abort. The connector did NOT accept any action locally.
	if result.LocalRejection != nil {
		t.Fatalf("LocalRejection = %+v, want nil (PP10: QF rejection is not a local abort and not a local action acceptance)", result.LocalRejection)
	}

	// Exactly one POST, no retry — PP10/Scope 8 forbid retry of QF
	// rejections.
	if got := stub.Hits(); got != 1 {
		t.Fatalf("stub hits = %d, want 1 (Scope 8: never retry QF rejection)", got)
	}

	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues("open", "rejected_v1_deferred")); got != 1 {
		t.Fatalf("qf_callback_attempts_total{action=open,status=rejected_v1_deferred} = %v, want 1", got)
	}
	if got := testutil.CollectAndCount(metrics.QFCallbackSignatureFailuresTotal); got != 0 {
		t.Fatalf("qf_callback_signature_failures_total series count = %d, want 0", got)
	}
}

// TestQFCallbackStartupFailsLoudWhenKeystoreHasNoActiveKey proves the
// startup-time half of SCN-SM-041-030: a keystore where every key has a
// future not_before causes Connector.Connect to fail-loud with the documented
// "no active callback signing key" error AND emit a cross-product audit
// envelope tagging reason=callback_keystore_no_active_key. The runtime-time
// NO_ACTIVE_KEY abort path is covered by the unit/integration tests that
// instantiate the signer directly; the e2e path exercises the live-stack
// connector lifecycle.
func TestQFCallbackStartupFailsLoudWhenKeystoreHasNoActiveKey(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)
	resetCallbackMetricsE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	futureNotBefore := time.Now().UTC().Add(24 * time.Hour)
	type entry struct {
		KeyID     string `json:"key_id"`
		Secret    string `json:"secret"`
		NotBefore string `json:"not_before"`
	}
	raw, err := json.Marshal([]entry{{
		KeyID:     "k-future-only",
		Secret:    "future-only-secret",
		NotBefore: futureNotBefore.Format(time.RFC3339),
	}})
	if err != nil {
		t.Fatalf("marshal future-only keystore: %v", err)
	}

	stub := newE2ECallbackStub(t, e2eDefaultCapability())
	t.Setenv(qfdecisions.CallbackSigningKeysEnvVar, string(raw))

	c := qfdecisions.New("qf-decisions-e2e-callback-startup-fail")
	connectErr := c.Connect(ctx, connector.ConnectorConfig{
		AuthType:     "token",
		Credentials:  map[string]string{"credential_ref": "qf-bridge-e2e-token"},
		Enabled:      true,
		SyncSchedule: "*/5 * * * *",
		SourceConfig: map[string]any{
			"base_url":       stub.URL(),
			"packet_version": 1,
			"page_size":      25,
		},
	})
	if connectErr == nil {
		_ = c.Close()
		t.Fatalf("Connect returned nil error; want fail-loud for future-only keystore")
	}
	if !strings.Contains(connectErr.Error(), "no active callback signing key") {
		t.Fatalf("Connect error = %v; want substring %q", connectErr, "no active callback signing key")
	}
	if c.CallbackSigner() != nil {
		t.Fatalf("CallbackSigner must remain nil after fail-loud Connect")
	}
	if got := stub.Hits(); got != 0 {
		t.Fatalf("stub hits = %d, want 0 (startup probe must not touch the callback endpoint)", got)
	}
}

// TestQFCallbackSignatureFailureMatrixThroughLiveSurfaceNoNetworkSendAndDiagnosticsRecorded
// proves SCN-SM-041-030 for the two runtime-time signature-failure reasons that
// can occur AFTER the connector has Connected with a healthy keystore:
// MALFORMED_CANONICAL_PAYLOAD and EXPIRES_AT_OUTSIDE_TOLERANCE. Every failure
// aborts locally BEFORE any HTTP send and records the documented reason +
// audit envelope. (The NO_ACTIVE_KEY startup-time path is covered by
// TestQFCallbackStartupFailsLoudWhenKeystoreHasNoActiveKey above; the runtime
// NO_ACTIVE_KEY signer-time path is covered by the integration test
// TestQFCallbackSignatureFailureMatrixAbortsLocallyAndRecordsDiagnosticsAcrossAllThreeReasons.)
func TestQFCallbackSignatureFailureMatrixThroughLiveSurfaceNoNetworkSendAndDiagnosticsRecorded(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)

	type matrixCase struct {
		name        string
		envOverride func(qfdecisions.CallbackEnvelope) qfdecisions.CallbackEnvelope
		wantReason  string
		action      string
	}

	pastNotBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	healthyKeystore := keystoreJSONForE2E(t, "k-healthy", "healthy-secret", pastNotBefore)

	cases := []matrixCase{
		{
			name: "MALFORMED_CANONICAL_PAYLOAD",
			envOverride: func(e qfdecisions.CallbackEnvelope) qfdecisions.CallbackEnvelope {
				e.Nonce = "bad|nonce"
				return e
			},
			wantReason: qfdecisions.CallbackSignatureFailureMalformedCanonicalPayload,
			action:     qfdecisions.CallbackActionNoop,
		},
		{
			name: "EXPIRES_AT_OUTSIDE_TOLERANCE",
			envOverride: func(e qfdecisions.CallbackEnvelope) qfdecisions.CallbackEnvelope {
				// 5 minutes in the past — well outside the 60s tolerance.
				e.ExpiresAt = time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339)
				return e
			},
			wantReason: qfdecisions.CallbackSignatureFailureExpiresAtOutsideTolerance,
			action:     qfdecisions.CallbackActionNoop,
		},
	}

	for idx, tc := range cases {
		tc := tc
		idx := idx
		t.Run(tc.name, func(t *testing.T) {
			resetCallbackMetricsE2E(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Fail-loud QF stub: a 500 here would prove a regression
			// because Scope 8 must abort BEFORE any network send when
			// signing fails.
			stub := newE2ECallbackStub(t, e2eDefaultCapability())
			stub.SetResponse(http.StatusInternalServerError, `{"code":"E2E_FORBIDDEN_NETWORK_CALL_ON_SIGNATURE_FAILURE"}`)

			conn := connectQFWithKeystore(t, ctx, stub.URL(), healthyKeystore,
				fmt.Sprintf("qf-decisions-e2e-callback-sigfail-%d", idx))

			signer := conn.CallbackSigner()
			client := conn.Client()
			if signer == nil || client == nil {
				t.Fatalf("signer/client not wired: signer=%v client=%v", signer, client)
			}

			expiresAt := time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339)
			env := qfdecisions.CallbackEnvelope{
				CallbackID: fmt.Sprintf("cb-e2e-sigfail-%s", tc.name),
				TraceID:    fmt.Sprintf("tr-e2e-sigfail-%s", tc.name),
				PacketID:   fmt.Sprintf("pk-e2e-sigfail-%s", tc.name),
				Action:     tc.action,
				Nonce:      fmt.Sprintf("nonce-e2e-sigfail-%s", tc.name),
				ExpiresAt:  expiresAt,
				Surface:    "telegram",
			}
			env = tc.envOverride(env)

			result, err := render.EmitSignedCallback(ctx, client, signer, env)
			if err == nil {
				t.Fatalf("EmitSignedCallback returned nil error; want CallbackSignatureFailure for reason %q", tc.wantReason)
			}
			var sigErr *qfdecisions.CallbackSignatureFailure
			if !errors.As(err, &sigErr) {
				t.Fatalf("error %v is not *CallbackSignatureFailure", err)
			}
			if sigErr.Reason != tc.wantReason {
				t.Fatalf("Reason = %q, want %q", sigErr.Reason, tc.wantReason)
			}
			if result.Status != qfdecisions.CallbackStatusRejectedLocal {
				t.Fatalf("Status = %q, want %q", result.Status, qfdecisions.CallbackStatusRejectedLocal)
			}
			if result.QFResponse.RejectionCode != "" || result.QFResponse.HTTPStatus != 0 {
				t.Fatalf("QFResponse must be zero-value on local signature failure: %+v", result.QFResponse)
			}
			if got := stub.Hits(); got != 0 {
				t.Fatalf("stub hits = %d, want 0 (Scope 8: signature failures MUST NOT touch the network)", got)
			}

			if got := testutil.ToFloat64(metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(tc.wantReason)); got != 1 {
				t.Fatalf("qf_callback_signature_failures_total{reason=%q} = %v, want 1", tc.wantReason, got)
			}
			if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(tc.action, qfdecisions.CallbackStatusRejectedLocal)); got != 1 {
				t.Fatalf("qf_callback_attempts_total{action=%q,status=%q} = %v, want 1", tc.action, qfdecisions.CallbackStatusRejectedLocal, got)
			}
		})
	}
}
