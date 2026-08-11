//go:build integration

// Spec 108 Scope 02 — TP-02-04 and TP-02-05.
//
// These run in the `./smackerel.sh test integration` lane against the
// ephemeral test stack (`./tests/integration/...` is one of that lane's
// package targets; a test placed in `internal/api` with an integration build
// tag would compile but never execute, because the lane does not target
// `./internal/...`).
//
// SCOPE OF THE CLAIM — stated plainly so the evidence is not read as more than
// it is. These tests drive the REAL `api.CorpusGrantGate` middleware, the REAL
// `auth.GateGlobalCorpusRead` decision, and the REAL Prometheus registry, over
// all sixteen closed-set route groups. They do NOT drive HTTP routes on the
// running core service, because Scope 02 does not mount the gate: as of this
// scope `api.NewCorpusGrantGate` has no production caller, and mounting it on
// `router.go` is Scope 03. Asserting a live route here would be asserting
// Scope 03's work; asserting it and passing would mean the assertion was
// vacuous. The live-route half of SCN-108-O01/O02 belongs to TP-02-06 once the
// mount exists.
//
// What they add over the unit tests in `internal/api` is the log-hygiene
// assertion — the warn line must answer WHO would have been denied and must
// never carry WHAT they asked for — and the full sixteen-group sweep in one
// pass.
package integration

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/metrics"
)

// corpusGrantObserveSecrets are the strings that MUST NOT appear in the warn
// log. Each one is a real leak class named by design.md §4: the user's query
// text, the artifact they asked for, and the shape of the answer.
const (
	corpusGrantSecretQuery      = "my-private-medical-question-do-not-log"
	corpusGrantSecretArtifactID = "9f3c2a11-4d5e-4f60-9a7b-1c2d3e4f5a6b" // gitleaks:allow — synthetic fixture UUID; must keep real-ID shape to work as a leak canary
	corpusGrantSecretTitle      = "Confidential-Q3-Compensation-Review"
)

// corpusGrantObserveDownstream is the handler mounted behind the gate. It
// records whether it ran, which is how "the request was not denied" is proved
// from the downstream side rather than from the mere absence of a 403.
type corpusGrantObserveDownstream struct {
	ran int
}

func (d *corpusGrantObserveDownstream) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		d.ran++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
}

// corpusGrantObserveCaptureLogs redirects the default slog logger into a
// buffer for the duration of the test and restores it afterwards, following
// the convention already used in tests/integration/qf_audit_envelope_test.go.
func corpusGrantObserveCaptureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// corpusGrantObserveSensitiveTarget builds a request URL that carries every
// secret class at once, so the log-hygiene assertion has real material.
func corpusGrantObserveSensitiveTarget(group metrics.CorpusRouteGroup) string {
	return "/api/" + string(group) + "/" + corpusGrantSecretArtifactID +
		"?q=" + corpusGrantSecretQuery + "&title=" + corpusGrantSecretTitle
}

func corpusGrantObserveWouldDeny(group metrics.CorpusRouteGroup, userID, source string) float64 {
	return testutil.ToFloat64(metrics.AuthCorpusGrantWouldDeny.WithLabelValues(string(group), userID, source))
}

func corpusGrantObserveAllowed(group metrics.CorpusRouteGroup, source string) float64 {
	return testutil.ToFloat64(metrics.AuthCorpusGrantAllowed.WithLabelValues(string(group), source))
}

// TestIntegration_CorpusGrantObserve_UngrantedPrincipalIsCountedOnAllSixteenGroups
// is TP-02-04 / SCN-108-O01.
//
// For each of the sixteen closed-set route groups an ungranted principal is
// driven through the gate and three things are asserted together:
//
//	200 not 403   — the downstream handler ran and its own status survived.
//	counted       — the would-deny counter moved by exactly one for THAT group.
//	log is clean  — the warn line names the principal but carries no query
//	                text, artifact id, or title.
//
// The per-group counter assertion is what stops a gate that recorded every
// request under a single hardcoded group from passing a sweep like this.
func TestIntegration_CorpusGrantObserve_UngrantedPrincipalIsCountedOnAllSixteenGroups(t *testing.T) {
	logs := corpusGrantObserveCaptureLogs(t)
	gate := api.NewCorpusGrantGate(false)

	sess := auth.SessionWithRole("tp0204-integration-ungranted", "jti-tp0204", auth.RoleDailyUser)
	if auth.GateGlobalCorpusRead(sess).Allowed {
		t.Fatalf("fixture drift: %q holds %s, so this test cannot prove the ungranted path", sess.UserID, auth.GrantGlobalCorpusRead)
	}
	source := string(sess.Source)

	groups := metrics.CorpusRouteGroups()
	if len(groups) != 16 {
		t.Fatalf("metrics.CorpusRouteGroups() returned %d groups, want 16 (Tier A + Tier B, spec.md §4.2)", len(groups))
	}

	downstream := &corpusGrantObserveDownstream{}
	for _, group := range groups {
		before := corpusGrantObserveWouldDeny(group, sess.UserID, source)
		allowedBefore := corpusGrantObserveAllowed(group, source)

		req := httptest.NewRequest(http.MethodGet, corpusGrantObserveSensitiveTarget(group), nil)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		gate.Observe(group)(downstream.handler()).ServeHTTP(rec, req)

		if rec.Code == http.StatusForbidden {
			t.Fatalf("route_group=%q: OBSERVE returned 403; the observe stage must never deny", group)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("route_group=%q: status = %d, want 200 (the downstream handler's own status)", group, rec.Code)
		}
		if got := rec.Body.String(); got != `{"results":[]}` {
			t.Fatalf("route_group=%q: body = %q, want the downstream body unchanged", group, got)
		}
		if delta := corpusGrantObserveWouldDeny(group, sess.UserID, source) - before; delta != 1 {
			t.Fatalf("route_group=%q: would_deny delta = %v, want 1 — an ungranted request was admitted without being counted", group, delta)
		}
		if delta := corpusGrantObserveAllowed(group, source) - allowedBefore; delta != 0 {
			t.Fatalf("route_group=%q: allowed delta = %v, want 0", group, delta)
		}
	}

	if downstream.ran != len(groups) {
		t.Fatalf("downstream handler ran %d times, want %d — the gate short-circuited at least one group", downstream.ran, len(groups))
	}

	// ── Log hygiene: answers WHO, never WHAT (design.md §4) ─────────────────
	raw := logs.String()
	if raw == "" {
		t.Fatal("no log output captured; the would-deny warn line was never emitted")
	}
	for _, secret := range []string{corpusGrantSecretQuery, corpusGrantSecretArtifactID, corpusGrantSecretTitle} {
		if strings.Contains(raw, secret) {
			t.Fatalf("the corpus_grant_would_deny log leaked %q; the warn line must carry no query text, artifact id, or title", secret)
		}
	}

	var seenGroups int
	var sawEvent bool
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // non-JSON lines from other emitters are not this test's concern
		}
		if entry["event"] != "corpus_grant_would_deny" {
			continue
		}
		sawEvent = true
		seenGroups++

		if entry["level"] != "WARN" {
			t.Errorf("corpus_grant_would_deny logged at level %v, want WARN", entry["level"])
		}
		if entry["user_id"] != sess.UserID {
			t.Errorf("log user_id = %v, want %q — the line must answer WHO would have been denied", entry["user_id"], sess.UserID)
		}
		if entry["session_source"] != source {
			t.Errorf("log session_source = %v, want %q", entry["session_source"], source)
		}
		if entry["required_grant"] != auth.GrantGlobalCorpusRead {
			t.Errorf("log required_grant = %v, want %q", entry["required_grant"], auth.GrantGlobalCorpusRead)
		}
		if entry["enforcement_mode"] != "observe" {
			t.Errorf("log enforcement_mode = %v, want %q", entry["enforcement_mode"], "observe")
		}
		routeGroup, _ := entry["route_group"].(string)
		if err := metrics.ValidateCorpusRouteGroup(metrics.CorpusRouteGroup(routeGroup)); err != nil {
			t.Errorf("log route_group = %q, which is outside the closed sixteen-value set: %v", routeGroup, err)
		}
	}

	if !sawEvent {
		t.Fatal("no corpus_grant_would_deny event was logged; the observation window is silent")
	}
	if seenGroups != len(groups) {
		t.Errorf("logged %d corpus_grant_would_deny events, want %d (one per route group)", seenGroups, len(groups))
	}
}

// TestIntegration_CorpusGrantObserve_GrantedPrincipalCountsAllowedAndModeGaugeReportsObserve
// is TP-02-05 / SCN-108-O02.
//
// A principal WITH corpus:read is admitted and lands on the allowed counter
// while the would-deny counter stays flat, and no would-deny warn line is
// emitted for them. The flat would-deny assertion is the one that matters: a
// gate that counted every request as a counterfactual denial would make the
// UC-108-001 grant list name every principal, granted or not.
//
// The mode gauge is asserted through metrics.SetCorpusGrantEnforcementMode
// because that is the only writer that exists in this scope — `cmd/core`
// resolves the stage but does not yet publish it (that wiring belongs with the
// Scope 03 mount). The assertion is therefore about the metric's contract, not
// about startup wiring, and is written not to imply otherwise.
func TestIntegration_CorpusGrantObserve_GrantedPrincipalCountsAllowedAndModeGaugeReportsObserve(t *testing.T) {
	logs := corpusGrantObserveCaptureLogs(t)
	gate := api.NewCorpusGrantGate(false)

	sess := auth.SessionWithRole("tp0205-integration-granted", "jti-tp0205", auth.RoleDailyUser, auth.GrantGlobalCorpusRead)
	if !auth.GateGlobalCorpusRead(sess).Allowed {
		t.Fatalf("fixture drift: %q does not hold %s, so this test cannot prove the granted path", sess.UserID, auth.GrantGlobalCorpusRead)
	}
	source := string(sess.Source)

	groups := metrics.CorpusRouteGroups()
	downstream := &corpusGrantObserveDownstream{}
	for _, group := range groups {
		allowedBefore := corpusGrantObserveAllowed(group, source)
		denyBefore := corpusGrantObserveWouldDeny(group, sess.UserID, source)

		req := httptest.NewRequest(http.MethodGet, corpusGrantObserveSensitiveTarget(group), nil)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		rec := httptest.NewRecorder()
		gate.Observe(group)(downstream.handler()).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("route_group=%q: status = %d, want 200", group, rec.Code)
		}
		if delta := corpusGrantObserveAllowed(group, source) - allowedBefore; delta != 1 {
			t.Fatalf("route_group=%q: allowed delta = %v, want 1", group, delta)
		}
		if delta := corpusGrantObserveWouldDeny(group, sess.UserID, source) - denyBefore; delta != 0 {
			t.Fatalf("route_group=%q: would_deny delta = %v, want 0 — a granted principal is not a counterfactual denial", group, delta)
		}
	}

	if downstream.ran != len(groups) {
		t.Fatalf("downstream handler ran %d times, want %d", downstream.ran, len(groups))
	}

	if strings.Contains(logs.String(), "corpus_grant_would_deny") {
		t.Errorf("a corpus_grant_would_deny warn line was emitted for a GRANTED principal; the log would name principals who already hold the grant")
	}

	// Mode gauge: OBSERVE reports 0. An unset gauge also reads 0, so the
	// ENFORCE case is asserted first — that is what proves the 0 came from an
	// explicit write rather than from the zero value.
	t.Cleanup(func() { metrics.SetCorpusGrantEnforcementMode(false) })
	metrics.SetCorpusGrantEnforcementMode(true)
	if got := corpusGrantObserveModeGauge(t); got != 1 {
		t.Fatalf("smackerel_auth_corpus_grant_enforcement_mode = %v after ENFORCE, want 1", got)
	}
	metrics.SetCorpusGrantEnforcementMode(false)
	if got := corpusGrantObserveModeGauge(t); got != 0 {
		t.Fatalf("smackerel_auth_corpus_grant_enforcement_mode = %v after OBSERVE, want 0", got)
	}
}

// TestIntegration_CorpusGrantObserve_ScopeRejectedCounterIsNotReused is
// R-108-O2 asserted in the live lane: the spec-060 denial counter must stay
// flat across a full sixteen-group observe sweep, so an operator can tell a
// real ENFORCE 403 apart from an OBSERVE counterfactual.
func TestIntegration_CorpusGrantObserve_ScopeRejectedCounterIsNotReused(t *testing.T) {
	corpusGrantObserveCaptureLogs(t)
	gate := api.NewCorpusGrantGate(false)

	sess := auth.SessionWithRole("tp0204-o2-integration", "jti-o2", auth.RoleDailyUser)
	rejected := metrics.AuthScopeRejected.WithLabelValues(auth.GrantGlobalCorpusRead, sess.UserID)
	before := testutil.ToFloat64(rejected)

	downstream := &corpusGrantObserveDownstream{}
	for _, group := range metrics.CorpusRouteGroups() {
		req := httptest.NewRequest(http.MethodGet, corpusGrantObserveSensitiveTarget(group), nil)
		req = req.WithContext(auth.WithSession(req.Context(), sess))
		gate.Observe(group)(downstream.handler()).ServeHTTP(httptest.NewRecorder(), req)
	}

	if delta := testutil.ToFloat64(rejected) - before; delta != 0 {
		t.Fatalf("smackerel_auth_scope_rejected_total moved by %v across an OBSERVE sweep; R-108-O2 reserves it for real ENFORCE denials", delta)
	}
}

// corpusGrantObserveModeGauge reads the enforcement-mode gauge from the real
// gatherer rather than from the package variable, so the assertion covers what
// a scraping dashboard would actually see.
func corpusGrantObserveModeGauge(t *testing.T) float64 {
	t.Helper()
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range gathered {
		if mf.GetName() != "smackerel_auth_corpus_grant_enforcement_mode" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if g := m.GetGauge(); g != nil {
				return g.GetValue()
			}
		}
	}
	t.Fatal("smackerel_auth_corpus_grant_enforcement_mode is not exposed by the gatherer")
	return 0
}
