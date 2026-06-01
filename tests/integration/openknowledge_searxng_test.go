//go:build integration

// SearxNG integration test for spec 064 SCOPE-07.
//
// This test drives the real SearxNG container against the
// disposable test compose. As of SCOPE-07, the docker-compose test
// stack does NOT yet ship a SearxNG service — adding it is a routed
// finding for SCOPE-06/the test-environment owner. When the
// container is unreachable, the test t.Skip's with an explicit
// message rather than failing, so the integration suite stays green
// until the service is wired in.
//
// To enable locally once the compose service ships:
//
//	OPEN_KNOWLEDGE_SEARXNG_URL=http://localhost:<port> \
//	    ./smackerel.sh test integration --go-run TestSearxNGIntegration
package integration

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/openknowledge/web"
)

func TestSearxNGIntegration_Smoke(t *testing.T) {
	endpoint := os.Getenv("OPEN_KNOWLEDGE_SEARXNG_URL")
	if endpoint == "" {
		t.Skip("OPEN_KNOWLEDGE_SEARXNG_URL not set; SearxNG container is not yet wired into the test compose (SCOPE-07 routed finding to SCOPE-06).")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	// Reachability probe — skip rather than fail when the container
	// is not running locally.
	probeReq, _ := http.NewRequest(http.MethodGet, endpoint+"/healthz", nil)
	if resp, err := client.Do(probeReq); err != nil {
		t.Skipf("SearxNG endpoint %s not reachable: %v", endpoint, err)
	} else {
		resp.Body.Close()
	}

	p, err := web.NewSearxNG(endpoint, client)
	if err != nil {
		t.Fatalf("NewSearxNG: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	snips, err := p.Search(ctx, "kale recipes", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(snips) == 0 {
		t.Fatalf("expected at least one snippet from live SearxNG; got 0")
	}
	for i, s := range snips {
		if s.URL == "" {
			t.Errorf("[%d] empty URL", i)
		}
		if s.ContentHash != web.CanonicalContentHash(s.URL, s.Title, s.Snippet) {
			t.Errorf("[%d] ContentHash not canonical", i)
		}
		if s.Provider != "searxng" {
			t.Errorf("[%d] Provider=%q", i, s.Provider)
		}
		if s.FetchedAt.IsZero() {
			t.Errorf("[%d] FetchedAt zero", i)
		}
	}
}
