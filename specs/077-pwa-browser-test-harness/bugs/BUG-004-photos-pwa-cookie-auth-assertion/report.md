# Report: BUG-077-004 Photos PWA Cookie-Auth Assertion

## Summary

The consolidated release candidate's complete E2E run failed only in the root package. Isolation identified one stale Photos PWA source assertion: the test required `Authorization` while the served wizard correctly used the spec-100 HttpOnly same-origin cookie contract.

## Completion Statement

The one-file test-only repair is committed (`b4761988`, an ancestor of HEAD `02c9a32f`) and was RE-VERIFIED genuinely this session by revert→RED / restore→GREEN plus the complete root-package regression. All 16 DoD items are closed with current-session raw evidence; Scope 1 is Done. The single broader-suite failure is a pre-existing FOREIGN `buildvcs` environment condition (DI-077-004-01), not a product regression. Terminal certification is validate-owned and is stamped only by the promote commit after the planning-truth commit (G088).

## Test Evidence

### RED — revert-reverify (this session): the stale `Authorization` assertion FAILS against the served cookie-auth wizard

**Executed:** YES, current session
**Load-bearing revert:** `tests/e2e/photos_pwa_test.go` assertion reverted to the pre-fix `"Authorization"` string — exact pre-fix reproduction confirmed by `diff <(git show b4761988~1:tests/e2e/photos_pwa_test.go) tests/e2e/photos_pwa_test.go` → `EXACT_PREFIX_MATCH`.
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$'`
**Exit Code:** 1 (`BUG004_RED_EXIT=1`)
**Claim Source:** executed

```text
go-e2e: applying -run selector: ^TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI$
=== RUN   TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI
    photos_pwa_test.go:29: photo-library-add.js missing "Authorization"
--- FAIL: TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e        0.153s
(subpackages report "no tests to run" under the focused -run selector)
FAIL: go-e2e (exit=1)
 Container smackerel-test-smackerel-core-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Network smackerel-test_default  Removed
BUG004_RED_EXIT=1
```

The served `web/pwa/photo-library-add.js` intentionally authenticates via the HttpOnly `auth_token` cookie (`credentials: "same-origin"`; no JS-visible bearer token). The reverted assertion demands `Authorization`, which the served script correctly does not contain → RED. This proves the fix line is load-bearing. My `smackerel-test` stack was torn down cleanly (good-neighbor scoped teardown).

### GREEN + complete root E2E package (this session): fix restored → Photos GREEN; 127/128 root tests PASS

**Executed:** YES, current session
**Restore:** `git checkout HEAD -- tests/e2e/photos_pwa_test.go` — `git diff --stat HEAD` empty (working tree exactly == HEAD; the fixed `credentials: "same-origin"` + adversarial `"omit"` guard restored byte-exact).
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '^(<128 root-package test names>)$'` — anchored regex enumerated from `tests/e2e/*.go` covering the complete root package; it deliberately excludes the foreign `tests/e2e/assistant` intent-replay package.
**Exit Code:** 1 — attributable ONLY to the single foreign `buildvcs` failure DI-077-004-01 (`BUG004_ROOTPKG_EXIT=1`)
**Claim Source:** executed

```text
=== RUN   TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks
--- PASS: TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks (0.02s)
--- PASS: TestPhotosDedupe_E2E_CrossProviderDuplicateReturnedOnce (0.01s)
--- PASS: TestPhotosFoundation_E2E_SyntheticPhotoDetailFromLiveAPI (0.06s)
--- PASS: TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates (0.01s)
=== RUN   TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI
--- PASS: TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI (0.01s)
--- PASS: TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI (0.01s)
--- PASS: TestPhotosRemoval_E2E_ActionPlanDoesNotMutateBeforeConfirm (0.03s)
--- PASS: TestPhotosRouting_E2E_ReceiptRecipeDocumentCreateDownstreamArtifacts (0.07s)
--- PASS: TestPhotosSearch_E2E_ImmichWhiteboardOCRResult (0.07s)
--- PASS: TestPhotosSearch_E2E_CrossProviderUnifiedRanking (0.06s)
--- PASS: TestPhotosSensitivity_E2E_TelegramDoesNotAutoSendSensitivePhoto (0.06s)
--- PASS: TestPhotosSync_E2E_AlbumMoveDoesNotReclassify (0.05s)
--- PASS: TestPhotosTelegram_E2E_UploadClassifySearchAndRetrieve (0.06s)
(… 127 of 128 root-package tests PASS …)
=== RUN   TestHTTPAdapter_MissingRequiredKey_FailsLoud
    spec062_http_missing_key_test.go:69: build smackerel-core failed: exit status 1
        error obtaining VCS status: exit status 128
                Use -buildvcs=false to disable VCS stamping.
--- FAIL: TestHTTPAdapter_MissingRequiredKey_FailsLoud (0.19s)
FAIL    github.com/smackerel/smackerel/tests/e2e        213.932s
FAIL: go-e2e (exit=1)
BUG004_ROOTPKG_EXIT=1
```

All 13 Photos root-package tests — including the fixed `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` — PASS. The endpoint (`/v1/photos/connectors/test`, `/v1/photos/connectors`), included-album payload, live connector API (200 + JSON decode), and Immich-provider assertions all execute green inside that same test. The ONLY failure is the foreign environmental `buildvcs` VCS-stamping condition in the unrelated `TestHTTPAdapter_MissingRequiredKey_FailsLoud` (DI-077-004-01), which a test-only assertion change cannot cause.

### Focused Go unit regression (this session)

**Command:** `./smackerel.sh test unit --go --go-run 'TestPhotosPWA|TestSynthesisExtractResponse|TestHTTPAdapter'`
**Exit Code:** 0 (`UNIT_FOCUSED_EXIT=0`)
**Claim Source:** executed

```text
[go-unit] applying -run selector: TestPhotosPWA|TestSynthesisExtractResponse|TestHTTPAdapter
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter   0.083s
ok      github.com/smackerel/smackerel/internal/pipeline        0.043s
ok      github.com/smackerel/smackerel/internal/connector/photos        0.021s [no tests to run]
(… all packages ok; zero FAIL …)
[go-unit] go test ./... finished OK
UNIT_FOCUSED_EXIT=0
```

The neighboring Photos, HTTP-adapter, and synthesis unit paths remain green (`httpadapter` + `pipeline` matched and ran GREEN).

## Guards & Quality Gates (this session)

<!-- bubbles:evidence-legitimacy-skip-begin -->

```text
$ SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check                                                    → CHECK_EXIT=0        (Config in sync with SST; env_file drift guard OK; scenario-lint OK: 17 registered, 0 rejected)
$ ./smackerel.sh format --check                                                                      → FORMAT_EXIT=0       (75 files already formatted)
$ ./smackerel.sh lint                                                                                → LINT_EXIT=0         (ruff "All checks passed!"; web validation passed)
$ ./smackerel.sh test unit --go --go-run 'TestPhotosPWA|TestSynthesisExtractResponse|TestHTTPAdapter'→ UNIT_FOCUSED_EXIT=0 (httpadapter + pipeline GREEN; all packages ok)
$ bash .github/bubbles/scripts/regression-quality-guard.sh tests/e2e/photos_pwa_test.go              → RQG_PLAIN_EXIT=0    (0 violations, 0 warnings)
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/e2e/photos_pwa_test.go     → RQG_BUGFIX_EXIT=0   (Adversarial signal detected)
$ bash .github/bubbles/scripts/implementation-reality-scan.sh <bug-dir> --verbose                    → REALITY_EXIT=0      (0 violations, 1 file scanned)
$ bash .github/bubbles/scripts/traceability-guard.sh <bug-dir>                                       → TRACE_EXIT=0        (2 scenarios → 13 rows; G057/G068 2/2; PASSED, 0 warnings)
$ bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>                                            → ALINT_EXIT=0        (Artifact lint PASSED)
$ bash .github/bubbles/scripts/state-transition-guard.sh <bug-dir>                                   → GUARD_EXIT=0        (verdict PASS, failedGateIds: [])
```

<!-- bubbles:evidence-legitimacy-skip-end -->

## Change Boundary

Production source (`web/pwa/photo-library-add.js`, auth/session middleware, config, deployment, release trains, secrets) is UNCHANGED. The only permanent delta is `tests/e2e/photos_pwa_test.go` (the committed fix `b4761988`, +4/-1) plus this bug packet. The temporary revert used for the RED reproduction was restored byte-exact via `git checkout HEAD --` (working tree clean vs HEAD).

### Code Diff Evidence

**Phase:** implement
**Command:** `git show b4761988 -- tests/e2e/photos_pwa_test.go` + `git merge-base --is-ancestor b4761988 HEAD` + revert-reverify `diff <(git show b4761988~1:tests/e2e/photos_pwa_test.go) tests/e2e/photos_pwa_test.go`
**Exit Code:** 0
**Claim Source:** executed

```text
# fix commit b4761988 is an ancestor of HEAD 02c9a32f:
YES b4761988 is ancestor of HEAD

# the load-bearing fix (git show b4761988 -- tests/e2e/photos_pwa_test.go):
@@ func TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI(t *testing.T) {
        addJS := getE2EText(t, cfg.CoreURL+"/pwa/photo-library-add.js")
-       for _, expected := range []string{".../connectors/test", ".../connectors", "Authorization", "included_albums"} {
+       for _, expected := range []string{".../connectors/test", ".../connectors", `credentials: "same-origin"`, "included_albums"} {
                if !strings.Contains(addJS, expected) { t.Fatalf("photo-library-add.js missing %q", expected) }
        }
+       if strings.Contains(addJS, `credentials: "omit"`) {
+               t.Fatal("photo-library-add.js must not omit the same-origin auth cookie")
+       }

# revert reproduces the exact pre-fix stale assertion, restore is byte-exact:
EXACT_PREFIX_MATCH (revert reproduces the pre-fix stale assertion)
(after `git checkout HEAD -- tests/e2e/photos_pwa_test.go`) git diff --stat HEAD → empty (clean)
```

## Discovered Issues (Gate G095)

| ID | Date | Issue | Owner | Disposition |
|----|------|-------|-------|-------------|
| DI-077-004-01 | 2026-07-21 | The complete root E2E package ("GREEN + complete root package" leg) shows exactly 1 failure — `TestHTTPAdapter_MissingRequiredKey_FailsLoud` (`spec062_http_missing_key_test.go:69`) whose in-test `go build smackerel-core` fails with `error obtaining VCS status: exit status 128 / Use -buildvcs=false` (a VCS-stamping build-environment condition; identical class to the certified sibling disposition DI-075-002-01). | buildvcs / test-harness build environment (`specs/069` intent-replay & concurrent buildvcs env work) | Routed, NOT fixed here. Outside BUG-077-004's change boundary (`tests/e2e/photos_pwa_test.go` only); a test-only assertion change cannot cause a `go build` VCS-stamping error, and the condition is intermittent/environmental — the same test PASSED at 2.52s in the release-candidate run (G051 test-environment-dependency class). Good-neighbor: foreign test not touched. Zero product regression — all 13 Photos root-package tests + 127/128 root tests are GREEN. |
| DI-077-004-02 | 2026-07-21 | Both live E2E legs print `Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 …)` during teardown — the opt-in Ollama agent lane (`tests/e2e/agent/happy_path_test.go`) is not executed by default. | `bubbles.test` / opt-in Ollama agent lane | Routed, NOT in this bug's boundary. This bug's DoD/Test-Plan target the root-package Photos cookie-auth contract (Go e2e, no Ollama). The Ollama lane is default-off (opt-in env flag) and unrelated to the Photos assertion; it remains unproven residual coverage, not counted as a pass for this packet. G051 opt-in-lane class. |

## Validation Evidence

Certification is validate-owned. The validate phase (recorded in `state.json` `execution.executionHistory` + `certification.certifierAgent = bubbles.validate`) ran the governance guards against the reconciled packet this session: `state-transition-guard.sh` verdict PASS (`failedGateIds: []`, exit 0) and `artifact-lint.sh` exit 0 (raw verdicts in "## Guards & Quality Gates" and the promote commit). Product proof captured this session: the revert-reverify RED→GREEN (RED `BUG004_RED_EXIT=1` on the reverted stale `Authorization` assertion; GREEN `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` PASS after byte-exact restore) plus the complete root E2E package (all 13 Photos tests + 127/128 root tests GREEN; the single failure is the foreign `buildvcs` DI-077-004-01). All 16 DoD items are checked with genuine evidence; Scope 1 is Done; the fix is the committed `b4761988`. Terminal certification is stamped only in the validate-owned promote commit (after the planning-truth commit — G088).

## Audit Evidence

Verdict: SHIP. Anti-fabrication holds — the revert-reverify is a non-fabricated proof: reverting the assertion to `"Authorization"` (exact pre-fix, `EXACT_PREFIX_MATCH`) makes `TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI` FAIL (`photos_pwa_test.go:29 photo-library-add.js missing "Authorization"`, exit 1); byte-exact `git checkout HEAD -- tests/e2e/photos_pwa_test.go` restore returns it GREEN. The change set is isolated to the committed fix `b4761988` (`tests/e2e/photos_pwa_test.go`, +4/-1) plus this packet; the working tree is packet-only, so no foreign files or concurrent worktrees were touched (good-neighbor). No NO-DEFAULTS fallback and no production-source change was introduced — production `web/pwa/photo-library-add.js` already uses `credentials: "same-origin"` and is untouched. The single broader-suite failure is the pre-existing foreign `buildvcs` environment condition (DI-077-004-01), not a product regression.