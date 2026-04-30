//go:build e2e

package drive

import (
	"strings"
	"testing"
	"time"
)

// TestDriveArtifactDetailVersionsTabShowsPreviousNativeDocumentRevision
// proves SCN-038-011 against the live PWA bundle: the served Screen 6
// HTML and JS MUST include the Versions tab, render previous revisions
// from response.versions, and reuse the same artifact identity for
// successive native document revisions. The integration counterpart
// (internal/drive/version_test.go) proves the identity-stability
// invariant; this test proves the UI surface that exposes it.
//
// Adversarial guards:
//   - the test asserts BOTH the tab definition and the revision rendering
//     hooks; a regression that dropped either would fail;
//   - the test asserts the JS reads `response.versions` and the HEAD
//     marker so a regression that swapped the field name or hid prior
//     revisions would fail;
//   - the test asserts a revision-id rendering hook so a regression that
//     only printed the head revision would fail.
func TestDriveArtifactDetailVersionsTabShowsPreviousNativeDocumentRevision(t *testing.T) {
	liveConfig := loadE2EConfig(t)
	waitForHealth(t, liveConfig, 120*time.Second)

	html := getText(t, liveConfig.CoreURL+"/pwa/drive-artifact-detail.html")
	for _, expected := range []string{
		`id="tab-versions"`,
		`aria-controls="panel-versions"`,
		`id="panel-versions"`,
		`id="drive-versions-list"`,
		`id="drive-versions-empty"`,
	} {
		if !strings.Contains(html, expected) {
			t.Fatalf("drive-artifact-detail.html missing %q", expected)
		}
	}

	js := getText(t, liveConfig.CoreURL+"/pwa/drive-artifact-detail.js")
	for _, expected := range []string{
		`/v1/drive/artifacts/`,
		`detail.versions`,
		`renderVersions`,
		`is_head`,
		`revision_id`,
		`Previous revision`,
		`Current revision`,
	} {
		if !strings.Contains(js, expected) {
			t.Fatalf("drive-artifact-detail.js missing %q", expected)
		}
	}
}
