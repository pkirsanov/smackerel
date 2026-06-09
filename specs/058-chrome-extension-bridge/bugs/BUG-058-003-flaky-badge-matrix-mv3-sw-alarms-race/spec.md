# Spec: BUG-058-003 — flaky `badge matrix` e2e test (MV3 `chrome.alarms` spin-up race)

## Expected Behavior

The spec 058 MV3 Chrome Extension Bridge e2e harness (`./smackerel.sh test
e2e-ext`) MUST be **deterministic**. In particular, the `triggerDrain()` test
helper — which fires the `smackerel-bridge-drain` alarm from the service-worker
context — MUST be robust to the real MV3 service-worker lifecycle (idle
termination + cold re-spin), so the `badge matrix` test (and any test that
drives a drain) passes reliably on every run, including cold-process starts.

## Actual Behavior

`triggerDrain()` called `chrome.alarms.create(...)` with no readiness guard.
On a cold MV3 service-worker spin-up there is a brief window where the worker's
global scope and the base `chrome` namespace exist (so `sw.evaluate` runs) but
the permission-gated `chrome.alarms` binding is not yet installed, so
`chrome.alarms.create` dereferences `undefined` and throws
`TypeError: Cannot read properties of undefined (reading 'create')`. With
`retries: 0`, this intermittently (~1-in-6 to ~1-in-3 cold runs) fails the whole
`./smackerel.sh test e2e-ext` suite. See `bug.md` and `report.md`.

## Acceptance Criteria

1. **AC-1 (robust helper):** `triggerDrain()` waits for `chrome.alarms.create`
   to be a function before calling it, performing the readiness check **inside**
   the `sw.evaluate` worker context (no cross-process check-then-call gap), then
   fires the drain alarm.
2. **AC-2 (bounded + loud, no masking):** the wait is bounded (≤ 5s, 50ms poll)
   and **rejects with a clear error** on timeout, so a genuinely-broken worker
   surfaces as a real failure rather than a silent hang or a flake.
3. **AC-3 (no retry-masking):** `test/e2e/playwright.config.ts` keeps
   `retries: 0`; the flake is fixed at the source (the race), not papered over
   with Playwright retries.
4. **AC-4 (assertion preserved):** `sideload_smoke.spec.ts`'s
   `expect(badge).toBe("SETUP")` is unchanged; the SETUP-badge contract is not
   weakened.
5. **AC-5 (proven non-flaky):** the isolated `badge matrix` test passes on **at
   least 10 consecutive cold invocations** of `./smackerel.sh test e2e-ext --
   sideload_smoke.spec.ts --grep "badge matrix"` (delivered: 20/20), and the
   **full** `./smackerel.sh test e2e-ext` suite passes 11/11.
6. **AC-6 (product code untouched):** no change to `src/background/index.ts`,
   `manifest.json`, or any production source — the product alarms wiring and the
   `alarms` permission are already correct.
7. **AC-7 (one CLI surface):** all verification runs through
   `./smackerel.sh test e2e-ext` (no ad-hoc `playwright`/`npx` invocations as the
   sanctioned workflow).

## Out of Scope

- Unblocking the parent spec 058 — it stays `blocked` on the keyless-OIDC
  `cosign verify-blob` against a real Rekor log (operator/CI-release-gated).
- Editing the parent spec 058 `spec.md`/`design.md`/`scopes.md` planning truth.
- Any product-code change (the `alarms` permission + top-level
  `chrome.alarms.create` registration are correct).
- Adding `retries` to `playwright.config.ts` (explicitly forbidden as a fix).
- Re-architecting the other 10 e2e tests (they pass; only the cold-start drain
  helper is hardened).

## Cross-References

- Bug detail + reproduction: `bug.md`
- Root cause + fix design: `design.md`
- Fixed helper: `extensions/chrome-bridge/test/e2e/fixtures.ts` (`triggerDrain`)
- Sibling packet (authored the harness): `../BUG-058-002-mv3-e2e-sideload-harness/`
