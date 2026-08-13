// Spec 108 Scope 03 — route-manifest contract for the corpus-grant gate
// (design.md T8). Proves that ALL SIXTEEN corpus route groups of spec.md §4.2
// sit behind the gate, that the gated set matches EXACTLY (not merely a
// subset), and that the routes design.md §2 lists as deliberately ungated are
// never corpus-denied in either stage.
//
// ── Enumeration approach: chi.Walk for the inventory, BEHAVIOUR for gatedness
//
// Routes are discovered with chi.Walk over the real router, so no
// hand-maintained path list decides WHICH routes exist — that is the drift a
// hand list invites, and it is exactly how a new corpus handler ships ungated.
//
// Gate membership, however, is asserted BEHAVIOURALLY (a 403 naming
// `corpus:read`) rather than by inspecting the middleware chain chi.Walk
// hands back, for two independent reasons:
//
//  1. chi.Walk cannot see the gate on six of the routes. `r.Group` builds an
//     inline mux whose middlewares are baked into a *ChainHandler at insert
//     time (chi v5.2.2 mux.go `handle`: `h = Chain(mx.middlewares...)...`).
//     chi's `walk` unwraps that ChainHandler only in the `route.Handlers`
//     loop — the `route.SubRoutes` branch recurses WITHOUT unwrapping. Group 8
//     mounts its six endpoints via `r.Route("/knowledge", …)` INSIDE the
//     gated inline group, so those six expose none of the gate's middleware
//     to Walk. A middleware-introspection test would silently cover 15 of 21
//     routes.
//  2. Middleware values are closures, comparable only by code pointer, and
//     `RequireScope("corpus:read")` shares its code pointer with
//     `RequireScope("annotation:edit")` and `RequireScope("knowledge-graph:read")`
//     — the same func literal. Introspection therefore cannot tell the corpus
//     gate from the adjacent scope gates it must NOT be confused with. The
//     denial body names the required scope, so behaviour discriminates
//     precisely where introspection cannot.
//
// ── Non-vacuity
//
// The router is built with a NON-NIL IntelligenceEngine and ContextHandler.
// With either nil, those route groups never register, and a subset-style
// assertion would pass against a router that never mounted them.
// TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap makes that trap explicit.
package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/metrics"
)

// routeKey identifies one mounted endpoint as chi reports it.
type routeKey struct {
	Method  string
	Pattern string
}

func (r routeKey) String() string { return r.Method + " " + r.Pattern }

// corpusGroupRoutes is the EXPECTED mount manifest: the sixteen route groups
// of spec.md §4.2 (Tier A 1-8, Tier B 9-16) against the routes each one
// covers.
//
// This table is written out rather than derived from router.go, for the same
// reason internal/metrics/corpus_grant_test.go writes out its label set:
// deriving the expectation from the thing under test makes the assertion
// tautological. Set equality against the WALKED router is what gives the
// explicit table its teeth — a seventeenth gated route fails the test even
// though no line here mentions it.
var corpusGroupRoutes = map[metrics.CorpusRouteGroup][]routeKey{
	// Tier A — raw corpus retrieval (groups 1-8).
	metrics.CorpusRouteGroupSearch:         {{http.MethodPost, "/api/search"}},
	metrics.CorpusRouteGroupDigest:         {{http.MethodGet, "/api/digest"}},
	metrics.CorpusRouteGroupRecent:         {{http.MethodGet, "/api/recent"}},
	metrics.CorpusRouteGroupArtifactDetail: {{http.MethodGet, "/api/artifact/{id}"}},
	metrics.CorpusRouteGroupArtifactDomain: {{http.MethodGet, "/api/artifacts/{id}/domain"}},
	metrics.CorpusRouteGroupExport:         {{http.MethodGet, "/api/export"}},
	metrics.CorpusRouteGroupContextFor:     {{http.MethodPost, "/api/context-for"}},
	metrics.CorpusRouteGroupKnowledge: {
		{http.MethodGet, "/api/knowledge/concepts"},
		{http.MethodGet, "/api/knowledge/concepts/{id}"},
		{http.MethodGet, "/api/knowledge/entities"},
		{http.MethodGet, "/api/knowledge/entities/{id}"},
		{http.MethodGet, "/api/knowledge/lint"},
		{http.MethodGet, "/api/knowledge/stats"},
	},

	// Tier B — corpus-derived Phase-5 intelligence (groups 9-16).
	metrics.CorpusRouteGroupExpertise:        {{http.MethodGet, "/api/expertise"}},
	metrics.CorpusRouteGroupLearningPaths:    {{http.MethodGet, "/api/learning-paths"}},
	metrics.CorpusRouteGroupSubscriptions:    {{http.MethodGet, "/api/subscriptions"}},
	metrics.CorpusRouteGroupSerendipity:      {{http.MethodGet, "/api/serendipity"}},
	metrics.CorpusRouteGroupContentFuel:      {{http.MethodGet, "/api/content-fuel"}},
	metrics.CorpusRouteGroupQuickReferences:  {{http.MethodGet, "/api/quick-references"}},
	metrics.CorpusRouteGroupMonthlyReport:    {{http.MethodGet, "/api/monthly-report"}},
	metrics.CorpusRouteGroupSeasonalPatterns: {{http.MethodGet, "/api/seasonal-patterns"}},
}

// tierBGroups are the eight groups that register only when
// deps.IntelligenceEngine is non-nil — the vacuity trap.
var tierBGroups = []metrics.CorpusRouteGroup{
	metrics.CorpusRouteGroupExpertise,
	metrics.CorpusRouteGroupLearningPaths,
	metrics.CorpusRouteGroupSubscriptions,
	metrics.CorpusRouteGroupSerendipity,
	metrics.CorpusRouteGroupContentFuel,
	metrics.CorpusRouteGroupQuickReferences,
	metrics.CorpusRouteGroupMonthlyReport,
	metrics.CorpusRouteGroupSeasonalPatterns,
}

// ungatedRoutes is the design.md §2 "Routes deliberately NOT gated" list,
// transcribed as concrete probe requests. Path parameters carry the same
// "probe-id" placeholder concretePath substitutes, and collection endpoints
// registered as `r.Route("/x", r.Get("/"))` carry the trailing slash chi
// actually mounts — a probe that misses the mounted shape is caught by the
// not-mounted guard below rather than passing as a silent 404.
//
// The §2 row for `/api/expertise` and the other Phase-5 endpoints is
// deliberately ABSENT: that row is struck through in design.md and marked
// "SUPERSEDED 2026-07-29 by spec.md §18 decision 5 — these eight endpoints are
// now IN SCOPE and gated as Tier B". Reading §2's table without honoring the
// supersession would assert the exact opposite of §4.2.
var ungatedRoutes = []struct {
	method string
	path   string
	why    string
}{
	{http.MethodPost, "/api/capture", "write path; gating it breaks capture for every daily user"},
	{http.MethodPost, "/api/bookmarks/import", "write path"},
	{http.MethodPost, "/api/assistant/turn", "mediated corpus access, gated by the assistant:turn claim"},
	{http.MethodPost, "/api/artifacts/probe-id/annotations/", "user-authored bodies, gated by annotation:edit"},
	{http.MethodGet, "/api/artifacts/probe-id/annotations/", "user-authored bodies, gated by annotation:edit"},
	{http.MethodGet, "/api/artifacts/probe-id/annotations/summary", "user-authored bodies, gated by annotation:edit"},
	{http.MethodDelete, "/api/artifacts/probe-id/tags/probe-id", "user-authored bodies, gated by annotation:edit"},
	{http.MethodGet, "/api/annotations", "user-authored bodies, gated by annotation:edit"},
	{http.MethodPost, "/api/internal/telegram-message-artifact", "internal id-to-id mapping; returns no corpus content"},
	{http.MethodGet, "/api/internal/telegram-message-artifact", "internal id-to-id mapping; returns no corpus content"},
	{http.MethodGet, "/api/topics/", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/topics/probe-id", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/people/", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/people/probe-id", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/places/", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/places/probe-id", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/time", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/graph/edges", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/health", "unauthenticated by design; carries no corpus data"},
	{http.MethodGet, "/readyz", "unauthenticated by design; carries no corpus data"},
	{http.MethodGet, "/metrics", "unauthenticated by design; carries no corpus data"},
}

// NOT LISTED ABOVE, and deliberately so: the server-rendered HTMX routes
// `POST /search`, `GET /artifact/{id}`, `GET /digest`, `GET /knowledge/concepts/{id}`
// (SEC-108-01). They READ THE CORPUS server-side — `SearchResults` calls the
// executor and `ArtifactDetail` queries `artifacts` directly — so they are not
// API clients of the gated group and the gate never sees them in either stage.
//
// They belong in this fixture on the merits, and adding them was ATTEMPTED. The
// non-vacuity guard below correctly rejected it: `corpusGateDeps` leaves
// `Dependencies.WebHandler` nil, so the whole `if deps.WebHandler != nil` group
// is unmounted here and every assertion about those four paths would have been
// vacuous. Listing them anyway — or relaxing the mounted-check to let them
// through — would have produced four lines that look like coverage and prove
// nothing, which is worse than the honest gap.
//
// Pinning them for real requires wiring a REAL web handler into this fixture,
// and that handler needs a live pool (`h.Pool`), which makes it an integration-
// tier assertion rather than a unit one. Recorded as SEC-108-01 and routed
// rather than faked here. Their disposition is documented in design.md §2.


// corpusGateDeps builds a router-ready Dependencies for the gate contract.
//
// withIntelligence controls the vacuity trap: false leaves
// deps.IntelligenceEngine nil, which un-registers all eight Tier B groups.
// Every OTHER conditional handler is wired so the deliberately-ungated routes
// of design.md §2 actually mount and can be probed rather than silently 404ing.
func corpusGateDeps(t *testing.T, enforce, withIntelligence bool) (*Dependencies, string) {
	t.Helper()
	silenceRouterLogs(t)
	deps, priv, _ := newProductionAuthDeps(t)

	deps.CorpusGrantEnforce = enforce
	deps.ContextHandler = &ContextHandler{}
	if withIntelligence {
		deps.IntelligenceEngine = &intelligence.Engine{}
	}

	// Mount the adjacent non-corpus surfaces so the no-over-reach probe is
	// asserting against real routes.
	deps.AnnotationHandlers = &AnnotationHandlers{}
	deps.GraphCapability = graphapi.NewGraphCapability(graphapi.Config{})
	deps.AssistantTurnHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return deps, priv
}

// silenceRouterLogs mutes the router's per-request structured logging for the
// duration of one test. These tests sweep every mounted route across four
// router constructions; left on, that is several hundred log lines of noise
// that bury a real failure. Nothing here asserts on log output.
func silenceRouterLogs(t *testing.T) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
}

// ungrantedToken mints a valid per-user PASETO carrying NO scope claim: an
// authenticated principal that holds no grant at all. It is the adversarial
// input — the exact principal that passes an ungated router — so these tests
// fail against a router whose gate has been removed.
func ungrantedToken(t *testing.T, deps *Dependencies, priv string) string {
	t.Helper()
	issued, err := auth.IssueToken(auth.IssueOptions{
		UserID:     "ungranted-principal",
		TokenID:    "tok-corpus-gate-contract",
		SigningKey: priv,
		KeyID:      deps.AuthConfig.SigningActiveKeyID,
		TTL:        time.Hour,
		Issuer:     "smackerel",
		Now:        time.Now,
		Scopes:     nil,
	})
	if err != nil {
		t.Fatalf("issue ungranted token: %v", err)
	}
	return issued.WireToken
}

// walkMountedRoutes enumerates the routes the router ACTUALLY mounted.
func walkMountedRoutes(t *testing.T, h http.Handler) []routeKey {
	t.Helper()
	mux, ok := h.(*chi.Mux)
	if !ok {
		t.Fatalf("NewRouter returned %T, want *chi.Mux — route enumeration needs chi.Routes", h)
	}

	seen := make(map[routeKey]struct{})
	err := chi.Walk(mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		// Mount stubs surface as a trailing wildcard; their leaves are
		// reported separately by the SubRoutes recursion.
		route = strings.TrimSuffix(route, "/*")
		if route == "" || strings.Contains(route, "*") {
			return nil
		}
		seen[routeKey{method, route}] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}

	out := make([]routeKey, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Pattern != out[j].Pattern {
			return out[i].Pattern < out[j].Pattern
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// isCorpusDenied reports whether a response is the corpus gate's refusal,
// distinguished from every adjacent scope gate by the scope the body names.
func isCorpusDenied(rec *httptest.ResponseRecorder) bool {
	if rec.Code != http.StatusForbidden {
		return false
	}
	var body struct {
		Error    string   `json:"error"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		return false
	}
	return body.Error == "scope_required" && slices.Contains(body.Required, auth.GrantGlobalCorpusRead)
}

// probe issues one authenticated request and reports whether the corpus gate
// refused it.
func probe(t *testing.T, router http.Handler, token, method, path string) bool {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return isCorpusDenied(rec)
}

// concretePath substitutes chi path parameters so a pattern can be requested.
// Under ENFORCE the gate refuses before the handler, so the value never
// reaches a lookup.
func concretePath(pattern string) string {
	segs := strings.Split(pattern, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, "{") {
			segs[i] = "probe-id"
		}
	}
	return strings.Join(segs, "/")
}

// expectedGatedRoutes flattens corpusGroupRoutes into one route set.
func expectedGatedRoutes() map[routeKey]metrics.CorpusRouteGroup {
	out := make(map[routeKey]metrics.CorpusRouteGroup)
	for group, routes := range corpusGroupRoutes {
		for _, r := range routes {
			out[r] = group
		}
	}
	return out
}

// observeGatedRoutes probes every mounted route and returns the set the corpus
// gate actually refuses. The set is DERIVED from the live router, never read
// from a list.
func observeGatedRoutes(t *testing.T, router http.Handler, token string) map[routeKey]struct{} {
	t.Helper()
	gated := make(map[routeKey]struct{})
	for _, r := range walkMountedRoutes(t, router) {
		if probe(t, router, token, r.Method, concretePath(r.Pattern)) {
			gated[r] = struct{}{}
		}
	}
	return gated
}

// TestCorpusGate_AllSixteenRouteGroupsGated is the core contract: the gated
// surface is EXACTLY the sixteen route groups of spec.md §4.2.
//
// Set equality is asserted in BOTH directions. A subset check would pass on a
// router that mounted only Tier A, and an "every expected route is gated"
// check alone would miss a seventeenth corpus route added inside the group.
func TestCorpusGate_AllSixteenRouteGroupsGated(t *testing.T) {
	deps, priv := corpusGateDeps(t, true, true)
	router := NewRouter(deps)
	token := ungrantedToken(t, deps, priv)

	expected := expectedGatedRoutes()
	observed := observeGatedRoutes(t, router, token)

	t.Run("expectation_covers_the_closed_sixteen_value_label_set", func(t *testing.T) {
		if len(corpusGroupRoutes) != 16 {
			t.Fatalf("corpusGroupRoutes has %d groups, want exactly 16 (spec.md §4.2 Tier A + Tier B)", len(corpusGroupRoutes))
		}
		for _, group := range metrics.CorpusRouteGroups() {
			if _, ok := corpusGroupRoutes[group]; !ok {
				t.Errorf("route_group %q is in the closed metrics label set but has NO mounted route in the expectation — "+
					"a label that no route can ever emit", group)
			}
		}
		for group := range corpusGroupRoutes {
			if err := metrics.ValidateCorpusRouteGroup(group); err != nil {
				t.Errorf("expected group %q is not in the closed metrics label set: %v", group, err)
			}
		}
	})

	t.Run("every_expected_corpus_route_is_gated", func(t *testing.T) {
		for r, group := range expected {
			if _, ok := observed[r]; !ok {
				t.Errorf("UNGATED corpus route: %s (group %q) did not return 403 scope_required[%s] for a principal holding no grant — "+
					"it is reachable without the corpus grant", r, group, auth.GrantGlobalCorpusRead)
			}
		}
	})

	t.Run("no_unexpected_route_is_gated", func(t *testing.T) {
		for r := range observed {
			if _, ok := expected[r]; !ok {
				t.Errorf("UNEXPECTED gated route: %s is behind the corpus gate but is not one of the sixteen groups of spec.md §4.2 — "+
					"either it is gate over-reach, or it is a new corpus route that must be added to §4.2, the metrics label set, and this table", r)
			}
		}
	})

	t.Run("gated_group_count_is_exactly_sixteen", func(t *testing.T) {
		covered := make(map[metrics.CorpusRouteGroup]struct{})
		for r := range observed {
			if group, ok := expected[r]; ok {
				covered[group] = struct{}{}
			}
		}
		if len(covered) != 16 {
			missing := make([]string, 0)
			for group := range corpusGroupRoutes {
				if _, ok := covered[group]; !ok {
					missing = append(missing, string(group))
				}
			}
			sort.Strings(missing)
			t.Fatalf("only %d of 16 corpus route groups are gated; ungated groups: %v", len(covered), missing)
		}
	})
}

// TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap documents WHY the core
// test must construct a non-nil IntelligenceEngine.
//
// With a nil engine the eight Tier B routes never register, so a subset-style
// assertion ("every route I know about is gated") passes vacuously against a
// router that mounted only Tier A. This test asserts the trap is real: fewer
// than sixteen groups are gated, and precisely the Tier B eight are missing.
//
// It also fails if Tier B is ever made unconditional without updating the
// expectation — at which point the vacuity trap is gone and this test, not a
// production incident, is what says so.
func TestCorpusGate_NilIntelligenceEngineIsAVacuityTrap(t *testing.T) {
	deps, priv := corpusGateDeps(t, true, false)
	router := NewRouter(deps)
	token := ungrantedToken(t, deps, priv)

	expected := expectedGatedRoutes()
	observed := observeGatedRoutes(t, router, token)

	covered := make(map[metrics.CorpusRouteGroup]struct{})
	for r := range observed {
		if group, ok := expected[r]; ok {
			covered[group] = struct{}{}
		}
	}

	if len(covered) >= 16 {
		t.Fatalf("a NIL IntelligenceEngine yielded %d gated groups (>= 16) — the Tier B routes are no longer conditional. "+
			"The core test's non-nil construction may no longer be what makes it non-vacuous; re-derive the expectation.", len(covered))
	}

	for _, group := range tierBGroups {
		if _, ok := covered[group]; ok {
			t.Errorf("Tier B group %q is gated with a NIL IntelligenceEngine — it was expected not to register at all", group)
		}
	}

	if len(covered) != len(corpusGroupRoutes)-len(tierBGroups) {
		t.Errorf("with a NIL IntelligenceEngine %d groups are gated, want %d (Tier A only) — "+
			"a Tier A group is no longer gated, or the tier split has moved",
			len(covered), len(corpusGroupRoutes)-len(tierBGroups))
	}
}

// TestCorpusGate_DoesNotOverReachUngatedRoutes is the second half of the
// contract: gate over-reach is as much a defect as gate absence. A corpus
// denial on /api/capture breaks ingestion for every daily user.
//
// Checked in BOTH stages, because a stage-conditional over-reach would be
// invisible in whichever stage the test skipped.
func TestCorpusGate_DoesNotOverReachUngatedRoutes(t *testing.T) {
	for _, stage := range []struct {
		name    string
		enforce bool
	}{
		{"observe", false},
		{"enforce", true},
	} {
		t.Run(stage.name, func(t *testing.T) {
			deps, priv := corpusGateDeps(t, stage.enforce, true)
			router := NewRouter(deps)
			token := ungrantedToken(t, deps, priv)

			mounted := make(map[string]struct{})
			for _, r := range walkMountedRoutes(t, router) {
				mounted[r.Method+" "+concretePath(r.Pattern)] = struct{}{}
			}

			for _, u := range ungatedRoutes {
				key := u.method + " " + u.path
				if _, ok := mounted[key]; !ok {
					t.Errorf("design.md §2 lists %s as deliberately ungated, but it is NOT MOUNTED — "+
						"the no-over-reach assertion for it would be vacuous", key)
					continue
				}
				if probe(t, router, token, u.method, u.path) {
					t.Errorf("GATE OVER-REACH: %s is corpus-denied but design.md §2 lists it as deliberately ungated (%s)",
						key, u.why)
				}
			}
		})
	}
}

// TestCorpusGate_RouteManifestIsStageIndependent proves the stage flag selects
// only whether the ENFORCE half mounts — never WHICH routes exist. A corpus
// route that registered in one stage and not the other would make the
// OBSERVE-stage counter a false predictor of the ENFORCE outcome, which is the
// whole basis on which the operator decides to flip the flag.
func TestCorpusGate_RouteManifestIsStageIndependent(t *testing.T) {
	observeDeps, _ := corpusGateDeps(t, false, true)
	enforceDeps, _ := corpusGateDeps(t, true, true)

	observeRoutes := walkMountedRoutes(t, NewRouter(observeDeps))
	enforceRoutes := walkMountedRoutes(t, NewRouter(enforceDeps))

	if !slices.Equal(observeRoutes, enforceRoutes) {
		t.Fatalf("the mounted route manifest differs by stage:\n OBSERVE (%d): %v\n ENFORCE (%d): %v",
			len(observeRoutes), observeRoutes, len(enforceRoutes), enforceRoutes)
	}
}
