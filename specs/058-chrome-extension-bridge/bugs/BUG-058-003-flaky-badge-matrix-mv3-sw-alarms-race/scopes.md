# Scopes: BUG-058-003

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles.workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Test-harness reliability fix; no product code change; does NOT unblock parent
spec 058.

## Scope 1 — Make `triggerDrain()` robust to the MV3 SW spin-up `chrome.alarms` race

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] Flake independently re-confirmed BEFORE the fix: 12 separate cold
      `./smackerel.sh test e2e-ext -- sideload_smoke.spec.ts --grep "badge matrix"`
      invocations → 10 pass / 2 fail (runs 4, 7) with
      `Cannot read properties of undefined (reading 'create')` at `fixtures.ts:192`
      → Evidence: report.md `### Before Fix (RED)`
- [x] `triggerDrain()` in `extensions/chrome-bridge/test/e2e/fixtures.ts` waits
      INSIDE the `sw.evaluate` worker context for `typeof chrome?.alarms?.create
      === "function"` before calling it (no cross-process check-then-use gap),
      then fires the drain alarm (AC-1)
      → Evidence: report.md `### Code Diff Evidence`
- [x] The readiness wait is bounded (≤ 5s, 50ms poll) and rejects LOUDLY with a
      clear MV3-lifecycle error on timeout — no silent hang, no masking (AC-2)
      → Evidence: report.md `### Code Diff Evidence`
- [x] `test/e2e/playwright.config.ts` keeps `retries: 0` (the flake is fixed at
      the race, NOT papered over with retries) (AC-3)
      → Evidence: report.md `### Audit Evidence` (`retries: 0` unchanged; no retries added)
- [x] `sideload_smoke.spec.ts` assertion `expect(badge).toBe("SETUP")` is
      unchanged; the SETUP-badge contract is not weakened (AC-4)
      → Evidence: report.md `### Audit Evidence` (spec file unmodified)
- [x] Proven non-flaky AFTER the fix: the isolated `badge matrix` test passes on
      20 consecutive separate cold invocations (≥ 10 required) — 20/20 PASS (AC-5)
      → Evidence: report.md `### After Fix (GREEN)` (`AFTER_FIX_TALLY: pass=20 fail=0 of 20`)
- [x] The FULL `./smackerel.sh test e2e-ext` suite passes 11/11 after the fix (AC-5)
      → Evidence: report.md `### After Fix (GREEN)` (`11 passed`)
- [x] Product code untouched: no change to `src/background/index.ts`,
      `manifest.json`, or any production source (AC-6)
      → Evidence: report.md `### Audit Evidence` (`git status` shows only `test/e2e/fixtures.ts`)
- [x] All verification ran through the sanctioned `./smackerel.sh test e2e-ext`
      CLI surface (no ad-hoc `playwright`/`npx`/`pytest`/`docker` workflow) (AC-7)
      → Evidence: report.md `### Before Fix (RED)` + `### After Fix (GREEN)` (every run is `./smackerel.sh test e2e-ext ...`)
- [x] RED→GREEN regression evidence recorded (the adversarial cold-run battery:
      red BEFORE = 2/12 fail, green AFTER = 20/20 — reverting the helper guard
      reintroduces the red); this is the scenario-first red-green (TDD) proof
      → Evidence: report.md `### Regression Evidence`
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior:
      the `badge matrix` e2e test (sideload_smoke.spec.ts:49) IS the persistent
      scenario-specific regression that drives the hardened `triggerDrain()`
      helper; proven across 20 cold invocations
      → Evidence: report.md `### After Fix (GREEN)` + `### Regression Evidence`
- [x] Broader E2E regression suite passes: the full `./smackerel.sh test e2e-ext`
      suite is green 11/11 (no regression to the other 10 tests)
      → Evidence: report.md `### After Fix (GREEN)` (`11 passed`)
- [x] `SCN-058-003-01` recorded in `scenario-manifest.json` with `linkedTests`
      → Evidence: `scenario-manifest.json`
- [x] Parent spec 058 planning truth NOT touched; honest caveat recorded that
      spec 058 stays `blocked` on the keyless-OIDC cosign/Rekor row
      → Evidence: bug.md "Scope boundary"; uservalidation.md "Notes"

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-058-003-01 | `badge matrix: an unconfigured install shows SETUP after a drain attempt` is deterministic across cold MV3 SW spin-ups (drive the now-robust `triggerDrain()` helper) | extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts | e2e-ui (extension-client) | SCN-058-003-01 |
| T-058-003-02 | Regression E2E: the `badge matrix` test stays green across ≥10 cold invocations AND the full suite passes 11/11 (persistent scenario-specific + broader regression) | extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts | Regression E2E (e2e-ui) | SCN-058-003-01 |

**Regression contract (adversarial, per bugfix policy):** the "test" IS the e2e
test. The adversarial methodology is the **separate-cold-invocation battery**
(the only methodology that surfaces the flake — `--repeat-each` warm runs do
not). RED→GREEN: BEFORE the helper guard the battery failed 2/12; AFTER, 20/20.
Reverting the `chrome.alarms.create` readiness poll re-introduces the BEFORE red.

### Non-Goals

- Parent spec 058 stays `blocked` on its keyless-OIDC cosign/Rekor verification row (`specs/058-chrome-extension-bridge/`, operator / CI-release-gated); this test-harness packet intentionally does not modify it.
- Editing parent spec 058 `spec.md`/`design.md`/`scopes.md`.
- Product-code changes (`alarms` permission + top-level registration are correct).
- Adding Playwright `retries` (forbidden as a fix).
- Hardening the unrelated `chrome.action.getBadgeText` readout (proven
  unnecessary by 20/20 cold + 11/11 full-suite green).
