//go:build e2e

// Spec 106 SCOPE-106-02 — XP106-02-A (regression E2E, e2e-api).
//
// "Generated catalog binds only registered authorized browser destinations and
// route-free groups."
//
// This is a REAL live-stack e2e test: it drives the running core over
// CORE_EXTERNAL_URL (exported by the disposable `./smackerel.sh test e2e`
// stack) with NO interception and NO mock, and it adds NO new browser route or
// API — it probes ONLY destinations the generated catalog already binds, which
// are already-registered routes. It proves the AUTHORIZATION posture of those
// bindings against the actual server:
//
//   - every catalog active-leaf href resolves to a really-registered
//     destination whose response is a genuine authorization outcome
//     (public 200/3xx OR access-controlled 401/403) — NEVER a 404 (which would
//     mean the catalog bound an invented/unregistered endpoint) and NEVER a
//     5xx (broken destination),
//   - the access-controlled server destinations actually ENFORCE auth
//     (401/403 unauthenticated) when the stack has auth enabled — i.e. the
//     catalog binds AUTHORIZED destinations, not open holes,
//   - route-free groups (Work, Sources, Admin) and every honestly-unavailable
//     leaf (Lists, Meals, Expenses, Graph) expose NO href — they are never
//     clickable destinations.
//
// Adversarial (non-tautological): a deliberately-unregistered control path MUST
// 404, so the "registered-authorized" probe can distinguish a real route from
// an invented one.
//
// It SKIPs (not fails) when CORE_EXTERNAL_URL is unset, matching the repo e2e
// convention, so it is a no-op outside the live e2e lane.
package e2e

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/experience"
)

func TestGeneratedCatalogBindsOnlyRegisteredAuthorizedBrowserDestinationsAndRouteFreeGroups(t *testing.T) {
	coreURL := strings.TrimRight(strings.TrimSpace(os.Getenv("CORE_EXTERNAL_URL")), "/")
	if coreURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}

	// Do NOT follow redirects: a 302/303 to /login is itself a real
	// authorization outcome we want to observe, not chase.
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
	// A registered destination answers with a real authorization outcome, never
	// a 404 (unregistered/invented) and never a 5xx (broken).
	isPublic := func(code int) bool {
		return code == http.StatusOK || code == http.StatusMovedPermanently ||
			code == http.StatusFound || code == http.StatusSeeOther
	}
	isProtected := func(code int) bool {
		return code == http.StatusUnauthorized || code == http.StatusForbidden
	}

	cat, err := experience.GeneratedCatalog()
	if err != nil {
		t.Fatalf("GeneratedCatalog: %v", err)
	}

	// --- 0. ADVERSARIAL CONTROL: an unregistered path MUST 404.
	const control = "/definitely-not-registered-xp106-02-a"
	if code := probe(control); code != http.StatusNotFound {
		t.Fatalf("adversarial control %s returned %d, want 404 — probe cannot distinguish registered-authorized from invented routes", control, code)
	}
	t.Logf("adversarial control %-40s -> 404 (correctly unregistered)", control)

	// --- 1. Detect whether the live stack enforces auth, using a known
	//        access-controlled server page. The disposable e2e stack sets a
	//        non-empty SMACKEREL_AUTH_TOKEN, so auth is expected ON; the check
	//        stays adaptive so a dev/no-auth stack degrades honestly instead of
	//        flaking.
	settingsCode := probe("/settings")
	authEnforced := isProtected(settingsCode)
	t.Logf("auth-mode probe /settings -> %d (authEnforced=%v)", settingsCode, authEnforced)

	// --- 2. Every active-leaf href is a registered, authorized destination.
	activeLeaves := 0
	protectedServerLeaves := 0
	for _, s := range cat.Surfaces {
		if s.ReadinessDiscoverabilityPolicy != experience.PolicyReadyWhenJourneyReady &&
			s.ReadinessDiscoverabilityPolicy != experience.PolicyOperatorOnlyWhenReady {
			continue
		}
		activeLeaves++
		if s.Href == "" {
			t.Errorf("active leaf %q has empty href — cannot be a browser destination", s.ID)
			continue
		}
		code := probe(s.Href)
		if code == http.StatusNotFound {
			t.Errorf("active leaf %q href %s returned 404 — catalog bound an unregistered/invented destination", s.ID, s.Href)
			continue
		}
		if code >= http.StatusInternalServerError {
			t.Errorf("active leaf %q href %s returned %d — destination is broken", s.ID, s.Href, code)
			continue
		}

		// Public surfaces: the PWA file tree (served without auth) and the
		// /assistant front-door alias. Everything else is an access-controlled
		// server destination.
		public := strings.HasPrefix(s.Href, "/pwa/") || s.Href == "/assistant"
		switch {
		case public:
			if !isPublic(code) {
				t.Errorf("public destination %q href %s returned %d, want 200/3xx", s.ID, s.Href, code)
			}
		default:
			protectedServerLeaves++
			if authEnforced {
				if !isProtected(code) {
					t.Errorf("access-controlled destination %q href %s returned %d, want 401/403 (catalog must bind AUTHORIZED destinations, not open holes)", s.ID, s.Href, code)
				}
			} else if !isPublic(code) && !isProtected(code) {
				t.Errorf("destination %q href %s returned %d, want a real authorization outcome (200/3xx or 401/403)", s.ID, s.Href, code)
			}
		}
		t.Logf("active leaf %-18s href=%-30s -> %d (public=%v)", s.ID, s.Href, code, public)
	}
	if activeLeaves < 10 {
		t.Fatalf("suspiciously few active leaves probed (%d) — catalog likely not loaded", activeLeaves)
	}
	if authEnforced && protectedServerLeaves < 5 {
		t.Fatalf("expected several access-controlled server destinations, saw %d — auth-enforcement assertion is vacuous", protectedServerLeaves)
	}

	// --- 3. Route-free groups and honestly-unavailable leaves expose NO href.
	for _, id := range []string{"work", "sources", "admin"} {
		s := aMustSurface(t, cat, id)
		if s.Kind != experience.KindRouteGroup {
			t.Errorf("route-free group %q kind = %s, want route_group", id, s.Kind)
		}
		if s.Href != "" {
			t.Errorf("route-free group %q exposes href %q — a group must never be a clickable destination", id, s.Href)
		}
	}
	for _, id := range []string{"work_lists", "work_meals", "work_expenses", "knowledge_graph"} {
		s := aMustSurface(t, cat, id)
		if s.Href != "" {
			t.Errorf("unavailable leaf %q exposes href %q — an unproven destination must not be bound", id, s.Href)
		}
	}
	t.Logf("route-free groups (work/sources/admin) + unavailable leaves (lists/meals/expenses/graph) expose no href (ok)")
}

func aMustSurface(t *testing.T, c experience.ProductExperienceCatalog, id string) experience.Surface {
	t.Helper()
	for _, s := range c.Surfaces {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("generated catalog missing expected surface %q", id)
	return experience.Surface{}
}
