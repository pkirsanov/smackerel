//go:build integration

// BUG-080-001 — T080-02-MANIFEST (SCN-080-001-02). Atomic route-manifest
// registration proof over the REAL production router (internal/api.NewRouter)
// and the disposable stack's REAL PostgreSQL (DATABASE_URL). NO request
// interception, NO mock, NO stub.
//
// SCN-080-001-02 has two clauses, both proven here:
//
//  1. "all eight required family routes register as ONE authenticated group
//     behind bearer + knowledge-graph:read" — proven two ways: (a) chi.Walk of
//     the real ENABLED router shows its mounted graph routes are EXACTLY the
//     eight canonical manifest entries (no missing, no extra — atomic full
//     mount, never a subset); (b) every one of the eight family request paths
//     is rejected 401 when unauthenticated (they sit behind the shared bearer
//     group) and is served by a live PostgreSQL-backed graph handler when
//     authenticated.
//
//  2. "removing OR duplicating any manifest entry rejects construction rather
//     than mounting a subset" — proven as an adversarial red→green against the
//     manifest validator: the canonical eight-entry manifest validates; every
//     one-entry removal and every one-entry duplication is REJECTED with the
//     typed, value-safe F080-ROUTE-MANIFEST-INCOMPLETE error. This is the
//     contract the OLD hardcoded-route wiring lacked — a dropped route silently
//     mounted a seven-route subset (the silent-absence class this bug fix
//     eliminates); the new manifest registrar fails loud instead.
//
// The DISABLED router is also walked to prove the fail-soft state mounts the
// SAME complete eight-route manifest (present-but-503), never a subset.

package graphapi_integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/api/graphapi"
)

// manifestFamilyRequestPaths are the eight family REQUEST forms (list roots
// without a trailing slash, detail routes with an id, plus /api/time and
// /api/graph/edges) exercised against the live router. They correspond 1:1 to
// the eight canonical manifest PathTemplate entries.
var manifestFamilyRequestPaths = []string{
	"/api/topics",
	"/api/topics/bug080-manifest-nonexistent",
	"/api/people",
	"/api/people/bug080-manifest-nonexistent",
	"/api/places",
	"/api/places/bug080-manifest-nonexistent",
	"/api/time",
	"/api/graph/edges",
}

// isGraphFamilyRoute reports whether a chi route pattern belongs to one of the
// eight graph families (used to filter chi.Walk output down to the graph group).
func isGraphFamilyRoute(route string) bool {
	switch {
	case route == "/api/time", route == "/api/graph/edges":
		return true
	case strings.HasPrefix(route, "/api/topics"),
		strings.HasPrefix(route, "/api/people"),
		strings.HasPrefix(route, "/api/places"):
		return true
	}
	return false
}

// walkGraphRoutes enumerates the "METHOD PATTERN" of every graph-family route
// the real router actually mounts, via chi.Walk over the production route tree.
func walkGraphRoutes(t *testing.T, router http.Handler) map[string]bool {
	t.Helper()
	routes, ok := router.(chi.Routes)
	if !ok {
		t.Fatalf("router is not chi.Routes; cannot walk the mounted route tree")
	}
	mounted := map[string]bool{}
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if isGraphFamilyRoute(route) {
			mounted[method+" "+route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("chi.Walk: %v", err)
	}
	return mounted
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestGraphRouteManifestRegistersAllFamiliesAtomically proves SCN-080-001-02:
// the eight graph families register as one authenticated group and any missing
// or duplicated manifest entry rejects construction rather than mounting a
// subset.
func TestGraphRouteManifestRegistersAllFamiliesAtomically(t *testing.T) {
	canonical := graphapi.CanonicalGraphRouteManifest()

	// --- Clause 2 prerequisite: the canonical manifest is complete. ---
	t.Run("canonical_manifest_is_complete_and_router_validates", func(t *testing.T) {
		if got := len(canonical); got != 8 {
			t.Fatalf("canonical manifest has %d entries; want exactly 8 families", got)
		}
		if err := graphapi.ValidateGraphRouteManifest(canonical); err != nil {
			t.Fatalf("canonical manifest MUST validate, got: %v", err)
		}
		// The router calls this at construction; it must not panic on the
		// canonical (complete) manifest.
		graphapi.MustValidateGraphRouteManifest()
		for _, e := range canonical {
			t.Logf("manifest entry: %-6s %-20s family=%s scope=%s", e.Method, e.PathTemplate, e.Family, e.Scope)
		}
	})

	// --- Clause 2 (adversarial red→green): removing any entry is REJECTED. ---
	// OLD hardcoded wiring: dropping a route silently mounted a 7-route subset.
	// NEW manifest registrar: a 7-entry manifest fails construction (F080).
	t.Run("removing_any_manifest_entry_rejects_construction", func(t *testing.T) {
		for i := range canonical {
			removed := canonical[i]
			subset := make([]graphapi.GraphRouteEntry, 0, len(canonical)-1)
			subset = append(subset, canonical[:i]...)
			subset = append(subset, canonical[i+1:]...)

			err := graphapi.ValidateGraphRouteManifest(subset)
			if err == nil {
				t.Errorf("RED-EXPECTED-FAIL: removing family %q left a %d-route subset that validated; construction MUST be rejected, never mount a subset",
					removed.Family, len(subset))
				continue
			}
			if !strings.Contains(err.Error(), graphapi.CodeRouteManifestIncomplete) {
				t.Errorf("removing family %q: error %q must carry code %s", removed.Family, err, graphapi.CodeRouteManifestIncomplete)
			}
			if !strings.Contains(err.Error(), string(removed.Family)) {
				t.Errorf("removing family %q: error %q must name the absent family", removed.Family, err)
			}
			t.Logf("remove %-12s -> REJECTED: %v", removed.Family, err)
		}
	})

	// --- Clause 2 (adversarial red→green): duplicating any entry is REJECTED. ---
	t.Run("duplicating_any_manifest_entry_rejects_construction", func(t *testing.T) {
		for i := range canonical {
			dup := canonical[i]
			withDup := make([]graphapi.GraphRouteEntry, 0, len(canonical)+1)
			withDup = append(withDup, canonical...)
			withDup = append(withDup, dup)

			err := graphapi.ValidateGraphRouteManifest(withDup)
			if err == nil {
				t.Errorf("RED-EXPECTED-FAIL: duplicating family %q validated; a duplicated family MUST reject construction", dup.Family)
				continue
			}
			if !strings.Contains(err.Error(), graphapi.CodeRouteManifestIncomplete) {
				t.Errorf("duplicating family %q: error %q must carry code %s", dup.Family, err, graphapi.CodeRouteManifestIncomplete)
			}
			t.Logf("duplicate %-12s -> REJECTED: %v", dup.Family, err)
		}
	})

	// --- Clause 1 (disabled state): the fail-soft router mounts all 8. ---
	// No live PostgreSQL needed: the disabled capability owns no handlers.
	t.Run("disabled_router_mounts_all_eight_present", func(t *testing.T) {
		cfg := graphActivationLimits()
		cfg.CursorSecretEnv = "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_MANIFEST_UNSET_DO_NOT_SET"
		router := newDisabledGraphRouter(t, cfg)

		mounted := walkGraphRoutes(t, router)
		t.Logf("DISABLED router mounted graph routes:\n  %s", strings.Join(sortedKeys(mounted), "\n  "))
		assertMountedEqualsManifest(t, mounted, canonical)
	})

	// --- Clause 1 (enabled state): live PostgreSQL, one authenticated group. ---
	t.Run("enabled_router_mounts_all_eight_as_one_authenticated_group", func(t *testing.T) {
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			t.Skip("integration: DATABASE_URL not set — live stack not available")
		}
		ctx := context.Background()
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			t.Fatalf("pgxpool.New: %v", err)
		}
		t.Cleanup(pool.Close)
		if err := pool.Ping(ctx); err != nil {
			t.Fatalf("ping postgres: %v", err)
		}

		// Present, non-empty cursor secret -> ENABLED activation.
		envName := "KNOWLEDGE_GRAPH_API_CURSOR_SECRET_BUG080IT_MANIFEST"
		t.Setenv(envName, "bug080-001-manifest-cursor-secret-32bytes!!")
		cfg := graphActivationLimits()
		cfg.CursorSecretEnv = envName
		graphCap := graphapi.NewGraphCapability(cfg)
		if graphCap.Disabled() {
			t.Fatalf("test setup: expected ENABLED capability with a present secret")
		}
		secret, err := cfg.LoadCursorSecret()
		if err != nil {
			t.Fatalf("LoadCursorSecret: %v", err)
		}
		codec, err := graphapi.NewCursorCodec(secret)
		if err != nil {
			t.Fatalf("NewCursorCodec: %v", err)
		}
		limits := cfg.Limits()

		// Seed one real topic so the enabled list handler serves a real row,
		// proving the mounted routes are live PostgreSQL-backed, not stubs.
		prefix := "bug080-manifest-" + time.Now().UTC().Format("20060102150405.000000")
		topicID := prefix + "-topic-0"
		if _, err := pool.Exec(ctx,
			`INSERT INTO topics (id, name, capture_count_total, momentum_score) VALUES ($1,$2,$3,$4)`,
			topicID, topicID, 7, float32(1.0)); err != nil {
			t.Fatalf("seed topic: %v", err)
		}
		t.Cleanup(func() {
			cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := pool.Exec(cctx, `DELETE FROM topics WHERE id LIKE $1`, prefix+"-%"); err != nil {
				t.Logf("cleanup topics %s-%%: %v", prefix, err)
			}
		})

		// A NON-empty AuthToken makes the bearer middleware enforce the bearer
		// (Environment="test" is not production, so the empty-token dev bypass
		// does NOT apply when AuthToken is set) — this lets us prove the eight
		// routes sit behind the shared bearer group.
		const sharedToken = "bug080-manifest-shared-token"
		router := api.NewRouter(&api.Dependencies{
			Environment:     "test",
			AuthToken:       sharedToken,
			GraphCapability: graphCap,
			TopicsHandlers:  graphapi.NewTopicsHandlers(pool, limits, codec),
			PeopleHandlers:  graphapi.NewPeopleHandlers(pool, limits, codec),
			PlacesHandlers:  graphapi.NewPlacesHandlers(pool, limits, codec),
			TimeHandlers:    graphapi.NewTimeHandlers(pool, limits),
			EdgesHandlers:   graphapi.NewEdgesHandlers(pool, limits, codec),
		})

		// (a) chi.Walk equivalence: the ENABLED router mounts EXACTLY the eight
		//     canonical manifest routes — atomic full mount, no subset, no extra.
		mounted := walkGraphRoutes(t, router)
		t.Logf("ENABLED router mounted graph routes:\n  %s", strings.Join(sortedKeys(mounted), "\n  "))
		assertMountedEqualsManifest(t, mounted, canonical)

		// (b) one authenticated group: unauthenticated → 401 for every family
		//     (they sit behind the shared bearer group); authenticated → served
		//     by a live graph handler (never 401/403; typed JSON envelope).
		srv := httptest.NewServer(router)
		t.Cleanup(srv.Close)
		client := &http.Client{Timeout: 15 * time.Second}

		for _, p := range manifestFamilyRequestPaths {
			// Unauthenticated.
			req, _ := http.NewRequest(http.MethodGet, srv.URL+p, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("unauthed GET %s: %v", p, err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("unauthed GET %s -> %d; want 401 (family MUST sit behind the shared bearer group)", p, resp.StatusCode)
			} else {
				t.Logf("unauthed GET %-45s -> 401 (behind bearer)", p)
			}

			// Authenticated (shared token bypasses RequireScope; live handler serves).
			areq, _ := http.NewRequest(http.MethodGet, srv.URL+p, nil)
			areq.Header.Set("Authorization", "Bearer "+sharedToken)
			aresp, err := client.Do(areq)
			if err != nil {
				t.Fatalf("authed GET %s: %v", p, err)
			}
			abody, _ := io.ReadAll(aresp.Body)
			_ = aresp.Body.Close()
			ct := aresp.Header.Get("Content-Type")
			if aresp.StatusCode == http.StatusUnauthorized || aresp.StatusCode == http.StatusForbidden {
				t.Errorf("authed GET %s -> %d; the authenticated shared-token caller MUST reach the graph handler", p, aresp.StatusCode)
			}
			if !strings.Contains(ct, "application/json") {
				t.Errorf("authed GET %s -> Content-Type %q; a mounted graph handler MUST emit application/json (a chi route-absent 404 is text/plain)", p, ct)
			} else {
				t.Logf("authed   GET %-45s -> %d %s", p, aresp.StatusCode, ct)
			}
			_ = abody
		}

		// (c) enabled serves REAL PostgreSQL data: the seeded topic appears.
		areq, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/topics?limit=50", nil)
		areq.Header.Set("Authorization", "Bearer "+sharedToken)
		aresp, err := client.Do(areq)
		if err != nil {
			t.Fatalf("authed GET /api/topics: %v", err)
		}
		abody, _ := io.ReadAll(aresp.Body)
		_ = aresp.Body.Close()
		if aresp.StatusCode != http.StatusOK {
			t.Fatalf("authed GET /api/topics -> %d; want 200 (enabled, live PostgreSQL). body=%s", aresp.StatusCode, string(abody))
		}
		if !strings.Contains(string(abody), topicID) {
			t.Fatalf("seeded topic %s missing from the ENABLED list response; the mounted handler did not serve real PostgreSQL rows", topicID)
		}
		t.Logf("authed   GET /api/topics?limit=50           -> 200, seeded topic %s present (live PostgreSQL)", topicID)
	})
}

// assertMountedEqualsManifest proves the router's mounted graph routes are
// EXACTLY the canonical manifest — every entry present (atomic full mount, never
// a subset) and no stray graph route beyond the manifest.
func assertMountedEqualsManifest(t *testing.T, mounted map[string]bool, canonical []graphapi.GraphRouteEntry) {
	t.Helper()
	want := map[string]bool{}
	for _, e := range canonical {
		want[e.Method+" "+e.PathTemplate] = true
	}
	for k := range want {
		if !mounted[k] {
			t.Errorf("manifest route %q is NOT mounted (subset mount); mounted graph routes=%v", k, sortedKeys(mounted))
		}
	}
	for k := range mounted {
		if !want[k] {
			t.Errorf("router mounts graph route %q that is NOT in the canonical manifest (stray route)", k)
		}
	}
	if len(mounted) != len(want) {
		t.Errorf("mounted graph route count=%d, manifest count=%d — the router must mount the manifest exactly", len(mounted), len(want))
	}
}
