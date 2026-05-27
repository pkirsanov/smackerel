# Report — BUG-033-001 — Extension manifest parity not lint-protected

## Summary

`web/extension/manifest.json` (Chrome MV3) and `web/extension/manifest.firefox.json`
(Firefox MV2) describe the same browser extension across two schemas that
express the same surfaces differently (MV3 splits API permissions from URL
host patterns into two arrays; MV2 merges both; MV3 stores CSP as a dict
keyed by `extension_pages`; MV2 stores CSP as a flat string). Spec 033's own
gaps probe documented two real cross-browser drifts on this exact surface
(GAP-F01: Firefox missing `alarms` permission and the `http://*/api/*` host
pattern; GAP-F03: Chrome and Firefox disagreeing on `content_security_policy`
`object-src`). Both gaps were fixed by hand, but no machine enforcement
guarded against recurrence. The round 9 devops probe of sweep
`sweep-2026-05-23-r30` surfaced this absence-of-enforcement as a devops
drift gap. Closed by adding `internal/web/extension_parity_contract_test.go`
(`package web`) with 1 live-file test + 1 baseline-sanity test + 7
adversarial sub-tests, locking name, version, description, API permissions,
host patterns, and CSP `object-src` parity at unit-test time.

## Completion Statement

All 10 DoD items in BUG-033-001-S1 are satisfied with executable evidence
below. New contract test green (9 sub-tests PASS), full `internal/web/...`
package gate green with zero regressions. Bug `state.json` transitions from
`open` → `resolved` with the state-transition-guard verifying all required
artifact + content invariants at commit time. No live spec 055 WIP files
are touched by this commit (verified via path-limited `git add` +
`git diff --cached --name-status` filter).

## Execution Summary

Discovered during sweep `sweep-2026-05-23-r30` round 9 (parent-expanded
child workflow, `mode: devops-to-doc` mapped from trigger `devops`) at HEAD
`a910f952fe55a506b85be6aea558ee0a70deb712`. Spec 033 ships a browser
extension targeting Chrome MV3 and Firefox MV2 from two manifest files in
`web/extension/`. The two manifests are currently in lockstep across every
parity surface — but no lint check, contract test, or pre-commit hook
enforces the parity invariant. Two historical bugs (GAP-F01, GAP-F03) prove
the class of drift is real and silent.

This report is appended after the implementation, test, validation, audit,
and docs steps below complete.

Closed in a single scope (BUG-033-001-S1) with one new file:

1. Added `internal/web/extension_parity_contract_test.go` (`package web`,
   511 lines) parsing both live manifest files with `encoding/json` plus
   `runtime.Caller`-based cwd-independent path resolution. The file
   contains:
   - `TestExtensionManifestParity_LiveFiles` — live-file parity check
     covering all six invariants plus GAP-F01 and GAP-F03 smoke checks.
   - `TestExtensionManifestParity_CanonicalBaselinePasses` — non-vacuity
     sanity check on the in-memory canonical pair used by the
     adversarial mutations.
   - 7 adversarial sub-tests (one per parity surface).

No manifest content changes, no lint changes, no package-extension script
changes, no docs changes outside the spec 033 report addendum and this bug
packet.

## Evidence

### Code Diff Evidence

Git-backed proof of the runtime/source/config delta (non-artifact paths only).
Executed: `git status --short -- specs/033-mobile-capture/ internal/web/` and
`wc -l internal/web/extension_parity_contract_test.go` against the working
tree before commit.

```text
$ git status --short -- specs/033-mobile-capture/ internal/web/
 M internal/web/handler.go
 M internal/web/templates.go
?? internal/web/extension_parity_contract_test.go
?? specs/033-mobile-capture/bugs/
```

```text
$ wc -l internal/web/extension_parity_contract_test.go
511 internal/web/extension_parity_contract_test.go
$ echo "Exit Code: $?"
Exit Code: 0
```

Runtime/source files touched by this bug:

- `internal/web/extension_parity_contract_test.go` — NEW file, 511 lines.
  `package web`. Pure JSON parsing plus comparison logic. 1 live-file
  test + 1 baseline sanity test + 7 adversarial sub-tests. No new
  dependencies (uses only `encoding/json`, `fmt`, `os`,
  `path/filepath`, `runtime`, `sort`, `strings`, `testing`).

The two modified files appearing in the working tree (`internal/web/handler.go`
and `internal/web/templates.go`) belong to spec 055 (notification source /
ntfy adapter) WIP and are deliberately EXCLUDED from this commit via
path-limited `git add`. The same exclusion applies to every other
spec 055 file in the working tree (`cmd/core/services.go`,
`cmd/core/wiring.go`, `config/smackerel.yaml`, `docs/API.md`,
`docs/Architecture.md`, `docs/Development.md`, `docs/Operations.md`,
`internal/api/health.go`, `internal/api/notifications.go`,
`internal/api/router.go`, `internal/api/router_test.go`,
`internal/config/config.go`, `internal/config/validate_test.go`,
`internal/notification/types.go`, `scripts/commands/config.sh`,
all `specs/055-*/`, `internal/api/notifications_ntfy.go`,
`internal/api/notifications_ntfy_test.go`,
`internal/db/migrations/038_notification_ntfy_source_adapter.sql`,
`internal/notification/source/`, `tests/e2e/notification_ntfy_source_*`,
`tests/stress/notification_ntfy_source_*`).

### Test Evidence

Four pieces of executable evidence below: the focused contract test green
with all 9 sub-tests PASS (Step 2), the full `internal/web/...` gate green
with zero regressions (Step 3), the adversarial sub-tests demonstrating the
regression catch via in-line error messages (also Step 2), and the
state-transition-guard PASS at commit time (Step 5).

### Step 1 — New file (internal/web/extension_parity_contract_test.go)

The new file declares `package web` and exposes no public API — every
identifier is unexported (test functions live in `_test.go` files which Go
links into the same package but only at `go test` time). Key shapes:

```go
// File: internal/web/extension_parity_contract_test.go (excerpt; verified via $ go test ./internal/web/... finished in 0.083s)
// Minimal JSON-tagged structs reading only the parity surfaces.
type chromeManifest struct {
    ManifestVersion int                  `json:"manifest_version"`
    Name            string               `json:"name"`
    Version         string               `json:"version"`
    Description     string               `json:"description"`
    Permissions     []string             `json:"permissions"`
    HostPermissions []string             `json:"host_permissions"`
    CSP             chromeCSPContainer   `json:"content_security_policy"`
}
type firefoxManifest struct {
    ManifestVersion int      `json:"manifest_version"`
    Name            string   `json:"name"`
    Version         string   `json:"version"`
    Description     string   `json:"description"`
    Permissions     []string `json:"permissions"`
    CSP             string   `json:"content_security_policy"`
}

// Six parity invariants asserted by assertExtensionManifestParity:
//   1. chrome.ManifestVersion == 3 && firefox.ManifestVersion == 2
//   2. chrome.Name == firefox.Name
//   3. chrome.Version == firefox.Version
//   4. chrome.Description == firefox.Description
//   5. set(chrome.Permissions) ⇔ set(firefox.Permissions − hostPatterns)
//      AND set(chrome.HostPermissions) ⇔ hostPatterns(firefox.Permissions)
//   6. object-src(chrome.CSP.ExtensionPages) == object-src(firefox.CSP) == 'none'
```

The host-pattern matcher `isHostPattern(s)` keys off the URL-pattern
grammar (prefixes `http://`, `https://`, `*://`, or literal
`<all_urls>`). The CSP helper `extractObjectSrc(csp)` splits on `;`,
lowercases per CSP grammar, and returns the `object-src` source-list
tokens; it returns `""` if the directive is absent (caught by invariant
6).

### Step 2 — Focused contract test (9 sub-tests, all PASS)

```text
$ go test -v -run TestExtensionManifestParity ./internal/web/...
=== RUN   TestExtensionManifestParity_LiveFiles
--- PASS: TestExtensionManifestParity_LiveFiles (0.00s)
=== RUN   TestExtensionManifestParity_CanonicalBaselinePasses
--- PASS: TestExtensionManifestParity_CanonicalBaselinePasses (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialMissingAlarmsInFirefox
    extension_parity_contract_test.go:387: adversarial OK: missing `alarms` in Firefox rejected with: parity violation: API permissions drift — missing-from-Firefox=[alarms], missing-from-Chrome=[] (every non-host API permission like alarms/storage/contextMenus MUST appear in BOTH manifests or extension behaviour silently differs across browsers; this is exactly the GAP-F01 class of bug where `alarms` was missed in Firefox; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialMissingAlarmsInFirefox (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialMissingHostPatternInFirefox
    extension_parity_contract_test.go:415: adversarial OK: missing host pattern in Firefox rejected with: parity violation: host pattern drift — missing-from-Firefox-permissions=[http://*/api/*], missing-from-Chrome-host_permissions=[] (every URL match pattern MUST appear in BOTH manifests, in Chrome under host_permissions and in Firefox merged into permissions, or cross-origin fetch() calls silently fail on one browser; this is the GAP-F01 root cause; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialMissingHostPatternInFirefox (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialMismatchedCSPObjectSrc
    extension_parity_contract_test.go:434: adversarial OK: CSP object-src drift rejected with: parity violation: CSP object-src drift — Chrome="'none'", Firefox="'self'" (object-src MUST match across both manifests for defense-in-depth parity with the PWA; this is exactly the GAP-F03 regression; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialMismatchedCSPObjectSrc (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialMismatchedName
    extension_parity_contract_test.go:453: adversarial OK: name drift rejected with: parity violation: name drift — Chrome="Smackerel", Firefox="Smackerel (Firefox Edition)" (the user-visible product name MUST be identical across browsers; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialMismatchedName (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialMismatchedVersion
    extension_parity_contract_test.go:473: adversarial OK: version drift rejected with: parity violation: version drift — Chrome="1.0.0", Firefox="1.0.1" (extension version MUST be identical so package-extension.sh produces matched filenames and users see the same version string; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialMismatchedVersion (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialExtraPermissionInChrome
    extension_parity_contract_test.go:492: adversarial OK: extra Chrome permission rejected with: parity violation: API permissions drift — missing-from-Firefox=[downloads], missing-from-Chrome=[] (every non-host API permission like alarms/storage/contextMenus MUST appear in BOTH manifests or extension behaviour silently differs across browsers; this is exactly the GAP-F01 class of bug where `alarms` was missed in Firefox; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialExtraPermissionInChrome (0.00s)
=== RUN   TestExtensionManifestParity_AdversarialMismatchedDescription
    extension_parity_contract_test.go:510: adversarial OK: description drift rejected with: parity violation: description drift — Chrome="Capture anything to Smackerel with one click", Firefox="Save pages and snippets to Smackerel" (the user-visible description MUST be identical across browsers; BUG-033-001)
--- PASS: TestExtensionManifestParity_AdversarialMismatchedDescription (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/web     0.029s
```

Each adversarial sub-test's `t.Logf` line names the specific drifted
surface (`alarms`, `http://*/api/*`, `object-src`, `name`, `version`,
`downloads`, `description`) — proving the contract assertion is
non-tautological and would catch the next GAP-F01- or GAP-F03-class
regression.

### Step 3 — Broader regression gate (full internal/web/... package)

```text
$ go test ./internal/web/...
ok      github.com/smackerel/smackerel/internal/web     0.083s
ok      github.com/smackerel/smackerel/internal/web/icons       0.005s
```

Zero regressions across the full sibling test suite (web handler tests,
icons tests). The new contract test adds <0.05s to the gate.

### Step 4 — Bubbles artifact gates

```text
$ bash .github/bubbles/scripts/artifact-lint.sh specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/ 2>&1 | tail -3
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```text
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/ 2>&1 | tail -3
ℹ️  DoD fidelity scenarios: 2 (mapped: 2, unmapped: 0)

RESULT: PASSED (0 warnings)
$ echo "Exit Code: $?"
Exit Code: 0
```

(Full output captured at commit time; both gates PASS with no warnings.)

### Step 5 — State-transition guard PASS at commit time

```text
$ bash scripts/runtime/state-transition-guard.sh specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/
PASS — bug status open → resolved
  Check 4: 11/11 DoD items checked in scopes.md
  Check 4A: zero non-checkbox list items in DoD sections
  Check 4B: zero non-canonical scope statuses
  Check 5: state.json scopes[].dod.{total,checked} reflect scopes.md
  Check 6: certification.completedScopes matches state.json::scopes
  Check 7: report.md present and references every DoD item
  Check 8: uservalidation.md present and accepts the bug
  Check 9: no spec 055 WIP contamination in commit-pending diff
```

(Full output captured at commit time after artifact lint and contract
test gates are green.)

## DoD Closure Accounting (10 items)

| DoD | Item | Evidence |
|-----|------|----------|
| 1 | `internal/web/extension_parity_contract_test.go` exists, `package web`, `encoding/json` + `runtime.Caller` | Step 1 (struct snippets), Step 3 (file named in `go test` output) |
| 2 | `TestExtensionManifestParity_LiveFiles` green | Step 2 (PASS line at top of verbose output) |
| 3 | 7 adversarial sub-tests cover one parity surface each | Step 2 (each sub-test PASS line + adversarial error mentioning the drifted surface) |
| 4 | Baseline-sanity sub-test proves canonical pair passes | Step 2 (`TestExtensionManifestParity_CanonicalBaselinePasses` PASS) |
| 5 | Test classifies as `unit`, runs under `go test ./internal/web/...` | Step 3 (package gate green at 0.083s; pure-CPU runtime confirms no integration deps) |
| 6 | SCN-033-B001 and SCN-033-B002 mapped to concrete test functions | scopes.md Test Plan table; Step 2 verbose output |
| 7 | Broader regression suite passes | Step 3 (`ok github.com/smackerel/smackerel/internal/web 0.083s` + icons) |
| 8 | `artifact-lint.sh` PASS at commit time | Step 4 |
| 9 | `traceability-guard.sh` PASS at commit time | Step 4 |
| 10 | `state-transition-guard.sh` PASS for open → resolved | Step 5 |
| 11 | No spec 055 WIP contamination | Step 5 Check 9; Step 5 Code Diff Evidence section above |

## Cross-References

- Parent spec report: `specs/033-mobile-capture/report.md` — addendum
  "Devops Probe (devops-to-doc, SQS round 9, 2026-05-23)" appended in
  this commit, citing BUG-033-001 closure.
- Sibling devops-to-doc closure pattern: `specs/049-monitoring-stack/bugs/BUG-049-001-prometheus-external-image-contract-drift/`
  (round 7 of the same sweep) — established the 1-live + N-adversarial
  Go contract test pattern reused here.
- Sweep state: `.specify/memory/sweep-2026-05-23-r30.json` — round 9
  marked `completed_owned` in this commit.
- Historical GAP-F01: `specs/033-mobile-capture/report.md` (gaps probe)
  — Firefox missing `alarms`; the adversarial sub-test
  `AdversarialMissingAlarmsInFirefox` is the regression test for this
  exact class of bug.
- Historical GAP-F03: `specs/033-mobile-capture/report.md` (gaps probe)
  — CSP `object-src` mismatch; the adversarial sub-test
  `AdversarialMismatchedCSPObjectSrc` is the regression test for this
  exact class of bug.
