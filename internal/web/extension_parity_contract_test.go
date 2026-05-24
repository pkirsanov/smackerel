// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package web — BUG-033-001 browser-extension manifest parity contract test.
//
// The contract: web/extension/manifest.json (Chrome MV3) and
// web/extension/manifest.firefox.json (Firefox MV2) describe the SAME
// browser extension. Several semantic surfaces MUST stay in lockstep
// across both manifests or the extension silently behaves differently
// on the two browsers:
//
//  1. name           — user-visible product name must be identical
//  2. version        — package version must match (already checked by
//                      scripts/runtime/web-validate.sh; re-checked here
//                      so the Go test suite is self-contained)
//  3. description    — user-visible description must be identical
//  4. permissions    — every non-host API permission (storage,
//                      contextMenus, notifications, activeTab, alarms,
//                      …) must appear in BOTH manifests. Chrome MV3
//                      lists API permissions under `permissions` and
//                      host patterns under `host_permissions`; Firefox
//                      MV2 merges both lists into `permissions`. The
//                      contract normalises by extracting host patterns
//                      from the Firefox `permissions` array and then
//                      comparing the remaining API-permission sets.
//  5. host patterns  — every URL pattern (https://*/api/*, http://*/api/*)
//                      must appear in BOTH manifests (Chrome
//                      `host_permissions`, Firefox `permissions`).
//  6. CSP object-src — both manifests must set object-src 'none' for
//                      defense-in-depth parity with the PWA. Chrome
//                      stores CSP as a dict (`content_security_policy.
//                      extension_pages`); Firefox stores it as a flat
//                      string (`content_security_policy`).
//
// Why this exists:
// Spec 033's gaps probe (gaps-to-doc, 2026-04-22) found two manifest
// parity drifts that had already shipped to disk:
//
//   - GAP-F01: Chrome manifest declared host_permissions for
//     "https://*/api/*" and "http://*/api/*" but the Firefox manifest
//     omitted them, breaking every cross-origin capture fetch on
//     Firefox.
//   - GAP-F03: Chrome and Firefox manifests disagreed on
//     content_security_policy object-src; one had 'self' and the other
//     had 'none' before unification.
//
// Both were fixed by hand in spec 033, but no lint or test guarded
// against recurrence. Round 9 (devops-to-doc) of sweep
// sweep-2026-05-23-r30 surfaced this as a devops drift gap. The class
// of bugs has historically caused real Firefox-only breakage. This
// contract test closes the drift permanently. Adversarial sub-tests
// prove the check would fail if any of the six parity surfaces above
// drifted.
//
// Cross-reference:
//   - specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/
//   - specs/033-mobile-capture/report.md (GAP-F01, GAP-F03 history)
//   - web/extension/manifest.json
//   - web/extension/manifest.firefox.json
//   - scripts/runtime/web-validate.sh (sibling lint-time version check)
package web

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// chromeManifest is the minimal MV3 manifest shape needed for parity
// assertions. Unmodelled fields (icons, action, background, etc.) are
// ignored so adding unrelated MV3 keys stays a non-event.
type chromeManifest struct {
	ManifestVersion int                  `json:"manifest_version"`
	Name            string               `json:"name"`
	Version         string               `json:"version"`
	Description     string               `json:"description"`
	Permissions     []string             `json:"permissions"`
	HostPermissions []string             `json:"host_permissions"`
	CSP             chromeCSPContainer   `json:"content_security_policy"`
}

// chromeCSPContainer is the MV3 CSP dict shape. MV3 requires CSP under
// `extension_pages` for extension pages (popup, options, sandboxed
// frames). Other CSP scopes (sandbox, isolated_world) are not used by
// this extension and are not modelled.
type chromeCSPContainer struct {
	ExtensionPages string `json:"extension_pages"`
}

// firefoxManifest is the minimal MV2 manifest shape needed for parity
// assertions. Firefox MV2 merges API permissions and host patterns into
// a single `permissions` array; CSP is a flat string.
type firefoxManifest struct {
	ManifestVersion int      `json:"manifest_version"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Description     string   `json:"description"`
	Permissions     []string `json:"permissions"`
	CSP             string   `json:"content_security_policy"`
}

// isHostPattern returns true for entries that look like URL match
// patterns (used by both MV3 host_permissions and MV2 merged
// permissions). The exact MV2/MV3 host-pattern grammar allows `*://*/*`,
// `<all_urls>`, scheme-prefixed patterns, and wildcards; this matcher
// recognises the forms actually used by Smackerel's extension
// manifests. Adding a new scheme prefix (file://, ws://) here is
// expected if the extension ever needs it.
func isHostPattern(s string) bool {
	switch {
	case strings.HasPrefix(s, "http://"),
		strings.HasPrefix(s, "https://"),
		strings.HasPrefix(s, "*://"),
		s == "<all_urls>":
		return true
	}
	return false
}

// extractObjectSrc returns the value of the `object-src` directive from
// a Content-Security-Policy string, or "" if the directive is absent.
// CSP grammar: directives are semicolon-separated; each directive name
// is whitespace-separated from its source list. Case-insensitive per
// the CSP spec, so we lowercase before scanning. The returned value
// preserves the source-list tokens joined by a single space (e.g.
// "'none'", "'self'", "'self' https://example.com").
func extractObjectSrc(csp string) string {
	lower := strings.ToLower(csp)
	for _, raw := range strings.Split(lower, ";") {
		directive := strings.TrimSpace(raw)
		if directive == "" {
			continue
		}
		fields := strings.Fields(directive)
		if len(fields) >= 2 && fields[0] == "object-src" {
			return strings.Join(fields[1:], " ")
		}
	}
	return ""
}

// assertExtensionManifestParity returns nil iff every parity invariant
// holds for the two parsed manifests. The error names the specific
// parity surface and the specific drift so adversarial sub-tests can
// pattern-match the failure mode.
func assertExtensionManifestParity(chrome chromeManifest, firefox firefoxManifest) error {
	// Sanity: both manifests must declare their own MV. This guards
	// against accidentally feeding a Chrome manifest to the Firefox
	// parser (or vice versa) and getting a vacuously-passing parity
	// check.
	if chrome.ManifestVersion != 3 {
		return fmt.Errorf("parity precondition: Chrome manifest manifest_version=%d, expected 3 (MV3)", chrome.ManifestVersion)
	}
	if firefox.ManifestVersion != 2 {
		return fmt.Errorf("parity precondition: Firefox manifest manifest_version=%d, expected 2 (MV2)", firefox.ManifestVersion)
	}

	// Parity 1 — name
	if chrome.Name != firefox.Name {
		return fmt.Errorf("parity violation: name drift — Chrome=%q, Firefox=%q (the user-visible product name MUST be identical across browsers; BUG-033-001)", chrome.Name, firefox.Name)
	}

	// Parity 2 — version
	if chrome.Version != firefox.Version {
		return fmt.Errorf("parity violation: version drift — Chrome=%q, Firefox=%q (extension version MUST be identical so package-extension.sh produces matched filenames and users see the same version string; BUG-033-001)", chrome.Version, firefox.Version)
	}

	// Parity 3 — description
	if chrome.Description != firefox.Description {
		return fmt.Errorf("parity violation: description drift — Chrome=%q, Firefox=%q (the user-visible description MUST be identical across browsers; BUG-033-001)", chrome.Description, firefox.Description)
	}

	// Parity 4 — non-host API permissions
	chromeAPIPerms := stringSet(chrome.Permissions)
	firefoxHostFromPerms := []string{}
	firefoxAPIPerms := map[string]struct{}{}
	for _, p := range firefox.Permissions {
		if isHostPattern(p) {
			firefoxHostFromPerms = append(firefoxHostFromPerms, p)
			continue
		}
		firefoxAPIPerms[p] = struct{}{}
	}
	if missing, extra := setDiff(chromeAPIPerms, firefoxAPIPerms); len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("parity violation: API permissions drift — missing-from-Firefox=%v, missing-from-Chrome=%v (every non-host API permission like alarms/storage/contextMenus MUST appear in BOTH manifests or extension behaviour silently differs across browsers; this is exactly the GAP-F01 class of bug where `alarms` was missed in Firefox; BUG-033-001)", missing, extra)
	}

	// Parity 5 — host patterns
	chromeHostSet := stringSet(chrome.HostPermissions)
	firefoxHostSet := stringSet(firefoxHostFromPerms)
	if missing, extra := setDiff(chromeHostSet, firefoxHostSet); len(missing) > 0 || len(extra) > 0 {
		return fmt.Errorf("parity violation: host pattern drift — missing-from-Firefox-permissions=%v, missing-from-Chrome-host_permissions=%v (every URL match pattern MUST appear in BOTH manifests, in Chrome under host_permissions and in Firefox merged into permissions, or cross-origin fetch() calls silently fail on one browser; this is the GAP-F01 root cause; BUG-033-001)", missing, extra)
	}

	// Parity 6 — CSP object-src
	chromeObj := extractObjectSrc(chrome.CSP.ExtensionPages)
	firefoxObj := extractObjectSrc(firefox.CSP)
	if chromeObj != firefoxObj {
		return fmt.Errorf("parity violation: CSP object-src drift — Chrome=%q, Firefox=%q (object-src MUST match across both manifests for defense-in-depth parity with the PWA; this is exactly the GAP-F03 regression; BUG-033-001)", chromeObj, firefoxObj)
	}
	if chromeObj != "'none'" {
		return fmt.Errorf("parity violation: CSP object-src=%q in both manifests, expected 'none' (BUG-033-001 / GAP-F03 fixed both to 'none'; any future widening to 'self' or '*' must be a deliberate spec change with a separate adversarial regression test)", chromeObj)
	}

	return nil
}

// stringSet builds a set from a slice for cheap difference checks.
func stringSet(in []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, s := range in {
		out[s] = struct{}{}
	}
	return out
}

// setDiff returns (missing-from-b, missing-from-a) as sorted slices for
// stable error-message formatting.
func setDiff(a, b map[string]struct{}) (missingFromB, missingFromA []string) {
	for k := range a {
		if _, ok := b[k]; !ok {
			missingFromB = append(missingFromB, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			missingFromA = append(missingFromA, k)
		}
	}
	sort.Strings(missingFromB)
	sort.Strings(missingFromA)
	return missingFromB, missingFromA
}

// extensionRepoRoot returns the repository root by climbing two
// directories up from this test file (internal/web/ -> repo root).
// Using runtime.Caller makes the path independent of `go test` CWD,
// which makes the test work both from `cd internal/web && go test` and
// from `cd /workspace && go test ./...` (the path used by go-unit.sh
// and by the CI build matrix).
func extensionRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// loadLiveExtensionManifests parses the live Chrome and Firefox manifests
// from web/extension/. Returns the two parsed structs or fatals the
// test with the JSON path and parse error.
func loadLiveExtensionManifests(t *testing.T) (chromeManifest, firefoxManifest) {
	t.Helper()
	root := extensionRepoRoot(t)

	chromePath := filepath.Join(root, "web", "extension", "manifest.json")
	chromeBytes, err := os.ReadFile(chromePath)
	if err != nil {
		t.Fatalf("read %s: %v", chromePath, err)
	}
	var chrome chromeManifest
	if err := json.Unmarshal(chromeBytes, &chrome); err != nil {
		t.Fatalf("json.Unmarshal %s: %v", chromePath, err)
	}

	firefoxPath := filepath.Join(root, "web", "extension", "manifest.firefox.json")
	firefoxBytes, err := os.ReadFile(firefoxPath)
	if err != nil {
		t.Fatalf("read %s: %v", firefoxPath, err)
	}
	var firefox firefoxManifest
	if err := json.Unmarshal(firefoxBytes, &firefox); err != nil {
		t.Fatalf("json.Unmarshal %s: %v", firefoxPath, err)
	}

	return chrome, firefox
}

// TestExtensionManifestParity_LiveFiles parses the live Chrome MV3 and
// Firefox MV2 manifests and asserts every parity invariant holds.
func TestExtensionManifestParity_LiveFiles(t *testing.T) {
	chrome, firefox := loadLiveExtensionManifests(t)
	if err := assertExtensionManifestParity(chrome, firefox); err != nil {
		t.Fatalf("live-file parity violation: %v", err)
	}

	// Smoke checks for surfaces with historical regression precedent.
	// If any of these fire, the parity assertion above also fires; the
	// duplicate signal is intentional so a future regression can be
	// triaged by surface in O(1) rather than parsing a generic error.
	chromePerms := stringSet(chrome.Permissions)
	if _, ok := chromePerms["alarms"]; !ok {
		t.Fatal("smoke: Chrome manifest missing `alarms` permission (GAP-F01 added it; periodic offline-sync flush will silently fail without it)")
	}
	firefoxPerms := stringSet(firefox.Permissions)
	if _, ok := firefoxPerms["alarms"]; !ok {
		t.Fatal("smoke: Firefox manifest missing `alarms` permission (GAP-F01 added it to BOTH manifests; if this fires the parity test was bypassed)")
	}
	if extractObjectSrc(chrome.CSP.ExtensionPages) != "'none'" {
		t.Fatalf("smoke: Chrome CSP object-src=%q, expected 'none' (GAP-F03 tightened from 'self' to 'none' for defense-in-depth parity with the PWA)", extractObjectSrc(chrome.CSP.ExtensionPages))
	}
	if extractObjectSrc(firefox.CSP) != "'none'" {
		t.Fatalf("smoke: Firefox CSP object-src=%q, expected 'none' (GAP-F03 tightened from 'self' to 'none' for defense-in-depth parity with the PWA)", extractObjectSrc(firefox.CSP))
	}
}

// ---- Adversarial sub-tests ----
//
// Each adversarial test crafts an in-memory pair of manifests that
// REGRESSES one specific parity dimension, then asserts the contract
// function REJECTS the pair with a clear error message. These prove the
// live-file test above is not tautological — that the parity assertion
// would actually catch each historically-documented class of drift.

// canonicalChrome returns a known-good Chrome MV3 manifest used as the
// baseline for adversarial mutations. Adversarial tests mutate ONE
// field and assert the parity check rejects the pair.
func canonicalChrome() chromeManifest {
	return chromeManifest{
		ManifestVersion: 3,
		Name:            "Smackerel",
		Version:         "1.0.0",
		Description:     "Capture anything to Smackerel with one click",
		Permissions:     []string{"storage", "contextMenus", "notifications", "activeTab", "alarms"},
		HostPermissions: []string{"https://*/api/*", "http://*/api/*"},
		CSP:             chromeCSPContainer{ExtensionPages: "script-src 'self'; object-src 'none'"},
	}
}

// canonicalFirefox returns a known-good Firefox MV2 manifest matching
// canonicalChrome — every API permission and host pattern present in
// the Chrome version is reflected here.
func canonicalFirefox() firefoxManifest {
	return firefoxManifest{
		ManifestVersion: 2,
		Name:            "Smackerel",
		Version:         "1.0.0",
		Description:     "Capture anything to Smackerel with one click",
		Permissions: []string{
			"storage", "contextMenus", "notifications", "activeTab", "alarms",
			"https://*/api/*", "http://*/api/*",
		},
		CSP: "script-src 'self'; object-src 'none'",
	}
}

// TestExtensionManifestParity_CanonicalBaselinePasses sanity-checks
// that the canonical baselines used by the adversarial tests below
// actually satisfy the parity contract. If this test fails, every
// adversarial test below is meaningless (they all start from a broken
// baseline).
func TestExtensionManifestParity_CanonicalBaselinePasses(t *testing.T) {
	if err := assertExtensionManifestParity(canonicalChrome(), canonicalFirefox()); err != nil {
		t.Fatalf("canonical baseline failed parity check (the adversarial tests below would be vacuous): %v", err)
	}
}

// TestExtensionManifestParity_AdversarialMissingAlarmsInFirefox proves
// the contract fails if `alarms` is added to Chrome but dropped from
// Firefox — the EXACT regression GAP-F01 fixed.
func TestExtensionManifestParity_AdversarialMissingAlarmsInFirefox(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	// Drop alarms from Firefox only.
	pruned := firefox.Permissions[:0]
	for _, p := range firefox.Permissions {
		if p == "alarms" {
			continue
		}
		pruned = append(pruned, p)
	}
	firefox.Permissions = pruned

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: dropping `alarms` from Firefox was ACCEPTED (the parity check is tautological — it would NOT catch the GAP-F01 regression)")
	}
	if !strings.Contains(err.Error(), "alarms") {
		t.Fatalf("adversarial parity test failed: error did not mention `alarms`: %v", err)
	}
	t.Logf("adversarial OK: missing `alarms` in Firefox rejected with: %v", err)
}

// TestExtensionManifestParity_AdversarialMissingHostPatternInFirefox
// proves the contract fails if a host pattern declared in Chrome's
// host_permissions is missing from Firefox's permissions (where MV2
// expects host patterns merged in). This is the GAP-F01 root cause for
// cross-origin fetch failures.
func TestExtensionManifestParity_AdversarialMissingHostPatternInFirefox(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	// Drop http://*/api/* from Firefox only.
	pruned := firefox.Permissions[:0]
	for _, p := range firefox.Permissions {
		if p == "http://*/api/*" {
			continue
		}
		pruned = append(pruned, p)
	}
	firefox.Permissions = pruned

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: dropping `http://*/api/*` from Firefox permissions was ACCEPTED (the parity check does not verify host-pattern parity — cross-origin fetch() to non-HTTPS server URLs would silently fail on Firefox)")
	}
	if !strings.Contains(err.Error(), "http://*/api/*") {
		t.Fatalf("adversarial parity test failed: error did not mention `http://*/api/*`: %v", err)
	}
	t.Logf("adversarial OK: missing host pattern in Firefox rejected with: %v", err)
}

// TestExtensionManifestParity_AdversarialMismatchedCSPObjectSrc proves
// the contract fails if Chrome and Firefox disagree on
// object-src — the EXACT regression GAP-F03 fixed.
func TestExtensionManifestParity_AdversarialMismatchedCSPObjectSrc(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	// Loosen Firefox CSP back to 'self' (the pre-GAP-F03 state).
	firefox.CSP = "script-src 'self'; object-src 'self'"

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: object-src `self` in Firefox while Chrome has `none` was ACCEPTED (the parity check does not verify CSP parity — it would NOT catch the GAP-F03 regression)")
	}
	if !strings.Contains(err.Error(), "object-src") {
		t.Fatalf("adversarial parity test failed: error did not mention `object-src`: %v", err)
	}
	t.Logf("adversarial OK: CSP object-src drift rejected with: %v", err)
}

// TestExtensionManifestParity_AdversarialMismatchedName proves the
// contract fails if the user-visible name drifts. A contributor renaming
// only one manifest would otherwise ship a confusingly-named extension
// to one browser silently.
func TestExtensionManifestParity_AdversarialMismatchedName(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	firefox.Name = "Smackerel (Firefox Edition)"

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: name drift was ACCEPTED (the parity check does not verify name parity)")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Fatalf("adversarial parity test failed: error did not mention `name`: %v", err)
	}
	t.Logf("adversarial OK: name drift rejected with: %v", err)
}

// TestExtensionManifestParity_AdversarialMismatchedVersion proves the
// contract fails if the version drifts. This duplicates the version
// check in scripts/runtime/web-validate.sh; the redundancy is
// intentional because Go tests run on every CI build and lint may be
// skipped in some local workflows.
func TestExtensionManifestParity_AdversarialMismatchedVersion(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	firefox.Version = "1.0.1"

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: version drift was ACCEPTED (the parity check does not verify version parity — package-extension.sh would produce mismatched filenames for the two browsers)")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("adversarial parity test failed: error did not mention `version`: %v", err)
	}
	t.Logf("adversarial OK: version drift rejected with: %v", err)
}

// TestExtensionManifestParity_AdversarialExtraPermissionInChrome
// proves the contract fails when a NEW permission is added to Chrome
// but a contributor forgets to mirror it to Firefox — the most likely
// future regression mode given GAP-F01's history.
func TestExtensionManifestParity_AdversarialExtraPermissionInChrome(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	chrome.Permissions = append(chrome.Permissions, "downloads")

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: extra permission in Chrome (`downloads`) without Firefox mirror was ACCEPTED (the parity check would NOT catch the next GAP-F01-style regression)")
	}
	if !strings.Contains(err.Error(), "downloads") {
		t.Fatalf("adversarial parity test failed: error did not mention `downloads`: %v", err)
	}
	t.Logf("adversarial OK: extra Chrome permission rejected with: %v", err)
}

// TestExtensionManifestParity_AdversarialMismatchedDescription proves
// the contract fails when descriptions drift. Documented as a
// user-visible parity surface alongside name.
func TestExtensionManifestParity_AdversarialMismatchedDescription(t *testing.T) {
	chrome := canonicalChrome()
	firefox := canonicalFirefox()
	firefox.Description = "Save pages and snippets to Smackerel"

	err := assertExtensionManifestParity(chrome, firefox)
	if err == nil {
		t.Fatal("adversarial parity test failed: description drift was ACCEPTED")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Fatalf("adversarial parity test failed: error did not mention `description`: %v", err)
	}
	t.Logf("adversarial OK: description drift rejected with: %v", err)
}
