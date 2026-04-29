//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestOperatorStatus_RecommendationProvidersEmptyByDefault(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	resp, err := apiGet(cfg, "/status")
	if err != nil {
		t.Fatalf("status page request failed: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read status body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	html := string(body)
	if !strings.Contains(html, "Recommendation Providers") {
		t.Fatal("status page missing Recommendation Providers block")
	}
	if !strings.Contains(html, "0 recommendation providers configured") {
		t.Fatal("status page should render zero-provider recommendation state")
	}
}