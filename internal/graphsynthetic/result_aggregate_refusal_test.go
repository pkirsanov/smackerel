package graphsynthetic

// result_aggregate_refusal_test.go — BUG-080-001 SCOPE-03 (SCN-080-001-03),
// REFUSAL arm, Layer 1.
//
// The acceptance claim has two halves. That the synthetic EXECUTES its
// fixed family sequence and emits one value-safe row per family plus one
// aggregate is proven elsewhere. This file proves the other half: that an
// aggregate carrying a 401, 403, 404, 5xx, schema, cursor, or missing-row
// outcome on a REQUIRED family is REFUSED.
//
// Exercising those codes as telemetry inputs proves only that they are
// reportable. Refusal is a different claim, and this is where it is made.
//
// Four properties per failure class, asserted against the production
// reducer with no mock, stub, or test double of any kind:
//
//  1. Available() is false.
//  2. State is AggregateUnavailable.
//  3. Code is the FAILING FAMILY'S OWN code — the specific cause
//     propagates and is never flattened into a generic value.
//  4. Validate() still passes — a refusal must remain a contract-valid,
//     closed-vocabulary result, not a malformed one.
//
// The family set is derived from graphapi.RequiredGraphFamilies() on every
// path. No family list and no family count is hardcoded anywhere in this
// file, so adding a ninth canonical family cannot silently pass it.

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// aggregateRefusalCases is the SCN-080-001-03 outcome-class list, each
// mapped to the closed diagnostic code the synthetic assigns it.
var aggregateRefusalCases = []struct {
	name string
	code string
}{
	{name: "401_unauthenticated", code: CodeUnauthenticated},
	{name: "403_forbidden", code: CodeForbidden},
	{name: "404_route_absent", code: CodeRouteAbsent},
	{name: "5xx_server_error", code: CodeServerError},
	{name: "schema_invalid", code: CodeSchemaInvalid},
	{name: "cursor_invalid", code: CodeCursorInvalid},
	{name: "row_missing", code: CodeRowMissing},
}

// aggregateTestObservedAt is a fixed non-zero UTC instant. Validate
// rejects a zero observation time, so the reducer's output must carry a
// real one for the contract-validity assertions to mean anything.
var aggregateTestObservedAt = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

// canonicalPopulatedRows builds one contract-valid populated row per
// canonical family, in canonical order, DERIVED from the manifest.
func canonicalPopulatedRows(t *testing.T) []GraphFamilyResult {
	t.Helper()
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; every aggregate assertion in this file would be vacuous")
	}
	rows := make([]GraphFamilyResult, 0, len(required))
	for _, family := range required {
		rows = append(rows, newFamilyResult(family, StatePopulated, CodeOK, 3*time.Millisecond))
	}
	return rows
}

// TestAggregateRefusesRequiredFamilyFailure sweeps every failure class
// across every canonical family position. Position matters: a reducer that
// only refused the first family, or that flattened the cause into a
// generic code, would pass a single-position test and fail this one.
func TestAggregateRefusesRequiredFamilyFailure(t *testing.T) {
	if len(aggregateRefusalCases) == 0 {
		t.Fatalf("anti-vacuity: the refusal case table is empty; this test would assert nothing")
	}
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; the refusal sweep would assert nothing")
	}

	asserted := 0
	for _, tc := range aggregateRefusalCases {
		for i, failing := range required {
			t.Run(tc.name+"/"+string(failing), func(t *testing.T) {
				rows := canonicalPopulatedRows(t)
				rows[i] = newFamilyResult(failing, StateFailed, tc.code, 7*time.Millisecond)

				// OptionalFamilies is nil: nothing is excused, so the only
				// reachable verdict for a failed family is refusal.
				agg := aggregate(graphapi.ActivationEnabled, nil, rows, aggregateTestObservedAt, 40*time.Millisecond)

				if agg.Available() {
					t.Fatalf("Available() = true after REQUIRED family %q failed with %s; a failed required family MUST refuse the aggregate", failing, tc.code)
				}
				if agg.State != AggregateUnavailable {
					t.Fatalf("State = %q; want %q after REQUIRED family %q failed with %s", agg.State, AggregateUnavailable, failing, tc.code)
				}
				if agg.Code != tc.code {
					t.Fatalf("Code = %q; want %q — the failing family's OWN cause MUST propagate to the aggregate and never be flattened into a generic code", agg.Code, tc.code)
				}
				if err := agg.Validate(); err != nil {
					t.Fatalf("the refused aggregate failed its own closed-vocabulary contract: %v; a refusal MUST remain publishable, not malformed", err)
				}

				if got := agg.Families[i]; got.State != StateFailed || got.Code != tc.code {
					t.Fatalf("aggregate family row %d = (state %q, code %q); want (%q, %q) — the refusal MUST carry the failing row as its evidence",
						i, got.State, got.Code, StateFailed, tc.code)
				}
				asserted++
			})
		}
	}

	if want := len(aggregateRefusalCases) * len(required); asserted != want {
		t.Fatalf("anti-vacuity: %d of %d refusal combinations executed; the sweep did not assert what it claims", asserted, want)
	}
}

// TestAggregateAvailableRequiresContractValidReads is the positive
// control. Without it, a reducer that refused unconditionally would pass
// every assertion above while being useless.
func TestAggregateAvailableRequiresContractValidReads(t *testing.T) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; the availability assertions would be vacuous")
	}

	cases := []struct {
		name  string
		state ReadState
		code  string
	}{
		{name: "every_required_family_populated", state: StatePopulated, code: CodeOK},
		{name: "every_required_family_permitted_true_empty", state: StateTrueEmpty, code: CodeEmptyPermitted},
	}
	if len(cases) == 0 {
		t.Fatalf("anti-vacuity: the availability case table is empty; this test would assert nothing")
	}

	executed := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows := make([]GraphFamilyResult, 0, len(required))
			for _, family := range required {
				rows = append(rows, newFamilyResult(family, tc.state, tc.code, 2*time.Millisecond))
			}

			agg := aggregate(graphapi.ActivationEnabled, nil, rows, aggregateTestObservedAt, 20*time.Millisecond)

			if !agg.Available() {
				t.Fatalf("Available() = false with every required family in state %q; a contract-valid read set MUST reach available", tc.state)
			}
			if agg.State != AggregateAvailable {
				t.Fatalf("State = %q; want %q", agg.State, AggregateAvailable)
			}
			if agg.Code != CodeOK {
				t.Fatalf("Code = %q; want %q for an available aggregate", agg.Code, CodeOK)
			}
			if err := agg.Validate(); err != nil {
				t.Fatalf("available aggregate failed its own closed-vocabulary contract: %v", err)
			}
			executed++
		})
	}

	if executed != len(cases) {
		t.Fatalf("anti-vacuity: %d of %d availability cases executed", executed, len(cases))
	}
}

// TestAggregateRefusesMissingRequiredFamilyRow proves an ABSENT canonical
// row refuses with CodeFamilyMissing rather than passing on a short set.
//
// Such an aggregate is additionally rejected by Validate, because its row
// set violates the canonical row-count contract. Both behaviours are
// asserted: the reducer names the cause for its caller, and the
// structurally incomplete result is never publishable.
func TestAggregateRefusesMissingRequiredFamilyRow(t *testing.T) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) < 2 {
		t.Fatalf("anti-vacuity: the canonical manifest has %d families; omitting one cannot be exercised", len(required))
	}

	executed := 0
	for i, omitted := range required {
		t.Run(string(omitted), func(t *testing.T) {
			full := canonicalPopulatedRows(t)
			rows := make([]GraphFamilyResult, 0, len(full)-1)
			rows = append(rows, full[:i]...)
			rows = append(rows, full[i+1:]...)
			if len(rows) != len(required)-1 {
				t.Fatalf("anti-vacuity: built %d rows omitting %q; want %d", len(rows), omitted, len(required)-1)
			}

			agg := aggregate(graphapi.ActivationEnabled, nil, rows, aggregateTestObservedAt, 20*time.Millisecond)

			if agg.Available() {
				t.Fatalf("Available() = true with the %q row absent; a missing canonical row MUST refuse the aggregate", omitted)
			}
			if agg.State != AggregateUnavailable {
				t.Fatalf("State = %q; want %q with the %q row absent", agg.State, AggregateUnavailable, omitted)
			}
			if agg.Code != CodeFamilyMissing {
				t.Fatalf("Code = %q; want %q with the %q row absent", agg.Code, CodeFamilyMissing, omitted)
			}
			if err := agg.Validate(); err == nil {
				t.Fatalf("Validate() = nil for an aggregate missing the %q row; an incomplete canonical family set MUST never be publishable", omitted)
			}
			executed++
		})
	}

	if executed != len(required) {
		t.Fatalf("anti-vacuity: %d of %d omission cases executed", executed, len(required))
	}
}

// TestAggregateDegradesOnlyForExplicitlyNamedOptionalFamily proves the
// refusal above is DISCRIMINATING rather than a blanket rule: the same
// failed family degrades instead of refusing when, and only when, the
// configuration explicitly names it optional. Degraded is still not
// available.
func TestAggregateDegradesOnlyForExplicitlyNamedOptionalFamily(t *testing.T) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		t.Fatalf("anti-vacuity: graphapi.RequiredGraphFamilies() is empty; the optional-family assertions would be vacuous")
	}

	executed := 0
	for i, family := range required {
		t.Run(string(family), func(t *testing.T) {
			rows := canonicalPopulatedRows(t)
			rows[i] = newFamilyResult(family, StateFailed, CodeServerError, 5*time.Millisecond)

			agg := aggregate(
				graphapi.ActivationEnabled,
				[]graphapi.GraphRouteFamily{family},
				rows,
				aggregateTestObservedAt,
				25*time.Millisecond,
			)

			if agg.State != AggregateDegraded {
				t.Fatalf("State = %q; want %q when the explicitly-named optional family %q failed", agg.State, AggregateDegraded, family)
			}
			if agg.Code != CodeOptionalOmitted {
				t.Fatalf("Code = %q; want %q for a named optional omission", agg.Code, CodeOptionalOmitted)
			}
			if agg.Available() {
				t.Fatalf("Available() = true for a degraded aggregate; only %q is available", AggregateAvailable)
			}
			if err := agg.Validate(); err != nil {
				t.Fatalf("degraded aggregate failed its own closed-vocabulary contract: %v", err)
			}
			executed++
		})
	}

	if executed != len(required) {
		t.Fatalf("anti-vacuity: %d of %d optional-family cases executed", executed, len(required))
	}
}
