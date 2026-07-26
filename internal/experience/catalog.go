// Package experience defines the spec 106 (SCOPE-106-02) canonical
// ProductExperienceCatalog: one generated, content-free presentation-identity
// and route-binding model that every renderer (server, PWA, card) will share.
//
// The catalog is GENERATED, never handwritten. Its single source of truth is
// the `product_experience` block in config/smackerel.yaml; `./smackerel.sh
// config generate` extracts it and writes internal/experience/catalog.gen.json,
// which this package embeds via //go:embed. Handwritten navigation authorities
// (internal/web/appshell.go `appShellNav`, web/pwa/lib/appnav.js `ITEMS`) remain
// ACTIVE and untouched in this slice — the generated catalog is added ALONGSIDE
// them; the shell cutover that replaces them is a later scope (SCOPE-106-04/05).
//
// The catalog is CONTENT-FREE: it carries surface identity, hierarchy, exact
// route bindings, audiences, renderer support, and a readiness-discoverability
// policy only. It never carries session scope, evidence IDs, user content, or a
// readiness fact — readiness truth is owned by the readiness packet, not here.
package experience

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// catalog.gen.json is produced by scripts/commands/config.sh from the
// config/smackerel.yaml `product_experience` block. It is embedded (not
// re-parsed from disk) so the compiled binary carries the exact generated
// catalog.
//
//go:embed catalog.gen.json
var generatedCatalogJSON []byte

// SurfaceKind is the closed set of catalog node kinds.
//
//   - linked_leaf: a destination with an exact registered browser route, OR an
//     honestly-unavailable/dependency-pending leaf that carries no href.
//   - route_group: a route-FREE grouping node (opens its child menu, never a
//     guessed route); it MUST carry no href.
//   - utility: an authorized utility surface.
//   - local_view: a view composed within a parent's page (no independent route).
type SurfaceKind string

const (
	KindLinkedLeaf SurfaceKind = "linked_leaf"
	KindRouteGroup SurfaceKind = "route_group"
	KindUtility    SurfaceKind = "utility"
	KindLocalView  SurfaceKind = "local_view"
)

func (k SurfaceKind) valid() bool {
	switch k {
	case KindLinkedLeaf, KindRouteGroup, KindUtility, KindLocalView:
		return true
	}
	return false
}

// Renderer is the closed set of renderers that can present a surface.
type Renderer string

const (
	RendererServer Renderer = "server"
	RendererPWA    Renderer = "pwa"
	RendererCard   Renderer = "card"
)

func (r Renderer) valid() bool {
	switch r {
	case RendererServer, RendererPWA, RendererCard:
		return true
	}
	return false
}

// DiscoverabilityPolicy is the closed set of readiness-discoverability policies.
// It is the discriminator the validator uses to decide whether a surface is an
// ACTIVE leaf (must have an exact registered route), a route-free GROUP (must
// have no href), or an honestly UNAVAILABLE leaf (must have no href — never a
// guessed route).
type DiscoverabilityPolicy string

const (
	// PolicyReadyWhenJourneyReady — an active leaf discoverable when its
	// end-to-end journey is usable. It MUST bind an exact registered route.
	PolicyReadyWhenJourneyReady DiscoverabilityPolicy = "ready_when_journey_ready"
	// PolicyOperatorOnlyWhenReady — an active operator-only leaf. It MUST bind
	// an exact registered route; non-operators do not discover it.
	PolicyOperatorOnlyWhenReady DiscoverabilityPolicy = "operator_only_when_ready"
	// PolicyRouteFreeGroup — a route-free grouping node. It MUST carry no href.
	PolicyRouteFreeGroup DiscoverabilityPolicy = "route_free_group"
	// PolicyUnavailablePendingOwnership — an honestly unavailable leaf whose
	// browser route + complete journey are not yet proven (Lists/Meals/
	// Expenses). It MUST carry no href; no endpoint or parent route is guessed.
	PolicyUnavailablePendingOwnership DiscoverabilityPolicy = "unavailable_pending_ownership"
	// PolicyUnavailablePendingDependency — an honestly unavailable leaf blocked
	// on another packet (Graph -> spec 105 registering /knowledge/graph). It
	// MUST carry no href until the dependency registers the route.
	PolicyUnavailablePendingDependency DiscoverabilityPolicy = "unavailable_pending_dependency"
)

func (p DiscoverabilityPolicy) valid() bool {
	switch p {
	case PolicyReadyWhenJourneyReady, PolicyOperatorOnlyWhenReady,
		PolicyRouteFreeGroup, PolicyUnavailablePendingOwnership,
		PolicyUnavailablePendingDependency:
		return true
	}
	return false
}

// active reports whether the policy denotes an active leaf that MUST bind an
// exact registered route.
func (p DiscoverabilityPolicy) active() bool {
	return p == PolicyReadyWhenJourneyReady || p == PolicyOperatorOnlyWhenReady
}

// unavailable reports whether the policy denotes an honestly-unavailable leaf
// that MUST carry no href.
func (p DiscoverabilityPolicy) unavailable() bool {
	return p == PolicyUnavailablePendingOwnership || p == PolicyUnavailablePendingDependency
}

// Surface is one content-free presentation identity + route binding.
type Surface struct {
	// ID is the stable, cross-renderer surface identity.
	ID string `json:"id"`
	// Label is the human wayfinding label.
	Label string `json:"label"`
	// Kind is the closed-set node kind.
	Kind SurfaceKind `json:"kind"`
	// ParentID is the parent surface ID; "" denotes a root.
	ParentID string `json:"parent_id"`
	// Order is the sibling ordering key.
	Order int `json:"order"`
	// CapabilityID references the readiness capability owned by the readiness
	// packet; the catalog references it and never derives readiness truth.
	CapabilityID string `json:"capability_id"`
	// Audiences is the set of identities that may discover the surface.
	Audiences []string `json:"audiences"`
	// Href is the exact registered browser route for an active leaf, or "" for
	// a route group / honestly-unavailable / dependency-pending leaf.
	Href string `json:"href"`
	// CurrentPaths are the existing registered paths this surface preserves
	// (backward-compatible bookmarks/redirect targets). Empty for groups and
	// unavailable leaves.
	CurrentPaths []string `json:"current_paths"`
	// RendererSupport is the closed-set list of renderers that present it.
	RendererSupport []Renderer `json:"renderer_support"`
	// LocalViewID is the local-view identity when Kind is local_view; "" else.
	LocalViewID string `json:"local_view_id"`
	// ReadinessDiscoverabilityPolicy discriminates active/group/unavailable.
	ReadinessDiscoverabilityPolicy DiscoverabilityPolicy `json:"readiness_discoverability_policy"`
}

// ProductExperienceCatalog is the generated catalog: schema version + the full
// content-free surface inventory.
type ProductExperienceCatalog struct {
	SchemaVersion string    `json:"schema_version"`
	Surfaces      []Surface `json:"surfaces"`
}

// GeneratedCatalog parses the embedded, generated catalog produced by
// `./smackerel.sh config generate` from config/smackerel.yaml.
func GeneratedCatalog() (ProductExperienceCatalog, error) {
	var c ProductExperienceCatalog
	if err := json.Unmarshal(generatedCatalogJSON, &c); err != nil {
		return ProductExperienceCatalog{}, fmt.Errorf("experience: parse generated catalog.gen.json: %w", err)
	}
	if c.SchemaVersion == "" {
		return ProductExperienceCatalog{}, fmt.Errorf("experience: generated catalog has empty schema_version")
	}
	if len(c.Surfaces) == 0 {
		return ProductExperienceCatalog{}, fmt.Errorf("experience: generated catalog has zero surfaces")
	}
	return c, nil
}
