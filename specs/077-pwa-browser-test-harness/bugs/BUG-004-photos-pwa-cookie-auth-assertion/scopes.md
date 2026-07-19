# Scopes: BUG-077-004 Photos PWA Cookie-Auth Assertion

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Align the Photos PWA E2E Auth Contract

**Status:** In Progress
**Priority:** P1
**Depends On:** None
**Scope-Kind:** test-harness bugfix

### Gherkin Scenarios

```gherkin
Scenario: Served Photos wizard uses the HttpOnly session cookie
  Given the disposable Smackerel stack serves photo-library-add.js
  When the Photos wizard E2E contract inspects the served script
  Then the script contains credentials set to same-origin
  And both connector endpoints and included-album wiring remain present

Scenario: Omitted cookies fail the regression
  Given the Photos wizard depends on an HttpOnly same-origin session cookie
  When the served script contains credentials set to omit
  Then the E2E contract fails
```

### Change Boundary

Allowed:

- `tests/e2e/photos_pwa_test.go`
- `specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion/**`

Excluded:

- `web/pwa/**` production assets
- Authentication middleware and session storage
- Config, deployment, release-train, and secret surfaces

### Implementation Files

- `tests/e2e/photos_pwa_test.go`
- `specs/077-pwa-browser-test-harness/bugs/BUG-004-photos-pwa-cookie-auth-assertion/**`

### Test Plan

| ID | Scenario | Test Type | Category | File/Location | Description | Command | Live System |
|----|----------|-----------|----------|---------------|-------------|---------|-------------|
| TP-01 | Served Photos wizard uses the HttpOnly session cookie | Focused regression | `e2e-api` | `tests/e2e/photos_pwa_test.go` | Requires `credentials: "same-origin"`, connector endpoints, included-album wiring, and the live connector API | `./smackerel.sh test e2e --go-run '^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$'` | Yes |
| TP-02 | Omitted cookies fail the regression | Adversarial regression | `e2e-api` | `tests/e2e/photos_pwa_test.go` | Fails if the served script contains `credentials: "omit"` | `./smackerel.sh test e2e --go-run '^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$'` | Yes |
| TP-03 | Root package regression | Root package regression | `e2e-api` | `tests/e2e/*.go` | Confirms all root E2E scenarios pass after the repair | complete root package execution through `./smackerel.sh test e2e` | Yes |
| TP-04 | Focused Go regression | Focused Go regression | `unit` | impacted Go packages | Confirms neighboring HTTP adapter and synthesis paths remain green | `./smackerel.sh test unit --go --go-run 'TestPhotosPWA|TestSynthesisExtractResponse|TestHTTPAdapter'` | No |
| TP-05 | Static quality | Static quality | `lint/format/check` | repository | Validates config, lint, format, and packet gates | repo-standard commands | No |

### Definition of Done

- [ ] TP-01 served Photos wizard uses the HttpOnly session cookie: the focused live regression requires same-origin credentials and passes after the before-fix failure.
- [ ] TP-02 omitted cookies fail the regression: the adversarial assertion rejects `credentials: "omit"`.
- [ ] TP-03 complete root E2E package passes.
- [ ] TP-04 focused Go regression passes.
- [ ] TP-05 check, lint, format, artifact, traceability, reality, and regression guards pass.
- [ ] Served Photos wizard uses the HttpOnly session cookie, preserving connector endpoints and included-album wiring.
- [ ] Omitted cookies fail the regression through an explicit adversarial assertion.
- [ ] Endpoint, payload, live API, and Immich-provider assertions remain intact.
- [ ] Change Boundary is respected with zero production-code changes.
- [ ] Validate-owned certification records the strongest evidence-supported state.

Test Plan rows: 5. Matching TP-labeled DoD items: 5.