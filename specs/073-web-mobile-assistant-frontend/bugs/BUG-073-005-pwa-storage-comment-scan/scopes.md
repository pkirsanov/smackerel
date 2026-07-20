# Scopes: BUG-073-005 - Comment-aware PWA storage scan

## Scope 1: Share executable-source inspection across unit and live E2E guards

**Status:** In Progress

**Scope-Kind:** runtime-behavior

### Gherkin Scenarios

```gherkin
Scenario: Policy comments do not fail the live route test (SCN-BUG073005-001)
  Given assistant.js comments name localStorage as forbidden
  When the served executable source is inspected
  Then the comment token is ignored and the test passes

Scenario: Executable browser storage access is rejected (SCN-BUG073005-002)
  Given JavaScript calls localStorage.getItem outside a comment
  When the shared scanner and forbidden-pattern check run
  Then the executable reference is reported

Scenario: Comment markers inside strings remain source text (SCN-BUG073005-003)
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

### Implementation Files

- `internal/testsupport/jssource/comments.go`
- `internal/testsupport/jssource/comments_test.go`
- `tests/e2e/assistant/web_pwa_chat_e2e_test.go`
- `web/pwa/tests/assistant_storage_guard_test.go`
- `specs/073-web-mobile-assistant-frontend/bugs/BUG-073-005-pwa-storage-comment-scan/`

### Change Boundary

**Allowed file families:** the four scanner consumer/test files listed under Implementation Files and this BUG-073-005 packet.

**Excluded surfaces:** production PWA JavaScript, auth/session/storage behavior, unrelated E2E packages, runtime handlers, config, deployment, secrets, release-train artifacts, and any scanner semantics beyond lexical comment removal that preserves executable source.

### Shared Infrastructure Impact Sweep

- **Downstream contracts:** line/block comment removal, preservation of quoted strings/templates/escaped quotes/URLs, detection of executable storage access, served-asset inspection, and PWA unit guard behavior.
- **Blast radius:** the shared `jssource.WithoutComments` helper and its two declared first-party scanner consumers only; production browser code and runtime storage contracts remain unchanged.
- **Independent canary:** run focused helper/PWA scanner units and the served-route live E2E before the broader assistant package selector.
- **Restore path:** revert the two helper files and restore both scanner consumers to their prior local logic; adversarial helper tests must prove executable forbidden calls remain detectable before broad execution.

### Test Plan

| Test Type | ID | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|---|
| Pre-fix focused regression | `TP-BUG073005-000` | `e2e-api` | `tests/e2e/assistant/web_pwa_chat_e2e_test.go` | The served-route test fails on a comment-only `localStorage` token before scanner reuse | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09$'` | Yes |
| Regression E2E: SCN-BUG073005-001 | `TP-BUG073005-001` | `e2e-api` | `tests/e2e/assistant/web_pwa_chat_e2e_test.go` | The live served executable source ignores policy comments while retaining forbidden executable access checks | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09$'` | Yes |
| Unit: SCN-BUG073005-002 | `TP-BUG073005-002` | `unit` | `internal/testsupport/jssource/comments_test.go`, `web/pwa/tests/assistant_storage_guard_test.go` | Adversarial scanner tests retain and reject real executable browser storage access | `./smackerel.sh test unit --go --go-run 'TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess|TestWebAssistantStorageGuard' --verbose` | No |
| Unit: SCN-BUG073005-003 | `TP-BUG073005-003` | `unit` | `internal/testsupport/jssource/comments_test.go` | Lexical removal preserves URLs, escaped quotes, templates, and following executable code | `./smackerel.sh test unit --go --go-run '^TestWithoutComments_PreservesStringsTemplatesAndFollowingCode$' --verbose` | No |
| Fixture Canary: scanner unit consumers | `TP-BUG073005-004A` | `unit` | `internal/testsupport/jssource/comments_test.go`, `web/pwa/tests/assistant_storage_guard_test.go` | Independent helper-consumer unit canary validates false-positive removal and true-positive retention before live/broad execution | `./smackerel.sh test unit --go --go-run 'TestWithoutComments|TestWebAssistantStorageGuard' --verbose` | No |
| Fixture Canary: live served route | `TP-BUG073005-004B` | `e2e-api` | `tests/e2e/assistant/web_pwa_chat_e2e_test.go` | Independent live canary validates the served asset after unit canaries and before the broad assistant package | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09$'` | Yes |
| Broader E2E regression | `TP-BUG073005-005` | `e2e-api` | `tests/e2e/assistant/` | The complete assistant package executes in package order and preserves the repaired route scenario | exact anchored selector generated from package test declarations and passed to `./smackerel.sh test e2e --go-run` | Yes |
| Impacted Go units | `TP-BUG073005-006` | `unit` | `internal/testsupport/jssource/`, `web/pwa/tests/` | Full Go regression lane covers every shared scanner consumer | `./smackerel.sh test unit --go` | No |
| Impacted Python units | `TP-BUG073005-007` | `unit` | `ml/tests/` | Full Python regression lane remains green | `./smackerel.sh test unit --python` | No |
| Configuration check | `TP-BUG073005-008` | `guard` | Repository config contract | SST and generated config remain valid | `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` | No |
| Lint | `TP-BUG073005-009` | `guard` | Changed files | Repository lint reports no warnings | `./smackerel.sh lint` | No |
| Format | `TP-BUG073005-010` | `guard` | Changed Go and packet files | Repository format check remains clean | `./smackerel.sh format --check` | No |
| Adversarial regression guard | `TP-BUG073005-011` | `guard` | `internal/testsupport/jssource/comments_test.go`, `tests/e2e/assistant/web_pwa_chat_e2e_test.go` | Required regressions contain no bailout or tautological bugfix pattern | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/testsupport/jssource/comments_test.go tests/e2e/assistant/web_pwa_chat_e2e_test.go` | No |
| Artifact lint | `TP-BUG073005-012` | `artifact` | BUG-073-005 packet | Packet template and evidence structure remain valid | `bash .github/bubbles/scripts/artifact-lint.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-005-pwa-storage-comment-scan` | No |
| Traceability | `TP-BUG073005-013` | `artifact` | BUG-073-005 packet | Gherkin, scenario manifest, tests, and DoD remain linked | `bash .github/bubbles/scripts/traceability-guard.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-005-pwa-storage-comment-scan` | No |
| Implementation reality | `TP-BUG073005-014` | `artifact` | BUG-073-005 packet | Referenced implementation files contain no stub/fake/default regressions | `bash .github/bubbles/scripts/implementation-reality-scan.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-005-pwa-storage-comment-scan --verbose` | No |
| State-transition guard | `TP-BUG073005-015` | `artifact` | BUG-073-005 packet | The packet reports exact remaining owner-routed findings | `bash .github/bubbles/scripts/state-transition-guard.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-005-pwa-storage-comment-scan` | No |

### Definition of Done

- [ ] `TP-BUG073005-000` - Pre-fix live E2E is RED on comment-only storage tokens.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG073005-001` / SCN-BUG073005-001: policy comments do not fail the live route test because the shared comment-aware helper is used by the served-source scan).
- [ ] `TP-BUG073005-002` / SCN-BUG073005-002 - Executable browser storage access is rejected: adversarial tests retain and detect a real executable forbidden API call.
- [ ] `TP-BUG073005-003` / SCN-BUG073005-003 - Comment markers inside strings remain source text: line/block comments are ignored without corrupting strings or templates.
- [ ] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (`TP-BUG073005-004A`).
- [ ] `TP-BUG073005-004B` - Live served-route canary passes after the scanner unit canary and before broad package execution.
- [ ] Rollback or restore path for shared infrastructure changes is documented and verified.
- [ ] Broader E2E regression suite passes (`TP-BUG073005-005`: complete assistant package in package order).
- [ ] `TP-BUG073005-006` - Impacted Go unit suite passes.
- [ ] `TP-BUG073005-007` - Impacted Python unit suite passes.
- [ ] `TP-BUG073005-008` - Configuration check passes.
- [ ] `TP-BUG073005-009` - Lint passes with no warnings.
- [ ] `TP-BUG073005-010` - Format check passes.
- [ ] `TP-BUG073005-011` - Adversarial regression guard passes with no silent-pass or tautological patterns.
- [ ] `TP-BUG073005-012` - Artifact lint passes.
- [ ] `TP-BUG073005-013` - Traceability guard passes.
- [ ] `TP-BUG073005-014` - Implementation-reality scan passes.
- [ ] `TP-BUG073005-015` - State-transition guard records the exact remaining owner-routed findings.
- [ ] Change Boundary is respected and zero excluded file families were changed.

All items remain unchecked until current-session execution evidence is recorded.
