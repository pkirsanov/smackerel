// Package availability owns the single, provider-backed readiness determination
// for the recommendation capability (BUG-039-005).
//
// It replaces the distributed false-ready inference that treated feature
// enablement, route mounting, or provider-registry cardinality as proof that a
// relevant healthy production provider exists. Readiness is granted only when at
// least one operator-selected, enabled, fully-configured, production-class,
// registered, category-compatible provider ALSO carries a fresh healthy health
// observation. Zero eligible providers is honest not-ready, never false-ready.
//
// Provider health and eligibility are an INJECTED value input (ProviderState).
// The determination performs no live health probe, so the gate logic is fully
// unit-testable. Live provider connectivity and the wiring of this snapshot into
// startup, request, watch, scheduler, and status consumers are owned by later
// scopes and are intentionally not implemented here.
package availability

import (
	"sort"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
)

// SnapshotSchemaVersion versions the immutable AvailabilitySnapshot contract so
// consumers can detect an incompatible availability projection.
const SnapshotSchemaVersion = 1

// ProviderClass is the typed runtime class of a declared provider. A fixture
// provider can never enter the readiness denominator of a production
// determination; exclusion is structural and typed, not an ID-prefix guess.
type ProviderClass string

const (
	// ProviderClassProduction marks a real production adapter.
	ProviderClassProduction ProviderClass = "production"
	// ProviderClassFixture marks a build- and type-isolated test provider.
	ProviderClassFixture ProviderClass = "fixture"
)

// HealthStatus is the bounded, credential-free provider health observation fed
// into the determination. It is an injected fact, not a live probe result.
type HealthStatus string

const (
	// HealthHealthy is a fresh, usable provider observation.
	HealthHealthy HealthStatus = "healthy"
	// HealthUnhealthy is a failing/quota/auth/error observation.
	HealthUnhealthy HealthStatus = "unhealthy"
	// HealthStale is an observation older than the freshness window.
	HealthStale HealthStatus = "stale"
	// HealthUnknown is the absence of a usable observation.
	HealthUnknown HealthStatus = "unknown"
)

// CapabilityState is the closed availability state of the recommendation
// capability for one evaluated category and operation.
type CapabilityState string

const (
	// CapabilityDisabled means the capability is not enabled.
	CapabilityDisabled CapabilityState = "disabled"
	// CapabilityAvailable means every eligible provider is healthy.
	CapabilityAvailable CapabilityState = "available"
	// CapabilityDegraded means some eligible providers are healthy and others
	// are not; usable but with explicit limitation.
	CapabilityDegraded CapabilityState = "degraded"
	// CapabilityUnavailable means no eligible provider can currently serve.
	CapabilityUnavailable CapabilityState = "unavailable"
)

// AvailabilityCause is the closed, safe cause that explains a CapabilityState.
// No cause value ever carries a credential, query, location, or upstream body.
type AvailabilityCause string

const (
	// CauseCapabilityDisabled - enabled=false, required=false.
	CauseCapabilityDisabled AvailabilityCause = "capability_disabled"
	// CauseRequiredCapabilityDisabled - required=true and enabled=false.
	CauseRequiredCapabilityDisabled AvailabilityCause = "required_capability_disabled"
	// CauseZeroConfiguredProviders - enabled with no operator-selected fully
	// configured production provider at all.
	CauseZeroConfiguredProviders AvailabilityCause = "zero_configured_providers"
	// CauseConfiguredAdapterMissing - a selected configured production provider
	// exists but none has a registered adapter.
	CauseConfiguredAdapterMissing AvailabilityCause = "configured_adapter_missing"
	// CauseZeroCategoryProviders - registered selected providers exist but none
	// supports the requested category.
	CauseZeroCategoryProviders AvailabilityCause = "zero_category_providers"
	// CauseAllProvidersUnavailable - the eligible denominator is non-empty and
	// every member is failing, stale, or unknown.
	CauseAllProvidersUnavailable AvailabilityCause = "all_providers_unavailable"
	// CausePartialProviderCoverage - at least one eligible provider is healthy
	// while at least one other eligible provider is not.
	CausePartialProviderCoverage AvailabilityCause = "partial_provider_coverage"
	// CauseProviderCoverageComplete - the eligible denominator is non-empty and
	// every member is healthy.
	CauseProviderCoverageComplete AvailabilityCause = "provider_coverage_complete"
)

// Operation is the closed set of capability operations that availability is
// evaluated for. Only provider-dependent operations require a ready provider.
type Operation string

const (
	// OperationRequest evaluates a recommendation request.
	OperationRequest Operation = "request"
	// OperationWatchCreate creates a standing watch.
	OperationWatchCreate Operation = "watch_create"
	// OperationWatchEnable re-enables a disabled watch.
	OperationWatchEnable Operation = "watch_enable"
	// OperationWatchResume resumes a paused watch.
	OperationWatchResume Operation = "watch_resume"
	// OperationWatchRefresh triggers a watch evaluation.
	OperationWatchRefresh Operation = "watch_refresh"
	// OperationWatchPause pauses a watch (safe during outages).
	OperationWatchPause Operation = "watch_pause"
	// OperationWatchSilence silences a watch (safe during outages).
	OperationWatchSilence Operation = "watch_silence"
	// OperationWatchDelete deletes a watch (safe during outages).
	OperationWatchDelete Operation = "watch_delete"
	// OperationProviderRecheck is an operator-only health recheck.
	OperationProviderRecheck Operation = "provider_recheck"
)

// RequiresProvider reports whether the operation may proceed only when a ready
// relevant provider exists. Pause, silence, delete, and provider recheck reduce
// or inspect activity and remain safe during a provider outage.
func (o Operation) RequiresProvider() bool {
	switch o {
	case OperationRequest, OperationWatchCreate, OperationWatchEnable,
		OperationWatchResume, OperationWatchRefresh:
		return true
	default:
		return false
	}
}

// ProviderState is the injected, credential-free description of one declared
// provider used to determine readiness. Every field is a value input: none is a
// live probe, so the determination is fully unit-testable without connectivity.
type ProviderState struct {
	ID               string
	DisplayName      string
	Class            ProviderClass
	OperatorSelected bool
	Enabled          bool
	Configured       bool
	Registered       bool
	Categories       []recommendation.Category
	Health           HealthStatus
}

func (p ProviderState) supportsCategory(cat recommendation.Category) bool {
	for _, c := range p.Categories {
		if c == cat {
			return true
		}
	}
	return false
}

// eligible reports whether the provider belongs in the readiness denominator for
// the requested category. Disabled, unconfigured, fixture, unregistered, and
// category-irrelevant declarations are excluded before numerator/denominator
// calculation and therefore cannot dilute or flip readiness.
func (p ProviderState) eligible(cat recommendation.Category) bool {
	return p.OperatorSelected &&
		p.Enabled &&
		p.Configured &&
		p.Registered &&
		p.Class == ProviderClassProduction &&
		p.supportsCategory(cat)
}

func (p ProviderState) healthy() bool { return p.Health == HealthHealthy }

// ProviderCounts are bounded, credential-free provider tallies for safe status
// and telemetry projections.
type ProviderCounts struct {
	Declared               int // every declared provider (operator view only)
	DisabledOrUnconfigured int // disabled/unconfigured setup inventory (operator view only)
	Eligible               int // readiness denominator
	HealthyEligible        int // readiness numerator
	UnhealthyEligible      int // eligible but failing/stale/unknown
	Fixtures               int // registered fixture class (operator safety view; always excluded)
}

// ProviderEvidence is renderer-safe evidence for a participating or missing
// eligible provider. It never carries credentials, queries, or upstream bodies.
type ProviderEvidence struct {
	ProviderID  string
	DisplayName string
	Health      HealthStatus
}

// AvailabilitySnapshot is the immutable, category- and operation-scoped
// availability projection consumed by every recommendation surface.
type AvailabilitySnapshot struct {
	SchemaVersion int
	Enabled       bool
	Required      bool
	Configured    bool
	ProviderReady bool
	State         CapabilityState
	Cause         AvailabilityCause
	Category      recommendation.Category
	Operation     Operation
	Counts        ProviderCounts
	Participating []ProviderEvidence
	Missing       []ProviderEvidence
	EvaluatedAt   time.Time
	ValidUntil    time.Time
}

// MustRefuseStartup reports whether a required capability that is not
// provider-ready must refuse process startup. An optional capability never
// refuses startup; it reports unavailable in isolation so an optional outage
// does not become a whole-product outage.
func (s AvailabilitySnapshot) MustRefuseStartup() bool {
	return s.Required && !s.ProviderReady
}

// Ready reports whether the evaluated operation may proceed. A
// provider-independent operation is ready whenever the capability is enabled; a
// provider-dependent operation is ready only when a healthy eligible provider
// exists. Feature enablement alone never yields ready for a dependent operation.
func (s AvailabilitySnapshot) Ready() bool {
	if !s.Enabled {
		return false
	}
	if !s.Operation.RequiresProvider() {
		return true
	}
	return s.ProviderReady
}

// Input carries the injected facts a determination consumes: capability
// enablement and requiredness, the evaluated category and operation, the
// declared provider states, and the freshness window.
type Input struct {
	Enabled     bool
	Required    bool
	Category    recommendation.Category
	Operation   Operation
	Providers   []ProviderState
	EvaluatedAt time.Time
	ValidFor    time.Duration
}

// Determine computes the immutable availability snapshot for the input. It is a
// pure function of its injected state; readiness derives from configured healthy
// non-fixture eligible providers, never from enablement or registry cardinality.
func Determine(in Input) AvailabilitySnapshot {
	providers := append([]ProviderState(nil), in.Providers...)
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })

	snap := AvailabilitySnapshot{
		SchemaVersion: SnapshotSchemaVersion,
		Enabled:       in.Enabled,
		Required:      in.Required,
		Category:      in.Category,
		Operation:     in.Operation,
		EvaluatedAt:   in.EvaluatedAt,
	}
	if in.ValidFor > 0 && !in.EvaluatedAt.IsZero() {
		snap.ValidUntil = in.EvaluatedAt.Add(in.ValidFor)
	}

	snap.Counts.Declared = len(providers)
	for _, p := range providers {
		if p.Class == ProviderClassFixture {
			snap.Counts.Fixtures++
		}
		if !p.OperatorSelected || !p.Enabled || !p.Configured || p.Class != ProviderClassProduction {
			snap.Counts.DisabledOrUnconfigured++
		}
	}

	if !in.Enabled {
		snap.State = CapabilityDisabled
		if in.Required {
			snap.Cause = CauseRequiredCapabilityDisabled
		} else {
			snap.Cause = CauseCapabilityDisabled
		}
		return snap
	}

	// Layered sets yield precise, distinguishable causes without leaking data.
	var selectedConfiguredProduction, registered, eligible, healthyEligible, unhealthyEligible []ProviderState
	for _, p := range providers {
		if p.OperatorSelected && p.Enabled && p.Configured && p.Class == ProviderClassProduction {
			selectedConfiguredProduction = append(selectedConfiguredProduction, p)
			if p.Registered {
				registered = append(registered, p)
			}
		}
		if p.eligible(in.Category) {
			eligible = append(eligible, p)
			if p.healthy() {
				healthyEligible = append(healthyEligible, p)
			} else {
				unhealthyEligible = append(unhealthyEligible, p)
			}
		}
	}

	snap.Configured = len(selectedConfiguredProduction) > 0
	snap.Counts.Eligible = len(eligible)
	snap.Counts.HealthyEligible = len(healthyEligible)
	snap.Counts.UnhealthyEligible = len(unhealthyEligible)
	snap.ProviderReady = len(healthyEligible) > 0
	snap.Participating = evidenceFrom(healthyEligible)
	snap.Missing = evidenceFrom(unhealthyEligible)

	switch {
	case len(eligible) == 0:
		snap.State = CapabilityUnavailable
		switch {
		case len(selectedConfiguredProduction) == 0:
			snap.Cause = CauseZeroConfiguredProviders
		case len(registered) == 0:
			snap.Cause = CauseConfiguredAdapterMissing
		default:
			snap.Cause = CauseZeroCategoryProviders
		}
	case len(healthyEligible) == 0:
		snap.State = CapabilityUnavailable
		snap.Cause = CauseAllProvidersUnavailable
	case len(unhealthyEligible) > 0:
		snap.State = CapabilityDegraded
		snap.Cause = CausePartialProviderCoverage
	default:
		snap.State = CapabilityAvailable
		snap.Cause = CauseProviderCoverageComplete
	}
	return snap
}

func evidenceFrom(providers []ProviderState) []ProviderEvidence {
	if len(providers) == 0 {
		return nil
	}
	ev := make([]ProviderEvidence, 0, len(providers))
	for _, p := range providers {
		ev = append(ev, ProviderEvidence{
			ProviderID:  p.ID,
			DisplayName: p.DisplayName,
			Health:      p.Health,
		})
	}
	return ev
}
