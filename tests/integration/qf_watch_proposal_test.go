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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Scope 9 integration tests (SCN-SM-041-031..033). Each test wires
// the connector watch-proposal client + Scope 8 keystore (reused
// verbatim) against the live disposable test stack (Postgres + NATS
// via testPool / qfDecisionsNATSClient) and a per-test httptest QF
// watch-proposal stub.
//
// The integration boundary covers:
//
//   - keystore load via SST env var (QF_DECISIONS_CALLBACK_SIGNING_KEYS_JSON)
//   - watch-proposal signer construction (verbatim Scope 8 reuse)
//   - canonical-payload composition + HMAC-SHA256 + envelope POST
//   - QF response parsing (pre-MVP WATCH_PROPOSALS_DEFERRED_TO_V1)
//   - metric + Cross-Product Audit Envelope v1 emission
//
// The disposable stack is referenced via testPool and
// qfDecisionsNATSClient even though Scope 9 does NOT persist
// proposals or publish them on NATS pre-MVP; the reference ensures
// the integration build tag triggers the same stack provisioning the
// other QF integration tests rely on, AND the live-stack DB handle
// is available for the adversarial "no watch-state mutation"
// assertion in
// TestQFWatchProposalPreMVPRejectionParsedAndNoLocalWatchStateMutatedAcrossLiveStack.

// TestQFWatchProposalSignedEnvelopePostedAndScope8SignerReusedAgainstLiveQFStub
// (SCN-SM-041-031 + SCN-SM-041-032) wires the watch-proposal client
// against the live disposable stack and a watch-proposal QF stub
// that:
//
//   - records every POST body so the test can confirm the envelope
//     carried both `signature` (lower-case hex, 64 chars) and `key_id`
//     pulled from the SST-loaded keystore — proving verbatim Scope 8
//     keystore reuse end-to-end;
//   - returns HTTP 503 + {"code":"WATCH_PROPOSALS_DEFERRED_TO_V1"}
//     for every attempt (pre-MVP rejection contract);
//
// and asserts the watch-proposal client:
//
//   - parses the rejection without retry (server saw exactly 1 request);
//   - records smackerel_qf_watch_proposal_attempts_total{status=rejected_v1_deferred}=1;
//   - records ZERO signature-failure increments (Scope 8 metric
//     vocabulary inherits unchanged for Scope 9 signing path);
//   - returns Status=rejected_v1_deferred with the parsed RejectionCode.
//
// Adversarial trip-wire: a regression that re-tried on
// WATCH_PROPOSALS_DEFERRED_TO_V1 would push the request counter past
// 1; this test fails if the counter is anything other than 1.
func TestQFWatchProposalSignedEnvelopePostedAndScope8SignerReusedAgainstLiveQFStub(t *testing.T) {
	_ = testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stub := newWatchProposalQFStub(t)
	defer stub.Close()
	stub.SetResponse(http.StatusServiceUnavailable, `{"code":"WATCH_PROPOSALS_DEFERRED_TO_V1","message":"pre-MVP: bridge does not accept watch proposals"}`)

	metrics.QFCallbackAttemptsTotal.Reset()
	metrics.QFCallbackSignatureFailuresTotal.Reset()
	metrics.QFWatchProposalAttemptsTotal.Reset()

	// Load keystore the same way the connector does at Connect time:
	// from the SST env var. Scope 9 REUSES the Scope 8 keystore
	// verbatim — the integration boundary proves that the same
	// keystore type and same selection algorithm sign Scope 9
	// proposals as sign Scope 8 callbacks.
	t.Setenv(qfdecisions.CallbackSigningKeysEnvVar, `[{"key_id":"k-wp-int","secret":"sek-wp-int-2026","not_before":"2026-01-01T00:00:00Z"}]`)
	keystore, err := qfdecisions.LoadCallbackKeystoreFromEnv()
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromEnv: %v", err)
	}
	if keystore == nil {
		t.Fatal("LoadCallbackKeystoreFromEnv: want non-nil keystore, got nil")
	}

	nowFn := func() time.Time { return mustParseRFC3339(t, "2026-05-23T12:00:00Z") }
	signer := qfdecisions.NewWatchProposalSigner(keystore, nowFn)
	client := qfdecisions.NewClient(stub.URL(), "bridge-int-token", 1, 100)
	wpc := qfdecisions.NewWatchProposalClient(client, signer, 5*time.Minute, nowFn,
		func() (string, error) { return "01970000-0000-7000-8000-000000000131", nil })

	expiresAt := mustParseRFC3339(t, "2026-05-23T12:05:00Z")
	result, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", expiresAt)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if result.Status != qfdecisions.WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("result.Status: want %q, got %q", qfdecisions.WatchProposalStatusRejectedV1Deferred, result.Status)
	}
	if result.QFResponse.RejectionCode != qfdecisions.WatchProposalRejectionCodeDeferredV1 {
		t.Fatalf("RejectionCode: want %q, got %q", qfdecisions.WatchProposalRejectionCodeDeferredV1, result.QFResponse.RejectionCode)
	}
	if got := stub.Hits(); got != 1 {
		t.Fatalf("watch-proposal QF stub hits: want 1 (no retry), got %d", got)
	}
	bodies := stub.Bodies()
	if len(bodies) != 1 {
		t.Fatalf("stub bodies: want 1, got %d", len(bodies))
	}
	var posted qfdecisions.WatchProposalEnvelope
	if jerr := json.Unmarshal([]byte(bodies[0]), &posted); jerr != nil {
		t.Fatalf("unmarshal posted envelope: %v", jerr)
	}
	if posted.Signature == "" {
		t.Fatal("posted envelope Signature is empty")
	}
	if posted.KeyID != "k-wp-int" {
		t.Fatalf("posted envelope KeyID: want k-wp-int, got %q", posted.KeyID)
	}
	if len(posted.Signature) != 64 {
		t.Fatalf("posted envelope Signature length: want 64 hex chars, got %d (%q)", len(posted.Signature), posted.Signature)
	}
	if strings.ToLower(posted.Signature) != posted.Signature {
		t.Fatalf("posted envelope Signature is not lower-case hex: %q", posted.Signature)
	}
	if posted.Source != qfdecisions.WatchProposalSourceSmackerelPropose {
		t.Fatalf("posted envelope Source: want %q, got %q", qfdecisions.WatchProposalSourceSmackerelPropose, posted.Source)
	}
	// Scope 8 verbatim keystore reuse: independently select the
	// same key from the keystore and confirm the posted key_id
	// matches. This is the adversarial trip-wire that would catch
	// a regression that swapped the keystore implementation
	// pre-MVP.
	key, kerr := keystore.SelectActiveKey(nowFn())
	if kerr != nil {
		t.Fatalf("keystore.SelectActiveKey: %v", kerr)
	}
	if posted.KeyID != key.KeyID {
		t.Fatalf("Scope 8 keystore-reuse contract violated: posted key_id %q != keystore.SelectActiveKey key_id %q", posted.KeyID, key.KeyID)
	}
	// Metric assertions.
	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(qfdecisions.WatchProposalStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFWatchProposalAttemptsTotal{status=rejected_v1_deferred}: want 1, got %v", got)
	}
	if got := testutil.CollectAndCount(metrics.QFCallbackSignatureFailuresTotal); got != 0 {
		t.Fatalf("QFCallbackSignatureFailuresTotal collected count: want 0, got %d", got)
	}
}

// TestQFWatchProposalPreMVPRejectionParsedAndNoLocalWatchStateMutatedAcrossLiveStack
// (SCN-SM-041-033) is the adversarial guard that a Scope 9
// watch-proposal POST against the live disposable stack does NOT:
//
//   - mutate ANY local watch state — verified by snapshotting
//     `pg_stat_user_tables.n_tup_ins + n_tup_upd + n_tup_del` for
//     every `watch_*`, `proposal_*`, or `qf_*` table BEFORE the
//     Propose call and asserting the deltas are zero AFTER;
//   - retry the QF rejection — verified by stub hit count == 1;
//   - emit any user-visible "proposal submitted" affordance — no
//     web/digest/Telegram package is invoked (structural guarantee:
//     this test imports neither web nor render nor digest);
//   - trigger any Smackerel-side trade approval, mandate change,
//     watch creation/evaluation, EmergencyStop, or execution
//     behavior — verified by zero rows added to any forbidden
//     action table.
//
// Adversarial trip-wire: ANY new row in a watch_* / proposal_* /
// qf_* table after a single Propose call MUST fail this test.
func TestQFWatchProposalPreMVPRejectionParsedAndNoLocalWatchStateMutatedAcrossLiveStack(t *testing.T) {
	pool := testPool(t)
	_ = qfDecisionsNATSClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stub := newWatchProposalQFStub(t)
	defer stub.Close()
	stub.SetResponse(http.StatusServiceUnavailable, `{"code":"WATCH_PROPOSALS_DEFERRED_TO_V1","message":"pre-MVP"}`)

	metrics.QFCallbackAttemptsTotal.Reset()
	metrics.QFCallbackSignatureFailuresTotal.Reset()
	metrics.QFWatchProposalAttemptsTotal.Reset()

	t.Setenv(qfdecisions.CallbackSigningKeysEnvVar, `[{"key_id":"k-wp-int-2","secret":"sek-wp-int-2","not_before":"2026-01-01T00:00:00Z"}]`)
	keystore, err := qfdecisions.LoadCallbackKeystoreFromEnv()
	if err != nil {
		t.Fatalf("LoadCallbackKeystoreFromEnv: %v", err)
	}

	// Snapshot tuple-counter sums for every table whose name
	// matches the forbidden watch/proposal/qf prefixes. This is
	// the adversarial assertion: Scope 9 MUST NOT touch any of
	// these tables pre-MVP.
	before, beforeErr := snapshotForbiddenTableMutations(ctx, pool)
	if beforeErr != nil {
		t.Fatalf("snapshotForbiddenTableMutations(before): %v", beforeErr)
	}

	nowFn := func() time.Time { return mustParseRFC3339(t, "2026-05-23T12:00:00Z") }
	signer := qfdecisions.NewWatchProposalSigner(keystore, nowFn)
	client := qfdecisions.NewClient(stub.URL(), "bridge-int-token", 1, 100)
	wpc := qfdecisions.NewWatchProposalClient(client, signer, 5*time.Minute, nowFn,
		func() (string, error) { return "01970000-0000-7000-8000-000000000133", nil })

	expiresAt := mustParseRFC3339(t, "2026-05-23T12:05:00Z")
	result, err := wpc.Propose(ctx, "qf:security:NVDA", "attention_signal_over_threshold", expiresAt)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if result.Status != qfdecisions.WatchProposalStatusRejectedV1Deferred {
		t.Fatalf("Status: want %q, got %q", qfdecisions.WatchProposalStatusRejectedV1Deferred, result.Status)
	}
	if got := stub.Hits(); got != 1 {
		t.Fatalf("watch-proposal QF stub hits: want 1 (no retry), got %d", got)
	}

	after, afterErr := snapshotForbiddenTableMutations(ctx, pool)
	if afterErr != nil {
		t.Fatalf("snapshotForbiddenTableMutations(after): %v", afterErr)
	}
	// Compute deltas per matched table; any non-zero delta is a
	// boundary violation.
	for tbl, beforeCount := range before {
		afterCount := after[tbl]
		if afterCount != beforeCount {
			t.Errorf("Scope 9 boundary violation: table %s tuple mutations went from %d to %d (delta=%d). Pre-MVP MUST NOT mutate ANY watch/proposal/qf table.",
				tbl, beforeCount, afterCount, afterCount-beforeCount)
		}
	}
	// Catch tables that were created during the test (would show up
	// in `after` but not `before`).
	for tbl, afterCount := range after {
		if _, ok := before[tbl]; !ok {
			t.Errorf("Scope 9 boundary violation: NEW table %s appeared after Propose call with %d tuple mutations. Pre-MVP MUST NOT mutate ANY watch/proposal/qf table.", tbl, afterCount)
		}
	}

	if got := testutil.ToFloat64(metrics.QFWatchProposalAttemptsTotal.WithLabelValues(qfdecisions.WatchProposalStatusRejectedV1Deferred)); got != 1 {
		t.Fatalf("QFWatchProposalAttemptsTotal{status=rejected_v1_deferred}: want 1, got %v", got)
	}
}

// snapshotForbiddenTableMutations queries pg_stat_user_tables for
// every table whose relname matches a forbidden Scope 9 prefix
// (watch_, proposal_, qf_) and returns a map of relname →
// (n_tup_ins + n_tup_upd + n_tup_del). The map MUST be byte-equal
// before and after a Scope 9 Propose call pre-MVP.
//
// Note: pg_stat_user_tables counters are per-table cumulative since
// last reset of statistics. Snapshotting before/after a single
// Propose call captures the delta even if other parallel test runs
// have driven the counters higher in the past.
func snapshotForbiddenTableMutations(ctx context.Context, pool *pgxpool.Pool) (map[string]int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT relname,
		       coalesce(n_tup_ins, 0) + coalesce(n_tup_upd, 0) + coalesce(n_tup_del, 0) AS mutations
		FROM pg_stat_user_tables
		WHERE relname LIKE 'watch_%'
		   OR relname LIKE 'proposal_%'
		   OR relname LIKE 'qf_%'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int64)
	for rows.Next() {
		var relname string
		var mutations int64
		if scanErr := rows.Scan(&relname, &mutations); scanErr != nil {
			return nil, scanErr
		}
		out[relname] = mutations
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, rerr
	}
	return out, nil
}

// watchProposalQFStub is a per-test QF Companion Bridge stub for the
// /api/private/smackerel/v1/watch-signal-proposals endpoint. Mirrors
// the Scope 8 callbackQFStub structure so the integration boundary
// assertions are symmetric. Records every request body and counts
// every request so the integration tests can assert "no retry" and
// "no network reached" guarantees.
type watchProposalQFStub struct {
	t      *testing.T
	server *httptest.Server
	hits   int64
	mu     sync.Mutex
	bodies []string
	status int
	body   string
}

func newWatchProposalQFStub(t *testing.T) *watchProposalQFStub {
	t.Helper()
	stub := &watchProposalQFStub{t: t, status: http.StatusOK, body: `{}`}
	stub.server = httptest.NewServer(http.HandlerFunc(stub.handle))
	return stub
}

func (s *watchProposalQFStub) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != qfdecisions.WatchProposalPath {
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

func (s *watchProposalQFStub) URL() string { return s.server.URL }
func (s *watchProposalQFStub) Close()      { s.server.Close() }
func (s *watchProposalQFStub) Hits() int64 { return atomic.LoadInt64(&s.hits) }
func (s *watchProposalQFStub) Bodies() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.bodies))
	copy(out, s.bodies)
	return out
}

func (s *watchProposalQFStub) SetResponse(status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	s.body = body
}
