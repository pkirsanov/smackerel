package experience

import (
	"reflect"
	"sort"
	"testing"
)

// XP106-05-U (SCN-106-001, SCN-106-002, SCN-106-015) — the SCOPE-106-05 additive
// GENERATED NAVIGATION PROJECTION foundation. This is a REAL test over the real
// generated catalog + the real registered router routes (no mocks of internal
// code): it fails if the active hierarchy, audience gating, route authorization,
// or compatibility map is wrong. It proves the data model the shared-shell
// cutover will LATER consume, and asserts (adversarially) that the compatibility
// map invents no `/today` / `/work` / `/sources` / first-child fallback and
// points at no missing route. It touches NO live renderer.

// availabilityFor is the TEST acting as the readiness owner: it supplies a
// resolved-readiness availability outcome for every audience-visible surface, so
// BuildExperienceProjection (which refuses any non-readiness source) composes.
// Active leaves and route-free groups present "available"; honestly-unavailable
// leaves present "unavailable". No availability is derived from a route, flag,
// or health — every outcome carries SignalReadinessResolved.
func availabilityFor(cat ProductExperienceCatalog, audience string) map[string]SurfaceAvailabilityOutcome {
	m := map[string]SurfaceAvailabilityOutcome{}
	for _, s := range cat.Surfaces {
		if !audienceIncludes(s.Audiences, audience) {
			continue
		}
		value := AvailabilityAvailable
		if s.ReadinessDiscoverabilityPolicy.unavailable() {
			value = AvailabilityUnavailable
		}
		m[s.ID] = SurfaceAvailabilityOutcome{Signal: SignalReadinessResolved, Value: value}
	}
	return m
}

func buildProjectionT(t *testing.T, cat ProductExperienceCatalog, audience, current string) ExperienceProjection {
	t.Helper()
	proj, err := BuildExperienceProjection(cat, ProjectionRequest{
		Audience:         audience,
		CurrentSurfaceID: current,
		Appearance:       ShellAppearance{Theme: ShellThemeSystem, Density: ShellDensityComfortable},
		Availability:     availabilityFor(cat, audience),
	})
	if err != nil {
		t.Fatalf("BuildExperienceProjection(%s, current=%q) unexpected error: %v", audience, current, err)
	}
	return proj
}

func buildNavT(t *testing.T, cat ProductExperienceCatalog, audience, current string, hasSession bool) NavigationProjection {
	t.Helper()
	proj := buildProjectionT(t, cat, audience, current)
	np, err := BuildNavigationProjection(cat, proj, Principal{Audience: audience, HasSession: hasSession})
	if err != nil {
		t.Fatalf("BuildNavigationProjection(%s) unexpected error: %v", audience, err)
	}
	return np
}

func projectedByID(p ExperienceProjection, id string) (ProjectedSurface, bool) {
	for _, s := range p.Surfaces {
		if s.SurfaceID == id {
			return s, true
		}
	}
	return ProjectedSurface{}, false
}

func TestGeneratedNavigationActiveHierarchyAudienceRoutesAndCompatibilityMap(t *testing.T) {
	cat, err := GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog: %v", err)
	}

	// ── Active hierarchy: a representative current route resolves to its surface
	// and marks the correct active + parent-active surfaces. ────────────────────
	t.Run("ActiveHierarchy", func(t *testing.T) {
		// Resolve a representative browser route to its surface id THROUGH the
		// projection (no hardcoded id) then rebuild with that current surface.
		base := buildNavT(t, cat, "daily_user", "", true)
		wiki := base.ResolveRoute("/pwa/wiki.html")
		if !wiki.Exists || wiki.SurfaceID != "knowledge_wiki" {
			t.Fatalf("/pwa/wiki.html resolved to %+v, want exists=true surface=knowledge_wiki", wiki)
		}

		np := buildNavT(t, cat, "daily_user", wiki.SurfaceID, true)
		child, ok := projectedByID(np.Projection, "knowledge_wiki")
		if !ok || !child.Current || child.ParentCurrent {
			t.Fatalf("knowledge_wiki active state = {current=%v parentCurrent=%v ok=%v}, want current=true parentCurrent=false", child.Current, child.ParentCurrent, ok)
		}
		parent, ok := projectedByID(np.Projection, "knowledge")
		if !ok || parent.Current || !parent.ParentCurrent {
			t.Fatalf("knowledge active state = {current=%v parentCurrent=%v ok=%v}, want current=false parentCurrent=true", parent.Current, parent.ParentCurrent, ok)
		}
		// A sibling root (search) is neither current nor parent-current.
		if search, ok := projectedByID(np.Projection, "search"); !ok || search.Current || search.ParentCurrent {
			t.Fatalf("search active state = {current=%v parentCurrent=%v ok=%v}, want both false", search.Current, search.ParentCurrent, ok)
		}

		// A root current route marks itself current with no parent-active.
		rootNP := buildNavT(t, cat, "daily_user", "search", true)
		if s, ok := projectedByID(rootNP.Projection, "search"); !ok || !s.Current || s.ParentCurrent {
			t.Fatalf("search-as-current = {current=%v parentCurrent=%v ok=%v}, want current=true parentCurrent=false", s.Current, s.ParentCurrent, ok)
		}
	})

	// ── Audience + routes: audience gating hides an unauthorized surface from
	// VISIBILITY while its route still EXISTS and rechecks auth. 401 and 403 are
	// distinct; a daily user gets 403 (not a login loop) on an operator route. ──
	t.Run("AudienceAndRoutes", func(t *testing.T) {
		// daily = authenticated daily user; dailyNoSession = unauthenticated.
		daily := buildNavT(t, cat, "daily_user", "", true)
		dailyNoSession := buildNavT(t, cat, "daily_user", "", false)
		operator := buildNavT(t, cat, "operator", "", true)

		const adminRoute = "/pwa/model-connections.html"

		// Operator-only surfaces are OMITTED from a daily user's visible nav.
		if daily.Visible("admin") || daily.Visible("admin_models") {
			t.Fatalf("daily user must not see admin/admin_models: admin=%v admin_models=%v", daily.Visible("admin"), daily.Visible("admin_models"))
		}
		if _, seen := projectedByID(daily.Projection, "admin_models"); seen {
			t.Fatalf("admin_models leaked into daily user's visible projection")
		}

		// ...yet the route still EXISTS and a daily user's direct access is a
		// DISTINCT 403 access-denied, never a 401 login loop.
		dRes := daily.ResolveRoute(adminRoute)
		if !dRes.Exists || dRes.Visible || dRes.Outcome != AuthAccessDenied {
			t.Fatalf("daily direct access to %s = %+v, want exists=true visible=false outcome=access_denied", adminRoute, dRes)
		}
		// Without a session the same route is a DISTINCT 401 session-ended.
		nRes := dailyNoSession.ResolveRoute(adminRoute)
		if !nRes.Exists || nRes.Outcome != AuthSessionEnded {
			t.Fatalf("no-session access to %s = %+v, want exists=true outcome=session_ended", adminRoute, nRes)
		}
		// 401 and 403 MUST remain distinct concepts.
		if AuthSessionEnded == AuthAccessDenied || nRes.Outcome == dRes.Outcome {
			t.Fatalf("401 (session_ended) and 403 (access_denied) must be distinct: 401=%q 403=%q", nRes.Outcome, dRes.Outcome)
		}
		// An operator IS authorized and DOES see the surface.
		oRes := operator.ResolveRoute(adminRoute)
		if !oRes.Exists || !oRes.Visible || oRes.Outcome != AuthAuthorized {
			t.Fatalf("operator access to %s = %+v, want exists=true visible=true outcome=authorized", adminRoute, oRes)
		}

		// A permitted route for a daily user is authorized + visible.
		if r := daily.ResolveRoute("/"); !r.Exists || !r.Visible || r.SurfaceID != "search" || r.Outcome != AuthAuthorized {
			t.Fatalf("daily access to / = %+v, want exists=true visible=true surface=search outcome=authorized", r)
		}
		// A path bound by no surface is a routing 404 (Exists=false), not an auth
		// outcome — distinct from a 403.
		if r := daily.ResolveRoute("/no/such/route"); r.Exists || r.Outcome != "" {
			t.Fatalf("unbound route = %+v, want exists=false outcome=\"\"", r)
		}
	})

	// ── Compatibility map: every entry maps a REAL current path to a REAL target
	// or explicit redirect; adversarially prove no invented path / first-child
	// fallback / missing route. ─────────────────────────────────────────────────
	t.Run("CompatibilityMap", func(t *testing.T) {
		cm, err := BuildCompatibilityMap(cat)
		if err != nil {
			t.Fatalf("BuildCompatibilityMap: %v", err)
		}
		if len(cm.Entries) == 0 {
			t.Fatal("compatibility map is empty")
		}

		byID := map[string]Surface{}
		for _, s := range cat.Surfaces {
			byID[s.ID] = s
		}

		// Completeness: the compat sources are EXACTLY the active leaves'
		// preserved current paths — nothing invented, nothing dropped.
		var wantPaths []string
		for _, s := range cat.Surfaces {
			if s.ReadinessDiscoverabilityPolicy.active() {
				wantPaths = append(wantPaths, s.CurrentPaths...)
			}
		}
		var gotPaths []string
		for _, e := range cm.Entries {
			gotPaths = append(gotPaths, e.CurrentPath)
		}
		sort.Strings(wantPaths)
		sort.Strings(gotPaths)
		if !reflect.DeepEqual(wantPaths, gotPaths) {
			t.Fatalf("compat current paths = %v, want exactly the active-leaf current paths %v", gotPaths, wantPaths)
		}

		// Cross-check every target/source against the REAL router + PWA tree:
		// no entry may point at a missing route (no invented endpoints).
		root, err := repoRoot()
		if err != nil {
			t.Fatalf("repoRoot: %v", err)
		}
		serverRoutes, err := serverRouteLiterals(root)
		if err != nil {
			t.Fatalf("serverRouteLiterals: %v", err)
		}

		for _, e := range cm.Entries {
			if e.CurrentPath == "" || e.Target == "" {
				t.Fatalf("compat entry has empty path/target: %+v", e)
			}
			s, ok := byID[e.SurfaceID]
			if !ok {
				t.Fatalf("compat entry surface %q not in catalog", e.SurfaceID)
			}
			// No first-child fallback: an entry ALWAYS belongs to an active leaf
			// (never a route-free group / unavailable leaf), and its target is
			// that same surface's own canonical Href.
			if !s.ReadinessDiscoverabilityPolicy.active() {
				t.Fatalf("compat entry %q sourced from non-active surface (first-child fallback shape)", e.SurfaceID)
			}
			if e.Target != s.Href {
				t.Fatalf("compat target %q != owning surface %q Href %q", e.Target, s.ID, s.Href)
			}
			if e.Redirect != (e.CurrentPath != s.Href) {
				t.Fatalf("compat redirect flag wrong for %+v (Href=%q)", e, s.Href)
			}
			// The target MUST resolve to a real registered route / static asset.
			if !resolves(root, e.Target, serverRoutes) {
				t.Fatalf("compat target %q does not resolve to a real route", e.Target)
			}
			if !resolves(root, e.CurrentPath, serverRoutes) {
				t.Fatalf("compat current path %q does not resolve to a real route", e.CurrentPath)
			}
		}

		// Adversarial: the classic invented paths and route-free group paths must
		// NEVER appear as a source OR a target.
		for _, invented := range []string{"/today", "/work", "/sources", "/admin", "/knowledge/graph"} {
			if _, ok := cm.Lookup(invented); ok {
				t.Fatalf("compat map invented a source for %q", invented)
			}
			for _, e := range cm.Entries {
				if e.CurrentPath == invented || e.Target == invented {
					t.Fatalf("compat map references invented path %q in %+v", invented, e)
				}
			}
		}

		// Honestly-unavailable leaves and route-free groups contribute NOTHING.
		for _, id := range []string{"knowledge_graph", "work", "work_lists", "work_meals", "work_expenses", "sources", "admin"} {
			for _, e := range cm.Entries {
				if e.SurfaceID == id {
					t.Fatalf("unavailable/group surface %q produced a compat entry %+v", id, e)
				}
			}
		}
	})

	// ── Deterministic: identical inputs yield an identical projection digest,
	// supported-path set, and compatibility map (no hidden fallback / ordering
	// nondeterminism). ──────────────────────────────────────────────────────────
	t.Run("Deterministic", func(t *testing.T) {
		a := buildNavT(t, cat, "daily_user", "search", true)
		b := buildNavT(t, cat, "daily_user", "search", true)
		if a.Projection.ProjectionDigest() != b.Projection.ProjectionDigest() {
			t.Fatalf("projection digest not deterministic: %q vs %q", a.Projection.ProjectionDigest(), b.Projection.ProjectionDigest())
		}
		if !reflect.DeepEqual(a.SupportedPaths(), b.SupportedPaths()) {
			t.Fatalf("supported paths not deterministic: %v vs %v", a.SupportedPaths(), b.SupportedPaths())
		}
		cm1, err1 := BuildCompatibilityMap(cat)
		cm2, err2 := BuildCompatibilityMap(cat)
		if err1 != nil || err2 != nil {
			t.Fatalf("BuildCompatibilityMap errors: %v / %v", err1, err2)
		}
		if !reflect.DeepEqual(cm1, cm2) {
			t.Fatalf("compatibility map not deterministic")
		}
	})

	// ── Fail-closed: malformed input yields a typed *F106PresentationError, never
	// an optimistic or static fallback. ─────────────────────────────────────────
	t.Run("FailClosed", func(t *testing.T) {
		proj := buildProjectionT(t, cat, "daily_user", "")

		// Empty principal audience.
		if _, err := BuildNavigationProjection(cat, proj, Principal{Audience: "", HasSession: true}); err == nil {
			t.Fatal("expected error for empty principal audience")
		} else if _, ok := err.(*F106PresentationError); !ok {
			t.Fatalf("empty-audience error type = %T, want *F106PresentationError", err)
		}

		// Projection built for daily_user but principal claims operator.
		if _, err := BuildNavigationProjection(cat, proj, Principal{Audience: "operator", HasSession: true}); err == nil {
			t.Fatal("expected error for projection/principal audience mismatch")
		} else if _, ok := err.(*F106PresentationError); !ok {
			t.Fatalf("audience-mismatch error type = %T, want *F106PresentationError", err)
		}

		// A route-free group carrying a preserved path is a malformed catalog:
		// the route index and the compatibility map both fail closed.
		malformed := ProductExperienceCatalog{
			SchemaVersion: cat.SchemaVersion,
			Surfaces: []Surface{
				{ID: "assistant", Label: "Assistant", Kind: KindLinkedLeaf, Order: 10, Audiences: []string{"daily_user"}, Href: "/assistant", CurrentPaths: []string{"/assistant"}, ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady},
				{ID: "work", Label: "Work", Kind: KindRouteGroup, Order: 20, Audiences: []string{"daily_user"}, Href: "", CurrentPaths: []string{"/work"}, ReadinessDiscoverabilityPolicy: PolicyRouteFreeGroup},
			},
		}
		if _, err := buildRouteIndex(malformed); err == nil {
			t.Fatal("expected buildRouteIndex to fail closed on a group carrying a path")
		} else if _, ok := err.(*F106PresentationError); !ok {
			t.Fatalf("buildRouteIndex error type = %T, want *F106PresentationError", err)
		}
		if _, err := BuildCompatibilityMap(malformed); err == nil {
			t.Fatal("expected BuildCompatibilityMap to fail closed on a group carrying a path")
		} else if _, ok := err.(*F106PresentationError); !ok {
			t.Fatalf("BuildCompatibilityMap error type = %T, want *F106PresentationError", err)
		}
	})
}
