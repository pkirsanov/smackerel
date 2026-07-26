package availability

import (
	"context"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
)

// ProviderLister is the minimal runtime provider-registry surface the live
// availability Service consumes. *provider.Registry (and any registry exposing
// List) satisfies it. It is deliberately narrow so the Service never depends on
// registration or mutation behavior.
type ProviderLister interface {
	List() []recprovider.Provider
}

// fixtureClassed is the optional TYPED marker a build- and type-isolated fixture
// provider implements to declare itself a fixture. Classification is by type,
// never by ID prefix (BUG-039-005): a provider that implements it and returns
// true is fixture-class and can never enter a production readiness denominator.
// Production adapters do not implement it and are production-class by default.
type fixtureClassed interface {
	IsFixtureProvider() bool
}

// Service derives the live recommendation availability snapshot from the real
// runtime provider registry and SST enablement, reusing the pure Determine gate.
//
// It performs NO live probe of its own: it reads each provider's already-observed
// RuntimeHealth (an injected fact exposed by the provider's own health surface)
// and maps it into the value-type ProviderState the gate consumes. This keeps the
// determination logic pure and the wiring thin — the honest readiness verdict is
// still produced solely by Determine, never by feature enablement, route
// mounting, or registry cardinality.
type Service struct {
	lister   ProviderLister
	enabled  bool
	validFor time.Duration
	now      func() time.Time
}

// NewService wires the live availability Service. enabled is the SST
// recommendations.enabled flag; validFor bounds snapshot freshness (0 leaves the
// snapshot open-ended); now is injectable for deterministic tests and defaults to
// time.Now.
func NewService(lister ProviderLister, enabled bool, validFor time.Duration, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{lister: lister, enabled: enabled, validFor: validFor, now: now}
}

// Snapshot maps the live registry to []ProviderState and returns the immutable
// availability snapshot for the category and operation via the pure gate. A nil
// or empty registry yields zero providers, so an enabled capability with no
// eligible healthy production provider is honest not-ready — never false-ready.
func (s *Service) Snapshot(ctx context.Context, category recommendation.Category, op Operation, required bool) AvailabilitySnapshot {
	var states []ProviderState
	if s.lister != nil {
		providers := s.lister.List()
		states = make([]ProviderState, 0, len(providers))
		for _, p := range providers {
			states = append(states, providerStateFrom(ctx, p))
		}
	}
	return Determine(Input{
		Enabled:     s.enabled,
		Required:    required,
		Category:    category,
		Operation:   op,
		Providers:   states,
		EvaluatedAt: s.now().UTC(),
		ValidFor:    s.validFor,
	})
}

// providerStateFrom projects one live runtime provider into the injected
// value-type ProviderState the gate consumes. A provider present in the runtime
// registry was operator-selected, enabled, configured, and registered by
// construction; a disabled health observation demotes Enabled so the provider
// drops out of the denominator. Fixture class is resolved by the TYPED marker.
func providerStateFrom(ctx context.Context, p recprovider.Provider) ProviderState {
	health := p.Health(ctx)
	class := ProviderClassProduction
	if fc, ok := p.(fixtureClassed); ok && fc.IsFixtureProvider() {
		class = ProviderClassFixture
	}
	return ProviderState{
		ID:               p.ID(),
		DisplayName:      p.DisplayName(),
		Class:            class,
		OperatorSelected: true,
		Enabled:          health.Status != recprovider.StatusDisabled,
		Configured:       true,
		Registered:       true,
		Categories:       p.Categories(),
		Health:           healthFrom(health.Status),
	}
}

// healthFrom maps the provider-neutral RuntimeStatus to the gate's bounded
// HealthStatus. Degraded and failing are not fresh-healthy, so they stay in the
// denominator but out of the numerator; disabled maps to unknown (the provider
// is already excluded via Enabled=false).
func healthFrom(status recprovider.RuntimeStatus) HealthStatus {
	switch status {
	case recprovider.StatusHealthy:
		return HealthHealthy
	case recprovider.StatusDegraded, recprovider.StatusFailing:
		return HealthUnhealthy
	default:
		return HealthUnknown
	}
}
