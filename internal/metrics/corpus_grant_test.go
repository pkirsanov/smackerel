// Spec 108 Scope 02 — TP-02-03.
//
// The contract under test is the CLOSED-SET guarantee, not merely "the three
// metrics exist". The defect class these assertions exist to prevent is
// unbounded label cardinality (R-108-O3/O4): a raw request path such as
// `/api/artifact/{id}` reaching the `route_group` label would mint one
// Prometheus series per artifact and take the process down by memory long
// before an operator noticed a dashboard was wrong.
//
// Two separate things are therefore proved, and a test that proved only the
// first would be worthless:
//
//  1. The sixteen documented values are ACCEPTED, in the documented Tier A
//     then Tier B order. A silently-added seventeenth group, a renamed group,
//     or a reordered set fails here.
//  2. Anything outside that set is REJECTED and emits NOTHING. It is not
//     enough that the recorder returns an error — the assertion walks the real
//     Prometheus gatherer afterwards and proves no series carrying the forged
//     value was created. A recorder that returned an error *after* calling
//     `.Inc()` would pass a return-value-only check and still be the bug.
//
// R-108-O2 is proved here too: `smackerel_auth_scope_rejected_total` keeps its
// spec-060 meaning (a real 403 emitted under ENFORCE) and MUST NOT move when
// the observe signal is recorded, so an operator can tell an actual denial
// apart from a counterfactual one.
//
// All assertions go through the shared global registry and take before/after
// deltas, matching the convention already established in auth_test.go.
package metrics

import (
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const (
	metricNameCorpusGrantWouldDeny = "smackerel_auth_corpus_grant_would_deny_total"
	metricNameCorpusGrantAllowed   = "smackerel_auth_corpus_grant_allowed_total"
	metricNameCorpusGrantBypassed  = "smackerel_auth_corpus_grant_bypassed_total"
	metricNameCorpusGrantMode      = "smackerel_auth_corpus_grant_enforcement_mode"
	metricNameAuthScopeRejected    = "smackerel_auth_scope_rejected_total"
)

// corpusGrantDocumentedGroups restates the sixteen-value closed set from
// spec.md §4.2 as an INDEPENDENT literal rather than deriving it from
// CorpusRouteGroups(). Deriving it would make the test tautological: adding a
// seventeenth group to the canonical slice would silently update the
// "expected" list too and the assertion could never fail.
var corpusGrantDocumentedGroups = []CorpusRouteGroup{
	// Tier A — raw corpus retrieval.
	"search",
	"digest",
	"recent",
	"artifact_detail",
	"artifact_domain",
	"export",
	"context_for",
	"knowledge",
	// Tier B — corpus-derived Phase-5 intelligence (§18 decision 5).
	"expertise",
	"learning_paths",
	"subscriptions",
	"serendipity",
	"content_fuel",
	"quick_references",
	"monthly_report",
	"seasonal_patterns",
}

// corpusGrantRouteGroupLabelValues returns every `route_group` label value
// currently present on the named metric family, read from the real gatherer.
// This is what makes the cardinality assertions real: it observes the series
// that actually exist rather than trusting a recorder's return value.
func corpusGrantRouteGroupLabelValues(t *testing.T, metricName string) []string {
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

// corpusGrantGaugeValue reads a single non-vector gauge out of the gatherer.
func corpusGrantGaugeValue(t *testing.T, metricName string) (float64, bool) {
	t.Helper()
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range gathered {
		if mf.GetName() != metricName {
			continue
		}
		for _, m := range mf.GetMetric() {
			if g := m.GetGauge(); g != nil {
				return g.GetValue(), true
			}
		}
	}
	return 0, false
}

// TestCorpusGrantMetrics_CoverageCellIsClosableByEitherOutcome is the
// capability F-108-COVERAGE-LABEL-01 said was missing: for a given
// (user_id, route_group) cell, coverage must be establishable from observed
// traffic alone, whichever way the gate went.
//
// Before the allowed counter carried `user_id`, only the DENIED half of the
// population was attributable, so an operator could never distinguish "this
// principal exercised this route group and was allowed" from "this principal
// never touched it" — and decision 1(b) fell back to per-cell attestation.
// The two sub-cases below are exactly the two ways a real cell gets closed.
func TestCorpusGrantMetrics_CoverageCellIsClosableByEitherOutcome(t *testing.T) {
	const source = "per_user_token"
	group := CorpusRouteGroupExpertise // a silent Tier B group

	// Cell closed by an ALLOWED observation.
	grantedUser := "tp0203-coverage-granted"
	allowChild := AuthCorpusGrantAllowed.WithLabelValues(string(group), grantedUser, source)
	before := testutil.ToFloat64(allowChild)
	if err := RecordCorpusGrantAllowed(group, grantedUser, source); err != nil {
		t.Fatalf("RecordCorpusGrantAllowed: %v", err)
	}
	if got := testutil.ToFloat64(allowChild) - before; got != 1 {
		t.Fatalf("allowed delta for (user=%s, group=%s) = %v, want 1; a granted principal's traffic must be attributable to its own cell", grantedUser, group, got)
	}

	// Cell closed by a WOULD-DENY observation, for a DIFFERENT principal on
	// the same route group. The two must not collide into one series.
	deniedUser := "tp0203-coverage-denied"
	denyChild := AuthCorpusGrantWouldDeny.WithLabelValues(string(group), deniedUser, source)
	before = testutil.ToFloat64(denyChild)
	if err := RecordCorpusGrantWouldDeny(group, deniedUser, source); err != nil {
		t.Fatalf("RecordCorpusGrantWouldDeny: %v", err)
	}
	if got := testutil.ToFloat64(denyChild) - before; got != 1 {
		t.Fatalf("would-deny delta for (user=%s, group=%s) = %v, want 1", deniedUser, group, got)
	}

	// The granted principal's cell must not have moved when the other
	// principal was observed. Per-principal attribution is the whole point;
	// if one principal's traffic bled into another's cell, coverage would be
	// claimed for a principal that never called.
	if got := testutil.ToFloat64(allowChild) - 0; got == 0 {
		t.Fatalf("the granted principal's allowed series vanished")
	}
	crossTalk := AuthCorpusGrantAllowed.WithLabelValues(string(group), deniedUser, source)
	if got := testutil.ToFloat64(crossTalk); got != 0 {
		t.Errorf("the denied principal has a non-zero ALLOWED count (%v) on the same route group; coverage would be credited to a principal that was never allowed", got)
	}
}

// TestCorpusGrantMetrics_BypassedBandIsObservableButNotPredictive is the
// SEC-108-03 regression.
//
// The bypass band (shared-token, bootstrap) is exempt from `RequireScope`, so
// counting it as a would-be denial would make the would-deny counter a false
// predictor of the ENFORCE outcome. The original code therefore returned
// silently — and that traded one defect for a worse one: under OBSERVE
// `RequireScope` is not mounted at all, so this band appeared in NO spec-108
// series whatsoever. An operator reading the coverage table before authorising
// the flip could not distinguish "no bypassing principal used this route
// group" from "we had no way to see whether one did".
//
// Both properties are asserted together because either alone is satisfiable by
// the wrong implementation: emitting into would-deny would restore visibility
// while corrupting the prediction, and emitting nothing keeps the prediction
// clean while restoring the blind spot.
func TestCorpusGrantMetrics_BypassedBandIsObservableButNotPredictive(t *testing.T) {
	group := CorpusRouteGroupSearch

	for _, source := range []string{"shared_token", "bootstrap"} {
		t.Run(source, func(t *testing.T) {
			bypassChild := AuthCorpusGrantBypassed.WithLabelValues(string(group), source)
			denyChild := AuthCorpusGrantWouldDeny.WithLabelValues(string(group), "", source)
			allowChild := AuthCorpusGrantAllowed.WithLabelValues(string(group), "", source)

			bypassBefore := testutil.ToFloat64(bypassChild)
			denyBefore := testutil.ToFloat64(denyChild)
			allowBefore := testutil.ToFloat64(allowChild)

			if err := RecordCorpusGrantBypassed(group, source); err != nil {
				t.Fatalf("RecordCorpusGrantBypassed(%q): %v", source, err)
			}

			// (1) The band is now VISIBLE.
			if got := testutil.ToFloat64(bypassChild) - bypassBefore; got != 1 {
				t.Fatalf("bypassed delta for (group=%s, source=%s) = %v, want 1; without this series the OBSERVE window cannot tell silence from zero", group, source, got)
			}

			// (2) The would-deny PREDICTION is untouched. A bypassing session
			// is never denied under ENFORCE, so crediting it here would inflate
			// the UC-108-001 grant list with principals that will never be
			// refused.
			if got := testutil.ToFloat64(denyChild) - denyBefore; got != 0 {
				t.Errorf("would-deny moved by %v for a BYPASSED %s session; the counter would then mispredict the ENFORCE outcome", got, source)
			}
			if got := testutil.ToFloat64(allowChild) - allowBefore; got != 0 {
				t.Errorf("allowed moved by %v for a BYPASSED %s session; a bypass is not a grant and must not close a coverage cell", got, source)
			}
		})
	}
}

// TestCorpusGrantMetrics_BypassedRejectsUnknownRouteGroup proves the new
// recorder inherits the closed-set guarantee rather than opting out of it. A
// third recorder that accepted a raw path would reintroduce exactly the
// unbounded-cardinality defect the other two are built to prevent.
func TestCorpusGrantMetrics_BypassedRejectsUnknownRouteGroup(t *testing.T) {
	forged := CorpusRouteGroup("/api/artifact/01JQFORGEDBYPASSXXXXXXXXXX")

	if err := RecordCorpusGrantBypassed(forged, "shared_token"); !errors.Is(err, ErrUnknownCorpusRouteGroup) {
		t.Fatalf("RecordCorpusGrantBypassed(%q) = %v, want ErrUnknownCorpusRouteGroup", forged, err)
	}

	// Returning an error is not enough: a recorder that incremented BEFORE
	// validating would pass a return-value-only check and still create the
	// series. Walk the real gatherer and prove no such series exists.
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != "smackerel_auth_corpus_grant_bypassed_total" {
			continue
		}
		for _, m := range fam.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetValue() == string(forged) {
					t.Fatalf("a series carrying the forged route group %q was created; the closed-set guard ran too late", forged)
				}
			}
		}
	}
}

// TestCorpusGrantMetrics_RegisteredInAuthFamily proves the four additions are
// registered with the default registry under the existing `smackerel_auth_*`
// name family — they extend that family rather than forking a parallel one.
func TestCorpusGrantMetrics_RegisteredInAuthFamily(t *testing.T) {
	// CounterVecs only surface under Gather() once a labeled child exists, so
	// seed one through the real recorder (which also proves the happy path
	// returns nil for an in-set group).
	if err := RecordCorpusGrantWouldDeny(CorpusRouteGroupSearch, "tp0203-seed", "per_user_token"); err != nil {
		t.Fatalf("RecordCorpusGrantWouldDeny(search): unexpected error %v", err)
	}
	if err := RecordCorpusGrantAllowed(CorpusRouteGroupSearch, "tp0203-seed", "per_user_token"); err != nil {
		t.Fatalf("RecordCorpusGrantAllowed(search): unexpected error %v", err)
	}
	if err := RecordCorpusGrantBypassed(CorpusRouteGroupSearch, "shared_token"); err != nil {
		t.Fatalf("RecordCorpusGrantBypassed(search): unexpected error %v", err)
	}

	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	present := make(map[string]bool, len(gathered))
	for _, mf := range gathered {
		present[mf.GetName()] = true
	}

	for _, name := range []string{
		metricNameCorpusGrantWouldDeny,
		metricNameCorpusGrantAllowed,
		metricNameCorpusGrantBypassed,
		metricNameCorpusGrantMode,
	} {
		if !present[name] {
			t.Errorf("metric %q is not registered with the default Prometheus registry", name)
		}
		if !strings.HasPrefix(name, "smackerel_auth_") {
			t.Errorf("metric %q does not extend the smackerel_auth_* family", name)
		}
	}
}

// TestCorpusGrantRouteGroups_ClosedSixteenValueSet proves the canonical set is
// exactly the sixteen documented values in the documented order. Tier A and
// Tier B are asserted together because §18 decision 5 made Tier B carry the
// same authority; a set that silently dropped Tier B would still be "eight
// valid groups" and would pass a count-only check.
func TestCorpusGrantRouteGroups_ClosedSixteenValueSet(t *testing.T) {
	got := CorpusRouteGroups()
	if len(got) != 16 {
		t.Fatalf("CorpusRouteGroups() has %d entries, want exactly 16 (spec.md §4.2 Tier A + Tier B): %v", len(got), got)
	}
	if len(corpusGrantDocumentedGroups) != 16 {
		t.Fatalf("test fixture drift: corpusGrantDocumentedGroups has %d entries, want 16", len(corpusGrantDocumentedGroups))
	}
	for i, want := range corpusGrantDocumentedGroups {
		if got[i] != want {
			t.Errorf("CorpusRouteGroups()[%d] = %q, want %q (Tier A then Tier B order)", i, got[i], want)
		}
	}
	// Every documented value must validate.
	for _, group := range corpusGrantDocumentedGroups {
		if err := ValidateCorpusRouteGroup(group); err != nil {
			t.Errorf("ValidateCorpusRouteGroup(%q) = %v, want nil", group, err)
		}
	}
}

// TestCorpusGrantRouteGroups_ReturnsDefensiveCopy proves a caller cannot
// corrupt the canonical set through the slice it is handed. The gate mount
// (Scope 03) iterates this slice; if it returned the backing array, one
// careless caller could rewrite the closed set for the whole process.
func TestCorpusGrantRouteGroups_ReturnsDefensiveCopy(t *testing.T) {
	first := CorpusRouteGroups()
	forged := CorpusRouteGroup("/api/artifact/9f3c2a11-forged")
	first[0] = forged

	second := CorpusRouteGroups()
	if second[0] == forged {
		t.Fatalf("CorpusRouteGroups() exposed its backing array: a caller mutation leaked into the canonical set (%q)", second[0])
	}
	if second[0] != CorpusRouteGroupSearch {
		t.Fatalf("CorpusRouteGroups()[0] = %q, want %q", second[0], CorpusRouteGroupSearch)
	}
	if err := ValidateCorpusRouteGroup(forged); err == nil {
		t.Fatalf("ValidateCorpusRouteGroup(%q) = nil, want ErrUnknownCorpusRouteGroup after a caller-side mutation", forged)
	}
}

// TestCorpusGrantRouteGroup_RejectsOutOfSetValues is the cardinality guard.
// Each case is a shape that a real regression would produce: a raw request
// path, a path carrying a query string, a per-artifact path, a near-miss on
// spelling or case, and the empty value a zero-valued struct field would give.
//
// The assertion has two halves and BOTH are required. The error return proves
// the recorder refused; the gatherer walk proves it refused BEFORE touching
// the counter. A recorder that incremented and then returned an error would
// satisfy the first half and still mint the unbounded series.
func TestCorpusGrantRouteGroup_RejectsOutOfSetValues(t *testing.T) {
	outOfSet := []CorpusRouteGroup{
		"",
		"/api/search",
		"/api/search?q=my+private+medical+question",
		"/api/artifact/9f3c2a11-4d5e-4f60-9a7b-1c2d3e4f5a6b",
		"SEARCH",
		"search ",
		"artifact",
		"unknown",
	}

	for _, group := range outOfSet {
		t.Run("group="+string(group), func(t *testing.T) {
			if err := ValidateCorpusRouteGroup(group); !errors.Is(err, ErrUnknownCorpusRouteGroup) {
				t.Fatalf("ValidateCorpusRouteGroup(%q) = %v, want ErrUnknownCorpusRouteGroup", group, err)
			}

			if err := RecordCorpusGrantWouldDeny(group, "tp0203-user", "per_user_token"); !errors.Is(err, ErrUnknownCorpusRouteGroup) {
				t.Fatalf("RecordCorpusGrantWouldDeny(%q) = %v, want ErrUnknownCorpusRouteGroup", group, err)
			}
			if err := RecordCorpusGrantAllowed(group, "tp0203-user", "per_user_token"); !errors.Is(err, ErrUnknownCorpusRouteGroup) {
				t.Fatalf("RecordCorpusGrantAllowed(%q) = %v, want ErrUnknownCorpusRouteGroup", group, err)
			}

			// The refusal must have happened before any series was created.
			for _, metricName := range []string{metricNameCorpusGrantWouldDeny, metricNameCorpusGrantAllowed} {
				for _, seen := range corpusGrantRouteGroupLabelValues(t, metricName) {
					if seen == string(group) {
						t.Fatalf("%s minted a series with the out-of-set route_group %q — the recorder incremented before refusing (R-108-O3/O4)", metricName, group)
					}
				}
			}
		})
	}
}

// TestCorpusGrantMetrics_AllEmittedLabelValuesStayInClosedSet is the global
// invariant: whatever else the rest of this package's tests emitted, EVERY
// `route_group` series that exists is one of the sixteen. It is deliberately
// an assertion over observed reality rather than over the inputs this test
// chose, so an emitter added elsewhere that bypasses ValidateCorpusRouteGroup
// is caught here.
func TestCorpusGrantMetrics_AllEmittedLabelValuesStayInClosedSet(t *testing.T) {
	allowed := make(map[string]bool, len(corpusGrantDocumentedGroups))
	for _, group := range corpusGrantDocumentedGroups {
		allowed[string(group)] = true
	}

	// Emit across the whole closed set so the assertion has real material.
	for _, group := range CorpusRouteGroups() {
		if err := RecordCorpusGrantWouldDeny(group, "tp0203-closed-set", "per_user_token"); err != nil {
			t.Fatalf("RecordCorpusGrantWouldDeny(%q): unexpected error %v", group, err)
		}
		if err := RecordCorpusGrantAllowed(group, "tp0203-closed-set", "per_user_token"); err != nil {
			t.Fatalf("RecordCorpusGrantAllowed(%q): unexpected error %v", group, err)
		}
	}

	for _, metricName := range []string{metricNameCorpusGrantWouldDeny, metricNameCorpusGrantAllowed} {
		values := corpusGrantRouteGroupLabelValues(t, metricName)
		if len(values) == 0 {
			t.Fatalf("%s exposed no route_group series after emitting across the closed set", metricName)
		}
		for _, value := range values {
			if !allowed[value] {
				t.Errorf("%s carries route_group=%q, which is outside the closed sixteen-value set (unbounded-cardinality regression)", metricName, value)
			}
			if strings.ContainsAny(value, "/?&=") {
				t.Errorf("%s carries route_group=%q, which looks like a raw request path — raw paths are never label values (R-108-O3/O4)", metricName, value)
			}
		}
	}
}

// TestCorpusGrantMetrics_DoNotReuseScopeRejectedCounter is R-108-O2. The
// observe signal is a COUNTERFACTUAL ("this would have been denied"); reusing
// smackerel_auth_scope_rejected_total for it would make a real ENFORCE denial
// and an OBSERVE-stage non-denial indistinguishable in the same series, which
// is exactly the ambiguity the requirement forbids.
func TestCorpusGrantMetrics_DoNotReuseScopeRejectedCounter(t *testing.T) {
	// AuthScopeRejected is labelled {required_scope, user_id}; seed the exact
	// child a corpus denial would land on so the delta read is well defined.
	rejectedChild := AuthScopeRejected.WithLabelValues("corpus:read", "tp0203-o2")
	before := testutil.ToFloat64(rejectedChild)

	if err := RecordCorpusGrantWouldDeny(CorpusRouteGroupDigest, "tp0203-o2", "per_user_token"); err != nil {
		t.Fatalf("RecordCorpusGrantWouldDeny: %v", err)
	}
	if err := RecordCorpusGrantAllowed(CorpusRouteGroupDigest, "tp0203-o2", "per_user_token"); err != nil {
		t.Fatalf("RecordCorpusGrantAllowed: %v", err)
	}

	after := testutil.ToFloat64(rejectedChild)
	if delta := after - before; delta != 0 {
		t.Fatalf("%s moved by %v while recording the observe signal; R-108-O2 forbids reusing it for the counterfactual", metricNameAuthScopeRejected, delta)
	}

	// The three names must also be distinct from the spec-060 counter.
	for _, name := range []string{
		metricNameCorpusGrantWouldDeny,
		metricNameCorpusGrantAllowed,
		metricNameCorpusGrantMode,
	} {
		if name == metricNameAuthScopeRejected {
			t.Fatalf("%q collides with the spec-060 scope-rejected counter", name)
		}
	}
}

// TestCorpusGrantMetrics_WouldDenyAndAllowedIncrementIndependently pins each
// recorder to exactly one increment on exactly its own series. A wiring change
// that double-counted, or that incremented both counters for one request,
// would make the would-deny / allowed ratio meaningless for UC-108-001.
func TestCorpusGrantMetrics_WouldDenyAndAllowedIncrementIndependently(t *testing.T) {
	const (
		userID = "tp0203-independent"
		source = "per_user_token"
	)
	group := CorpusRouteGroupExpertise // a Tier B group, to keep Tier B live in the assertions

	denyChild := AuthCorpusGrantWouldDeny.WithLabelValues(string(group), userID, source)
	allowChild := AuthCorpusGrantAllowed.WithLabelValues(string(group), userID, source)

	denyBefore := testutil.ToFloat64(denyChild)
	allowBefore := testutil.ToFloat64(allowChild)

	if err := RecordCorpusGrantWouldDeny(group, userID, source); err != nil {
		t.Fatalf("RecordCorpusGrantWouldDeny: %v", err)
	}
	if got := testutil.ToFloat64(denyChild) - denyBefore; got != 1 {
		t.Errorf("would-deny delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(allowChild) - allowBefore; got != 0 {
		t.Errorf("allowed counter moved by %v while recording a would-deny; the two must be independent", got)
	}

	allowBefore = testutil.ToFloat64(allowChild)
	denyBefore = testutil.ToFloat64(denyChild)
	if err := RecordCorpusGrantAllowed(group, userID, source); err != nil {
		t.Fatalf("RecordCorpusGrantAllowed: %v", err)
	}
	if got := testutil.ToFloat64(allowChild) - allowBefore; got != 1 {
		t.Errorf("allowed delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(denyChild) - denyBefore; got != 0 {
		t.Errorf("would-deny counter moved by %v while recording an allowed; the two must be independent", got)
	}
}

// TestCorpusGrantMetrics_BothCountersCarryUserIDSoCoverageIsComputable pins
// the label sets that make the §18 decision 1(b) coverage bar computable.
//
// `user_id` on the would-deny counter answers UC-108-001 ("who would be
// denied"). `user_id` on the ALLOWED counter is what closes
// F-108-COVERAGE-LABEL-01: without it a principal that holds the grant and
// uses it is indistinguishable from one that never called, so a coverage cell
// could only be closed by operator attestation. With both labelled, a cell is
// closed by observed traffic of either outcome.
//
// This test previously asserted the OPPOSITE for the allowed counter, pinning
// the gap while it was still open. Inverting it is the point: the assertion
// now fails if the label is ever dropped again.
func TestCorpusGrantMetrics_BothCountersCarryUserIDSoCoverageIsComputable(t *testing.T) {
	if err := RecordCorpusGrantWouldDeny(CorpusRouteGroupRecent, "tp0203-labels", "per_user_token"); err != nil {
		t.Fatalf("RecordCorpusGrantWouldDeny: %v", err)
	}
	if err := RecordCorpusGrantAllowed(CorpusRouteGroupRecent, "tp0203-labels", "per_user_token"); err != nil {
		t.Fatalf("RecordCorpusGrantAllowed: %v", err)
	}

	labelNames := func(metricName string) map[string]bool {
		gathered, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Fatalf("Gather: %v", err)
		}
		names := map[string]bool{}
		for _, mf := range gathered {
			if mf.GetName() != metricName {
				continue
			}
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					names[lp.GetName()] = true
				}
			}
		}
		return names
	}

	denyLabels := labelNames(metricNameCorpusGrantWouldDeny)
	for _, want := range []string{"route_group", "user_id", "session_source"} {
		if !denyLabels[want] {
			t.Errorf("%s is missing the %q label; UC-108-001 cannot be answered without it", metricNameCorpusGrantWouldDeny, want)
		}
	}

	allowLabels := labelNames(metricNameCorpusGrantAllowed)
	for _, want := range []string{"route_group", "user_id", "session_source"} {
		if !allowLabels[want] {
			t.Errorf("%s is missing the %q label; without user_id a granted principal's traffic is invisible and the decision 1(b) coverage bar collapses back to per-cell operator attestation (F-108-COVERAGE-LABEL-01)", metricNameCorpusGrantAllowed, want)
		}
	}
}

// TestSetCorpusGrantEnforcementMode_ReportsResolvedStage proves the gauge
// distinguishes the two stages. The 0 case matters most: an UNSET gauge also
// reads 0, so the value is only trustworthy because the resolution point sets
// it explicitly. This asserts the setter actually publishes both values rather
// than relying on the zero value for OBSERVE.
func TestSetCorpusGrantEnforcementMode_ReportsResolvedStage(t *testing.T) {
	t.Cleanup(func() { SetCorpusGrantEnforcementMode(false) })

	SetCorpusGrantEnforcementMode(true)
	got, ok := corpusGrantGaugeValue(t, metricNameCorpusGrantMode)
	if !ok {
		t.Fatalf("%s is not exposed by the gatherer", metricNameCorpusGrantMode)
	}
	if got != 1 {
		t.Errorf("%s after SetCorpusGrantEnforcementMode(true) = %v, want 1 (ENFORCE)", metricNameCorpusGrantMode, got)
	}

	SetCorpusGrantEnforcementMode(false)
	got, ok = corpusGrantGaugeValue(t, metricNameCorpusGrantMode)
	if !ok {
		t.Fatalf("%s is not exposed by the gatherer", metricNameCorpusGrantMode)
	}
	if got != 0 {
		t.Errorf("%s after SetCorpusGrantEnforcementMode(false) = %v, want 0 (OBSERVE)", metricNameCorpusGrantMode, got)
	}

	// Guard against a gauge that is a counter in disguise: setting the same
	// stage twice must be idempotent, not additive.
	SetCorpusGrantEnforcementMode(true)
	SetCorpusGrantEnforcementMode(true)
	got, _ = corpusGrantGaugeValue(t, metricNameCorpusGrantMode)
	if got != 1 {
		t.Errorf("%s after two ENFORCE sets = %v, want 1 (a gauge, not an accumulator)", metricNameCorpusGrantMode, got)
	}
}
