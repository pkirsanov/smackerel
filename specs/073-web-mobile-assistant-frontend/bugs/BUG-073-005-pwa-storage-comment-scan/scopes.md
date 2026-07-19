# Scopes: BUG-073-005 - Comment-aware PWA storage scan

## Scope 1: Share executable-source inspection across unit and live E2E guards

**Status:** In Progress

**Scope-Kind:** test-infrastructure

### Gherkin Scenarios

```gherkin
Scenario: Policy comments do not fail the live route test
  Given assistant.js comments name localStorage as forbidden
  When the served executable source is inspected
  Then the comment token is ignored and the test passes

Scenario: Executable browser storage access is rejected
  Given JavaScript calls localStorage.getItem outside a comment
  When the shared scanner and forbidden-pattern check run
  Then the executable reference is reported

Scenario: Comment markers inside strings remain source text
  Given JavaScript contains an https URL and escaped quotes
  When comments are removed
  Then string content and following executable code remain intact
```

### Implementation Plan

1. Capture the current false-positive RED output.
2. Extract a reusable lexical JavaScript comment-removal helper.
3. Migrate existing PWA guards and the served-route E2E to the helper.
4. Add adversarial helper tests that detect real executable storage access.
5. Run focused live E2E, web PWA units, impacted units, and quality gates.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| SCN-BUG073005-001 | e2e-api | `tests/e2e/assistant/web_pwa_chat_e2e_test.go` | Policy comments do not fail the live route test | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run 'TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09'` | Yes |
| SCN-BUG073005-002 | unit | `internal/testsupport/jssource/comments_test.go` | Executable browser storage access is rejected | `./smackerel.sh test unit --go --go-run 'TestWithoutComments|TestWebAssistantStorageGuard' --verbose` | No |
| SCN-BUG073005-003 | unit | `internal/testsupport/jssource/comments_test.go` | Comment markers inside strings remain source text | same focused scanner unit command | No |
| Assistant package order | e2e-api | `tests/e2e/assistant/` | Entire assistant package executes in package order | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| Impacted units | unit | `internal/testsupport/jssource/`, `web/pwa/tests/`, `ml/tests/` | Full Go and Python regression lanes | `./smackerel.sh test unit --go`; `./smackerel.sh test unit --python` | No |
| Quality gates | guard | changed files and packet | Check, lint, format, regression and packet gates | repo CLI plus Bubbles guards | No |

### Definition of Done

- [ ] Pre-fix live E2E is RED on comment-only storage tokens.
- [ ] SCN-BUG073005-001 - Policy comments do not fail the live route test: the shared comment-aware helper is used by unit and live source scans.
- [ ] SCN-BUG073005-002 - Executable browser storage access is rejected: adversarial tests retain and detect a real executable forbidden API call.
- [ ] SCN-BUG073005-003 - Comment markers inside strings remain source text: line/block comments are ignored without corrupting strings or templates.
- [ ] Served-route E2E remains live and passes against the disposable stack.
- [ ] Assistant package E2E passes in package order.
- [ ] Impacted Go and Python units pass.
- [ ] Check, lint, format, regression guard, and packet gates pass.

All items remain unchecked until current-session execution evidence is recorded.
