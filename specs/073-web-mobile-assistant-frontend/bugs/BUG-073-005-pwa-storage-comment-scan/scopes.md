# Scopes: BUG-073-005 - Comment-aware PWA storage scan

## Scope 1: Share executable-source inspection across unit and live E2E guards

**Status:** Done

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

- [x] `TP-BUG073005-000` - Pre-fix live E2E is RED on comment-only storage tokens. → Evidence: [report.md](report.md) "RED: Pre-fix served-route live E2E (prior session)" — the served-route E2E FAILs with `web_pwa_chat_e2e_test.go:107: assistant.js must not reference forbidden auth surface "localStorage" (SCN-073-A11)` (exit 1) on the raw-source scan of the leading policy comment; [report.md](report.md) "Revert-Reverify … RED" re-establishes the identical false-positive class this session deterministically at the unit-guard layer (`UNIT_RED_REVERT_EXIT=1`).
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass (`TP-BUG073005-001` / SCN-BUG073005-001: policy comments do not fail the live route test because the shared comment-aware helper is used by the served-source scan). → Evidence: [report.md](report.md) "Live Served-Route E2E (current session)" — `--- PASS: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09` against the REAL served `/pwa/assistant.js` (`SERVED_E2E_EXIT=0`, clean teardown); the served-route scan now uses `executableJS := jssource.WithoutComments(js)` so the policy comments no longer trip the check.
- [x] `TP-BUG073005-002` / SCN-BUG073005-002 - Executable browser storage access is rejected: adversarial tests retain and detect a real executable forbidden API call. → Evidence: [report.md](report.md) "Revert-Reverify … RED" — the adversary `TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess` RETAINS the executable `localStorage.getItem("bearer")`, and the PWA guard adversary `TestWebAssistantStorageGuard_Adversarial_TP_073_06` PASSes; the revert proves a real executable access is still detected (`assistant_storage_guard_test.go:98` FAILs when comment-stripping is disabled), then GREEN on restore (`UNIT_GREEN_RESTORE_EXIT=0`). [report.md](report.md) "## Guards & Quality Gates" RQG `--bugfix` confirms the adversarial signal in both files (`REGGUARD_EXIT=0`).
- [x] `TP-BUG073005-003` / SCN-BUG073005-003 - Comment markers inside strings remain source text: line/block comments are ignored without corrupting strings or templates. → Evidence: [report.md](report.md) "Revert-Reverify … GREEN" — `--- PASS: TestWithoutComments_PreservesStringsTemplatesAndFollowingCode` (URLs, escaped quotes, templates, and following executable code preserved with the length invariant); the same test remains PASS even in the RED run, proving string/template preservation is orthogonal to comment detection.
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (`TP-BUG073005-004A`). → Evidence: [report.md](report.md) "Revert-Reverify … GREEN" — the scanner-consumer unit canary `./smackerel.sh test unit --go --go-run 'TestWithoutComments|TestWebAssistantStorageGuard|TestWebAssistantRobustnessGuard'` is GREEN (all four tests PASS, `ok internal/testsupport/jssource`, `ok web/pwa/tests`, `UNIT_GREEN_RESTORE_EXIT=0`) and ran BEFORE the broad live package rerun.
- [x] `TP-BUG073005-004B` - Live served-route canary passes after the scanner unit canary and before broad package execution. → Evidence: [report.md](report.md) "Live Served-Route E2E (current session)" — the isolated served-route canary `TestAssistantWebPWAChatE2E_...TP_073_09` PASSes (`SERVED_E2E_EXIT=0`) on the disposable stack, executed after the unit canary and before the full assistant package leg.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified. → Evidence: [design.md](design.md) "Rollback" documents reverting the helper + test changes (no runtime or persisted state touched); VERIFIED this session — [report.md](report.md) "Revert-Reverify … GREEN" shows `git checkout HEAD -- internal/testsupport/jssource/comments.go` restored the helper byte-exact (`restore_rc=0`, `git status --short` empty) and returned all four tests to GREEN.
- [x] Broader E2E regression suite passes (`TP-BUG073005-005`: complete assistant package in package order). → Evidence: [report.md](report.md) "Broader Assistant-Package Regression (current session)" — the complete assistant package runs in package order with **40 PASS** including `TestAssistantWebPWAChatE2E_...TP_073_09`; the ONLY 2 failures are pre-existing FOREIGN `buildvcs` failures in `intent_replay_test.go` (spec-069), dispositioned [report.md](report.md) "## Discovered Issues (Gate G095)" DI-073-005-01 — outside this change boundary, working tree packet-only, not a product regression.
- [x] `TP-BUG073005-006` - Impacted Go unit suite passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh test unit --go` → `FULL_GO_UNITS_EXIT=0` (`[go-unit] go test ./... finished OK`, 0 failures across every shared scanner consumer including `internal/testsupport/jssource` and `web/pwa/tests`).
- [x] `TP-BUG073005-007` - Impacted Python unit suite passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh test unit --python` → `FULL_PY_UNITS_EXIT=0` (`708 passed, 2 deselected`).
- [x] `TP-BUG073005-008` - Configuration check passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check` → `CHECK_EXIT=0` (config-validate OK; env_file drift guard OK; scenario-lint OK).
- [x] `TP-BUG073005-009` - Lint passes with no warnings. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh lint` → `LINT_EXIT=0` (web PWA/extension manifest + JS lint OK).
- [x] `TP-BUG073005-010` - Format check passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `./smackerel.sh format --check` → `FORMAT_EXIT=0` (75 files already formatted).
- [x] `TP-BUG073005-011` - Adversarial regression guard passes with no silent-pass or tautological patterns. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `regression-quality-guard.sh --bugfix internal/testsupport/jssource/comments_test.go tests/e2e/assistant/web_pwa_chat_e2e_test.go` → `REGGUARD_EXIT=0` (adversarial signal detected in BOTH files; 0 violations, 0 warnings).
- [x] `TP-BUG073005-012` - Artifact lint passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `artifact-lint.sh <bug-dir>` → `ARTLINT_EXIT=0` (Artifact lint PASSED).
- [x] `TP-BUG073005-013` - Traceability guard passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `traceability-guard.sh <bug-dir>` → `TRACE_EXIT=0` (3 scenarios → 17 rows; G057/G068 fidelity 3/3; PASSED, 0 warnings).
- [x] `TP-BUG073005-014` - Implementation-reality scan passes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `implementation-reality-scan.sh <bug-dir> --verbose` → `IMPLREALITY_EXIT=0` (0 violations, 0 warnings, 4 files; Sensitive Client Storage scan clean).
- [x] `TP-BUG073005-015` - State-transition guard records the exact remaining owner-routed findings. → Evidence: [report.md](report.md) "### Validation Evidence" — the state-transition guard runs and records the exact remaining owner-routed findings as the EMPTY set (verdict PASS, `failedGateIds: []`, exit 0); the only broader-suite failures are the foreign `buildvcs` issue dispositioned DI-073-005-01 (G095), not owner-routed findings for this packet.
- [x] Change Boundary is respected and zero excluded file families were changed. → Evidence: [report.md](report.md) "### Code Diff Evidence" — `git show c5ddf562 --numstat` lists exactly the allowed files (`internal/testsupport/jssource/comments.go`, `internal/testsupport/jssource/comments_test.go`, `tests/e2e/assistant/web_pwa_chat_e2e_test.go`, `web/pwa/tests/assistant_storage_guard_test.go`); `git status --short` is packet-only. No excluded surface (production PWA JavaScript, auth/session/storage behavior, unrelated E2E packages, runtime handlers, config, deployment, release-train artifacts, secrets) was changed.

All 19 DoD items are closed with current-session execution evidence recorded in [report.md](report.md).
