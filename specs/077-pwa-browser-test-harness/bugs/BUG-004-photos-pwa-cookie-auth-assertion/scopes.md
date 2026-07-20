# Scopes: BUG-077-004 Photos PWA Cookie-Auth Assertion

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Align the Photos PWA E2E Auth Contract

**Status:** In Progress
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

- [ ] `TP-BUG077004-000` - Pre-fix root-package regression is RED because the stale `Authorization` assertion rejects the served cookie-auth wizard.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG077004-001` / BUG-077-004-SCN-001: served Photos wizard uses the HttpOnly session cookie, preserving connector endpoints and included-album wiring).
- [ ] `TP-BUG077004-002` / BUG-077-004-SCN-002 - Omitted cookies fail the regression through an explicit adversarial `credentials: "omit"` assertion.
- [ ] Broader E2E regression suite passes (`TP-BUG077004-003`: complete root E2E package).
- [ ] `TP-BUG077004-004` - Focused Go regression passes.
- [ ] `TP-BUG077004-005` - Configuration check passes.
- [ ] `TP-BUG077004-006` - Lint passes with no warnings.
- [ ] `TP-BUG077004-007` - Format check passes.
- [ ] `TP-BUG077004-008` - Adversarial regression guard passes with no silent-pass or tautological patterns.
- [ ] `TP-BUG077004-009` - Artifact lint passes.
- [ ] `TP-BUG077004-010` - Traceability guard passes.
- [ ] `TP-BUG077004-011` - Implementation-reality scan passes.
- [ ] `TP-BUG077004-012` - State-transition guard records the exact remaining owner-routed findings.
- [ ] Endpoint, payload, live API, and Immich-provider assertions remain intact.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] Validate-owned certification records the strongest evidence-supported state.

Test Plan rows: 13. Matching TP-labeled DoD items: 13.