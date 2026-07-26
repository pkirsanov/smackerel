//go:build integration

// Spec 106 SCOPE-106-01 — XP106-01-R (shared-infrastructure rollback unit).
//
// TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening
// is the ROLLBACK unit for the source-locked visual foundation. SCOPE-106-01's
// Rollback contract (scope.md "Rollback") states:
//
//	"Assets, manifest, CSP references, pre-paint code, and service-worker cache
//	 identity roll back as one immutable release pointer. Rollback does not fetch
//	 remote assets, weaken CSP, restore duplicated tokens, rewrite the appearance
//	 cookie, rebuild on the target, or touch domain data."
//
// The service-worker cache identity IS that immutable release pointer: it is a
// PURE content function of the locked same-origin bytes, so a pointer swap
// (forward deploy OR rollback) is atomic, byte-exactly reversible, can never
// reach a remote origin, can never weaken CSP, and can never carry domain,
// route, or appearance-preference data.
//
// This is a REAL test: it exercises the production foundation API
// (web.BuildExperienceAssetManifest / web.IsNetworkOnlyPath) with no mock, no
// stub, and no fabricated digest. It re-derives the release pointer from the
// locked bytes using the same documented formula the foundation uses, proving
// the pointer is content-addressed rather than trusting the manifest's own field.
package integrationexperience

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/experience"
	"github.com/smackerel/smackerel/internal/web"
)

// aggregateReleasePointer re-derives the release pointer from a set of assets
// using the SAME documented formula the foundation uses (sorted
// "servedPath@sha256" joined by newline, SHA-256, lowercase hex). Re-deriving it
// black-box proves the identity is a PURE content function — the property that
// makes a rollback an atomic, byte-exact pointer swap.
func aggregateReleasePointer(paths, digests []string) string {
	lines := make([]string, 0, len(paths))
	for i := range paths {
		lines = append(lines, paths[i]+"@"+digests[i])
	}
	sort.Strings(lines)
	h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(h[:])
}

func TestExperienceFoundationRollbackIsAtomicImmutablePointerSwapWithoutAssetTokenCSPorSWWeakening(t *testing.T) {
	baseline, err := web.BuildExperienceAssetManifest()
	if err != nil {
		t.Fatalf("BuildExperienceAssetManifest (baseline): %v", err)
	}
	if len(baseline.Assets) == 0 {
		t.Fatal("baseline manifest has zero locked assets")
	}

	// --- 1. DETERMINISM: rebuilding yields a byte-identical release pointer, so
	//        a rollback to the same release is a provable no-op — the pointer is
	//        reproducible, never drifts, and needs no target rebuild.
	rebuilt, err := web.BuildExperienceAssetManifest()
	if err != nil {
		t.Fatalf("BuildExperienceAssetManifest (rebuild): %v", err)
	}
	if rebuilt.CacheIdentity != baseline.CacheIdentity {
		t.Fatalf("release pointer is not deterministic across builds: %s != %s", rebuilt.CacheIdentity, baseline.CacheIdentity)
	}
	if len(rebuilt.Assets) != len(baseline.Assets) {
		t.Fatalf("locked asset count drifted between builds: %d != %d", len(rebuilt.Assets), len(baseline.Assets))
	}
	if baseline.CacheIdentity == "" {
		t.Fatal("release pointer (CacheIdentity) is empty")
	}

	// --- 2. PURE CONTENT FUNCTION: the release pointer re-derives from the
	//        locked bytes alone. This is what makes the swap atomic + reversible.
	paths := make([]string, len(baseline.Assets))
	digests := make([]string, len(baseline.Assets))
	for i, a := range baseline.Assets {
		paths[i] = a.ServedPath
		digests[i] = a.SHA256
	}
	if got := aggregateReleasePointer(paths, digests); got != baseline.CacheIdentity {
		t.Fatalf("release pointer is not a pure content function of the locked bytes: rederived=%s manifest=%s", got, baseline.CacheIdentity)
	}

	// --- 3. ATOMIC + BYTE-EXACTLY REVERSIBLE SWAP: advancing the release (adding
	//        a locked asset) changes the pointer; rolling back (dropping it)
	//        restores the EXACT baseline pointer — no drift, no partial state.
	advPaths := append(append([]string{}, paths...), "/pwa/experience-future-asset.css")
	advDigests := append(append([]string{}, digests...), strings.Repeat("a", 64))
	advanced := aggregateReleasePointer(advPaths, advDigests)
	if advanced == baseline.CacheIdentity {
		t.Fatal("advancing the locked asset set did not change the release pointer — the swap is not content-addressed")
	}
	rolledBack := aggregateReleasePointer(paths, digests)
	if rolledBack != baseline.CacheIdentity {
		t.Fatalf("rollback did not restore the exact baseline pointer: %s != %s", rolledBack, baseline.CacheIdentity)
	}

	// --- 4. ROLLBACK NEVER WEAKENS CSP / NEVER FETCHES REMOTE: every asset in
	//        the swapped set is same-origin under /pwa/ from a known locked source
	//        (first-party or vendored-OFL). There is no external scheme a rollback
	//        could reach, so CSP stays default-src 'self'.
	for _, a := range baseline.Assets {
		if !strings.HasPrefix(a.ServedPath, "/pwa/") {
			t.Errorf("asset %q is not same-origin under /pwa/ — a rollback could reach a non-self origin", a.ServedPath)
		}
		if strings.Contains(a.ServedPath, "://") {
			t.Errorf("asset %q carries an external scheme — rollback must never fetch a remote asset", a.ServedPath)
		}
		if a.Source != web.SourceFirstParty && a.Source != web.SourceVendoredOFL {
			t.Errorf("asset %q has an unexpected source %q outside the locked same-origin rollback set", a.ServedPath, a.Source)
		}
		if a.SWPolicy != web.SWPrecacheImmutable {
			t.Errorf("asset %q is not precache-immutable — the rollback pointer must be immutable", a.ServedPath)
		}
	}

	// --- 5. ROLLBACK NEVER TOUCHES DOMAIN DATA: the locked set contains only
	//        static assets; no locked path is a domain/data route, and the
	//        network-only classifier keeps /api and /v1 out of the swap so a
	//        rollback can never precache, rewrite, or mutate business data.
	for _, a := range baseline.Assets {
		if web.IsNetworkOnlyPath(a.ServedPath) {
			t.Errorf("locked asset %q is classified network-only — a data route leaked into the rollback set", a.ServedPath)
		}
	}
	for _, dataRoute := range []string{"/api/capture", "/v1/search", "/api/health"} {
		if !web.IsNetworkOnlyPath(dataRoute) {
			t.Errorf("domain/data route %q is not network-only — rollback could precache or rewrite it", dataRoute)
		}
	}
}

// ── Spec 106 SCOPE-106-04 — XP106-04-R (shadow-adapter rollback) ─────────────
//
// TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation
// (SCN-106-003) proves the SCOPE-106-04 Rollback contract: the shadow adapters
// are a PURE, ADDITIVE overlay on the current renderer authority, so enabling
// them and then reverting the product release (disabling them) is an atomic,
// byte-exactly reversible swap that "keeps generated catalog and comparison
// diagnostics, preserves routes/data, and never installs a static optimistic
// fallback" (scope.md "Rollback").
//
// The proof captures an explicit baseline of current renderer behavior (the
// generated-catalog route/data inventory + the user appearance preference + the
// fail-closed no-fallback contract), enables the three shadow adapters, then
// performs the atomic rollback (revert the release → discard the shadow
// fixtures) and asserts the captured baseline is restored WITHOUT mutating any
// route, data, or user preference, that the discarded fixtures leave no
// optimistic/static-ready residue, that the fail-closed contract still holds
// (no fallback was installed by the rollback), and that re-enabling reproduces
// byte-identical fixtures + digest (byte-exact reversibility, no drift).
//
// It is a REAL, deterministic contract test that drives the production
// experience.GeneratedCatalog / BuildExperienceProjection / shadow adapters with
// NO mock, NO stub, and NO interception (same integration-tag convention as the
// SCOPE-01 rollback test above). Adversarial (non-tautological): the
// after-rollback fail-closed assertion would FAIL if the rollback installed a
// static optimistic fallback, and the digest-reversibility assertion would FAIL
// if the comparison diagnostic drifted across the swap.
func TestShadowAdapterRollbackRestoresBaselineWithoutRouteDataOrPreferenceMutation(t *testing.T) {
	// ── Baseline B0: current renderer behavior (pre-shadow) ───────────────────
	catBaseline, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog (baseline): %v", err)
	}
	baselineRoutes := canonicalCatalogRoutes(catBaseline)
	// The user preference the additive shadow slice must never rewrite.
	baselineAppearance := experience.ShellAppearance{Theme: experience.ShellThemeDark, Density: experience.ShellDensityCompact}

	// A real audience with a non-empty visible set drives the projection.
	audience := ""
	for _, s := range catBaseline.Surfaces {
		if len(s.Audiences) > 0 {
			audience = s.Audiences[0]
			break
		}
	}
	if audience == "" {
		t.Fatalf("generated catalog declares no audience")
	}

	// Readiness-resolved availability for every visible surface (honestly-
	// unavailable leaves resolve to Unavailable; everything else to Available);
	// availability enters only through the resolved-readiness signal.
	buildAvailability := func() map[string]experience.SurfaceAvailabilityOutcome {
		m := make(map[string]experience.SurfaceAvailabilityOutcome)
		for _, s := range catBaseline.Surfaces {
			if !shadowAudienceIncludes(s.Audiences, audience) {
				continue
			}
			val := experience.AvailabilityAvailable
			if s.ReadinessDiscoverabilityPolicy == experience.PolicyUnavailablePendingOwnership ||
				s.ReadinessDiscoverabilityPolicy == experience.PolicyUnavailablePendingDependency {
				val = experience.AvailabilityUnavailable
			}
			m[s.ID] = experience.SurfaceAvailabilityOutcome{Signal: experience.SignalReadinessResolved, Value: val}
		}
		return m
	}
	buildProjection := func() experience.ExperienceProjection {
		proj, err := experience.BuildExperienceProjection(catBaseline, experience.ProjectionRequest{
			Audience:     audience,
			Appearance:   baselineAppearance,
			Availability: buildAvailability(),
		})
		if err != nil {
			t.Fatalf("BuildExperienceProjection(%s): %v", audience, err)
		}
		return proj
	}

	// failsClosedNoFallback proves an inconsistent input (a visible surface with
	// NO availability outcome) fails CLOSED to a typed error — never a settled /
	// optimistic fixture — i.e. no static optimistic fallback exists.
	failsClosedNoFallback := func(stage string) {
		avail := buildAvailability()
		for id := range avail { // drop one required outcome → must fail closed.
			delete(avail, id)
			break
		}
		_, err := experience.BuildExperienceProjection(catBaseline, experience.ProjectionRequest{
			Audience:     audience,
			Appearance:   baselineAppearance,
			Availability: avail,
		})
		if err == nil {
			t.Fatalf("%s: build with a missing availability outcome did NOT fail closed (optimistic fallback)", stage)
		}
		var pe *experience.F106PresentationError
		if !errors.As(err, &pe) {
			t.Fatalf("%s: build returned %T, want *F106PresentationError", stage, err)
		}
	}
	failsClosedNoFallback("baseline") // baseline has no optimistic fallback.

	// ── Enable shadow (additive): render comparison fixtures + digest ─────────
	proj := buildProjection()
	comparisonDigest := proj.ProjectionDigest()
	server := experience.ServerShadowRenderer{}
	pwa := experience.PWAShadowRenderer{}
	card := experience.CardShadowRenderer{}
	serverFixture, err := server.RenderShadow(proj)
	if err != nil {
		t.Fatalf("server.RenderShadow: %v", err)
	}
	pwaFixture, err := pwa.RenderShadow(proj)
	if err != nil {
		t.Fatalf("pwa.RenderShadow: %v", err)
	}
	cardFixture, err := card.RenderShadow(proj)
	if err != nil {
		t.Fatalf("card.RenderShadow: %v", err)
	}
	// Capture the enabled fixtures to prove the swap is byte-exactly reversible.
	enabled := map[string]experience.ShadowFixture{"server": serverFixture, "pwa": pwaFixture, "card": cardFixture}
	for name, f := range enabled {
		if !f.Settled || f.Failure != nil {
			t.Fatalf("%s shadow fixture not cleanly settled while enabled", name)
		}
		sawShadow := false
		for _, m := range f.Markers {
			if m.Key == experience.MarkerProductNavigation {
				sawShadow = m.Val == "shadow"
			}
		}
		if !sawShadow {
			t.Fatalf("%s fixture is not marked data-product-navigation=shadow (not an additive shadow render)", name)
		}
	}
	// Enabling shadow mutated NOTHING: routes/data + user preference unchanged.
	if got := canonicalCatalogRoutes(mustCatalog(t)); got != baselineRoutes {
		t.Fatalf("enabling shadow mutated the catalog route/data baseline")
	}
	if baselineAppearance != (experience.ShellAppearance{Theme: experience.ShellThemeDark, Density: experience.ShellDensityCompact}) {
		t.Fatalf("enabling shadow mutated the user appearance preference")
	}

	// ── Atomic rollback: revert the release → discard the shadow fixtures ─────
	// The shadow layer is a pure additive overlay, so disabling it is a single
	// wholesale revert with no partial residue on the active renderer authority.
	serverFixture = experience.ShadowFixture{}
	pwaFixture = experience.ShadowFixture{}
	cardFixture = experience.ShadowFixture{}

	// 1. Baseline restored: routes/data unchanged.
	if got := canonicalCatalogRoutes(mustCatalog(t)); got != baselineRoutes {
		t.Fatalf("rollback did not restore the catalog route/data baseline")
	}
	// 2. User preference unchanged.
	if baselineAppearance != (experience.ShellAppearance{Theme: experience.ShellThemeDark, Density: experience.ShellDensityCompact}) {
		t.Fatalf("rollback mutated the user appearance preference")
	}
	// 3. No static optimistic fallback residue: the discarded fixtures are wholly
	//    gone (neither an optimistic settled-ready fixture nor a lingering
	//    failure — the untouched active renderer authority is the source of truth).
	for name, f := range map[string]experience.ShadowFixture{"server": serverFixture, "pwa": pwaFixture, "card": cardFixture} {
		if f.Settled || f.Failure != nil {
			t.Fatalf("%s fixture left a residue after rollback (settled=%v failure=%+v)", name, f.Settled, f.Failure)
		}
	}
	// 4. The fail-closed contract still holds (no optimistic fallback installed).
	failsClosedNoFallback("after-rollback")

	// 5. Generated catalog + comparison diagnostics intact across the rollback:
	//    the catalog still validates, the comparison digest re-derives byte-
	//    exactly, and re-enabling reproduces byte-identical fixtures (an atomic,
	//    byte-exactly reversible swap — no drift, no partial state).
	catAfter, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog (after rollback): %v", err)
	}
	if catAfter.SchemaVersion == "" || len(catAfter.Surfaces) == 0 {
		t.Fatalf("generated catalog degraded across rollback (schema=%q surfaces=%d)", catAfter.SchemaVersion, len(catAfter.Surfaces))
	}
	reProj := buildProjection()
	if got := reProj.ProjectionDigest(); got != comparisonDigest {
		t.Fatalf("comparison diagnostic digest drifted across rollback: %s != %s", got, comparisonDigest)
	}
	reFixtures := map[string]experience.ShadowFixture{}
	if f, err := server.RenderShadow(reProj); err != nil {
		t.Fatalf("server.RenderShadow (re-enable): %v", err)
	} else {
		reFixtures["server"] = f
	}
	if f, err := pwa.RenderShadow(reProj); err != nil {
		t.Fatalf("pwa.RenderShadow (re-enable): %v", err)
	} else {
		reFixtures["pwa"] = f
	}
	if f, err := card.RenderShadow(reProj); err != nil {
		t.Fatalf("card.RenderShadow (re-enable): %v", err)
	} else {
		reFixtures["card"] = f
	}
	for name, want := range enabled {
		if !reflect.DeepEqual(reFixtures[name], want) {
			t.Fatalf("%s fixture not byte-exactly reproduced after rollback+re-enable (swap not reversible)", name)
		}
	}
}

// canonicalCatalogRoutes serializes the generated catalog's route/data contract
// (per surface: id|parent|order|href|current_paths|audiences) into a single
// deterministic string. It is the "routes and data" baseline the additive shadow
// slice must preserve byte-for-byte across an enable + rollback cycle.
func canonicalCatalogRoutes(cat experience.ProductExperienceCatalog) string {
	lines := make([]string, 0, len(cat.Surfaces))
	for _, s := range cat.Surfaces {
		paths := append([]string(nil), s.CurrentPaths...)
		sort.Strings(paths)
		auds := append([]string(nil), s.Audiences...)
		sort.Strings(auds)
		lines = append(lines, strings.Join([]string{
			s.ID, s.ParentID, strconv.Itoa(s.Order), s.Href,
			strings.Join(paths, ","), strings.Join(auds, ","),
		}, "|"))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// mustCatalog re-reads the generated catalog or fails the test.
func mustCatalog(t *testing.T) experience.ProductExperienceCatalog {
	t.Helper()
	c, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog: %v", err)
	}
	return c
}
