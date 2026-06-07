# Report: BUG-058-002 — local MV3 Playwright e2e harness + automated sideload smoke

**Workflow mode:** `bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`)
**Owner:** `bubbles.workflow`
**Resolved:** 2026-06-07
**Resolves:** `../BUG-058-EXTERNAL-INFRA-MISSING/` BLOCKER-1 + BLOCKER-4

## Summary

Spec 058 had no real-browser proof of its MV3 extension: behavioral coverage
stopped at the vitest unit tier, and SCN-058-019 sideload verification was a
manual-only runbook. This delivers a local Playwright e2e harness under
`extensions/chrome-bridge/test/e2e/` that loads the REAL built extension into a
REAL headless Chromium (`--headless=new`) and proves the genuine capture → queue
→ drain → POST pipeline, the options-page flow, the auth-failure classification,
and an automated sideload smoke — wired into the repo's one CLI surface as
`./smackerel.sh test e2e-ext`. Resolves BLOCKER-1 + BLOCKER-4.

## Root Cause

`extensions/chrome-bridge/test/e2e/` did not exist. A contributing reason it was
thought infeasible: MV3 service workers and `chrome.bookmarks`/`chrome.history`
do not initialise under Chromium's OLD headless mode (the worker never starts);
they require the NEW headless mode.

## Fix

New Playwright harness (`fixtures.ts`, `playwright.config.ts`, four `*.spec.ts`),
a self-contained lane wrapper (`scripts/runtime/extension-e2e.sh`), and the
`./smackerel.sh test e2e-ext` dispatcher. The ingest endpoint is a real per-test
in-process HTTP recording server (NOT request interception), so the extension's
real `fetch`, bearer header, status classification, and drain all execute.

## Test Evidence

### Canonical run through the repo CLI surface (`./smackerel.sh test e2e-ext`)

```
$ ./smackerel.sh test e2e-ext
Building chrome-bridge extension…
Running MV3 extension e2e suite (real headless Chromium)…
Running 11 tests using 1 worker

  ✓  1 …e ingest endpoint sets the AUTH badge and retains the queued item (1.1s)
  ✓  2 …extension/ingest with the bearer token and correct artifact shape (1.0s)
  ✓  3 …tern URL is dropped before it leaves the browser (no ingest POST) (3.8s)
  ✓  4 …d bookmark emits a tombstone artifact with bookmark_event=removed (1.6s)
  ✓  5 …re the extension through the options page and the values persist (939ms)
  ✓  6 …ld is masked by default and the Reveal button toggles visibility (827ms)
  ✓  7 …figuration is rejected with a visible error and is not persisted (799ms)
  ✓  8 …e built extension sideloads and its MV3 service worker registers (530ms)
  ✓  9 …nifest is MV3 with the minimum permissions and a restrictive CSP (625ms)
  ✓  10 …pec.ts:41:1 › the options page renders for a sideloaded install (659ms)
  ✓  11 …trix: an unconfigured install shows SETUP after a drain attempt (507ms)

  11 passed (13.4s)
```

11/11 e2e tests PASS in real headless Chromium. Tests 2/3/4 are the bookmark
roundtrip (SCN-058-010..013: created→POST artifact shape + bearer; deny-drop;
removed→tombstone); test 1 is auth failure (SCN-058-014: 401 → AUTH badge); tests
5/6/7 are the options flow; tests 8/9/10/11 are the automated sideload smoke
(BLOCKER-4 / SCN-058-019: sideload + SW register + manifest MV3/perms/CSP +
options render + SETUP badge).

## Code Diff Evidence

```
$ git status --porcelain extensions/chrome-bridge scripts/runtime/extension-e2e.sh smackerel.sh
 M extensions/chrome-bridge/.gitignore
 M extensions/chrome-bridge/package-lock.json
 M extensions/chrome-bridge/package.json
 M extensions/chrome-bridge/tsconfig.json
 M smackerel.sh
?? extensions/chrome-bridge/test/e2e/
?? scripts/runtime/extension-e2e.sh
```

`package.json` adds `@playwright/test@1.49.1` (matching the repo's pinned
chromium-1148 from spec 077) + a `test:e2e` script; `tsconfig.json` excludes
`test/e2e/**` from the unit `tsc` (Playwright transpiles its own specs);
`smackerel.sh` adds the `e2e-ext` dispatcher; `test/e2e/` holds `fixtures.ts`,
`playwright.config.ts`, and the four spec files.

### Validation Evidence

**Focused bookmark-roundtrip run**

```
$ npx playwright test --config test/e2e/playwright.config.ts bookmark_roundtrip.spec.ts
Running 3 tests using 1 worker

  ✓  1 …extension/ingest with the bearer token and correct artifact shape (1.4s)
  ✓  2 …tern URL is dropped before it leaves the browser (no ingest POST) (4.4s)
  ✓  3 …d bookmark emits a tombstone artifact with bookmark_event=removed (1.4s)

  3 passed (8.3s)
```

**Unit suite + typecheck unaffected by the harness**

```
$ npm test
 Test Files  6 passed (6)
      Tests  39 passed (39)
$ npm run typecheck   # tsc --noEmit, test/e2e excluded
TYPECHECK_EXIT=0
```

The vitest unit suite remains 39/39 PASS and `tsc` exits 0 — the new e2e harness
does not disturb the unit lane (vitest globs only `test/unit/**`; `tsc` excludes
`test/e2e/**`).

### Audit Evidence

```
$ git check-ignore extensions/chrome-bridge/test-results extensions/chrome-bridge/dist extensions/chrome-bridge/node_modules
extensions/chrome-bridge/test-results
extensions/chrome-bridge/dist
extensions/chrome-bridge/node_modules
$ grep -rnE "route\(|\.intercept\(|msw|nock|fulfill\(" test/e2e/
test/e2e/fixtures.ts:11:// (no route()/intercept()/msw/nock) — the extension's fetch, Authorization
```

The only interception-pattern match is the explanatory doc-comment asserting
their absence — there is no executable `route()`/`intercept()`/`msw`/`nock`, so
the suite legitimately drives the real extension transport (extension-client e2e
tier). Playwright transient outputs (`test-results/`, `playwright-report/`,
`.playwright/`) and `dist/`/`node_modules/` are git-ignored; no generated config
is staged. Parent reconciliation in this change: `specs/058-chrome-extension-bridge/state.json`
(`blockingDependencies` BLOCKER-1 + sideload entries → resolved) and
`bugs/BUG-058-EXTERNAL-INFRA-MISSING/state.json` (BLOCKER-1 + BLOCKER-4 → resolved).
No `.github/bubbles` framework files touched.

## Completion Statement

Spec 058 now has a local real-browser MV3 e2e harness under
`extensions/chrome-bridge/test/e2e/` and an automated sideload smoke, runnable
through `./smackerel.sh test e2e-ext`. The canonical run executes 11 tests in
real headless Chromium — bookmark roundtrip (created→ingest POST with the bearer
token and correct `RawArtifact` shape; deny-pattern drop; removed→tombstone via
the real worker-eviction lifecycle), auth-failure (401 → `AUTH` badge + item
retained), the options-page flow (configure/persist/repopulate, mask/reveal,
invalid→error), and the sideload smoke (sideload + SW register + manifest
MV3/perms/CSP + options render + `SETUP` badge) — all 11/11 PASS. The lane is
self-contained (a per-test in-process recording HTTP server, real HTTP, no
interception) so it needs no live stack; the extension vitest unit suite remains
39/39 PASS and `tsc` exits 0. Scope 1 DoD is complete (10/10). BUG-058-002 is
Done and discharges BUG-058-EXTERNAL-INFRA-MISSING BLOCKER-1 + BLOCKER-4; with
BLOCKER-2 (2026-06-05) and BLOCKER-3 (BUG-058-001, 2026-06-07) already resolved,
all four causes are discharged and the parent spec 058 blocking-dependencies are
reconciled in this change.
