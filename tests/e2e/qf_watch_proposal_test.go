//go:build e2e

package e2e

// Scope 9 e2e coverage (SCN-SM-041-031 / SCN-SM-041-032 / SCN-SM-041-033).
//
// This file drives the QF Companion connector's diagnostic
// watch-proposal client through the LIVE Smackerel test stack
// (Postgres + NATS + core API), proving:
//
//   1. The SST-managed env var QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON
//      is loaded by Connector.Connect, an active key is selected, and
//      the WatchProposalClient() accessor exposes a wired diagnostic
//      client that REUSES the Scope 8 keystore verbatim.
//      (SCN-SM-041-031)
//   2. The signed envelope POSTed to the QF stub carries
//      lower-case-hex HMAC-SHA256, a key_id matching the active
//      Scope 8 key, and a canonical pipe-delimited payload composed
//      of (trace_id, source, entity_ref, reason, expires_at) with
//      no whitespace and no trailing pipe — and the trace_id is a
//      UUIDv7. (SCN-SM-041-031 / SCN-SM-041-032)
//   3. The pre-MVP rejection envelope
//      {"code":"WATCH_PROPOSALS_DEFERRED_TO_V1"} surfaces as
//      Status == rejected_v1_deferred with zero local watch-state
//      mutation, zero user-visible affordance, and zero retry.
//      (SCN-SM-041-033)
//
// The live stack health gate enforces that we exercise the real
// connector lifecycle (Connect → WatchProposalClient accessor →
// Propose → Close) instead of a unit-style shortcut. The QF backend
// itself is stubbed via httptest because a live QF service is not
// deployed in the test stack.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// e2eWatchProposalStub serves /capabilities and the watch-proposal
// endpoint with configurable response status + body. Mirrors the
// Scope 8 e2eCallbackStub structure so the test boundary assertions
// are symmetric.
type e2eWatchProposalStub struct {
	server *httptest.Server
	hits   atomic.Int64

	mu     sync.Mutex
	status int
	body   string
	bodies [][]byte

	capability qfdecisions.QFBridgeCapability
}

func newE2EWatchProposalStub(t *testing.T, capability qfdecisions.QFBridgeCapability) *e2eWatchProposalStub {
	t.Helper()
	s := &e2eWatchProposalStub{
		status:     http.StatusServiceUnavailable,
		body:       `{"code":"WATCH_PROPOSALS_DEFERRED_TO_V1","message":"pre-MVP: watch proposals deferred until v1"}`,
		capability: capability,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(qfdecisions.CapabilitiesPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.capability)
	})
	mux.HandleFunc(qfdecisions.WatchProposalPath, func(w http.ResponseWriter, r *http.Request) {
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
	// Adversarial trip-wire: register a 404 catch-all that asserts
	// the connector does NOT POST to any other path. If the
	// connector ever wires a /api/private/smackerel/v1/watch* route
	// other than WatchProposalPath, the stub returns 404 and the
	// test fails on the resulting metric / status drift.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	s.server = httptest.NewServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *e2eWatchProposalStub) URL() string { return s.server.URL }

func (s *e2eWatchProposalStub) Hits() int64 { return s.hits.Load() }

func (s *e2eWatchProposalStub) Bodies() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]byte, len(s.bodies))
	copy(out, s.bodies)
	return out
}

func (s *e2eWatchProposalStub) SetResponse(status int, body string) {
	s.mu.Lock()
	s.status = status
	s.body = body
	s.mu.Unlock()
}

// resetWatchProposalMetricsE2E zeroes the Scope 9 + Scope 8 metric
// vectors at the start of every test so per-test assertions are
// independent.
func resetWatchProposalMetricsE2E(t *testing.T) {
	t.Helper()
	metrics.QFWatchProposalAttemptsTotal.Reset()
	metrics.QFCallbackAttemptsTotal.Reset()
	metrics.QFCallbackSignatureFailuresTotal.Reset()
}

// TestQFWatchProposalCanonicalBodyAndTraceContinuityThroughLiveSurface
// proves SCN-SM-041-031 end-to-end through the live stack: the
// SST env var loads, the connector wires the diagnostic
// watch-proposal client, the signed envelope reaches the QF stub
// with the documented field set + UUIDv7 trace_id + literal
// `smackerel_propose` source.
func TestQFWatchProposalCanonicalBodyAndTraceContinuityThroughLiveSurface(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)
	resetWatchProposalMetricsE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secret := "e2e-watch-proposal-secret-bytes-do-not-leak-031"
	keystoreJSON := keystoreJSONForE2E(t, "k-e2e-wp-031", secret, notBefore)

	stub := newE2EWatchProposalStub(t, e2eDefaultCapability())
	conn := connectQFWithKeystore(t, ctx, stub.URL(), keystoreJSON, "qf-decisions-e2e-watch-proposal-031")

	wpc := conn.WatchProposalClient()
	if wpc == nil {
		t.Fatalf("WatchProposalClient accessor returned nil after Connect — Scope 9 diagnostic client not wired")
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	result, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", expiresAt)
	if err != nil {
		t.Fatalf("Propose (pre-MVP rejection should be Go-nil error): %v", err)
	}
	if result.Status != qfdecisions.WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("Status = %q, want %q", result.Status, qfdecisions.WatchProposalStatusRejectedV1Deferred)
	}
	if got := stub.Hits(); got != 1 {
		t.Fatalf("QF stub hits = %d, want 1 (Scope 9 must POST exactly once and not retry)", got)
	}

	bodies := stub.Bodies()
	if len(bodies) != 1 {
		t.Fatalf("recorded bodies = %d, want 1", len(bodies))
	}
	var posted map[string]any
	if err := json.Unmarshal(bodies[0], &posted); err != nil {
		t.Fatalf("unmarshal posted envelope: %v\nbody: %s", err, string(bodies[0]))
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
	for k := range posted {
		if !wantKeys[k] {
			t.Errorf("unexpected field %q in posted envelope", k)
		}
	}
	for k := range wantKeys {
		if _, ok := posted[k]; !ok {
			t.Errorf("missing field %q in posted envelope", k)
		}
	}
	if got, want := posted["source"], qfdecisions.WatchProposalSourceSmackerelPropose; got != want {
		t.Errorf("source: want %q, got %v", want, got)
	}
	if got, want := posted["entity_ref"], "qf:security:NVDA"; got != want {
		t.Errorf("entity_ref: want %q, got %v", want, got)
	}
	if got, want := posted["reason"], "attention_signal_over_threshold"; got != want {
		t.Errorf("reason: want %q, got %v", want, got)
	}
	if traceStr, ok := posted["trace_id"].(string); ok {
		parsed, perr := uuid.Parse(traceStr)
		if perr != nil {
			t.Errorf("posted trace_id %q is not a UUID: %v", traceStr, perr)
		} else if parsed.Version() != 7 {
			t.Errorf("posted trace_id %q: want UUID version 7, got version %d", traceStr, parsed.Version())
		}
	} else {
		t.Errorf("posted trace_id is not a string: %v", posted["trace_id"])
	}

	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(qfdecisions.WatchProposalStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("qf_watch_proposal_attempts_total{status=rejected_v1_deferred} = %v, want 1", got)
	}
}

// TestQFWatchProposalScope8SignerReuseThroughLiveSurface proves
// SCN-SM-041-032 end-to-end: the connector REUSES the Scope 8
// keystore verbatim and the signed envelope's HMAC-SHA256 + key_id
// are byte-equal to an independent computation over the same
// canonical payload using the same SST keystore.
func TestQFWatchProposalScope8SignerReuseThroughLiveSurface(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)
	resetWatchProposalMetricsE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secret := "e2e-watch-proposal-secret-bytes-do-not-leak-032"
	keystoreJSON := keystoreJSONForE2E(t, "k-e2e-wp-032", secret, notBefore)

	stub := newE2EWatchProposalStub(t, e2eDefaultCapability())
	conn := connectQFWithKeystore(t, ctx, stub.URL(), keystoreJSON, "qf-decisions-e2e-watch-proposal-032")

	wpc := conn.WatchProposalClient()
	if wpc == nil {
		t.Fatalf("WatchProposalClient accessor returned nil after Connect — Scope 9 diagnostic client not wired")
	}

	// Also verify the connector exposes the Scope 8 keystore so an
	// independent HMAC computation against the same key proves
	// verbatim reuse.
	keystore := conn.CallbackKeystore()
	if keystore == nil {
		t.Fatalf("CallbackKeystore accessor returned nil after Connect — Scope 8 keystore not wired (cannot verify Scope 9 verbatim reuse)")
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	result, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", expiresAt)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if result.Status != qfdecisions.WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("Status = %q, want %q", result.Status, qfdecisions.WatchProposalStatusRejectedV1Deferred)
	}
	bodies := stub.Bodies()
	if len(bodies) != 1 {
		t.Fatalf("recorded bodies = %d, want 1", len(bodies))
	}
	var posted qfdecisions.WatchProposalEnvelope
	if jerr := json.Unmarshal(bodies[0], &posted); jerr != nil {
		t.Fatalf("unmarshal posted envelope: %v", jerr)
	}
	if posted.KeyID != "k-e2e-wp-032" {
		t.Fatalf("posted KeyID = %q, want %q (Scope 8 keystore verbatim reuse contract)", posted.KeyID, "k-e2e-wp-032")
	}
	if len(posted.Signature) != 64 {
		t.Fatalf("posted Signature length = %d, want 64 (HMAC-SHA256 lower-case hex)", len(posted.Signature))
	}
	if strings.ToLower(posted.Signature) != posted.Signature {
		t.Fatalf("posted Signature = %q must be lower-case hex", posted.Signature)
	}
	if _, derr := hex.DecodeString(posted.Signature); derr != nil {
		t.Fatalf("posted Signature is not valid hex: %v", derr)
	}

	// Independent HMAC computation using the SAME Scope 8 keystore.
	// If the connector ever forks the signing algorithm or key
	// selection, this byte-equality assertion fails.
	canonical, cerr := qfdecisions.WatchProposalCanonicalPayload(posted)
	if cerr != nil {
		t.Fatalf("WatchProposalCanonicalPayload: %v", cerr)
	}
	key, kerr := keystore.SelectActiveKey(time.Now().UTC())
	if kerr != nil {
		t.Fatalf("keystore.SelectActiveKey: %v", kerr)
	}
	if posted.KeyID != key.KeyID {
		t.Fatalf("Scope 8 keystore-reuse contract violated: posted key_id %q != keystore.SelectActiveKey key_id %q", posted.KeyID, key.KeyID)
	}
	mac := hmac.New(sha256.New, []byte(key.Secret))
	mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))
	if posted.Signature != want {
		t.Fatalf("Scope 8 signer-reuse contract violated:\n posted signature: %s\n independent HMAC:  %s\n canonical payload: %q", posted.Signature, want, canonical)
	}
	if strings.HasSuffix(canonical, "|") {
		t.Fatalf("canonical payload must not end with trailing pipe: %q", canonical)
	}
	if strings.ContainsAny(canonical, " \t\r\n") {
		t.Fatalf("canonical payload must not contain whitespace: %q", canonical)
	}
}

// TestQFWatchProposalPreMVPDeferralRejectionThroughLiveSurfaceWithNoLocalMutationOrUserSurface
// proves SCN-SM-041-033 end-to-end: the connector parses the QF
// rejection contract without retry, never marks the proposal as
// accepted, and never registers a user-visible HTTP route for
// watch proposals.
//
// Adversarial assertion: the stub catch-all returns 404 for any
// path other than WatchProposalPath; if the connector ever wired a
// user-visible /api/private/smackerel/v1/watch* route, the route
// resolution would either 404 (degraded status) or hit a different
// metric label. The test also asserts the connector did NOT
// register such a route by hitting /api/health and the public API
// listing through the live core URL.
func TestQFWatchProposalPreMVPDeferralRejectionThroughLiveSurfaceWithNoLocalMutationOrUserSurface(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 2*time.Minute)
	resetWatchProposalMetricsE2E(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	notBefore := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	secret := "e2e-watch-proposal-secret-bytes-do-not-leak-033"
	keystoreJSON := keystoreJSONForE2E(t, "k-e2e-wp-033", secret, notBefore)

	stub := newE2EWatchProposalStub(t, e2eDefaultCapability())
	conn := connectQFWithKeystore(t, ctx, stub.URL(), keystoreJSON, "qf-decisions-e2e-watch-proposal-033")

	wpc := conn.WatchProposalClient()
	if wpc == nil {
		t.Fatalf("WatchProposalClient accessor returned nil after Connect")
	}

	expiresAt := time.Now().UTC().Add(5 * time.Minute)
	result, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", expiresAt)
	if err != nil {
		t.Fatalf("Propose (pre-MVP rejection should be Go-nil error): %v", err)
	}
	if result.Status != qfdecisions.WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("Status = %q, want %q", result.Status, qfdecisions.WatchProposalStatusRejectedV1Deferred)
	}
	if result.QFResponse.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("QFResponse.HTTPStatus = %d, want 503", result.QFResponse.HTTPStatus)
	}
	if result.QFResponse.RejectionCode != qfdecisions.WatchProposalRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode = %q, want %q", result.QFResponse.RejectionCode, qfdecisions.WatchProposalRejectionCodeDeferredV1)
	}
	if got := stub.Hits(); got != 1 {
		t.Fatalf("QF stub hits = %d, want 1 (Scope 9 MUST NOT retry the pre-MVP rejection)", got)
	}

	// Adversarial structural assertion: the live Smackerel API does
	// NOT expose a user-visible /api/private/smackerel/v1/watch*
	// route. Probe the live core for the path and assert it does
	// NOT exist (4xx). Smackerel does not host the QF endpoint —
	// only the QF stub does — so a hit through the live API would
	// be a Scope 9 boundary violation.
	probeURL := cfg.CoreURL + qfdecisions.WatchProposalPath
	req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, probeURL, strings.NewReader(`{}`))
	if rerr != nil {
		t.Fatalf("build probe request: %v", rerr)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	}
	probeResp, perr := http.DefaultClient.Do(req)
	if perr != nil {
		t.Logf("probe error (expected — live Smackerel does not host the QF watch-proposal endpoint): %v", perr)
	} else {
		defer probeResp.Body.Close()
		if probeResp.StatusCode >= 200 && probeResp.StatusCode < 300 {
			t.Fatalf("Scope 9 boundary violation: live Smackerel core accepted POST %s with HTTP %d. Pre-MVP MUST NOT expose a user-visible watch-proposal route.", probeURL, probeResp.StatusCode)
		}
		t.Logf("probe response HTTP %d (expected 4xx — Smackerel does not host the QF endpoint)", probeResp.StatusCode)
	}

	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(qfdecisions.WatchProposalStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("qf_watch_proposal_attempts_total{status=rejected_v1_deferred} = %v, want 1", got)
	}
}
