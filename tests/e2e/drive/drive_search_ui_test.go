//go:build e2e

package drive

import (
	"strings"
	"testing"
	"time"
)

// TestDriveSearchResultsShowSnippetBreadcrumbProviderSharingAndSensitivity
// proves SCN-038-010 against the live PWA bundle: the served Screen 5
// HTML and JS MUST expose the snippet, folder breadcrumb, provider chip,
// sharing badge, sensitivity badge, provider URL, and accessible action
// hooks Screen 5 needs. Without these elements, the UI cannot render
// drive search results, so a regression that dropped one would fail the
// test.
//
// Adversarial guards:
//   - the asserted substrings include both HTML element classes/ids AND
//     the JS keys that read drive-aware response fields; a regression
//     that renamed a field on either side would fail;
//   - the test queries the live HTTP endpoint, not a local file fixture,
//     so a build that forgot to embed the new PWA assets fails.
func TestDriveSearchResultsShowSnippetBreadcrumbProviderSharingAndSensitivity(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)

	html := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/drive-search.html")
	for _, expected := range []string{
		`id="drive-search-form"`,
		`id="drive-search-list"`,
		`drive-result-snippet`,
		`folder-breadcrumb-list`,
		`drive-result-badges`,
		`provider-chip`,
		`sharing-badge`,
		`sensitivity-badge`,
		`drive-availability-banner`,
		`drive-open-in-drive`,
		`drive-open-detail`,
	} {
		if !strings.Contains(html, expected) {
			previewLen := 400
			if len(html) < previewLen {
				previewLen = len(html)
			}
			t.Fatalf("drive-search.html missing %q; payload prefix=%q", expected, html[:previewLen])
		}
	}

	js := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/drive-search.js")
	for _, expected := range []string{
		`/api/search`,
		`result.drive`,
		`folder_breadcrumb`,
		`sharing_state`,
		`sensitivity`,
		`provider_url`,
		`actions_enabled`,
		`tombstoned`,
		`permission_lost`,
		`snippet`,
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("drive-search.js missing %q", expected)
		}
	}
}
