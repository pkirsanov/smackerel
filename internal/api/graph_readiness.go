package api

// graph_readiness.go — BUG-080-001 SCOPE-03 product wiring: the ONE
// place the Knowledge Graph read synthetic (internal/graphsynthetic)
// becomes product truth.
//
// It answers exactly two product questions and nothing else:
//
//	1. What is the AUTHENTICATED Graph capability detail on
//	   GET /api/health?  (Snapshot -> HealthResponse.Graph)
//	2. Is the Graph journey STRICTLY ready?
//	   (Snapshot().Ready -> GET /readyz?strict=true)
//
// Both answers are derived from EXACTLY two inputs — the explicit
// activation policy (graphapi.GraphCapability) and a published,
// validated graphsynthetic.AggregateResult. Static Wiki assets, route
// presence, general database liveness, and process uptime are
// STRUCTURALLY incapable of producing a ready claim here: there is no
// code path that sets Ready from anything but AggregateResult.Available
// under an ENABLED policy (SCN-080-001-04).
//
// Value safety (SCN-080-001-07): the projection carries ONLY the closed
// activation state, the closed aggregate state, a closed diagnostic
// code, the derived evidence reference, a bounded duration, an
// observation instant, and the synthetic's own closed per-family rows.
// Every one of those values comes from a closed vocabulary that
// graphsynthetic.Validate already enforced. There is no field capable
// of carrying a node id, a topic/person/place label, a query value, a
// cursor body, a credential, secret material, or a target host.

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/graphsynthetic"
)

// Closed, value-safe readiness-projection diagnostic codes. They name
// ONLY the condition class of the PROJECTION itself — the states the
// synthetic's own closed code vocabulary cannot express because they
// describe the absence or the age of an observation rather than the
// outcome of one. A secret value, its length, its hash, a node id, a
// label, a query value, a cursor body, or a target host NEVER appears
// in a code.
const (
	// GraphReadinessCodeNotObserved marks an ENABLED activation policy
	// for which NO synthetic observation has ever been published. It is
	// the fail-closed starting state: route presence alone never
	// promotes the Graph journey to ready.
	GraphReadinessCodeNotObserved = "F080-READINESS-NOT-OBSERVED"
	// GraphReadinessCodeStale marks an ENABLED activation policy whose
	// most recent published observation is older than the freshness
	// bound. A once-available observation must never keep asserting a
	// ready Graph journey indefinitely.
	GraphReadinessCodeStale = "F080-READINESS-STALE"
	// GraphReadinessCodeActivationMismatch marks a publication REFUSED
	// because the observation's activation contradicts the current
	// explicit activation policy.
	GraphReadinessCodeActivationMismatch = "F080-READINESS-ACTIVATION-MISMATCH"
	// GraphReadinessCodeConfigInvalid marks a fail-loud construction
	// refusal. It names only the offending field and the contract.
	GraphReadinessCodeConfigInvalid = "F080-READINESS-CONFIG-INVALID"
)

// GraphObservationMaxAge is the product-owned freshness bound for a
// published synthetic observation. It is a CODE-LEVEL contract, not a
// runtime default: NewGraphReadiness REQUIRES the bound as an explicit
// argument and refuses a non-positive value, so there is no
// silently-substituted fallback anywhere on this path. The value is
// sized so a periodic validate-plane observation cadence measured in
// minutes keeps readiness fresh, while a publisher that has stopped
// reporting demotes the Graph journey to unavailable rather than
// leaving a stale ready claim standing.
const GraphObservationMaxAge = 10 * time.Minute

// GraphHealthSection is the value-safe Knowledge Graph capability
// projection. It is emitted ONLY to authenticated callers of
// GET /api/health (HealthResponse.Graph) — the unauthenticated health
// response omits it entirely, so capability detail cannot be used for
// infrastructure reconnaissance (CWE-200), exactly as the pre-existing
// Services/Version/CommitHash/Knowledge sections already are.
type GraphHealthSection struct {
	// Ready is the strict Graph-journey readiness claim. It is true
	// ONLY when the explicit activation policy is ENABLED and a fresh
	// published synthetic aggregate reports available.
	Ready bool `json:"ready"`
	// Activation is the closed explicit activation policy state.
	Activation string `json:"activation"`
	// State is the closed aggregate outcome the product publishes.
	State string `json:"state"`
	// Code is a closed value-safe diagnostic code — either the
	// synthetic's own aggregate code or a projection code above.
	Code string `json:"code"`
	// EvidenceRef is the constant aggregate evidence reference.
	EvidenceRef string `json:"evidence_ref"`
	// ObservedAt is the instant the most recent published observation
	// completed. Omitted when no observation has been published.
	ObservedAt *time.Time `json:"observed_at,omitempty"`
	// DurationMs is the bounded duration of that observation. Omitted
	// when no observation has been published.
	DurationMs *int64 `json:"duration_ms,omitempty"`
	// Families carries the synthetic's own closed per-family rows in
	// canonical order, in the wire shape design.md pins for the
	// synthetic contract. Omitted when no observation has been
	// published.
	Families []graphsynthetic.GraphFamilyResult `json:"families,omitempty"`
}

// GraphReadiness holds the explicit activation policy and the most
// recent validated synthetic observation, and derives the product
// readiness projection from those two inputs alone.
//
// It implements graphsynthetic.Observer, so a synthetic run publishes
// into product readiness by composing it into the observer chain
// (graphsynthetic.MultiObserver{telemetryObserver, graphReadiness})
// with no additional adapter. Publish remains the explicit,
// error-returning seam for a caller that owns the observation outside
// an in-process run; concrete credential injection and target
// acceptance for such a caller remain operator-owned and are not
// decided here.
type GraphReadiness struct {
	capability *graphapi.GraphCapability
	maxAge     time.Duration
	now        func() time.Time

	mu       sync.RWMutex
	observed *graphsynthetic.AggregateResult
}

// GraphReadiness is a synthetic observer: an in-process run publishes
// straight into product readiness through the existing Observer seam.
var _ graphsynthetic.Observer = (*GraphReadiness)(nil)

// NewGraphReadiness returns the readiness projection for an explicit
// activation policy. It is FAIL-LOUD: a nil capability or a
// non-positive freshness bound is refused with a consolidated,
// actionable, value-safe error rather than substituting a guessed
// policy or a guessed bound.
func NewGraphReadiness(capability *graphapi.GraphCapability, maxObservationAge time.Duration) (*GraphReadiness, error) {
	var errs []string
	if capability == nil {
		errs = append(errs, "capability is required; readiness derives from the EXPLICIT activation policy and never infers one")
	}
	if maxObservationAge <= 0 {
		errs = append(errs, "maxObservationAge is required and must be positive; a stale observation must be able to expire")
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("[%s] %s", GraphReadinessCodeConfigInvalid, strings.Join(errs, "; "))
	}
	return &GraphReadiness{
		capability: capability,
		maxAge:     maxObservationAge,
		now:        func() time.Time { return time.Now().UTC() },
	}, nil
}

// Publish validates and stores a synthetic aggregate observation.
//
// It refuses an observation that fails the closed graphsynthetic
// contract, and it refuses an observation whose activation contradicts
// the current explicit activation policy — a contradicting observation
// is never allowed to become product truth.
func (g *GraphReadiness) Publish(result graphsynthetic.AggregateResult) error {
	if g == nil || g.capability == nil {
		return fmt.Errorf("[%s] readiness projection is not constructed; publication has no explicit activation policy to agree with", GraphReadinessCodeConfigInvalid)
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if want := g.capability.Activation().State; result.Activation != want {
		return fmt.Errorf(
			"[%s] observation carries activation %q while the explicit activation policy is %q",
			GraphReadinessCodeActivationMismatch, result.Activation, want,
		)
	}

	stored := result
	stored.Families = slices.Clone(result.Families)

	g.mu.Lock()
	defer g.mu.Unlock()
	g.observed = &stored
	return nil
}

// ObserveActivation satisfies graphsynthetic.Observer. Activation
// telemetry is owned by graphsynthetic.TelemetryObserver; readiness
// reads the activation policy directly from the capability, so this
// observation contributes nothing to the projection.
func (g *GraphReadiness) ObserveActivation(graphapi.Activation) {}

// ObserveFamilyRead satisfies graphsynthetic.Observer. Per-family rows
// reach the projection as part of the validated aggregate, never
// individually, so an interrupted run can never leave a partial family
// set standing as product truth.
func (g *GraphReadiness) ObserveFamilyRead(graphsynthetic.GraphFamilyResult) {}

// ObserveAggregate publishes the observation into product readiness. A
// refused publication is LOGGED with value-safe fields and leaves the
// previous projection untouched — it is never silently accepted.
func (g *GraphReadiness) ObserveAggregate(result graphsynthetic.AggregateResult) {
	if err := g.Publish(result); err != nil {
		slog.Warn("graph read-synthetic aggregate refused by readiness publication",
			"activation", string(result.Activation),
			"state", string(result.State),
			"code", result.Code,
			"evidence_ref", result.EvidenceRef,
			"error", err.Error(),
		)
	}
}

// Snapshot derives the value-safe readiness projection from the
// explicit activation policy and the most recent published observation.
//
// Derivation, in order:
//
//   - policy DISABLED                  -> policy_disabled, ready=false
//   - policy ENABLED, no observation   -> unavailable,     ready=false
//   - policy ENABLED, observation old  -> unavailable,     ready=false
//   - policy ENABLED, fresh observation-> the aggregate's own state,
//     ready = aggregate.Available()
//
// Ready is set from AggregateResult.Available alone. No branch reads a
// static asset, a mounted route, a database handle, or process uptime,
// so none of those can produce a ready Graph journey.
func (g *GraphReadiness) Snapshot() GraphHealthSection {
	if g == nil || g.capability == nil {
		// Fail closed: an unconstructed projection never claims ready.
		return GraphHealthSection{
			Ready:       false,
			Activation:  string(graphapi.ActivationDisabled),
			State:       string(graphsynthetic.AggregateUnavailable),
			Code:        GraphReadinessCodeConfigInvalid,
			EvidenceRef: graphsynthetic.AggregateEvidenceRef,
		}
	}

	section := GraphHealthSection{
		Ready:       false,
		Activation:  string(g.capability.Activation().State),
		EvidenceRef: graphsynthetic.AggregateEvidenceRef,
	}
	observed := g.observation()
	if observed != nil {
		observedAt := observed.ObservedAt
		durationMs := observed.DurationMs
		section.ObservedAt = &observedAt
		section.DurationMs = &durationMs
		section.Families = slices.Clone(observed.Families)
	}

	// Explicit activation policy DISABLED is a truthful non-ready
	// result and a valid deployment state — never a fault, and never a
	// ready Graph journey.
	if g.capability.Disabled() {
		section.State = string(graphsynthetic.AggregatePolicyDisabled)
		section.Code = graphsynthetic.CodePolicyDisabled
		return section
	}

	if observed == nil {
		section.State = string(graphsynthetic.AggregateUnavailable)
		section.Code = GraphReadinessCodeNotObserved
		return section
	}
	if g.now().Sub(observed.ObservedAt) > g.maxAge {
		section.State = string(graphsynthetic.AggregateUnavailable)
		section.Code = GraphReadinessCodeStale
		return section
	}

	section.State = string(observed.State)
	section.Code = observed.Code
	section.Ready = observed.Available()
	return section
}

// observation returns the most recent published observation, or nil.
func (g *GraphReadiness) observation() *graphsynthetic.AggregateResult {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.observed
}

// graphJourneyReady reports whether the Graph journey satisfies STRICT
// readiness. It fails CLOSED when the readiness projection is not
// wired, so an unwired deployment can never present a ready Graph
// journey on the strength of mounted routes or a healthy database.
func (d *Dependencies) graphJourneyReady() bool {
	if d.GraphReadiness == nil {
		return false
	}
	return d.GraphReadiness.Snapshot().Ready
}
