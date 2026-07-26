//go:build e2e

// Spec 106 SCOPE-106-03 — XP106-03-A (regression E2E, e2e-api).
//
// "Availability content auth and mutation outcomes remain structurally distinct
//
//	through real routes."
//
// HONESTY / COUPLING NOTE (read before extending this file):
// The spec-106 renderer-neutral presenters are NOT yet wired into the live
// server/PWA routes — that cutover is SCOPE-106-04/05, and the handwritten
// renderers remain the active, untouched authority. Therefore the FULL DoD-row
// behavior (the AVAILABILITY band surfaced as a distinct route axis, and the
// shared-presenter PROJECTION of the availability/content/auth/mutation outcomes
// through the live routes) is genuinely NOT provable at the route layer yet, and
// the XP106-03-A DoD row + its regression-planning row stay UNCHECKED, coupled
// forward to SCOPE-106-04/05 (exactly as SCOPE-106-02 coupled XP106-02-W). The
// type-level and real-owner projection of these outcomes is already proven by
// XP106-03-U (unit) and XP106-03-I (real-owner integration); the 401/403-vs-empty
// redaction/privacy-clear by the shared directive is proven by XP106-03-P.
//
// What IS truthfully provable NOW — and is proven live below — is that the
// EXISTING real routes ALREADY keep the outcome classes STRUCTURALLY DISTINCT:
// an auth-loss (401/403), a served-content shell (2xx/3xx), a not-found (404),
// and an unauthenticated mutation are each a DIFFERENT observable outcome, so the
// running server never collapses a failure into an empty page or a fabricated
// success. This is the pre-existing invariant the shared-state foundation must
// preserve, and it is what this lane certifies against the live stack.
//
// Real live-stack e2e over CORE_EXTERNAL_URL (exported by the disposable
// `./smackerel.sh test e2e` stack) with NO interception, NO mock, NO auth
// injection. It SKIPs (not fails) when CORE_EXTERNAL_URL is unset, matching the
// repo e2e convention, so it is a no-op outside the live e2e lane.
package e2e

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAvailabilityContentAuthAndMutationOutcomesRemainStructurallyDistinctThroughRealRoutes(t *testing.T) {
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
	get := func(path string) (int, string) {
		resp, err := client.Get(coreURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		return resp.StatusCode, string(b)
	}
	post := func(path string, body []byte) int {
		// Auth middleware runs before any body parsing, so an unauthenticated POST
		// resolves to its auth outcome regardless of content-type/body shape.
		resp, err := client.Post(coreURL+path, "application/octet-stream", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	isAuthLoss := func(c int) bool { return c == http.StatusUnauthorized || c == http.StatusForbidden }
	isServed := func(c int) bool {
		return c == http.StatusOK || c == http.StatusMovedPermanently ||
			c == http.StatusFound || c == http.StatusSeeOther
	}

	// 0. ADVERSARIAL CONTROL: an unregistered route MUST 404 — a distinct outcome
	//    class that proves the probe distinguishes a real outcome from an invented
	//    one (non-tautological).
	const control = "/definitely-not-registered-xp106-03-a"
	if c, _ := get(control); c != http.StatusNotFound {
		t.Fatalf("adversarial control %s -> %d, want 404 — probe cannot distinguish outcome classes", control, c)
	}
	t.Logf("adversarial control %-38s -> 404 (distinct not-found class)", control)

	// 1. Adaptive auth detection using a known protected read. The disposable e2e
	//    stack sets a non-empty SMACKEREL_AUTH_TOKEN, so auth is expected ON; the
	//    check stays adaptive so a dev/no-auth stack degrades honestly, not flaky.
	settingsCode, _ := get("/settings")
	authEnforced := isAuthLoss(settingsCode)
	t.Logf("auth-mode probe /settings -> %d (authEnforced=%v)", settingsCode, authEnforced)

	// 2. AUTH-LOSS is structurally distinct from content and never collapsed into
	//    a served success: every protected read returns an auth-loss status
	//    (401/403) unauthenticated — NOT a 200 serving protected business content.
	protectedReads := []string{
		"/", "/digest", "/settings", "/knowledge", "/recommendations",
		"/notifications", "/api/digest", "/api/recent",
	}
	authLossSeen := 0
	for _, p := range protectedReads {
		c, body := get(p)
		if authEnforced {
			if !isAuthLoss(c) {
				t.Errorf("protected read %s unauthenticated -> %d, want auth-loss (401/403) — a failure must not be served as content/success", p, c)
				continue
			}
			// The auth-loss response must not serve an authenticated business
			// surface (structural no-collapse: failure never becomes content).
			if strings.Contains(strings.ToLower(body), "sign out") {
				t.Errorf("auth-loss response for %s appears to serve an authenticated surface (contains 'sign out')", p)
			}
			authLossSeen++
			t.Logf("protected read %-14s -> %d (auth-loss, distinct from content)", p, c)
		} else {
			t.Logf("protected read %-14s -> %d (auth not enforced on this stack)", p, c)
		}
	}

	// 3. CONTENT served is structurally distinct from auth-loss and not-found:
	//    public surfaces return a served 2xx/3xx shell, never an auth-loss/404.
	for _, p := range []string{"/pwa/", "/login"} {
		c, _ := get(p)
		if !isServed(c) {
			t.Errorf("public content %s -> %d, want served (2xx/3xx) — content class distinct from auth-loss/not-found", p, c)
			continue
		}
		t.Logf("public content %-14s -> %d (served content, distinct from auth-loss/not-found)", p, c)
	}

	// 4. MUTATION outcome is structurally distinct and never a fabricated success:
	//    an unauthenticated POST to the REAL capture mutation is REFUSED
	//    (auth-loss), NOT a 200-accepted. This is the live-route form of
	//    "refused/unauthorized is never announced as success" (SCN-106-010).
	captureCode := post("/api/capture", []byte{})
	if authEnforced {
		if !isAuthLoss(captureCode) {
			t.Errorf("POST /api/capture unauthenticated -> %d, want auth-loss (401/403) — a refused mutation must not be a fabricated success", captureCode)
		}
		if isServed(captureCode) {
			t.Errorf("POST /api/capture unauthenticated returned a served/success code %d — a failure was collapsed into success", captureCode)
		}
		t.Logf("mutation POST /api/capture -> %d (auth-loss; refused, not a fabricated success)", captureCode)
	} else {
		t.Logf("mutation POST /api/capture -> %d (auth not enforced on this stack)", captureCode)
	}

	// 5. DISTINCTNESS MATRIX: when auth is enforced, the outcome classes observed
	//    through the real routes are mutually distinct — auth-loss (401/403),
	//    served content (2xx/3xx), and not-found (404) never share a code — so the
	//    existing routes keep the axes structurally distinct with no collapse.
	if authEnforced {
		if authLossSeen == 0 {
			t.Fatalf("no protected read produced an auth-loss outcome — cannot prove distinctness")
		}
		pwaCode, _ := get("/pwa/")
		if isAuthLoss(pwaCode) || pwaCode == http.StatusNotFound {
			t.Errorf("served-content probe /pwa/ (%d) is not distinct from auth-loss/not-found", pwaCode)
		}
		t.Logf("distinctness matrix (auth enforced): auth-loss=401/403 vs content=%d vs not-found=404 — mutually distinct (ok)", pwaCode)
	}
}
