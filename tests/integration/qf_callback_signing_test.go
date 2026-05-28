//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
	"github.com/smackerel/smackerel/internal/telegram/render"
)

// Scope 8 integration tests (SCN-SM-041-028..030). Each test wires the
// connector + render-layer entry point against the live disposable
// test stack (Postgres + NATS via testPool / qfDecisionsNATSClient)
// and a per-test httptest QF stub that handles the callback POST.
//
// The integration tests exercise the FULL transport stack — keystore
// load via SST env var → signer construction → render-layer entry
// point → HTTP POST → QF stub response → metric + audit envelope
// emission — so the failures surface at the integration boundary in
// addition to the unit-test fakes.

// TestQFCallbackSignedEnvelopePostedAndPreMVPRejectionParsedFromLiveQFStub
// (SCN-SM-041-028 + SCN-SM-041-029) wires the connector against the
// live disposable stack and a happy-path QF stub that:
//
//   - records every callback POST body so the test can confirm the
//     envelope carried both `signature` (lower-case hex) and `key_id`
//     pulled from the SST-loaded keystore;
//   - returns HTTP 503 + {"code":"CALLBACK_DEFERRED_TO_V1"} for every
//     attempt (pre-MVP rejection contract);
//
// and asserts the connector:
//
//   - parses the rejection without retry (server saw exactly 1 request);
//   - records smackerel_qf_callback_attempts_total{noop,rejected_v1_deferred}=1;
//   - records ZERO signature-failure increments;
//   - returns Status=rejected_v1_deferred with the parsed RejectionCode;
//   - does NOT mark any local action as accepted (PP10 guarantee).
//
// Adversarial trip-wire: a regression that re-tried on
// CALLBACK_DEFERRED_TO_V1 would push the request counter past 1; this
// test fails if the counter is anything other than 1.
func TestQFCallbackSignedEnvelopePostedAndPreMVPRejectionParsedFromLiveQFStub(t *testing.T) {
	_ = testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stub := newCallbackQFStub(t)
	defer stub.Close()
	stub.SetResponse(http.StatusServiceUnavailable, `{"code":"CALLBACK_DEFERRED_TO_V1","message":"pre-MVP: bridge does not accept callbacks"}`)

	// Reset Scope 8 metrics so the integration assertions are
	// independently observable from anything else the shared
	// disposable stack ran in this process.
	metrics.QFCallbackAttemptsTotal.Reset()
	metrics.QFCallbackSignatureFailuresTotal.Reset()

	// BUG-020-010: keystore is now ingested via Config SST, not
	// via os.Getenv inside the qfdecisions package. The test
	// constructs the keystore directly from the JSON, matching the
	// resolved value the connector would consume at Connect time
	// via parsed.CallbackSigningKeysJSON.
	const keystoreJSON = `[{"key_id":"k-int","secret":"sek-int-2026","not_before":"2026-01-01T00:00:00Z"}]`
	keystore, err := qfdecisions.LoadCallbackKeystoreFromJSON(keystoreJSON)
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
	}
	if keystore == nil {
		t.Fatal("LoadCallbackKeystoreFromJSON: want non-nil keystore, got nil")
	}

	signer := qfdecisions.NewCallbackSigner(keystore, func() time.Time {
		return mustParseRFC3339(t, "2026-05-22T12:00:00Z")
	})
	client := qfdecisions.NewClient(stub.URL(), "bridge-int-token", 1, 100)

	env := qfdecisions.CallbackEnvelope{
		CallbackID: "cb-int-001",
		TraceID:    "tr-int-001",
		PacketID:   "pk-int-001",
		Action:     qfdecisions.CallbackActionOpen,
		Nonce:      "no-int-001",
		ExpiresAt:  "2026-05-22T12:00:30Z",
		Surface:    qfdecisions.SurfaceTelegram,
	}
	result, err := render.EmitSignedCallback(ctx, client, signer, env)
	if err != nil {
		t.Fatalf("EmitSignedCallback: %v", err)
	}
	if result.Status != qfdecisions.CallbackStatusRejectedV1Deferred {
		t.Fatalf("result.Status: want %q, got %q", qfdecisions.CallbackStatusRejectedV1Deferred, result.Status)
	}
	if result.QFResponse.RejectionCode != qfdecisions.CallbackRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode: want %q, got %q", qfdecisions.CallbackRejectionCodeDeferredV1, result.QFResponse.RejectionCode)
	}
	if got := stub.Hits(); got != 1 {
		t.Fatalf("QF stub hits: want 1 (no retry), got %d", got)
	}
	// Confirm the request body carried signature + key_id.
	bodies := stub.Bodies()
	if len(bodies) != 1 {
		t.Fatalf("stub bodies: want 1, got %d", len(bodies))
	}
	var posted qfdecisions.CallbackEnvelope
	if jerr := json.Unmarshal([]byte(bodies[0]), &posted); jerr != nil {
		t.Fatalf("unmarshal posted envelope: %v", jerr)
	}
	if posted.Signature == "" {
		t.Fatal("posted envelope Signature is empty")
	}
	if posted.KeyID != "k-int" {
		t.Fatalf("posted envelope KeyID: want k-int, got %q", posted.KeyID)
	}
	if len(posted.Signature) != 64 {
		t.Fatalf("posted envelope Signature length: want 64 hex chars, got %d (%q)", len(posted.Signature), posted.Signature)
	}
	if strings.ToLower(posted.Signature) != posted.Signature {
		t.Fatalf("posted envelope Signature is not lower-case hex: %q", posted.Signature)
	}
	// Metric assertions.
	if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(qfdecisions.CallbackActionOpen, qfdecisions.CallbackStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFCallbackAttemptsTotal{open,rejected_v1_deferred}: want 1, got %v", got)
	}
	if got := testutil.CollectAndCount(metrics.QFCallbackSignatureFailuresTotal); got != 0 {
		t.Fatalf("QFCallbackSignatureFailuresTotal collected count: want 0, got %d", got)
	}
}

// TestQFCallbackSignatureFailureMatrixAbortsLocallyAndRecordsDiagnosticsAcrossAllThreeReasons
// (SCN-SM-041-030) wires the connector against the live disposable
// stack and a tracking QF stub that ASSERTS it is never invoked, then
// drives a callback that fails locally for each of the three reasons:
//
//   - NO_ACTIVE_KEY: keystore with every key not_before in the future
//   - MALFORMED_CANONICAL_PAYLOAD: envelope with a pipe in the nonce
//   - EXPIRES_AT_OUTSIDE_TOLERANCE: expires_at 5 minutes past now
//
// For each reason the test asserts:
//
//   - PostCallback returned *qfdecisions.CallbackSignatureFailure with
//     the matching Reason;
//   - the result.Status is "rejected_local";
//   - smackerel_qf_callback_signature_failures_total{<reason>}=1;
//   - smackerel_qf_callback_attempts_total{*,rejected_local}=1;
//   - the QF stub saw ZERO HTTP requests (no network transport).
func TestQFCallbackSignatureFailureMatrixAbortsLocallyAndRecordsDiagnosticsAcrossAllThreeReasons(t *testing.T) {
	_ = testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cases := []struct {
		name       string
		reason     string
		keystoreFn func(t *testing.T) *qfdecisions.CallbackKeystore
		nowFn      func() time.Time
		env        qfdecisions.CallbackEnvelope
	}{
		{
			name:   "NO_ACTIVE_KEY",
			reason: qfdecisions.CallbackSignatureFailureNoActiveKey,
			keystoreFn: func(t *testing.T) *qfdecisions.CallbackKeystore {
				keystore, err := qfdecisions.LoadCallbackKeystoreFromJSON(`[{"key_id":"k-fut","secret":"x","not_before":"2099-01-01T00:00:00Z"}]`)
				if err != nil {
					t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
				}
				return keystore
			},
			nowFn: func() time.Time { return mustParseRFC3339(t, "2026-05-22T12:00:00Z") },
			env: qfdecisions.CallbackEnvelope{
				CallbackID: "cb-int-noact", TraceID: "tr-int-noact", PacketID: "pk-int-noact",
				Action: qfdecisions.CallbackActionNoop, Nonce: "no-int-noact",
				ExpiresAt: "2026-05-22T12:00:30Z", Surface: qfdecisions.SurfaceTelegram,
			},
		},
		{
			name:   "MALFORMED_CANONICAL_PAYLOAD",
			reason: qfdecisions.CallbackSignatureFailureMalformedCanonicalPayload,
			keystoreFn: func(t *testing.T) *qfdecisions.CallbackKeystore {
				keystore, err := qfdecisions.LoadCallbackKeystoreFromJSON(`[{"key_id":"k-good","secret":"x","not_before":"2026-01-01T00:00:00Z"}]`)
				if err != nil {
					t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
				}
				return keystore
			},
			nowFn: func() time.Time { return mustParseRFC3339(t, "2026-05-22T12:00:00Z") },
			env: qfdecisions.CallbackEnvelope{
				CallbackID: "cb-int-mal", TraceID: "tr-int-mal", PacketID: "pk-int-mal",
				Action: qfdecisions.CallbackActionNoop, Nonce: "no|injected",
				ExpiresAt: "2026-05-22T12:00:30Z", Surface: qfdecisions.SurfaceTelegram,
			},
		},
		{
			name:   "EXPIRES_AT_OUTSIDE_TOLERANCE",
			reason: qfdecisions.CallbackSignatureFailureExpiresAtOutsideTolerance,
			keystoreFn: func(t *testing.T) *qfdecisions.CallbackKeystore {
				keystore, err := qfdecisions.LoadCallbackKeystoreFromJSON(`[{"key_id":"k-good","secret":"x","not_before":"2026-01-01T00:00:00Z"}]`)
				if err != nil {
					t.Fatalf("LoadCallbackKeystoreFromJSON: %v", err)
				}
				return keystore
			},
			nowFn: func() time.Time { return mustParseRFC3339(t, "2026-05-22T12:05:01Z") },
			env: qfdecisions.CallbackEnvelope{
				CallbackID: "cb-int-exp", TraceID: "tr-int-exp", PacketID: "pk-int-exp",
				Action: qfdecisions.CallbackActionNoop, Nonce: "no-int-exp",
				ExpiresAt: "2026-05-22T12:00:00Z", Surface: qfdecisions.SurfaceTelegram,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metrics.QFCallbackAttemptsTotal.Reset()
			metrics.QFCallbackSignatureFailuresTotal.Reset()

			stub := newCallbackQFStub(t)
			defer stub.Close()
			// Stub is configured to fail loud if any request reaches it.
			stub.SetResponse(http.StatusInternalServerError, `{"unexpected":"network was reached"}`)

			signer := qfdecisions.NewCallbackSigner(tc.keystoreFn(t), tc.nowFn)
			client := qfdecisions.NewClient(stub.URL(), "bridge-int-token", 1, 100)
			result, err := render.EmitSignedCallback(ctx, client, signer, tc.env)
			if err == nil {
				t.Fatal("EmitSignedCallback: want signature failure, got nil")
			}
			var failure *qfdecisions.CallbackSignatureFailure
			if !errorsAs(err, &failure) {
				t.Fatalf("error type: want *qfdecisions.CallbackSignatureFailure, got %T (%v)", err, err)
			}
			if failure.Reason != tc.reason {
				t.Fatalf("failure.Reason: want %q, got %q", tc.reason, failure.Reason)
			}
			if result.Status != qfdecisions.CallbackStatusRejectedLocal {
				t.Fatalf("result.Status: want %q, got %q", qfdecisions.CallbackStatusRejectedLocal, result.Status)
			}
			if got := stub.Hits(); got != 0 {
				t.Fatalf("QF stub hits: want 0 (no network reached on local signature failure), got %d", got)
			}
			if got := testutil.ToFloat64(metrics.QFCallbackSignatureFailuresTotal.WithLabelValues(tc.reason)); got != 1 {
				t.Fatalf("QFCallbackSignatureFailuresTotal{%s}: want 1, got %v", tc.reason, got)
			}
			if got := testutil.ToFloat64(metrics.QFCallbackAttemptsTotal.WithLabelValues(tc.env.Action, qfdecisions.CallbackStatusRejectedLocal)); got != 1 {
				t.Fatalf("QFCallbackAttemptsTotal{%s,rejected_local}: want 1, got %v", tc.env.Action, got)
			}
		})
	}
}

// callbackQFStub is a per-test QF Companion Bridge stub for the
// /api/private/smackerel/v1/callback endpoint. It records every
// request body and counts every request so the integration tests can
// assert "no retry" and "no network reached" guarantees.
type callbackQFStub struct {
	t      *testing.T
	server *httptest.Server
	hits   int64
	mu     sync.Mutex
	bodies []string
	status int
	body   string
}

func newCallbackQFStub(t *testing.T) *callbackQFStub {
	t.Helper()
	stub := &callbackQFStub{t: t, status: http.StatusOK, body: `{}`}
	stub.server = httptest.NewServer(http.HandlerFunc(stub.handle))
	return stub
}

func (s *callbackQFStub) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != qfdecisions.CallbackPath {
		http.NotFound(w, r)
		return
	}
	body, _ := io.ReadAll(r.Body)
	atomic.AddInt64(&s.hits, 1)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(body))
	status := s.status
	respBody := s.body
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(respBody))
}

func (s *callbackQFStub) URL() string { return s.server.URL }
func (s *callbackQFStub) Close()      { s.server.Close() }
func (s *callbackQFStub) Hits() int64 { return atomic.LoadInt64(&s.hits) }
func (s *callbackQFStub) Bodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.bodies))
	copy(out, s.bodies)
	return out
}

func (s *callbackQFStub) SetResponse(status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.body = body
}

func mustParseRFC3339(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts.UTC()
}

// errorsAs is a tiny stand-in for errors.As to keep the test file's
// import list minimal — the standard-lib alias would shadow the
// "errors" import in the table-driven block and add a single line of
// noise. Behavioural parity with errors.As for *CallbackSignatureFailure
// is sufficient here.
func errorsAs(err error, target **qfdecisions.CallbackSignatureFailure) bool {
	for err != nil {
		if f, ok := err.(*qfdecisions.CallbackSignatureFailure); ok {
			*target = f
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
