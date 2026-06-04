//go:build e2e

// Spec 073 SCOPE-073-05 TP-073-26 — SCN-073-B02 served-route canary.
package wiki

import (
	"testing"
	"time"
)

func TestWiki_TP_073_26_PeopleIndex(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	html := getText(t, cfg.CoreURL+"/pwa/wiki_people.html")
	mustContain(t, "wiki_people.html", html,
		`id="wiki-people-index"`,
		`data-endpoint="/api/people"`,
		`id="wiki-person-detail"`,
		`/pwa/wiki_people.js`,
	)

	js := getText(t, cfg.CoreURL+"/pwa/wiki_people.js")
	mustContain(t, "wiki_people.js", js,
		`/api/people?limit=50`,
		`/api/people/`,
		`/api/graph/edges?source=person:`,
		`wiki-timeline-entry`,
		`renderCrossLinkList`,
	)

	resp, body := apiGetJSON(t, cfg, "/api/people?limit=5", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/people status=%d body=%s", resp.StatusCode, string(body))
	}
}
