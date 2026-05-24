# Design — BUG-033-001 — Extension manifest parity not lint-protected

## Current Truth

Pre-fix scan (executed against HEAD `a910f952fe55a506b85be6aea558ee0a70deb712`).

```text
$ grep -nE 'permissions|content_security_policy|version' web/extension/manifest.json
4:  "version": "1.0.0",
9:  "permissions": [
16:  "host_permissions": [
20:  "content_security_policy": {

$ grep -nE 'permissions|content_security_policy|version' web/extension/manifest.firefox.json
4:  "version": "1.0.0",
9:  "permissions": [
20:  "content_security_policy": "script-src 'self'; object-src 'none'",

$ python3 -c "
import json
c = json.load(open('web/extension/manifest.json'))
f = json.load(open('web/extension/manifest.firefox.json'))
print('chrome.name=', c['name'])
print('firefox.name=', f['name'])
print('chrome.permissions=', sorted(c['permissions']))
print('chrome.host_permissions=', sorted(c['host_permissions']))
firefox_api = sorted(p for p in f['permissions'] if not (p.startswith('http://') or p.startswith('https://')))
firefox_hosts = sorted(p for p in f['permissions'] if p.startswith('http://') or p.startswith('https://'))
print('firefox.api_permissions=', firefox_api)
print('firefox.host_patterns_inline=', firefox_hosts)
print('chrome.csp_extension_pages=', c['content_security_policy']['extension_pages'])
print('firefox.csp=', f['content_security_policy'])
"
chrome.name= Smackerel
firefox.name= Smackerel
chrome.permissions= ['activeTab', 'alarms', 'contextMenus', 'notifications', 'storage']
chrome.host_permissions= ['http://*/api/*', 'https://*/api/*']
firefox.api_permissions= ['activeTab', 'alarms', 'contextMenus', 'notifications', 'storage']
firefox.host_patterns_inline= ['http://*/api/*', 'https://*/api/*']
chrome.csp_extension_pages= script-src 'self'; object-src 'none'
firefox.csp= script-src 'self'; object-src 'none'
```

The two manifests are presently in lockstep across every parity surface
(name, version, description, API permissions, host patterns, CSP
`object-src`). The `scripts/runtime/web-validate.sh` lint already
enforces version parity at lint-time but does not enforce the other
five surfaces.

```text
$ grep -nE 'manifest.firefox.json|version_consistency|extension' scripts/runtime/web-validate.sh | head -40
116:    log_check "Checking extension manifest.json"
... (version consistency block exists; parity for other surfaces does not)
```

The lint script is shell-driven and the existing version check uses a
small Python block. Adding five more parity checks in shell+Python
would work but would split the canonical parity invariant across two
languages (shell+Python lint AND any future Go consumer), and would
weaken the regression-test layer (lint can be skipped in some local
workflows; the Go test suite runs on every CI build).

## Design

### Fix — Go contract test

Add `internal/web/extension_parity_contract_test.go` (new file,
`package web`) that parses both manifests with `encoding/json` and
asserts six parity invariants. The test mirrors the BUG-049-001 pattern
(`internal/deploy/external_images_contract_test.go`): a single live-file
test plus a baseline-sanity test plus one adversarial sub-test per
parity dimension.

**Parsed shapes (minimal, JSON-tagged):**

```go
type chromeManifest struct {
    ManifestVersion int    `json:"manifest_version"`           // must be 3
    Name            string `json:"name"`
    Version         string `json:"version"`
    Description     string `json:"description"`
    Permissions     []string `json:"permissions"`              // API perms only
    HostPermissions []string `json:"host_permissions"`         // URL patterns
    CSP struct {
        ExtensionPages string `json:"extension_pages"`         // dict shape
    } `json:"content_security_policy"`
}

type firefoxManifest struct {
    ManifestVersion int      `json:"manifest_version"`         // must be 2
    Name            string   `json:"name"`
    Version         string   `json:"version"`
    Description     string   `json:"description"`
    Permissions     []string `json:"permissions"`              // API perms + URL patterns merged
    CSP             string   `json:"content_security_policy"`  // flat string
}
```

Both structs are intentionally minimal. Adding unrelated MV2 / MV3 keys
(icons, action, background, browser_specific_settings, …) stays a
non-event because the JSON decoder ignores unknown fields by default.

**Normalisation:**

The Firefox `permissions` array merges API permissions with URL host
patterns. The test partitions Firefox `permissions` by whether each
entry matches the URL-pattern grammar (`http://`, `https://`, `*://`,
`<all_urls>`). After partitioning, two sets compare cleanly:

- Chrome.Permissions vs Firefox.Permissions − Firefox.HostPatterns
- Chrome.HostPermissions vs Firefox.HostPatterns

This avoids declaring a static list of "known API permissions" — the
matcher is keyed off the URL-pattern grammar, which is stable across
manifest revisions.

**Six invariants:**

1. `chrome.ManifestVersion == 3` and `firefox.ManifestVersion == 2`
   (precondition; guards against accidentally feeding the wrong file to
   the parser).
2. `chrome.Name == firefox.Name`.
3. `chrome.Version == firefox.Version`.
4. `chrome.Description == firefox.Description`.
5. `set(chrome.Permissions) == set(firefox.Permissions) - firefoxHostPatterns`
   AND `set(chrome.HostPermissions) == firefoxHostPatterns`.
6. `objectSrc(chrome.CSP.ExtensionPages) == objectSrc(firefox.CSP) == 'none'`.

**CSP object-src extraction:**

A small `extractObjectSrc(csp string) string` helper splits the CSP on
`;`, lowercases (per the CSP spec, directive names are
case-insensitive), and returns the source-list tokens for `object-src`
joined by single spaces. This avoids brittle full-string equality on
the entire CSP and isolates the `object-src` parity surface from
unrelated directive edits (e.g. a future `style-src` tightening).

### Adversarial sub-tests (regression layer)

One adversarial sub-test per parity dimension, each mutating ONE field
in an in-memory canonical baseline and asserting `assertExtensionManifestParity`
rejects the pair with an error mentioning the drifted surface:

| Sub-test                                          | Mutation                                                    | Catches                |
|---------------------------------------------------|-------------------------------------------------------------|------------------------|
| `AdversarialMissingAlarmsInFirefox`               | Drop `alarms` from Firefox.permissions                      | GAP-F01 regression     |
| `AdversarialMissingHostPatternInFirefox`          | Drop `http://*/api/*` from Firefox.permissions              | GAP-F01 root cause     |
| `AdversarialMismatchedCSPObjectSrc`               | Loosen Firefox CSP to `object-src 'self'`                   | GAP-F03 regression     |
| `AdversarialMismatchedName`                       | Change Firefox.name to `"Smackerel (Firefox Edition)"`      | name drift             |
| `AdversarialMismatchedVersion`                    | Change Firefox.version to `"1.0.1"`                         | version drift          |
| `AdversarialExtraPermissionInChrome`              | Append `downloads` to Chrome.permissions                    | future GAP-F01 mode    |
| `AdversarialMismatchedDescription`                | Change Firefox.description                                  | description drift      |

Plus `CanonicalBaselinePasses` as a sanity test — if the baseline
itself were broken, every adversarial sub-test would be vacuous.

Total: 1 live-file + 1 baseline sanity + 7 adversarial = 9 tests.

Test categorization: `unit` (no docker, no browser runtime, pure JSON
parsing + comparison logic; runs in `./smackerel.sh test unit --go`).

### Why not also extend `scripts/runtime/web-validate.sh`?

Considered and rejected:

- Splits the canonical parity invariant across shell+Python (lint) and
  Go (test). Future maintainers must keep two implementations in sync.
- Lint can be skipped in some local workflows (`./smackerel.sh build`
  without lint). The Go test runs on every CI build via
  `./smackerel.sh test unit --go` and on the pre-push hook.
- The BUG-049-001 precedent (sibling devops-to-doc closure) is
  Go-test-only with no parallel shell check. Following the precedent
  keeps the repo consistent.

The lint version check stays as-is (it pre-dates this bug and would be
redundant-but-harmless to remove; out of scope).

## Adversarial Considerations

- **Schema brittleness**: a future MV2-to-MV3 Firefox migration (when
  Firefox completes MV3 rollout) would change Firefox's host-pattern
  shape. The test asserts the current MV2 schema explicitly via the
  manifest_version precondition. A future schema migration must update
  the parser (additive — the contract assertion functions stay the
  same).
- **MV3-only keys**: the contract MUST NOT assert parity for
  Chrome-only keys (`background.service_worker`, `host_permissions` as
  a separate array) or Firefox-only keys
  (`browser_specific_settings.gecko`). The minimal parsed structs only
  read shared-surface fields, so additions to either browser-specific
  area are ignored.
- **False negative — extra host pattern in Firefox**: the test catches
  this symmetrically via `setDiff` (it reports BOTH missing-from-A and
  missing-from-B). Verified by the `AdversarialExtraPermissionInChrome`
  sub-test which exercises the symmetric path.
- **CSP tokenisation**: the `extractObjectSrc` helper assumes CSP
  directives are separated by `;`, which is the only delimiter per the
  CSP grammar. Edge case: a CSP with no `object-src` directive returns
  `""`; invariant 6 then catches the mismatch with the actual `'none'`.
- **Future browser**: if a third browser is added (Edge MV3, Safari Web
  Extension), the parity contract extends naturally to N manifests by
  pairwise comparison. Not in scope for this bug.

## Out Of Scope

Already enumerated in spec.md. No changes to lint script, no
package-extension script changes, no manifest content changes (the live
manifests are already in lockstep — the gap is enforcement, not content).

## Risk

- Low: a pure additive test file in `internal/web/`. No runtime path,
  no manifest edits. Worst case: a future contributor finds the
  adversarial error messages annoying and weakens them; the live-file
  test still asserts parity holds. The adversarial messages cite
  GAP-F01 and GAP-F03 by ID so the history is discoverable.
