//go:build integration

// Spec 108 Scope 03 — TP-03-02, TP-03-11, TP-03-05.
//
// THE ENFORCE STAGE, PROVED ON THE WIRE.
//
// Until this file, the claim "an ungranted principal is REFUSED on the corpus
// surface" was proved only at the unit tier, by
// `internal/api.TestCorpusGate_AllSixteenRouteGroupsGated`. That test is a
// route-MANIFEST assertion: it proves the middleware is MOUNTED on the right
// set of routes. It does not prove that a real bearer, arriving over a real
// socket at a real `api.NewRouter`, is actually turned away — nor that a real
// `corpus:read` holder still gets through. The neighbouring integration file
// `tests/integration/corpus_grant_observe_test.go` constructs
// `api.NewCorpusGrantGate(false)` — OBSERVE — at every one of its three call
// sites, so nothing anywhere exercised ENFORCE end to end.
//
// This file closes that gap. It drives the REAL production router over a REAL
// loopback HTTP server (httptest) against the disposable stack's REAL
// PostgreSQL (DATABASE_URL), with the REAL per-user PASETO bearer middleware
// and the REAL `auth.RequireScope(auth.GrantGlobalCorpusRead)` ENFORCE half.
// There is NO request interception, NO mock, NO stub handler on any corpus
// route.
//
// Why in-process rather than the running smackerel-core container? For exactly
// the reason `corpus_authorization_test.go` records in its own header: the live
// test stack runs AUTH_ENABLED=false, so the shared bearer collapses
// `RequireScope` to its `SessionSourceSharedToken` bypass and every identity
// looks identical. A container-based test structurally cannot express a grant
// matrix. Building `api.NewRouter` in-process with Environment="production" +
// AuthConfig.Enabled=true (mirroring cmd/core/wiring.go's per-user branch) is
// the only way to vary the scope set PER REQUEST while keeping the router, the
// middleware chain, the PostgreSQL-backed handlers, and both gate halves real.
//
// ── NON-VACUITY, which is the entire point ──────────────────────────────────
//
//  1. BOTH ARMS ON THE SAME ROUTE. Every route is driven twice in the same
//     test: once by a principal WITHOUT `corpus:read` (must be refused) and
//     once by a principal WITH it (must not be refused). A negative-only test
//     passes when the whole stack is broken — 403 everywhere is indis-
//     tinguishable from a gate that works. A positive-only test passes when
//     the gate is absent entirely. Only the PAIR separates "the gate works"
//     from "everything is broken" and from "nothing is guarding anything".
//
//  2. ALL SIXTEEN GROUPS, COUNTED. The route table's key set is asserted EQUAL
//     to `metrics.CorpusRouteGroups()` — the exported closed set — and the
//     number of groups actually exercised is asserted to be sixteen. The loop
//     cannot silently shrink, and a seventeenth group added to the closed set
//     fails this file until it is given a route here.
//
//  3. A NON-NIL INTELLIGENCE ENGINE. `router.go` registers the eight Tier B
//     Phase-5 groups only when `deps.IntelligenceEngine != nil`. With it nil,
//     every Tier B assertion would pass vacuously against a router that only
//     ever mounted eight groups. This file wires a REAL
//     `intelligence.NewEngine` over the live pool, and the ungranted arm's
//     demand for a corpus denial on each Tier B route is itself the mount
//     proof: an unmounted route 404s and fails the assertion.
//
//  4. GATE OVER-REACH IS A DEFECT TOO. The routes design.md §2 lists as
//     deliberately NOT gated are probed under ENFORCE and must still be
//     reachable. Several of them are gated by a DIFFERENT scope, and the
//     assertion checks that their denial names that other scope rather than
//     `corpus:read` — so a gate that swallowed the whole `/api` surface fails
//     here even though every corpus assertion above would still pass.
//
// The route table below is transcribed from `internal/api/router_corpus_gate_test.go`
// (`corpusGroupRoutes`, `ungatedRoutes`) and spec.md §4.2. It cannot be
// imported: that file is an internal test file of package `api`. The
// transcription is anchored against drift by the closed-set equality check in
// (2), which is derived from exported production code, not from the table.
package graphapi_integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
	"github.com/smackerel/smackerel/internal/auth"
	"github.com/smackerel/smackerel/internal/auth/revocation"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/intelligence"
	"github.com/smackerel/smackerel/internal/metrics"
)

// Canaries planted in every request's path, query, and body. A refusal that
// echoed any of them back would be telling an unauthorized caller what it had
// just asked for; the denial-shape assertion scans for them.
const (
	corpusEnforceCanaryQuery = "tp0302-canary-private-question-do-not-echo"
	corpusEnforceCanaryID    = "tp0302-canary-artifact-identifier"
)

// corpusEnforceRoute is one concrete, requestable endpoint. Path parameters are
// already substituted: under ENFORCE the gate refuses ahead of the handler, so
// the value never reaches a lookup, and under the granted arm it only has to be
// well-formed enough to reach the handler's own validation.
type corpusEnforceRoute struct {
	method string
	path   string
	// body is the JSON request body for POST routes; empty for GET.
	body string
}

// corpusEnforceTierA is groups 1-8 of spec.md §4.2 — raw corpus retrieval.
// Group 8 lists all six knowledge endpoints because `router.go` attaches the
// gate to their enclosing `r.Route("/knowledge", …)` as one unit; probing a
// single member would not show that the other five inherited it.
var corpusEnforceTierA = map[metrics.CorpusRouteGroup][]corpusEnforceRoute{
	metrics.CorpusRouteGroupSearch: {
		{http.MethodPost, "/api/search", `{"query":"` + corpusEnforceCanaryQuery + `","limit":5}`},
	},
	metrics.CorpusRouteGroupDigest: {
		{http.MethodGet, "/api/digest?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupRecent: {
		{http.MethodGet, "/api/recent?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupArtifactDetail: {
		{http.MethodGet, "/api/artifact/" + corpusEnforceCanaryID, ""},
	},
	metrics.CorpusRouteGroupArtifactDomain: {
		{http.MethodGet, "/api/artifacts/" + corpusEnforceCanaryID + "/domain", ""},
	},
	metrics.CorpusRouteGroupExport: {
		{http.MethodGet, "/api/export?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupContextFor: {
		{http.MethodPost, "/api/context-for", `{"entityType":"unknown-type","entityId":"` + corpusEnforceCanaryID + `"}`},
	},
	metrics.CorpusRouteGroupKnowledge: {
		{http.MethodGet, "/api/knowledge/concepts?q=" + corpusEnforceCanaryQuery, ""},
		{http.MethodGet, "/api/knowledge/concepts/" + corpusEnforceCanaryID, ""},
		{http.MethodGet, "/api/knowledge/entities?q=" + corpusEnforceCanaryQuery, ""},
		{http.MethodGet, "/api/knowledge/entities/" + corpusEnforceCanaryID, ""},
		{http.MethodGet, "/api/knowledge/lint", ""},
		{http.MethodGet, "/api/knowledge/stats", ""},
	},
}

// corpusEnforceTierB is groups 9-16 — the corpus-DERIVED Phase-5 intelligence
// endpoints brought in scope by spec.md §18 decision 5. They register only when
// `deps.IntelligenceEngine != nil`; see non-vacuity note (3) in the header.
var corpusEnforceTierB = map[metrics.CorpusRouteGroup][]corpusEnforceRoute{
	metrics.CorpusRouteGroupExpertise: {
		{http.MethodGet, "/api/expertise?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupLearningPaths: {
		{http.MethodGet, "/api/learning-paths?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupSubscriptions: {
		{http.MethodGet, "/api/subscriptions?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupSerendipity: {
		{http.MethodGet, "/api/serendipity?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupContentFuel: {
		{http.MethodGet, "/api/content-fuel?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupQuickReferences: {
		{http.MethodGet, "/api/quick-references?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupMonthlyReport: {
		{http.MethodGet, "/api/monthly-report?q=" + corpusEnforceCanaryQuery, ""},
	},
	metrics.CorpusRouteGroupSeasonalPatterns: {
		{http.MethodGet, "/api/seasonal-patterns?q=" + corpusEnforceCanaryQuery, ""},
	},
}

// corpusEnforceUngated transcribes the design.md §2 "routes deliberately NOT
// gated" rows that this in-process router actually mounts, with the reason each
// one is out of scope. `otherScope`, when set, is the scope that DOES gate the
// route: the ungranted principal below holds `annotation:edit` and not
// `knowledge-graph:read`, so those rows must produce a denial naming
// `knowledge-graph:read` — which simultaneously proves the route is mounted,
// proves the corpus gate did not swallow it, and proves the adjacent gate is
// still doing its own job.
//
// The annotation and assistant rows of §2 are absent by construction: they
// mount only when `deps.AnnotationHandlers` / `deps.AssistantTurnHandler` are
// wired, and wiring a stub handler for them inside an integration test would
// substitute a fake for the thing under test. They are covered at the unit tier
// by `TestCorpusGate_AllSixteenRouteGroupsGated`'s no-over-reach probe.
var corpusEnforceUngated = []struct {
	method     string
	path       string
	otherScope string
	why        string
}{
	{http.MethodGet, "/api/topics/", "knowledge-graph:read", "gated by knowledge-graph:read, which daily users legitimately hold"},
	{http.MethodGet, "/api/topics/" + corpusEnforceCanaryID, "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodGet, "/api/people/", "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodGet, "/api/people/" + corpusEnforceCanaryID, "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodGet, "/api/places/", "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodGet, "/api/places/" + corpusEnforceCanaryID, "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodGet, "/api/time", "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodGet, "/api/graph/edges", "knowledge-graph:read", "gated by knowledge-graph:read"},
	{http.MethodPost, "/api/capture", "", "write path; gating it breaks capture for every daily user"},
	{http.MethodPost, "/api/bookmarks/import", "", "write path"},
	{http.MethodGet, "/api/health", "", "unauthenticated by design; carries no corpus data"},
	{http.MethodGet, "/readyz", "", "unauthenticated by design; carries no corpus data"},
	{http.MethodGet, "/metrics", "", "unauthenticated by design; carries no corpus data"},
}

// corpusEnforceAllRoutes merges the two tiers into the full sixteen-group
// table.
func corpusEnforceAllRoutes() map[metrics.CorpusRouteGroup][]corpusEnforceRoute {
	all := make(map[metrics.CorpusRouteGroup][]corpusEnforceRoute, len(corpusEnforceTierA)+len(corpusEnforceTierB))
	for g, r := range corpusEnforceTierA {
		all[g] = r
	}
	for g, r := range corpusEnforceTierB {
		all[g] = r
	}
	return all
}

// corpusEnforceClosedSet asserts the transcribed table covers EXACTLY the
// exported closed set, then returns that set in its canonical Tier A → Tier B
// order so every sweep below iterates production's list rather than the table's
// map order.
//
// This is the anti-drift anchor for the whole file: the table is hand-written,
// but WHICH groups must appear in it is derived from `metrics.CorpusRouteGroups()`
// — production code, not test data. A seventeenth group, a renamed group, or a
// quietly deleted table row fails here.
func corpusEnforceClosedSet(t *testing.T, table map[metrics.CorpusRouteGroup][]corpusEnforceRoute) []metrics.CorpusRouteGroup {
	t.Helper()

	canonical := metrics.CorpusRouteGroups()
	if len(canonical) != 16 {
		t.Fatalf("metrics.CorpusRouteGroups() returned %d groups, want 16 (Tier A + Tier B, spec.md §4.2): %v", len(canonical), canonical)
	}

	var missing []string
	for _, g := range canonical {
		if len(table[g]) == 0 {
			missing = append(missing, string(g))
		}
	}
	var extra []string
	for g := range table {
		if !slices.Contains(canonical, g) {
			extra = append(extra, string(g))
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("corpus route table drifted from the closed set: missing=%v extra=%v. Every group in metrics.CorpusRouteGroups() needs at least one probe route here, and nothing outside it may appear.",
			missing, extra)
	}
	if len(table) != 16 {
		t.Fatalf("corpus route table covers %d groups, want exactly 16", len(table))
	}
	return canonical
}

// newCorpusEnforceRouter builds the REAL production router with per-user PASETO
// validation ACTIVE, at the requested corpus-grant stage.
//
// It differs from `newCorpusAuthRouter` in exactly the two fields the ENFORCE
// claim needs, and takes the stage as a parameter so one builder serves both
// the enforcement sweep and the rollback test:
//
//   - CorpusGrantEnforce — the ONLY input that selects the stage. `router.go`
//     mounts `auth.RequireScope(auth.GrantGlobalCorpusRead)` on the corpus
//     group if and only if this is true; the OBSERVE half is mounted either
//     way. That this boolean is the sole difference is what makes the rollback
//     test below a genuine no-rebuild claim.
//   - IntelligenceEngine — REAL, over the live pool, so the eight Tier B groups
//     register. Nil here would make every Tier B assertion vacuous.
func newCorpusEnforceRouter(
	t *testing.T,
	pool *pgxpool.Pool,
	codec *graphapi.CursorCodec,
	limits graphapi.Limits,
	graphCap *graphapi.GraphCapability,
	activePublicHex string,
	enforce bool,
) http.Handler {
	t.Helper()
	return api.NewRouter(&api.Dependencies{
		Environment: "production",
		AuthToken:   corpusOperatorToken,
		AuthConfig: config.AuthConfig{
			Enabled:                              true,
			TokenFormat:                          "paseto-v4-public",
			ProductionSharedTokenFallbackEnabled: true,
		},
		AuthVerifyOptions: auth.VerifyOptions{
			ActivePublicKey:    activePublicHex,
			ActiveKeyID:        corpusKeyID,
			Issuer:             corpusIssuer,
			ClockSkewTolerance: 30 * time.Second,
			Now:                time.Now,
		},
		RevocationCache: revocation.NewCache(),

		// Spec 108 Scope 03 — the stage under test.
		CorpusGrantEnforce: enforce,

		// Non-vacuity: a real engine over the live pool registers Tier B.
		IntelligenceEngine: intelligence.NewEngine(pool, nil),
		ContextHandler:     &api.ContextHandler{},

		// The adjacent knowledge-graph surface, so the over-reach probe runs
		// against real routes behind their own real scope gate.
		GraphCapability: graphCap,
		TopicsHandlers:  graphapi.NewTopicsHandlers(pool, limits, codec),
		PeopleHandlers:  graphapi.NewPeopleHandlers(pool, limits, codec),
		PlacesHandlers:  graphapi.NewPlacesHandlers(pool, limits, codec),
		TimeHandlers:    graphapi.NewTimeHandlers(pool, limits),
		EdgesHandlers:   graphapi.NewEdgesHandlers(pool, limits, codec),
	})
}

// corpusDo drives one bearer against one concrete route on the real router.
func corpusDo(t *testing.T, base, bearer string, rt corpusEnforceRoute) (*http.Response, []byte) {
	t.Helper()
	var payload io.Reader
	if rt.body != "" {
		payload = bytes.NewBufferString(rt.body)
	}
	req, err := http.NewRequest(rt.method, base+rt.path, payload)
	if err != nil {
		t.Fatalf("NewRequest(%s %s): %v", rt.method, rt.path, err)
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	if rt.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", rt.method, rt.path, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body for %s %s: %v", rt.method, rt.path, err)
	}
	return resp, body
}

// corpusScopeDenial is the canonical `auth.RequireScope` refusal envelope.
type corpusScopeDenial struct {
	Error    string   `json:"error"`
	Required []string `json:"required"`
}

// isCorpusGateDenial reports whether a response is THE corpus gate's refusal,
// discriminated from every adjacent scope gate by the scope its body names.
// A bare 403 is not enough: `/api/topics/` also answers 403 to this same
// principal, and conflating the two would let a corpus gate that had been
// deleted "pass" on the strength of the knowledge-graph gate next door.
func isCorpusGateDenial(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusForbidden {
		return false
	}
	var d corpusScopeDenial
	if err := json.Unmarshal(body, &d); err != nil {
		return false
	}
	return d.Error == "scope_required" && slices.Contains(d.Required, auth.GrantGlobalCorpusRead)
}

// corpusIsChiNotFound reports chi's plain-text miss, which is how an UNMOUNTED
// route answers. Every "must still be reachable" assertion checks this so a
// route that silently stopped registering cannot pass by never being there.
func corpusIsChiNotFound(resp *http.Response, body []byte) bool {
	return resp.StatusCode == http.StatusNotFound && strings.HasPrefix(string(body), "404 page not found")
}

// assertCorpusDenialIsClean checks the refusal envelope beyond its status: it
// must be the typed JSON shape, must not echo any canary the caller planted in
// the request, and must not disclose corpus size or existence.
func assertCorpusDenialIsClean(t *testing.T, label string, resp *http.Response, body []byte) {
	t.Helper()
	assertDenialLeaksNothing(t, label, resp, body, []string{corpusEnforceCanaryQuery, corpusEnforceCanaryID})
}

// assertNotRefusedByCorpusGate is the POSITIVE arm. It asserts admission past
// the gate, which is the only thing the gate is responsible for — what the
// handler then answers (200, 400, 500, 503) is the handler's own business and
// depends on collaborators this test deliberately does not fake.
//
// Three separate ways an admission failure could hide are closed:
//   - the corpus denial shape itself,
//   - any 403 at all (no other 403 source exists inside the corpus group, so
//     one appearing would mean a second gate was introduced unnoticed),
//   - 401, which would mean the bearer stopped authenticating and the whole
//     comparison had become meaningless.
func assertNotRefusedByCorpusGate(t *testing.T, label string, resp *http.Response, body []byte) {
	t.Helper()
	if isCorpusGateDenial(resp, body) {
		t.Fatalf("%s: the corpus gate REFUSED a principal holding %s. The grant is not being honored; ENFORCE is denying everyone, which is indistinguishable from a broken stack. body=%s",
			label, auth.GrantGlobalCorpusRead, string(body))
	}
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("%s: status=403 from something other than the corpus gate. No other 403 source exists inside the corpus route group, so this means a second gate was introduced. body=%s",
			label, string(body))
	}
	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatalf("%s: status=401 — the bearer stopped authenticating, so this run proves nothing about the grant. body=%s",
			label, string(body))
	}
	if corpusIsChiNotFound(resp, body) {
		t.Fatalf("%s: route is NOT MOUNTED (chi 404). A missing route would pass an admission check vacuously.", label)
	}
}

// corpusWouldDenyCount reads the OBSERVE half's would-deny counter for one
// (route group, principal) pair.
func corpusWouldDenyCount(group metrics.CorpusRouteGroup, userID string) float64 {
	return testutil.ToFloat64(metrics.AuthCorpusGrantWouldDeny.WithLabelValues(
		string(group), userID, string(auth.SessionSourcePerUserToken)))
}

// corpusEnforceStack brings up one live server at one stage and returns its
// base URL. Everything except the stage is identical between calls.
type corpusEnforceStack struct {
	pool      *pgxpool.Pool
	codec     *graphapi.CursorCodec
	limits    graphapi.Limits
	graphCap  *graphapi.GraphCapability
	publicHex string
}

func (s corpusEnforceStack) serve(t *testing.T, enforce bool) string {
	t.Helper()
	srv := httptest.NewServer(newCorpusEnforceRouter(t, s.pool, s.codec, s.limits, s.graphCap, s.publicHex, enforce))
	t.Cleanup(srv.Close)
	return srv.URL
}

// newCorpusEnforceStack opens the live pool and resolves the graph wiring
// shared by every test in this file.
func newCorpusEnforceStack(t *testing.T, envSuffix string) (corpusEnforceStack, string) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("integration: DATABASE_URL not set — live stack not available")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	privateHex, publicHex := auth.GenerateSigningKeypair()
	graphCap, codec, limits := enabledGraphWiring(t, envSuffix)
	return corpusEnforceStack{
		pool:      pool,
		codec:     codec,
		limits:    limits,
		graphCap:  graphCap,
		publicHex: publicHex,
	}, privateHex
}

// TestIntegration_CorpusGrantEnforce_RefusesUngrantedAndServesGrantedOnAllSixteenGroups
// is TP-03-02 (SCN-108-G01) plus the allow half of TP-03-11.
//
// One sweep, both arms, every group. For each of the sixteen route groups:
//
//	ungranted → MUST be the corpus gate's 403, with a clean envelope.
//	granted   → MUST NOT be refused by the corpus gate.
//	operator  → MUST NOT be refused (RequireScope's documented shared-token
//	            bypass; asserted here so a change that broke operator access
//	            while tightening the gate cannot pass unnoticed).
//
// The two principals differ ONLY in their scope claim — same signing key, same
// issuer, same TTL, same middleware chain, same route, same request bytes. So
// any divergence in outcome is attributable to the grant and to nothing else.
func TestIntegration_CorpusGrantEnforce_RefusesUngrantedAndServesGrantedOnAllSixteenGroups(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSENFORCE")

	table := corpusEnforceAllRoutes()
	groups := corpusEnforceClosedSet(t, table)

	ungrantedBearer := mintCorpusToken(t, privateHex, "tp0302-ungranted", []string{corpusOtherScope})
	grantedBearer := mintCorpusToken(t, privateHex, "tp0302-granted", []string{auth.GrantGlobalCorpusRead})

	base := stack.serve(t, true)

	exercisedGroups := 0
	exercisedRoutes := 0
	refusals := 0
	admissions := 0

	for _, group := range groups {
		routes := table[group]
		if len(routes) == 0 {
			t.Fatalf("route_group=%q has no probe route; the closed-set check should have caught this", group)
		}
		exercisedGroups++

		for _, rt := range routes {
			exercisedRoutes++
			label := string(group) + " " + rt.method + " " + rt.path

			// ---- NEGATIVE ARM: no grant, must be refused ----
			resp, body := corpusDo(t, base, ungrantedBearer, rt)
			if !isCorpusGateDenial(resp, body) {
				t.Fatalf("ENFORCE/%s: ungranted principal was NOT refused by the corpus gate (status=%d). Under ENFORCE every one of the sixteen groups must answer 403 scope_required naming %s. body=%s",
					label, resp.StatusCode, auth.GrantGlobalCorpusRead, string(body))
			}
			assertCorpusDenialIsClean(t, "ENFORCE/"+label, resp, body)
			refusals++

			// ---- POSITIVE ARM: same route, same bytes, grant present ----
			gResp, gBody := corpusDo(t, base, grantedBearer, rt)
			assertNotRefusedByCorpusGate(t, "ENFORCE/granted/"+label, gResp, gBody)
			admissions++

			// ---- Operator bypass (RequireScope source switch) ----
			oResp, oBody := corpusDo(t, base, corpusOperatorToken, rt)
			assertNotRefusedByCorpusGate(t, "ENFORCE/operator/"+label, oResp, oBody)
		}
	}

	if exercisedGroups != 16 {
		t.Fatalf("exercised %d route groups, want exactly 16 — the sweep shrank", exercisedGroups)
	}
	if refusals != exercisedRoutes || admissions != exercisedRoutes {
		t.Fatalf("arm accounting mismatch: routes=%d refusals=%d admissions=%d; every route must contribute BOTH arms",
			exercisedRoutes, refusals, admissions)
	}
	t.Logf("TP-03-02: ENFORCE sweep complete — groups=%d routes=%d refusals=%d admissions=%d",
		exercisedGroups, exercisedRoutes, refusals, admissions)
}

// TestIntegration_CorpusGrantEnforce_DoesNotOverReachIntoUngatedRoutes proves
// the other half of correctness: the gate must not be too WIDE.
//
// Every assertion in the sweep above would still pass if `RequireScope` had
// been mounted on the entire `/api` surface instead of the corpus group. This
// test is what separates those two worlds. Each §2 route is probed under
// ENFORCE with the SAME ungranted principal that is refused everywhere in the
// corpus group, and must be reachable — and where the route is gated by a
// different scope, the denial must name THAT scope, which is simultaneously the
// mount proof and the discrimination proof.
func TestIntegration_CorpusGrantEnforce_DoesNotOverReachIntoUngatedRoutes(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSOVERREACH")
	ungrantedBearer := mintCorpusToken(t, privateHex, "tp0302-overreach", []string{corpusOtherScope})
	base := stack.serve(t, true)

	checked := 0
	for _, u := range corpusEnforceUngated {
		rt := corpusEnforceRoute{method: u.method, path: u.path}
		if u.method == http.MethodPost {
			rt.body = `{}`
		}
		resp, body := corpusDo(t, base, ungrantedBearer, rt)
		label := u.method + " " + u.path

		if corpusIsChiNotFound(resp, body) {
			t.Fatalf("over-reach probe %s is NOT MOUNTED (chi 404). An absent route would pass this test vacuously; fix the probe or the wiring. (%s)", label, u.why)
		}
		if isCorpusGateDenial(resp, body) {
			t.Fatalf("GATE OVER-REACH: %s is refused by the corpus gate under ENFORCE, but design.md §2 lists it as deliberately NOT gated (%s). body=%s",
				label, u.why, string(body))
		}

		if u.otherScope != "" {
			var d corpusScopeDenial
			if resp.StatusCode != http.StatusForbidden || json.Unmarshal(body, &d) != nil || !slices.Contains(d.Required, u.otherScope) {
				t.Fatalf("%s: expected the ADJACENT %s gate to refuse this principal (status=403, required=[%s]); got status=%d body=%s. If this route stopped being scope-gated, the over-reach probe no longer proves anything.",
					label, u.otherScope, u.otherScope, resp.StatusCode, string(body))
			}
		}
		checked++
	}

	if checked != len(corpusEnforceUngated) {
		t.Fatalf("checked %d ungated routes, want %d", checked, len(corpusEnforceUngated))
	}
	t.Logf("TP-03-02 over-reach: %d deliberately-ungated routes remain reachable under ENFORCE", checked)
}

// TestIntegration_CorpusGrantEnforce_TierBDeniesWithTheTierADenialShape is
// TP-03-11 (SCN-108-G04).
//
// The eight Tier B Phase-5 groups must refuse an ungranted principal, AND must
// refuse with the SAME body as Tier A. A Tier B refusal that differed — extra
// field, different code, different required list — would let a caller tell
// which TIER a route belongs to purely from a denial it was never entitled to
// learn anything from, and would signal that Tier B had acquired a second,
// parallel authorization path instead of sharing the one gate.
//
// The comparison is on raw BYTES, not on a decoded shape: `writeScopeError`
// marshals a two-key map, and Go sorts map keys, so byte equality is both
// deterministic and the strictest available statement of "same shape".
func TestIntegration_CorpusGrantEnforce_TierBDeniesWithTheTierADenialShape(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSTIERB")
	ungrantedBearer := mintCorpusToken(t, privateHex, "tp0311-ungranted", []string{corpusOtherScope})
	grantedBearer := mintCorpusToken(t, privateHex, "tp0311-granted", []string{auth.GrantGlobalCorpusRead})
	base := stack.serve(t, true)

	// Tier A reference denial. Taken from a GET and cross-checked against a
	// POST so the reference is not accidentally method-specific.
	refRoute := corpusEnforceTierA[metrics.CorpusRouteGroupRecent][0]
	refResp, refBody := corpusDo(t, base, ungrantedBearer, refRoute)
	if !isCorpusGateDenial(refResp, refBody) {
		t.Fatalf("Tier A reference route %s did not produce a corpus denial (status=%d); there is no baseline to compare Tier B against. body=%s",
			refRoute.path, refResp.StatusCode, string(refBody))
	}
	assertCorpusDenialIsClean(t, "TierA/reference", refResp, refBody)
	refContentType := refResp.Header.Get("Content-Type")

	postRoute := corpusEnforceTierA[metrics.CorpusRouteGroupSearch][0]
	postResp, postBody := corpusDo(t, base, ungrantedBearer, postRoute)
	if !bytes.Equal(refBody, postBody) || postResp.StatusCode != refResp.StatusCode {
		t.Fatalf("Tier A denial is method-dependent (GET %s vs POST %s): %q vs %q. The reference shape must be method-independent before Tier B is compared to it.",
			refRoute.path, postRoute.path, string(refBody), string(postBody))
	}

	tierBGroups := 0
	for _, group := range metrics.CorpusRouteGroups() {
		routes, ok := corpusEnforceTierB[group]
		if !ok {
			continue
		}
		tierBGroups++

		for _, rt := range routes {
			label := "TierB/" + string(group) + " " + rt.method + " " + rt.path

			// Deny half — this is ALSO the mount proof for Tier B: an
			// unregistered route (nil IntelligenceEngine) 404s and fails here.
			resp, body := corpusDo(t, base, ungrantedBearer, rt)
			if !isCorpusGateDenial(resp, body) {
				t.Fatalf("%s: ungranted principal was NOT refused (status=%d). Either Tier B is ungated, or the eight Phase-5 routes never registered — in which case every Tier B assertion in this file would be vacuous. body=%s",
					label, resp.StatusCode, string(body))
			}
			assertCorpusDenialIsClean(t, label, resp, body)

			// Same-shape half.
			if resp.StatusCode != refResp.StatusCode {
				t.Fatalf("%s: denial status=%d, Tier A reference=%d — a caller could tell the tiers apart from the refusal alone",
					label, resp.StatusCode, refResp.StatusCode)
			}
			if !bytes.Equal(body, refBody) {
				t.Fatalf("%s: Tier B denial body differs from Tier A.\n  TierB: %s\n  TierA: %s\nThe tier split is documentation, not a difference in authority; the refusal must not leak which tier a route belongs to.",
					label, string(body), string(refBody))
			}
			if ct := resp.Header.Get("Content-Type"); ct != refContentType {
				t.Fatalf("%s: denial Content-Type=%q, Tier A reference=%q", label, ct, refContentType)
			}

			// Allow half — same route, grant present.
			gResp, gBody := corpusDo(t, base, grantedBearer, rt)
			assertNotRefusedByCorpusGate(t, "granted/"+label, gResp, gBody)
		}
	}

	if tierBGroups != 8 {
		t.Fatalf("exercised %d Tier B groups, want exactly 8 (spec.md §18 decision 5, groups 9-16)", tierBGroups)
	}
	t.Logf("TP-03-11: %d Tier B groups deny with the byte-identical Tier A envelope %q", tierBGroups, string(refBody))
}

// TestIntegration_CorpusGrantEnforce_RollbackToObserveRestoresAccess is
// TP-03-05 (SCN-108-C04).
//
// SCOPE OF THE CLAIM, stated plainly. This proves the stage flip AT THE ROUTER
// BOUNDARY, which is where the stage enters the system: `CorpusGrantEnforce` is
// an already-resolved boolean input to `api.NewRouter`. It does NOT drive a
// `cmd/core` process restart; the Test Plan row names such a harness as a
// component and that half is not exercised here.
//
// What it proves instead is arguably the stronger form of "no rebuild": both
// stages are served by ONE running test binary, from ONE compiled router
// constructor, with byte-identical dependencies. The only differing input in
// the entire construction is the boolean. Nothing is recompiled, no image is
// built, no source is edited between the two observations.
//
// Four things are asserted together:
//
//	ENFORCE            → the principal is refused on all sixteen groups.
//	OBSERVE            → the SAME principal, SAME routes, is served again.
//	counting resumes   → the would-deny counter, which cannot move under
//	                     ENFORCE (RequireScope short-circuits ahead of the
//	                     OBSERVE gate), moves again under OBSERVE.
//	symmetric/idempotent → flipping back to ENFORCE refuses again, and a
//	                     second independently-constructed ENFORCE router
//	                     behaves identically to the first.
//
// The counter assertion is what makes this test impossible to satisfy by
// accident: it is a positive, stage-specific side effect, not merely the
// absence of a 403.
func TestIntegration_CorpusGrantEnforce_RollbackToObserveRestoresAccess(t *testing.T) {
	stack, privateHex := newCorpusEnforceStack(t, "CORPUSROLLBACK")

	table := corpusEnforceAllRoutes()
	groups := corpusEnforceClosedSet(t, table)

	const principal = "tp0305-rollback-ungranted"
	ungrantedBearer := mintCorpusToken(t, privateHex, principal, []string{corpusOtherScope})

	// Same builder, same deps, one boolean apart. No rebuild between them.
	enforceBase := stack.serve(t, true)
	observeBase := stack.serve(t, false)

	refusedUnderEnforce := 0
	servedUnderObserve := 0
	countingResumed := 0

	for _, group := range groups {
		rt := table[group][0]
		label := string(group) + " " + rt.method + " " + rt.path

		// ---- ENFORCE: refused, and the OBSERVE counter cannot move ----
		before := corpusWouldDenyCount(group, principal)
		resp, body := corpusDo(t, enforceBase, ungrantedBearer, rt)
		if !isCorpusGateDenial(resp, body) {
			t.Fatalf("ENFORCE/%s: principal was not refused (status=%d); there is nothing to roll back FROM. body=%s",
				label, resp.StatusCode, string(body))
		}
		refusedUnderEnforce++
		duringEnforce := corpusWouldDenyCount(group, principal)
		if duringEnforce != before {
			t.Fatalf("ENFORCE/%s: would-deny counter moved %v→%v. Under ENFORCE, RequireScope refuses OUTSIDE the observe gate, so the counterfactual counter must stay still — a moving counter here means the middleware order inverted.",
				label, before, duringEnforce)
		}

		// ---- OBSERVE: the SAME principal on the SAME route is served ----
		oResp, oBody := corpusDo(t, observeBase, ungrantedBearer, rt)
		if isCorpusGateDenial(oResp, oBody) {
			t.Fatalf("OBSERVE/%s: principal is STILL refused after rolling the stage back. Rollback did not restore access; the flag is not the only thing selecting the stage. body=%s",
				label, string(oBody))
		}
		assertNotRefusedByCorpusGate(t, "OBSERVE/"+label, oResp, oBody)
		servedUnderObserve++

		// ---- counting resumes ----
		afterObserve := corpusWouldDenyCount(group, principal)
		if afterObserve != duringEnforce+1 {
			t.Fatalf("OBSERVE/%s: would-deny counter %v→%v, want exactly +1. OBSERVE must resume recording the counterfactual for a principal it now admits; a flat counter means the observe half stopped emitting.",
				label, duringEnforce, afterObserve)
		}
		countingResumed++

		// ---- symmetric: flipping forward refuses again ----
		reResp, reBody := corpusDo(t, enforceBase, ungrantedBearer, rt)
		if !isCorpusGateDenial(reResp, reBody) {
			t.Fatalf("ENFORCE-again/%s: the stage flip is not symmetric — the principal was admitted after a round trip through OBSERVE. body=%s",
				label, string(reBody))
		}
	}

	if refusedUnderEnforce != 16 || servedUnderObserve != 16 || countingResumed != 16 {
		t.Fatalf("rollback accounting: refused=%d served=%d counted=%d; each must be 16",
			refusedUnderEnforce, servedUnderObserve, countingResumed)
	}

	// ---- idempotent: a second, independently constructed ENFORCE router
	// behaves identically to the first, on the same principal and route.
	secondEnforceBase := stack.serve(t, true)
	probeGroup := groups[0]
	probeRoute := table[probeGroup][0]
	aResp, aBody := corpusDo(t, enforceBase, ungrantedBearer, probeRoute)
	bResp, bBody := corpusDo(t, secondEnforceBase, ungrantedBearer, probeRoute)
	if !isCorpusGateDenial(bResp, bBody) {
		t.Fatalf("second ENFORCE construction did not refuse (status=%d) — the stage is not idempotent across constructions. body=%s",
			bResp.StatusCode, string(bBody))
	}
	if aResp.StatusCode != bResp.StatusCode || !bytes.Equal(aBody, bBody) {
		t.Fatalf("two ENFORCE constructions disagree on %s: %d/%s vs %d/%s",
			probeRoute.path, aResp.StatusCode, string(aBody), bResp.StatusCode, string(bBody))
	}

	t.Logf("TP-03-05: ENFORCE→OBSERVE rollback restored access on %d/16 groups in one process, no rebuild; would-deny counting resumed on all 16; flip is symmetric and idempotent",
		servedUnderObserve)
}
