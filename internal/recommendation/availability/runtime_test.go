package availability

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
	recprovider "github.com/smackerel/smackerel/internal/recommendation/provider"
)

// stubProvider is a production-class provider double (it does NOT implement the
// fixture marker, so the gate classifies it as production).
type stubProvider struct {
	id     string
	name   string
	cats   []recommendation.Category
	status recprovider.RuntimeStatus
}

func (p stubProvider) ID() string          { return p.id }
func (p stubProvider) DisplayName() string { return p.name }
func (p stubProvider) Categories() []recommendation.Category {
	return append([]recommendation.Category(nil), p.cats...)
}
func (p stubProvider) Fetch(context.Context, recprovider.ReducedQuery) (recprovider.FactsBundle, error) {
	return recprovider.FactsBundle{ProviderID: p.id}, nil
}
func (p stubProvider) Health(context.Context) recprovider.RuntimeHealth {
	return recprovider.RuntimeHealth{ProviderID: p.id, DisplayName: p.name, Status: p.status, ObservedAt: time.Now().UTC(), CategoryList: p.cats}
}

// fixtureStubProvider is a fixture-class provider double: it implements the
// typed fixture marker and therefore MUST be excluded from production readiness.
type fixtureStubProvider struct{ stubProvider }

func (fixtureStubProvider) IsFixtureProvider() bool { return true }

type stubLister struct{ providers []recprovider.Provider }

func (s stubLister) List() []recprovider.Provider { return s.providers }

func prod(id string, status recprovider.RuntimeStatus, cats ...recommendation.Category) stubProvider {
	return stubProvider{id: id, name: id, cats: cats, status: status}
}

func fixture(id string, status recprovider.RuntimeStatus, cats ...recommendation.Category) fixtureStubProvider {
	return fixtureStubProvider{stubProvider{id: id, name: id, cats: cats, status: status}}
}

func snapshotFor(providers []recprovider.Provider) AvailabilitySnapshot {
	clock := func() time.Time { return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC) }
	svc := NewService(stubLister{providers: providers}, true, time.Minute, clock)
	return svc.Snapshot(context.Background(), recommendation.CategoryPlace, OperationRequest, false)
}

func assertReadiness(t *testing.T, snap AvailabilitySnapshot, wantReady bool, wantState CapabilityState, wantCause AvailabilityCause) {
	t.Helper()
	if got := snap.Ready(); got != wantReady {
		t.Fatalf("Ready() = %v, want %v (state=%s cause=%s)", got, wantReady, snap.State, snap.Cause)
	}
	if snap.State != wantState {
		t.Fatalf("State = %s, want %s", snap.State, wantState)
	}
	if snap.Cause != wantCause {
		t.Fatalf("Cause = %s, want %s", snap.Cause, wantCause)
	}
}

func assertCounts(t *testing.T, snap AvailabilitySnapshot, declared, eligible, healthy, fixtures int) {
	t.Helper()
	if snap.Counts.Declared != declared {
		t.Fatalf("Counts.Declared = %d, want %d", snap.Counts.Declared, declared)
	}
	if snap.Counts.Eligible != eligible {
		t.Fatalf("Counts.Eligible = %d, want %d", snap.Counts.Eligible, eligible)
	}
	if snap.Counts.HealthyEligible != healthy {
		t.Fatalf("Counts.HealthyEligible = %d, want %d", snap.Counts.HealthyEligible, healthy)
	}
	if snap.Counts.Fixtures != fixtures {
		t.Fatalf("Counts.Fixtures = %d, want %d", snap.Counts.Fixtures, fixtures)
	}
}

// TestServiceSnapshotFromRegistry proves the live availability Service maps the
// real provider registry into the pure gate correctly. The adversarial subtests
// are the exact BUG-039-005 false-ready condition: an ENABLED capability with
// zero ELIGIBLE providers (empty / fixture-only / disabled-only /
// wrong-category-only) must report ready=false, never false-ready from
// enablement or provider cardinality.
func TestServiceSnapshotFromRegistry(t *testing.T) {
	t.Run("empty registry enabled is not ready (the bug)", func(t *testing.T) {
		snap := snapshotFor(nil)
		assertReadiness(t, snap, false, CapabilityUnavailable, CauseZeroConfiguredProviders)
		assertCounts(t, snap, 0, 0, 0, 0)
	})

	t.Run("one healthy production provider is ready", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{prod("google_places", recprovider.StatusHealthy, recommendation.CategoryPlace)})
		assertReadiness(t, snap, true, CapabilityAvailable, CauseProviderCoverageComplete)
		assertCounts(t, snap, 1, 1, 1, 0)
	})

	t.Run("fixture-only registry is not ready (fixtures never dilute)", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{fixture("fixture_google_places", recprovider.StatusHealthy, recommendation.CategoryPlace)})
		assertReadiness(t, snap, false, CapabilityUnavailable, CauseZeroConfiguredProviders)
		assertCounts(t, snap, 1, 0, 0, 1)
	})

	t.Run("healthy production plus healthy fixture stays ready with fixture excluded", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{
			prod("google_places", recprovider.StatusHealthy, recommendation.CategoryPlace),
			fixture("fixture_yelp", recprovider.StatusHealthy, recommendation.CategoryPlace),
		})
		assertReadiness(t, snap, true, CapabilityAvailable, CauseProviderCoverageComplete)
		assertCounts(t, snap, 2, 1, 1, 1)
	})

	t.Run("only-failing production provider is unavailable not ready", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{prod("google_places", recprovider.StatusFailing, recommendation.CategoryPlace)})
		assertReadiness(t, snap, false, CapabilityUnavailable, CauseAllProvidersUnavailable)
		assertCounts(t, snap, 1, 1, 0, 0)
	})

	t.Run("only-disabled production provider is excluded and not ready", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{prod("google_places", recprovider.StatusDisabled, recommendation.CategoryPlace)})
		assertReadiness(t, snap, false, CapabilityUnavailable, CauseZeroConfiguredProviders)
		assertCounts(t, snap, 1, 0, 0, 0)
	})

	t.Run("healthy production provider for a different category is not ready for place", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{prod("event_source", recprovider.StatusHealthy, recommendation.CategoryEvent)})
		assertReadiness(t, snap, false, CapabilityUnavailable, CauseZeroCategoryProviders)
		assertCounts(t, snap, 1, 0, 0, 0)
	})

	t.Run("one healthy plus one failing production provider is degraded but ready", func(t *testing.T) {
		snap := snapshotFor([]recprovider.Provider{
			prod("google_places", recprovider.StatusHealthy, recommendation.CategoryPlace),
			prod("yelp", recprovider.StatusFailing, recommendation.CategoryPlace),
		})
		assertReadiness(t, snap, true, CapabilityDegraded, CausePartialProviderCoverage)
		assertCounts(t, snap, 2, 2, 1, 0)
	})
}

// TestServiceNilListerIsHonestNotReady proves a nil registry (no provider source
// wired at all) is honest not-ready rather than panicking or false-ready.
func TestServiceNilListerIsHonestNotReady(t *testing.T) {
	svc := NewService(nil, true, 0, nil)
	snap := svc.Snapshot(context.Background(), recommendation.CategoryPlace, OperationRequest, false)
	if snap.Ready() {
		t.Fatalf("nil-lister Ready() = true, want false")
	}
	if snap.Cause != CauseZeroConfiguredProviders {
		t.Fatalf("nil-lister Cause = %s, want %s", snap.Cause, CauseZeroConfiguredProviders)
	}
}

// TestServiceDisabledCapabilityIsNotReady proves that when the SST enabled flag
// is false, the capability is disabled and not ready even with healthy providers.
func TestServiceDisabledCapabilityIsNotReady(t *testing.T) {
	svc := NewService(stubLister{providers: []recprovider.Provider{prod("google_places", recprovider.StatusHealthy, recommendation.CategoryPlace)}}, false, 0, nil)
	snap := svc.Snapshot(context.Background(), recommendation.CategoryPlace, OperationRequest, false)
	if snap.Ready() {
		t.Fatalf("disabled capability Ready() = true, want false")
	}
	if snap.State != CapabilityDisabled {
		t.Fatalf("disabled capability State = %s, want %s", snap.State, CapabilityDisabled)
	}
}
