# Scopes: BUG-058-002

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bugfix-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).
Resolves BLOCKER-1 + BLOCKER-4 of `../BUG-058-EXTERNAL-INFRA-MISSING/`.

## Scope 1 — Local MV3 Playwright e2e harness + automated sideload smoke

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `extensions/chrome-bridge/test/e2e/fixtures.ts`: Playwright fixtures load the REAL built extension into a REAL headless Chromium (`--headless=new`, `--load-extension`), wait for the MV3 service worker, and expose `configure`/`openOptions`/`triggerDrain`/`reloadServiceWorker`; the ingest endpoint is a real per-test in-process HTTP recording server (no `route()`/`msw`/`nock`)
      → Evidence: report.md `### Test Evidence` (`$ ./smackerel.sh test e2e-ext` → 11/11 PASS); report.md `### Audit Evidence` (grep finds no interception primitives)
- [x] `bookmark_roundtrip.spec.ts` (SCN-058-010..013): created bookmark → POST `/v1/connectors/extension/ingest` with `Authorization: Bearer <token>` + correct `RawArtifact` shape; deny-pattern URL dropped (no POST); removed bookmark → `bookmark_event=removed` tombstone
      → Evidence: report.md `### Test Evidence` (specs 2,3,4 PASS); report.md `### Validation Evidence` (focused `bookmark_roundtrip.spec.ts` run → 3/3 PASS)
- [x] `auth_failure.spec.ts` (SCN-058-014): a 401 from the ingest endpoint sets the `AUTH` badge (auth_terminal) and retains the queued item
      → Evidence: report.md `### Test Evidence` (spec 1 PASS)
- [x] `options_setup.spec.ts`: options page configure/persist/repopulate; bearer-token mask default + working reveal toggle; invalid input → visible error, not persisted
      → Evidence: report.md `### Test Evidence` (specs 5,6,7 PASS)
- [x] `sideload_smoke.spec.ts` (BLOCKER-4): built extension sideloads; MV3 SW registers; manifest is MV3 with permissions `[alarms, bookmarks, history, storage]` + restrictive CSP; options page renders; unconfigured install → `SETUP` badge
      → Evidence: report.md `### Test Evidence` (specs 8,9,10,11 PASS)
- [x] `scripts/runtime/extension-e2e.sh` lane wrapper + `./smackerel.sh test e2e-ext` dispatcher: build the extension then run Playwright; self-contained (no live stack, no Compose project); fail-loud on missing node/npm; exit code propagated
      → Evidence: report.md `### Test Evidence` (the canonical run is invoked through `./smackerel.sh test e2e-ext`); report.md `### Validation Evidence`
- [x] `@playwright/test@1.49.1` added as a devDependency (matches the repo's pinned chromium-1148 from spec 077); `test:e2e` npm script added; `test/e2e/**` excluded from the unit `tsconfig` so `tsc`/`vitest` are unaffected
      → Evidence: report.md `### Code Diff Evidence` (package.json + tsconfig.json diff); report.md `### Validation Evidence` (vitest unit suite still 39/39 PASS)
- [x] `SCN-058-002-01..05` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Playwright transient outputs git-ignored (`test-results/`, `playwright-report/`, `.playwright/`); no `dist/`, `node_modules/`, or `config/generated/` staged
      → Evidence: report.md `### Audit Evidence` (`git check-ignore` + `git status --porcelain`)
- [x] Parent spec 058 `blockingDependencies` + tracking bug BLOCKER-1/BLOCKER-4 reconciled to resolved in the same change
      → Evidence: report.md `### Audit Evidence` (reconciled state files listed)

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-058-002-01 | created bookmark → ingest POST with bearer + artifact shape | extensions/chrome-bridge/test/e2e/bookmark_roundtrip.spec.ts | e2e (extension-client) | SCN-058-002-01 |
| T-058-002-02 | deny-pattern URL dropped (no POST); removed → tombstone | extensions/chrome-bridge/test/e2e/bookmark_roundtrip.spec.ts | e2e (extension-client) | SCN-058-002-02 |
| T-058-002-03 | 401 → AUTH badge + item retained | extensions/chrome-bridge/test/e2e/auth_failure.spec.ts | e2e (extension-client) | SCN-058-002-03 |
| T-058-002-04 | options configure/persist/repopulate + mask/reveal + invalid→error | extensions/chrome-bridge/test/e2e/options_setup.spec.ts | e2e (extension-client) | SCN-058-002-04 |
| T-058-002-05 | sideload + SW register + manifest MV3/perms/CSP + options render + SETUP badge | extensions/chrome-bridge/test/e2e/sideload_smoke.spec.ts | e2e (extension-client) | SCN-058-002-05 |

### Non-Goals

- A CI-runner variant (build/deploy is local; the lane stays CI-portable).
- History-visit roundtrip e2e (dwell estimate is 0 in v1; unit-covered).
- Re-testing the server ingest contract (BLOCKER-2 live-Postgres integration).
