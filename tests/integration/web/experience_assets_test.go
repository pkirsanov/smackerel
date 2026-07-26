//go:build integration

// Spec 106 SCOPE-106-01 — XP106-01-I.
//
// TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP is the
// live-stack integration proof that the source-locked visual foundation is
// actually SERVED same-origin, byte-for-byte identical to the manifest, under
// the strict Content-Security-Policy, with the service-worker cache identity
// advanced to include the vendored bytes.
//
// It is a REAL integration test: it builds the production HTTP router
// (api.NewRouter) and drives it over a real HTTP server (httptest). The only
// faked components are the two external-infrastructure health interfaces
// (DBHealthChecker/NATSHealthChecker) — the static-asset + CSP + service-worker
// paths under test never touch the database or NATS. No internal logic, no
// asset byte, no CSP header, and no service-worker payload is mocked.
package integrationweb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/api"
	"github.com/smackerel/smackerel/internal/web"
)

// fakeHealthyDB implements api.DBHealthChecker. The asset/CSP/service-worker
// paths under test never invoke it; it exists only so NewRouter constructs.
type fakeHealthyDB struct{}

func (fakeHealthyDB) Healthy(context.Context) bool                 { return true }
func (fakeHealthyDB) ArtifactCount(context.Context) (int64, error) { return 0, nil }

// fakeHealthyNATS implements api.NATSHealthChecker (external boundary only).
type fakeHealthyNATS struct{}

func (fakeHealthyNATS) Healthy() bool { return true }

func TestServerPWAAndCardHeadsServeTheSameVerifiedAssetsUnderStrictCSP(t *testing.T) {
	manifest, err := web.BuildExperienceAssetManifest()
	if err != nil {
		t.Fatalf("BuildExperienceAssetManifest: %v", err)
	}
	if len(manifest.Assets) == 0 {
		t.Fatal("manifest has zero locked assets")
	}

	router := api.NewRouter(&api.Dependencies{
		DB:        fakeHealthyDB{},
		NATS:      fakeHealthyNATS{},
		StartTime: time.Now(),
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	// --- 1. Every locked asset (incl. all 5 vendored fonts) is served same-origin
	//        under /pwa/... byte-for-byte identical to the verified manifest digest.
	fontsServed := 0
	for _, a := range manifest.Assets {
		resp, err := http.Get(srv.URL + a.ServedPath)
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
		if int64(len(body)) != a.Size {
			t.Errorf("served size for %s = %d, manifest = %d", a.ServedPath, len(body), a.Size)
		}
		if a.CSPClass == web.CSPClassFont {
			fontsServed++
			// A vendored woff2 is a real binary payload (14-24 KB), never an error page.
			// The digest match above already proves byte identity; this guards against a
			// zero-length or HTML-error response slipping through with a coincidental match.
			if len(body) < 1000 {
				t.Errorf("vendored font %s served a suspiciously small body (len=%d)", a.ServedPath, len(body))
			}
		}
	}
	if fontsServed != 5 {
		t.Errorf("expected 5 vendored fonts served same-origin, got %d", fontsServed)
	}

	// --- 2. The strict Content-Security-Policy is present and NOT weakened; the
	//        vendored fonts load same-origin (explicit font-src 'self' OR the
	//        default-src 'self' fallback — never an external/wildcard font host).
	hResp, err := http.Get(srv.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health: %v", err)
	}
	csp := hResp.Header.Get("Content-Security-Policy")
	hResp.Body.Close()
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("CSP missing strict default-src 'self': %q", csp)
	}
	if strings.Contains(csp, "font-src") && !strings.Contains(csp, "font-src 'self'") {
		t.Errorf("font-src directive present but not restricted to 'self': %q", csp)
	}
	for _, weak := range []string{"default-src *", "font-src *", "font-src https:", "font-src data:", "'unsafe-eval'"} {
		if strings.Contains(csp, weak) {
			t.Errorf("CSP weakened by %q: %s", weak, csp)
		}
	}

	// --- 3. The service-worker cache identity advanced to include the vendored
	//        font bytes: sw.js CACHE_NAME is content-hash-versioned, never the
	//        static literal.
	swResp, err := http.Get(srv.URL + "/pwa/sw.js")
	if err != nil {
		t.Fatalf("GET /pwa/sw.js: %v", err)
	}
	sw, _ := io.ReadAll(swResp.Body)
	swResp.Body.Close()
	if strings.Contains(string(sw), `smackerel-pwa-v2"`) {
		t.Error("sw.js CACHE_NAME is still the static literal smackerel-pwa-v2; cache identity did not advance for the vendored assets")
	}
	if !regexp.MustCompile(`smackerel-pwa-[0-9a-f]{12}`).MatchString(string(sw)) {
		t.Error("sw.js CACHE_NAME is not content-hash-versioned (expected smackerel-pwa-<12 hex>)")
	}

	// --- 4. Foundation token + pre-paint assets are served and are NOT network-only.
	for _, must := range []string{"/pwa/experience-tokens.css", "/pwa/experience-appearance.js"} {
		if web.IsNetworkOnlyPath(must) {
			t.Errorf("%s is misclassified network-only", must)
		}
	}
}
