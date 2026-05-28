// Spec 061 SCOPE-04 — three-band post-processor.
//
// Borderline is the ONE net-new routing knob added by spec 061. It
// post-processes an agent.RoutingDecision returned by spec 037
// agent.Router.Route and classifies it into one of three bands:
//
//   - High       : decision.OK && TopScore >= borderlineFloor
//                  → facade proceeds to executor.
//   - Borderline : decision.OK && agentConfidenceFloor <= TopScore < borderlineFloor
//                  → facade emits DisambiguationPrompt; executor NOT called.
//   - Low        : !decision.OK || TopScore < agentConfidenceFloor
//                  → facade emits CaptureRoute=true; executor NOT called.
//
// borderlineFloor MUST be strictly greater than agentConfidenceFloor
// (validated at SCOPE-01 startup; this function does NOT re-validate
// to keep it pure for stress / hot-path use).
//
// Purity contract:
//   - No I/O, no allocations beyond the Band return.
//   - Safe for concurrent use.
//   - Deterministic over (decision, ok, borderlineFloor, agentConfidenceFloor).
//
// Source of truth: design.md §3.2.

package assistant

import "github.com/smackerel/smackerel/internal/agent"

// Band is the closed-vocabulary three-band classification for the
// borderline post-processor. Source of truth: design.md §3.2.
type Band string

const (
	// BandHigh — decision.OK && TopScore >= borderlineFloor.
	// Facade proceeds to executor.
	BandHigh Band = "high"

	// BandBorderline — decision.OK && agentConfidenceFloor <= TopScore
	// < borderlineFloor. Facade emits a DisambiguationPrompt; executor
	// is NOT invoked.
	BandBorderline Band = "borderline"

	// BandLow — !decision.OK || TopScore < agentConfidenceFloor.
	// Facade emits CaptureRoute=true; executor is NOT invoked.
	BandLow Band = "low"
)

// AllBands is the exhaustive closed-vocabulary list. borderline_test.go
// asserts that every Band literal declared in this file appears here
// exactly once.
var AllBands = []Band{BandHigh, BandBorderline, BandLow}

// Borderline classifies a routing decision into one of three bands.
// See package doc for the band definitions.
//
// The ok parameter is the third return value of agent.Router.Route —
// passed explicitly because RoutingDecision does NOT carry it as a
// field (the router signature exposes ok separately so this function
// must too, in order to avoid false-High classifications when the
// router itself reported unknown-intent with a non-zero TopScore from
// the considered list).
//
// Edge cases (design §3.2):
//   - TopScore == borderlineFloor       → BandHigh        (boundary inclusive at top)
//   - TopScore == agentConfidenceFloor  → BandBorderline  (boundary inclusive at bottom of borderline)
//   - decision.Reason == ReasonUnknownIntent → BandLow    (regardless of TopScore)
//   - !ok                                → BandLow        (regardless of TopScore)
func Borderline(decision agent.RoutingDecision, ok bool, borderlineFloor, agentConfidenceFloor float64) Band {
	if !ok {
		return BandLow
	}
	if decision.Reason == agent.ReasonUnknownIntent {
		return BandLow
	}
	if decision.TopScore < agentConfidenceFloor {
		return BandLow
	}
	if decision.TopScore < borderlineFloor {
		return BandBorderline
	}
	return BandHigh
}
