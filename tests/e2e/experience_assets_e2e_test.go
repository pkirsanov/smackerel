//go:build e2e

// Spec 106 SCOPE-106-01 — XP106-01-A (regression E2E, e2e-api).
//
// "Experience assets expose immutable headers exact digests and network-only
// protected routes."
//
// This is a REAL live-stack e2e test: it drives the running core over
// CORE_EXTERNAL_URL (exported by the disposable `./smackerel.sh test e2e`
// stack) with NO interception and NO mock. It asserts, against the actual
// server, the three halves of the SCOPE-106-01 asset contract:
//
//  1. EXACT DIGESTS — every locked manifest asset served under /pwa/... is
//     byte-for-byte identical to the manifest's computed SHA-256 (content
//     integrity is real, never fabricated).
//  2. IMMUTABLE HEADERS — each locked static asset advertises a long-lived
//     immutable Cache-Control, and the service-worker cache identity is
//     content-hash-versioned so an asset change busts the immutable cache.
//  3. NETWORK-ONLY PROTECTED ROUTES — /api/* and /v1/* are never served as
//     precacheable immutable assets; they stay network-only and auth-gated.
//
// It SKIPs (not fails) when CORE_EXTERNAL_URL is unset, matching the repo e2e
// convention, so it is a no-op outside the live e2e lane.
package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/web"
)

func TestExperienceAssetsExposeImmutableHeadersExactDigestsAndNetworkOnlyProtectedRoutes(t *testing.T) {
	coreURL := strings.TrimSpace(os.Getenv("CORE_EXTERNAL_URL"))
	if coreURL == "" {
		t.Skip("e2e: CORE_EXTERNAL_URL not set — live stack not available")
	}
	coreURL = strings.TrimRight(coreURL, "/")

	manifest, err := web.BuildExperienceAssetManifest()
	if err != nil {
		t.Fatalf("BuildExperienceAssetManifest: %v", err)
	}
	if len(manifest.Assets) == 0 {
		t.Fatal("manifest has zero locked assets")
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// --- 1 + 2: exact digests + immutable headers on every locked asset.
	for _, a := range manifest.Assets {
		resp, err := client.Get(coreURL + a.ServedPath)
		if err != nil {
			t.Errorf("GET %s: %v", a.ServedPath, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", a.ServedPath, resp.StatusCode)
			continue
		}
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != a.SHA256 {
			t.Errorf("served bytes for %s do not match verified manifest digest: served=%s manifest=%s", a.ServedPath, got, a.SHA256)
		}
		cc := resp.Header.Get("Cache-Control")
		if !strings.Contains(cc, "immutable") || !strings.Contains(cc, "max-age=") {
			t.Errorf("locked asset %s must advertise an immutable long-lived Cache-Control; got %q", a.ServedPath, cc)
		}
	}

	// --- 2 (cont.): the service-worker cache identity is content-hash-versioned
	//        so the immutable assets are busted on any byte change.
	swResp, err := client.Get(coreURL + "/pwa/sw.js")
	if err != nil {
		t.Fatalf("GET /pwa/sw.js: %v", err)
	}
	sw, _ := io.ReadAll(swResp.Body)
	swResp.Body.Close()
	if !regexp.MustCompile(`smackerel-pwa-[0-9a-f]{12}`).MatchString(string(sw)) {
		t.Error("sw.js CACHE_NAME is not content-hash-versioned (expected smackerel-pwa-<12 hex>)")
	}

	// --- 3: protected data routes are network-only (never immutable-cached), and
	//        the classifier agrees with the served posture.
	for _, protected := range []string{"/api/health", "/v1/search"} {
		if !web.IsNetworkOnlyPath(protected) {
			t.Errorf("%s must be classified network-only", protected)
		}
		resp, err := client.Get(coreURL + protected)
		if err != nil {
			t.Errorf("GET %s: %v", protected, err)
			continue
		}
		cc := resp.Header.Get("Cache-Control")
		resp.Body.Close()
		if strings.Contains(cc, "immutable") {
			t.Errorf("protected route %s must NOT be served immutable; got Cache-Control=%q", protected, cc)
		}
	}
}
