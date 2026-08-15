package graphsynthetic

// projection.go — the exported reduction seam required by BUG-080-001
// SCOPE-04 implementation-plan item 1: "Add one typed response decoder
// and activation/read model consumed by Wiki Browse, Graph
// availability, and readiness; projections must not infer state from
// HTTP code or `items.length` independently."
//
// Three surfaces reading the same graph must never be able to DISAGREE
// about what a read meant. The only way to guarantee that structurally
// is for them to share ONE reducer, not three that happen to be
// written alike today. So this file exports thin wrappers over the
// SAME unexported row builder, status classifier, and aggregate reducer
// the SCOPE-03 synthetic already runs.
//
// It deliberately adds NO new state name, NO new diagnostic code, and
// NO second reduction rule. A parallel vocabulary is exactly how a
// route-missing 404 becomes a "true empty" on one surface while
// readiness still calls it unavailable.

import (
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// NewFamilyRow builds one closed, value-safe family row. The evidence
// reference is DERIVED from the family name alone — a caller cannot
// supply one — so a projected row structurally cannot carry a node id,
// a label, a query value, a cursor body, or a target host.
func NewFamilyRow(family graphapi.GraphRouteFamily, state ReadState, code string, d time.Duration) GraphFamilyResult {
	return newFamilyResult(family, state, code, d)
}

// ClassifyHTTPOutcome maps one observed non-200 HTTP outcome to a
// closed value-safe diagnostic code, reading ONLY the typed graphapi
// error code out of the envelope. It is the SAME classifier the
// synthetic uses, so a 404 cannot be read as route-absent by readiness
// and as something else by the Knowledge surface.
func ClassifyHTTPOutcome(status int, body []byte) string {
	return classifyStatus(status, body)
}

// Aggregate reduces the explicit activation policy plus a
// canonical-order family row set to EXACTLY ONE closed aggregate
// state. It is the single reduction in the repository; readiness, the
// synthetic, and any UI projection all resolve through this one call.
func Aggregate(
	activation graphapi.ActivationState,
	optional []graphapi.GraphRouteFamily,
	rows []GraphFamilyResult,
	observedAt time.Time,
	total time.Duration,
) AggregateResult {
	return aggregate(activation, optional, rows, observedAt, total)
}
