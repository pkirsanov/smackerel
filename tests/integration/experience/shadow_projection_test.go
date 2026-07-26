//go:build integration

// Spec 106 SCOPE-106-04 — XP106-04-I (integration, live stack).
//
// TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover
// (SCN-106-003) builds the renderer-neutral ExperienceProjection from the REAL
// generated catalog (internal/experience.GeneratedCatalog — the same embedded,
// `./smackerel.sh config generate`-produced catalog the running stack serves),
// a REAL principal/session audience (the audiences the real catalog declares),
// and REAL owner-availability outcomes derived from the ACTUAL readiness owner
// (internal/recommendation/availability.Determine), then renders that ONE
// projection through the three SHADOW adapters (server / PWA / Card) and proves:
//
//   - the three adapters produce identical surface IDs, parents, order, labels,
//     hrefs (via the digest + catalog faithfulness), audience, availability, and
//     action, AND an identical projection digest, for the equivalent
//     principal/release input;
//   - SHADOW mode: the render mutates NO active navigation, route authorization,
//     or page body — every fixture is a content-free comparison + telemetry
//     artifact (data-product-navigation=shadow, closed content-free markers only,
//     safe DOM ops only, no href/active-link attribute, no user content);
//   - the projected availability tracks the REAL readiness determination — a real
//     not-ready owner outcome never projects Available (no fabricated
//     availability) and a real ready outcome never projects Unavailable (no false
//     outage);
//   - it fails CLOSED: an inconsistent adapter input surfaces a visible
//     ShadowFailure with NO optimistic/static-ready fixture, and availability
//     sourced from a structural fact (a registered route) instead of a resolved
//     readiness fact is rejected at build time.
//
// This is a REAL integration test: it exercises the actual production catalog +
// the actual readiness owner (availability.Determine is a pure production
// function; its own live-DB derivation is the owner's deferred slice, not this
// scope's) + the actual shadow adapters, with NO httptest interception of
// internal handlers, NO route()/intercept(), and NO mock of internal code. It is
// integration-tagged and runs in the `./smackerel.sh test integration` live lane;
// it needs no DATABASE_URL, so it PASSES (never SKIPs) whenever the lane runs.
//
// Adversarial (non-tautological): the readiness/structural fail-closed cases
// prove a registered route CANNOT fabricate availability — otherwise a projected
// "unavailable" could not tell a resolved-readiness fact from a structural one.
package integrationexperience

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/experience"
	"github.com/smackerel/smackerel/internal/recommendation"
	"github.com/smackerel/smackerel/internal/recommendation/availability"
)

// shadowAudienceIncludes reports whether audience a is in the surface's audience
// set (exported-only re-statement of the package-internal predicate, since this
// external test package cannot call the unexported helper).
func shadowAudienceIncludes(auds []string, a string) bool {
	for _, x := range auds {
		if x == a {
			return true
		}
	}
	return false
}

// shadowIsUnavailablePolicy reports whether the policy denotes an honestly-
// unavailable leaf (built from exported constants only).
func shadowIsUnavailablePolicy(p experience.DiscoverabilityPolicy) bool {
	return p == experience.PolicyUnavailablePendingOwnership ||
		p == experience.PolicyUnavailablePendingDependency
}

// shadowIsActivePolicy reports whether the policy denotes an active leaf bound to
// an exact registered route.
func shadowIsActivePolicy(p experience.DiscoverabilityPolicy) bool {
	return p == experience.PolicyReadyWhenJourneyReady ||
		p == experience.PolicyOperatorOnlyWhenReady
}

func TestShadowProjectionUsesRealSessionAudienceCatalogAndOwnerStatesWithoutCutover(t *testing.T) {
	// ── REAL generated catalog (not a hand-built fixture) ─────────────────────
	cat, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("experience.GeneratedCatalog() error: %v", err)
	}
	if len(cat.Surfaces) == 0 {
		t.Fatalf("generated catalog has zero surfaces")
	}

	// ── REAL readiness owner (recommendation/availability.Determine) ──────────
	// The availability VALUE enters the projection only through a resolved
	// readiness fact and is the owner's real determination, never hand-stamped.
	prodProvider := func(id string, health availability.HealthStatus) availability.ProviderState {
		return availability.ProviderState{
			ID: id, DisplayName: id,
			Class:            availability.ProviderClassProduction,
			OperatorSelected: true, Enabled: true, Configured: true, Registered: true,
			Categories: []recommendation.Category{recommendation.CategoryPlace},
			Health:     health,
		}
	}
	determine := func(enabled bool, providers ...availability.ProviderState) availability.AvailabilitySnapshot {
		return availability.Determine(availability.Input{
			Enabled:     enabled,
			Category:    recommendation.CategoryPlace,
			Operation:   availability.OperationRequest,
			Providers:   providers,
			EvaluatedAt: time.Now().UTC(),
			ValidFor:    time.Minute,
		})
	}
	availFromSnapshot := func(s availability.AvailabilitySnapshot) experience.Availability {
		switch s.State {
		case availability.CapabilityAvailable:
			return experience.AvailabilityAvailable
		case availability.CapabilityDegraded:
			return experience.AvailabilityDegraded
		default: // CapabilityDisabled, CapabilityUnavailable
			return experience.AvailabilityUnavailable
		}
	}
	// snapshotFor runs the REAL readiness determination for a surface: an
	// honestly-unavailable leaf resolves to a real not-ready snapshot (enabled but
	// zero providers); everything else to a real ready snapshot (one healthy
	// production provider). The owner does the readiness logic; we only present it.
	snapshotFor := func(s experience.Surface) availability.AvailabilitySnapshot {
		if shadowIsUnavailablePolicy(s.ReadinessDiscoverabilityPolicy) {
			return determine(true)
		}
		return determine(true, prodProvider("place-primary", availability.HealthHealthy))
	}
	// buildOwnerAvailability returns the readiness-resolved availability contract
	// for every surface visible to the audience, plus the real snapshots behind
	// each value (so the test can assert the projection tracks owner truth).
	buildOwnerAvailability := func(audience string) (map[string]experience.SurfaceAvailabilityOutcome, map[string]availability.AvailabilitySnapshot) {
		outcomes := make(map[string]experience.SurfaceAvailabilityOutcome)
		snaps := make(map[string]availability.AvailabilitySnapshot)
		for _, s := range cat.Surfaces {
			if !shadowAudienceIncludes(s.Audiences, audience) {
				continue
			}
			snap := snapshotFor(s)
			snaps[s.ID] = snap
			outcomes[s.ID] = experience.SurfaceAvailabilityOutcome{
				Signal: experience.SignalReadinessResolved,
				Value:  availFromSnapshot(snap),
			}
		}
		return outcomes, snaps
	}

	// ── REAL audiences from the real catalog (the principal/session axis) ─────
	audienceSet := map[string]bool{}
	for _, s := range cat.Surfaces {
		for _, a := range s.Audiences {
			audienceSet[a] = true
		}
	}
	var audiences []string
	for a := range audienceSet {
		audiences = append(audiences, a)
	}
	sort.Strings(audiences)
	if len(audiences) == 0 {
		t.Fatalf("generated catalog declares zero audiences")
	}

	appearance := experience.ShellAppearance{Theme: experience.ShellThemeDark, Density: experience.ShellDensityCompact}
	server := experience.ServerShadowRenderer{}
	pwa := experience.PWAShadowRenderer{}
	card := experience.CardShadowRenderer{}

	normalize := func(f experience.ShadowFixture) experience.ShadowFixture { f.Renderer = ""; return f }
	denyContent := []string{"evidence", "session", "token", "bearer", "password", "secret"}

	digestByAudience := map[string]string{}
	visibleCountByAudience := map[string]int{}

	// ── Per real audience: three shadow adapters agree exactly, in shadow mode ─
	for _, audience := range audiences {
		audience := audience
		t.Run("audience_"+audience, func(t *testing.T) {
			outcomes, snaps := buildOwnerAvailability(audience)
			proj, err := experience.BuildExperienceProjection(cat, experience.ProjectionRequest{
				Audience:     audience,
				Appearance:   appearance,
				Availability: outcomes,
			})
			if err != nil {
				t.Fatalf("BuildExperienceProjection(%s) error: %v", audience, err)
			}
			if len(proj.Surfaces) == 0 {
				t.Fatalf("audience %s: projection has zero surfaces", audience)
			}
			digestByAudience[audience] = proj.ProjectionDigest()
			visibleCountByAudience[audience] = len(proj.Surfaces)

			sf, err := server.RenderShadow(proj)
			if err != nil {
				t.Fatalf("server.RenderShadow error: %v", err)
			}
			pf, err := pwa.RenderShadow(proj)
			if err != nil {
				t.Fatalf("pwa.RenderShadow error: %v", err)
			}
			cf, err := card.RenderShadow(proj)
			if err != nil {
				t.Fatalf("card.RenderShadow error: %v", err)
			}
			fixtures := map[string]experience.ShadowFixture{"server": sf, "pwa": pf, "card": cf}

			// (a) Three adapters over ONE projection: byte-identical fixtures +
			//     identical projection digest (the digest encodes the hrefs, so
			//     href parity is proven even though hrefs never enter a fixture).
			want := proj.ProjectionDigest()
			for name, f := range fixtures {
				if !f.Settled || f.Failure != nil {
					t.Fatalf("%s fixture not cleanly settled: settled=%v failure=%+v", name, f.Settled, f.Failure)
				}
				if f.ProjectionDigest != want {
					t.Fatalf("%s fixture digest %q != projection digest %q", name, f.ProjectionDigest, want)
				}
			}
			if !reflect.DeepEqual(normalize(sf), normalize(pf)) {
				t.Fatalf("audience %s: server and pwa shadow fixtures diverge", audience)
			}
			if !reflect.DeepEqual(normalize(sf), normalize(cf)) {
				t.Fatalf("audience %s: server and card shadow fixtures diverge", audience)
			}
			if sf.Renderer != experience.RendererServer || pf.Renderer != experience.RendererPWA || cf.Renderer != experience.RendererCard {
				t.Fatalf("audience %s: renderer identities not distinct: %q %q %q", audience, sf.Renderer, pf.Renderer, cf.Renderer)
			}

			// (b) The shared projection faithfully carries the REAL catalog's IDs,
			//     parents, order, labels, and exact hrefs.
			catByID := map[string]experience.Surface{}
			for _, s := range cat.Surfaces {
				catByID[s.ID] = s
			}
			labelSet := map[string]bool{}
			for _, ps := range proj.Surfaces {
				cs, ok := catByID[ps.SurfaceID]
				if !ok {
					t.Fatalf("projected surface %q not in generated catalog", ps.SurfaceID)
				}
				if ps.ParentSurfaceID != cs.ParentID || ps.Order != cs.Order || ps.Label != cs.Label || ps.Href != cs.Href {
					t.Fatalf("projected surface %q diverges from catalog (parent %q/%q order %d/%d label %q/%q href %q/%q)",
						ps.SurfaceID, ps.ParentSurfaceID, cs.ParentID, ps.Order, cs.Order, ps.Label, cs.Label, ps.Href, cs.Href)
				}
				if ps.Audience != audience {
					t.Fatalf("projected surface %q audience %q != %q", ps.SurfaceID, ps.Audience, audience)
				}
				labelSet[ps.Label] = true
			}

			// (c) SHADOW MODE: content-free comparison telemetry only — no active
			//     navigation, route, or page-body mutation is emitted.
			for name, f := range fixtures {
				if !f.UsesOnlySafeDOMOps() {
					t.Fatalf("%s fixture used an unsafe DOM op (active-link/innerHTML mutation)", name)
				}
				allowed := experience.ShadowMarkerKeys()
				sawShadowMarker := false
				for _, m := range f.Markers {
					if !allowed[m.Key] {
						t.Fatalf("%s fixture emitted out-of-contract marker key %q (user content leak)", name, m.Key)
					}
					if m.Key == "data-href" {
						t.Fatalf("%s fixture leaked an href marker (active-link data)", name)
					}
					if m.Key == experience.MarkerProductNavigation {
						sawShadowMarker = true
						if m.Val != "shadow" {
							t.Fatalf("%s fixture data-product-navigation=%q, want shadow (no active-nav cutover)", name, m.Val)
						}
					}
					low := strings.ToLower(m.Val)
					for _, bad := range denyContent {
						if strings.Contains(low, bad) {
							t.Fatalf("%s fixture marker %q leaked non-content-free value %q", name, m.Key, m.Val)
						}
					}
				}
				if !sawShadowMarker {
					t.Fatalf("%s fixture missing data-product-navigation=shadow marker", name)
				}
				for _, op := range f.DOMOps {
					if op.Kind == experience.DOMSetDataAttr && !strings.HasPrefix(op.AttrKey, "data-") {
						t.Fatalf("%s fixture set a non-data-* attribute %q (active-link mutation)", name, op.AttrKey)
					}
					if op.Kind == experience.DOMSetTextContent && op.Text != "" && !labelSet[op.Text] {
						t.Fatalf("%s fixture set a text node %q that is not a catalog wayfinding label (user content leak)", name, op.Text)
					}
				}
			}

			// (d) Real-owner truth: every projected availability tracks the REAL
			//     readiness determination.
			for _, ps := range proj.Surfaces {
				snap, ok := snaps[ps.SurfaceID]
				if !ok {
					t.Fatalf("no readiness snapshot recorded for visible surface %q", ps.SurfaceID)
				}
				if want := availFromSnapshot(snap); ps.Availability != want {
					t.Fatalf("surface %q projected availability %q != owner-derived %q", ps.SurfaceID, ps.Availability, want)
				}
				if snap.Ready() && ps.Availability == experience.AvailabilityUnavailable {
					t.Fatalf("surface %q: real Ready() owner outcome projected Unavailable (false outage)", ps.SurfaceID)
				}
				if !snap.Ready() && ps.Availability == experience.AvailabilityAvailable {
					t.Fatalf("surface %q: real not-ready owner outcome projected Available (fabricated availability)", ps.SurfaceID)
				}
			}
		})
	}

	// Distinct real audiences with distinct visible sets yield distinct
	// projections — the principal input really drives the projection.
	for i := 0; i < len(audiences); i++ {
		for j := i + 1; j < len(audiences); j++ {
			a, b := audiences[i], audiences[j]
			if digestByAudience[a] == "" || digestByAudience[b] == "" {
				continue
			}
			if visibleCountByAudience[a] != visibleCountByAudience[b] && digestByAudience[a] == digestByAudience[b] {
				t.Fatalf("audiences %s (%d surfaces) and %s (%d surfaces) share a projection digest; the principal input did not change the projection",
					a, visibleCountByAudience[a], b, visibleCountByAudience[b])
			}
		}
	}

	// ── Rich audience: current highlight + fail-closed (no fallback) ──────────
	rich := audiences[0]
	for _, a := range audiences {
		if visibleCountByAudience[a] > visibleCountByAudience[rich] {
			rich = a
		}
	}

	t.Run("current_highlight_and_fail_closed_"+rich, func(t *testing.T) {
		outcomes, _ := buildOwnerAvailability(rich)

		// A real active leaf visible to the rich audience becomes the current
		// surface so current / parent-current highlight is exercised.
		var current, currentParent string
		for _, s := range cat.Surfaces {
			if !shadowAudienceIncludes(s.Audiences, rich) {
				continue
			}
			if shadowIsActivePolicy(s.ReadinessDiscoverabilityPolicy) {
				current, currentParent = s.ID, s.ParentID
				break
			}
		}
		if current == "" {
			t.Fatalf("rich audience %s has no active leaf to mark current", rich)
		}

		proj, err := experience.BuildExperienceProjection(cat, experience.ProjectionRequest{
			Audience:         rich,
			CurrentSurfaceID: current,
			Appearance:       appearance,
			Availability:     outcomes,
		})
		if err != nil {
			t.Fatalf("BuildExperienceProjection(%s, current=%s) error: %v", rich, current, err)
		}

		findPS := func(id string) (experience.ProjectedSurface, bool) {
			for _, ps := range proj.Surfaces {
				if ps.SurfaceID == id {
					return ps, true
				}
			}
			return experience.ProjectedSurface{}, false
		}
		cur, ok := findPS(current)
		if !ok {
			t.Fatalf("projection missing current surface %q", current)
		}
		if !cur.Current || cur.ParentCurrent {
			t.Fatalf("current surface %q current=%v parentCurrent=%v, want current=true parentCurrent=false", current, cur.Current, cur.ParentCurrent)
		}
		if currentParent != "" {
			if p, ok := findPS(currentParent); ok && (p.Current || !p.ParentCurrent) {
				t.Fatalf("parent %q current=%v parentCurrent=%v, want current=false parentCurrent=true", currentParent, p.Current, p.ParentCurrent)
			}
		}

		// Three adapters still agree exactly with a current surface set.
		sf, err := server.RenderShadow(proj)
		if err != nil {
			t.Fatalf("server.RenderShadow(current) error: %v", err)
		}
		pf, err := pwa.RenderShadow(proj)
		if err != nil {
			t.Fatalf("pwa.RenderShadow(current) error: %v", err)
		}
		cf, err := card.RenderShadow(proj)
		if err != nil {
			t.Fatalf("card.RenderShadow(current) error: %v", err)
		}
		if !reflect.DeepEqual(normalize(sf), normalize(pf)) || !reflect.DeepEqual(normalize(sf), normalize(cf)) {
			t.Fatalf("shadow fixtures diverge with a current surface set")
		}

		// Fail-closed A: tamper the shared projection so a navigate leaf loses its
		// route (and, if present, a route-free group gains one). Every adapter
		// must surface the failure with no optimistic/settled fixture.
		tamper := func(mutate func([]experience.ProjectedSurface) bool) (experience.ExperienceProjection, bool) {
			bad := proj
			bad.Surfaces = append([]experience.ProjectedSurface(nil), proj.Surfaces...)
			ok := mutate(bad.Surfaces)
			return bad, ok
		}
		var badProjections []experience.ExperienceProjection
		if navBad, ok := tamper(func(s []experience.ProjectedSurface) bool {
			for i := range s {
				if s[i].Action == experience.ActionNavigate && s[i].Href != "" {
					s[i].Href = ""
					return true
				}
			}
			return false
		}); ok {
			badProjections = append(badProjections, navBad)
		}
		if grpBad, ok := tamper(func(s []experience.ProjectedSurface) bool {
			for i := range s {
				if s[i].Action == experience.ActionOpenGroup {
					s[i].Href = "/tampered"
					return true
				}
			}
			return false
		}); ok {
			badProjections = append(badProjections, grpBad)
		}
		if len(badProjections) == 0 {
			t.Fatalf("rich audience %s exposed no navigate/group surface to tamper for the fail-closed proof", rich)
		}
		adapters := map[string]experience.ShadowRenderer{"server": server, "pwa": pwa, "card": card}
		for _, bad := range badProjections {
			for name, a := range adapters {
				f, err := a.RenderShadow(bad)
				if err == nil {
					t.Fatalf("%s adapter accepted an inconsistent projection with no error (silent fallback)", name)
				}
				var pe *experience.F106PresentationError
				if !errors.As(err, &pe) {
					t.Fatalf("%s adapter returned %T, want *F106PresentationError", name, err)
				}
				if f.Failure == nil {
					t.Fatalf("%s adapter hid the failure (no ShadowFailure marker)", name)
				}
				if f.Settled {
					t.Fatalf("%s adapter produced a SETTLED fixture on bad input (optimistic fallback)", name)
				}
			}
		}

		// Fail-closed B (adversarial, non-tautological): availability sourced from
		// a STRUCTURAL fact (a registered route) instead of a resolved readiness
		// fact cannot fabricate availability — the build fails closed.
		nonReadiness, _ := buildOwnerAvailability(rich)
		var victim string
		for id := range nonReadiness {
			victim = id
			break
		}
		nonReadiness[victim] = experience.SurfaceAvailabilityOutcome{Signal: experience.SignalRouteRegistered, Value: experience.AvailabilityAvailable}
		if _, err := experience.BuildExperienceProjection(cat, experience.ProjectionRequest{
			Audience:     rich,
			Appearance:   appearance,
			Availability: nonReadiness,
		}); err == nil {
			t.Fatalf("build accepted a route-registered (non-readiness) availability signal (fabricated availability)")
		} else {
			var pe *experience.F106PresentationError
			if !errors.As(err, &pe) {
				t.Fatalf("build returned %T, want *F106PresentationError", err)
			}
		}
	})
}
