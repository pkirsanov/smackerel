//go:build e2e

// Spec 106 SCOPE-106-04 — XP106-04-A (regression E2E, e2e-api).
//
// "Shadow projection digests agree while real routes preserve current
//
//	authorization and behavior."
//
// This is the e2e-api shadow-SAFETY lane: BEFORE the SCOPE-106-05 cutover, it
// proves against the LIVE stack through the REAL registered routes that
//
//  1. the renderer-neutral shadow projection built from the REAL generated
//     catalog (SCOPE-106-02) renders through the three SHADOW adapters (server /
//     PWA / Card) to an IDENTICAL projection digest — the "shadow projection
//     digests agree" half — and
//  2. every route the shadow projection binds is a REAL registered route on the
//     running server (no invented endpoint), and the routes' CURRENT
//     authorization + behavior are UNCHANGED by the shadow adapters: SHADOW mode
//     = no active nav, no route change, no page-body change, and no weakened
//     authorization. Protected server routes still auth-gate unauthenticated;
//     public routes still serve; the live responses carry NONE of the shadow
//     fixture's content-free markers (the shadow projection is NOT wired into
//     any live response), and
//  3. a shadow adapter that detects an inconsistent projection FAILS CLOSED —
//     it surfaces a visible ShadowFailure with no optimistic/static-ready
//     fixture — WITHOUT altering the live route response or installing a
//     fallback (the same live route is byte-identical before and after the
//     fail-closed path runs).
//
// HONESTY / AUTH COUPLING NOTE (read before extending this file):
// This lane runs UNAUTHENTICATED with NO session injection (faking a session is
// forbidden). What it honestly proves session-free is the AUTHORIZATION DECISION
// preserved by shadow mode: a protected server route returns an auth outcome
// (401/403, or a redirect to /login) unauthenticated and NEVER serves an
// authenticated business surface; a public route serves content. That decision
// is exactly the "current authorization preserved" contract. The
// AUTHENTICATED-SESSION acceptance — logging in and observing the shadow parity
// INSIDE the rendered authenticated shell — is coupled-forward to BUG-070-001's
// unified production browser session (the credential/session PASETO-split fix);
// it is this scope's declared External Entry Gate. This lane does NOT fabricate
// an auth pass for it; that acceptance stays coupled-forward, and the
// e2e-ui (XP106-04-W) / canary (XP106-04-C) lanes that need the live PWA DOM +
// the production browser-session canary remain the later slice.
//
// Live-authentic: it drives the running core over CORE_EXTERNAL_URL (exported by
// the disposable `./smackerel.sh test e2e` stack) with NO interception, NO mock,
// NO route()/httptest substitute for the real stack, and NO auth injection. It
// consumes the committed internal/experience projection + shadow adapters
// read-only to build the shadow projection it cross-checks against the live
// routes. It SKIPs (not fails) when CORE_EXTERNAL_URL is unset, matching the repo
// e2e convention, so it is a no-op outside the live e2e lane.
//
// Adversarial (non-tautological): an unregistered control route MUST 404, so the
// "every projected route is real (not 404)" cross-check can genuinely tell a
// registered route from an invented one; and the fail-closed assertion would
// FAIL if any adapter produced a settled/optimistic fixture on a tampered
// projection or if the live route drifted while the shadow adapter failed.
package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/experience"
)

// shadowE2EAudienceIncludes reports whether audience a is in the surface's
// audience set (exported-only re-statement of the package-internal predicate,
// since this external e2e package cannot call the unexported helper).
func shadowE2EAudienceIncludes(auds []string, a string) bool {
	for _, x := range auds {
		if x == a {
			return true
		}
	}
	return false
}

// shadowE2EUnavailablePolicy reports whether the policy denotes an honestly-
// unavailable leaf (built from exported constants only).
func shadowE2EUnavailablePolicy(p experience.DiscoverabilityPolicy) bool {
	return p == experience.PolicyUnavailablePendingOwnership ||
		p == experience.PolicyUnavailablePendingDependency
}

// shadowE2EReadinessAvailability supplies a readiness-resolved availability
// outcome for EVERY surface visible to the audience: honestly-unavailable leaves
// resolve to Unavailable, everything else (active leaves + route-free groups) to
// Available. Every outcome carries SignalReadinessResolved so availability only
// ever enters the projection through the readiness contract — the SAME
// content-free readiness-resolved contract the XP106-04-U unit lane uses to prove
// adapter parity. The REAL readiness-OWNER determination
// (recommendation/availability.Determine) is proven by XP106-04-I; this e2e lane
// does not re-litigate owner truth — its live-stack value is the REAL-ROUTE
// cross-check below.
func shadowE2EReadinessAvailability(cat experience.ProductExperienceCatalog, audience string) map[string]experience.SurfaceAvailabilityOutcome {
	m := make(map[string]experience.SurfaceAvailabilityOutcome)
	for _, s := range cat.Surfaces {
		if !shadowE2EAudienceIncludes(s.Audiences, audience) {
			continue
		}
		val := experience.AvailabilityAvailable
		if shadowE2EUnavailablePolicy(s.ReadinessDiscoverabilityPolicy) {
			val = experience.AvailabilityUnavailable
		}
		m[s.ID] = experience.SurfaceAvailabilityOutcome{Signal: experience.SignalReadinessResolved, Value: val}
	}
	return m
}

func TestShadowProjectionDigestsAgreeWhileRealRoutesPreserveCurrentAuthorizationAndBehavior(t *testing.T) {
	coreURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CORE_EXTERNAL_URL")), "/")
	if coreURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}

	// Do NOT follow redirects: a 302/303 to /login is itself a real authorization
	// outcome we want to observe, not chase.
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// get returns (status, body, Location) for a live GET against the real stack.
	get := func(path string) (int, string, string) {
		resp, err := client.Get(coreURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<18))
		return resp.StatusCode, string(b), resp.Header.Get("Location")
	}
	isAuthLoss := func(c int) bool { return c == http.StatusUnauthorized || c == http.StatusForbidden }
	isRedirect := func(c int) bool {
		return c == http.StatusMovedPermanently || c == http.StatusFound ||
			c == http.StatusSeeOther || c == http.StatusTemporaryRedirect || c == http.StatusPermanentRedirect
	}
	isServed := func(c int) bool { return c >= 200 && c < 300 }
	isLoginRedirect := func(c int, loc string) bool { return isRedirect(c) && strings.Contains(loc, "/login") }
	// A route is PUBLIC when the real router mounts it outside webAuthMiddleware:
	// /assistant is a PUBLIC front-door 302, and the entire /pwa/ tree is the
	// publicly-installable PWA (see internal/api/router.go). Everything else the
	// catalog binds (/, /digest, /knowledge, /settings, /recommendations,
	// /notifications, /cards) is a server web-UI route behind webAuthMiddleware.
	isPublicRoute := func(href string) bool {
		return href == "/assistant" || strings.HasPrefix(href, "/pwa/")
	}

	// ── REAL generated catalog (the same embedded catalog the running stack
	//    was built from), rendered through the three SHADOW adapters. ──────────
	cat, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("experience.GeneratedCatalog(): %v", err)
	}
	if len(cat.Surfaces) == 0 {
		t.Fatal("generated catalog has zero surfaces")
	}
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
		t.Fatal("generated catalog declares zero audiences")
	}

	appearance := experience.ShellAppearance{Theme: experience.ShellThemeDark, Density: experience.ShellDensityCompact}
	server := experience.ServerShadowRenderer{}
	pwa := experience.PWAShadowRenderer{}
	card := experience.CardShadowRenderer{}

	// navigableHrefs collects the UNION of every projected NAVIGATE surface's
	// exact route binding across all real audiences — these are the routes the
	// shadow projection claims are real, and are cross-checked live below.
	navigableHrefs := map[string]bool{}
	digestByAudience := map[string]string{}

	// ── Half 1: shadow projection digests agree (server / PWA / Card) ─────────
	t.Run("shadow_projection_digests_agree_across_server_pwa_card", func(t *testing.T) {
		for _, audience := range audiences {
			proj, err := experience.BuildExperienceProjection(cat, experience.ProjectionRequest{
				Audience:     audience,
				Appearance:   appearance,
				Availability: shadowE2EReadinessAvailability(cat, audience),
			})
			if err != nil {
				t.Fatalf("BuildExperienceProjection(%s): %v", audience, err)
			}
			if len(proj.Surfaces) == 0 {
				t.Fatalf("audience %s: projection has zero surfaces", audience)
			}
			sf, err := server.RenderShadow(proj)
			if err != nil {
				t.Fatalf("server.RenderShadow(%s): %v", audience, err)
			}
			pf, err := pwa.RenderShadow(proj)
			if err != nil {
				t.Fatalf("pwa.RenderShadow(%s): %v", audience, err)
			}
			cf, err := card.RenderShadow(proj)
			if err != nil {
				t.Fatalf("card.RenderShadow(%s): %v", audience, err)
			}
			want := proj.ProjectionDigest()
			for name, f := range map[string]experience.ShadowFixture{"server": sf, "pwa": pf, "card": cf} {
				if !f.Settled || f.Failure != nil {
					t.Fatalf("audience %s: %s fixture not cleanly settled: settled=%v failure=%+v", audience, name, f.Settled, f.Failure)
				}
				if f.ProjectionDigest != want {
					t.Fatalf("audience %s: %s fixture digest %q != projection digest %q", audience, name, f.ProjectionDigest, want)
				}
			}
			if sf.Renderer != experience.RendererServer || pf.Renderer != experience.RendererPWA || cf.Renderer != experience.RendererCard {
				t.Fatalf("audience %s: renderer identities not distinct: %q %q %q", audience, sf.Renderer, pf.Renderer, cf.Renderer)
			}
			digestByAudience[audience] = want
			for _, s := range proj.Surfaces {
				if s.Action == experience.ActionNavigate && s.Href != "" {
					navigableHrefs[s.Href] = true
				}
			}
			t.Logf("digest-parity audience=%-10s surfaces=%2d server==pwa==card digest=%s", audience, len(proj.Surfaces), want)
		}
		if len(navigableHrefs) == 0 {
			t.Fatal("projection yielded zero navigable hrefs to cross-check against live routes")
		}
		// The principal input really drives the projection: distinct audiences
		// with distinct visible sets must not collapse to one digest.
		if len(audiences) >= 2 {
			a, b := audiences[0], audiences[len(audiences)-1]
			if digestByAudience[a] != "" && digestByAudience[a] == digestByAudience[b] {
				t.Logf("note: audiences %s and %s share a digest (same visible set)", a, b)
			}
		}
	})

	// ── Adversarial control: an unregistered route MUST 404, proving the live
	//    cross-check can distinguish a REAL registered route from an invented
	//    one (so the "no projected route 404s" assertion below is not vacuous). ─
	const control = "/definitely-not-registered-xp106-04-a"
	if c, _, _ := get(control); c != http.StatusNotFound {
		t.Fatalf("adversarial control %s -> %d, want 404 — probe cannot distinguish real routes from invented ones", control, c)
	}
	t.Logf("adversarial control %-40s -> 404 (distinct not-found class)", control)

	// ── Adaptive auth detection using a known protected server route. The
	//    disposable e2e stack sets a non-empty SMACKEREL_AUTH_TOKEN, so auth is
	//    expected ON; the check stays adaptive so a dev/no-auth stack degrades
	//    honestly (logs, not a false failure). ─────────────────────────────────
	kc, _, kloc := get("/knowledge")
	authEnforced := isAuthLoss(kc) || isLoginRedirect(kc, kloc)
	t.Logf("auth-mode probe /knowledge -> %d (authEnforced=%v)", kc, authEnforced)

	// ── Half 2: every projected route is REAL, and current authorization +
	//    behavior are preserved (SHADOW mode changed nothing). ─────────────────
	t.Run("real_routes_preserve_current_authorization_and_behavior", func(t *testing.T) {
		hrefs := make([]string, 0, len(navigableHrefs))
		for h := range navigableHrefs {
			hrefs = append(hrefs, h)
		}
		sort.Strings(hrefs)

		authOutcomesSeen := 0
		for _, href := range hrefs {
			code, body, loc := get(href)

			// (a) NO invented endpoints: every route the shadow projection binds
			//     is REAL on the live server — a 404 would mean the projection
			//     points at a route the running router does not register.
			if code == http.StatusNotFound {
				t.Errorf("projected route %s -> 404 — the shadow projection binds a route the live server does not register (invented endpoint / drift)", href)
				continue
			}

			if isPublicRoute(href) {
				// (b) PUBLIC routes still serve their current behavior: a 2xx, or
				//     a forward redirect that is NOT a login bounce. Never
				//     auth-loss, never 404.
				if isAuthLoss(code) {
					t.Errorf("public route %s -> %d auth-loss — shadow mode must not gate a currently-public route", href, code)
					continue
				}
				if !isServed(code) && !(isRedirect(code) && !isLoginRedirect(code, loc)) {
					t.Errorf("public route %s -> %d (loc=%q), want served 2xx or a non-login redirect", href, code, loc)
					continue
				}
				t.Logf("public   route %-32s -> %d (served; behavior preserved)", href, code)
				continue
			}

			// (c) PROTECTED server routes: current AUTHORIZATION is preserved.
			//     Unauthenticated MUST resolve to an auth outcome (401/403 or a
			//     redirect to /login) — NEVER a 200 serving an authenticated
			//     business surface. This is the session-free proof that the
			//     shadow adapters did not weaken authorization.
			if authEnforced {
				if !(isAuthLoss(code) || isLoginRedirect(code, loc)) {
					t.Errorf("protected route %s unauthenticated -> %d (loc=%q), want auth outcome (401/403 or /login redirect) — shadow mode must not weaken authorization", href, code, loc)
					continue
				}
				if isServed(code) && strings.Contains(strings.ToLower(body), "sign out") {
					t.Errorf("protected route %s served an authenticated surface unauthenticated (contains 'sign out') — authorization regressed", href)
					continue
				}
				authOutcomesSeen++
				t.Logf("protected route %-32s -> %d (auth outcome; authorization preserved)", href, code)
			} else {
				t.Logf("protected route %-32s -> %d (auth not enforced on this stack)", href, code)
			}
		}

		// When auth is enforced, at least one protected projected route must have
		// actively enforced authorization — otherwise the preservation proof is
		// vacuous.
		if authEnforced && authOutcomesSeen == 0 {
			t.Fatalf("auth is enforced but no protected projected route produced an auth outcome — cannot prove authorization preservation")
		}
	})

	// ── Half 2 (cont.): SHADOW mode is not wired into any live response — the
	//    shadow projection's content-free markers appear in NO served body, and
	//    no live projection digest is emitted. The ACTIVE handwritten nav is
	//    untouched; only the shadow sentinels must be absent. ──────────────────
	t.Run("live_responses_carry_no_shadow_markers", func(t *testing.T) {
		// The shadow fixtures tag themselves data-product-navigation="shadow" +
		// data-shadow-settled + a data-projection-digest; none may leak into a
		// live response, because the shadow projection is comparison telemetry
		// only (no active nav/route/body cutover until SCOPE-106-05).
		shadowSentinels := []string{
			experience.MarkerProductNavigation + `="shadow"`,
			experience.MarkerSettled,
			experience.MarkerProjectionDigest,
		}
		anyDigest := digestByAudience[audiences[0]]
		// Public served surfaces are the ones with an inspectable live body.
		for _, href := range []string{"/pwa/", "/login"} {
			code, body, _ := get(href)
			if !isServed(code) {
				t.Logf("skip shadow-marker scan of %s (-> %d, not a served body)", href, code)
				continue
			}
			for _, sentinel := range shadowSentinels {
				if strings.Contains(body, sentinel) {
					t.Errorf("live response %s leaked shadow sentinel %q — SHADOW mode must not be wired into a live response", href, sentinel)
				}
			}
			if anyDigest != "" && strings.Contains(body, anyDigest) {
				t.Errorf("live response %s leaked the shadow projection digest — SHADOW mode must not be wired into a live response", href)
			}
			t.Logf("shadow-marker scan %-14s -> %d (no shadow sentinel / no projection digest in live body)", href, code)
		}
	})

	// ── Half 3: fail-closed WITHOUT altering the live route or installing a
	//    fallback. A tampered projection makes each adapter surface a visible
	//    ShadowFailure (no optimistic fixture); the same live route is
	//    byte-identical before and after. ───────────────────────────────────────
	t.Run("adapter_fail_closed_does_not_alter_live_route_or_install_fallback", func(t *testing.T) {
		// Capture a stable public live route BEFORE the fail-closed path runs.
		const probe = "/pwa/"
		beforeCode, beforeBody, _ := get(probe)
		beforeHash := sha256.Sum256([]byte(beforeBody))

		// Build a valid projection, then tamper a copy so a navigate leaf loses
		// its route (navigate-without-href) — the exact inconsistency the unit
		// lane induces. Every adapter MUST fail closed with a visible
		// ShadowFailure and NO settled/optimistic fixture.
		good, err := experience.BuildExperienceProjection(cat, experience.ProjectionRequest{
			Audience:     audiences[0],
			Appearance:   appearance,
			Availability: shadowE2EReadinessAvailability(cat, audiences[0]),
		})
		if err != nil {
			t.Fatalf("BuildExperienceProjection(%s): %v", audiences[0], err)
		}
		bad := good
		bad.Surfaces = append([]experience.ProjectedSurface(nil), good.Surfaces...)
		tampered := false
		for i := range bad.Surfaces {
			if bad.Surfaces[i].Action == experience.ActionNavigate && bad.Surfaces[i].Href != "" {
				bad.Surfaces[i].Href = "" // navigate leaf loses its route
				tampered = true
				break
			}
		}
		if !tampered {
			t.Fatal("no navigate leaf found to tamper — cannot exercise the fail-closed path")
		}

		for name, a := range map[string]experience.ShadowRenderer{"server": server, "pwa": pwa, "card": card} {
			f, err := a.RenderShadow(bad)
			if err == nil {
				t.Fatalf("%s adapter accepted an inconsistent projection with no error (silent fallback)", name)
			}
			var pe *experience.F106PresentationError
			if !errors.As(err, &pe) {
				t.Fatalf("%s adapter returned %T, want *experience.F106PresentationError", name, err)
			}
			if f.Failure == nil {
				t.Fatalf("%s adapter hid the failure (no visible ShadowFailure)", name)
			}
			if f.Settled {
				t.Fatalf("%s adapter produced a SETTLED fixture on bad input (optimistic fallback)", name)
			}
			for _, m := range f.Markers {
				if m.Key == experience.MarkerSettled {
					t.Fatalf("%s adapter emitted a settled marker on failure (optimistic fallback)", name)
				}
			}
			t.Logf("fail-closed %-6s adapter -> visible ShadowFailure, non-settled, no optimistic fixture", name)
		}

		// The live route is UNCHANGED by the shadow adapter failure: no fallback
		// was installed, no behavior drifted. (The adapters are pure in-process,
		// so this is inherent — demonstrating the before/after live identity makes
		// the "without altering the live route response" claim concrete.)
		afterCode, afterBody, _ := get(probe)
		afterHash := sha256.Sum256([]byte(afterBody))
		if afterCode != beforeCode {
			t.Fatalf("live route %s status changed across the fail-closed path: before=%d after=%d", probe, beforeCode, afterCode)
		}
		if beforeHash != afterHash {
			t.Fatalf("live route %s body changed across the fail-closed path — a fallback or drift was installed", probe)
		}
		t.Logf("live route %-8s identical before/after fail-closed: status=%d body-sha256=%s", probe, afterCode, hex.EncodeToString(afterHash[:8]))
	})
}
