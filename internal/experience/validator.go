package experience

import (
	"fmt"
	"sort"
	"strings"
)

// RouteInventory is the universe a catalog is validated against: the set of
// REAL registered browser routes, plus the known capability and audience
// universes. On a successful Validate it is returned with ResolvedBindings
// populated (surface ID -> exact bound href for each active leaf).
type RouteInventory struct {
	// RegisteredPaths is the set of exact browser routes that actually exist.
	// A nil map disables the registration check (non-emptiness of an active
	// leaf's href is still enforced) — tests that only exercise structural
	// rules may pass nil.
	RegisteredPaths map[string]bool
	// KnownCapabilities is the set of readiness capability IDs the catalog may
	// reference. A nil map disables the membership check (non-emptiness is
	// still enforced).
	KnownCapabilities map[string]bool
	// KnownAudiences is the closed audience universe. A nil map disables the
	// membership check.
	KnownAudiences map[string]bool
	// ResolvedBindings is populated by Validate on success: surface ID -> href
	// for every active leaf that bound an exact registered route.
	ResolvedBindings map[string]string
}

// F106RouteDrift is the typed catalog/route drift failure. Its Violations slice
// is deterministic (stable order) so callers and tests can assert on it.
type F106RouteDrift struct {
	Violations []string
}

func (e *F106RouteDrift) Error() string {
	return "F106RouteDrift: " + strings.Join(e.Violations, "; ")
}

// ExperienceRouteValidator validates a ProductExperienceCatalog against a
// RouteInventory. It is the single gate that rejects a catalog which invents a
// route, guesses an href, cycles, duplicates an identity, or references an
// unknown parent/capability/audience.
type ExperienceRouteValidator struct{}

// Validate checks catalog against inv and returns the resolved RouteInventory
// (with ResolvedBindings) on success, or a *F106RouteDrift enumerating every
// violation on failure. It rejects:
//
//   - duplicate surface IDs
//   - parent cycles
//   - unknown parent / capability / audience / kind / renderer / policy
//   - an active leaf without an exact registered route (leaf-without-route)
//   - an active leaf whose href is not registered (guessed-route)
//   - an active leaf whose href is absent from its own current paths
//   - a route group carrying an href (group-with-href)
//   - an honestly-unavailable leaf carrying an href (guessed route on an
//     unavailable leaf)
//   - a current path that is not registered
//   - a kind/policy mismatch (route_free_group not on a route_group)
func (ExperienceRouteValidator) Validate(catalog ProductExperienceCatalog, inv RouteInventory) (RouteInventory, error) {
	var v []string

	idCount := map[string]int{}
	byID := map[string]Surface{}
	for _, s := range catalog.Surfaces {
		idCount[s.ID]++
		byID[s.ID] = s
	}

	// Duplicate IDs (sorted for determinism).
	var dups []string
	for id, n := range idCount {
		if n > 1 {
			dups = append(dups, id)
		}
	}
	sort.Strings(dups)
	for _, id := range dups {
		v = append(v, "duplicate-id:"+id)
	}

	resolved := map[string]string{}

	for _, s := range catalog.Surfaces {
		if !s.Kind.valid() {
			v = append(v, fmt.Sprintf("unknown-kind:%s:%s", s.ID, s.Kind))
		}
		if !s.ReadinessDiscoverabilityPolicy.valid() {
			v = append(v, fmt.Sprintf("unknown-policy:%s:%s", s.ID, s.ReadinessDiscoverabilityPolicy))
		}
		for _, r := range s.RendererSupport {
			if !r.valid() {
				v = append(v, fmt.Sprintf("unknown-renderer:%s:%s", s.ID, r))
			}
		}
		if s.CapabilityID == "" || (inv.KnownCapabilities != nil && !inv.KnownCapabilities[s.CapabilityID]) {
			v = append(v, fmt.Sprintf("unknown-capability:%s:%s", s.ID, s.CapabilityID))
		}
		for _, a := range s.Audiences {
			if inv.KnownAudiences != nil && !inv.KnownAudiences[a] {
				v = append(v, fmt.Sprintf("unknown-audience:%s:%s", s.ID, a))
			}
		}
		if s.ParentID != "" {
			if _, ok := byID[s.ParentID]; !ok {
				v = append(v, fmt.Sprintf("unknown-parent:%s->%s", s.ID, s.ParentID))
			}
		}

		switch {
		case s.ReadinessDiscoverabilityPolicy == PolicyRouteFreeGroup:
			if s.Kind != KindRouteGroup {
				v = append(v, fmt.Sprintf("kind-policy-mismatch:%s:%s/%s", s.ID, s.Kind, s.ReadinessDiscoverabilityPolicy))
			}
			if s.Href != "" {
				v = append(v, fmt.Sprintf("group-with-href:%s:%s", s.ID, s.Href))
			}
		case s.ReadinessDiscoverabilityPolicy.unavailable():
			if s.Href != "" {
				v = append(v, fmt.Sprintf("unavailable-leaf-with-href:%s:%s", s.ID, s.Href))
			}
		case s.ReadinessDiscoverabilityPolicy.active():
			if s.Href == "" {
				v = append(v, fmt.Sprintf("active-leaf-without-route:%s", s.ID))
			} else {
				if inv.RegisteredPaths != nil && !inv.RegisteredPaths[s.Href] {
					v = append(v, fmt.Sprintf("guessed-route:%s:%s", s.ID, s.Href))
				} else {
					resolved[s.ID] = s.Href
				}
				if !contains(s.CurrentPaths, s.Href) {
					v = append(v, fmt.Sprintf("href-not-in-current-paths:%s:%s", s.ID, s.Href))
				}
			}
		}

		for _, p := range s.CurrentPaths {
			if inv.RegisteredPaths != nil && !inv.RegisteredPaths[p] {
				v = append(v, fmt.Sprintf("unregistered-current-path:%s:%s", s.ID, p))
			}
		}
	}

	// Cycle detection (report each node whose ancestor chain loops, once,
	// sorted for determinism).
	cyclicIDs := map[string]bool{}
	for _, s := range catalog.Surfaces {
		if hasCycle(s.ID, byID) {
			cyclicIDs[s.ID] = true
		}
	}
	var cyc []string
	for id := range cyclicIDs {
		cyc = append(cyc, id)
	}
	sort.Strings(cyc)
	for _, id := range cyc {
		v = append(v, "cycle:"+id)
	}

	if len(v) > 0 {
		return RouteInventory{}, &F106RouteDrift{Violations: v}
	}

	out := inv
	out.ResolvedBindings = resolved
	return out, nil
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// hasCycle reports whether following ParentID from start ever revisits a node.
func hasCycle(start string, byID map[string]Surface) bool {
	visited := map[string]bool{}
	cur := start
	for cur != "" {
		if visited[cur] {
			return true
		}
		visited[cur] = true
		s, ok := byID[cur]
		if !ok {
			return false // unknown parent — reported separately
		}
		cur = s.ParentID
	}
	return false
}
