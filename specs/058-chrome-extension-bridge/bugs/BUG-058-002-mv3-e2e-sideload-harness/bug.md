# BUG-058-002: local MV3 Playwright e2e harness + automated sideload smoke (BLOCKER-1 + BLOCKER-4)

**Status:** Resolved (real-browser MV3 e2e harness + automated sideload smoke via bugfix-fastlane — see report.md)
**Severity:** Blocker (discharges the final two `blocked` causes of BUG-058-EXTERNAL-INFRA-MISSING: BLOCKER-1 + BLOCKER-4)
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "need proper long term solution, no shortcuts/simplifications" + "do everything … the build/deployment should be local" (resolving the remaining spec 058 blockers)
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/058-chrome-extension-bridge/`
**Tracking bug:** `../BUG-058-EXTERNAL-INFRA-MISSING/` (BLOCKER-1, BLOCKER-4)

## Summary

Spec 058 stayed `blocked` after BUG-058-001 (BLOCKER-3) because its two
remaining gating items were assumed to require infrastructure that "cannot be
validated in this environment":

- **BLOCKER-1** — the **MV3-extension-specific Playwright e2e harness** under
  `extensions/chrome-bridge/test/e2e/` did not exist, leaving SCN-058-010..015
  e2e rows, the bookmark-roundtrip path, and `auth_failure` with no real-browser
  proof. (The cross-cutting `web/pwa` Playwright harness shipped via spec 077,
  but that does not drive the actual Chrome extension.)
- **BLOCKER-4** — the SCN-058-019 sideload walkthrough was a **manual-only**
  8-step runbook with no automated verification path.

Both are now resolved **locally** — the owner confirmed build + deployment is
local (the `knb` sibling overlay owns the deploy target), so the e2e harness was
built and proven on this machine against a real headless Chromium, not deferred
to a hypothetical CI runner.

## Mechanism (what was missing)

- `extensions/chrome-bridge/test/e2e/` did not exist. The extension shipped with
  a vitest **unit** suite (39 specs) that exercises each module in isolation
  (privacy filter, dedup keyer, backoff curve, transport classifier, queue
  drainer) but **nothing loaded the built MV3 bundle into a real browser** to
  prove the end-to-end capture → queue → drain → POST pipeline, the options-page
  setup flow, or the sideload-load surface.
- A key reason this was thought infeasible: under Chromium's **old** headless
  mode, MV3 service workers and the `chrome.bookmarks`/`chrome.history` APIs do
  not initialise (the service worker never starts; the extension id resolves to
  null). The harness was blocked on this until the **new** headless mode
  (`--headless=new`) was identified as the requirement.

## Fix (delivered — the proper local foundation)

1. **`extensions/chrome-bridge/test/e2e/fixtures.ts`** — Playwright fixtures that
   load the REAL built extension (`dist/extension/chrome-bridge`) into a REAL
   headless Chromium via `chromium.launchPersistentContext(dir, { headless:
   false, args: ['--headless=new', '--load-extension=…'] })`, wait for the MV3
   service worker, and expose `configure()`, `openOptions()`, `triggerDrain()`,
   and `reloadServiceWorker()`. The ingest endpoint is a REAL per-test in-process
   recording HTTP server the extension POSTs to over real HTTP — **NOT** request
   interception (`route()`/`msw`/`nock`), so the extension's `fetch`,
   `Authorization` header, retry classification, and drain all run for real. This
   is a legitimate extension-**client** e2e tier (the server contract is
   separately covered by the Go live-Postgres integration tests, BLOCKER-2).
2. **`bookmark_roundtrip.spec.ts`** — SCN-058-010..013: a real
   `chrome.bookmarks.create` flows through the genuine background pipeline and is
   POSTed to `/v1/connectors/extension/ingest` with the bearer token and the
   correct `RawArtifact` shape; a deny-pattern URL is dropped before it leaves
   the browser; a removal emits a `bookmark_event=removed` tombstone.
3. **`auth_failure.spec.ts`** — SCN-058-014: a 401 from the ingest endpoint sets
   the `AUTH` badge (auth_terminal classification) and retains the queued item.
4. **`options_setup.spec.ts`** — the operator options-page flow: configure +
   persist + repopulate, the bearer-token mask/reveal toggle, and validation
   rejection.
5. **`sideload_smoke.spec.ts`** (BLOCKER-4) — the **automated** local sideload
   smoke that replaces the manual-only walkthrough: the built extension
   sideloads, its MV3 service worker registers, the manifest is MV3 with the
   minimum permissions + restrictive CSP, the options page renders, and the badge
   shows `SETUP` for an unconfigured install (the badge-state-matrix entry point).
6. **Lane wiring** — `scripts/runtime/extension-e2e.sh` + the
   `./smackerel.sh test e2e-ext` dispatcher run the suite through the repo's one
   CLI surface (build the extension, then `playwright test`). The lane is
   self-contained (per-test recording server; no live stack, no Compose project).

## Deliberate no-shortcut decisions (recorded)

- **Real browser, real HTTP, no mocks.** The recording server is a real local
  HTTP server, not request interception — so the lane legitimately earns the
  e2e-client classification (per the repo's live-stack-authenticity rule).
- **`reloadServiceWorker()` instead of a fake clock for the removed-tombstone
  and deny-filter cases.** For bookmarks the client dedup bucket is fixed at `0`
  and the key omits the event, so a rapid create→remove of the same URL is
  (correctly) collapsed by the bandwidth-saver. The harness reproduces the REAL
  production lifecycle — `chrome.runtime.reload()` evicts the worker and clears
  its in-memory dedup cache between the create and the remove — rather than
  contriving a second device id or weakening the dedup. The same reload makes a
  freshly-configured privacy filter deterministically active before the next
  event (no reliance on `storage.onChanged` propagation timing).
- **Skipped browser download on install.** `@playwright/test@1.49.1` matches the
  repo's pinned Chromium revision (chromium-1148, already cached by spec 077), so
  the lane reuses it; the install skips browser downloads and never invokes the
  sudo `--with-deps` path.

## Scope boundary

This packet resolves **BLOCKER-1 + BLOCKER-4**. Together with BUG-058-001
(BLOCKER-3, resolved 2026-06-07) and BLOCKER-2 (live-Postgres, resolved
2026-06-05), all four causes of BUG-058-EXTERNAL-INFRA-MISSING are now
discharged. The parent spec 058 `blockingDependencies` are reconciled in this
change; whether spec 058 promotes out of `blocked` is left to a deliberate
state-transition pass (verified, not forced).

## Cross-References

- Harness: `extensions/chrome-bridge/test/e2e/{fixtures,playwright.config}.ts`
- Specs: `extensions/chrome-bridge/test/e2e/{bookmark_roundtrip,auth_failure,options_setup,sideload_smoke}.spec.ts`
- Lane: `scripts/runtime/extension-e2e.sh`, `./smackerel.sh test e2e-ext`
- Tracking bug: `../BUG-058-EXTERNAL-INFRA-MISSING/` (BLOCKER-1, BLOCKER-4)
- Sibling packet: `../BUG-058-001-admin-devices-html-surface/` (BLOCKER-3)
- Production surface under test: `extensions/chrome-bridge/src/background/{index,transport,bookmarks,dedup_local}.ts`, `src/options/index.ts`
