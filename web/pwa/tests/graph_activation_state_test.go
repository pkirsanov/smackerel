// graph_activation_state_test.go — BUG-080-001 SCOPE-04, test row
// T080-08-UNIT (`ui-unit`, SCN-080-001-08).
//
// SCOPE-04 implementation-plan item 1 requires ONE typed response
// decoder and activation/read model consumed by Wiki Browse, Graph
// availability, and readiness, and forbids a projection from inferring
// state "from HTTP code or `items.length` independently".
//
// The projection that satisfies that clause in production is
// internal/api.GraphReadiness.Snapshot(). Its GraphHealthSection is the
// wire shape a surface actually renders: it is served to authenticated
// callers of GET /api/health, and its Ready field is the aggregate
// answer behind GET /readyz?strict=true. Because there is exactly ONE
// publisher of ONE aggregate that every surface reads, two surfaces
// structurally cannot disagree about what a read meant — which is the
// invariant SCOPE-04 exists to establish. A second, on-demand reduction
// path would reintroduce the disagreement, so this test targets the
// shipped shape rather than a parallel projection.
//
// This file is the CONTRACT SWEEP over that rendered shape. It is
// deliberately complementary to tests/integration/graphapi/
// readiness_test.go, which proves the BEHAVIOUR — what makes readiness
// flip — against the live stack. The claim here is about the VALUE
// SPACE instead: every Activation, State, Code, and family row a
// reachable case can render belongs to a closed vocabulary, and the
// State/Ready pairing holds across the WHOLE derived state vocabulary
// rather than at the handful of points a behavioural test visits.
//
// Every vocabulary is DERIVED, never copied: the aggregate states from
// graphsynthetic.AggregateStates(), the canonical families from
// graphapi.RequiredGraphFamilies(), closed-vocabulary membership from
// the production Validate() oracle, and the readiness projection codes
// from a source scan of internal/api/graph_readiness.go. No state name,
// no code, no family name, and no count is written down here, so a
// fifth aggregate state, a ninth family, or a new projection code
// cannot be added upstream and silently pass this test.
package webcodegen_drift_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// Observation ages are expressed RELATIVE to the product's own
// freshness bound, so no case writes down a duration that could drift
// out of step with GraphObservationMaxAge.
var (
	graphSectionFreshAge = api.GraphObservationMaxAge / 10
	graphSectionStaleAge = api.GraphObservationMaxAge * 2
)

// graphSectionRepoRoot resolves the repository root from this file's
// own on-disk location, so the source scan below does not depend on the
// working directory the test runner happens to use.
func graphSectionRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed; cannot locate the readiness projection source to derive its closed code vocabulary")
	}
	// web/pwa/tests/ -> the repository root is three parents up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
}

// graphReadinessProjectionCodes derives the closed readiness-projection
// code vocabulary by PARSING the production source rather than copying
// it. A newly declared GraphReadinessCode* constant therefore joins the
// membership assertions the moment it exists, instead of falling
// outside a hand-maintained list.
func graphReadinessProjectionCodes(t *testing.T) []string {
	t.Helper()
	source := filepath.Join(graphSectionRepoRoot(t), "internal", "api", "graph_readiness.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse the readiness projection source: %v", err)
	}

	var codes []string
	for _, decl := range parsed.Decls {
		generic, ok := decl.(*ast.GenDecl)
		if !ok || generic.Tok != token.CONST {
			continue
		}
		for _, spec := range generic.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if !strings.HasPrefix(name.Name, "GraphReadinessCode") || i >= len(value.Values) {
					continue
				}
				literal, ok := value.Values[i].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Fatalf("readiness projection code %s is not a plain string literal; the closed code vocabulary must stay statically readable", name.Name)
				}
				unquoted, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("readiness projection code %s carries an unreadable literal: %v", name.Name, err)
				}
				codes = append(codes, unquoted)
			}
		}
	}
	return codes
}

// graphSectionFamilyRows builds one contract-valid row per canonical
// family, in canonical order, with the evidence reference DERIVED from
// the family name exactly as graphsynthetic.Validate requires.
func graphSectionFamilyRows(state graphsynthetic.ReadState, code string) []graphsynthetic.GraphFamilyResult {
	families := graphapi.RequiredGraphFamilies()
	rows := make([]graphsynthetic.GraphFamilyResult, 0, len(families))
	for _, family := range families {
		rows = append(rows, graphsynthetic.GraphFamilyResult{
			Family:      family,
			State:       state,
			DurationMs:  7,
			Code:        code,
			EvidenceRef: graphsynthetic.EvidenceRef(family),
		})
	}
	return rows
}

// graphSectionMixedFamilyRows builds a populated row set in which the
// LAST canonical family instead carries the given failure. The failing
// family is selected from the manifest, never named literally.
func graphSectionMixedFamilyRows(state graphsynthetic.ReadState, code string) []graphsynthetic.GraphFamilyResult {
	rows := graphSectionFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK)
	last := len(rows) - 1
	rows[last].State = state
	rows[last].Code = code
	return rows
}

// graphSectionProbe is a contract-valid aggregate used ONLY as the
// membership oracle below: one field is replaced with a candidate value
// and the PRODUCTION validator decides whether that value is inside the
// closed vocabulary. Nothing built here is ever published.
func graphSectionProbe() graphsynthetic.AggregateResult {
	return graphsynthetic.AggregateResult{
		Activation:  graphapi.ActivationEnabled,
		State:       graphsynthetic.AggregateAvailable,
		ObservedAt:  time.Now().UTC(),
		DurationMs:  9,
		Code:        graphsynthetic.CodeOK,
		EvidenceRef: graphsynthetic.AggregateEvidenceRef,
		Families:    graphSectionFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK),
	}
}

// closedAggregateState reports whether the production validator accepts
// the candidate as an aggregate state, every other field held valid.
func closedAggregateState(candidate string) bool {
	probe := graphSectionProbe()
	probe.State = graphsynthetic.AggregateState(candidate)
	return probe.Validate() == nil
}

// closedActivation reports whether the production validator accepts the
// candidate as an activation state.
func closedActivation(candidate string) bool {
	probe := graphSectionProbe()
	probe.Activation = graphapi.ActivationState(candidate)
	return probe.Validate() == nil
}

// closedSyntheticCode reports whether the production validator accepts
// the candidate as a synthetic diagnostic code.
func closedSyntheticCode(candidate string) bool {
	probe := graphSectionProbe()
	probe.Code = candidate
	return probe.Validate() == nil
}

// graphSectionEnabledCapability resolves an ENABLED activation policy
// from a cursor-secret env var set to a non-secret presence marker. The
// classifier reads only whether the value is non-empty, never the value
// itself, so no credential-shaped literal is needed here.
func graphSectionEnabledCapability(t *testing.T) *graphapi.GraphCapability {
	t.Helper()
	const envName = "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080_UIUNIT_ENABLED"
	t.Setenv(envName, "present")
	capability := graphapi.NewGraphCapability(graphapi.Config{CursorSecretEnv: envName})
	if capability.Disabled() {
		t.Fatal("test setup: expected an ENABLED capability from a set cursor-secret env var")
	}
	return capability
}

// graphSectionDisabledCapability resolves an explicitly DISABLED
// activation policy from a cursor-secret env var that is never set —
// the same unavailable-enabler trigger cmd/core classifies at boot.
func graphSectionDisabledCapability(t *testing.T) *graphapi.GraphCapability {
	t.Helper()
	capability := graphapi.NewGraphCapability(graphapi.Config{
		CursorSecretEnv: "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080_UIUNIT_DISABLED_DO_NOT_SET",
	})
	if !capability.Disabled() {
		t.Fatal("test setup: expected a DISABLED capability from an unset cursor-secret env var")
	}
	return capability
}

// graphSectionReadiness constructs the REAL production projection with
// the product-owned freshness bound. Construction is fail-loud, so an
// error here is a genuine failure and never a skip.
func graphSectionReadiness(t *testing.T, capability *graphapi.GraphCapability) *api.GraphReadiness {
	t.Helper()
	readiness, err := api.NewGraphReadiness(capability, api.GraphObservationMaxAge)
	if err != nil {
		t.Fatalf("api.NewGraphReadiness: %v", err)
	}
	return readiness
}

// graphSectionAggregate builds a contract-valid observation. Its age is
// supplied relative to the product freshness bound so a case can be
// fresh or stale without naming an absolute instant.
func graphSectionAggregate(
	t *testing.T,
	activation graphapi.ActivationState,
	state graphsynthetic.AggregateState,
	code string,
	rows []graphsynthetic.GraphFamilyResult,
	age time.Duration,
) graphsynthetic.AggregateResult {
	t.Helper()
	result := graphsynthetic.AggregateResult{
		Activation:  activation,
		State:       state,
		ObservedAt:  time.Now().UTC().Add(-age),
		DurationMs:  11,
		Code:        code,
		EvidenceRef: graphsynthetic.AggregateEvidenceRef,
		Families:    rows,
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("test setup: observation fixture is not contract-valid: %v", err)
	}
	return result
}

// graphSectionPublish publishes an observation and fails loudly if the
// projection refuses it, so a silently dropped fixture can never leave
// a case asserting against an empty projection.
func graphSectionPublish(t *testing.T, readiness *api.GraphReadiness, result graphsynthetic.AggregateResult) {
	t.Helper()
	if err := readiness.Publish(result); err != nil {
		t.Fatalf("Publish refused a contract-valid observation that agrees with the activation policy: %v", err)
	}
}

// graphSectionProjectionIsWired reports whether a projection carries an
// explicit activation policy, DERIVED from the projection itself rather
// than from anything a case expected it to render.
//
// The oracle is Publish, NOT Snapshot. Publish refuses an unwired
// projection — nil receiver OR nil capability — with the config-invalid
// projection code BEFORE it validates the observation, so a
// deliberately contract-INVALID probe separates the two answers:
//
//	unwired -> refused, naming api.GraphReadinessCodeConfigInvalid
//	wired   -> refused by the synthetic contract validator instead
//
// Both answers are refusals, so the probe is never stored and cannot
// disturb the projection it interrogates. Deciding wiring on a
// DIFFERENT code path than the one under test is what lets the wiring
// rule below survive a regression that rewrites only Snapshot's
// fail-closed branch.
func graphSectionProjectionIsWired(t *testing.T, projection *api.GraphReadiness) bool {
	t.Helper()
	var probe graphsynthetic.AggregateResult
	if probe.Validate() == nil {
		t.Fatal("anti-vacuity: the wiring probe is contract-VALID; a wired projection would STORE it and this oracle would corrupt the very projection it interrogates")
	}
	err := projection.Publish(probe)
	if err == nil {
		t.Fatal("Publish accepted a contract-invalid observation; the wiring oracle can no longer tell an unwired projection from a wired one, so the fail-closed rule below would be decided arbitrarily")
	}
	return !strings.Contains(err.Error(), api.GraphReadinessCodeConfigInvalid)
}

// graphSectionCase is one reachable production projection plus the
// single rendered shape it is allowed to produce.
type graphSectionCase struct {
	name string
	// build returns the REAL projection under test. A nil return is the
	// UNCONSTRUCTED projection — the fail-closed case.
	build          func(t *testing.T) *api.GraphReadiness
	wantActivation graphapi.ActivationState
	wantState      graphsynthetic.AggregateState
	wantCode       string
	wantReady      bool
	// wantObservation records whether a published observation should
	// reach the wire shape. It governs Families, ObservedAt, and
	// DurationMs together: those three travel as one or not at all.
	wantObservation bool
	// mustNotBe names the states this case would tempt a naive
	// projection into. Asserting them explicitly is what makes the
	// exclusivity claim adversarial rather than incidental.
	mustNotBe []graphsynthetic.AggregateState
	// why records the confusion the case exists to prevent.
	why string
}

// graphSectionCases enumerates every rendered shape Snapshot can reach:
// the unconstructed projection, both activation policies, the absent
// and stale observation paths, and each closed aggregate outcome.
var graphSectionCases = []graphSectionCase{
	{
		name:            "unconstructed_projection_fails_closed_instead_of_panicking",
		build:           func(*testing.T) *api.GraphReadiness { return nil },
		wantActivation:  graphapi.ActivationDisabled,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        api.GraphReadinessCodeConfigInvalid,
		wantReady:       false,
		wantObservation: false,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "an unwired deployment still renders something; if that something panicked or claimed ready, a missing projection would be indistinguishable from a healthy graph",
	},
	{
		name:            "constructed_shell_with_no_capability_also_fails_closed",
		build:           func(*testing.T) *api.GraphReadiness { return &api.GraphReadiness{} },
		wantActivation:  graphapi.ActivationDisabled,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        api.GraphReadinessCodeConfigInvalid,
		wantReady:       false,
		wantObservation: false,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "a projection value that EXISTS but carries no activation policy is the second half of the unwired condition; covering only the nil receiver would leave the fail-closed rule below binding one of the two shapes it claims to bind",
	},
	{
		name: "disabled_policy_is_truthfully_disabled_and_never_ready",
		build: func(t *testing.T) *api.GraphReadiness {
			return graphSectionReadiness(t, graphSectionDisabledCapability(t))
		},
		wantActivation:  graphapi.ActivationDisabled,
		wantState:       graphsynthetic.AggregatePolicyDisabled,
		wantCode:        graphsynthetic.CodePolicyDisabled,
		wantReady:       false,
		wantObservation: false,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregateUnavailable,
		},
		why: "an explicitly disabled capability is a valid deployment state, not a fault; rendering it as unavailable sends an operator hunting an outage that does not exist",
	},
	{
		name: "disabled_policy_with_a_published_observation_is_still_disabled",
		build: func(t *testing.T) *api.GraphReadiness {
			readiness := graphSectionReadiness(t, graphSectionDisabledCapability(t))
			graphSectionPublish(t, readiness, graphSectionAggregate(t,
				graphapi.ActivationDisabled,
				graphsynthetic.AggregatePolicyDisabled,
				graphsynthetic.CodePolicyDisabled,
				graphSectionFamilyRows(graphsynthetic.StateDisabled, graphsynthetic.CodePolicyDisabled),
				graphSectionFreshAge))
			return readiness
		},
		wantActivation:  graphapi.ActivationDisabled,
		wantState:       graphsynthetic.AggregatePolicyDisabled,
		wantCode:        graphsynthetic.CodePolicyDisabled,
		wantReady:       false,
		wantObservation: true,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregateUnavailable,
		},
		why: "the disabled short-circuit must outrank any observation, or an optimistic or stale read could advertise a disabled deployment as ready",
	},
	{
		name: "enabled_policy_without_an_observation_is_unavailable_not_empty",
		build: func(t *testing.T) *api.GraphReadiness {
			return graphSectionReadiness(t, graphSectionEnabledCapability(t))
		},
		wantActivation:  graphapi.ActivationEnabled,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        api.GraphReadinessCodeNotObserved,
		wantReady:       false,
		wantObservation: false,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "route presence alone must never promote the graph journey; a never-observed graph is unavailable, not an empty one waiting for the user to capture something",
	},
	{
		name: "enabled_policy_with_a_stale_available_observation_is_unavailable_not_ready",
		build: func(t *testing.T) *api.GraphReadiness {
			readiness := graphSectionReadiness(t, graphSectionEnabledCapability(t))
			graphSectionPublish(t, readiness, graphSectionAggregate(t,
				graphapi.ActivationEnabled,
				graphsynthetic.AggregateAvailable,
				graphsynthetic.CodeOK,
				graphSectionFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK),
				graphSectionStaleAge))
			return readiness
		},
		wantActivation:  graphapi.ActivationEnabled,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        api.GraphReadinessCodeStale,
		wantReady:       false,
		wantObservation: true,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "the observation itself says available and only its AGE demotes it, so a publisher that stopped reporting cannot leave a ready claim standing forever",
	},
	{
		name: "enabled_policy_with_a_fresh_available_observation_is_ready",
		build: func(t *testing.T) *api.GraphReadiness {
			readiness := graphSectionReadiness(t, graphSectionEnabledCapability(t))
			graphSectionPublish(t, readiness, graphSectionAggregate(t,
				graphapi.ActivationEnabled,
				graphsynthetic.AggregateAvailable,
				graphsynthetic.CodeOK,
				graphSectionFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK),
				graphSectionFreshAge))
			return readiness
		},
		wantActivation:  graphapi.ActivationEnabled,
		wantState:       graphsynthetic.AggregateAvailable,
		wantCode:        graphsynthetic.CodeOK,
		wantReady:       true,
		wantObservation: true,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateUnavailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "the projection must still be able to reach ready, or every not-ready assertion above would pass on a projection hardwired to refuse",
	},
	{
		name: "enabled_policy_with_a_fresh_degraded_observation_is_not_ready",
		build: func(t *testing.T) *api.GraphReadiness {
			readiness := graphSectionReadiness(t, graphSectionEnabledCapability(t))
			graphSectionPublish(t, readiness, graphSectionAggregate(t,
				graphapi.ActivationEnabled,
				graphsynthetic.AggregateDegraded,
				graphsynthetic.CodeOptionalOmitted,
				graphSectionMixedFamilyRows(graphsynthetic.StateFailed, graphsynthetic.CodeRouteAbsent),
				graphSectionFreshAge))
			return readiness
		},
		wantActivation:  graphapi.ActivationEnabled,
		wantState:       graphsynthetic.AggregateDegraded,
		wantCode:        graphsynthetic.CodeOptionalOmitted,
		wantReady:       false,
		wantObservation: true,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateUnavailable,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "a named optional omission is a partial state the UX must show; rendering it as available would claim a completeness the read never had",
	},
	{
		name: "enabled_policy_with_a_fresh_route_absent_observation_is_not_ready",
		build: func(t *testing.T) *api.GraphReadiness {
			readiness := graphSectionReadiness(t, graphSectionEnabledCapability(t))
			graphSectionPublish(t, readiness, graphSectionAggregate(t,
				graphapi.ActivationEnabled,
				graphsynthetic.AggregateUnavailable,
				graphsynthetic.CodeRouteAbsent,
				graphSectionFamilyRows(graphsynthetic.StateFailed, graphsynthetic.CodeRouteAbsent),
				graphSectionFreshAge))
			return readiness
		},
		wantActivation:  graphapi.ActivationEnabled,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeRouteAbsent,
		wantReady:       false,
		wantObservation: true,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "a route-missing read returns no rows; rendering that as ready or as an empty corpus is the original silent-absence defect",
	},
}

// TestGraphActivationProjectionUsesClosedExclusiveStates is
// T080-08-UNIT. It proves the SHIPPED activation/read projection —
// api.GraphReadiness.Snapshot(), the shape authenticated health and
// strict readiness render — carries only closed backend vocabulary,
// resolves exactly one state per rendered shape, refuses an invented
// state, and never lets a disabled, unobserved, stale, degraded, or
// failed outcome collapse into a ready one.
func TestGraphActivationProjectionUsesClosedExclusiveStates(t *testing.T) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatal("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; every family assertion below would sweep zero rows and pass without exercising anything")
	}
	closedStates := graphsynthetic.AggregateStates()
	if len(closedStates) == 0 {
		t.Fatal("anti-vacuity: graphsynthetic.AggregateStates() is empty; the exclusivity count below would be trivially satisfied")
	}
	if len(graphSectionCases) == 0 {
		t.Fatal("anti-vacuity: graphSectionCases is empty; this test would report PASS having rendered nothing")
	}

	projectionCodes := graphReadinessProjectionCodes(t)
	if len(projectionCodes) == 0 {
		t.Fatal("anti-vacuity: the source scan derived no GraphReadinessCode* constant; the code-membership assertion would then reject every projection code for the wrong reason")
	}
	// Control: the derived set must contain the codes Snapshot
	// demonstrably emits. Without it, a scan that matched nothing useful
	// could still come back non-empty and look healthy.
	for _, emitted := range []string{
		api.GraphReadinessCodeNotObserved,
		api.GraphReadinessCodeStale,
		api.GraphReadinessCodeConfigInvalid,
	} {
		if !slices.Contains(projectionCodes, emitted) {
			t.Fatalf("anti-vacuity: the derived projection-code set %v omits %q, which Snapshot emits; the source scan is not reading the real const block", projectionCodes, emitted)
		}
	}

	// A rendered Code is closed when the production synthetic validator
	// accepts it OR it is one of the readiness projection codes derived
	// from source. Nothing else may reach the wire.
	closedCode := func(candidate string) bool {
		return closedSyntheticCode(candidate) || slices.Contains(projectionCodes, candidate)
	}

	reached := make(map[graphsynthetic.AggregateState]bool, len(closedStates))
	rendered, readyTrue, readyFalse := 0, 0, 0
	wiredRendered, unwiredRendered := 0, 0

	for _, testCase := range graphSectionCases {
		t.Run(testCase.name, func(t *testing.T) {
			projection := testCase.build(t)
			section := projection.Snapshot()

			// Two facts about the RENDERED shape, derived once and used
			// by every rule clause below that reasons about the claim.
			enabled := graphapi.ActivationState(section.Activation) == graphapi.ActivationEnabled
			available := graphsynthetic.AggregateState(section.State) == graphsynthetic.AggregateAvailable

			// 1. WIRING binds the CLAIM, and it is judged BEFORE any of
			// this case's expected values. A projection carrying NO
			// activation policy — nil receiver or nil capability — MUST
			// render the fail-closed shape. The predicate is taken from
			// the projection itself, over a DIFFERENT code path than the
			// one that produced this section, so it holds no matter what
			// quadruple Snapshot chose to return.
			//
			// This is deliberately NOT a consistency check. A fail-open
			// answer that is internally CONSISTENT — ready + enabled +
			// available + an OK code — satisfies every pairing clause in
			// 6, so 6 structurally cannot see it; only wiring can.
			// Without this rule the fail-closed guarantee would rest
			// entirely on one case's pinned expectations, and relaxing
			// those expectations or deleting the case would let an
			// unwired deployment go back to rendering as a healthy
			// graph.
			wired := graphSectionProjectionIsWired(t, projection)
			if !wired {
				if section.Ready {
					t.Fatalf("an UNWIRED projection rendered ready=true; readiness derives from an explicit activation policy and this projection carries none — %s", testCase.why)
				}
				if enabled {
					t.Fatalf("an UNWIRED projection rendered activation %q; it has no activation policy that could have been enabled — %s", section.Activation, testCase.why)
				}
				if available {
					t.Fatalf("an UNWIRED projection rendered state %q; it has no observation source that could have reported availability — %s", section.State, testCase.why)
				}
				if got := graphapi.ActivationState(section.Activation); got != graphapi.ActivationDisabled {
					t.Fatalf("an UNWIRED projection rendered activation %q; the fail-closed shape is %q", got, graphapi.ActivationDisabled)
				}
				if got := graphsynthetic.AggregateState(section.State); got != graphsynthetic.AggregateUnavailable {
					t.Fatalf("an UNWIRED projection rendered state %q; the fail-closed shape is %q", got, graphsynthetic.AggregateUnavailable)
				}
				if section.Code != api.GraphReadinessCodeConfigInvalid {
					t.Fatalf("an UNWIRED projection rendered code %q; the fail-closed shape names %q, which tells an operator the projection is not constructed rather than that the graph is healthy", section.Code, api.GraphReadinessCodeConfigInvalid)
				}
				if section.ObservedAt != nil || section.DurationMs != nil || len(section.Families) != 0 {
					t.Fatalf("an UNWIRED projection rendered observation evidence (observed-at present=%v, duration present=%v, %d family rows); it has never received an observation to describe", section.ObservedAt != nil, section.DurationMs != nil, len(section.Families))
				}
			}

			// 2. Activation is inside the closed activation vocabulary.
			if !closedActivation(section.Activation) {
				t.Fatalf("rendered activation %q is outside the closed activation vocabulary the production validator enforces — %s", section.Activation, testCase.why)
			}
			if got := graphapi.ActivationState(section.Activation); got != testCase.wantActivation {
				t.Fatalf("rendered activation %q; want %q — %s", got, testCase.wantActivation, testCase.why)
			}

			// 3. State is inside the closed aggregate vocabulary, and
			// EXACTLY ONE closed state matches. This is the structural
			// exclusivity claim: "disabled AND available" or "empty AND
			// failed" must be unrepresentable, not merely unlikely.
			if !closedAggregateState(section.State) {
				t.Fatalf("rendered state %q is outside the closed aggregate vocabulary %v — %s", section.State, closedStates, testCase.why)
			}
			matches := 0
			for _, candidate := range closedStates {
				if graphsynthetic.AggregateState(section.State) == candidate {
					matches++
				}
			}
			if matches != 1 {
				t.Fatalf("rendered state %q matched %d closed states; exactly one must be active — %s", section.State, matches, testCase.why)
			}
			if got := graphsynthetic.AggregateState(section.State); got != testCase.wantState {
				t.Fatalf("rendered state %q; want %q — %s", got, testCase.wantState, testCase.why)
			}
			for _, forbidden := range testCase.mustNotBe {
				if graphsynthetic.AggregateState(section.State) == forbidden {
					t.Fatalf("rendered the forbidden state %q — %s", forbidden, testCase.why)
				}
			}

			// 4. Code is inside the closed value-safe code vocabulary.
			if !closedCode(section.Code) {
				t.Fatalf("rendered code %q is outside the closed code vocabulary; it is neither a synthetic code the production validator accepts nor one of the derived projection codes %v", section.Code, projectionCodes)
			}
			if section.Code != testCase.wantCode {
				t.Fatalf("rendered code %q; want %q — the closed code is what distinguishes recovery actions that share a state", section.Code, testCase.wantCode)
			}

			// 5. The evidence reference is the constant, so the wire
			// shape cannot carry a derived identifier or target.
			if section.EvidenceRef != graphsynthetic.AggregateEvidenceRef {
				t.Fatalf("rendered evidence reference %q; it MUST be the constant %q", section.EvidenceRef, graphsynthetic.AggregateEvidenceRef)
			}

			// 6. The State/Ready pairing is EXCLUSIVE and unambiguous.
			// Each clause is asserted as a rule over the rendered values
			// rather than only against the case's expectation, so a case
			// whose expectation was written wrong still cannot pass.
			if section.Ready && !available {
				t.Fatalf("rendered ready=true with state %q; ready is claimable ONLY with %q — %s", section.State, graphsynthetic.AggregateAvailable, testCase.why)
			}
			if section.Ready && !enabled {
				t.Fatalf("rendered ready=true with activation %q; a policy that is not enabled is never ready — %s", section.Activation, testCase.why)
			}
			if want := available && enabled; section.Ready != want {
				t.Fatalf("rendered ready=%v for activation %q + state %q; the exclusive pairing requires ready=%v", section.Ready, section.Activation, section.State, want)
			}
			if section.Ready != testCase.wantReady {
				t.Fatalf("rendered ready=%v; want %v — %s", section.Ready, testCase.wantReady, testCase.why)
			}

			// 7. The three observation-derived fields travel together. A
			// half-populated section would let a surface show an
			// observation instant for rows it never received.
			if (section.ObservedAt != nil) != testCase.wantObservation {
				t.Fatalf("observed-at present=%v; want %v — the observation instant must appear only alongside the rows it describes", section.ObservedAt != nil, testCase.wantObservation)
			}
			if (section.DurationMs != nil) != testCase.wantObservation {
				t.Fatalf("duration present=%v; want %v — the observation duration must appear only alongside the rows it describes", section.DurationMs != nil, testCase.wantObservation)
			}

			// 8. Family rows are DERIVED from the canonical manifest, in
			// canonical order, and every one carries only closed
			// family/state/code values with a derived evidence reference
			// — enforced by the SAME validator production publishes with.
			if !testCase.wantObservation {
				if len(section.Families) != 0 {
					t.Fatalf("rendered %d family rows with no published observation; a projection must never invent rows it did not observe", len(section.Families))
				}
			} else {
				if len(section.Families) != len(required) {
					t.Fatalf("rendered %d family rows for %d canonical families", len(section.Families), len(required))
				}
				for i, want := range required {
					if section.Families[i].Family != want {
						t.Fatalf("family row %d is %q; the canonical manifest order requires %q", i, section.Families[i].Family, want)
					}
				}
				for _, row := range section.Families {
					if err := row.Validate(); err != nil {
						t.Fatalf("rendered family row %q carries a value outside the closed per-family vocabulary: %v", row.Family, err)
					}
				}
			}

			reached[graphsynthetic.AggregateState(section.State)] = true
			rendered++
			if section.Ready {
				readyTrue++
			} else {
				readyFalse++
			}
			if wired {
				wiredRendered++
			} else {
				unwiredRendered++
			}
		})
	}

	if rendered != len(graphSectionCases) {
		t.Fatalf("anti-vacuity: %d of %d cases rendered a projection; a skipped case cannot prove exclusivity", rendered, len(graphSectionCases))
	}
	for _, state := range closedStates {
		if !reached[state] {
			t.Fatalf("anti-vacuity: no case ever rendered the closed state %q; a projection that can never reach it would still satisfy every assertion above", state)
		}
	}
	if readyTrue == 0 {
		t.Fatal("anti-vacuity: no case rendered ready=true; every exclusivity assertion would hold on a projection hardwired to not-ready")
	}
	if readyFalse == 0 {
		t.Fatal("anti-vacuity: no case rendered ready=false; every exclusivity assertion would hold on a projection hardwired to ready")
	}
	if unwiredRendered == 0 {
		t.Fatal("anti-vacuity: no case rendered an UNWIRED projection; the fail-closed wiring rule never evaluated, so a projection that answered fail-OPEN with nothing wired would satisfy every assertion above")
	}
	if wiredRendered == 0 {
		t.Fatal("anti-vacuity: no case rendered a WIRED projection; the wiring oracle would then be answering \"unwired\" unconditionally rather than discriminating, and the rule above would be binding every case's shape by accident")
	}

	t.Run("closed_vocabularies_reject_invented_values", func(t *testing.T) {
		assertClosedVocabulariesRejectInventedValues(t, projectionCodes)
	})
	t.Run("ready_is_exclusive_to_the_available_state_across_the_whole_vocabulary", func(t *testing.T) {
		assertReadyExclusiveAcrossEveryClosedState(t, closedStates)
	})
	t.Run("disabled_policy_is_never_ready_for_any_published_state", func(t *testing.T) {
		assertDisabledPolicyNeverReady(t, closedStates)
	})
}

// assertClosedVocabulariesRejectInventedValues proves the membership
// oracles used above are not hollow. Every oracle must REJECT a value
// outside its vocabulary and ACCEPT a genuine one; without both halves,
// a validator that passed everything — or refused everything — would
// leave the membership assertions proving nothing.
func assertClosedVocabulariesRejectInventedValues(t *testing.T, projectionCodes []string) {
	t.Helper()

	families := graphapi.RequiredGraphFamilies()
	if len(families) == 0 {
		t.Fatal("anti-vacuity: the canonical manifest is empty; the per-family rejections below would inspect nothing")
	}

	inventedStates := []string{"ready", "empty", "partial", "healthy", "loading", ""}
	if len(inventedStates) == 0 {
		t.Fatal("anti-vacuity: no invented aggregate states to reject")
	}
	for _, invented := range inventedStates {
		if closedAggregateState(invented) {
			t.Fatalf("the production validator accepted the invented aggregate state %q; the vocabulary is not closed and every state assertion above is meaningless", invented)
		}
	}
	if !closedAggregateState(string(graphsynthetic.AggregateAvailable)) {
		t.Fatal("control: the production validator rejected a genuine closed aggregate state; the rejections above would then be about the validator, not the state name")
	}

	inventedActivations := []string{"maybe", "enabling", "on", "off", ""}
	for _, invented := range inventedActivations {
		if closedActivation(invented) {
			t.Fatalf("the production validator accepted the invented activation state %q; the activation vocabulary is not closed", invented)
		}
	}
	if !closedActivation(string(graphapi.ActivationEnabled)) {
		t.Fatal("control: the production validator rejected a genuine closed activation state")
	}

	inventedCodes := []string{"OKAY", "F080-SYNTH-DOES-NOT-EXIST", "error", ""}
	for _, invented := range inventedCodes {
		if closedSyntheticCode(invented) {
			t.Fatalf("the production validator accepted the invented diagnostic code %q; the code vocabulary is not closed", invented)
		}
		if slices.Contains(projectionCodes, invented) {
			t.Fatalf("the derived projection-code set contains the invented code %q", invented)
		}
	}
	if !closedSyntheticCode(graphsynthetic.CodeOK) {
		t.Fatal("control: the production validator rejected a genuine closed diagnostic code")
	}

	// The per-family vocabulary is closed by the SAME validator the
	// rendered rows are checked with, so an invented read state cannot
	// reach a surface through the Families array either.
	inventedReadStates := []graphsynthetic.ReadState{"empty", "ready", "error", "loading", ""}
	for _, invented := range inventedReadStates {
		row := graphsynthetic.GraphFamilyResult{
			Family:      families[0],
			State:       invented,
			DurationMs:  1,
			Code:        graphsynthetic.CodeOK,
			EvidenceRef: graphsynthetic.EvidenceRef(families[0]),
		}
		if err := row.Validate(); err == nil {
			t.Fatalf("a family row accepted the invented read state %q; the per-family vocabulary is not closed", invented)
		}
	}
	control := graphsynthetic.GraphFamilyResult{
		Family:      families[0],
		State:       graphsynthetic.StatePopulated,
		DurationMs:  1,
		Code:        graphsynthetic.CodeOK,
		EvidenceRef: graphsynthetic.EvidenceRef(families[0]),
	}
	if err := control.Validate(); err != nil {
		t.Fatalf("control: a family row carrying only closed values was rejected: %v; every per-family rejection above would then prove nothing", err)
	}
}

// assertReadyExclusiveAcrossEveryClosedState publishes a fresh
// observation carrying EACH closed aggregate state in turn under an
// ENABLED policy and proves Ready tracks the available state and
// nothing else.
//
// Sweeping the DERIVED vocabulary is what makes the pairing exhaustive
// rather than incidental: a fifth aggregate state added upstream is
// swept here the moment it joins AggregateStates(), so a new state
// cannot arrive carrying an accidental ready claim.
func assertReadyExclusiveAcrossEveryClosedState(t *testing.T, closedStates []graphsynthetic.AggregateState) {
	t.Helper()
	if len(closedStates) < 2 {
		t.Fatalf("anti-vacuity: %d closed state(s) available; exclusivity cannot be demonstrated below two", len(closedStates))
	}

	swept, readyStates := 0, 0
	for _, state := range closedStates {
		readiness := graphSectionReadiness(t, graphSectionEnabledCapability(t))
		graphSectionPublish(t, readiness, graphSectionAggregate(t,
			graphapi.ActivationEnabled, state, graphsynthetic.CodeOK,
			graphSectionFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK),
			graphSectionFreshAge))

		section := readiness.Snapshot()
		if got := graphsynthetic.AggregateState(section.State); got != state {
			t.Fatalf("a fresh observation carrying state %q rendered state %q; the projection must publish the observed state, never a re-derived one", state, got)
		}
		if want := state == graphsynthetic.AggregateAvailable; section.Ready != want {
			t.Fatalf("a fresh observation carrying state %q rendered ready=%v; ready is claimable ONLY with %q", state, section.Ready, graphsynthetic.AggregateAvailable)
		}
		if section.Ready {
			readyStates++
		}
		swept++
	}

	if swept != len(closedStates) {
		t.Fatalf("anti-vacuity: swept %d of %d closed states", swept, len(closedStates))
	}
	if readyStates != 1 {
		t.Fatalf("anti-vacuity: %d closed states rendered ready=true; EXACTLY one — the available state — may", readyStates)
	}
}

// assertDisabledPolicyNeverReady proves an explicitly DISABLED policy
// outranks every observation. It sweeps the derived state vocabulary
// through a disabled projection, then confirms the reason the sweep can
// only publish disabled-activation observations: an ENABLED-activation
// observation is REFUSED outright rather than quietly becoming product
// truth.
func assertDisabledPolicyNeverReady(t *testing.T, closedStates []graphsynthetic.AggregateState) {
	t.Helper()
	if len(closedStates) == 0 {
		t.Fatal("anti-vacuity: no closed states to sweep through a disabled policy")
	}

	swept := 0
	for _, state := range closedStates {
		readiness := graphSectionReadiness(t, graphSectionDisabledCapability(t))
		graphSectionPublish(t, readiness, graphSectionAggregate(t,
			graphapi.ActivationDisabled, state, graphsynthetic.CodeOK,
			graphSectionFamilyRows(graphsynthetic.StateDisabled, graphsynthetic.CodePolicyDisabled),
			graphSectionFreshAge))

		section := readiness.Snapshot()
		if section.Ready {
			t.Fatalf("a DISABLED policy rendered ready=true after an observation carrying state %q; a disabled capability is never ready", state)
		}
		if got := graphsynthetic.AggregateState(section.State); got != graphsynthetic.AggregatePolicyDisabled {
			t.Fatalf("a DISABLED policy rendered state %q after an observation carrying %q; the explicit policy must win over any observation", got, state)
		}
		swept++
	}
	if swept != len(closedStates) {
		t.Fatalf("anti-vacuity: swept %d of %d closed states through the disabled policy", swept, len(closedStates))
	}

	readiness := graphSectionReadiness(t, graphSectionDisabledCapability(t))
	contradiction := graphSectionAggregate(t,
		graphapi.ActivationEnabled, graphsynthetic.AggregateAvailable, graphsynthetic.CodeOK,
		graphSectionFamilyRows(graphsynthetic.StatePopulated, graphsynthetic.CodeOK),
		graphSectionFreshAge)
	if err := readiness.Publish(contradiction); err == nil {
		t.Fatal("a DISABLED policy accepted an ENABLED-activation observation; the sweep above would then be projecting a shape production never stores")
	}
}
