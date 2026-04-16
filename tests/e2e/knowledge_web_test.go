//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// T6-08: Knowledge dashboard renders at /knowledge.
func TestKnowledgeWeb_DashboardRenders(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/knowledge")
	if err != nil {
		t.Fatalf("dashboard request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body)[:200])
	}
	html := string(body)
	if !strings.Contains(html, "Knowledge") {
		t.Error("dashboard page missing 'Knowledge' text")
	}
	t.Logf("dashboard rendered: %d bytes", len(body))
}

// T6-09: Concepts list renders at /knowledge/concepts.
func TestKnowledgeWeb_ConceptsListRenders(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/knowledge/concepts")
	if err != nil {
		t.Fatalf("concepts page request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	t.Logf("concepts page rendered: %d bytes", len(body))
}

// T6-10: Lint report renders at /knowledge/lint.
func TestKnowledgeWeb_LintReportRenders(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/knowledge/lint")
	if err != nil {
		t.Fatalf("lint page request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	// 200 if report exists, could be 200 with empty state too
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	t.Logf("lint page rendered: %d bytes", len(body))
}

// T6-11: Existing pages still render with new nav (regression).
func TestKnowledgeWeb_ExistingPagesHaveKnowledgeNav(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	pages := []string{"/", "/digest", "/topics", "/settings", "/status"}
	for _, path := range pages {
		resp, err := apiGet(cfg, path)
		if err != nil {
			t.Fatalf("%s request failed: %v", path, err)
		}
		body, err := readBody(resp)
		if err != nil {
			t.Fatalf("%s read body: %v", path, err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("%s expected 200, got %d", path, resp.StatusCode)
			continue
		}
		if !strings.Contains(string(body), "/knowledge") {
			t.Errorf("%s page missing /knowledge nav link", path)
		}
	}
}
