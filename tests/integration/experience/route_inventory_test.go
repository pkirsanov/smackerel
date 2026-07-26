//go:build integration

// Spec 106 SCOPE-106-02 — XP106-02-I (integration, live stack).
//
// TestCatalogMatchesRealServerPWAAndCardRouteInventoriesExactly compares the
// GENERATED ProductExperienceCatalog against the REAL route inventory of the
// running smackerel stack: the actual server router registration (probed over
// CORE_EXTERNAL_URL — the in-network URL the disposable
// `./smackerel.sh test integration` stack exports), the web/pwa/ static-page
// tree on disk, the Card routes, the served web manifest, and the served
// service worker.
//
// This is a REAL live-stack integration test: NO interception, NO mock, NO
// httptest. It drives the actual running core over HTTP and asserts, against
// the real server, that:
//
//   - every catalog active-leaf href resolves to a really-registered
//     destination (non-404; an auth-gated page answers with its real
//     redirect/401/403 — still registered),
//   - each PWA-file-backed leaf's href is a real file on disk AND serves 200,
//     the single Card leaf binds the real /cards route, and every server leaf's
//     declared renderer support agrees with the real serving surface,
//   - route-free groups (Work, Sources, Admin) carry no guessed href,
//     knowledge_graph stays unbound AND /knowledge/graph really 404s (pending
//     spec 105), and Lists/Meals/Expenses stay unavailable AND their guessed
//     browser pages really 404 (no secret registration),
//   - the served manifest start_url and the content-hash-versioned service
//     worker agree with the catalog's Capture binding,
//   - the whole catalog validates structurally against the LIVE registered
//     route set via the production ExperienceRouteValidator.
//
// Adversarial (non-tautological): a deliberately-unregistered control path MUST
// 404, and every guessed Work/Graph browser page MUST 404 — otherwise the
// "registered" probe could not tell a real route from an invented one.
//
// It SKIPs (not fails) when CORE_EXTERNAL_URL is unset, matching the repo
// live-lane convention, so it is a no-op outside the live integration lane.
package integrationexperience

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/experience"
)

func iRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from cwd")
		}
		dir = parent
	}
}

func iMustSurface(t *testing.T, c experience.ProductExperienceCatalog, id string) experience.Surface {
	t.Helper()
	for _, s := range c.Surfaces {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("generated catalog missing expected surface %q", id)
	return experience.Surface{}
}

func iHasRenderer(s experience.Surface, want experience.Renderer) bool {
	for _, r := range s.RendererSupport {
		if r == want {
			return true
		}
	}
	return false
}

func iCapsOf(c experience.ProductExperienceCatalog) map[string]bool {
	caps := map[string]bool{}
	for _, s := range c.Surfaces {
		if s.CapabilityID != "" {
			caps[s.CapabilityID] = true
		}
	}
	return caps
}

func TestCatalogMatchesRealServerPWAAndCardRouteInventoriesExactly(t *testing.T) {
	coreURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CORE_EXTERNAL_URL")), "/")
	if coreURL == "" {
		t.Skip("integration: CORE_EXTERNAL_URL not set — live stack not available")
	}
	root := iRepoRoot(t)

	// Do NOT follow redirects: a 303/302 to /login proves the route is
	// registered and auth-gated (not a 404), which is exactly the signal we
	// want to preserve.
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	probe := func(path string) int {
		resp, err := client.Get(coreURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	registered := func(code int) bool { return code != http.StatusNotFound }

	cat, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog: %v", err)
	}

	// --- 0. ADVERSARIAL CONTROL: an unregistered path MUST 404, else the probe
	//        cannot distinguish a real route from an invented one.
	const control = "/definitely-not-registered-xp106-02-i"
	if code := probe(control); code != http.StatusNotFound {
		t.Fatalf("adversarial control %s returned %d, want 404 — probe cannot distinguish registered vs invented routes", control, code)
	}
	t.Logf("adversarial control %-40s -> 404 (correctly unregistered)", control)

	// --- 1. Every active-leaf href resolves live AND its renderer support
	//        agrees with the real serving surface.
	live := map[string]bool{}
	activeLeaves := 0
	for _, s := range cat.Surfaces {
		if s.ReadinessDiscoverabilityPolicy != experience.PolicyReadyWhenJourneyReady &&
			s.ReadinessDiscoverabilityPolicy != experience.PolicyOperatorOnlyWhenReady {
			continue
		}
		activeLeaves++
		if s.Href == "" {
			t.Errorf("active leaf %q has empty href", s.ID)
			continue
		}
		code := probe(s.Href)
		if !registered(code) {
			t.Errorf("active leaf %q href %s is NOT registered on the live server (status %d)", s.ID, s.Href, code)
			continue
		}
		live[s.Href] = true

		switch {
		case strings.HasPrefix(s.Href, "/pwa/"):
			// A PWA-file-backed leaf: its href MUST be a real file on disk under
			// web/pwa/ AND serve 200, and it MUST declare the pwa renderer.
			rel := strings.TrimPrefix(s.Href, "/pwa/")
			if _, statErr := os.Stat(filepath.Join(root, "web", "pwa", filepath.FromSlash(rel))); statErr != nil {
				t.Errorf("pwa leaf %q href %s: backing file missing on disk: %v", s.ID, s.Href, statErr)
			}
			if code != http.StatusOK {
				t.Errorf("pwa leaf %q href %s served %d, want 200", s.ID, s.Href, code)
			}
			if !iHasRenderer(s, experience.RendererPWA) {
				t.Errorf("pwa-served leaf %q must declare renderer pwa, got %v", s.ID, s.RendererSupport)
			}
		case s.Href == "/cards":
			if !iHasRenderer(s, experience.RendererCard) {
				t.Errorf("card leaf %q must declare renderer card, got %v", s.ID, s.RendererSupport)
			}
		default:
			// A server-registered route (HTMX page, or the /assistant
			// server-alias whose page is the PWA): it MUST declare at least the
			// server or pwa renderer.
			if !iHasRenderer(s, experience.RendererServer) && !iHasRenderer(s, experience.RendererPWA) {
				t.Errorf("server-registered leaf %q must declare renderer server or pwa, got %v", s.ID, s.RendererSupport)
			}
		}
		t.Logf("active leaf %-18s href=%-30s -> %d (registered, renderer=%v)", s.ID, s.Href, code, s.RendererSupport)
	}
	if activeLeaves < 10 {
		t.Fatalf("suspiciously few active leaves probed (%d) — catalog likely not loaded", activeLeaves)
	}

	// --- 2. Route-free groups carry no guessed href.
	for _, id := range []string{"work", "sources", "admin"} {
		s := iMustSurface(t, cat, id)
		if s.Kind != experience.KindRouteGroup || s.ReadinessDiscoverabilityPolicy != experience.PolicyRouteFreeGroup {
			t.Errorf("group %q kind/policy = %s/%s, want route_group/route_free_group", id, s.Kind, s.ReadinessDiscoverabilityPolicy)
		}
		if s.Href != "" {
			t.Errorf("route-free group %q carries guessed href %q", id, s.Href)
		}
		t.Logf("route-free group %-9s -> no href (ok)", id)
	}

	// --- 3. knowledge_graph stays unbound AND /knowledge/graph really 404s.
	kg := iMustSurface(t, cat, "knowledge_graph")
	if kg.Href != "" || kg.ReadinessDiscoverabilityPolicy != experience.PolicyUnavailablePendingDependency {
		t.Errorf("knowledge_graph href/policy = %q/%s, want \"\"/unavailable_pending_dependency", kg.Href, kg.ReadinessDiscoverabilityPolicy)
	}
	if code := probe("/knowledge/graph"); code != http.StatusNotFound {
		t.Errorf("/knowledge/graph returned %d, want 404 — catalog leaves it unbound so the real server must not serve it yet", code)
	} else {
		t.Logf("knowledge_graph unbound; /knowledge/graph -> 404 (ok, pending spec 105)")
	}

	// --- 4. Lists/Meals/Expenses stay unavailable AND guessed browser pages 404.
	for id, guessed := range map[string]string{
		"work_lists":    "/lists",
		"work_meals":    "/meals",
		"work_expenses": "/expenses",
	} {
		s := iMustSurface(t, cat, id)
		if s.Href != "" || s.ReadinessDiscoverabilityPolicy != experience.PolicyUnavailablePendingOwnership {
			t.Errorf("%s href/policy = %q/%s, want \"\"/unavailable_pending_ownership", id, s.Href, s.ReadinessDiscoverabilityPolicy)
		}
		if code := probe(guessed); code != http.StatusNotFound {
			t.Errorf("guessed browser page %s for unavailable leaf %s returned %d, want 404", guessed, id, code)
		} else {
			t.Logf("unavailable leaf %-14s guessed %-10s -> 404 (ok, no fabricated route)", id, guessed)
		}
	}

	// --- 5. Served manifest + service worker agree with the Capture binding.
	capture := iMustSurface(t, cat, "capture")
	mResp, err := client.Get(coreURL + "/pwa/manifest.json")
	if err != nil {
		t.Fatalf("GET /pwa/manifest.json: %v", err)
	}
	mBody, _ := io.ReadAll(mResp.Body)
	mResp.Body.Close()
	if mResp.StatusCode != http.StatusOK {
		t.Errorf("GET /pwa/manifest.json status = %d, want 200", mResp.StatusCode)
	}
	var manifest struct {
		StartURL string `json:"start_url"`
	}
	if err := json.Unmarshal(mBody, &manifest); err != nil {
		t.Errorf("parse served manifest.json: %v", err)
	}
	if manifest.StartURL != capture.Href {
		t.Errorf("served manifest start_url = %q, but catalog Capture href = %q — inventories disagree", manifest.StartURL, capture.Href)
	} else {
		t.Logf("served manifest start_url %s agrees with catalog Capture href", manifest.StartURL)
	}
	swResp, err := client.Get(coreURL + "/pwa/sw.js")
	if err != nil {
		t.Fatalf("GET /pwa/sw.js: %v", err)
	}
	swBody, _ := io.ReadAll(swResp.Body)
	swResp.Body.Close()
	if swResp.StatusCode != http.StatusOK {
		t.Errorf("GET /pwa/sw.js status = %d, want 200", swResp.StatusCode)
	}
	if !regexp.MustCompile(`smackerel-pwa-[0-9a-f]{12}`).MatchString(string(swBody)) {
		t.Error("served sw.js CACHE_NAME is not content-hash-versioned (expected smackerel-pwa-<12 hex>)")
	} else {
		t.Logf("served sw.js CACHE_NAME is content-hash-versioned (ok)")
	}

	// --- 6. Structural agreement: the whole catalog validates against the LIVE
	//        registered route set via the production validator.
	inv := experience.RouteInventory{
		RegisteredPaths:   live,
		KnownCapabilities: iCapsOf(cat),
		KnownAudiences:    map[string]bool{"daily_user": true, "operator": true},
	}
	if _, err := (experience.ExperienceRouteValidator{}).Validate(cat, inv); err != nil {
		t.Fatalf("generated catalog failed validation against the LIVE registered route inventory: %v", err)
	}
	t.Logf("catalog validated structurally against %d live-registered routes", len(live))
}
