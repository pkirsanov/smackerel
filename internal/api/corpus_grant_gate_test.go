// Spec 108 Scope 02 — unit coverage for the OBSERVE half of the corpus-grant
// stage machine (`internal/api/corpus_grant_gate.go`).
//
// The claim this scope makes is narrow and easy to fake, so the tests are
// built around the two ways it could be faked:
//
//   - A gate that DENIED would still make the would-deny counter move. A
//     counter-only assertion would pass while the OBSERVE stage silently broke
//     production for every ungranted principal — the exact opposite of what
//     OBSERVE exists to do.
//   - A gate that never EVALUATED would still let every request through. A
//     status-only assertion would pass while the observation window stayed
//     silent, and the operator would flip to ENFORCE against a counter that was
//     structurally incapable of moving.
//
// TestCorpusGrantGate_Observe_UngrantedSessionIsCountedButNeverDenied asserts
// BOTH halves in one test for that reason. Neither half alone is the claim.
package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

// corpusGrantDownstreamBody is the body only the downstream handler writes. If
// it survives to the recorder, the middleware neither denied nor rewrote the
// response.
const corpusGrantDownstreamBody = `{"results":[]}`

// corpusGrantCounters snapshots the three counters a single observed request
// can touch, so every assertion below can be expressed as an exact delta
// rather than an absolute value (the registry is global and shared).
type corpusGrantCounters struct {
	wouldDeny     float64
	allowed       float64
	scopeRejected float64
}

func corpusGrantSnapshot(group metrics.CorpusRouteGroup, userID, sessionSource string) corpusGrantCounters {
	return corpusGrantCounters{
		wouldDeny:     testutil.ToFloat64(metrics.AuthCorpusGrantWouldDeny.WithLabelValues(string(group), userID, sessionSource)),
		allowed:       testutil.ToFloat64(metrics.AuthCorpusGrantAllowed.WithLabelValues(string(group), userID, sessionSource)),
		scopeRejected: testutil.ToFloat64(metrics.AuthScopeRejected.WithLabelValues(auth.GrantGlobalCorpusRead, userID)),
	}
}

// corpusGrantSentinelHandler records whether the downstream handler actually
// ran and writes a distinctive status and body, so "the request was not
// denied" is proved by the DOWNSTREAM response rather than by the absence of
// a 403 (a middleware that swallowed the request without writing anything
// would also produce "not 403").
type corpusGrantSentinelHandler struct {
	ran    bool
	status int
	body   string
}

func (h *corpusGrantSentinelHandler) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h.ran = true
		w.WriteHeader(h.status)
		_, _ = w.Write([]byte(h.body))
	})
}

// corpusGrantRequest builds a request carrying the given session. The path and
// query are deliberately sensitive-looking so the cardinality and log-hygiene
// assertions have real material to catch a leak with.
func corpusGrantRequest(sess auth.Session, target string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	return req.WithContext(auth.WithSession(req.Context(), sess))
}

// corpusGrantRunObserve drives one request through the gate and returns the
// recorder plus the sentinel, so callers can assert on both the response and
// whether the downstream handler ran.
func corpusGrantRunObserve(gate *CorpusGrantGate, group metrics.CorpusRouteGroup, req *http.Request, status int, body string) (*httptest.ResponseRecorder, *corpusGrantSentinelHandler) {
	sentinel := &corpusGrantSentinelHandler{status: status, body: body}
	rec := httptest.NewRecorder()
	gate.Observe(group)(sentinel.handler()).ServeHTTP(rec, req)
	return rec, sentinel
}

// corpusGrantObservation is everything one request through a corpus middleware
// reveals: what the caller saw, and what the counters did.
type corpusGrantObservation struct {
	ran                bool
	status             int
	body               string
	wouldDenyDelta     float64
	allowedDelta       float64
	scopeRejectedDelta float64
}

// corpusGrantObserveThrough drives one request through ANY middleware — the
// real gate or a deliberately-broken stand-in — and reports the observation.
// Taking the middleware as a parameter is what lets the mutation control below
// exercise the SAME assertion code the real test uses, so the two can never
// drift apart.
func corpusGrantObserveThrough(mw func(http.Handler) http.Handler, group metrics.CorpusRouteGroup, sess auth.Session, target string) corpusGrantObservation {
	before := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))

	sentinel := &corpusGrantSentinelHandler{status: http.StatusOK, body: corpusGrantDownstreamBody}
	rec := httptest.NewRecorder()
	mw(sentinel.handler()).ServeHTTP(rec, corpusGrantRequest(sess, target))

	after := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
	return corpusGrantObservation{
		ran:                sentinel.ran,
		status:             rec.Code,
		body:               rec.Body.String(),
		wouldDenyDelta:     after.wouldDeny - before.wouldDeny,
		allowedDelta:       after.allowed - before.allowed,
		scopeRejectedDelta: after.scopeRejected - before.scopeRejected,
	}
}

// Violation prefixes, so the mutation control can assert WHICH half fired
// rather than merely that something did.
const (
	corpusGrantViolationDenied    = "NON-DENIAL VIOLATION"
	corpusGrantViolationUncounted = "OBSERVATION VIOLATION"
)

// corpusGrantCountedNotDeniedViolations is the paired assertion at the heart of
// SCN-108-O01: an ungranted request must be COUNTED and must NOT be DENIED.
// It returns violations instead of failing directly so the same code can be
// used twice — once to judge the real gate, and once to prove it rejects a
// broken one.
func corpusGrantCountedNotDeniedViolations(obs corpusGrantObservation) []string {
	var violations []string

	// Half 1 — the gate must never deny.
	if !obs.ran {
		violations = append(violations, corpusGrantViolationDenied+": the downstream handler did not run, so Observe short-circuited an ungranted request")
	}
	if obs.status == http.StatusForbidden {
		violations = append(violations, corpusGrantViolationDenied+": Observe returned 403; denial belongs to auth.RequireScope under ENFORCE (Scope 03), never to this gate")
	} else if obs.status != http.StatusOK {
		violations = append(violations, fmt.Sprintf("%s: status = %d, want %d (the downstream handler's own status, passed through unchanged)", corpusGrantViolationDenied, obs.status, http.StatusOK))
	}
	if obs.body != corpusGrantDownstreamBody {
		violations = append(violations, fmt.Sprintf("%s: body = %q, want the downstream handler's own body; Observe must not rewrite the response", corpusGrantViolationDenied, obs.body))
	}

	// Half 2 — the gate must still observe.
	if obs.wouldDenyDelta != 1 {
		violations = append(violations, fmt.Sprintf("%s: smackerel_auth_corpus_grant_would_deny_total delta = %v, want 1; the ungranted request was admitted without being counted, so the observation window is silent", corpusGrantViolationUncounted, obs.wouldDenyDelta))
	}
	if obs.allowedDelta != 0 {
		violations = append(violations, fmt.Sprintf("%s: smackerel_auth_corpus_grant_allowed_total delta = %v, want 0; an ungranted request must never be counted as allowed", corpusGrantViolationUncounted, obs.allowedDelta))
	}
	if obs.scopeRejectedDelta != 0 {
		violations = append(violations, fmt.Sprintf("%s: smackerel_auth_scope_rejected_total delta = %v, want 0; R-108-O2 reserves it for real ENFORCE denials", corpusGrantViolationUncounted, obs.scopeRejectedDelta))
	}

	return violations
}

// TestCorpusGrantGate_Observe_UngrantedSessionIsCountedButNeverDenied is the
// adversarial non-denial proof for SCN-108-O01 and the load-bearing test of
// this scope.
//
// A principal WITHOUT corpus:read passes through the gate. The assertions are
// paired on purpose:
//
//	non-denial   — the downstream handler ran, the status is the downstream's
//	               own status (unchanged, and specifically not 403), and the
//	               body is the downstream's own body.
//	observation  — smackerel_auth_corpus_grant_would_deny_total incremented by
//	               exactly one for that route_group / user_id / session_source.
//
// Drop either half and the test stops proving the scope's claim: a silent
// denial passes a counter-only check, and an unwired gate passes a status-only
// check.
func TestCorpusGrantGate_Observe_UngrantedSessionIsCountedButNeverDenied(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupSearch
	sess := auth.SessionWithRole("tp0204-ungranted", "jti-ungranted", auth.RoleDailyUser)

	// Precondition: this principal genuinely lacks the grant. Without it the
	// test could pass against a session that was allowed all along.
	if auth.GateGlobalCorpusRead(sess).Allowed {
		t.Fatalf("fixture drift: %q holds %s, so this test cannot prove the ungranted path", sess.UserID, auth.GrantGlobalCorpusRead)
	}

	obs := corpusGrantObserveThrough(gate.Observe(group), group, sess, "/api/search?q=my+private+medical+question")

	if violations := corpusGrantCountedNotDeniedViolations(obs); len(violations) > 0 {
		t.Fatalf("an ungranted request through route_group=%q (user_id=%q, session_source=%q) broke the OBSERVE contract:\n  - %s",
			group, sess.UserID, sess.Source, strings.Join(violations, "\n  - "))
	}
}

// TestCorpusGrantGate_PairedAssertionRejectsBothFailureModes is the mutation
// control for the test above. It proves the paired assertion is not vacuous by
// running the SAME function against two middlewares that are each broken in
// exactly one of the two ways that matter.
//
// This exists because the paired assertion is the whole scope's claim, and a
// passing assertion is only meaningful if it is capable of failing. Both
// stand-ins deliberately get the OTHER half right, so each case isolates one
// half:
//
//	silently_denies    — counts correctly, then denies. Only a status/ran
//	                     assertion catches it. Without half 1, OBSERVE could
//	                     403 every ungranted principal and still look green.
//	admits_but_silent  — admits correctly, but records nothing. Only a counter
//	                     assertion catches it. Without half 2, the gate could
//	                     be entirely unwired and still look green.
//
// The stand-ins call the same shared assertion the real test calls, so the
// control cannot drift away from what it is controlling.
func TestCorpusGrantGate_PairedAssertionRejectsBothFailureModes(t *testing.T) {
	group := metrics.CorpusRouteGroupContentFuel
	sess := auth.SessionWithRole("tp0204-mutation-control", "jti-mutation", auth.RoleDailyUser)

	// Broken stand-in 1: observes correctly, then denies.
	silentlyDenies := func(_ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, _ := auth.SessionFromContext(r.Context())
			_ = metrics.RecordCorpusGrantWouldDeny(group, s.UserID, string(s.Source))
			w.WriteHeader(http.StatusForbidden)
		})
	}

	// Broken stand-in 2: admits correctly, but records nothing.
	admitsButSilent := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	for _, tc := range []struct {
		name         string
		mw           func(http.Handler) http.Handler
		wantViolated string
		wantClean    string
	}{
		{"silently_denies", silentlyDenies, corpusGrantViolationDenied, corpusGrantViolationUncounted},
		{"admits_but_silent", admitsButSilent, corpusGrantViolationUncounted, corpusGrantViolationDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs := corpusGrantObserveThrough(tc.mw, group, sess, "/api/content-fuel")
			got := corpusGrantCountedNotDeniedViolations(obs)
			if len(got) == 0 {
				t.Fatalf("the paired assertion ACCEPTED a gate that %s; it is vacuous and proves nothing about SCN-108-O01", tc.name)
			}

			joined := strings.Join(got, "\n  - ")
			if !strings.Contains(joined, tc.wantViolated) {
				t.Fatalf("expected a %s for %s, got:\n  - %s", tc.wantViolated, tc.name, joined)
			}
			if strings.Contains(joined, tc.wantClean) {
				t.Fatalf("%s should have satisfied the %s half, but it was reported too:\n  - %s", tc.name, tc.wantClean, joined)
			}
		})
	}
}

// TestCorpusGrantGate_Observe_PassesDownstreamStatusThroughUnchanged closes the
// remaining escape from the test above: a gate that always forced 200 would
// satisfy a "status == 200" assertion while destroying every real error
// response. The downstream statuses here are chosen to be neither 200 nor 403.
func TestCorpusGrantGate_Observe_PassesDownstreamStatusThroughUnchanged(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupRecent
	sess := auth.SessionWithRole("tp0204-passthrough", "jti-passthrough", auth.RoleDailyUser)

	for _, status := range []int{http.StatusCreated, http.StatusNotFound, http.StatusInternalServerError} {
		req := corpusGrantRequest(sess, "/api/recent")
		rec, sentinel := corpusGrantRunObserve(gate, group, req, status, "downstream")
		if !sentinel.ran {
			t.Fatalf("downstream handler did not run for status %d", status)
		}
		if rec.Code != status {
			t.Errorf("status = %d, want %d — Observe must not rewrite the downstream status", rec.Code, status)
		}
		if got := rec.Body.String(); got != "downstream" {
			t.Errorf("body = %q, want %q", got, "downstream")
		}
	}
}

// TestCorpusGrantGate_Observe_GrantedSessionCountsAllowed is SCN-108-O02: a
// principal WITH corpus:read passes through and lands on the allowed counter,
// leaving the would-deny counter flat. The would-deny assertion is what stops
// a gate that counted every request as a would-be denial from passing.
func TestCorpusGrantGate_Observe_GrantedSessionCountsAllowed(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupDigest
	sess := auth.SessionWithRole("tp0205-granted", "jti-granted", auth.RoleDailyUser, auth.GrantGlobalCorpusRead)

	if !auth.GateGlobalCorpusRead(sess).Allowed {
		t.Fatalf("fixture drift: %q does not hold %s, so this test cannot prove the granted path", sess.UserID, auth.GrantGlobalCorpusRead)
	}

	before := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
	rec, sentinel := corpusGrantRunObserve(gate, group, corpusGrantRequest(sess, "/api/digest"), http.StatusOK, "ok")
	after := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))

	if !sentinel.ran || rec.Code != http.StatusOK {
		t.Fatalf("granted request: ran=%v status=%d, want ran=true status=200", sentinel.ran, rec.Code)
	}
	if delta := after.allowed - before.allowed; delta != 1 {
		t.Errorf("smackerel_auth_corpus_grant_allowed_total delta = %v, want 1", delta)
	}
	if delta := after.wouldDeny - before.wouldDeny; delta != 0 {
		t.Errorf("smackerel_auth_corpus_grant_would_deny_total delta = %v, want 0 — a granted request is not a counterfactual denial", delta)
	}
	if delta := after.scopeRejected - before.scopeRejected; delta != 0 {
		t.Errorf("smackerel_auth_scope_rejected_total delta = %v, want 0 (R-108-O2)", delta)
	}
}

// TestCorpusGrantGate_Observe_ConsultsGateGlobalCorpusRead proves the gate
// really calls auth.GateGlobalCorpusRead rather than re-deciding locally.
// `Observe` is that function's first production caller, so "is it actually
// consulted?" is a live question, not a formality.
//
// The discriminating case is `{"*", "corpus:read"}`. A local re-implementation
// written as `slices.Contains(sess.Scopes, "corpus:read")` — the obvious
// shortcut — reports ALLOWED for it. auth.GateGlobalCorpusRead reports DENIED,
// because a wildcard sentinel must never widen authority. The two answers
// differ, so this case can only come out right if the real gate function
// produced it. `{"*"}` alone covers the mirror-image shortcut that treats a
// wildcard as universal grant.
//
// Every case additionally asserts the recorded outcome equals
// auth.GateGlobalCorpusRead's own answer, so the test tracks the gate's
// definition instead of restating it.
func TestCorpusGrantGate_Observe_ConsultsGateGlobalCorpusRead(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupKnowledge

	cases := []struct {
		name string
		sess auth.Session
		why  string
	}{
		{
			name: "ungranted_daily_user",
			sess: auth.SessionWithRole("consult-daily", "jti-1", auth.RoleDailyUser),
			why:  "no corpus:read in the persisted grant snapshot",
		},
		{
			name: "granted_daily_user",
			sess: auth.SessionWithRole("consult-granted", "jti-2", auth.RoleDailyUser, auth.GrantGlobalCorpusRead),
			why:  "explicitly granted corpus:read",
		},
		{
			name: "operator",
			sess: auth.SessionWithRole("consult-operator", "jti-3", auth.RoleOperator),
			why:  "operator role carries corpus:read",
		},
		{
			name: "bare_session_no_scopes",
			sess: auth.Session{UserID: "consult-bare", TokenID: "jti-4", Source: auth.SessionSourcePerUserToken},
			why:  "a bare valid session implies no grant",
		},
		{
			name: "wildcard_only",
			sess: auth.Session{UserID: "consult-wildcard", TokenID: "jti-5", Source: auth.SessionSourcePerUserToken, Scopes: []string{"*"}},
			why:  "a wildcard is never honored; a shortcut that treated it as universal grant would allow here",
		},
		{
			name: "wildcard_plus_corpus_read",
			sess: auth.Session{UserID: "consult-wildcard-corpus", TokenID: "jti-6", Source: auth.SessionSourcePerUserToken, Scopes: []string{"*", auth.GrantGlobalCorpusRead}},
			why:  "DISCRIMINATOR: a local slices.Contains(scopes, \"corpus:read\") shortcut allows, auth.GateGlobalCorpusRead denies because the wildcard poisons the snapshot",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := auth.GateGlobalCorpusRead(tc.sess).Allowed

			before := corpusGrantSnapshot(group, tc.sess.UserID, string(tc.sess.Source))
			rec, sentinel := corpusGrantRunObserve(gate, group, corpusGrantRequest(tc.sess, "/api/knowledge"), http.StatusOK, "ok")
			after := corpusGrantSnapshot(group, tc.sess.UserID, string(tc.sess.Source))

			if !sentinel.ran || rec.Code != http.StatusOK {
				t.Fatalf("%s: ran=%v status=%d, want ran=true status=200 (Observe never denies, whatever the decision)", tc.name, sentinel.ran, rec.Code)
			}

			allowedDelta := after.allowed - before.allowed
			denyDelta := after.wouldDeny - before.wouldDeny

			if want {
				if allowedDelta != 1 || denyDelta != 0 {
					t.Fatalf("%s (%s): auth.GateGlobalCorpusRead says ALLOWED, but the gate recorded allowed=%v would_deny=%v — the gate did not use that decision",
						tc.name, tc.why, allowedDelta, denyDelta)
				}
				return
			}
			if denyDelta != 1 || allowedDelta != 0 {
				t.Fatalf("%s (%s): auth.GateGlobalCorpusRead says DENIED, but the gate recorded allowed=%v would_deny=%v — the gate did not use that decision",
					tc.name, tc.why, allowedDelta, denyDelta)
			}
		})
	}
}

// TestCorpusGrantGate_Observe_AcceptsEveryClosedSetRouteGroup proves all
// sixteen groups — Tier A and Tier B alike — are mountable, so a gate that
// silently rejected Tier B (the set §18 decision 5 added) cannot ship.
func TestCorpusGrantGate_Observe_AcceptsEveryClosedSetRouteGroup(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	sess := auth.SessionWithRole("tp0204-allgroups", "jti-allgroups", auth.RoleDailyUser)

	groups := metrics.CorpusRouteGroups()
	if len(groups) != 16 {
		t.Fatalf("metrics.CorpusRouteGroups() returned %d groups, want 16", len(groups))
	}

	for _, group := range groups {
		before := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
		rec, sentinel := corpusGrantRunObserve(gate, group, corpusGrantRequest(sess, "/api/"+string(group)), http.StatusOK, "ok")
		after := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))

		if !sentinel.ran || rec.Code != http.StatusOK {
			t.Errorf("route_group=%q: ran=%v status=%d, want ran=true status=200", group, sentinel.ran, rec.Code)
		}
		if delta := after.wouldDeny - before.wouldDeny; delta != 1 {
			t.Errorf("route_group=%q: would_deny delta = %v, want 1", group, delta)
		}
	}
}

// TestCorpusGrantGate_Observe_PanicsOnOutOfSetRouteGroup proves the closed set
// is enforced at MOUNT time and that an out-of-set value emits nothing.
//
// Rejecting at construction rather than per request is what makes an unbounded
// label structurally impossible: a route group is a wiring-time constant, so a
// caller-supplied string can never become one. A gate that instead recorded
// the forged value (or silently skipped it) would leave the door open.
func TestCorpusGrantGate_Observe_PanicsOnOutOfSetRouteGroup(t *testing.T) {
	gate := NewCorpusGrantGate(false)

	forged := []metrics.CorpusRouteGroup{
		"",
		"/api/search",
		"/api/artifact/9f3c2a11-4d5e-4f60-9a7b-1c2d3e4f5a6b",
		"SEARCH",
		"not_a_group",
	}

	for _, group := range forged {
		t.Run("group="+string(group), func(t *testing.T) {
			func() {
				defer func() {
					r := recover()
					if r == nil {
						t.Fatalf("Observe(%q) did not panic; an out-of-set route group must be refused at mount time (R-108-O3/O4)", group)
					}
					msg, ok := r.(string)
					if !ok || !strings.Contains(msg, metrics.ErrUnknownCorpusRouteGroup.Error()) {
						t.Fatalf("Observe(%q) panicked with %v, want a message naming the closed-set violation", group, r)
					}
				}()
				_ = gate.Observe(group)
			}()

			// Nothing may have been emitted under the forged value.
			for _, metricName := range []string{
				"smackerel_auth_corpus_grant_would_deny_total",
				"smackerel_auth_corpus_grant_allowed_total",
			} {
				for _, seen := range corpusGrantEmittedRouteGroups(t, metricName) {
					if seen == string(group) {
						t.Fatalf("%s minted a series with the out-of-set route_group %q", metricName, group)
					}
				}
			}
		})
	}
}

// TestCorpusGrantGate_Observe_RequestPathNeverBecomesALabelValue is the
// cardinality proof stated the way the failure would actually appear: many
// distinct request paths flowing through ONE mounted route group must produce
// exactly ONE series, not one per path.
//
// `/api/artifact/{id}` is the concrete shape R-108-O3/O4 calls out — a
// per-artifact label would mint a series per artifact.
func TestCorpusGrantGate_Observe_RequestPathNeverBecomesALabelValue(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupArtifactDetail
	sess := auth.SessionWithRole("tp0204-cardinality", "jti-cardinality", auth.RoleDailyUser)

	before := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
	// Counted AFTER the snapshot: reading a counter child materialises it, so
	// taking the baseline here makes the expected growth exactly zero rather
	// than "zero plus whatever the baseline read created".
	beforeSeries := corpusGrantSeriesCount(t, "smackerel_auth_corpus_grant_would_deny_total")

	paths := []string{
		"/api/artifact/11111111-1111-4111-8111-111111111111",
		"/api/artifact/22222222-2222-4222-8222-222222222222",
		"/api/artifact/33333333-3333-4333-8333-333333333333",
		"/api/artifact/44444444-4444-4444-8444-444444444444",
		"/api/artifact/55555555-5555-4555-8555-555555555555",
		"/api/search?q=deeply+personal+search+text",
	}
	for _, path := range paths {
		if _, sentinel := corpusGrantRunObserve(gate, group, corpusGrantRequest(sess, path), http.StatusOK, "ok"); !sentinel.ran {
			t.Fatalf("downstream handler did not run for %q", path)
		}
	}

	after := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
	if delta := after.wouldDeny - before.wouldDeny; delta != float64(len(paths)) {
		t.Fatalf("would_deny delta = %v, want %d (one per request)", delta, len(paths))
	}

	afterSeries := corpusGrantSeriesCount(t, "smackerel_auth_corpus_grant_would_deny_total")
	if grew := afterSeries - beforeSeries; grew != 0 {
		t.Fatalf("%d requests across %d distinct paths created %d new series; one mounted route group + one principal must create none (unbounded-cardinality regression)",
			len(paths), len(paths), grew)
	}

	for _, seen := range corpusGrantEmittedRouteGroups(t, "smackerel_auth_corpus_grant_would_deny_total") {
		if strings.ContainsAny(seen, "/?&=") {
			t.Fatalf("route_group=%q looks like a raw request path; raw paths are never label values (R-108-O3/O4)", seen)
		}
	}
}

// TestCorpusGrantGate_Observe_BypassedSessionSourcesEmitNothing mirrors the
// documented auth.RequireScope source switch. Shared-token and bootstrap
// sessions carry no scope claim and are BYPASSED under ENFORCE, so counting
// them as would-be denials would make the counter a false predictor of the
// ENFORCE outcome and inflate the UC-108-001 grant list with principals that
// will never be denied.
func TestCorpusGrantGate_Observe_BypassedSessionSourcesEmitNothing(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupExport

	for _, source := range []auth.SessionSource{auth.SessionSourceSharedToken, auth.SessionSourceBootstrap} {
		t.Run(string(source), func(t *testing.T) {
			sess := auth.Session{UserID: "bypass-" + string(source), Source: source}
			before := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
			rec, sentinel := corpusGrantRunObserve(gate, group, corpusGrantRequest(sess, "/api/export"), http.StatusOK, "ok")
			after := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))

			if !sentinel.ran || rec.Code != http.StatusOK {
				t.Fatalf("bypassed source %q: ran=%v status=%d, want ran=true status=200", source, sentinel.ran, rec.Code)
			}
			if delta := after.wouldDeny - before.wouldDeny; delta != 0 {
				t.Errorf("source %q: would_deny delta = %v, want 0 — RequireScope bypasses this source under ENFORCE, so it is not a would-be denial", source, delta)
			}
			if delta := after.allowed - before.allowed; delta != 0 {
				t.Errorf("source %q: allowed delta = %v, want 0", source, delta)
			}
		})
	}
}

// TestCorpusGrantGate_Observe_MissingSessionEmitsNothing covers the wiring
// defect path: bearerAuthMiddleware must populate the session before this gate
// runs. Recording a would-be denial without a session would invent a principal
// that does not exist and poison the UC-108-001 grant list, so nothing is
// emitted — but the request is still never denied.
func TestCorpusGrantGate_Observe_MissingSessionEmitsNothing(t *testing.T) {
	gate := NewCorpusGrantGate(false)
	group := metrics.CorpusRouteGroupContextFor

	beforeSeries := corpusGrantSeriesCount(t, "smackerel_auth_corpus_grant_would_deny_total")

	req := httptest.NewRequest(http.MethodGet, "/api/context-for", nil) // no session in context
	sentinel := &corpusGrantSentinelHandler{status: http.StatusOK, body: "ok"}
	rec := httptest.NewRecorder()
	gate.Observe(group)(sentinel.handler()).ServeHTTP(rec, req)

	if !sentinel.ran {
		t.Fatalf("downstream handler did not run; a missing session is a wiring defect, not a reason to deny")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if grew := corpusGrantSeriesCount(t, "smackerel_auth_corpus_grant_would_deny_total") - beforeSeries; grew != 0 {
		t.Fatalf("a sessionless request created %d new would-deny series; it must invent no principal", grew)
	}
}

// TestNewCorpusGrantGate_ModeReportsResolvedStage pins the two lowercase
// enforcement_mode log-field values. The stage is telemetry only — the
// following test proves it cannot change the admission decision.
func TestNewCorpusGrantGate_ModeReportsResolvedStage(t *testing.T) {
	if got := NewCorpusGrantGate(false).Mode(); got != "observe" {
		t.Errorf("NewCorpusGrantGate(false).Mode() = %q, want %q", got, "observe")
	}
	if got := NewCorpusGrantGate(true).Mode(); got != "enforce" {
		t.Errorf("NewCorpusGrantGate(true).Mode() = %q, want %q", got, "enforce")
	}
}

// TestCorpusGrantGate_Observe_NeverDeniesInEitherStage proves the gate is
// stage-independent by construction: an ENFORCE-constructed gate admits an
// ungranted principal exactly as an OBSERVE-constructed one does, and counts
// it the same way.
//
// This matters because the gate is mounted in BOTH stages. Denial under
// ENFORCE belongs to auth.RequireScope (Scope 03); if this gate ever gained a
// denial branch, the two would double-deny and the counterfactual counter
// would stop being a counterfactual.
func TestCorpusGrantGate_Observe_NeverDeniesInEitherStage(t *testing.T) {
	group := metrics.CorpusRouteGroupSubscriptions

	for _, tc := range []struct {
		name    string
		enforce bool
	}{
		{"observe_stage", false},
		{"enforce_stage", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := NewCorpusGrantGate(tc.enforce)
			sess := auth.SessionWithRole("stage-"+tc.name, "jti-"+tc.name, auth.RoleDailyUser)

			before := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))
			rec, sentinel := corpusGrantRunObserve(gate, group, corpusGrantRequest(sess, "/api/subscriptions"), http.StatusOK, "ok")
			after := corpusGrantSnapshot(group, sess.UserID, string(sess.Source))

			if !sentinel.ran {
				t.Fatalf("%s: downstream handler did not run — the gate denied, which it must never do in either stage", tc.name)
			}
			if rec.Code == http.StatusForbidden {
				t.Fatalf("%s: gate returned 403; denial belongs to auth.RequireScope (Scope 03), not this gate", tc.name)
			}
			if delta := after.wouldDeny - before.wouldDeny; delta != 1 {
				t.Fatalf("%s: would_deny delta = %v, want 1 — the gate must observe identically in both stages", tc.name, delta)
			}
		})
	}
}

// corpusGrantEmittedRouteGroups reads every `route_group` label value present
// on the named metric family from the real gatherer.
func corpusGrantEmittedRouteGroups(t *testing.T, metricName string) []string {
	t.Helper()
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var values []string
	for _, mf := range gathered {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "route_group" {
					values = append(values, lp.GetValue())
				}
			}
		}
	}
	return values
}

// corpusGrantSeriesCount counts the distinct label combinations (series)
// currently registered for the named metric family.
func corpusGrantSeriesCount(t *testing.T, metricName string) int {
	t.Helper()
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range gathered {
		if mf.GetName() == metricName {
			return len(mf.GetMetric())
		}
	}
	return 0
}
