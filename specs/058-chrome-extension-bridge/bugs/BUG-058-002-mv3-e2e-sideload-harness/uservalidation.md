# User Validation: BUG-058-002

**Reported by:** Owner directive — "need proper long term solution, no shortcuts/simplifications" + "do everything … the build/deployment should be local" (resolving the remaining spec 058 blockers)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — Playwright fixtures load the REAL built extension into a REAL headless Chromium (`--headless=new`, `--load-extension`) and wait for the MV3 service worker; no mocked browser.
- [x] AC-2 — the ingest endpoint is a real per-test in-process HTTP server; NO `route()`/`intercept()`/`msw`/`nock` (the only match is the comment asserting their absence). Legitimate extension-client e2e tier.
- [x] AC-3 — created bookmark → POST `/v1/connectors/extension/ingest` with `Authorization: Bearer <token>` + correct `RawArtifact` shape; deny-pattern URL dropped (no POST); removal → `bookmark_event=removed` tombstone. (11/11 PASS.)
- [x] AC-4 — a 401 sets the `AUTH` badge (auth_terminal) and retains the queued item.
- [x] AC-5 — options page configure/persist/repopulate; bearer-token mask default + working reveal toggle; invalid input → visible error, not persisted.
- [x] AC-6 — automated sideload smoke (BLOCKER-4): sideload + SW register + manifest MV3 + permissions `[alarms, bookmarks, history, storage]` + restrictive CSP + options render + `SETUP` badge.
- [x] AC-7 — the suite runs via `./smackerel.sh test e2e-ext` (build then `playwright test`); self-contained (no live stack, no Compose project).

## Notes

This is the proper long-term solution for the spec 058 e2e + sideload blockers
(BLOCKER-1 + BLOCKER-4 of BUG-058-EXTERNAL-INFRA-MISSING), proven **locally** per
the owner's confirmation that build/deploy is local:

- It loads the REAL built MV3 bundle into a REAL browser and drives REAL
  `chrome.bookmarks`/`chrome.history`/options-page APIs — not a mock, not a unit
  shim. The transport runs over real HTTP to a real recording server.
- The removed-tombstone and deny-drop cases are proven via the REAL production
  worker-eviction lifecycle (`chrome.runtime.reload()` clears the in-memory dedup
  cache and recompiles the privacy filter on spin-up), not a faked clock, a
  contrived second device id, or a weakened dedup.
- The automated sideload smoke replaces the manual-only SCN-058-019 runbook with
  a deterministic load-and-assert that proves the artifact an operator sideloads
  actually loads (SW registers, manifest/permissions/CSP correct, options render,
  badge matrix entry).

Two deliberate no-shortcut decisions are recorded in `bug.md`: (1) a real HTTP
recording server instead of request interception (so the lane earns the e2e
classification honestly); (2) the worker-reload lifecycle instead of faking
state to defeat the bandwidth-saver dedup.

Scope boundary: this resolves BLOCKER-1 + BLOCKER-4. Together with BLOCKER-2
(live-Postgres, 2026-06-05) and BLOCKER-3 (BUG-058-001, 2026-06-07), all four
causes of BUG-058-EXTERNAL-INFRA-MISSING are discharged; the parent spec 058
`blockingDependencies` are reconciled in this change. Whether spec 058 promotes
out of `blocked` is left to a deliberate, verified state-transition pass.
