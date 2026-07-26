package experience

import (
	"strings"
	"testing"
)

// TestExperienceConsumerInventoryContainsNoStaleNavigationRedirectManifestServiceWorkerOrTestTarget
// is XP106-02-C: it inventories the REAL server nav, PWA nav, web-manifest
// shortcuts/share-target, service-worker precache list, and the newly-generated
// catalog's bound routes, then proves the blocking stale-reference scan finds
// ZERO destinations that fail to resolve to a real registered route or real PWA
// static asset (no invented or dead endpoints). It includes an adversarial guard
// proving the scan is non-tautological.
func TestExperienceConsumerInventoryContainsNoStaleNavigationRedirectManifestServiceWorkerOrTestTarget(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot: %v", err)
	}
	catalog, err := GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog: %v", err)
	}

	// The inventory must actually find real consumers — otherwise a "no stale"
	// result would be vacuously true.
	refs, err := InventoryAll(root, catalog)
	if err != nil {
		t.Fatalf("InventoryAll: %v", err)
	}
	if len(refs) < 20 {
		t.Fatalf("consumer inventory suspiciously small (%d refs) — scan likely failed to read a real consumer surface", len(refs))
	}
	present := map[string]bool{}
	for _, r := range refs {
		present[r.Consumer] = true
	}
	for _, want := range []string{"server-nav", "pwa-nav", "manifest-shortcut", "service-worker", "catalog-href"} {
		if !present[want] {
			t.Fatalf("consumer inventory missing expected consumer surface %q (found: %v)", want, present)
		}
	}

	// Blocking stale-reference scan: every navigation, manifest, service-worker,
	// and catalog destination MUST resolve to a real registered route or PWA
	// static asset.
	stale, err := StaleReferences(root, catalog)
	if err != nil {
		t.Fatalf("StaleReferences: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("stale consumer references found (%d): %s", len(stale), strings.Join(stale, ", "))
	}

	// Adversarial guard (non-tautological): a fabricated catalog binding a route
	// that is neither a registered server route nor a real PWA file MUST be
	// reported stale. If it is not, the scan cannot detect an invented endpoint.
	fakePath := "/definitely-not-a-registered-route-106-02"
	fake := ProductExperienceCatalog{
		SchemaVersion: "smackerel-product-experience/v1",
		Surfaces: []Surface{{
			ID: "fake", Label: "Fake", Kind: KindLinkedLeaf, Order: 1,
			CapabilityID: "cap.fake", Audiences: []string{"daily_user"},
			Href: fakePath, CurrentPaths: []string{fakePath},
			RendererSupport:                []Renderer{RendererServer},
			ReadinessDiscoverabilityPolicy: PolicyReadyWhenJourneyReady,
		}},
	}
	fakeStale, err := StaleReferences(root, fake)
	if err != nil {
		t.Fatalf("StaleReferences(fake): %v", err)
	}
	foundFake := false
	for _, s := range fakeStale {
		if strings.Contains(s, fakePath) {
			foundFake = true
			break
		}
	}
	if !foundFake {
		t.Fatalf("adversarial: fabricated route %q was NOT flagged stale — the scan is tautological (got: %v)", fakePath, fakeStale)
	}
}
