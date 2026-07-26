package experience

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection is the
// SCOPE-106-04 slice-1 fast unit lane (XP106-04-U, SCN-106-003). It proves that
// the server, PWA, and Card SHADOW adapters consume ONE renderer-neutral
// ExperienceProjection — built from the generated catalog, the shell
// appearance, and the readiness-owned availability state contract — and render
// IDENTICAL content-free comparison fixtures + an identical projection digest,
// while:
//
//   - carrying surface IDs, labels, order, hrefs, parents, audience,
//     availability, and action that agree exactly across renderers (golden
//     parity by one shared projection);
//   - emitting only the closed content-free data-* contract markers + a terminal
//     settled marker, with NO user content or evidence IDs;
//   - constructing PWA nodes from a SAFE DOM-op vocabulary only (no innerHTML,
//     no bearer injection, no arbitrary-attribute op);
//   - failing CLOSED with a visible ShadowFailure and NO optimistic/static-ready
//     fixture when an adapter is handed an inconsistent projection, and when the
//     builder is handed availability that is not sourced from a resolved
//     readiness fact.
//
// It is a REAL test: it fails if any adapter diverges, if the safe-DOM contract
// is broken, or if a fallback is silently produced instead of a visible failure.
func TestServerPWAAndCardShadowAdaptersProduceIdenticalGoldenProjection(t *testing.T) {
	cat, err := GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog() error: %v", err)
	}

	// availabilityFor supplies a readiness-resolved availability outcome for
	// EVERY surface visible to the audience: honestly-unavailable leaves resolve
	// to unavailable, everything else (active leaves + route-free groups) to
	// available. Every outcome carries SignalReadinessResolved, so availability
	// only ever enters the projection through the readiness contract.
	availabilityFor := func(c ProductExperienceCatalog, audience string) map[string]SurfaceAvailabilityOutcome {
		m := make(map[string]SurfaceAvailabilityOutcome)
		for _, s := range c.Surfaces {
			if !audienceIncludes(s.Audiences, audience) {
				continue
			}
			val := AvailabilityAvailable
			if s.ReadinessDiscoverabilityPolicy.unavailable() {
				val = AvailabilityUnavailable
			}
			m[s.ID] = SurfaceAvailabilityOutcome{Signal: SignalReadinessResolved, Value: val}
		}
		return m
	}

	req := ProjectionRequest{
		Audience:         "daily_user",
		CurrentSurfaceID: "knowledge_wiki",
		Appearance:       ShellAppearance{Theme: ShellThemeDark, Density: ShellDensityCompact},
		Availability:     availabilityFor(cat, "daily_user"),
	}
	proj, err := BuildExperienceProjection(cat, req)
	if err != nil {
		t.Fatalf("BuildExperienceProjection(daily_user) error: %v", err)
	}
	if len(proj.Surfaces) == 0 {
		t.Fatalf("projection has zero surfaces")
	}

	server := ServerShadowRenderer{}
	pwa := PWAShadowRenderer{}
	card := CardShadowRenderer{}

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
	fixtures := map[string]ShadowFixture{"server": sf, "pwa": pf, "card": cf}

	// ── Golden parity: one projection, three identical fixtures ───────────────
	t.Run("three_shadow_adapters_produce_identical_golden_fixtures", func(t *testing.T) {
		// Each adapter renders under its own renderer identity.
		if server.RendererID() != RendererServer || pwa.RendererID() != RendererPWA || card.RendererID() != RendererCard {
			t.Fatalf("renderer identities not distinct: %q %q %q",
				server.RendererID(), pwa.RendererID(), card.RendererID())
		}
		// Every fixture settled cleanly with no failure marker.
		for name, f := range fixtures {
			if !f.Settled {
				t.Fatalf("%s fixture not settled", name)
			}
			if f.Failure != nil {
				t.Fatalf("%s fixture carries a failure on a valid projection: %+v", name, f.Failure)
			}
		}
		// The digest is a property of the shared projection: identical across
		// renderers and equal to the projection's own digest.
		want := proj.ProjectionDigest()
		for name, f := range fixtures {
			if f.ProjectionDigest != want {
				t.Fatalf("%s fixture digest %q != projection digest %q", name, f.ProjectionDigest, want)
			}
		}
		// Normalising away the renderer identity, the three fixtures are byte-
		// identical: markers, safe DOM ops, digest, and settled state all agree.
		norm := func(f ShadowFixture) ShadowFixture { f.Renderer = ""; return f }
		if !reflect.DeepEqual(norm(sf), norm(pf)) {
			t.Fatalf("server and pwa shadow fixtures diverge")
		}
		if !reflect.DeepEqual(norm(sf), norm(cf)) {
			t.Fatalf("server and card shadow fixtures diverge")
		}
	})

	// ── Content-free contract markers + settled hook ──────────────────────────
	t.Run("fixtures_emit_only_content_free_contract_markers", func(t *testing.T) {
		allowed := ShadowMarkerKeys()
		denyContent := []string{"evidence", "session", "token", "bearer", "password"}
		for name, f := range fixtures {
			markerVals := map[string][]string{}
			for _, m := range f.Markers {
				if !allowed[m.Key] {
					t.Fatalf("%s fixture emits out-of-contract marker key %q (possible user content leak)", name, m.Key)
				}
				markerVals[m.Key] = append(markerVals[m.Key], m.Val)
				lower := strings.ToLower(m.Val)
				for _, bad := range denyContent {
					if strings.Contains(lower, bad) {
						t.Fatalf("%s fixture marker %q leaks non-content-free value %q", name, m.Key, m.Val)
					}
				}
			}
			// Required shell markers.
			if got := markerVals[MarkerExperienceVersion]; len(got) != 1 || got[0] != cat.SchemaVersion {
				t.Fatalf("%s fixture missing/incorrect %s: %v", name, MarkerExperienceVersion, got)
			}
			if got := markerVals[MarkerProductNavigation]; len(got) != 1 || got[0] != "shadow" {
				t.Fatalf("%s fixture missing/incorrect %s: %v", name, MarkerProductNavigation, got)
			}
			if got := markerVals[MarkerProjectionDigest]; len(got) != 1 || got[0] != proj.ProjectionDigest() {
				t.Fatalf("%s fixture missing/incorrect %s: %v", name, MarkerProjectionDigest, got)
			}
			if got := markerVals[MarkerSettled]; len(got) != 1 || got[0] != "true" {
				t.Fatalf("%s fixture missing terminal settled marker: %v", name, got)
			}
			// One surface-id + one parent-surface-id marker per projected surface.
			if got := len(markerVals[MarkerSurfaceID]); got != len(proj.Surfaces) {
				t.Fatalf("%s fixture %s count %d != projected surfaces %d", name, MarkerSurfaceID, got, len(proj.Surfaces))
			}
			if got := len(markerVals[MarkerParentSurfaceID]); got != len(proj.Surfaces) {
				t.Fatalf("%s fixture %s count %d != projected surfaces %d", name, MarkerParentSurfaceID, got, len(proj.Surfaces))
			}
			// No href ever enters the fixture markers (hrefs live in the digest).
			if _, leaked := markerVals["data-href"]; leaked {
				t.Fatalf("%s fixture leaked a data-href marker", name)
			}
		}
	})

	// ── Safe DOM construction (no innerHTML / no bearer / data-* attrs only) ───
	t.Run("pwa_and_peers_construct_nodes_safely", func(t *testing.T) {
		safeKinds := map[DOMOpKind]bool{
			DOMCreateElement: true, DOMSetDataAttr: true, DOMSetTextContent: true,
			DOMAppendChild: true, DOMMarkSettled: true,
		}
		for name, f := range fixtures {
			if !f.UsesOnlySafeDOMOps() {
				t.Fatalf("%s fixture uses an unsafe DOM op", name)
			}
			if len(f.DOMOps) == 0 {
				t.Fatalf("%s fixture emitted no DOM ops", name)
			}
			var sawTextNode, sawSettled bool
			for _, op := range f.DOMOps {
				if !safeKinds[op.Kind] {
					t.Fatalf("%s fixture op kind %q outside the safe vocabulary", name, op.Kind)
				}
				if op.Kind == DOMSetDataAttr && !strings.HasPrefix(op.AttrKey, "data-") {
					t.Fatalf("%s fixture set a non-data-* attribute %q", name, op.AttrKey)
				}
				if op.Kind == DOMSetTextContent {
					sawTextNode = true
				}
			}
			// The wayfinding label reaches the DOM only as a safe text node.
			if !sawTextNode {
				t.Fatalf("%s fixture never assigned a label via a safe text node", name)
			}
			// The terminal op marks the shell settled.
			if last := f.DOMOps[len(f.DOMOps)-1]; last.Kind != DOMMarkSettled {
				t.Fatalf("%s fixture terminal op is %q, want mark_settled", name, last.Kind)
			}
			_ = sawSettled
		}
	})

	// ── Field-level parity: current/parent-current, audience, availability ────
	t.Run("projection_fields_match_catalog_and_owner_truth", func(t *testing.T) {
		find := func(id string) (ProjectedSurface, bool) {
			for _, s := range proj.Surfaces {
				if s.SurfaceID == id {
					return s, true
				}
			}
			return ProjectedSurface{}, false
		}
		// Current leaf and its parent group's parent-current highlight.
		wiki, ok := find("knowledge_wiki")
		if !ok {
			t.Fatalf("projection missing knowledge_wiki")
		}
		if !wiki.Current || wiki.ParentCurrent {
			t.Fatalf("knowledge_wiki current=%v parentCurrent=%v, want current=true parentCurrent=false", wiki.Current, wiki.ParentCurrent)
		}
		if wiki.ParentSurfaceID != "knowledge" {
			t.Fatalf("knowledge_wiki parent=%q, want knowledge", wiki.ParentSurfaceID)
		}
		knowledge, ok := find("knowledge")
		if !ok {
			t.Fatalf("projection missing knowledge")
		}
		if knowledge.Current || !knowledge.ParentCurrent {
			t.Fatalf("knowledge current=%v parentCurrent=%v, want current=false parentCurrent=true", knowledge.Current, knowledge.ParentCurrent)
		}
		// Active leaf: navigate action, real route, available.
		search, ok := find("search")
		if !ok {
			t.Fatalf("projection missing search")
		}
		if search.Action != ActionNavigate || search.Href != "/" || search.Availability != AvailabilityAvailable {
			t.Fatalf("search action=%q href=%q avail=%q, want navigate // available", search.Action, search.Href, search.Availability)
		}
		// Route-free group: open_group action, no route.
		work, ok := find("work")
		if !ok {
			t.Fatalf("projection missing work")
		}
		if work.Action != ActionOpenGroup || work.Href != "" {
			t.Fatalf("work action=%q href=%q, want open_group and empty href", work.Action, work.Href)
		}
		// Honestly-unavailable leaf: unavailable action + availability, no route.
		lists, ok := find("work_lists")
		if !ok {
			t.Fatalf("projection missing work_lists")
		}
		if lists.Action != ActionUnavailable || lists.Href != "" || lists.Availability != AvailabilityUnavailable {
			t.Fatalf("work_lists action=%q href=%q avail=%q, want unavailable/empty/unavailable", lists.Action, lists.Href, lists.Availability)
		}
		// Audience filtering: operator-only surfaces are absent for a daily_user.
		if _, present := find("admin"); present {
			t.Fatalf("daily_user projection leaked the operator-only admin group")
		}
		if _, present := find("admin_models"); present {
			t.Fatalf("daily_user projection leaked the operator-only admin_models leaf")
		}
		for _, s := range proj.Surfaces {
			if s.Audience != "daily_user" {
				t.Fatalf("surface %s audience=%q, want daily_user", s.SurfaceID, s.Audience)
			}
		}
	})

	// ── Audience changes the projection and its digest ────────────────────────
	t.Run("operator_audience_changes_projection_and_digest", func(t *testing.T) {
		opReq := ProjectionRequest{
			Audience:     "operator",
			Appearance:   ShellAppearance{Theme: ShellThemeDark, Density: ShellDensityCompact},
			Availability: availabilityFor(cat, "operator"),
		}
		opProj, err := BuildExperienceProjection(cat, opReq)
		if err != nil {
			t.Fatalf("BuildExperienceProjection(operator) error: %v", err)
		}
		hasAdmin := false
		for _, s := range opProj.Surfaces {
			if s.SurfaceID == "admin_models" {
				hasAdmin = true
			}
		}
		if !hasAdmin {
			t.Fatalf("operator projection missing operator-only admin_models")
		}
		if len(opProj.Surfaces) <= len(proj.Surfaces) {
			t.Fatalf("operator projection (%d) not larger than daily_user (%d)", len(opProj.Surfaces), len(proj.Surfaces))
		}
		if opProj.ProjectionDigest() == proj.ProjectionDigest() {
			t.Fatalf("operator and daily_user projections share a digest; audience did not change the projection")
		}
	})

	// ── Fail closed: adapter inconsistency is visible, never a fallback ───────
	t.Run("adapters_fail_closed_without_optimistic_fallback", func(t *testing.T) {
		tamper := func(mutate func(s []ProjectedSurface)) ExperienceProjection {
			bad := proj
			bad.Surfaces = append([]ProjectedSurface(nil), proj.Surfaces...)
			mutate(bad.Surfaces)
			return bad
		}
		// Case A: a route-free group is given a route -> route-on-nonnavigable.
		groupWithRoute := tamper(func(s []ProjectedSurface) {
			for i := range s {
				if s[i].SurfaceID == "work" {
					s[i].Href = "/tampered"
				}
			}
		})
		// Case B: a navigate leaf loses its route -> navigate-without-href.
		leafNoRoute := tamper(func(s []ProjectedSurface) {
			for i := range s {
				if s[i].SurfaceID == "search" {
					s[i].Href = ""
				}
			}
		})

		adapters := map[string]ShadowRenderer{"server": server, "pwa": pwa, "card": card}
		for _, bad := range []ExperienceProjection{groupWithRoute, leafNoRoute} {
			for name, a := range adapters {
				f, err := a.RenderShadow(bad)
				if err == nil {
					t.Fatalf("%s adapter accepted an inconsistent projection with no error (silent fallback)", name)
				}
				var pe *F106PresentationError
				if !errors.As(err, &pe) {
					t.Fatalf("%s adapter returned %T, want *F106PresentationError", name, err)
				}
				if f.Failure == nil {
					t.Fatalf("%s adapter hid the failure (no ShadowFailure marker)", name)
				}
				if f.Settled {
					t.Fatalf("%s adapter produced a SETTLED fixture on bad input (optimistic fallback)", name)
				}
				for _, m := range f.Markers {
					if m.Key == MarkerSettled {
						t.Fatalf("%s adapter emitted a settled marker on failure (optimistic fallback)", name)
					}
				}
			}
		}
	})

	// ── Fail closed at build: availability must come from readiness ───────────
	t.Run("build_rejects_non_readiness_availability_and_missing_outcomes", func(t *testing.T) {
		// A route-registered signal is a structural fact, not readiness, so it
		// can never create availability: the build must fail closed.
		nonReadiness := availabilityFor(cat, "daily_user")
		nonReadiness["search"] = SurfaceAvailabilityOutcome{Signal: SignalRouteRegistered, Value: AvailabilityAvailable}
		_, err := BuildExperienceProjection(cat, ProjectionRequest{
			Audience:     "daily_user",
			Appearance:   ShellAppearance{Theme: ShellThemeSystem, Density: ShellDensityComfortable},
			Availability: nonReadiness,
		})
		if err == nil {
			t.Fatalf("build accepted a non-readiness availability signal (optimistic fallback)")
		}
		var pe *F106PresentationError
		if !errors.As(err, &pe) {
			t.Fatalf("build returned %T, want *F106PresentationError", err)
		}
		if !containsSubstr(pe.Violations, "availability-not-readiness-resolved:search") {
			t.Fatalf("build violations %v missing availability-not-readiness-resolved:search", pe.Violations)
		}

		// A visible surface with no availability outcome must also fail closed.
		missing := availabilityFor(cat, "daily_user")
		delete(missing, "today")
		_, err = BuildExperienceProjection(cat, ProjectionRequest{
			Audience:     "daily_user",
			Appearance:   ShellAppearance{Theme: ShellThemeSystem, Density: ShellDensityComfortable},
			Availability: missing,
		})
		if err == nil {
			t.Fatalf("build accepted a visible surface with no availability outcome")
		}
		if !errors.As(err, &pe) || !containsSubstr(pe.Violations, "missing-availability-outcome:today") {
			t.Fatalf("build error %v missing missing-availability-outcome:today", err)
		}
	})
}

// containsSubstr reports whether any element of in contains sub.
func containsSubstr(in []string, sub string) bool {
	for _, s := range in {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
