# Scopes — BUG-033-001 — Extension manifest parity not lint-protected

## Scope BUG-033-001-S1 — Manifest parity contract test + adversarial regression suite

**Status:** Done

### Gherkin Scenarios (inlined from spec.md for traceability-guard coverage)

```gherkin
Scenario: SCN-033-B001 Live extension manifests pass the parity contract
  Given the Chrome MV3 manifest at web/extension/manifest.json
  And   the Firefox MV2 manifest at web/extension/manifest.firefox.json
  When  the parity contract test parses both manifests
  Then  the user-visible name MUST be identical across both manifests
  And   the version MUST be identical
  And   the description MUST be identical
  And   every non-host API permission in Chrome.permissions MUST appear in Firefox.permissions
  And   every URL match pattern in Chrome.host_permissions MUST appear (merged) in Firefox.permissions
  And   the CSP object-src directive MUST match across both manifests
  And   the CSP object-src directive MUST be 'none' in both (the GAP-F03 fixed value)
```

```gherkin
Scenario: SCN-033-B002 Adversarial parity drift is rejected by the contract test
  Given the parity contract test from SCN-033-B001
  When  a developer adds an API permission, a host pattern, or tightens CSP for one browser
  But   forgets to mirror the change to the other manifest
  Then  the contract test MUST fail with a clear, actionable error
  And   the failure MUST name the specific parity surface that drifted
  And   the failure MUST name the specific permission, pattern, or directive that mismatched
  And   adversarial coverage MUST exist for at minimum missing API permission, missing host pattern, mismatched CSP object-src, mismatched name, mismatched version, mismatched description
```

### Work Items

1. Add `internal/web/extension_parity_contract_test.go` (new file,
   `package web`) that parses `web/extension/manifest.json` (Chrome MV3)
   and `web/extension/manifest.firefox.json` (Firefox MV2) and asserts
   six parity invariants: manifest_version preconditions, name,
   version, description, API permissions (normalised against
   URL-pattern grammar to separate host patterns from API perms),
   host patterns (Chrome `host_permissions` ⇔ Firefox merged
   permissions), and CSP `object-src` (extracted from Chrome dict and
   Firefox flat-string forms).
2. Add adversarial sub-tests proving each parity invariant is
   non-tautological, at minimum:
   - Missing `alarms` permission in Firefox (GAP-F01 regression).
   - Missing host pattern (`http://*/api/*`) in Firefox (GAP-F01 root cause).
   - Mismatched CSP `object-src` (GAP-F03 regression).
   - Mismatched name.
   - Mismatched version.
   - Mismatched description.
   - Extra permission in Chrome without Firefox mirror.
3. Add a baseline-sanity sub-test that asserts the canonical in-memory
   pair used by the adversarial sub-tests passes the parity contract.
4. Run the focused test (`go test -v -run TestExtensionManifestParity
   ./internal/web/...`) and the full package gate (`go test
   ./internal/web/...`) and capture green output in `report.md`.
5. Append a "Devops Probe (devops-to-doc, SQS round 9, 2026-05-23)"
   section to `specs/033-mobile-capture/report.md` documenting the
   finding closure and citing this bug packet.

### Definition of Done

- [x] `internal/web/extension_parity_contract_test.go` exists with
      `package web` and parses both live manifest files using
      `encoding/json` + `runtime.Caller` for cwd-independent paths.

  Evidence: see `report.md` Step 1 (diff of the new file plus the
  package-level `go test ./internal/web/...` output naming the file).

- [x] (SCN-033-B001) `TestExtensionManifestParity_LiveFiles` is green
      against the live `web/extension/manifest.json` and
      `web/extension/manifest.firefox.json` — verifies the
      live-extension parity contract scenario.

  Evidence: see `report.md` Step 2 (`-run TestExtensionManifestParity`
  verbose output naming `TestExtensionManifestParity_LiveFiles` PASS).

- [x] (SCN-033-B002) At least 7 adversarial sub-tests cover one parity
      surface each — proving the adversarial-drift rejection scenario:
      `AdversarialMissingAlarmsInFirefox`,
      `AdversarialMissingHostPatternInFirefox`,
      `AdversarialMismatchedCSPObjectSrc`,
      `AdversarialMismatchedName`,
      `AdversarialMismatchedVersion`,
      `AdversarialExtraPermissionInChrome`,
      `AdversarialMismatchedDescription`.

  Evidence: see `report.md` Step 2 (each sub-test PASS line is in the
  verbose output; each adversarial error message names the drifted
  surface — `alarms`, `http://*/api/*`, `object-src`, `name`, `version`,
  `downloads`, `description`).

- [x] (SCN-033-B001, SCN-033-B002) A baseline-sanity sub-test
      (`TestExtensionManifestParity_CanonicalBaselinePasses`) proves
      the in-memory canonical pair used by adversarial mutations
      satisfies the parity contract — without it, both the live
      (SCN-033-B001) and the adversarial (SCN-033-B002) sub-tests
      would be vacuous.

  Evidence: see `report.md` Step 2.

- [x] The new test classifies as `unit` (no docker, no browser runtime,
      pure JSON parsing) and runs under `./smackerel.sh test unit --go`
      / `go test ./internal/web/...`.

  Evidence: see `report.md` Step 3 (package-level gate green at 0.083s
  — pure-CPU test runtime confirms no integration dependencies).

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior: SCN-033-B001 live extension manifests pass the parity contract and SCN-033-B002 adversarial parity drift is rejected by the contract test.
      Both scenarios are mapped to concrete test functions in
      `internal/web/extension_parity_contract_test.go` and execute on
      every `./smackerel.sh test unit --go` run. (Contract-only
      change with no runtime path — contract-test layer is the
      deepest applicable regression layer per Gate G028.)

  Evidence: see Test Plan below; the verbose output in `report.md`
  Step 2 maps each scenario to its concrete sub-tests.

- [x] Broader E2E regression suite passes: the change is guarded by the
      full `internal/web/...` package gate (`go test ./internal/web/...`),
      which runs the new contract test alongside all sibling web-handler
      and icons tests with zero regressions. (Contract-only change with
      no runtime path — package gate is the broader regression layer
      in scope.)

  Evidence: `report.md` Step 3 captures `ok github.com/smackerel/smackerel/internal/web 0.083s`
  and `ok github.com/smackerel/smackerel/internal/web/icons 0.005s` with
  zero regressions.

- [x] `bash .github/bubbles/scripts/artifact-lint.sh
      specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/`
      PASS at commit time.

  Evidence: see `report.md` Step 4 (artifact-lint output).

- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh
      specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/`
      PASS at commit time.

  Evidence: see `report.md` Step 4 (traceability-guard output).

- [x] `bash .github/bubbles/scripts/state-transition-guard.sh
      specs/033-mobile-capture/bugs/BUG-033-001-extension-manifest-parity-not-lint-protected/`
      PASS at commit time for the `open → resolved` transition.

  Evidence: see `report.md` Step 5 (state-transition-guard PASS output
  captured after artifact lint and contract test gates are green).

- [x] No changes touch unrelated WIP (live spec 055 notification source
      / ntfy adapter packet, the modified files under `cmd/core/`,
      `internal/api/`, `internal/config/`, `internal/notification/`,
      `internal/web/handler.go`, `internal/web/templates.go`,
      `scripts/commands/config.sh`, and the docs/config WIP). Verified
      by `git diff --cached --name-status` before commit, which is
      filtered to: this bug's 6 packet artifacts, the new
      `internal/web/extension_parity_contract_test.go`, the updated
      `specs/033-mobile-capture/report.md`, the updated
      `specs/033-mobile-capture/state.json`, and
      `.specify/memory/sweep-2026-05-23-r30.json`.

  Evidence: `git diff --cached --name-status` capture in `report.md`
  Step 5 (Code Diff Evidence section).

### Test Plan

| Scenario ID | Layer | Test File | Test Function(s) | Notes |
|-------------|-------|-----------|------------------|-------|
| SCN-033-B001 | Regression E2E (contract-layer, unit/Go) | `internal/web/extension_parity_contract_test.go` | `TestExtensionManifestParity_LiveFiles` | Live-file scenario-specific regression E2E: parses both live manifests and asserts all six parity invariants hold (name, version, description, API permissions, host patterns, CSP `object-src='none'`). Plus smoke checks that surface GAP-F01 (`alarms` present in both) and GAP-F03 (`object-src='none'` in both) regressions by name. |
| SCN-033-B002 | Regression E2E (contract-layer, unit/Go) | `internal/web/extension_parity_contract_test.go` | `TestExtensionManifestParity_AdversarialMissingAlarmsInFirefox`, `TestExtensionManifestParity_AdversarialMissingHostPatternInFirefox`, `TestExtensionManifestParity_AdversarialMismatchedCSPObjectSrc`, `TestExtensionManifestParity_AdversarialMismatchedName`, `TestExtensionManifestParity_AdversarialMismatchedVersion`, `TestExtensionManifestParity_AdversarialExtraPermissionInChrome`, `TestExtensionManifestParity_AdversarialMismatchedDescription` | Seven adversarial scenario-specific regression E2E sub-tests proving drift is caught for each parity surface; each error message names the drifted permission/pattern/directive (e.g., `alarms`, `http://*/api/*`, `object-src`, `name`, `version`, `downloads`, `description`). Plus `TestExtensionManifestParity_CanonicalBaselinePasses` as a non-vacuity sanity check on the in-memory baseline used by every adversarial mutation. |
| Broader regression | Regression E2E (web package gate, unit/Go) | `internal/web/...` (all `*_test.go` in the package) | full package run | Broader E2E regression suite coverage executed via `go test ./internal/web/...` — runs the new contract test alongside all sibling web-handler and icons tests (no regressions; full gate completes in <0.1s). |

This is a contract-and-test-only change with no runtime path, so the
regression E2E coverage lives at the Go contract-test layer (the same
enforcement layer as sibling `deploy/contract.yaml` and
`deploy/compose.deploy.yml` invariants under
`internal/deploy/external_images_contract_test.go`).
