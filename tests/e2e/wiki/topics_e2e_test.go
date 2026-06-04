//go:build e2e

// Spec 073 SCOPE-073-05 TP-073-25 — SCN-073-B01 served-route canary.
// Asserts /pwa/wiki.html and /pwa/wiki_topics.html serve, and that
// the live /api/topics index returns the documented shape.
package wiki

import (
	"strings"
	"testing"
	"time"
)

type topicRow struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	LinkedArtifactCount int    `json:"linkedArtifactCount"`
	PeopleCount         int    `json:"peopleCount"`
	PlaceCount          int    `json:"placeCount"`
}
type topicsList struct {
	Items      []topicRow `json:"items"`
	NextCursor string     `json:"nextCursor"`
}

func TestWiki_TP_073_25_TopicsIndex(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	landing := getText(t, cfg.CoreURL+"/pwa/wiki.html")
	mustContain(t, "wiki.html", landing,
		`href="/pwa/wiki_topics.html"`,
		`data-wiki-section="topics"`,
		`/pwa/wiki.js`,
	)

	topicsHTML := getText(t, cfg.CoreURL+"/pwa/wiki_topics.html")
	mustContain(t, "wiki_topics.html", topicsHTML,
		`id="wiki-topics-index"`,
		`data-endpoint="/api/topics"`,
		`/pwa/wiki_topics.js`,
	)

	topicsJS := getText(t, cfg.CoreURL+"/pwa/wiki_topics.js")
	mustContain(t, "wiki_topics.js", topicsJS,
		`/api/topics?limit=50`,
		`/api/topics/`,
		`/api/graph/edges?source=topic:`,
		`validateTopicsList`,
		`renderCrossLinkList`,
	)

	var list topicsList
	resp, body := apiGetJSON(t, cfg, "/api/topics?limit=5", &list)
	if resp.StatusCode != 200 {
		t.Fatalf("GET /api/topics status=%d body=%s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), `"items"`) {
		t.Fatalf("topics body missing items envelope: %s", string(body))
	}
}
