# Scopes: BUG-077-004 Photos PWA Cookie-Auth Assertion

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Align the Photos PWA E2E Auth Contract

**Status:** Done
**Priority:** P1
**Depends On:** None
**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: Served Photos wizard uses the HttpOnly session cookie (BUG-077-004-SCN-001)
  Given the disposable Smackerel stack serves photo-library-add.js
  When the Photos wizard E2E contract inspects the served script
  Then the script contains credentials set to same-origin
  And both connector endpoints and included-album wiring remain present

Scenario: Omitted cookies fail the regression (BUG-077-004-SCN-002)
  Given the Photos wizard depends on an HttpOnly same-origin session cookie
  When the served script contains credentials set to omit
  Then the E2E contract fails
```

### Change Boundary

**Allowed file families:**

- `tests/e2e/photos_pwa_test.go`
- `specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion/**`

**Excluded surfaces:**

- `web/pwa/**` production assets
- Authentication middleware and session storage
- Config, deployment, release-train, and secret surfaces

### Implementation Files

- `tests/e2e/photos_pwa_test.go`
- `specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion/**`

### Test Plan

| Test Type | ID | Scenario | Category | File/Location | Description | Command | Live System |
|----|----------|-----------|----------|---------------|-------------|---------|-------------|
| Regression E2E RED | `TP-BUG077004-000` | Pre-fix root-package failure | `e2e-api` | `tests/e2e/photos_pwa_test.go` | Proves the stale `Authorization` assertion fails against the served cookie-auth wizard | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e` | Yes |
| Regression E2E: BUG-077-004-SCN-001 | `TP-BUG077004-001` | Served Photos wizard uses the HttpOnly session cookie | `e2e-api` | `tests/e2e/photos_pwa_test.go` | Requires `credentials: "same-origin"`, connector endpoints, included-album wiring, and the live connector API | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$'` | Yes |
| Regression E2E: BUG-077-004-SCN-002 | `TP-BUG077004-002` | Omitted cookies fail the regression | `e2e-api` | `tests/e2e/photos_pwa_test.go` | Fails if the served script contains `credentials: "omit"` | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$'` | Yes |
| Broader E2E regression | `TP-BUG077004-003` | Root package regression | `e2e-api` | `tests/e2e/*.go` | Confirms all root E2E scenarios pass after the repair | complete root package execution through `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e` | Yes |
| Unit regression | `TP-BUG077004-004` | Focused Go regression | `unit` | impacted Go packages | Confirms neighboring Photos PWA, HTTP adapter, and synthesis paths remain green | `./smackerel.sh test unit --go --go-run 'TestPhotosPWA|TestSynthesisExtractResponse|TestHTTPAdapter'` | No |
| Static quality | `TP-BUG077004-005` | Configuration check | `guard` | repository | Validates SST and generated config | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` | No |
| Static quality | `TP-BUG077004-006` | Lint | `guard` | repository | Validates repository lint with no warnings | `./smackerel.sh lint` | No |
| Static quality | `TP-BUG077004-007` | Format | `guard` | repository | Validates repository formatting | `./smackerel.sh format --check` | No |
| Static quality | `TP-BUG077004-008` | Adversarial regression guard | `guard` | `tests/e2e/photos_pwa_test.go` | Proves the bugfix test has an adversarial omitted-cookie case and no bailout | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/photos_pwa_test.go` | No |
| Governance | `TP-BUG077004-009` | Artifact lint | `artifact` | BUG-077-004 packet | Validates packet template and evidence structure | `bash .github/bubbles/scripts/artifact-lint.sh specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion` | No |
| Governance | `TP-BUG077004-010` | Traceability | `artifact` | BUG-077-004 packet | Validates Gherkin, scenario manifest, tests, and DoD links | `bash .github/bubbles/scripts/traceability-guard.sh specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion` | No |
| Governance | `TP-BUG077004-011` | Implementation reality | `artifact` | BUG-077-004 packet | Validates referenced implementation files contain no stub/fake/default regressions | `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion --verbose` | No |
| Governance | `TP-BUG077004-012` | State-transition guard | `artifact` | BUG-077-004 packet | Records exact remaining owner-routed findings | `bash .github/bubbles/scripts/state-transition-guard.sh specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion` | No |

### Definition of Done

- [x] `TP-BUG077004-000` - Pre-fix root-package regression is RED because the stale `Authorization` assertion rejects the served cookie-auth wizard. → Evidence: [report.md](report.md) "### RED — revert-reverify" — reverting to the exact pre-fix `"Authorization"` assertion (`EXACT_PREFIX_MATCH` vs `b4761988~1`) makes `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` FAIL (`photos_pwa_test.go:29 photo-library-add.js missing "Authorization"`, `FAIL: go-e2e (exit=1)`, `BUG004_RED_EXIT=1`), proving the fix line is load-bearing.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG077004-001` / BUG-077-004-SCN-001: served Photos wizard uses the HttpOnly session cookie, preserving connector endpoints and included-album wiring). → Evidence: [report.md](report.md) "### GREEN + complete root E2E package" — `--- PASS: TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI (0.01s)` after byte-exact restore; the test asserts `credentials: "same-origin"`, both connector endpoints (`/v1/photos/connectors/test`, `/v1/photos/connectors`), `included_albums` wiring, and the live connector API — all GREEN. [scenario-manifest.json](scenario-manifest.json) maps SCN-001 → this test.
- [x] `TP-BUG077004-002` / BUG-077-004-SCN-002 - Omitted cookies fail the regression through an explicit adversarial `credentials: "omit"` assertion. → Evidence: [report.md](report.md) "### Code Diff Evidence" (committed adversarial guard: `if strings.Contains(addJS, \`credentials: "omit"\`) { t.Fatal(...) }`) + [report.md](report.md) "## Guards & Quality Gates" `RQG_BUGFIX_EXIT=0` (Adversarial signal detected). [scenario-manifest.json](scenario-manifest.json) maps SCN-002 → this test.
- [x] Broader E2E regression suite passes (`TP-BUG077004-003`: complete root E2E package). → Evidence: [report.md](report.md) "### GREEN + complete root E2E package" — the complete root `tests/e2e` package (128 tests via anchored regex) executed this session; all 13 Photos tests + 127/128 root tests are GREEN. The single failure is the pre-existing FOREIGN `buildvcs` VCS-stamping environment condition in the unrelated HTTP-adapter missing-required-key e2e (`spec062_http_missing_key_test.go`), dispositioned in [report.md](report.md) "## Discovered Issues (Gate G095)" DI-077-004-01 — not a product regression.
- [x] `TP-BUG077004-004` - Focused Go regression passes. → Evidence: [report.md](report.md) "### Focused Go unit regression" — `UNIT_FOCUSED_EXIT=0`; `httpadapter` + `pipeline` matched and ran GREEN, all packages `ok`.
- [x] `TP-BUG077004-005` - Configuration check passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `CHECK_EXIT=0` (Config in sync with SST; scenario-lint OK 17/0).
- [x] `TP-BUG077004-006` - Lint passes with no warnings. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `LINT_EXIT=0` (ruff "All checks passed!"; web validation passed).
- [x] `TP-BUG077004-007` - Format check passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `FORMAT_EXIT=0` (75 files already formatted).
- [x] `TP-BUG077004-008` - Adversarial regression guard passes with no silent-pass or tautological patterns. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `RQG_PLAIN_EXIT=0` (0 violations, 0 warnings) + `RQG_BUGFIX_EXIT=0` (Adversarial signal detected on `tests/e2e/photos_pwa_test.go`).
- [x] `TP-BUG077004-009` - Artifact lint passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `ALINT_EXIT=0` (Artifact lint PASSED).
- [x] `TP-BUG077004-010` - Traceability guard passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `TRACE_EXIT=0` (2 scenarios → 13 rows; G057/G068 2/2; PASSED, 0 warnings).
- [x] `TP-BUG077004-011` - Implementation-reality scan passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `REALITY_EXIT=0` (0 violations, 1 file scanned).
- [x] `TP-BUG077004-012` - State-transition guard records the exact remaining owner-routed findings. → Evidence: [report.md](report.md) "## Guards & Quality Gates" `GUARD_EXIT=0` (verdict PASS, `failedGateIds: []`).
- [x] Endpoint, payload, live API, and Immich-provider assertions remain intact. → Evidence: [report.md](report.md) "### GREEN + complete root E2E package" — `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` PASS exercises the connector endpoints, `included_albums` payload, the live `GET /v1/photos/connectors` (200 + JSON decode), and asserts `parsed.Connectors[0].Provider == "immich"`; the committed diff ([report.md](report.md) "### Code Diff Evidence") changed only the JS-source auth assertion and left those blocks intact.
- [x] Change Boundary is respected and zero excluded file families were changed. → Evidence: [report.md](report.md) "## Change Boundary" + "### Code Diff Evidence" — the only delta is `tests/e2e/photos_pwa_test.go` (committed `b4761988`, +4/-1) plus this packet; production `web/pwa/photo-library-add.js`, auth/session, config, deploy, and secrets are untouched; the RED revert was restored byte-exact (`git diff --stat HEAD` empty).
- [x] Validate-owned certification records the strongest evidence-supported state. → Evidence: [state.json](state.json) `certification.certifierAgent = bubbles.validate`, `certification.certificationReadiness = certified`, `certification.scopeProgress` (13/13 Photos surface within 16 DoD), and `certification.certifiedCompletedPhases` records the full 8-phase bugfix-fastlane claim set; terminal `certification.status = done` + `certifiedAt` are stamped only by the validate-owned promote commit after the planning-truth commit (G088). [report.md](report.md) "### Validation Evidence".

Test Plan rows: 13. Matching TP-labeled DoD items: 13.