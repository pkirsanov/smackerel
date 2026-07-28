// Package graphsynthetic implements the BUG-080-001 SCOPE-03
// product-owned Knowledge Graph read synthetic and the value-safe
// result contract that authenticated health, strict readiness, and
// capability status are derived from.
//
// The synthetic is READ-ONLY by construction: it issues GET requests
// only, against the canonical eight-family graph route manifest
// (internal/api/graphapi), over PRODUCTION HTTP behavior using a real
// scoped session. It performs no create, update, delete, refresh, or
// sync, and it never mutates a graph record.
//
// # Value safety (SCN-080-001-07)
//
// A GraphFamilyResult and an AggregateResult are CLOSED structs. They
// carry a fixed canonical family name, a closed safe state, a bounded
// duration, a closed diagnostic code, and an evidence reference that is
// DERIVED FROM THE FAMILY NAME ALONE. They structurally cannot carry a
// node id, a topic/person/place label, a query value, a cursor body, a
// bearer token, secret material, or a target host — there is no field
// to put one in, and Validate rejects any value outside the closed
// vocabularies before the result is published, logged, or emitted as a
// metric label.
package graphsynthetic

import (
	"fmt"
	"slices"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// ReadState is the closed per-family read outcome vocabulary.
type ReadState string

const (
	// StatePopulated — an authorized, schema-valid read that returned at
	// least one contract-valid row.
	StatePopulated ReadState = "populated"
	// StateTrueEmpty — an authorized, schema-valid read that returned
	// zero rows for a family the configuration explicitly permits to be
	// empty. An unpermitted empty read is StateFailed, never this.
	StateTrueEmpty ReadState = "true_empty"
	// StateFailed — any 401, 403, 404, 5xx, schema, cursor, missing-row,
	// transport, or unpermitted-empty outcome.
	StateFailed ReadState = "failed"
	// StateDisabled — the explicit activation policy resolved DISABLED,
	// so no read was attempted for this family.
	StateDisabled ReadState = "disabled"
)

// AggregateState is the closed aggregate outcome vocabulary.
type AggregateState string

const (
	// AggregateAvailable — every REQUIRED family returned a contract-valid
	// populated or permitted true-empty read.
	AggregateAvailable AggregateState = "available"
	// AggregateDegraded — every REQUIRED family is contract-valid but at
	// least one family the configuration explicitly names as OPTIONAL
	// failed. A named optional omission is the ONLY route to degraded.
	AggregateDegraded AggregateState = "degraded"
	// AggregateUnavailable — at least one REQUIRED family failed, or a
	// required family row is absent from the result set.
	AggregateUnavailable AggregateState = "unavailable"
	// AggregatePolicyDisabled — the explicit activation policy resolved
	// DISABLED. This is a truthful non-ready result and a valid
	// deployment state, never a fault and never a boot refusal.
	AggregatePolicyDisabled AggregateState = "policy_disabled"
)

// Closed, value-safe synthetic diagnostic codes. A code names ONLY the
// condition class. A secret value, its length, its hash, a node id, a
// label, a query value, a cursor body, or a target host NEVER appears
// in a code.
const (
	// CodeOK marks a contract-valid populated read or an available
	// aggregate.
	CodeOK = "OK"
	// CodeEmptyPermitted marks a zero-row read for a family the
	// configuration explicitly permits to be empty.
	CodeEmptyPermitted = "F080-SYNTH-EMPTY-PERMITTED"
	// CodeEmptyNotPermitted marks a zero-row read for a family the
	// configuration does NOT permit to be empty.
	CodeEmptyNotPermitted = "F080-SYNTH-EMPTY-NOT-PERMITTED"
	// CodeUnauthenticated marks an HTTP 401 outcome.
	CodeUnauthenticated = "F080-SYNTH-UNAUTHENTICATED"
	// CodeForbidden marks an HTTP 403 outcome (missing graph read grant).
	CodeForbidden = "F080-SYNTH-FORBIDDEN"
	// CodeRouteAbsent marks an HTTP 404 outcome — the silent-absence
	// class this bug fix exists to eliminate.
	CodeRouteAbsent = "F080-SYNTH-ROUTE-ABSENT"
	// CodeCapabilityDisabled marks a typed 503 capability_disabled
	// response observed while the explicit activation policy said
	// ENABLED. That contradiction is a failure, not a disabled state.
	CodeCapabilityDisabled = "F080-SYNTH-CAPABILITY-DISABLED"
	// CodeStoreUnavailable marks a typed 503 store_unavailable response.
	CodeStoreUnavailable = "F080-SYNTH-STORE-UNAVAILABLE"
	// CodeServerError marks any other 5xx outcome.
	CodeServerError = "F080-SYNTH-SERVER-ERROR"
	// CodeSchemaInvalid marks an undecodable or contract-invalid body.
	CodeSchemaInvalid = "F080-SYNTH-SCHEMA-INVALID"
	// CodeCursorInvalid marks an unreadable or rejected pagination
	// cursor.
	CodeCursorInvalid = "F080-SYNTH-CURSOR-INVALID"
	// CodeRowMissing marks a detail read whose seeded row was absent
	// while its list family was populated.
	CodeRowMissing = "F080-SYNTH-ROW-MISSING"
	// CodeTransport marks a request-build, dial, timeout, or body-read
	// failure. It names the class only, never the target.
	CodeTransport = "F080-SYNTH-TRANSPORT"
	// CodeUnexpectedStatus marks any status outside the closed handled
	// set.
	CodeUnexpectedStatus = "F080-SYNTH-UNEXPECTED-STATUS"
	// CodePolicyDisabled marks the aggregate reached when the explicit
	// activation policy resolved DISABLED.
	CodePolicyDisabled = "F080-SYNTH-POLICY-DISABLED"
	// CodeFamilyMissing marks an aggregate whose result set omitted a
	// canonical family row.
	CodeFamilyMissing = "F080-SYNTH-FAMILY-MISSING"
	// CodeOptionalOmitted marks a degraded aggregate produced by an
	// explicitly named optional-family omission.
	CodeOptionalOmitted = "F080-SYNTH-OPTIONAL-OMITTED"
)

// readStates is the closed ReadState vocabulary used by Validate.
var readStates = []ReadState{StatePopulated, StateTrueEmpty, StateFailed, StateDisabled}

// aggregateStates is the closed AggregateState vocabulary. It is also
// the full key space of the one-hot synthetic-result gauge.
var aggregateStates = []AggregateState{
	AggregateAvailable, AggregateDegraded, AggregateUnavailable, AggregatePolicyDisabled,
}

// syntheticCodes is the closed diagnostic-code vocabulary used by
// Validate. A code outside this set is rejected before publication.
var syntheticCodes = []string{
	CodeOK, CodeEmptyPermitted, CodeEmptyNotPermitted, CodeUnauthenticated,
	CodeForbidden, CodeRouteAbsent, CodeCapabilityDisabled, CodeStoreUnavailable,
	CodeServerError, CodeSchemaInvalid, CodeCursorInvalid, CodeRowMissing,
	CodeTransport, CodeUnexpectedStatus, CodePolicyDisabled, CodeFamilyMissing,
	CodeOptionalOmitted,
}

// AggregateStates returns the closed aggregate-state vocabulary. It is
// the key space the one-hot synthetic-result gauge zeroes before
// setting the current state, so a stale series can never be mistaken
// for current truth.
func AggregateStates() []AggregateState { return slices.Clone(aggregateStates) }

// EvidenceRef returns the value-safe evidence reference for a family.
// It is derived from the CANONICAL FAMILY NAME ALONE, so an evidence
// reference structurally cannot carry an id, a label, a query value, a
// cursor body, or a target host.
func EvidenceRef(family graphapi.GraphRouteFamily) string {
	return "graph-read/" + string(family)
}

// AggregateEvidenceRef is the value-safe evidence reference for the
// aggregate observation. It is a constant.
const AggregateEvidenceRef = "graph-read/aggregate"

// GraphFamilyResult is the CLOSED, value-safe result of one canonical
// family read. Its wire shape is exactly design.md §"Synthetic
// Contract":
//
//	{"family":"topics","state":"populated","durationMs":12,
//	 "code":"OK","evidenceRef":"graph-read/topics"}
//
// There is deliberately NO field for a label, an id, a query value, a
// cursor body, a credential, secret material, a target host, a row
// count, or a raw error.
type GraphFamilyResult struct {
	// Family is a canonical graph route family name.
	Family graphapi.GraphRouteFamily `json:"family"`
	// State is the closed per-family read outcome.
	State ReadState `json:"state"`
	// DurationMs is the bounded observed read duration in milliseconds.
	DurationMs int64 `json:"durationMs"`
	// Code is a closed value-safe diagnostic code.
	Code string `json:"code"`
	// EvidenceRef is derived from Family alone.
	EvidenceRef string `json:"evidenceRef"`
}

// newFamilyResult builds a validated family row. The evidence reference
// is always derived from the family, never supplied by a caller.
func newFamilyResult(family graphapi.GraphRouteFamily, state ReadState, code string, d time.Duration) GraphFamilyResult {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return GraphFamilyResult{
		Family:      family,
		State:       state,
		DurationMs:  ms,
		Code:        code,
		EvidenceRef: EvidenceRef(family),
	}
}

// Validate enforces the closed vocabularies and the derived evidence
// reference. A result that fails Validate is never published, logged,
// or emitted as a metric label.
func (r GraphFamilyResult) Validate() error {
	if !slices.Contains(graphapi.RequiredGraphFamilies(), r.Family) {
		return fmt.Errorf("graphsynthetic: family %q is not a canonical graph route family", r.Family)
	}
	if !slices.Contains(readStates, r.State) {
		return fmt.Errorf("graphsynthetic: family %q carries state %q outside the closed read-state vocabulary", r.Family, r.State)
	}
	if !slices.Contains(syntheticCodes, r.Code) {
		return fmt.Errorf("graphsynthetic: family %q carries code %q outside the closed diagnostic-code vocabulary", r.Family, r.Code)
	}
	if r.DurationMs < 0 {
		return fmt.Errorf("graphsynthetic: family %q carries a negative duration", r.Family)
	}
	if want := EvidenceRef(r.Family); r.EvidenceRef != want {
		return fmt.Errorf("graphsynthetic: family %q carries evidence reference %q; it MUST be derived from the family name as %q", r.Family, r.EvidenceRef, want)
	}
	return nil
}

// AggregateResult is the CLOSED, value-safe aggregate observation the
// product publishes for authenticated health, strict readiness, and
// capability status. Like GraphFamilyResult it has no field capable of
// carrying content.
type AggregateResult struct {
	// Activation is the explicit activation policy that governed this
	// observation.
	Activation graphapi.ActivationState `json:"activation"`
	// State is the closed aggregate outcome.
	State AggregateState `json:"state"`
	// ObservedAt is the UTC instant the observation completed.
	ObservedAt time.Time `json:"observedAt"`
	// DurationMs is the bounded total observation duration.
	DurationMs int64 `json:"durationMs"`
	// Code is a closed value-safe diagnostic code.
	Code string `json:"code"`
	// EvidenceRef is the constant aggregate evidence reference.
	EvidenceRef string `json:"evidenceRef"`
	// Families holds exactly one row per canonical family, in canonical
	// order.
	Families []GraphFamilyResult `json:"families"`
}

// Available reports whether the aggregate proves the graph journey is
// serving contract-valid authorized reads.
func (a AggregateResult) Available() bool { return a.State == AggregateAvailable }

// Validate enforces the closed vocabularies, the constant aggregate
// evidence reference, and one validated row per canonical family in
// canonical order.
func (a AggregateResult) Validate() error {
	switch a.Activation {
	case graphapi.ActivationEnabled, graphapi.ActivationDisabled:
	default:
		return fmt.Errorf("graphsynthetic: aggregate carries activation %q outside the closed activation vocabulary", a.Activation)
	}
	if !slices.Contains(aggregateStates, a.State) {
		return fmt.Errorf("graphsynthetic: aggregate carries state %q outside the closed aggregate-state vocabulary", a.State)
	}
	if !slices.Contains(syntheticCodes, a.Code) {
		return fmt.Errorf("graphsynthetic: aggregate carries code %q outside the closed diagnostic-code vocabulary", a.Code)
	}
	if a.EvidenceRef != AggregateEvidenceRef {
		return fmt.Errorf("graphsynthetic: aggregate carries evidence reference %q; it MUST be the constant %q", a.EvidenceRef, AggregateEvidenceRef)
	}
	if a.DurationMs < 0 {
		return fmt.Errorf("graphsynthetic: aggregate carries a negative duration")
	}
	if a.ObservedAt.IsZero() {
		return fmt.Errorf("graphsynthetic: aggregate carries a zero observation time")
	}
	required := graphapi.RequiredGraphFamilies()
	if len(a.Families) != len(required) {
		return fmt.Errorf("graphsynthetic: aggregate carries %d family rows; the canonical manifest requires exactly %d", len(a.Families), len(required))
	}
	for i, want := range required {
		if a.Families[i].Family != want {
			return fmt.Errorf("graphsynthetic: aggregate family row %d is %q; the canonical order requires %q", i, a.Families[i].Family, want)
		}
		if err := a.Families[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// aggregate reduces a canonical-order family result set plus the
// explicit activation policy to the closed aggregate outcome.
//
// Reduction rules (design.md §"Synthetic Contract"):
//
//   - activation DISABLED                       -> policy_disabled
//   - a canonical family row is absent           -> unavailable
//   - a REQUIRED family failed                   -> unavailable
//   - only explicitly-OPTIONAL families failed   -> degraded
//   - every required family populated/true-empty -> available
//
// The aggregate can therefore become available ONLY from contract-valid
// populated or explicitly-permitted empty reads.
func aggregate(
	activation graphapi.ActivationState,
	optional []graphapi.GraphRouteFamily,
	rows []GraphFamilyResult,
	observedAt time.Time,
	total time.Duration,
) AggregateResult {
	ms := total.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	out := AggregateResult{
		Activation:  activation,
		ObservedAt:  observedAt.UTC(),
		DurationMs:  ms,
		EvidenceRef: AggregateEvidenceRef,
		Families:    rows,
	}

	if activation == graphapi.ActivationDisabled {
		out.State = AggregatePolicyDisabled
		out.Code = CodePolicyDisabled
		return out
	}

	byFamily := make(map[graphapi.GraphRouteFamily]GraphFamilyResult, len(rows))
	for _, row := range rows {
		byFamily[row.Family] = row
	}

	degraded := false
	for _, family := range graphapi.RequiredGraphFamilies() {
		row, ok := byFamily[family]
		if !ok {
			out.State = AggregateUnavailable
			out.Code = CodeFamilyMissing
			return out
		}
		if row.State == StatePopulated || row.State == StateTrueEmpty {
			continue
		}
		if slices.Contains(optional, family) {
			degraded = true
			continue
		}
		out.State = AggregateUnavailable
		out.Code = row.Code
		return out
	}

	if degraded {
		out.State = AggregateDegraded
		out.Code = CodeOptionalOmitted
		return out
	}
	out.State = AggregateAvailable
	out.Code = CodeOK
	return out
}
