//go:build e2e

// Spec 073 SCOPE-073-05 TP-073-27 — SCN-073-B03 served-route canary.
package wiki

import (
	"testing"
	"time"
)

func TestWiki_TP_073_27_PlacesIndex(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	html := getText(t, cfg.CoreURL+"/pwa/wiki_places.html")
	mustContain(t, "wiki_places.html", html,
		`id="wiki-places-index"`,
		`data-endpoint="/api/places"`,
		`id="wiki-place-detail"`,
		`/pwa/wiki_places.js`,
	)

	js := getText(t, cfg.CoreURL+"/pwa/wiki_places.js")
	mustContain(t, "wiki_places.js", js,
		`/api/places?limit=50`,
		`/api/places/`,
		`/api/graph/edges?source=place:`,
		`data-lat`,
	)

	resp, body := apiGetJSON(t, cfg, "/api/places?limit=5", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/places status=%d body=%s", resp.StatusCode, string(body))
	}
}
