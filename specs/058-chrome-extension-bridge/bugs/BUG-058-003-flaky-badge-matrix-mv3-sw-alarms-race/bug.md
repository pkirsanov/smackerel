# BUG-058-003: flaky `badge matrix` e2e test — MV3 service-worker `chrome.alarms` spin-up race in the test helper

**Status:** Resolved (test-harness robustness fix verified non-flaky across 20 cold runs + full suite — see report.md)
**Severity:** Major (test-harness reliability — the `./smackerel.sh test e2e-ext` gate fails intermittently ~1-in-3 to ~1-in-6 cold runs; NO product impact)
**Reported:** 2026-06-09
**Resolved:** 2026-06-09
**Reporter:** Orchestrator directive — a genuine flake discovered while re-verifying spec 058's `./smackerel.sh test e2e-ext` discharge claim
**Owner:** `bubbles.workflow` (parent-expanded `bugfix-fastlane`; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/058-chrome-extension-bridge/` (test harness only)
**Parent spec status:** `blocked` — and **stays** `blocked` (this bug does NOT unblock it; see "Scope boundary" below)

## Summary

`./smackerel.sh test e2e-ext` is intermittently red. The full 11-test MV3
extension suite reports **10 passed, 1 failed**, with the failure isolated to:

- `extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts:49` —
  `badge matrix: an unconfigured install shows SETUP after a drain attempt`.

The error is always:

```
worker.evaluate: TypeError: Cannot read properties of undefined (reading 'create')
    at Object.triggerDrain (extensions/chrome-bridge/test/e2e/fixtures.ts:192)
    at extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts:54
```

Because `test/e2e/playwright.config.ts` sets `retries: 0`, a single flake fails
the whole suite — eroding trust in the e2e-ext gate and masking real signal.

## Mechanism (what is wrong — in the TEST HELPER, not product code)

`triggerDrain()` in [`fixtures.ts`](../../../../extensions/chrome-bridge/test/e2e/fixtures.ts)
did, with no readiness guard:

```ts
await sw.evaluate(
  (alarm) =>
    new Promise<void>((resolve) => {
      chrome.alarms.create(alarm, { when: Date.now() + 100 }); // <-- throws when chrome.alarms is undefined
      setTimeout(resolve, 50);
    }),
  DRAIN_ALARM,
);
```

MV3 service workers are torn down when idle and re-spun on the next
`sw.evaluate`. During the brief window right after a **cold** spin-up, the
worker's global scope and the base `chrome` namespace already exist — so the
`sw.evaluate` itself runs (this is NOT a Playwright "target closed" error) — but
the **permission-gated** `chrome.alarms` binding may not yet be installed. The
naive `chrome.alarms.create(...)` then dereferences `undefined.create` and
throws. The `badge matrix` test is the canary because it calls `triggerDrain()`
as its very first action against a freshly (cold) spun-up worker.

## Reproduction (independently re-confirmed, BEFORE the fix)

`./smackerel.sh test e2e-ext -- sideload_smoke.spec.ts --grep "badge matrix"`
run as **12 separate cold invocations**: runs **4 and 7 FAILED** with the
`chrome.alarms ... reading 'create'` error; the other 10 passed → **10 pass / 2
fail (~17%)**. (Note: `--repeat-each=15` in a single warm process passed 15/15 —
which is itself the proof that the race is a **cold-process-start** window, not a
warm per-fixture window.) Full raw output in [report.md](report.md) `### Before Fix (RED)`.

## Observed vs Expected

| | |
|---|---|
| **Observed** | `triggerDrain()` crashes ~1-in-6 cold runs with `Cannot read properties of undefined (reading 'create')`; the suite gate flakes red. |
| **Expected** | `triggerDrain()` is robust to the MV3 SW spin-up lifecycle and fires the drain alarm deterministically; `./smackerel.sh test e2e-ext` is reliably green. |

## Fix (test harness only)

Harden `triggerDrain()` to wait — **inside the worker context**, so there is no
cross-process TOCTOU gap — for `chrome.alarms.create` to become a function
before calling it, bounded to 5s and **rejecting loudly** on timeout (a
genuinely-broken worker surfaces as a clear error, not a flake). See
[design.md](design.md). The assertion `expect(badge).toBe("SETUP")` is unchanged
and **no `retries` were added** to `playwright.config.ts` (masking a flake with
retries is forbidden — the race itself is fixed).

## Product code is CORRECT (untouched)

Confirmed by reading the source — **not changed**:

- [`manifest.json`](../../../../extensions/chrome-bridge/manifest.json) declares the
  `alarms` permission: `"permissions": ["bookmarks", "history", "storage", "alarms"]`.
- [`src/background/index.ts:273-274`](../../../../extensions/chrome-bridge/src/background/index.ts)
  registers `chrome.alarms.create(ALARM_NAME, { periodInMinutes: 1 })` +
  `chrome.alarms.onAlarm.addListener(...)` correctly at top level.
- The other 10 tests (including "MV3 service worker registers" and the manifest
  permission assertion) pass. This is a test-harness robustness gap, not a
  product defect.

## Scope boundary (honest)

This bug makes the e2e-ext harness reliable. It does **NOT** unblock the parent
spec 058. Spec 058 remains `blocked` on its genuinely-irreducible
keyless-OIDC `cosign verify-blob` against a real Rekor transparency log
(operator / CI-release-gated; faking it would be forbidden Rekor pollution).
The parent spec's `spec.md`/`design.md`/`scopes.md` planning truth is **not**
touched by this packet.

## Cross-References

- Fixed helper: [`extensions/chrome-bridge/test/e2e/fixtures.ts`](../../../../extensions/chrome-bridge/test/e2e/fixtures.ts) (`triggerDrain`)
- Canary test (unchanged): [`extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts`](../../../../extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts) (`badge matrix`)
- Config (unchanged, `retries: 0`): [`extensions/chrome-bridge/test/e2e/playwright.config.ts`](../../../../extensions/chrome-bridge/test/e2e/playwright.config.ts)
- Sibling packet that authored the harness: [`../BUG-058-002-mv3-e2e-sideload-harness/`](../BUG-058-002-mv3-e2e-sideload-harness/)
- spec.md · design.md · scopes.md · report.md · uservalidation.md · scenario-manifest.json · state.json
