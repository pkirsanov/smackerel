package experience

// navigation_projection.go is the spec 106 (SCOPE-106-05, additive foundation
// slice XP106-05-U) GENERATED NAVIGATION PROJECTION model. It is the
// content-free data model the shared-shell cutover will LATER wire into the
// server, PWA, and Card renderers. It touches NO live renderer, route, session
// mechanism, or capability claim in this slice — it only derives, from the real
// catalog + real consumer inventory, the navigation projection + compatibility
// map the cutover will consume.
//
// It COMPOSES with the committed foundations rather than duplicating them:
//
//   - The VISIBLE, audience-gated, active-hierarchy, availability-presented
//     navigation is the existing renderer-neutral ExperienceProjection
//     (renderer_projection.go, BuildExperienceProjection) built from the SAME
//     generated catalog (catalog.go, SCOPE-106-02). This file never re-declares
//     a surface or re-embeds the catalog; it consumes an already-built
//     ExperienceProjection.
//   - Route AUTHORIZATION is resolved INDEPENDENTLY of navigation visibility
//     over the FULL catalog: an operator-only surface (Admin, Models) is OMITTED
//     from a daily user's visible projection, yet its route still EXISTS and a
//     direct access still rechecks authorization. It reuses the existing
//     AuthOutcome vocabulary (auth_adapter.go) so 401 (session ended) and 403
//     (access denied) stay DISTINCT concepts and the same
//     AuthenticatedRequestAdapter presents the result; a daily user hitting an
//     operator-only route receives a 403 access-denied, never a 401 login loop.
//   - The COMPATIBILITY MAP preserves every currently-supported path, mapping it
//     to its intended target (the owning surface's exact canonical route) or an
//     EXPLICIT compatible redirect. Both are drawn ONLY from the real catalog —
//     it NEVER invents `/today`, `/work`, `/sources`, or a first-child fallback,
//     and an unavailable destination (a route-free group or an honestly-
//     unavailable leaf) contributes nothing, so no entry ever points at a
//     missing route.
//
// Everything here is PURE, DETERMINISTIC, and FAIL-CLOSED: every builder returns
// a typed *F106PresentationError on any inconsistency and NEVER emits an
// optimistic or static-ready fallback.

import (
	"sort"
	"strings"
)

// Principal is the content-free identity context a navigation projection is
// resolved for: the audience token the surfaces gate on and whether an
// authenticated session is present. It carries NO user id, name, session token,
// or content.
type Principal struct {
	// Audience is the principal's audience token (e.g. "daily_user",
	// "operator"); only surfaces whose Audiences include it are visible.
	Audience string
	// HasSession reports whether an authenticated session is present. It gates
	// the 401-vs-403 distinction on a direct route access and never affects
	// which surfaces the catalog binds.
	HasSession bool
}

// RouteResolution is the independent-of-visibility resolution of a direct path
// access for a principal. Exists reports a real surface binds the path; Visible
// reports whether that surface appears in the principal's visible navigation;
// Outcome is the authenticated-request classification for the direct access,
// meaningful only when Exists is true. Outcome reuses the existing AuthOutcome
// vocabulary (authorized / session_ended [401] / access_denied [403]) so the
// same AuthenticatedRequestAdapter presents it. Exists stays TRUE and Outcome
// stays a real recheck even when Visible is FALSE — visibility never grants,
// implies, or gates authorization — and a path bound by no surface (Exists
// false) is a routing 404, never an auth outcome.
type RouteResolution struct {
	Path      string
	SurfaceID string
	Exists    bool
	Visible   bool
	Outcome   AuthOutcome
}

// routeBinding is the FULL-catalog binding for one supported path: the owning
// surface id and the audiences permitted to access it.
type routeBinding struct {
	surfaceID string
	audiences []string
}

// NavigationProjection is the generated navigation projection for one principal.
// Projection is the visible, audience-gated, active-hierarchy, availability-
// presented navigation (the composed ExperienceProjection). ResolveRoute answers
// a direct-access authorization over the FULL catalog, independent of what
// Projection makes visible.
type NavigationProjection struct {
	Projection ExperienceProjection
	Principal  Principal

	routes     map[string]routeBinding
	visibleIDs map[string]bool
}

// BuildNavigationProjection composes the generated navigation projection for a
// principal from an ALREADY-BUILT ExperienceProjection (its visible, audience-
// gated, deterministically ordered, active/parent-active, readiness-presented
// navigation) plus the SAME generated catalog the projection derived from. On
// top of the visible projection it builds a FULL-catalog route-authorization
// index so a direct access to any bound route is rechecked independently of
// visibility.
//
// It FAILS CLOSED to a typed *F106PresentationError on an empty principal
// audience, a projection/principal audience mismatch, an empty projection, a
// malformed catalog (the route index rejects it), or a projected surface that is
// absent from the catalog the route index was built from. It NEVER emits an
// optimistic or static-ready fallback.
func BuildNavigationProjection(cat ProductExperienceCatalog, proj ExperienceProjection, principal Principal) (NavigationProjection, error) {
	var violations []string
	if strings.TrimSpace(principal.Audience) == "" {
		violations = append(violations, "empty-principal-audience")
	}
	if proj.Audience != principal.Audience {
		violations = append(violations, "projection-principal-audience-mismatch")
	}
	if len(proj.Surfaces) == 0 {
		violations = append(violations, "empty-projection")
	}
	if len(violations) > 0 {
		return NavigationProjection{}, &F106PresentationError{Surface: "navigation", Violations: dedupeSorted(violations)}
	}

	routes, err := buildRouteIndex(cat)
	if err != nil {
		return NavigationProjection{}, err
	}

	// Every visible projected surface MUST exist in the catalog the route index
	// was built from (guards a mismatched projection/catalog pair).
	catIDs := make(map[string]bool, len(cat.Surfaces))
	for _, s := range cat.Surfaces {
		catIDs[s.ID] = true
	}
	visibleIDs := make(map[string]bool, len(proj.Surfaces))
	for _, s := range proj.Surfaces {
		if !catIDs[s.SurfaceID] {
			violations = append(violations, "projected-surface-absent-from-catalog:"+s.SurfaceID)
			continue
		}
		visibleIDs[s.SurfaceID] = true
	}
	if len(violations) > 0 {
		return NavigationProjection{}, &F106PresentationError{Surface: "navigation", Violations: dedupeSorted(violations)}
	}

	return NavigationProjection{
		Projection: proj,
		Principal:  principal,
		routes:     routes,
		visibleIDs: visibleIDs,
	}, nil
}

// buildRouteIndex maps every supported path (each active leaf's canonical Href
// and its preserved current paths) to its FULL-catalog binding. Route-free
// groups and honestly-unavailable leaves bind no path, so they contribute
// nothing. It FAILS CLOSED on a malformed catalog (an active leaf missing its
// route, or a group / unavailable leaf carrying one, or two surfaces claiming
// the same path) so the index never contains an invented or missing route.
func buildRouteIndex(cat ProductExperienceCatalog) (map[string]routeBinding, error) {
	var violations []string
	routes := map[string]routeBinding{}
	bind := func(path, id string, auds []string) {
		if path == "" {
			return
		}
		if existing, dup := routes[path]; dup && existing.surfaceID != id {
			violations = append(violations, "duplicate-route-path:"+path)
			return
		}
		routes[path] = routeBinding{surfaceID: id, audiences: auds}
	}
	for _, s := range cat.Surfaces {
		if s.ReadinessDiscoverabilityPolicy.active() {
			if s.Href == "" {
				violations = append(violations, "active-leaf-without-href:"+s.ID)
				continue
			}
			bind(s.Href, s.ID, s.Audiences)
			for _, p := range s.CurrentPaths {
				bind(p, s.ID, s.Audiences)
			}
			continue
		}
		// Route-free groups and honestly-unavailable leaves MUST bind no route:
		// a href or preserved path on one is a malformed catalog.
		if s.Href != "" {
			violations = append(violations, "nonactive-surface-with-href:"+s.ID)
		}
		if len(s.CurrentPaths) > 0 {
			violations = append(violations, "nonactive-surface-with-current-paths:"+s.ID)
		}
	}
	if len(routes) == 0 {
		violations = append(violations, "no-bound-routes")
	}
	if len(violations) > 0 {
		return nil, &F106PresentationError{Surface: "navigation", Violations: dedupeSorted(violations)}
	}
	return routes, nil
}

// ResolveRoute resolves a direct access to path for the projection's principal,
// INDEPENDENTLY of navigation visibility. A path bound by no surface has
// Exists=false and an empty Outcome (a routing 404, never an auth outcome). A
// bound path always Exists and is always rechecked: without a session it is
// AuthSessionEnded (401); with a session but a non-permitted audience it is
// AuthAccessDenied (403, never a login loop); with a permitted audience it is
// AuthAuthorized. Visible reflects only whether the surface appears in the
// visible projection and never changes the outcome.
func (np NavigationProjection) ResolveRoute(path string) RouteResolution {
	b, ok := np.routes[path]
	if !ok {
		return RouteResolution{Path: path, Exists: false, Visible: false, Outcome: ""}
	}
	permitted := audienceIncludes(b.audiences, np.Principal.Audience)
	var outcome AuthOutcome
	switch {
	case !np.Principal.HasSession:
		// Authentication precedes authorization: no session is a 401.
		outcome = AuthSessionEnded
	case permitted:
		outcome = AuthAuthorized
	default:
		// Authenticated but wrong audience: a DISTINCT 403, never a 401 loop.
		outcome = AuthAccessDenied
	}
	return RouteResolution{
		Path:      path,
		SurfaceID: b.surfaceID,
		Exists:    true,
		Visible:   np.visibleIDs[b.surfaceID],
		Outcome:   outcome,
	}
}

// Visible reports whether a surface id appears in the principal's visible
// navigation projection. It is a pure lookup over the composed projection.
func (np NavigationProjection) Visible(surfaceID string) bool {
	return np.visibleIDs[surfaceID]
}

// SupportedPaths returns every path the FULL catalog binds, sorted — the exact
// set a direct access can resolve to a real surface (never an invented route).
func (np NavigationProjection) SupportedPaths() []string {
	paths := make([]string, 0, len(np.routes))
	for p := range np.routes {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// ── Compatibility map ────────────────────────────────────────────────────────

// CompatibilityEntry is one preserved current path and the intended target it
// reaches. Redirect is false when the path reaches its intended content directly
// (Target == CurrentPath) and true when it is an EXPLICIT compatible redirect to
// the owning surface's canonical route (Target != CurrentPath). Target is always
// the owning surface's exact canonical Href — a real registered route, NEVER an
// invented path or a first-child fallback.
type CompatibilityEntry struct {
	CurrentPath string
	Target      string
	SurfaceID   string
	Redirect    bool
}

// CompatibilityMap is the closed set of preserved-path -> intended-target
// entries. It is derived ONLY from the catalog's active leaves' preserved
// current paths: every Target is the owning surface's exact canonical Href.
// Route-free groups and honestly-unavailable leaves contribute nothing (they
// bind no path and no route), so no entry ever sources from or points at a
// missing route, and no `/today` / `/work` / `/sources` / first-child fallback
// can appear.
type CompatibilityMap struct {
	Entries []CompatibilityEntry
}

// BuildCompatibilityMap builds the deterministic, fail-closed compatibility map
// from the catalog. It FAILS CLOSED to a typed *F106PresentationError when an
// active leaf preserves a path but binds no canonical route (an unavailable
// destination mapping to a missing route), when a route-free group or honestly-
// unavailable leaf carries a preserved path (a malformed catalog), when a
// preserved path is empty, or when two surfaces claim the same current path. It
// NEVER invents a path or a redirect and NEVER emits a fallback.
func BuildCompatibilityMap(cat ProductExperienceCatalog) (CompatibilityMap, error) {
	var violations []string
	var entries []CompatibilityEntry
	owner := map[string]string{} // current path -> surface id (duplicate guard)

	for _, s := range cat.Surfaces {
		if !s.ReadinessDiscoverabilityPolicy.active() {
			// A route-free group / honestly-unavailable leaf preserves no path.
			if len(s.CurrentPaths) > 0 {
				violations = append(violations, "nonactive-surface-with-current-paths:"+s.ID)
			}
			continue
		}
		for _, p := range s.CurrentPaths {
			if p == "" {
				violations = append(violations, "empty-current-path:"+s.ID)
				continue
			}
			if prev, dup := owner[p]; dup && prev != s.ID {
				violations = append(violations, "duplicate-current-path:"+p)
				continue
			}
			owner[p] = s.ID
			if s.Href == "" {
				// An active leaf that preserves a path MUST bind a canonical
				// route; otherwise the path would map to a missing route.
				violations = append(violations, "current-path-without-target:"+s.ID)
				continue
			}
			entries = append(entries, CompatibilityEntry{
				CurrentPath: p,
				Target:      s.Href,
				SurfaceID:   s.ID,
				Redirect:    p != s.Href,
			})
		}
	}
	if len(entries) == 0 {
		violations = append(violations, "no-compatibility-entries")
	}
	if len(violations) > 0 {
		return CompatibilityMap{}, &F106PresentationError{Surface: "compatibility", Violations: dedupeSorted(violations)}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CurrentPath < entries[j].CurrentPath
	})
	return CompatibilityMap{Entries: entries}, nil
}

// Lookup returns the compatibility entry for a current path, if present.
func (m CompatibilityMap) Lookup(path string) (CompatibilityEntry, bool) {
	for _, e := range m.Entries {
		if e.CurrentPath == path {
			return e, true
		}
	}
	return CompatibilityEntry{}, false
}
