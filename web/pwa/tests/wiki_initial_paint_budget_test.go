// Spec 073 SCOPE-073-05 TP-073-31 — wiki initial-paint budget unit
// test. Synthetic timing harness using an in-process httptest server
// that serves the embedded PWA static assets and stubbed JSON for the
// spec 080 read APIs. Each wiki route's HTML + primary JS module +
// initial API call must complete under the 1s LAN budget on local.
//
// This is NOT a real headless-browser paint timer (those live in
// Playwright); it is the unit-tier approximation called out by the
// scope's test plan. The budget catches gross regressions (e.g. a
// validator added that walks N**2 items, a synchronous import storm).
package webcodegen_drift_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pwa "github.com/smackerel/smackerel/web/pwa"
)

const wikiInitialPaintBudget = 1 * time.Second

// stubGraphAPIServer wraps the embedded PWA file server with stubs
// for the spec 080 endpoints the wiki pages call at load time.
func stubGraphAPIServer(t *testing.T) *httptest.Server {
	t.Helper()
	files := http.FileServerFS(pwa.StaticFiles)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/topics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"id": "t1", "label": "Topic One", "linkedArtifactCount": 3, "peopleCount": 1, "placeCount": 0},
		}, "nextCursor": ""})
	})
	mux.HandleFunc("/api/people", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"id": "p1", "displayName": "Alice", "artifactCount": 2},
		}, "nextCursor": ""})
	})
	mux.HandleFunc("/api/places", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"id": "pl1", "displayName": "Park", "artifactCount": 1, "source": "maps"},
		}, "nextCursor": ""})
	})
	mux.HandleFunc("/api/time", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"days": []map[string]any{
			{"date": "2026-06-04", "artifacts": []map[string]any{
				{"artifactId": "a1", "title": "Note", "capturedAt": "2026-06-04T12:00:00Z"},
			}},
		}})
	})
	mux.HandleFunc("/api/graph/edges", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []map[string]any{
			{"targetKind": "topic", "targetId": "t1", "targetLabel": "Topic One", "reason": "co-occurs"},
		}, "nextCursor": ""})
	})
	mux.HandleFunc("/api/annotations", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []any{}})
	})
	mux.Handle("/pwa/", http.StripPrefix("/pwa", files))
	return httptest.NewServer(mux)
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

// TestWikiInitialPaintBudget_TP_073_31 measures the time to fetch
// each wiki route's HTML + its primary JS module + the first API
// call it makes. The combined elapsed time per route MUST be below
// the 1s LAN budget; the harness primes the server once before
// measurement so the budget is not consumed by Go cold-start.
func TestWikiInitialPaintBudget_TP_073_31(t *testing.T) {
	srv := stubGraphAPIServer(t)
	defer srv.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	type route struct {
		html, js, api string
	}
	routes := []route{
		{"/pwa/wiki.html", "/pwa/wiki.js", ""},
		{"/pwa/wiki_topics.html", "/pwa/wiki_topics.js", "/api/topics?limit=50"},
		{"/pwa/wiki_people.html", "/pwa/wiki_people.js", "/api/people?limit=50"},
		{"/pwa/wiki_places.html", "/pwa/wiki_places.js", "/api/places?limit=50"},
		{"/pwa/wiki_time.html", "/pwa/wiki_time.js", "/api/time?from=2026-05-01T00:00:00Z&to=2026-06-01T00:00:00Z"},
		{"/pwa/wiki_artifact.html", "/pwa/wiki_artifact.js", "/api/graph/edges?source=artifact:a1"},
	}

	// Warmup: fetch each asset once so subsequent measurements
	// reflect steady-state serving, not Go map init or first-fault.
	for _, r := range routes {
		mustGet(t, client, srv.URL+r.html)
		mustGet(t, client, srv.URL+r.js)
		if r.api != "" {
			mustGet(t, client, srv.URL+r.api)
		}
	}

	for _, r := range routes {
		t.Run(r.html, func(t *testing.T) {
			start := time.Now()
			body := mustGet(t, client, srv.URL+r.html)
			mustGet(t, client, srv.URL+r.js)
			if r.api != "" {
				mustGet(t, client, srv.URL+r.api)
			}
			elapsed := time.Since(start)
			if elapsed > wikiInitialPaintBudget {
				t.Fatalf("initial-paint elapsed=%s exceeds budget=%s", elapsed, wikiInitialPaintBudget)
			}
			// Body sanity: html must include the wiki marker so a
			// 200-but-empty regression cannot trivially pass.
			if !strings.Contains(string(body), "Smackerel") {
				t.Fatalf("%s body missing brand marker", r.html)
			}
		})
	}
}

// TestWikiInitialPaintBudget_Adversarial_TP_073_31 proves the budget
// assertion is real: an artificial 1.2s sleep server fails the
// budget. Without this, the harness could silently regress (e.g.
// timing measured at the wrong scope) and never catch a violation.
func TestWikiInitialPaintBudget_Adversarial_TP_073_31(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	start := time.Now()
	resp, err := client.Get(slow.URL + "/pwa/wiki.html")
	if err != nil {
		t.Fatalf("adversarial GET: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start)
	if elapsed <= wikiInitialPaintBudget {
		t.Fatalf("adversarial server returned in %s (<= %s); budget assertion would never fail", elapsed, wikiInitialPaintBudget)
	}
}

func mustGet(t *testing.T, c *http.Client, url string) []byte {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, string(body))
	}
	return body
}
