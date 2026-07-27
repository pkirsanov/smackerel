package graphapi

// manifest.go — the canonical Knowledge Graph route manifest and its
// atomic-registration validator for BUG-080-001 SCN-080-001-02.
//
// design.md ("Atomic Wiring And Route Registration") requires that the
// eight graph family routes register as ONE group and that the router
// "validates this complete manifest before calling Chi": removing OR
// duplicating any required family REJECTS construction (fail-loud)
// rather than silently mounting a SUBSET — the very silent-absence class
// this bug fix exists to eliminate. The fail-soft Guard in activation.go
// already keeps every path PRESENT (typed 503 when disabled); this file
// supplies the complementary contract that the ENABLED path can never
// mount fewer than the eight required families.
//
// The validator is a pure, hermetically unit-testable function that
// imports no router, no datastore, and no secret material. Its error
// text names ONLY the family and the route contract (value-safe): a
// secret value, its length, hash, or any derivative NEVER appears.

import (
	"fmt"
	"net/http"
	"strings"
)

// CodeRouteManifestIncomplete is the value-safe construction-failure
// code emitted when the graph route manifest is missing, duplicates, or
// otherwise malforms any required family. It mirrors design.md's error
// model (F080-ROUTE-MANIFEST-INCOMPLETE) and names only the family and
// the route contract, never secret material.
const CodeRouteManifestIncomplete = "F080-ROUTE-MANIFEST-INCOMPLETE"

// GraphRouteFamily is the closed set of the eight required graph
// families. A valid manifest registers each family exactly once.
type GraphRouteFamily string

const (
	FamilyTopics       GraphRouteFamily = "topics"
	FamilyTopicDetail  GraphRouteFamily = "topic_detail"
	FamilyPeople       GraphRouteFamily = "people"
	FamilyPersonDetail GraphRouteFamily = "person_detail"
	FamilyPlaces       GraphRouteFamily = "places"
	FamilyPlaceDetail  GraphRouteFamily = "place_detail"
	FamilyTime         GraphRouteFamily = "time"
	FamilyEdges        GraphRouteFamily = "edges"
)

// requiredGraphFamilies is the canonical, ordered eight-family set. A
// valid manifest contains exactly these families, each exactly once.
var requiredGraphFamilies = []GraphRouteFamily{
	FamilyTopics, FamilyTopicDetail,
	FamilyPeople, FamilyPersonDetail,
	FamilyPlaces, FamilyPlaceDetail,
	FamilyTime, FamilyEdges,
}

// GraphRouteEntry is one row of the canonical route manifest: a family,
// its HTTP method, its chi path template (the exact pattern
// internal/api/router.go registers), and the scope every graph route
// requires.
type GraphRouteEntry struct {
	Family       GraphRouteFamily
	Method       string
	PathTemplate string
	Scope        string
}

// CanonicalGraphRouteManifest returns design.md's "Canonical Route
// Manifest": the eight family routes, every one a GET behind
// knowledge-graph:read. The PathTemplate strings are the exact chi
// patterns internal/api/router.go registers, so the router's mounted
// graph routes can be proven equivalent to this manifest (T080-02-MANIFEST).
func CanonicalGraphRouteManifest() []GraphRouteEntry {
	return []GraphRouteEntry{
		{Family: FamilyTopics, Method: http.MethodGet, PathTemplate: "/api/topics/", Scope: GraphReadScope},
		{Family: FamilyTopicDetail, Method: http.MethodGet, PathTemplate: "/api/topics/{id}", Scope: GraphReadScope},
		{Family: FamilyPeople, Method: http.MethodGet, PathTemplate: "/api/people/", Scope: GraphReadScope},
		{Family: FamilyPersonDetail, Method: http.MethodGet, PathTemplate: "/api/people/{id}", Scope: GraphReadScope},
		{Family: FamilyPlaces, Method: http.MethodGet, PathTemplate: "/api/places/", Scope: GraphReadScope},
		{Family: FamilyPlaceDetail, Method: http.MethodGet, PathTemplate: "/api/places/{id}", Scope: GraphReadScope},
		{Family: FamilyTime, Method: http.MethodGet, PathTemplate: "/api/time", Scope: GraphReadScope},
		{Family: FamilyEdges, Method: http.MethodGet, PathTemplate: "/api/graph/edges", Scope: GraphReadScope},
	}
}

// ValidateGraphRouteManifest returns nil iff entries register EXACTLY
// the eight required families — each once, each GET, each requiring
// knowledge-graph:read, each with a non-empty path template. A missing
// family, a duplicated family, an unknown family, a non-GET method, an
// empty path template, or a wrong scope is REJECTED with a typed,
// value-safe F080-ROUTE-MANIFEST-INCOMPLETE error naming only the
// family and the route contract.
//
// This is the "reject construction rather than mount a subset" contract
// (SCN-080-001-02): the router validates the manifest before calling
// Chi, so an incomplete or duplicated manifest fails loudly instead of
// silently mounting fewer than eight routes.
func ValidateGraphRouteManifest(entries []GraphRouteEntry) error {
	required := make(map[GraphRouteFamily]bool, len(requiredGraphFamilies))
	for _, f := range requiredGraphFamilies {
		required[f] = true
	}

	counts := make(map[GraphRouteFamily]int, len(entries))
	for _, e := range entries {
		if !required[e.Family] {
			return fmt.Errorf("[%s] unknown family %q is not part of the canonical graph route manifest", CodeRouteManifestIncomplete, e.Family)
		}
		if e.Method != http.MethodGet {
			return fmt.Errorf("[%s] family %q declares method %q; every graph route MUST be GET", CodeRouteManifestIncomplete, e.Family, e.Method)
		}
		if strings.TrimSpace(e.PathTemplate) == "" {
			return fmt.Errorf("[%s] family %q declares an empty path template", CodeRouteManifestIncomplete, e.Family)
		}
		if e.Scope != GraphReadScope {
			return fmt.Errorf("[%s] family %q declares scope %q; every graph route MUST require %q", CodeRouteManifestIncomplete, e.Family, e.Scope, GraphReadScope)
		}
		counts[e.Family]++
	}

	for _, f := range requiredGraphFamilies {
		switch counts[f] {
		case 1:
			// exactly once — correct
		case 0:
			return fmt.Errorf("[%s] required family %q is absent; the manifest MUST mount all %d families atomically, never a subset", CodeRouteManifestIncomplete, f, len(requiredGraphFamilies))
		default:
			return fmt.Errorf("[%s] family %q is registered %d times; each required family MUST appear exactly once", CodeRouteManifestIncomplete, f, counts[f])
		}
	}
	return nil
}

// MustValidateGraphRouteManifest validates the canonical manifest and
// panics if it is ever internally inconsistent. internal/api/router.go
// calls it before registering the graph group so an incomplete or
// duplicated canonical manifest REJECTS router construction (fail-loud)
// instead of silently mounting a subset of graph routes. The canonical
// manifest is always valid, so this never panics in production; it is a
// fail-loud guardrail against a future edit that would drift the
// manifest into an incoherent subset.
func MustValidateGraphRouteManifest() {
	if err := ValidateGraphRouteManifest(CanonicalGraphRouteManifest()); err != nil {
		panic("graphapi: " + err.Error())
	}
}
