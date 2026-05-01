//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates exercises
// SCN-040-007 and SCN-040-008 end to end against the live PWA + core stack
// served by `./smackerel.sh test e2e`. Each new health dashboard must
// expose its endpoint contract via data-* attributes and a role="status"
// region so the user can see live data, not stubbed copy.
func TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	pages := map[string][]string{
		"photo-health-lifecycle.html":  {"role=\"status\"", "/v1/photos/health/lifecycle", "Lifecycle"},
		"photo-health-duplicates.html": {"role=\"status\"", "/v1/photos/health/duplicates", "Duplicates"},
		"photo-health-removal.html":    {"role=\"status\"", "/v1/photos/health/removal", "Removal"},
		"photo-health-quality.html":    {"role=\"status\"", "/v1/photos/health/quality", "Quality"},
		"photo-confirm-action.html":    {"role=\"status\"", "/v1/photos/actions/plan", "/v1/photos/actions/confirm"},
	}
	for page, expected := range pages {
		body := getE2EText(t, cfg.CoreURL+"/pwa/"+page)
		for _, fragment := range expected {
			if !strings.Contains(body, fragment) {
				t.Fatalf("%s missing %q", page, fragment)
			}
		}
	}

	scripts := map[string][]string{
		"photo-health-lifecycle.js": {"/v1/photos/health/lifecycle", "review_state", "by_editor"},
		"photo-health-duplicates.js": {"/v1/photos/health/duplicates", "best-pick", "resolve",
			"action_token"},
		"photo-health-removal.js": {"/v1/photos/health/removal", "data-action-status", "method"},
		"photo-health-quality.js": {"/v1/photos/health/quality", "buckets"},
		"photo-confirm-action.js": {"/v1/photos/actions/plan", "/v1/photos/actions/confirm", "text_confirmation"},
	}
	for script, expected := range scripts {
		body := getE2EText(t, cfg.CoreURL+"/pwa/"+script)
		for _, fragment := range expected {
			if !strings.Contains(body, fragment) {
				t.Fatalf("%s missing %q", script, fragment)
			}
		}
	}

	// The lifecycle endpoint must return the dashboard envelope even when
	// no links exist yet, so the UI can render an empty state without
	// crashing.
	resp, err := apiGet(cfg, "/v1/photos/health/lifecycle")
	if err != nil {
		t.Fatalf("GET /v1/photos/health/lifecycle: %v", err)
	}
	body, err := readBody(resp)
	if err != nil {
		t.Fatalf("read lifecycle body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lifecycle status=%d body=%s", resp.StatusCode, string(body))
	}
	var summary struct {
		Total                 int     `json:"total"`
		ConfirmationThreshold float64 `json:"confirmation_threshold"`
	}
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode lifecycle summary: %v body=%s", err, string(body))
	}
	if summary.ConfirmationThreshold <= 0 || summary.ConfirmationThreshold > 1 {
		t.Fatalf("confirmation_threshold=%v, want (0,1]", summary.ConfirmationThreshold)
	}
}
