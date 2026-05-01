//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRecommendationsWatchesWeb_ListAndEditorPagesAvailable proves the spec 039
// Scope 4 web surface: the watches list page and editor page render under the
// authenticated web group, and the editor includes the consent confirmation
// fields required by BS-021/BS-022 (no auto-create, no silent broadening).
func TestRecommendationsWatchesWeb_ListAndEditorPagesAvailable(t *testing.T) {
	cfg := loadE2EConfig(t)
	waitForHealth(t, cfg, 120*time.Second)

	listResp, err := apiGet(cfg, "/recommendations/watches")
	if err != nil {
		t.Fatalf("get watches list page: %v", err)
	}
	listBody, err := readBody(listResp)
	if err != nil {
		t.Fatalf("read list body: %v", err)
	}
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("watches list status = %d, want 200", listResp.StatusCode)
	}
	listHTML := string(listBody)
	if !strings.Contains(listHTML, "Watches") {
		t.Fatalf("watches list page missing 'Watches' header; body length=%d", len(listHTML))
	}

	editorResp, err := apiGet(cfg, "/recommendations/watches/new")
	if err != nil {
		t.Fatalf("get watch editor page: %v", err)
	}
	editorBody, err := readBody(editorResp)
	if err != nil {
		t.Fatalf("read editor body: %v", err)
	}
	if editorResp.StatusCode != http.StatusOK {
		t.Fatalf("watch editor status = %d, want 200", editorResp.StatusCode)
	}
	editorHTML := string(editorBody)
	// Consent confirmation fields proving BS-021/BS-022 — the editor MUST
	// expose the user-named knobs so the form cannot bypass consent.
	requiredField := []string{
		"consent_confirmation_scope_named",
		"consent_confirmation_sources_named",
		"consent_confirmation_rate_limit_named",
		"consent_confirmation_precision_named",
		"consent_confirmation_delivery_named",
	}
	for _, field := range requiredField {
		if !strings.Contains(editorHTML, field) {
			t.Fatalf("watch editor missing required consent confirmation field %q; body length=%d", field, len(editorHTML))
		}
	}
}
