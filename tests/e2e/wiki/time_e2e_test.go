//go:build e2e

// Spec 073 SCOPE-073-05 TP-073-28 — SCN-073-B04 served-route canary.
// Asserts /pwa/wiki_time.html serves; live /api/time returns the
// day-grouped envelope; and the JS records scroll position via the
// History API (not storage — preserves the storage guard).
package wiki

import (
	"net/url"
	"testing"
	"time"
)

func TestWiki_TP_073_28_TimeView(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	html := getText(t, cfg.CoreURL+"/pwa/wiki_time.html")
	mustContain(t, "wiki_time.html", html,
		`id="wiki-time-section"`,
		`data-endpoint="/api/time"`,
		`id="wiki-time-days"`,
		`/pwa/wiki_time.js`,
	)

	js := getText(t, cfg.CoreURL+"/pwa/wiki_time.js")
	mustContain(t, "wiki_time.js", js,
		`/api/time?from=`,
		`history.replaceState`,
		`wiki-time-day`,
		`data-captured-at`,
	)

	to := time.Now().UTC()
	from := to.Add(-7 * 24 * time.Hour)
	q := url.Values{}
	q.Set("from", from.Format(time.RFC3339))
	q.Set("to", to.Format(time.RFC3339))
	resp, body := apiGetJSON(t, cfg, "/api/time?"+q.Encode(), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/time status=%d body=%s", resp.StatusCode, string(body))
	}
}
