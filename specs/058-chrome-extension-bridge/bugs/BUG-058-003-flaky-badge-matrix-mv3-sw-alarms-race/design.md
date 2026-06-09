# Design: BUG-058-003 — MV3 `chrome.alarms` spin-up race in `triggerDrain()`

## Problem

`./smackerel.sh test e2e-ext` flakes red ~1-in-6 to ~1-in-3 cold runs. The
failure is always the same: `triggerDrain()` in `fixtures.ts` throws
`TypeError: Cannot read properties of undefined (reading 'create')` because
`chrome.alarms` is transiently `undefined` when the helper calls
`chrome.alarms.create(...)`. With `retries: 0` a single flake fails the whole
suite, which directly undermines the spec 058 `e2e-ext` discharge claim.

## Root Cause Analysis

### The MV3 service-worker lifecycle

Manifest V3 background logic runs in a **service worker** that Chrome
**terminates when idle** and **re-spins on demand**. Playwright's
`worker.evaluate()` wakes / re-attaches to the worker. Two facts combine into
the race:

1. The base `chrome` namespace and the worker global scope are available
   essentially immediately on spin-up — so `sw.evaluate()` **resolves and runs**
   (this is why the error is a `TypeError` *inside* the evaluate, NOT a
   Playwright `"target closed"`/worker-died rejection).
2. The **permission-gated** API bindings (`chrome.alarms`, gated by the
   `alarms` permission) are installed onto the worker scope a beat **later**.
   For a short window, `chrome.alarms` is `undefined` while `chrome` is defined.

`triggerDrain()` dereferenced `chrome.alarms.create` during that window →
`undefined.create` → `TypeError`.

### Why it's the `badge matrix` test specifically

Each test gets a fresh `ext` fixture (a fresh `launchPersistentContext` + a
freshly spun-up MV3 worker). `badge matrix` calls `triggerDrain()` as its
**first** action, hitting the worker at its coldest moment. The other tests
either don't drain first or interact with the worker enough beforehand that the
`alarms` binding has already landed.

### Why cold-start, proven empirically

| Methodology | Result | Interpretation |
|-------------|--------|----------------|
| `--repeat-each=15` (one **warm** process) | 15/15 PASS | Repeats 2–15 reuse a warm browser process; the `alarms` binding is already resident → no race. |
| **12 separate cold** `./smackerel.sh test e2e-ext` invocations | 10 PASS / **2 FAIL** (runs 4, 7) | Each invocation cold-launches Chromium + a fresh worker → the spin-up binding window is exposed ~17% of the time. |

This is decisive: the race lives in the **cold-process-start** binding window,
so the meaningful proof (BEFORE and AFTER) uses **separate cold invocations**,
not `--repeat-each`.

## Fix Design (chosen: in-evaluate bounded readiness poll)

Wait for the binding **inside** the worker context, then fire the alarm:

```ts
triggerDrain: async () => {
  await sw.evaluate(
    (alarm) =>
      new Promise<void>((resolve, reject) => {
        const deadlineMs = Date.now() + 5000;
        const fireWhenReady = () => {
          if (typeof chrome?.alarms?.create === "function") {
            chrome.alarms.create(alarm, { when: Date.now() + 100 });
            setTimeout(resolve, 50);          // let onAlarm registration settle
            return;
          }
          if (Date.now() >= deadlineMs) {
            reject(new Error(
              "triggerDrain: chrome.alarms binding never became available within " +
              "5000ms of SW spin-up (MV3 service-worker lifecycle race)"));
            return;
          }
          setTimeout(fireWhenReady, 50);
        };
        fireWhenReady();
      }),
    DRAIN_ALARM,
  );
},
```

Why this is the cleanest of the three approaches the directive offered:

- **No cross-process TOCTOU gap.** The readiness check and the
  `chrome.alarms.create` call execute in the *same* worker evaluate, so the
  binding cannot disappear between "check" and "use". (A Playwright-side check —
  e.g. evaluate `() => !!chrome.alarms` then a second evaluate to create —
  reopens the exact window we are closing.)
- **Deterministic, not probabilistic.** It *waits for* the binding rather than
  reducing the odds. `chrome?.alarms?.create` optional-chaining also tolerates
  the (rarer) case where `chrome` itself is momentarily thin.
- **Loud on real breakage.** The 5s bound + `reject` means a genuinely
  misconfigured worker (e.g. the `alarms` permission removed from the manifest)
  fails with a clear, actionable error instead of hanging or silently passing.
- **Surgical.** It touches only the failing helper; the production-lifecycle
  `reloadServiceWorker()` heavyweight path is not needed here (we are not
  evicting state — just waking + draining a fresh worker).

## Alternatives Rejected

- **Add `retries: 1+` to `playwright.config.ts`** — REJECTED and explicitly
  forbidden by the directive. Retries mask a flake; they do not fix the race,
  and they would hide future real regressions on this path.
- **Weaken/relax the assertion** (`expect(badge).toBe("SETUP")`) — REJECTED.
  The SETUP-badge contract is the point of the test.
- **Playwright-side two-step "is `chrome.alarms` ready?" then "create"** —
  REJECTED. It reintroduces a cross-process check-then-use gap (the very window
  being closed).
- **`reloadServiceWorker()` before the drain** — REJECTED as heavier than
  needed: it does a full `chrome.runtime.reload()` (evicting in-memory state),
  which changes more than this test needs; the in-evaluate poll is the minimal
  deterministic fix for the observed binding race.

## Determinism / Robustness Notes

- The badge-readout poll later in `sideload_smoke.spec.ts` (`chrome.action
  .getBadgeText` in a 20× loop) was left unchanged: `chrome.action` is declared
  via the `action` manifest key (not a `permissions` entry) and it runs *after*
  `triggerDrain()` has already woken and exercised the worker, so the worker is
  warm by then. The 20/20 cold + 11/11 full-suite proof (report.md) confirms the
  helper fix alone is sufficient — no spec-side change was required.
- `playwright.config.ts` stays `workers: 1`, `fullyParallel: false`,
  `retries: 0` — unchanged.

## Cross-References

- `bug.md`, `spec.md`, `scopes.md`, `report.md`, `uservalidation.md`
- Fixed helper: `extensions/chrome-bridge/test/e2e/fixtures.ts` (`triggerDrain`)
- Product surface (confirmed correct, untouched): `extensions/chrome-bridge/src/background/index.ts`, `extensions/chrome-bridge/manifest.json`
