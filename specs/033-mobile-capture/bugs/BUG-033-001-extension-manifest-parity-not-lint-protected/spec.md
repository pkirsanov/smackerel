# BUG-033-001 — Extension manifest parity not lint-protected

| Field | Value |
|-------|-------|
| Parent spec | `specs/033-mobile-capture/` |
| Discovered by | Sweep `sweep-2026-05-23-r30` round 9 (`devops-to-doc` mapped from `devops` trigger) |
| Discovered at HEAD | `a910f952fe55a506b85be6aea558ee0a70deb712` |
| Severity | medium |
| Class | devops · web-asset contract drift · cross-browser parity |
| Status | resolved |

## Problem Statement

Spec 033 ships a browser extension that targets two browsers from two
manifest files in `web/extension/`:

- `manifest.json` — Chrome MV3 (the Chrome-distributed package)
- `manifest.firefox.json` — Firefox MV2 (the Firefox-distributed package)

The two manifests describe the same product, on the same backend, with the
same scopes — but the MV3 and MV2 schemas express the same surfaces
differently:

- Chrome MV3 lists API permissions in `permissions` and URL match patterns
  in `host_permissions`; Firefox MV2 merges both into `permissions`.
- Chrome MV3 stores CSP under `content_security_policy.extension_pages`
  (a dict); Firefox MV2 stores CSP under `content_security_policy` (a
  flat string).

These schema differences mean a contributor editing one manifest can
silently leave the other behind. Spec 033's own gaps probe documented
this exact failure mode twice:

- **GAP-F01**: Firefox manifest was missing the `alarms` permission that
  was added to Chrome for offline-flush retries. Periodic flush silently
  did not fire on Firefox.
- **GAP-F03**: Chrome and Firefox manifests disagreed on
  `content_security_policy` `object-src` (`'self'` on one, `'none'` on
  the other) until unified by hand to `'none'` for defense-in-depth
  parity with the PWA.

Both gaps were fixed by hand in spec 033, but no lint or test enforces
the parity invariant. The next time a contributor adds a permission, a
host pattern, or tightens CSP for one browser, the other will drift
silently. The round 9 devops probe surfaced this as a hardening gap.

```json
// web/extension/manifest.json (Chrome MV3, current)
{
  "manifest_version": 3,
  "permissions": ["storage", "contextMenus", "notifications", "activeTab", "alarms"],
  "host_permissions": ["https://*/api/*", "http://*/api/*"],
  "content_security_policy": {
    "extension_pages": "script-src 'self'; object-src 'none'"
  }
}
```

```json
// web/extension/manifest.firefox.json (Firefox MV2, current)
{
  "manifest_version": 2,
  "permissions": [
    "storage", "contextMenus", "notifications", "activeTab", "alarms",
    "https://*/api/*", "http://*/api/*"
  ],
  "content_security_policy": "script-src 'self'; object-src 'none'"
}
```

The two manifests are currently in lockstep. The bug is the absence of
machine-enforced parity, not a present drift.

## Why It Matters

Cross-browser parity is a Build-Once Deploy-Many concern at the web-asset
layer: `./smackerel.sh package extension` produces both packages from the
same `web/extension/` source tree, and operators distribute both side by
side. Asymmetric permissions or asymmetric CSP cause real, observable
behaviour differences in production:

- Missing API permission on one browser → the feature silently does not
  fire on that browser (GAP-F01 root cause).
- Missing host pattern on one browser → cross-origin capture fetch
  silently fails on that browser (GAP-F01 root cause).
- Looser CSP on one browser → defense-in-depth posture is weaker on that
  browser even though the product surface is identical (GAP-F03 root
  cause).

`scripts/runtime/web-validate.sh` already checks **version** parity
between the two manifests. The other parity dimensions (name,
description, API permissions, host patterns, CSP `object-src`) have no
machine enforcement today. The class of bugs has historical precedent
(two GAP-F entries against this very spec); the absence of enforcement
is a devops drift gap.

## Scenarios (Gherkin)

### SCN-033-B001 — Live extension manifests pass the parity contract

```gherkin
Given the Chrome MV3 manifest at web/extension/manifest.json
And   the Firefox MV2 manifest at web/extension/manifest.firefox.json
When  the parity contract test parses both manifests
Then  the user-visible name MUST be identical across both manifests
And   the version MUST be identical
And   the description MUST be identical
And   every non-host API permission in Chrome.permissions MUST appear in Firefox.permissions
And   every URL match pattern in Chrome.host_permissions MUST appear (merged) in Firefox.permissions
And   the CSP object-src directive MUST match across both manifests
And   the CSP object-src directive MUST be 'none' in both (the GAP-F03 fixed value).
```

### SCN-033-B002 — Adversarial parity drift is rejected

```gherkin
Given the parity contract test from SCN-033-B001
When  a developer adds an API permission, a host pattern, or tightens CSP for one browser
But   forgets to mirror the change to the other manifest
Then  the contract test MUST fail with a clear, actionable error
And   the failure MUST name the specific parity surface that drifted
And   the failure MUST name the specific permission, pattern, or directive that mismatched
And   adversarial coverage MUST exist for at minimum: missing API permission, missing host pattern,
      mismatched CSP object-src, mismatched name, mismatched version, mismatched description.
```

## Out Of Scope

- The PWA `web/pwa/sw.js` cache-busting story. Already correctly handled
  by `internal/api/pwa.go`'s init-time content-hash injection that
  rewrites `CACHE_NAME` in the served service worker (verified against
  the live file during the round 9 probe).
- Extension version-bump policy (semver, channel pinning, signed-update
  distribution). Owned by the package-extension script and the
  self-hosted-distribution contract; not a parity invariant.
- Chrome web store / Firefox AMO publisher metadata (icons, screenshots,
  store-listing copy). Out of repo scope — those live in publisher
  consoles, not in the source tree.
- Browser-specific keys that have no Chrome equivalent
  (`browser_specific_settings.gecko`) and MV3-only keys
  (`background.service_worker`, `host_permissions`). The contract MUST
  only assert parity for surfaces that have a counterpart in both
  schemas.

## Acceptance Criteria

1. A new Go contract test at `internal/web/extension_parity_contract_test.go`
   parses `web/extension/manifest.json` and
   `web/extension/manifest.firefox.json` and asserts parity for: name,
   version, description, API permissions, host patterns, and CSP
   `object-src`.
2. The contract test MUST include at minimum 7 adversarial sub-tests
   covering (one per surface): missing alarms permission in Firefox
   (the GAP-F01 regression), missing host pattern in Firefox (also
   GAP-F01), mismatched CSP `object-src` (the GAP-F03 regression),
   mismatched name, mismatched version, mismatched description, and
   extra permission in Chrome without Firefox mirror.
3. `./smackerel.sh test unit --go` is green with the new test included.
4. `bash .github/bubbles/scripts/artifact-lint.sh` is green for this bug
   packet.
5. `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh` is
   green for this bug packet.
6. `bash scripts/runtime/state-transition-guard.sh` is green for the
   bug's `open → resolved` transition.
7. The commit that closes this bug MUST NOT contaminate the live
   spec 055 (notification source / ntfy adapter) WIP. Verified by
   `git diff --cached --name-status` excluding every spec 055 file.

## Product Principle Alignment

This is a **Principle 9 ("Design For Restart, Not Perfection")** fix at
the contributor-experience layer: a future contributor should not need
to discover, by user-visible Firefox regression, that an API permission
or CSP directive added to Chrome was silently missed in Firefox. The
parity contract surfaces the drift at unit-test time, the cheapest
possible point.

Secondarily, **Principle 1 ("Observe First, Ask Second")** at the agent
layer: this bug was found by routine devops probing
(`mode: devops-to-doc`, sweep round 9), not by user report — exactly the
loop the sweep exists to close.
