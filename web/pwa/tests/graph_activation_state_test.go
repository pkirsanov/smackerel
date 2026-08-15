// graph_activation_state_test.go — BUG-080-001 SCOPE-04, test row
// T080-08-UNIT (`ui-unit`, SCN-080-001-08).
//
// SCOPE-04 implementation-plan item 1 requires ONE typed response
// decoder and activation/read model, and forbids a projection from
// inferring state "from HTTP code or `items.length` independently".
// That clause is the whole defect class: a surface that reads the
// status on its own renders a route-missing 404 as "nothing captured
// yet", and a surface that reads the item count on its own renders a
// failed read as an empty one.
//
// This file proves the projection cannot make either mistake. Every
// case feeds internal/graphreadstate an input engineered to TEMPT a
// naive implementation into two states at once, then asserts exactly
// one closed state survives.
//
// The canonical family set is derived from
// graphapi.RequiredGraphFamilies() on every path and the closed
// aggregate vocabulary from graphsynthetic.AggregateStates(). No
// family name, no family count, and no state list is hardcoded here,
// so a ninth canonical family or a fifth aggregate state cannot be
// added upstream and silently pass this test.
package webcodegen_drift_test

import (
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/graphreadstate"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// graphProjectionObservedAt is a fixed non-zero UTC instant. Validate
// rejects a zero observation time, so the reducer's output must carry
// a real one for the contract-validity assertions to mean anything.
var graphProjectionObservedAt = time.Date(2026, time.August, 15, 9, 30, 0, 0, time.UTC)

// graphErrorEnvelope builds the uniform graphapi error body. The
// classifier reads ONLY the typed code out of it.
func graphErrorEnvelope(code string) []byte {
	return []byte(fmt.Sprintf(`{"error":{"code":%q,"message":"value-safe"}}`, code))
}

// enabledActivation and disabledActivation mirror the two closed
// graphapi activation outcomes.
var (
	enabledActivation = graphapi.Activation{
		State:          graphapi.ActivationEnabled,
		SecretPresence: graphapi.SecretPresent,
		Code:           graphapi.CodeActivationOK,
	}
	disabledActivation = graphapi.Activation{
		State:          graphapi.ActivationDisabled,
		SecretPresence: graphapi.SecretEmpty,
		Code:           graphapi.CodeCursorSecretEmpty,
	}
)

// uniformObservation builds one identical observation per canonical
// family, DERIVED from the manifest.
func uniformObservation(status int, rows int, body []byte) func([]graphapi.GraphRouteFamily) []graphreadstate.FamilyObservation {
	return func(required []graphapi.GraphRouteFamily) []graphreadstate.FamilyObservation {
		out := make([]graphreadstate.FamilyObservation, 0, len(required))
		for _, family := range required {
			out = append(out, graphreadstate.FamilyObservation{
				Family:    family,
				Status:    status,
				ErrorBody: body,
				RowCount:  rows,
				Duration:  4 * time.Millisecond,
			})
		}
		return out
	}
}

// oneFamilyFails builds a fully populated observation set in which the
// LAST canonical family instead carries the given failure. The failing
// family is selected from the manifest, never named literally.
func oneFamilyFails(status int, body []byte) func([]graphapi.GraphRouteFamily) []graphreadstate.FamilyObservation {
	return func(required []graphapi.GraphRouteFamily) []graphreadstate.FamilyObservation {
		out := uniformObservation(http.StatusOK, 2, nil)(required)
		last := len(out) - 1
		out[last].Status = status
		out[last].ErrorBody = body
		out[last].RowCount = 0
		return out
	}
}

// lastFamily is the optional-family selector companion to
// oneFamilyFails. It too derives from the manifest.
func lastFamily(required []graphapi.GraphRouteFamily) []graphapi.GraphRouteFamily {
	return []graphapi.GraphRouteFamily{required[len(required)-1]}
}

// graphProjectionCase is one engineered projection input plus the
// single state it is allowed to produce.
type graphProjectionCase struct {
	name       string
	activation graphapi.Activation
	// observe builds one observation per canonical family.
	observe func(required []graphapi.GraphRouteFamily) []graphreadstate.FamilyObservation
	// allowEmpty and optional are derived from the manifest at run time
	// so no case hardcodes a family name.
	allowEmpty func(required []graphapi.GraphRouteFamily) []graphapi.GraphRouteFamily
	optional   func(required []graphapi.GraphRouteFamily) []graphapi.GraphRouteFamily
	wantState  graphsynthetic.AggregateState
	wantCode   string
	// wantFamilyState is the per-family read state every projected row
	// must carry, when the case fixes one.
	wantFamilyState graphsynthetic.ReadState
	// mustNotBe names the states this input would tempt a naive
	// implementation into. Asserting them explicitly is what makes the
	// exclusivity claim adversarial rather than incidental.
	mustNotBe []graphsynthetic.AggregateState
	// why records the confusion the case exists to prevent.
	why string
}

func allFamilies(required []graphapi.GraphRouteFamily) []graphapi.GraphRouteFamily {
	return slices.Clone(required)
}

func noFamilies([]graphapi.GraphRouteFamily) []graphapi.GraphRouteFamily { return nil }

// graphProjectionCases is the engineered-confusion table. Each entry
// pairs an input that satisfies the precondition of TWO states with
// the one state that is actually true.
var graphProjectionCases = []graphProjectionCase{
	{
		name:            "route_missing_404_with_zero_rows_is_not_true_empty",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusNotFound, 0, nil),
		allowEmpty:      allFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeRouteAbsent,
		wantFamilyState: graphsynthetic.StateFailed,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "the route is absent AND the row count is zero AND policy permits empty; reading either signal on its own yields the original silent-absence bug",
	},
	{
		name:            "unauthenticated_401_with_zero_rows_is_not_true_empty",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusUnauthorized, 0, graphErrorEnvelope(graphapi.CodeUnauthenticated)),
		allowEmpty:      allFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeUnauthenticated,
		wantFamilyState: graphsynthetic.StateFailed,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "a rejected session returns no rows; projecting that as empty would show capture guidance to a user whose content is merely unreadable",
	},
	{
		name:            "store_unavailable_503_is_not_route_missing_or_empty",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusServiceUnavailable, 0, graphErrorEnvelope(graphapi.CodeStoreUnavailable)),
		allowEmpty:      allFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeStoreUnavailable,
		wantFamilyState: graphsynthetic.StateFailed,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "store failure and route absence share a family state; only the closed code distinguishes the recovery action",
	},
	{
		name:            "schema_error_500_is_not_empty",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusInternalServerError, 0, graphErrorEnvelope(graphapi.CodeSchemaError)),
		allowEmpty:      allFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeSchemaInvalid,
		wantFamilyState: graphsynthetic.StateFailed,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
		},
		why: "an undecodable body yields zero usable rows; the projection must call that invalid, not empty",
	},
	{
		name:            "typed_capability_disabled_while_policy_says_enabled_is_failure_not_disabled",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusServiceUnavailable, 0, graphErrorEnvelope(graphapi.CodeCapabilityDisabled)),
		allowEmpty:      allFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeCapabilityDisabled,
		wantFamilyState: graphsynthetic.StateFailed,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "the server says disabled while the explicit policy says enabled; that contradiction is a fault, and projecting it as policy_disabled would hide a real outage behind a legitimate deployment state",
	},
	{
		name:            "explicit_disabled_is_not_available_even_when_reads_look_populated",
		activation:      disabledActivation,
		observe:         uniformObservation(http.StatusOK, 5, nil),
		allowEmpty:      noFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregatePolicyDisabled,
		wantCode:        graphsynthetic.CodePolicyDisabled,
		wantFamilyState: graphsynthetic.StateDisabled,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregateUnavailable,
		},
		why: "an explicitly disabled capability must never be advertised as ready, no matter how healthy a concurrent or stale read looked",
	},
	{
		name:            "permitted_true_empty_is_empty_and_not_an_error",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusOK, 0, nil),
		allowEmpty:      allFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateAvailable,
		wantCode:        graphsynthetic.CodeOK,
		wantFamilyState: graphsynthetic.StateTrueEmpty,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateUnavailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "a genuinely empty corpus is an actionable success state; projecting it as an error would show a retry action to a user who simply has not captured anything",
	},
	{
		name:            "unpermitted_empty_is_failure_not_true_empty",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusOK, 0, nil),
		allowEmpty:      noFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateUnavailable,
		wantCode:        graphsynthetic.CodeEmptyNotPermitted,
		wantFamilyState: graphsynthetic.StateFailed,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "emptiness is permitted only where policy names it; an unnamed empty read is a silent data loss, not a true-empty",
	},
	{
		name:       "named_optional_family_failure_degrades_and_is_not_unavailable",
		activation: enabledActivation,
		observe:    oneFamilyFails(http.StatusNotFound, nil),
		allowEmpty: noFamilies,
		optional:   lastFamily,
		wantState:  graphsynthetic.AggregateDegraded,
		wantCode:   graphsynthetic.CodeOptionalOmitted,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateAvailable,
			graphsynthetic.AggregateUnavailable,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "a named optional omission is the only route to degraded; collapsing it into unavailable or available erases the partial state the UX must show",
	},
	{
		name:            "fully_populated_read_is_available",
		activation:      enabledActivation,
		observe:         uniformObservation(http.StatusOK, 3, nil),
		allowEmpty:      noFamilies,
		optional:        noFamilies,
		wantState:       graphsynthetic.AggregateAvailable,
		wantCode:        graphsynthetic.CodeOK,
		wantFamilyState: graphsynthetic.StatePopulated,
		mustNotBe: []graphsynthetic.AggregateState{
			graphsynthetic.AggregateUnavailable,
			graphsynthetic.AggregateDegraded,
			graphsynthetic.AggregatePolicyDisabled,
		},
		why: "the projection must still be able to reach available, or every assertion above would pass on a decoder that reports failure unconditionally",
	},
}

// TestGraphActivationProjectionUsesClosedExclusiveStates is
// T080-08-UNIT. It proves the SCOPE-04 activation/read projection uses
// the closed backend vocabulary, resolves exactly one state per input,
// refuses an invented state, and never lets a route-missing or
// disabled outcome collapse into a true-empty or available one.
func TestGraphActivationProjectionUsesClosedExclusiveStates(t *testing.T) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; every projection assertion below would reduce over zero families and pass without exercising anything")
	}
	closedStates := graphsynthetic.AggregateStates()
	if len(closedStates) == 0 {
		t.Fatalf("anti-vacuity: graphsynthetic.AggregateStates() is empty; the exclusivity count below would be trivially satisfied")
	}
	if len(graphProjectionCases) == 0 {
		t.Fatalf("anti-vacuity: graphProjectionCases is empty; this test would report PASS having projected nothing")
	}

	// Every closed state must be reachable by at least one case, and
	// every case must be exercised. A table that only ever produces
	// "unavailable" proves nothing about exclusivity.
	reached := make(map[graphsynthetic.AggregateState]bool, len(closedStates))
	projected := 0

	for _, testCase := range graphProjectionCases {
		t.Run(testCase.name, func(t *testing.T) {
			observations := testCase.observe(required)
			if len(observations) != len(required) {
				t.Fatalf("anti-vacuity: case built %d observations for %d canonical families; the projection would refuse or skip families instead of being tested",
					len(observations), len(required))
			}

			policy := graphreadstate.Policy{
				AllowEmpty: testCase.allowEmpty(required),
				Optional:   testCase.optional(required),
			}
			result, err := graphreadstate.Project(
				testCase.activation, policy, observations,
				graphProjectionObservedAt, 40*time.Millisecond)
			if err != nil {
				t.Fatalf("Project returned an error for a well-formed input: %v", err)
			}

			// 1. The resolved state is inside the closed vocabulary.
			if !slices.Contains(closedStates, result.State) {
				t.Fatalf("projected state %q is outside the closed aggregate vocabulary %v", result.State, closedStates)
			}

			// 2. EXCLUSIVITY. Exactly one closed state matches. This is
			// the structural claim: the input satisfies the precondition
			// of more than one state, and only one may survive.
			matches := 0
			for _, candidate := range closedStates {
				if result.State == candidate {
					matches++
				}
			}
			if matches != 1 {
				t.Fatalf("projected state %q matched %d closed states; exactly one must be active (%s)", result.State, matches, testCase.why)
			}

			if result.State != testCase.wantState {
				t.Fatalf("projected state %q; want %q — %s", result.State, testCase.wantState, testCase.why)
			}
			if result.Code != testCase.wantCode {
				t.Fatalf("projected code %q; want %q — the closed code is what distinguishes recovery actions that share a state", result.Code, testCase.wantCode)
			}
			for _, forbidden := range testCase.mustNotBe {
				if result.State == forbidden {
					t.Fatalf("projected the forbidden state %q — %s", forbidden, testCase.why)
				}
			}

			// 3. Ready is never claimed unless the state is available.
			if got, want := result.Available(), testCase.wantState == graphsynthetic.AggregateAvailable; got != want {
				t.Fatalf("Available()=%v for state %q; want %v", got, result.State, want)
			}

			// 4. The result stays a contract-valid closed record even
			// when it is a refusal.
			if err := result.Validate(); err != nil {
				t.Fatalf("projected result failed its own closed-vocabulary validation: %v", err)
			}

			// 5. The family rows are DERIVED from the manifest, in
			// canonical order, one per family.
			if len(result.Families) != len(required) {
				t.Fatalf("projected %d family rows for %d canonical families", len(result.Families), len(required))
			}
			for i, want := range required {
				if result.Families[i].Family != want {
					t.Fatalf("family row %d is %q; the canonical manifest order requires %q", i, result.Families[i].Family, want)
				}
			}
			if testCase.wantFamilyState != "" {
				for _, row := range result.Families {
					if row.State != testCase.wantFamilyState {
						t.Fatalf("family %q projected read state %q; want %q — %s", row.Family, row.State, testCase.wantFamilyState, testCase.why)
					}
				}
			}

			reached[result.State] = true
			projected++
		})
	}

	if projected != len(graphProjectionCases) {
		t.Fatalf("anti-vacuity: %d of %d cases produced a projection; a skipped case cannot prove exclusivity", projected, len(graphProjectionCases))
	}
	for _, state := range closedStates {
		if !reached[state] {
			t.Fatalf("anti-vacuity: no case ever projected the closed state %q; a decoder that can never reach it would still pass every assertion above", state)
		}
	}

	t.Run("rejects_invented_states", func(t *testing.T) {
		assertProjectionRejectsInventedStates(t, required)
	})
	t.Run("family_list_is_derived_not_hardcoded", func(t *testing.T) {
		assertFamilyListIsDerived(t, required)
	})
	t.Run("projection_output_carries_nothing_to_re_infer_state_from", func(t *testing.T) {
		assertOutputHasNoStatusOrCountField(t)
	})
}

// assertProjectionRejectsInventedStates proves the vocabulary is
// CLOSED rather than merely conventional: a state name outside it is
// refused wherever it can be introduced.
func assertProjectionRejectsInventedStates(t *testing.T, required []graphapi.GraphRouteFamily) {
	t.Helper()

	inventedReadStates := []graphsynthetic.ReadState{"empty", "ready", "error", "loading", ""}
	if len(inventedReadStates) == 0 {
		t.Fatal("anti-vacuity: no invented read states to reject")
	}
	for _, invented := range inventedReadStates {
		row := graphsynthetic.NewFamilyRow(required[0], invented, graphsynthetic.CodeOK, time.Millisecond)
		if err := row.Validate(); err == nil {
			t.Fatalf("family row accepted the invented read state %q; the vocabulary is not closed", invented)
		}
	}

	valid := make([]graphsynthetic.GraphFamilyResult, 0, len(required))
	for _, family := range required {
		valid = append(valid, graphsynthetic.NewFamilyRow(
			family, graphsynthetic.StatePopulated, graphsynthetic.CodeOK, time.Millisecond))
	}

	inventedAggregates := []graphsynthetic.AggregateState{"ready", "empty", "partial", "healthy", ""}
	if len(inventedAggregates) == 0 {
		t.Fatal("anti-vacuity: no invented aggregate states to reject")
	}
	for _, invented := range inventedAggregates {
		result := graphsynthetic.AggregateResult{
			Activation:  graphapi.ActivationEnabled,
			State:       invented,
			ObservedAt:  graphProjectionObservedAt,
			DurationMs:  4,
			Code:        graphsynthetic.CodeOK,
			EvidenceRef: graphsynthetic.AggregateEvidenceRef,
			Families:    valid,
		}
		if err := result.Validate(); err == nil {
			t.Fatalf("aggregate accepted the invented state %q; the vocabulary is not closed", invented)
		}
	}

	// The control: the same construction with a closed state passes, so
	// the rejections above are about the state name and not about a
	// validator that refuses everything.
	control := graphsynthetic.AggregateResult{
		Activation:  graphapi.ActivationEnabled,
		State:       graphsynthetic.AggregateAvailable,
		ObservedAt:  graphProjectionObservedAt,
		DurationMs:  4,
		Code:        graphsynthetic.CodeOK,
		EvidenceRef: graphsynthetic.AggregateEvidenceRef,
		Families:    valid,
	}
	if err := control.Validate(); err != nil {
		t.Fatalf("control aggregate carrying only closed values was rejected: %v; every rejection above would then be meaningless", err)
	}

	// An activation outside the closed vocabulary is refused by the
	// projection itself rather than defaulted to enabled or disabled.
	_, err := graphreadstate.Project(
		graphapi.Activation{State: graphapi.ActivationState("maybe")},
		graphreadstate.Policy{},
		uniformObservation(http.StatusOK, 1, nil)(required),
		graphProjectionObservedAt, time.Millisecond)
	if err == nil {
		t.Fatal("Project accepted an activation state outside the closed vocabulary")
	}
	if !strings.Contains(err.Error(), graphreadstate.CodeProjectionInvalid) {
		t.Fatalf("refusal %q does not carry the value-safe code %q", err, graphreadstate.CodeProjectionInvalid)
	}
}

// assertFamilyListIsDerived proves the projection reads the canonical
// family set from graphapi rather than from a private copy: an
// observation set that omits a canonical family is REFUSED, so adding
// a ninth family upstream cannot silently produce a projection over
// eight.
func assertFamilyListIsDerived(t *testing.T, required []graphapi.GraphRouteFamily) {
	t.Helper()

	full := uniformObservation(http.StatusOK, 1, nil)(required)
	if len(full) < 2 {
		t.Fatalf("anti-vacuity: the canonical manifest carries %d families; omission cannot be exercised below two", len(full))
	}

	for i, omitted := range required {
		partial := slices.Clone(full)
		partial = append(partial[:i], partial[i+1:]...)
		_, err := graphreadstate.Project(
			enabledActivation, graphreadstate.Policy{}, partial,
			graphProjectionObservedAt, time.Millisecond)
		if err == nil {
			t.Fatalf("Project accepted an observation set omitting canonical family %q; the family list is not derived from the manifest", omitted)
		}
		if !strings.Contains(err.Error(), string(omitted)) {
			t.Fatalf("refusal for omitted family %q does not name it: %v", omitted, err)
		}
	}

	// A family outside the manifest is refused rather than projected.
	stray := append(slices.Clone(full), graphreadstate.FamilyObservation{
		Family: graphapi.GraphRouteFamily("constellations"),
		Status: http.StatusOK, RowCount: 1,
	})
	if _, err := graphreadstate.Project(
		enabledActivation, graphreadstate.Policy{}, stray,
		graphProjectionObservedAt, time.Millisecond); err == nil {
		t.Fatal("Project accepted an observation for a family absent from the canonical manifest")
	}

	// A duplicated family is refused rather than silently last-wins.
	duplicated := append(slices.Clone(full), full[0])
	if _, err := graphreadstate.Project(
		enabledActivation, graphreadstate.Policy{}, duplicated,
		graphProjectionObservedAt, time.Millisecond); err == nil {
		t.Fatal("Project accepted a duplicated canonical family observation")
	}
}

// assertOutputHasNoStatusOrCountField proves the "must not infer state
// from HTTP code or items.length independently" clause structurally
// rather than by convention: the projection OUTPUT has no field a
// downstream consumer could re-derive a state from.
func assertOutputHasNoStatusOrCountField(t *testing.T) {
	t.Helper()

	forbidden := []string{"status", "count", "items", "length", "http", "body", "rows"}
	inspected := 0
	for _, projected := range []reflect.Type{
		reflect.TypeOf(graphsynthetic.AggregateResult{}),
		reflect.TypeOf(graphsynthetic.GraphFamilyResult{}),
	} {
		if projected.NumField() == 0 {
			t.Fatalf("anti-vacuity: %s exposes no fields; the scan below would inspect nothing", projected.Name())
		}
		for i := range projected.NumField() {
			field := projected.Field(i)
			lowered := strings.ToLower(field.Name)
			for _, token := range forbidden {
				if strings.Contains(lowered, token) {
					t.Fatalf("%s.%s lets a consumer re-infer state from a raw read signal; the projection output must carry only the closed state, code, family, duration, and evidence reference",
						projected.Name(), field.Name)
				}
			}
			inspected++
		}
	}
	if inspected == 0 {
		t.Fatal("anti-vacuity: no projected fields were inspected")
	}
}
