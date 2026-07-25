package availability

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/recommendation"
)

// eligibleHealthy returns a fully-eligible, healthy production provider for the
// place category. Individual exclusion cases mutate one attribute from this base
// so each test isolates exactly one reason a provider leaves the denominator.
func eligibleHealthy(id string) ProviderState {
	return ProviderState{
		ID:               id,
		DisplayName:      id + " Provider",
		Class:            ProviderClassProduction,
		OperatorSelected: true,
		Enabled:          true,
		Configured:       true,
		Registered:       true,
		Categories:       []recommendation.Category{recommendation.CategoryPlace},
		Health:           HealthHealthy,
	}
}

func disabledProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Enabled = false
	return p
}

func unconfiguredProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Configured = false
	return p
}

func fixtureProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Class = ProviderClassFixture
	return p
}

func categoryIrrelevantProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Categories = []recommendation.Category{recommendation.CategoryEvent}
	return p
}

func unregisteredProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Registered = false
	return p
}

func unhealthyEligibleProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Health = HealthUnhealthy
	return p
}

func staleEligibleProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Health = HealthStale
	return p
}

func unknownEligibleProvider(id string) ProviderState {
	p := eligibleHealthy(id)
	p.Health = HealthUnknown
	return p
}

const skipCount = -1

func TestDetermineAvailability(t *testing.T) {
	cases := []struct {
		name           string
		enabled        bool
		required       bool
		operation      Operation
		providers      []ProviderState
		wantState      CapabilityState
		wantCause      AvailabilityCause
		wantReady      bool // ProviderReady
		wantRefuse     bool // MustRefuseStartup
		wantOpReady    bool // snapshot.Ready() for the evaluated operation
		wantEligible   int
		wantHealthy    int
		wantUnhealthy  int
		wantFixtures   int
		wantConfigured bool
	}{
		{
			name:          "disabled optional capability is disabled not ready",
			enabled:       false,
			required:      false,
			operation:     OperationRequest,
			providers:     []ProviderState{eligibleHealthy("alpha")},
			wantState:     CapabilityDisabled,
			wantCause:     CauseCapabilityDisabled,
			wantReady:     false,
			wantRefuse:    false,
			wantOpReady:   false,
			wantEligible:  skipCount,
			wantHealthy:   skipCount,
			wantUnhealthy: skipCount,
			wantFixtures:  skipCount,
		},
		{
			name:          "disabled required capability refuses startup",
			enabled:       false,
			required:      true,
			operation:     OperationRequest,
			providers:     []ProviderState{eligibleHealthy("alpha")},
			wantState:     CapabilityDisabled,
			wantCause:     CauseRequiredCapabilityDisabled,
			wantReady:     false,
			wantRefuse:    true,
			wantOpReady:   false,
			wantEligible:  skipCount,
			wantHealthy:   skipCount,
			wantUnhealthy: skipCount,
			wantFixtures:  skipCount,
		},
		{
			name:          "enabled zero providers optional is unavailable not ready",
			enabled:       true,
			required:      false,
			operation:     OperationRequest,
			providers:     nil,
			wantState:     CapabilityUnavailable,
			wantCause:     CauseZeroConfiguredProviders,
			wantReady:     false,
			wantRefuse:    false,
			wantOpReady:   false,
			wantEligible:  0,
			wantHealthy:   0,
			wantUnhealthy: 0,
			wantFixtures:  0,
		},
		{
			name:          "enabled zero providers required refuses startup",
			enabled:       true,
			required:      true,
			operation:     OperationRequest,
			providers:     nil,
			wantState:     CapabilityUnavailable,
			wantCause:     CauseZeroConfiguredProviders,
			wantReady:     false,
			wantRefuse:    true,
			wantOpReady:   false,
			wantEligible:  0,
			wantHealthy:   0,
			wantUnhealthy: 0,
			wantFixtures:  0,
		},
		{
			name:           "one healthy eligible provider is available and ready",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{eligibleHealthy("alpha")},
			wantState:      CapabilityAvailable,
			wantCause:      CauseProviderCoverageComplete,
			wantReady:      true,
			wantRefuse:     false,
			wantOpReady:    true,
			wantEligible:   1,
			wantHealthy:    1,
			wantUnhealthy:  0,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:           "all eligible providers unhealthy is unavailable",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{unhealthyEligibleProvider("alpha"), unhealthyEligibleProvider("beta")},
			wantState:      CapabilityUnavailable,
			wantCause:      CauseAllProvidersUnavailable,
			wantReady:      false,
			wantRefuse:     false,
			wantOpReady:    false,
			wantEligible:   2,
			wantHealthy:    0,
			wantUnhealthy:  2,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:           "stale eligible provider is in denominator but unavailable",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{staleEligibleProvider("alpha")},
			wantState:      CapabilityUnavailable,
			wantCause:      CauseAllProvidersUnavailable,
			wantReady:      false,
			wantRefuse:     false,
			wantOpReady:    false,
			wantEligible:   1,
			wantHealthy:    0,
			wantUnhealthy:  1,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:           "unknown-health eligible provider is in denominator but unavailable",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{unknownEligibleProvider("alpha")},
			wantState:      CapabilityUnavailable,
			wantCause:      CauseAllProvidersUnavailable,
			wantReady:      false,
			wantRefuse:     false,
			wantOpReady:    false,
			wantEligible:   1,
			wantHealthy:    0,
			wantUnhealthy:  1,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:           "one healthy one unhealthy eligible is degraded but ready",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{eligibleHealthy("alpha"), unhealthyEligibleProvider("beta")},
			wantState:      CapabilityDegraded,
			wantCause:      CausePartialProviderCoverage,
			wantReady:      true,
			wantRefuse:     false,
			wantOpReady:    true,
			wantEligible:   2,
			wantHealthy:    1,
			wantUnhealthy:  1,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:           "configured production provider without adapter is unavailable",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{unregisteredProvider("alpha")},
			wantState:      CapabilityUnavailable,
			wantCause:      CauseConfiguredAdapterMissing,
			wantReady:      false,
			wantRefuse:     false,
			wantOpReady:    false,
			wantEligible:   0,
			wantHealthy:    0,
			wantUnhealthy:  0,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:           "registered provider without category support is unavailable",
			enabled:        true,
			required:       false,
			operation:      OperationRequest,
			providers:      []ProviderState{categoryIrrelevantProvider("alpha")},
			wantState:      CapabilityUnavailable,
			wantCause:      CauseZeroCategoryProviders,
			wantReady:      false,
			wantRefuse:     false,
			wantOpReady:    false,
			wantEligible:   0,
			wantHealthy:    0,
			wantUnhealthy:  0,
			wantFixtures:   0,
			wantConfigured: true,
		},
		{
			name:          "only fixture provider is unavailable never ready",
			enabled:       true,
			required:      false,
			operation:     OperationRequest,
			providers:     []ProviderState{fixtureProvider("alpha")},
			wantState:     CapabilityUnavailable,
			wantCause:     CauseZeroConfiguredProviders,
			wantReady:     false,
			wantRefuse:    false,
			wantOpReady:   false,
			wantEligible:  0,
			wantHealthy:   0,
			wantUnhealthy: 0,
			wantFixtures:  1,
		},
		{
			name:          "only disabled provider is unavailable never ready",
			enabled:       true,
			required:      false,
			operation:     OperationRequest,
			providers:     []ProviderState{disabledProvider("alpha")},
			wantState:     CapabilityUnavailable,
			wantCause:     CauseZeroConfiguredProviders,
			wantReady:     false,
			wantRefuse:    false,
			wantOpReady:   false,
			wantEligible:  0,
			wantHealthy:   0,
			wantUnhealthy: 0,
			wantFixtures:  0,
		},
		{
			name:          "only unconfigured provider is unavailable never ready",
			enabled:       true,
			required:      false,
			operation:     OperationRequest,
			providers:     []ProviderState{unconfiguredProvider("alpha")},
			wantState:     CapabilityUnavailable,
			wantCause:     CauseZeroConfiguredProviders,
			wantReady:     false,
			wantRefuse:    false,
			wantOpReady:   false,
			wantEligible:  0,
			wantHealthy:   0,
			wantUnhealthy: 0,
			wantFixtures:  0,
		},
		{
			name:          "provider-independent operation is ready without a provider",
			enabled:       true,
			required:      false,
			operation:     OperationWatchPause,
			providers:     nil,
			wantState:     CapabilityUnavailable,
			wantCause:     CauseZeroConfiguredProviders,
			wantReady:     false,
			wantRefuse:    false,
			wantOpReady:   true, // pause reduces activity and stays safe during an outage
			wantEligible:  0,
			wantHealthy:   0,
			wantUnhealthy: 0,
			wantFixtures:  0,
		},
		{
			name:      "one healthy eligible amid excluded declarations stays available",
			enabled:   true,
			required:  false,
			operation: OperationRequest,
			providers: []ProviderState{
				eligibleHealthy("alpha"),
				disabledProvider("bravo"),
				unconfiguredProvider("charlie"),
				fixtureProvider("delta"),
				categoryIrrelevantProvider("echo"),
			},
			wantState:      CapabilityAvailable,
			wantCause:      CauseProviderCoverageComplete,
			wantReady:      true,
			wantRefuse:     false,
			wantOpReady:    true,
			wantEligible:   1,
			wantHealthy:    1,
			wantUnhealthy:  0,
			wantFixtures:   1,
			wantConfigured: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := Determine(Input{
				Enabled:     tc.enabled,
				Required:    tc.required,
				Category:    recommendation.CategoryPlace,
				Operation:   tc.operation,
				Providers:   tc.providers,
				EvaluatedAt: time.Unix(1_700_000_000, 0),
				ValidFor:    30 * time.Second,
			})

			if snap.SchemaVersion != SnapshotSchemaVersion {
				t.Fatalf("SchemaVersion = %d, want %d", snap.SchemaVersion, SnapshotSchemaVersion)
			}
			if snap.State != tc.wantState {
				t.Errorf("State = %q, want %q", snap.State, tc.wantState)
			}
			if snap.Cause != tc.wantCause {
				t.Errorf("Cause = %q, want %q", snap.Cause, tc.wantCause)
			}
			if snap.ProviderReady != tc.wantReady {
				t.Errorf("ProviderReady = %v, want %v", snap.ProviderReady, tc.wantReady)
			}
			if got := snap.MustRefuseStartup(); got != tc.wantRefuse {
				t.Errorf("MustRefuseStartup() = %v, want %v", got, tc.wantRefuse)
			}
			if got := snap.Ready(); got != tc.wantOpReady {
				t.Errorf("Ready() = %v, want %v", got, tc.wantOpReady)
			}
			// A capability can never both be available and not provider-ready.
			if snap.State == CapabilityAvailable && !snap.ProviderReady {
				t.Errorf("available state with ProviderReady=false is false-ready")
			}
			if tc.wantEligible != skipCount && snap.Counts.Eligible != tc.wantEligible {
				t.Errorf("Counts.Eligible = %d, want %d", snap.Counts.Eligible, tc.wantEligible)
			}
			if tc.wantHealthy != skipCount && snap.Counts.HealthyEligible != tc.wantHealthy {
				t.Errorf("Counts.HealthyEligible = %d, want %d", snap.Counts.HealthyEligible, tc.wantHealthy)
			}
			if tc.wantUnhealthy != skipCount && snap.Counts.UnhealthyEligible != tc.wantUnhealthy {
				t.Errorf("Counts.UnhealthyEligible = %d, want %d", snap.Counts.UnhealthyEligible, tc.wantUnhealthy)
			}
			if tc.wantFixtures != skipCount && snap.Counts.Fixtures != tc.wantFixtures {
				t.Errorf("Counts.Fixtures = %d, want %d", snap.Counts.Fixtures, tc.wantFixtures)
			}
			if snap.Configured != tc.wantConfigured {
				t.Errorf("Configured = %v, want %v", snap.Configured, tc.wantConfigured)
			}
			if len(snap.Participating) != snap.Counts.HealthyEligible {
				t.Errorf("Participating len = %d, want %d", len(snap.Participating), snap.Counts.HealthyEligible)
			}
			if len(snap.Missing) != snap.Counts.UnhealthyEligible {
				t.Errorf("Missing len = %d, want %d", len(snap.Missing), snap.Counts.UnhealthyEligible)
			}
		})
	}
}

// TestReadinessDenominatorExcludesDisabledUnconfiguredFixtureAndCategoryIrrelevantProviders
// proves REC-READY-011 / SCN-039-005-10: disabled, unconfigured, fixture, and
// category-irrelevant declarations are excluded from both numerator and
// denominator, one eligible healthy provider suffices, and unused declarations
// never dilute or flip readiness.
func TestReadinessDenominatorExcludesDisabledUnconfiguredFixtureAndCategoryIrrelevantProviders(t *testing.T) {
	withExclusions := []ProviderState{
		eligibleHealthy("alpha"),
		disabledProvider("bravo"),
		unconfiguredProvider("charlie"),
		fixtureProvider("delta"),
		categoryIrrelevantProvider("echo"),
	}
	base := Input{
		Enabled:   true,
		Required:  false,
		Category:  recommendation.CategoryPlace,
		Operation: OperationRequest,
		Providers: withExclusions,
	}

	snap := Determine(base)

	if snap.State != CapabilityAvailable {
		t.Fatalf("State = %q, want available", snap.State)
	}
	if !snap.ProviderReady {
		t.Fatalf("ProviderReady = false, want true (one eligible healthy provider suffices)")
	}
	if snap.Cause != CauseProviderCoverageComplete {
		t.Errorf("Cause = %q, want %q", snap.Cause, CauseProviderCoverageComplete)
	}
	if snap.Counts.Declared != 5 {
		t.Errorf("Counts.Declared = %d, want 5", snap.Counts.Declared)
	}
	if snap.Counts.Eligible != 1 {
		t.Errorf("Counts.Eligible (denominator) = %d, want 1", snap.Counts.Eligible)
	}
	if snap.Counts.HealthyEligible != 1 {
		t.Errorf("Counts.HealthyEligible (numerator) = %d, want 1", snap.Counts.HealthyEligible)
	}
	if snap.Counts.Fixtures != 1 {
		t.Errorf("Counts.Fixtures = %d, want 1", snap.Counts.Fixtures)
	}
	// disabled + unconfigured + fixture are disabled/unconfigured setup inventory;
	// the category-irrelevant provider is configured-production so it is not.
	if snap.Counts.DisabledOrUnconfigured != 3 {
		t.Errorf("Counts.DisabledOrUnconfigured = %d, want 3", snap.Counts.DisabledOrUnconfigured)
	}
	if len(snap.Participating) != 1 || snap.Participating[0].ProviderID != "alpha" {
		t.Errorf("Participating = %+v, want exactly the eligible provider alpha", snap.Participating)
	}

	// Excluded declarations must not dilute the readiness verdict: the same input
	// with ONLY the eligible provider must produce an identical readiness verdict.
	onlyEligible := base
	onlyEligible.Providers = []ProviderState{eligibleHealthy("alpha")}
	lean := Determine(onlyEligible)

	if lean.State != snap.State || lean.ProviderReady != snap.ProviderReady || lean.Cause != snap.Cause {
		t.Errorf("excluded declarations changed the readiness verdict: with=%q/%v/%q only=%q/%v/%q",
			snap.State, snap.ProviderReady, snap.Cause, lean.State, lean.ProviderReady, lean.Cause)
	}
}

// TestAdversarialEnabledZeroEligibleProvidersIsNotReady encodes the exact
// BUG-039-005 false-ready condition: an ENABLED capability with zero ELIGIBLE
// providers must never report ready. A naive `len(providers) > 0` gate would
// flip the fixture-only, disabled-only, and category-irrelevant cases to ready
// and fail these assertions.
func TestAdversarialEnabledZeroEligibleProvidersIsNotReady(t *testing.T) {
	cases := []struct {
		name      string
		providers []ProviderState
	}{
		{"zero providers", nil},
		{"only fixture", []ProviderState{fixtureProvider("alpha")}},
		{"only disabled", []ProviderState{disabledProvider("alpha")}},
		{"only unconfigured", []ProviderState{unconfiguredProvider("alpha")}},
		{"only unregistered", []ProviderState{unregisteredProvider("alpha")}},
		{"only category irrelevant", []ProviderState{categoryIrrelevantProvider("alpha")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, required := range []bool{false, true} {
				snap := Determine(Input{
					Enabled:   true,
					Required:  required,
					Category:  recommendation.CategoryPlace,
					Operation: OperationRequest,
					Providers: tc.providers,
				})
				if snap.State == CapabilityAvailable || snap.State == CapabilityDegraded {
					t.Errorf("required=%v: State = %q, want a not-ready state (never available/degraded)", required, snap.State)
				}
				if snap.ProviderReady {
					t.Errorf("required=%v: ProviderReady = true, want false (zero eligible providers)", required)
				}
				if snap.Ready() {
					t.Errorf("required=%v: Ready() = true for a provider-dependent request, want false", required)
				}
				if got := snap.MustRefuseStartup(); got != required {
					t.Errorf("required=%v: MustRefuseStartup() = %v, want %v", required, got, required)
				}
			}
		})
	}
}

// TestOneHealthyEligibleProviderSufficesNoSecondRequired proves REC-READY-012: a
// single eligible healthy provider is sufficient for first readiness and a
// second provider is not a completion dependency.
func TestOneHealthyEligibleProviderSufficesNoSecondRequired(t *testing.T) {
	providers := []ProviderState{eligibleHealthy("alpha")}
	if len(providers) != 1 {
		t.Fatalf("precondition: want exactly one provider (no second), got %d", len(providers))
	}

	snap := Determine(Input{
		Enabled:   true,
		Required:  true,
		Category:  recommendation.CategoryPlace,
		Operation: OperationRequest,
		Providers: providers,
	})

	if snap.State != CapabilityAvailable {
		t.Fatalf("State = %q, want available with a single healthy eligible provider", snap.State)
	}
	if !snap.ProviderReady {
		t.Errorf("ProviderReady = false, want true (one provider suffices)")
	}
	if !snap.Ready() {
		t.Errorf("Ready() = false, want true")
	}
	if snap.MustRefuseStartup() {
		t.Errorf("MustRefuseStartup() = true, want false (required capability is provider-ready)")
	}
	if snap.Cause != CauseProviderCoverageComplete {
		t.Errorf("Cause = %q, want %q", snap.Cause, CauseProviderCoverageComplete)
	}
	if snap.Counts.Eligible != 1 || snap.Counts.HealthyEligible != 1 {
		t.Errorf("Counts eligible/healthy = %d/%d, want 1/1", snap.Counts.Eligible, snap.Counts.HealthyEligible)
	}
}

// TestOperationRequiresProvider verifies the closed operation gating table:
// request and watch mount/refresh require a ready provider; pause, silence,
// delete, and provider recheck reduce or inspect activity and stay safe.
func TestOperationRequiresProvider(t *testing.T) {
	requires := map[Operation]bool{
		OperationRequest:         true,
		OperationWatchCreate:     true,
		OperationWatchEnable:     true,
		OperationWatchResume:     true,
		OperationWatchRefresh:    true,
		OperationWatchPause:      false,
		OperationWatchSilence:    false,
		OperationWatchDelete:     false,
		OperationProviderRecheck: false,
	}
	for op, want := range requires {
		if got := op.RequiresProvider(); got != want {
			t.Errorf("Operation(%q).RequiresProvider() = %v, want %v", op, got, want)
		}
	}
}
