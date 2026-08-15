// Package graphreadstate is the single typed activation/read state
// projection for BUG-080-001 SCOPE-04 implementation-plan item 1.
//
// # Why this package exists
//
// Before it, every graph consumer was free to answer "what state am I
// in?" for itself. A surface that asks the HTTP status independently
// renders a route-missing 404 as "nothing here yet"; a surface that
// asks `items.length` independently renders a failed read as an empty
// one; a surface that asks neither advertises a DISABLED capability as
// ready. Those three independent inferences are the silent-absence
// class BUG-080-001 exists to eliminate.
//
// Project is therefore the ONLY decoder. It takes RAW per-family
// observations and returns the closed
// graphsynthetic.AggregateResult — the SAME type, the SAME closed
// vocabularies, and the SAME reducer readiness and the SCOPE-03
// synthetic already use. There is no second state vocabulary here to
// drift out of step.
//
// # Exclusivity is structural, not conventional
//
//   - The result carries exactly ONE State field, so "empty AND
//     error" is unrepresentable.
//   - The gate order in decode is the contract: transport, then HTTP
//     status, then — and only then — row count. A non-200 outcome
//     never reaches the emptiness branch, so a route-missing 404 with
//     zero rows cannot be projected as a permitted true-empty.
//   - An explicit DISABLED activation short-circuits before any
//     observation is consulted, so a disabled deployment cannot be
//     projected as available by a stale or optimistic read.
//   - The returned rows carry no HTTP status and no row count, so a
//     downstream consumer has nothing left to re-infer state from.
//
// The canonical family list is derived from
// graphapi.RequiredGraphFamilies() on every path. No family name and
// no family count is hardcoded, so a ninth canonical family cannot be
// added to the manifest and silently omitted from a projection.
package graphreadstate

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// CodeProjectionInvalid is the value-safe refusal code for an input a
// projection cannot honestly reduce. It names only the condition class
// and the canonical family; a node id, a label, a query value, a
// cursor body, a credential, or a target host never appears in it.
const CodeProjectionInvalid = "F080-PROJECTION-INVALID"

// FamilyObservation is the RAW, UNDECODED outcome of one canonical
// family read. It is projection INPUT only: the fields a consumer must
// not reason about independently live here and are consumed by decode,
// never re-published.
type FamilyObservation struct {
	// Family is the canonical graph route family that was read.
	Family graphapi.GraphRouteFamily
	// TransportFailed reports that no HTTP status was ever observed —
	// a request-build, dial, timeout, or body-read failure.
	TransportFailed bool
	// Status is the observed HTTP status code. It is meaningful only
	// when TransportFailed is false.
	Status int
	// ErrorBody is the observed response body. It is read ONLY for its
	// typed graphapi error code, never for its message or field.
	ErrorBody []byte
	// RowCount is the number of contract-valid rows decoded from a
	// successful read. It is consulted ONLY after the status gate.
	RowCount int
	// Duration is the observed read duration.
	Duration time.Duration
}

// Policy names the explicit emptiness and optionality allowances the
// projection honors. Both are opt-in by name: a family not named in
// AllowEmpty that reads zero rows is a FAILURE, and a family not named
// in Optional that fails makes the aggregate unavailable. The
// projection never infers that an empty or missing read is acceptable.
type Policy struct {
	// AllowEmpty names the families whose zero-row read is a
	// policy-permitted true-empty.
	AllowEmpty []graphapi.GraphRouteFamily
	// Optional names the families whose failure degrades rather than
	// invalidates the aggregate.
	Optional []graphapi.GraphRouteFamily
}

// Project reduces the explicit activation policy plus one raw
// observation per canonical family to exactly one closed
// graphsynthetic.AggregateResult.
//
// It refuses rather than guesses: an activation outside the closed
// vocabulary, an observation naming a non-canonical family, a
// duplicated family, a missing canonical family, or a self-contradictory
// observation returns a typed value-safe error and NO result. An
// absent read is never quietly projected as empty or available.
func Project(
	activation graphapi.Activation,
	policy Policy,
	observations []FamilyObservation,
	observedAt time.Time,
	total time.Duration,
) (graphsynthetic.AggregateResult, error) {
	required := graphapi.RequiredGraphFamilies()
	if len(required) == 0 {
		return graphsynthetic.AggregateResult{}, fmt.Errorf(
			"%s: the canonical graph family manifest is empty; a projection over zero families would be vacuously available",
			CodeProjectionInvalid)
	}

	switch activation.State {
	case graphapi.ActivationEnabled, graphapi.ActivationDisabled:
	default:
		return graphsynthetic.AggregateResult{}, fmt.Errorf(
			"%s: activation state %q is outside the closed activation vocabulary",
			CodeProjectionInvalid, activation.State)
	}

	rows := make([]graphsynthetic.GraphFamilyResult, 0, len(required))

	if activation.Disabled() {
		// An explicit DISABLED policy means no read was attempted, so no
		// observation may influence the outcome. This short-circuit is
		// what stops a disabled deployment being advertised as ready.
		for _, family := range required {
			rows = append(rows, graphsynthetic.NewFamilyRow(
				family, graphsynthetic.StateDisabled, graphsynthetic.CodePolicyDisabled, 0))
		}
	} else {
		byFamily := make(map[graphapi.GraphRouteFamily]FamilyObservation, len(observations))
		for _, observation := range observations {
			if !slices.Contains(required, observation.Family) {
				return graphsynthetic.AggregateResult{}, fmt.Errorf(
					"%s: observation names family %q, which is not a canonical graph route family",
					CodeProjectionInvalid, observation.Family)
			}
			if _, duplicate := byFamily[observation.Family]; duplicate {
				return graphsynthetic.AggregateResult{}, fmt.Errorf(
					"%s: family %q is observed more than once; a projection reduces exactly one observation per canonical family",
					CodeProjectionInvalid, observation.Family)
			}
			if err := validateObservation(observation); err != nil {
				return graphsynthetic.AggregateResult{}, err
			}
			byFamily[observation.Family] = observation
		}

		for _, family := range required {
			observation, observed := byFamily[family]
			if !observed {
				return graphsynthetic.AggregateResult{}, fmt.Errorf(
					"%s: canonical family %q carries no observation; an unobserved read is never projected as empty or available",
					CodeProjectionInvalid, family)
			}
			rows = append(rows, decode(policy, observation))
		}
	}

	for _, row := range rows {
		if err := row.Validate(); err != nil {
			return graphsynthetic.AggregateResult{}, err
		}
	}

	result := graphsynthetic.Aggregate(activation.State, policy.Optional, rows, observedAt, total)
	if err := result.Validate(); err != nil {
		return graphsynthetic.AggregateResult{}, err
	}
	return result, nil
}

// validateObservation refuses a self-contradictory raw observation.
// Coercing one into a plausible state is how a defect becomes an
// "empty" screen, so each of these fails loudly instead.
func validateObservation(observation FamilyObservation) error {
	if observation.RowCount < 0 {
		return fmt.Errorf(
			"%s: family %q carries a negative row count; a projection never coerces an impossible count into an empty read",
			CodeProjectionInvalid, observation.Family)
	}
	if observation.Duration < 0 {
		return fmt.Errorf(
			"%s: family %q carries a negative read duration",
			CodeProjectionInvalid, observation.Family)
	}
	if !observation.TransportFailed && observation.Status < 100 {
		return fmt.Errorf(
			"%s: family %q reports no transport failure but carries no observed HTTP status",
			CodeProjectionInvalid, observation.Family)
	}
	return nil
}

// decode reduces ONE raw observation to ONE closed family row.
//
// The gate order is the contract, not a stylistic choice. Row count is
// consulted only in the HTTP 200 arm, so an outcome that never
// returned 200 — a route-missing 404, a 401, a store-unavailable 503 —
// cannot reach the emptiness branch no matter how many rows it
// carried or how permissive the emptiness policy is.
func decode(policy Policy, observation FamilyObservation) graphsynthetic.GraphFamilyResult {
	switch {
	case observation.TransportFailed:
		return graphsynthetic.NewFamilyRow(observation.Family,
			graphsynthetic.StateFailed, graphsynthetic.CodeTransport, observation.Duration)
	case observation.Status != http.StatusOK:
		return graphsynthetic.NewFamilyRow(observation.Family,
			graphsynthetic.StateFailed,
			graphsynthetic.ClassifyHTTPOutcome(observation.Status, observation.ErrorBody),
			observation.Duration)
	case observation.RowCount > 0:
		return graphsynthetic.NewFamilyRow(observation.Family,
			graphsynthetic.StatePopulated, graphsynthetic.CodeOK, observation.Duration)
	case slices.Contains(policy.AllowEmpty, observation.Family):
		return graphsynthetic.NewFamilyRow(observation.Family,
			graphsynthetic.StateTrueEmpty, graphsynthetic.CodeEmptyPermitted, observation.Duration)
	default:
		return graphsynthetic.NewFamilyRow(observation.Family,
			graphsynthetic.StateFailed, graphsynthetic.CodeEmptyNotPermitted, observation.Duration)
	}
}
