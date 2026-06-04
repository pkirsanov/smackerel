//go:build e2e

// Spec 073 SCOPE-073-05 TP-073-30 — SCN-073-B06 annotation entry
// point canary + extended storage guard.
//
// Branches:
//
//   - Enabled path: when /api/annotations responds 2xx, the wiki
//     artifact JS module exposes the entry point with
//     data-annotation-available=true (assertion is static-source
//     based; the HTTP branch is exercised by spec 027 Scope 9 tests).
//   - Disabled path: when /api/annotations returns 404/403/401, the
//     wiki JS renders aria-disabled and a tooltip-style affordance.
//     Static check: the source contains aria-disabled handling.
//   - Storage guard extension: scan the served wiki_*.js for any
//     reference to localStorage/sessionStorage/indexedDB/CacheStorage
//     — same vocabulary as TP-073-06 — to prove the wiki surface
//     does not persist bearer/session material.
package wiki

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestWiki_TP_073_30_AnnotationEntryAndStorageGuard(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 30*time.Second)

	libJS := getText(t, cfg.CoreURL+"/pwa/wiki_lib.js")
	artJS := getText(t, cfg.CoreURL+"/pwa/wiki_artifact.js")

	// Annotation entry point shape.
	mustContain(t, "wiki_lib.js", libJS,
		`renderAnnotationEntryPoint`,
		`probeAnnotationEndpoint`,
		`/api/annotations?actor=me&limit=1`,
		`aria-disabled`,
		`data-annotation-available`,
		`X-Smackerel-Source`,
	)
	mustContain(t, "wiki_artifact.js", artJS,
		`renderAnnotationEntryPoint`,
		`/api/graph/edges?source=artifact:`,
	)

	// Storage guard extension — wiki must never reference these.
	storage := []*regexp.Regexp{
		regexp.MustCompile(`\blocalStorage\b`),
		regexp.MustCompile(`\bsessionStorage\b`),
		regexp.MustCompile(`\bindexedDB\b`),
		regexp.MustCompile(`\bCacheStorage\b`),
		regexp.MustCompile(`\bcaches\s*\.\s*open\b`),
	}
	for _, label := range []string{"wiki.js", "wiki_lib.js", "wiki_topics.js", "wiki_people.js", "wiki_places.js", "wiki_time.js", "wiki_artifact.js"} {
		body := getText(t, cfg.CoreURL+"/pwa/"+label)
		// Strip line comments before pattern matching to mirror the
		// TP-073-06 storage guard semantics.
		var noComments strings.Builder
		for _, line := range strings.Split(body, "\n") {
			if idx := strings.Index(line, "//"); idx >= 0 {
				noComments.WriteString(line[:idx])
			} else {
				noComments.WriteString(line)
			}
			noComments.WriteString("\n")
		}
		stripped := noComments.String()
		for _, re := range storage {
			if re.MatchString(stripped) {
				t.Fatalf("%s contains forbidden storage API %s — storage guard extension failed", label, re.String())
			}
		}
	}

	// Live probe — verify the route is wired (any non-502/503
	// response means the handler exists). The wiki probe degrades
	// gracefully on 401/403/404, which is the disabled-affordance
	// branch from the scope's Gherkin. The test stack runs with
	// shared bootstrap auth so /api/annotations?actor=me may return
	// 200 (per-user PASETO) OR 403 (bootstrap subject rejected) —
	// either proves the endpoint is reachable.
	resp, body := apiGetJSON(t, cfg, "/api/annotations?actor=me&limit=1", nil)
	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		// All four are acceptable: probe maps each to a branch.
	default:
		t.Fatalf("GET /api/annotations status=%d body=%s — expected 200/401/403/404", resp.StatusCode, string(body))
	}
	_ = time.Now()
}
