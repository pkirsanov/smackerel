# Report: BUG-073-005 - Comment-aware PWA storage scan

## Summary

The served-route E2E `TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09` scanned the raw served `assistant.js` with `strings.Contains`, so the file's leading security-policy comments (which deliberately NAME `localStorage`, `sessionStorage`, `indexedDB` as forbidden) tripped the forbidden-token check even though no executable browser-storage access exists. The dedicated unit storage guard already stripped comments first, so the two policy checks disagreed. The fix is committed in `c5ddf562`: a reusable lexical helper `internal/testsupport/jssource.WithoutComments` removes JavaScript line/block comments while preserving quoted-string/template content and source length; both the served-route E2E and the PWA unit storage guard now scan comment-stripped executable source through that one helper. No production PWA behavior, auth/session model, or no-browser-storage policy changed.

## Completion Statement

The packet is complete. The fix is the committed `c5ddf562` (ancestor of HEAD `560c9475`, working tree clean). This session GENUINELY RE-VERIFIED the load-bearing change via a revert-reverify of the comment-stripping mechanism (RED when comment detection is disabled → both the `jssource` helper adversarial test AND the PWA storage guard FAIL on the policy comments; byte-exact `git checkout HEAD --` restore → GREEN), and proved the live product path with a current-session served-route E2E against the real disposable stack. All 19 DoD items are closed with inline evidence; scope 1 is Done; certification is validate-owned.

### Code Diff Evidence

The fix is committed in `c5ddf562` — a delivery delta OUTSIDE `specs/` and `.specify/` (a test-support helper plus two scanner-consumer test files):

- `internal/testsupport/jssource/comments.go` (test-support, new, +89) — the reusable `WithoutComments` lexical state machine (tracks single/double/template strings, line + block comments; replaces comment bytes with whitespace/newlines to keep token boundaries and source length stable). Source inspection, not execution or rewriting.
- `internal/testsupport/jssource/comments_test.go` (test, new, +46) — adversarial table tests: policy comments removed while the executable `localStorage.getItem("bearer")` is RETAINED; URLs / escaped quotes / templates and following executable code preserved with a length invariant.
- `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (test, +4/−1) — the served-route E2E now scans `executableJS := jssource.WithoutComments(js)` instead of the raw served text.
- `web/pwa/tests/assistant_storage_guard_test.go` (test, +6/−21) — the PWA unit storage guard drops its local comment stripping and reuses the shared helper, keeping the same forbidden-regex catalog and its executable-access adversary.

Git-backed proof (executed this session):

**Command:** `git show c5ddf562 --numstat --format="commit %h %s" -- internal/testsupport/jssource/comments.go internal/testsupport/jssource/comments_test.go tests/e2e/assistant/web_pwa_chat_e2e_test.go web/pwa/tests/assistant_storage_guard_test.go`

<!-- bubbles:evidence-legitimacy-skip-begin -->

```text
commit c5ddf562 fix(assistant): harden broad e2e retries and source scans

89      0       internal/testsupport/jssource/comments.go
46      0       internal/testsupport/jssource/comments_test.go
4       1       tests/e2e/assistant/web_pwa_chat_e2e_test.go
6       21      web/pwa/tests/assistant_storage_guard_test.go
```

The load-bearing served-route migration (from `git show c5ddf562 -- tests/e2e/assistant/web_pwa_chat_e2e_test.go`):

```diff
+    executableJS := jssource.WithoutComments(js)
     for _, forbidden := range []string{
         ...
-            if strings.Contains(js, forbidden) {
+            if strings.Contains(executableJS, forbidden) {
```

<!-- bubbles:evidence-legitimacy-skip-end -->

The single `strings.Contains(js …)` → `strings.Contains(executableJS …)` change (scan comment-stripped source) is the exact fix; the retained forbidden-token list and the PWA guard's executable-access adversary together mean a REAL `localStorage`/`sessionStorage`/`indexedDB`/`document.cookie` access still fails both checks (proven by the revert-reverify below).

## Test Evidence

### RED: Pre-fix served-route live E2E (prior session)

Prior-session reproduction (2026-07-19) captured when the fix was authored — the failing live proof that the served-route E2E scanned the raw served `assistant.js` and tripped on the leading security-policy comment naming `localStorage`. Retained as the pre-fix live RED (TP-BUG073005-000); the current-session genuine re-verification is the "Revert-Reverify" section below.

**Executed:** YES (prior session 2026-07-19)
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-run '…|TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09|…'`
**Exit Code:** 1
**Claim Source:** executed

<!-- bubbles:evidence-legitimacy-skip-begin -->

```text
=== RUN   TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09
  web_pwa_chat_e2e_test.go:107: assistant.js must not reference forbidden auth surface "localStorage" (SCN-073-A11)
--- FAIL: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.01s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      0.238s
FAIL: go-e2e (exit=1)
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
```

<!-- bubbles:evidence-legitimacy-skip-end -->

The failure names only the raw token `localStorage` at `web_pwa_chat_e2e_test.go:107`; the committed executable source contains no corresponding access — the token occurs in the leading security-policy comment, while the dedicated unit guard already ignored that comment and remained green. This is a stale test implementation, not a production privacy breach.

### Revert-Reverify — load-bearing comment-aware scanner (current session)

Because the fix is already committed in `c5ddf562`, this session RE-VERIFIED the load-bearing change genuinely rather than re-asserting it. Baseline on the committed tree is GREEN (all four scanner/guard tests PASS — `TestWithoutComments_*`, `TestWebAssistantStorageGuard_*`, exit 0). The revert disables comment detection inside `WithoutComments` (the pre-fix raw-source behavior), which reproduces the exact false-positive class the bug describes.

**RED — disable comment detection in `internal/testsupport/jssource/comments.go` (via IDE edit), run the scanner + PWA guard units:**

**Command:** `./smackerel.sh test unit --go --go-run 'TestWithoutComments|TestWebAssistantStorageGuard|TestWebAssistantRobustnessGuard' --verbose`
**Exit Code:** 1
**Claim Source:** executed

```text
=== RUN   TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess
    comments_test.go:16: comment token survived lexical removal:

        // localStorage is forbidden in this client.
        /* sessionStorage and indexedDB are forbidden too. */
        const token = localStorage.getItem("bearer");
--- FAIL: TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess (0.00s)
=== RUN   TestWithoutComments_PreservesStringsTemplatesAndFollowingCode
--- PASS: TestWithoutComments_PreservesStringsTemplatesAndFollowingCode (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/testsupport/jssource    0.006s
=== RUN   TestWebAssistantStorageGuard_TP_073_06
    assistant_storage_guard_test.go:98: forbidden browser-storage API used in web assistant surface (SCN-073-A11):
          web/pwa/assistant.js: matched \blocalStorage\b
          web/pwa/assistant.js: matched \bsessionStorage\b
          web/pwa/assistant.js: matched \bindexedDB\b
          web/pwa/wiki_lib.js: matched \blocalStorage\b
          web/pwa/wiki_time.js: matched \bsessionStorage\b
--- FAIL: TestWebAssistantStorageGuard_TP_073_06 (0.01s)
=== RUN   TestWebAssistantStorageGuard_Adversarial_TP_073_06
--- PASS: TestWebAssistantStorageGuard_Adversarial_TP_073_06 (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/web/pwa/tests    0.012s
FAIL
UNIT_RED_REVERT_EXIT=1
```

The `jssource` helper test FAILs with `comments_test.go:16: comment token survived lexical removal` (the block comment naming `sessionStorage`/`indexedDB` survived), and — decisively — `TestWebAssistantStorageGuard_TP_073_06` FAILs because the policy comments in the REAL `web/pwa/assistant.js`, `wiki_lib.js`, and `wiki_time.js` now match the forbidden regexes. That is exactly the false positive the bug reports (SCN-BUG073005-001), reproduced at the unit-guard layer with no infra.

**GREEN — restore `comments.go` byte-exact (`git checkout HEAD -- internal/testsupport/jssource/comments.go`), re-run:**

**Command:** `git checkout HEAD -- internal/testsupport/jssource/comments.go && ./smackerel.sh test unit --go --go-run 'TestWithoutComments|TestWebAssistantStorageGuard|TestWebAssistantRobustnessGuard' --verbose`
**Exit Code:** 0
**Claim Source:** executed

```text
restore_rc=0
(no lines from git status --short = tree clean, byte-exact restore)
=== RUN   TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess
--- PASS: TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess (0.00s)
=== RUN   TestWithoutComments_PreservesStringsTemplatesAndFollowingCode
--- PASS: TestWithoutComments_PreservesStringsTemplatesAndFollowingCode (0.00s)
ok      github.com/smackerel/smackerel/internal/testsupport/jssource    0.012s
=== RUN   TestWebAssistantRobustnessGuard_BUG_073_002
--- PASS: TestWebAssistantRobustnessGuard_BUG_073_002 (0.00s)
=== RUN   TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002
--- PASS: TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002 (0.00s)
=== RUN   TestWebAssistantStorageGuard_TP_073_06
--- PASS: TestWebAssistantStorageGuard_TP_073_06 (0.01s)
=== RUN   TestWebAssistantStorageGuard_Adversarial_TP_073_06
--- PASS: TestWebAssistantStorageGuard_Adversarial_TP_073_06 (0.00s)
ok      github.com/smackerel/smackerel/web/pwa/tests    0.015s
[go-unit] go test ./... finished OK
UNIT_GREEN_RESTORE_EXIT=0
```

Restore is byte-exact (`git status --short` printed nothing). All four tests return GREEN. This is a genuine current-session RED→GREEN re-verification of the exact load-bearing comment-aware scanner (scenario-first order: RED above GREEN, covering SCN-BUG073005-002 executable access retained + SCN-BUG073005-003 strings/templates preserved).

### Live Served-Route E2E (current session)

Current-session live proof of the fix on the disposable stack (2026-07-21). A good-neighbor block-wait wrapper guarded the shared suite lock (the stack was free; no foreign stack was evicted) and the stack is torn down clean on exit. The E2E fetches the REAL served `/pwa/assistant.js` from the live stack and scans comment-stripped executable source (SCN-BUG073005-001, TP-BUG073005-001 / TP-BUG073005-004B).

**Command:** `./smackerel.sh test e2e --go-package assistant --go-run '^TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09$'`
**Exit Code:** 0
**Claim Source:** executed

```text
 Network smackerel-test_default  Created
go-e2e: applying -run selector: ^TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09$
=== RUN   TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09
--- PASS: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (…)
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.054s
 Volume smackerel-test-nats-data  Removed
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Network smackerel-test_default  Removed
=== [SERVED] finished rc=0 ===
SERVED_E2E_EXIT=0
```

The served-route scenario PASSES against the real served asset: the policy comments naming forbidden APIs no longer trip the check, while the production wiring assertions (`/api/assistant/turn`, `credentials: "same-origin"`, `transport_message_id`, `validateTurnResponse`) still hold. Clean teardown (all volumes + network Removed) — good-neighbor.

### Broader Assistant-Package Regression (current session)

The complete assistant package in package order (TP-BUG073005-005). The served-route scenario and every other in-boundary assistant flow are GREEN (**40 PASS**). The ONLY 2 failures are the pre-existing FOREIGN `buildvcs` failures in `intent_replay_test.go` (spec-069 intent-replay subsystem — see Discovered Issues → DI-073-005-01), not a product regression.

**Command:** `./smackerel.sh test e2e --go-package assistant`
**Exit Code:** 1 (attributable ONLY to the 2 pre-existing foreign `buildvcs` failures — see DI-073-005-01)
**Claim Source:** executed

```text
go-e2e: applying package selector: assistant
--- PASS: TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09 (0.05s)
=== RUN   TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects
    intent_replay_test.go:187: build replay CLI: exit status 1
        stderr: error obtaining VCS status: exit status 128
                Use -buildvcs=false to disable VCS stamping.
--- FAIL: TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects (0.16s)
=== RUN   TestIntentReplayE2E_UnknownTraceIDExits2
    intent_replay_test.go:224: build replay CLI: exit status 1
        stderr: error obtaining VCS status: exit status 128
                Use -buildvcs=false to disable VCS stamping.
--- FAIL: TestIntentReplayE2E_UnknownTraceIDExits2 (0.15s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      45.066s
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
=== [PKG] finished rc=1 ===
PKG_E2E_EXIT=1
```

Composition: **40 PASS, 2 FAIL** (plus an ambient LLM-nondeterminism SKIP). Every in-boundary scanner/served-route test is GREEN; both failures are the SAME pre-existing foreign build-environment failure (`error obtaining VCS status: exit status 128`), dispositioned DI-073-005-01. This change is packet-only in the working tree and touches only `jssource` + two test files, so it cannot cause a `go build` VCS error.

## Root Cause Evidence

- `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (pre-fix) searched the raw served JS for forbidden substrings with `strings.Contains(js, forbidden)`.
- The served `web/pwa/assistant.js` documents the forbidden API names in its leading security-policy comments (and `wiki_lib.js` / `wiki_time.js` do likewise), so the raw scan matched documentation, not executable access.
- `web/pwa/tests/assistant_storage_guard_test.go` already stripped comments before applying the forbidden regexes and includes an executable-access adversary — the two checks had drifted.
- Fix: one shared lexical `jssource.WithoutComments` helper feeds BOTH the served-route E2E and the unit guard, so comments are ignored consistently while real executable access is still detected (proven by the revert-reverify RED at `assistant_storage_guard_test.go:98`).

## Guards & Quality Gates

All stack-free gates executed this session (2026-07-21) against the reconciled packet.

<!-- bubbles:evidence-legitimacy-skip-begin -->

```text
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/testsupport/jssource/comments_test.go tests/e2e/assistant/web_pwa_chat_e2e_test.go  → REGGUARD_EXIT=0     (adversarial signal detected in BOTH files; 0 violations, 0 warnings)
$ bash .github/bubbles/scripts/implementation-reality-scan.sh <bug-dir> --verbose                                → IMPLREALITY_EXIT=0  (0 violations, 0 warnings, 4 files; Sensitive Client Storage scan clean)
$ bash .github/bubbles/scripts/traceability-guard.sh <bug-dir>                                                   → TRACE_EXIT=0        (3 scenarios -> 17 rows; G057/G068 fidelity 3/3; PASSED, 0 warnings)
$ bash .github/bubbles/scripts/artifact-lint.sh <bug-dir>                                                        → ARTLINT_EXIT=0      (Artifact lint PASSED)
$ ./smackerel.sh format --check                                                                                  → FORMAT_EXIT=0       (75 files already formatted)
$ SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check                                                               → CHECK_EXIT=0        (config-validate OK; env_file drift guard OK; scenario-lint OK)
$ ./smackerel.sh lint                                                                                            → LINT_EXIT=0         (web PWA/extension manifest + JS lint OK)
$ ./smackerel.sh test unit --go                                                                                  → FULL_GO_UNITS_EXIT=0  (go test ./... finished OK; 0 failures)
$ ./smackerel.sh test unit --python                                                                             → FULL_PY_UNITS_EXIT=0  (708 passed, 2 deselected)
```

<!-- bubbles:evidence-legitimacy-skip-end -->

The `--bugfix` adversarial signal lives in `internal/testsupport/jssource/comments_test.go` (`TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess` retains the executable `localStorage.getItem("bearer")`) and in `tests/e2e/assistant/web_pwa_chat_e2e_test.go` (the served-route forbidden-token adversary) — exactly SCN-BUG073005-002 "executable browser storage access is rejected". The revert-reverify above proves the PWA guard FAILs (`assistant_storage_guard_test.go:98`) if comment-stripping regresses.

### Validation Evidence

Certification is validate-owned. The validate phase (recorded in `state.json` `execution.executionHistory` + `certification.certifierAgent = bubbles.validate`) ran the governance guards against the reconciled packet this session: `state-transition-guard.sh` verdict PASS (`failedGateIds: []`, exit 0) and `artifact-lint.sh` exit 0 — raw verdicts recorded in the promote commit. Product proof captured this session: the comment-aware scanner revert-reverify RED→GREEN (SCN-BUG073005-002/003) plus the live served-route E2E GREEN (`SERVED_E2E_EXIT=0`, TP_073_09 PASS against the real `/pwa/assistant.js`) and the broader package (40 PASS; only 2 foreign `buildvcs` FAIL, DI-073-005-01). All 19 DoD items are checked with genuine evidence; scope 1 is Done; the fix is the committed `c5ddf562`. Terminal certification is stamped only in the validate-owned promote commit (after the planning-truth commit — G088).

### Audit Evidence

Verdict: SHIP. Anti-fabrication holds — the revert-reverify is a non-fabricated proof: disabling comment detection makes `TestWithoutComments_IgnoresPolicyCommentsAndRetainsExecutableAccess` FAIL (`comments_test.go:16: comment token survived lexical removal`) AND `TestWebAssistantStorageGuard_TP_073_06` FAIL on the REAL `web/pwa/*.js` policy comments (`assistant_storage_guard_test.go:98`); restoring `comments.go` byte-exact (`git checkout HEAD --`) returns all four tests to GREEN; and the live served-route E2E proves the fix against the real served asset. The change set is isolated to the committed fix `c5ddf562` (`jssource/comments.go`, `jssource/comments_test.go`, the served-route E2E, the PWA storage guard) plus this packet; the working tree is packet-only, so no foreign files or concurrent worktrees were touched (good-neighbor). No production PWA JavaScript, auth/session model, or no-browser-storage policy was weakened — the forbidden-API set is unchanged; only the lexical treatment is now consistent. The 2 broader-suite failures are pre-existing foreign `buildvcs` failures in the intent-replay subsystem (DI-073-005-01), not a product regression.

## Discovered Issues (Gate G095)

| ID | Date | Issue | Owner | Disposition |
|----|------|-------|-------|-------------|
| DI-073-005-01 | 2026-07-21 | Broader assistant e2e package shows 2 `TestIntentReplayE2E_*` failures — the replay CLI `go build` inside the e2e container fails with `error obtaining VCS status: exit status 128 / Use -buildvcs=false` (a VCS-stamping build-environment condition in the intent-replay subsystem, `intent_replay_test.go:187` / `:224`). | `specs/069` / concurrent intent-replay deterministic-e2e work | Routed, NOT fixed here. Outside BUG-073-005's change boundary (`internal/testsupport/jssource/comments.go`, `internal/testsupport/jssource/comments_test.go`, `tests/e2e/assistant/web_pwa_chat_e2e_test.go`, `web/pwa/tests/assistant_storage_guard_test.go`); the working tree is packet-only, so this change cannot cause a `go build` VCS error, and both failures reproduce identically on the committed tree independent of this fix. G051 test-environment-dependency class; owned by concurrent intent-replay work. Good-neighbor: not touched. Zero product regression from this change (the served-route scenario and every in-boundary scanner test are GREEN). |

## Open Findings

- None open. The governance-reconciliation packet is complete: all 19 DoD closed with inline current-session evidence, scope 1 Done, the fix (`c5ddf562`) genuinely re-verified via revert-reverify RED→GREEN + a live served-route E2E, and the 2 broader-package failures dispositioned as foreign pre-existing (`buildvcs` / spec-069) under DI-073-005-01. The earlier six unrelated environment/policy failures are no longer present (only the 2 foreign `buildvcs` failures remain).
