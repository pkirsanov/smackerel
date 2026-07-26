package experience

import (
	"errors"
	"strings"
	"testing"
)

// baseValid is a small, self-contained, structurally-valid catalog used as the
// clean base for each rejection mutation.
func baseValid() ProductExperienceCatalog {
	return ProductExperienceCatalog{
		SchemaVersion: "smackerel-product-experience/v1",
		Surfaces: []Surface{
			{
				ID: "work", Label: "Work", Kind: KindRouteGroup, ParentID: "", Order: 10,
				CapabilityID: "cap.work", Audiences: []string{"daily_user"}, Href: "",
				CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyRouteFreeGroup,
			},
			{
				ID: "search", Label: "Search", Kind: KindLinkedLeaf, ParentID: "", Order: 20,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "/",
				CurrentPaths: []string{"/"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			},
			{
				ID: "work_lists", Label: "Lists", Kind: KindLinkedLeaf, ParentID: "work", Order: 10,
				CapabilityID: "cap.work.lists", Audiences: []string{"daily_user"}, Href: "",
				CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyUnavailablePendingOwnership,
			},
		},
	}
}

func baseInv() RouteInventory {
	return RouteInventory{
		RegisteredPaths:   map[string]bool{"/": true},
		KnownCapabilities: map[string]bool{"cap.work": true, "cap.search": true, "cap.work.lists": true},
		KnownAudiences:    map[string]bool{"daily_user": true, "operator": true},
	}
}

// withExtra clones baseValid and appends one extra surface (isolating a single
// rejection trigger).
func withExtra(extra Surface) ProductExperienceCatalog {
	c := baseValid()
	c.Surfaces = append(append([]Surface{}, c.Surfaces...), extra)
	return c
}

func selfConsistencyInventory(c ProductExperienceCatalog) RouteInventory {
	rp := map[string]bool{}
	caps := map[string]bool{}
	for _, s := range c.Surfaces {
		if s.Href != "" {
			rp[s.Href] = true
		}
		for _, p := range s.CurrentPaths {
			rp[p] = true
		}
		if s.CapabilityID != "" {
			caps[s.CapabilityID] = true
		}
	}
	return RouteInventory{
		RegisteredPaths:   rp,
		KnownCapabilities: caps,
		KnownAudiences:    map[string]bool{"daily_user": true, "operator": true},
	}
}

func findSurface(c ProductExperienceCatalog, id string) (Surface, bool) {
	for _, s := range c.Surfaces {
		if s.ID == id {
			return s, true
		}
	}
	return Surface{}, false
}

// TestProductExperienceCatalogRejectsCyclesDuplicatesGuessedRoutesAndUnknownCapabilities
// is XP106-02-U: it proves the ExperienceRouteValidator rejects every drift
// class (cycles, duplicates, guessed/unregistered routes, unknown capabilities,
// unknown parent/audience, leaf-without-route, group-with-href, unregistered
// current path) AND that the real generated catalog is self-consistent with its
// route-free Work group and honestly-unavailable Lists/Meals/Expenses + Graph
// leaves carrying no guessed href.
func TestProductExperienceCatalogRejectsCyclesDuplicatesGuessedRoutesAndUnknownCapabilities(t *testing.T) {
	val := ExperienceRouteValidator{}

	// Sanity: the clean base validates.
	if _, err := val.Validate(baseValid(), baseInv()); err != nil {
		t.Fatalf("baseValid should validate cleanly, got: %v", err)
	}

	// The real generated catalog is structurally self-consistent.
	gen, err := GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog: %v", err)
	}
	if _, err := val.Validate(gen, selfConsistencyInventory(gen)); err != nil {
		t.Fatalf("generated catalog failed self-consistency validation: %v", err)
	}

	// Grounding assertions on the generated catalog — no invented endpoints,
	// real route bindings, route-free groups + unavailable Work/Graph leaves.
	grounding := []struct {
		id           string
		wantKind     SurfaceKind
		wantHref     string
		wantPolicy   DiscoverabilityPolicy
		wantNullHref bool
	}{
		{"today", KindLinkedLeaf, "/digest", PolicyReadyWhenJourneyReady, false},
		{"search", KindLinkedLeaf, "/", PolicyReadyWhenJourneyReady, false},
		{"cards", KindLinkedLeaf, "/cards", PolicyReadyWhenJourneyReady, false},
		{"work", KindRouteGroup, "", PolicyRouteFreeGroup, true},
		{"work_lists", KindLinkedLeaf, "", PolicyUnavailablePendingOwnership, true},
		{"work_meals", KindLinkedLeaf, "", PolicyUnavailablePendingOwnership, true},
		{"work_expenses", KindLinkedLeaf, "", PolicyUnavailablePendingOwnership, true},
		{"knowledge_graph", KindLinkedLeaf, "", PolicyUnavailablePendingDependency, true},
	}
	for _, g := range grounding {
		s, ok := findSurface(gen, g.id)
		if !ok {
			t.Fatalf("generated catalog missing expected surface %q", g.id)
		}
		if s.Kind != g.wantKind {
			t.Errorf("surface %q kind = %q, want %q", g.id, s.Kind, g.wantKind)
		}
		if s.Href != g.wantHref {
			t.Errorf("surface %q href = %q, want %q", g.id, s.Href, g.wantHref)
		}
		if s.ReadinessDiscoverabilityPolicy != g.wantPolicy {
			t.Errorf("surface %q policy = %q, want %q", g.id, s.ReadinessDiscoverabilityPolicy, g.wantPolicy)
		}
		if g.wantNullHref && s.Href != "" {
			t.Errorf("surface %q must carry no guessed href, got %q", g.id, s.Href)
		}
	}

	// Rejection table — each mutation triggers its named drift class.
	cases := []struct {
		name         string
		catalog      ProductExperienceCatalog
		inv          RouteInventory
		wantContains string
	}{
		{
			name: "duplicate id",
			catalog: withExtra(Surface{
				ID: "search", Label: "Dup", Kind: KindLinkedLeaf, ParentID: "", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "/",
				CurrentPaths: []string{"/"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "duplicate-id:search",
		},
		{
			name: "cycle",
			catalog: ProductExperienceCatalog{SchemaVersion: "v", Surfaces: []Surface{
				{ID: "a", Label: "A", Kind: KindRouteGroup, ParentID: "b", Order: 1, CapabilityID: "cap.a", Audiences: []string{"daily_user"}, Href: "", CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer}, ReadinessDiscoverabilityPolicy: PolicyRouteFreeGroup},
				{ID: "b", Label: "B", Kind: KindRouteGroup, ParentID: "a", Order: 2, CapabilityID: "cap.b", Audiences: []string{"daily_user"}, Href: "", CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer}, ReadinessDiscoverabilityPolicy: PolicyRouteFreeGroup},
			}},
			inv:          RouteInventory{KnownCapabilities: map[string]bool{"cap.a": true, "cap.b": true}, KnownAudiences: map[string]bool{"daily_user": true}},
			wantContains: "cycle:",
		},
		{
			name: "unknown parent",
			catalog: withExtra(Surface{
				ID: "orphan", Label: "Orphan", Kind: KindLinkedLeaf, ParentID: "ghost_parent", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "/",
				CurrentPaths: []string{"/"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "unknown-parent:orphan->ghost_parent",
		},
		{
			name: "unknown capability",
			catalog: withExtra(Surface{
				ID: "badcap", Label: "BadCap", Kind: KindLinkedLeaf, ParentID: "", Order: 99,
				CapabilityID: "cap.nonexistent", Audiences: []string{"daily_user"}, Href: "/",
				CurrentPaths: []string{"/"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "unknown-capability:badcap:cap.nonexistent",
		},
		{
			name: "unknown audience",
			catalog: withExtra(Surface{
				ID: "badaud", Label: "BadAud", Kind: KindLinkedLeaf, ParentID: "", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"martian"}, Href: "/",
				CurrentPaths: []string{"/"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "unknown-audience:badaud:martian",
		},
		{
			name: "active leaf without route",
			catalog: withExtra(Surface{
				ID: "noroute", Label: "NoRoute", Kind: KindLinkedLeaf, ParentID: "", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "",
				CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "active-leaf-without-route:noroute",
		},
		{
			name: "group with href",
			catalog: withExtra(Surface{
				ID: "grouphref", Label: "GroupHref", Kind: KindRouteGroup, ParentID: "", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "/guessed",
				CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyRouteFreeGroup,
			}),
			inv: baseInv(), wantContains: "group-with-href:grouphref:/guessed",
		},
		{
			name: "guessed route",
			catalog: withExtra(Surface{
				ID: "guessed", Label: "Guessed", Kind: KindLinkedLeaf, ParentID: "", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "/not-registered",
				CurrentPaths: []string{"/not-registered"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "guessed-route:guessed:/not-registered",
		},
		{
			name: "unavailable leaf with guessed href",
			catalog: withExtra(Surface{
				ID: "fakeavail", Label: "FakeAvail", Kind: KindLinkedLeaf, ParentID: "work", Order: 99,
				CapabilityID: "cap.work.lists", Audiences: []string{"daily_user"}, Href: "/api/lists",
				CurrentPaths: []string{}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyUnavailablePendingOwnership,
			}),
			inv: baseInv(), wantContains: "unavailable-leaf-with-href:fakeavail:/api/lists",
		},
		{
			name: "unregistered current path",
			catalog: withExtra(Surface{
				ID: "ghostpath", Label: "GhostPath", Kind: KindLinkedLeaf, ParentID: "", Order: 99,
				CapabilityID: "cap.search", Audiences: []string{"daily_user"}, Href: "/",
				CurrentPaths: []string{"/", "/ghost"}, RendererSupport: []Renderer{RendererServer},
				ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
			}),
			inv: baseInv(), wantContains: "unregistered-current-path:ghostpath:/ghost",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := val.Validate(tc.catalog, tc.inv)
			if err == nil {
				t.Fatalf("%s: expected *F106RouteDrift, got nil (catalog accepted)", tc.name)
			}
			var drift *F106RouteDrift
			if !errors.As(err, &drift) {
				t.Fatalf("%s: error is not *F106RouteDrift: %T %v", tc.name, err, err)
			}
			joined := strings.Join(drift.Violations, "; ")
			if !strings.Contains(joined, tc.wantContains) {
				t.Fatalf("%s: violations %q do not contain %q", tc.name, joined, tc.wantContains)
			}
		})
	}
}
