# Report: BUG-058-003 — flaky `badge matrix` e2e test (MV3 `chrome.alarms` spin-up race)

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-09
**Scope:** test harness only (`extensions/chrome-bridge/test/e2e/fixtures.ts`); product code untouched; parent spec 058 stays `blocked`

## Summary

`./smackerel.sh test e2e-ext` flaked red ~1-in-6 cold runs: the `badge matrix`
test crashed in the `triggerDrain()` helper with
`TypeError: Cannot read properties of undefined (reading 'create')` because the
permission-gated `chrome.alarms` binding is transiently `undefined` during a
cold MV3 service-worker spin-up. Hardening `triggerDrain()` with an in-evaluate
bounded readiness poll eliminated the race: 20/20 consecutive cold runs and the
full 11-test suite now pass.

## Root Cause

MV3 service workers are torn down when idle and re-spun on the next
`sw.evaluate`. On a cold spin-up the base `chrome` namespace exists (so the
evaluate runs) but `chrome.alarms` (gated by the `alarms` permission) is not yet
bound. The unguarded `chrome.alarms.create(...)` dereferenced `undefined`. See
`design.md` (the cold-start nature is proven below: warm `--repeat-each` does
NOT reproduce; separate cold invocations do).

## Fix

In-evaluate bounded readiness poll inside `triggerDrain()`: wait (≤ 5s, 50ms
interval) for `typeof chrome?.alarms?.create === "function"` before calling it,
then fire the drain alarm; reject loudly on timeout. No retries added; assertion
unchanged. Diff in `### Code Diff Evidence`.

### Before Fix (RED)

`--repeat-each=15` in ONE warm process — does NOT reproduce (proves the race is
cold-start, not warm-fixture):

```
$ ./smackerel.sh test e2e-ext -- sideload_smoke.spec.ts --grep "badge matrix" --repeat-each=15
Running 15 tests using 1 worker
  ✓  1 …matrix: an unconfigured install shows SETUP after a drain attempt (1.2s)
  …
  ✓  15 …atrix: an unconfigured install shows SETUP after a drain attempt (1.1s)
  15 passed (31.2s)
BEFORE_FIX_REPRO_EXIT=0
```

12 SEPARATE COLD invocations — reproduces ~17% (runs 4 and 7 FAIL):

```
$ for i in $(seq 1 12); do echo "########## BEFORE-FIX SEPARATE COLD RUN $i ##########"; ./smackerel.sh test e2e-ext -- sideload_smoke.spec.ts --grep "badge matrix"; echo "BEFORE_RUN_${i}_EXIT=$?"; done
########## BEFORE-FIX SEPARATE COLD RUN 1 ##########
  ✓  1 …matrix: an unconfigured install shows SETUP after a drain attempt (2.0s)
  1 passed (3.5s)
BEFORE_RUN_1_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 2 ##########  → 1 passed   BEFORE_RUN_2_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 3 ##########  → 1 passed   BEFORE_RUN_3_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 4 ##########
  ✘  1 …matrix: an unconfigured install shows SETUP after a drain attempt (1.2s)

  1) sideload_smoke.spec.ts:49:1 › badge matrix: an unconfigured install shows SETUP after a drain attempt

    Error: worker.evaluate: TypeError: Cannot read properties of undefined (reading 'create')
        at eval (eval at evaluate (:234:30), <anonymous>:3:29)
        at new Promise (<anonymous>)
        at eval (eval at evaluate (:234:30), <anonymous>:2:13)
        at UtilityScript.evaluate (<anonymous>:236:17)
        at UtilityScript.<anonymous> (<anonymous>:1:44)
        at Object.triggerDrain (~/smackerel/extensions/chrome-bridge/test/e2e/fixtures.ts:192:18)
        at ~/smackerel/extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts:54:13

  1 failed
    sideload_smoke.spec.ts:49:1 › badge matrix: an unconfigured install shows SETUP after a drain attempt
BEFORE_RUN_4_EXIT=1
########## BEFORE-FIX SEPARATE COLD RUN 5 ##########  → 1 passed   BEFORE_RUN_5_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 6 ##########  → 1 passed   BEFORE_RUN_6_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 7 ##########
  ✘  1 …matrix: an unconfigured install shows SETUP after a drain attempt (1.1s)

  1) sideload_smoke.spec.ts:49:1 › badge matrix: an unconfigured install shows SETUP after a drain attempt

    Error: worker.evaluate: TypeError: Cannot read properties of undefined (reading 'create')
        at eval (eval at evaluate (:234:30), <anonymous>:3:29)
        at new Promise (<anonymous>)
        at Object.triggerDrain (~/smackerel/extensions/chrome-bridge/test/e2e/fixtures.ts:192:18)
        at ~/smackerel/extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts:54:13

  1 failed
BEFORE_RUN_7_EXIT=1
########## BEFORE-FIX SEPARATE COLD RUN 8 ##########  → 1 passed   BEFORE_RUN_8_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 9 ##########  → 1 passed   BEFORE_RUN_9_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 10 ########## → 1 passed   BEFORE_RUN_10_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 11 ########## → 1 passed   BEFORE_RUN_11_EXIT=0
########## BEFORE-FIX SEPARATE COLD RUN 12 ########## → 1 passed   BEFORE_RUN_12_EXIT=0
ALL_BEFORE_RUNS_DONE
```

**BEFORE tally: 10 pass / 2 fail of 12 cold runs (~17% flake); failures at
`fixtures.ts:192` — `chrome.alarms.create` on `undefined chrome.alarms`.**

### After Fix (GREEN)

20 SEPARATE COLD invocations of the isolated test (≥ 10 required) — all pass:

```
$ pass=0; fail=0; for i in $(seq 1 20); do echo "########## AFTER-FIX SEPARATE COLD RUN $i ##########"; ./smackerel.sh test e2e-ext -- sideload_smoke.spec.ts --grep "badge matrix"; rc=$?; echo "AFTER_RUN_${i}_EXIT=$rc"; if [ "$rc" -eq 0 ]; then pass=$((pass+1)); else fail=$((fail+1)); fi; done; echo "AFTER_FIX_TALLY: pass=$pass fail=$fail of 20"
########## AFTER-FIX SEPARATE COLD RUN 1 ##########   ✓ 1 passed (1.8s)   AFTER_RUN_1_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 2 ##########   ✓ 1 passed (1.8s)   AFTER_RUN_2_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 3 ##########   ✓ 1 passed (1.7s)   AFTER_RUN_3_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 4 ##########   ✓ 1 passed (1.7s)   AFTER_RUN_4_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 5 ##########   ✓ 1 passed (2.3s)   AFTER_RUN_5_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 6 ##########   ✓ 1 passed (2.5s)   AFTER_RUN_6_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 7 ##########   ✓ 1 passed (1.7s)   AFTER_RUN_7_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 8 ##########   ✓ 1 passed (1.9s)   AFTER_RUN_8_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 9 ##########   ✓ 1 passed (2.2s)   AFTER_RUN_9_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 10 ##########  ✓ 1 passed (2.0s)   AFTER_RUN_10_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 11 ##########  ✓ 1 passed (2.0s)   AFTER_RUN_11_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 12 ##########  ✓ 1 passed (2.2s)   AFTER_RUN_12_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 13 ##########  ✓ 1 passed (3.9s)   AFTER_RUN_13_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 14 ##########  ✓ 1 passed (3.1s)   AFTER_RUN_14_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 15 ##########  ✓ 1 passed (1.8s)   AFTER_RUN_15_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 16 ##########  ✓ 1 passed (1.6s)   AFTER_RUN_16_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 17 ##########  ✓ 1 passed (3.0s)   AFTER_RUN_17_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 18 ##########  ✓ 1 passed (2.3s)   AFTER_RUN_18_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 19 ##########  ✓ 1 passed (2.1s)   AFTER_RUN_19_EXIT=0
########## AFTER-FIX SEPARATE COLD RUN 20 ##########  ✓ 1 passed (2.1s)   AFTER_RUN_20_EXIT=0
AFTER_FIX_TALLY: pass=20 fail=0 of 20
```

Full suite once — 11/11 PASS (including test 11, the previously-flaky `badge matrix`):

```
$ ./smackerel.sh test e2e-ext
Running 11 tests using 1 worker
  ✓  1 …e ingest endpoint sets the AUTH badge and retains the queued item (1.5s)
  ✓  2 …extension/ingest with the bearer token and correct artifact shape (1.6s)
  ✓  3 …tern URL is dropped before it leaves the browser (no ingest POST) (4.4s)
  ✓  4 …d bookmark emits a tombstone artifact with bookmark_event=removed (2.0s)
  ✓  5 …ure the extension through the options page and the values persist (1.6s)
  ✓  6 …eld is masked by default and the Reveal button toggles visibility (1.1s)
  ✓  7 …nfiguration is rejected with a visible error and is not persisted (1.1s)
  ✓  8 …e built extension sideloads and its MV3 service worker registers (779ms)
  ✓  9 …nifest is MV3 with the minimum permissions and a restrictive CSP (981ms)
  ✓  10 …spec.ts:41:1 › the options page renders for a sideloaded install (1.0s)
  ✓  11 …trix: an unconfigured install shows SETUP after a drain attempt (763ms)
  11 passed (17.9s)
FULL_SUITE_EXIT=0
```

## Test Evidence

- Isolated `badge matrix`, AFTER fix: **20/20 cold invocations PASS**
  (`AFTER_FIX_TALLY: pass=20 fail=0 of 20`) — exceeds the ≥ 10 bar.
- Full `./smackerel.sh test e2e-ext` suite, AFTER fix: **11 passed**.
- All runs invoked through the sanctioned `./smackerel.sh test e2e-ext` surface.

### Code Diff Evidence

```diff
$ git --no-pager diff -- extensions/chrome-bridge/test/e2e/fixtures.ts
@@ export const test = base.extend<Fixtures>({
       triggerDrain: async () => {
+        // MV3 service-worker lifecycle race (BUG-058-003): Chrome terminates an
+        // idle MV3 service worker and re-spins it on the next `sw.evaluate`.
+        // During the brief window right after a (cold) spin-up the worker's
+        // global scope and the base `chrome` namespace already exist — so the
+        // evaluate itself runs — but the permission-gated `chrome.alarms`
+        // binding may not yet be installed, so a naive
+        // `chrome.alarms.create(...)` throws "Cannot read properties of
+        // undefined (reading 'create')". We wait INSIDE the worker context (so
+        // there is no cross-process TOCTOU gap between the readiness check and
+        // the call) for the binding to appear, then fire the drain alarm. The
+        // wait is bounded and rejects LOUDLY on timeout — a genuinely-broken
+        // worker surfaces as a clear error rather than a flake, and we
+        // deliberately do NOT mask the race with Playwright-side retries.
         await sw.evaluate(
           (alarm) =>
-            new Promise<void>((resolve) => {
-              chrome.alarms.create(alarm, { when: Date.now() + 100 });
-              setTimeout(resolve, 50);
+            new Promise<void>((resolve, reject) => {
+              const deadlineMs = Date.now() + 5000;
+              const fireWhenReady = () => {
+                if (typeof chrome?.alarms?.create === "function") {
+                  chrome.alarms.create(alarm, { when: Date.now() + 100 });
+                  // Let the onAlarm registration settle before returning.
+                  setTimeout(resolve, 50);
+                  return;
+                }
+                if (Date.now() >= deadlineMs) {
+                  reject(
+                    new Error(
+                      "triggerDrain: chrome.alarms binding never became available within " +
+                        "5000ms of SW spin-up (MV3 service-worker lifecycle race)",
+                    ),
+                  );
+                  return;
+                }
+                setTimeout(fireWhenReady, 50);
+              };
+              fireWhenReady();
             }),
           DRAIN_ALARM,
         );
       },
```

### Regression Evidence (adversarial RED→GREEN)

The "test" IS the e2e test. The adversarial methodology is the
**separate-cold-invocation battery** — the only methodology that exposes the
flake (warm `--repeat-each=15` passed 15/15 and would NOT catch it).

| Phase | Methodology | Result |
|-------|-------------|--------|
| **RED** (before guard) | 12 separate cold invocations | **2 FAIL** / 10 pass — `chrome.alarms.create` on `undefined` at `fixtures.ts:192` |
| **GREEN** (after guard) | 20 separate cold invocations | **20 PASS** / 0 fail |
| **GREEN** (after guard) | full `./smackerel.sh test e2e-ext` | **11 PASS** / 0 fail |

Reverting the in-evaluate readiness poll in `triggerDrain()` restores the BEFORE
red (the `chrome.alarms.create` call again races the cold-spin-up binding
window). This is the standing adversarial guarantee: the cold-run battery fails
iff the guard is absent.

### Audit Evidence

```
$ git status --porcelain
 M extensions/chrome-bridge/test/e2e/fixtures.ts
?? .vscode/
```

- **Only** `extensions/chrome-bridge/test/e2e/fixtures.ts` is modified. (`.vscode/`
  is local editor state that predates this change; it is not part of this fix and
  not staged.)
- **Product code untouched:** no diff to `src/background/index.ts`,
  `manifest.json`, or any production source. The `alarms` permission
  (`manifest.json`: `["bookmarks","history","storage","alarms"]`) and the
  top-level `chrome.alarms.create(ALARM_NAME, …)` registration
  (`src/background/index.ts:273`) were read and confirmed correct — not changed.
- **No retry-masking:** `test/e2e/playwright.config.ts` is unmodified;
  `retries: 0` stands.
- **Assertion preserved:** `sideload_smoke.spec.ts` is unmodified;
  `expect(badge).toBe("SETUP")` stands.

### Validation Evidence

Validated at close-out through the sanctioned guards (`artifact-lint.sh` and
`state-transition-guard.sh`). Captured output:

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/058-chrome-extension-bridge/bugs/BUG-058-003-flaky-badge-matrix-mv3-sw-alarms-race
✅ Detected state.json status: done
✅ DoD completion gate passed for status 'done' (all DoD checkboxes are checked)
✅ Workflow mode 'bugfix-fastlane' permits current status 'done' (ceiling: done)
✅ All 1 scope(s) in scopes.md are marked Done
✅ workflowMode gate satisfied: ### Validation Evidence
✅ workflowMode gate satisfied: ### Audit Evidence
Artifact lint PASSED.

$ bash .github/bubbles/scripts/state-transition-guard.sh specs/058-chrome-extension-bridge/bugs/BUG-058-003-flaky-badge-matrix-mv3-sw-alarms-race
--- Check 3E: Scenario-first TDD Evidence (Gate G060) ---
✅ PASS: Scenario-first TDD evidence is recorded in the scope/report artifacts
--- Check 8A: Scenario-Specific Regression E2E Coverage ---
✅ PASS: Scope DoD includes scenario-specific regression E2E requirement: scopes.md
✅ PASS: Scope DoD includes broader E2E regression suite requirement: scopes.md
✅ PASS: Scope Test Plan includes explicit regression E2E row(s): scopes.md
--- Check 9: DoD Evidence Presence ---
✅ PASS: All 14 checked DoD items across resolved scope files have evidence blocks
--- Check 18: Deferral Language Scan (Gate G040) ---
✅ PASS: Zero deferral language found in scope and report artifacts (Gate G040)
--- Check 30: Post-Certification Spec Edit Detection (Gate G088) ---
✅ PASS: Post-certification planning truth is aligned with certification state (Gate G088)
--- Check 35: Discovered-Issue Disposition (Gate G095) ---
✅ PASS: Discovered-issue disposition clean — no unfiled deferrals (Gate G095)

============================================================
  TRANSITION GUARD VERDICT
============================================================
🟡 TRANSITION PERMITTED with 1 warning(s)
state.json status may be set to 'done'.
STATE_GUARD_EXIT=0
```

The 1 warning is the non-blocking Check 8 "concrete test file paths" heuristic
(the real test file `extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts`
exists and is green 20/20 cold + 11/11 full-suite); it does not block the `done`
transition.

## Completion Statement

BUG-058-003 is **Done**. The MV3 `triggerDrain()` test helper is now robust to
the cold service-worker spin-up `chrome.alarms` binding race: an in-evaluate
bounded readiness poll waits for `chrome.alarms.create` before firing the drain
alarm and rejects loudly on a 5s timeout. Verification through the sanctioned
`./smackerel.sh test e2e-ext` surface is decisive — the isolated `badge matrix`
test passed **20/20** separate cold invocations (the same BEFORE methodology that
failed **2/12**), and the full suite passed **11/11**. No Playwright retries were
added, the `expect(badge).toBe("SETUP")` assertion is unchanged, and **no product
code was modified** (the `alarms` permission and the top-level
`chrome.alarms.create` registration were confirmed already correct). Scope 1 DoD
is complete (14/14). **Honest caveat:** this makes the e2e-ext harness reliable
but does NOT unblock the parent spec 058 — it stays `blocked` on the keyless-OIDC
`cosign verify-blob` against a real Rekor log (operator / CI-release-gated). The
parent spec's planning truth is untouched. The worktree is left uncommitted for
the orchestrator to review + commit.

## Commit-Ordering Note (Gate G088 — for the committing orchestrator)

This packet is left **uncommitted** for the orchestrator to review + commit.
`state.json` is `status: done` with `certifiedAt: 2026-06-09T16:58:00Z` (an
honest, non-future timestamp). While uncommitted/unstaged, G088 passes (the new
planning files are untracked, so `post-cert-spec-edit-guard.sh` finds no
post-cert edits). When committing, keep G088 satisfied: the planning-file
(`spec.md`/`design.md`/`scopes.md`) commit time must be **≤ `certifiedAt`**, OR
advance `certifiedAt` to the commit moment in the committing change. **Do NOT
future-date `certifiedAt`** to bypass G088 (forbidden — see the spec-063
incident). `state.json`/`report.md`/`scenario-manifest.json` are not G088-tracked.
