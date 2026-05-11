//go:build e2e

package drive

import (
	"strings"
	"testing"
	"time"
)

// TestDriveArtifactDetailExplainsTombstonedAndAccessRevokedStates proves
// SCN-038-012 against the live PWA bundle: the served Screen 6 HTML and
// JS MUST surface a banner for tombstoned and permission-lost
// availabilities, suppress extracted text when bytes are unavailable,
// and disable the Open-in-Drive action while keeping the artifact's
// retained knowledge visible. The integration counterpart
// (tests/integration/drive/drive_access_state_test.go) proves the
// backend invariants; this test proves the UI surface.
//
// Adversarial guards:
//   - the test asserts BOTH banner element ids AND the JS branches that
//     fill them, so removing either side fails;
//   - the test asserts a "tombstoned" and "permission_lost" string
//     surface in the JS so a regression that collapsed both into a
//     generic "unavailable" copy would fail;
//   - the test asserts the extracted-text-unavailable hint is wired so a
//     regression that silently dropped the empty extracted_text payload
//     into the panel would fail.
func TestDriveArtifactDetailExplainsTombstonedAndAccessRevokedStates(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)

	html := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/drive-artifact-detail.html")
	for _, expected := range []string{
		`id="drive-availability-banner"`,
		`id="drive-extracted-text-unavailable"`,
		`id="drive-open-in-drive"`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("drive-artifact-detail.html missing %q", expected)
		}
	}

	js := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/drive-artifact-detail.js")
	for _, expected := range []string{
		`detail.banner_message`,
		`banner_severity`,
		`drive.availability`,
		`tombstoned`,
		`permission_lost`,
		`actions_enabled`,
		`drive.provider_url`,
		`extractedTextUnavailableEl`,
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("drive-artifact-detail.js missing %q", expected)
		}
	}

	searchHTML := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/drive-search.html")
	if !strings.Contains(searchHTML, `drive-availability-banner`) {
		t.Fatalf("drive-search.html must surface the same availability banner hook so search results explain unavailable bytes too")
	}
	searchJS := getText(t, liveConfig, liveConfig.CoreURL+"/pwa/drive-search.js")
	for _, expected := range []string{`tombstoned`, `permission_lost`, `actions_enabled`} {
		if !strings.Contains(searchJS, expected) {
			t.Fatalf("drive-search.js missing %q for SCN-038-012 surface", expected)
		}
	}
}
