package acceptance

import (
	"testing"
)

// coversAllGroups reports whether m declares at least one journey for every
// closed journey group.
func coversAllGroups(m ProductJourneyManifest) bool {
	covered := m.CoveredGroups()
	for _, g := range ClosedJourneyGroups() {
		if !covered[g] {
			return false
		}
	}
	return true
}

// TestManifestCoversAllProductJourneyGroupsAndRouteAuthorities is TP-102-01-06
// (functional). It proves the canonical manifest compiles, covers every closed
// journey group (session/auth, search, digest, assistant, wiki, graph, cards,
// recommendations, notifications, capability-status, photos, connectors, models,
// synthesis), and references every route authority in the canonical
// route-side-effect registry. Removing any group's journeys turns coverage red,
// so the coverage assertion is a real canary, not a tautology.
func TestManifestCoversAllProductJourneyGroupsAndRouteAuthorities(t *testing.T) {
	m := CanonicalProductJourneyManifest()

	t.Run("canonical manifest compiles", func(t *testing.T) {
		if _, err := m.Compile(DefaultPolicyConfig(), nil); err != nil {
			t.Fatalf("Compile(canonical) error = %v; want nil", err)
		}
	})

	t.Run("every closed journey group is covered", func(t *testing.T) {
		covered := m.CoveredGroups()
		for _, g := range ClosedJourneyGroups() {
			if !covered[g] {
				t.Errorf("manifest does not cover journey group %q", g)
			}
		}
		if !coversAllGroups(m) {
			t.Fatal("coversAllGroups(canonical) = false; want true")
		}
	})

	t.Run("every route authority is referenced", func(t *testing.T) {
		coveredRoutes := m.CoveredRoutes()
		for route := range DefaultRouteSideEffectRegistry() {
			if !coveredRoutes[route] {
				t.Errorf("manifest does not reference route authority %q", route)
			}
		}
	})

	t.Run("removing a group's journeys turns coverage red", func(t *testing.T) {
		for _, group := range []JourneyGroup{GroupSearch, GroupSynthesis, GroupModels} {
			reduced := CanonicalProductJourneyManifest()
			kept := reduced.Journeys[:0]
			for _, j := range reduced.Journeys {
				if j.Group != group {
					kept = append(kept, j)
				}
			}
			reduced.Journeys = kept
			if coversAllGroups(reduced) {
				t.Errorf("coversAllGroups() still true after removing group %q", group)
			}
			if reduced.CoveredGroups()[group] {
				t.Errorf("CoveredGroups() still reports %q after removing it", group)
			}
			// A manifest missing a canonical group also fails to compile.
			if _, err := reduced.Compile(DefaultPolicyConfig(), nil); err == nil {
				t.Errorf("Compile succeeded after removing group %q; want failure", group)
			}
		}
	})
}
