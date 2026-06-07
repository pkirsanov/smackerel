# Spec: BUG-058-002 — local MV3 Playwright e2e harness + automated sideload smoke

## Expected Behavior

Spec 058's Chrome Extension Bridge MUST have a real-browser end-to-end harness
that loads the built MV3 extension into a real headless Chromium and proves the
genuine capture → queue → drain → POST pipeline, the options-page setup flow, the
auth-failure classification, and an automated sideload smoke — all runnable
**locally** through the repo's one CLI surface (`./smackerel.sh test e2e-ext`).

## Actual Behavior

`extensions/chrome-bridge/test/e2e/` did not exist. Behavioral coverage stopped
at the vitest **unit** tier (each module in isolation); nothing loaded the built
MV3 bundle into a real browser. SCN-058-019 sideload verification was a
manual-only runbook. See `bug.md`. (Background: MV3 service workers +
`chrome.bookmarks`/`history` only initialise under the **new** headless mode.)

## Acceptance Criteria

1. **AC-1 (real-browser harness):** Playwright fixtures load the REAL built
   extension into a REAL headless Chromium (`--headless=new`,
   `--load-extension`), wait for the MV3 service worker, and drive the extension
   via `chrome.bookmarks`/`chrome.history`/options-page — no mocked browser.
2. **AC-2 (real transport, not interception):** the ingest endpoint is a real
   per-test in-process HTTP server the extension POSTs to over real HTTP; the
   harness uses NO `route()`/`intercept()`/`msw`/`nock`. This earns the
   extension-client e2e classification.
3. **AC-3 (bookmark roundtrip, SCN-058-010..013):** a real `bookmarks.create`
   is POSTed to `/v1/connectors/extension/ingest` with `Authorization: Bearer
   <token>` and the correct `RawArtifact` shape (`source_id`, `source_ref`,
   `content_type`, `title`, `url`, `metadata.{source_device_id, bookmark_event,
   bookmark_id, client_event_id}`); a deny-pattern URL is dropped before it
   leaves the browser; a removal emits a `bookmark_event=removed` tombstone.
4. **AC-4 (auth failure, SCN-058-014):** a 401 from the ingest endpoint sets the
   `AUTH` badge (auth_terminal) and retains the queued item.
5. **AC-5 (options setup):** the options page configures + persists + repopulates
   the operator settings, masks the bearer token by default with a working reveal
   toggle, and rejects invalid input with a visible error (no persistence).
6. **AC-6 (automated sideload smoke, BLOCKER-4):** the built extension sideloads,
   its MV3 service worker registers, the manifest is MV3 with the minimum
   permissions `[alarms, bookmarks, history, storage]` and a restrictive CSP, the
   options page renders, and an unconfigured install shows the `SETUP` badge.
7. **AC-7 (one CLI surface):** the suite runs via
   `./smackerel.sh test e2e-ext` (build the extension, then `playwright test`),
   self-contained (no live stack, no Compose project), exit code propagated.

## Out of Scope

- The server-side ingest contract (covered by the Go live-Postgres integration
  tests — BLOCKER-2, resolved 2026-06-05).
- The HTMX admin scaffold (BLOCKER-3 — resolved by BUG-058-001).
- A CI-runner variant: the owner confirmed build/deploy is local; the lane is
  proven locally and remains CI-portable, but no CI wiring is added here.
- History-visit roundtrip e2e (the dwell estimate is 0 in v1 per design §4; the
  history listener wiring is unit-covered — out of scope for this packet).

## Cross-References

- Bug detail + no-shortcut decisions: `bug.md`
- Tracking bug: `../BUG-058-EXTERNAL-INFRA-MISSING/` (BLOCKER-1, BLOCKER-4)
- Sibling packet: `../BUG-058-001-admin-devices-html-surface/` (BLOCKER-3)
- Production surface under test: `extensions/chrome-bridge/src/background/`, `src/options/`
