# BUG-073-002: web assistant client had no single-flight guard, no request timeout, and no busy affordance

**Status:** Resolved (single-flight + AbortController timeout + busy affordance via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Stochastic Quality Sweep Round 15 (parent: stochastic-quality-sweep) — `chaos`, parent-expanded
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/073-web-mobile-assistant-frontend/`
**Affected surface:** `web/pwa/assistant.js` (web chat client)

## Summary

The web assistant chat client (`web/pwa/assistant.js`) had three robustness
gaps surfaced by the round-15 chaos lens:

1. **Submit race.** The form submit handler called `submitText(text)` without
   awaiting it, and `dispatchTurn` had no re-entry guard, so a rapid
   double-submit (Send button or Enter key) could fire two overlapping turns
   with DIFFERENT `transport_message_id`s — violating the SCN-073 idempotency
   contract ("one logical turn per user action").
2. **No request timeout.** `postTurn` used `fetch` with no `AbortController`, so
   a hung or unreachable assistant endpoint left the turn pending indefinitely
   with the composer frozen and no feedback.
3. **No busy affordance.** The Send button was never disabled during a pending
   turn, so the UI gave no indication a request was in flight (and made the
   race trivially triggerable).

The mobile Dart client was audited and is clean (fail-loud config, secure
channels, no race in the renderer core).

## Mechanism (verified by reading the committed source)

- `form.addEventListener("submit", …)` → `submitText(text)` (not awaited).
- `dispatchTurn(requestBody)` had no `inFlight` guard; each call dispatched a
  new turn with a fresh `newTransportMessageID()`.
- `postTurn` → `await fetch(ENDPOINT, { … })` with no `signal`.

## Impact / Severity rationale (Medium)

- **Duplicate-turn risk:** rapid submits create overlapping turns with distinct
  message ids — the server cannot dedup them (dedup keys on
  `transport_message_id`), so the user can unintentionally send two turns.
- **Frozen-UI risk:** a hung endpoint leaves the composer pending forever with
  no error and no retry affordance.
- **No security impact:** same-origin HttpOnly cookie auth is unchanged; no
  storage/XSS regression (the storage guard still passes).

## Fix (delivered)

1. **Single-flight `inFlight` guard:** `dispatchTurn` early-returns if a turn is
   already running; the submit handler also early-returns (without clearing the
   composer) when `inFlight`.
2. **`AbortController` timeout:** `postTurn` aborts the fetch after
   `TURN_TIMEOUT_MS` (30 s) and surfaces a retryable "request timed out" error.
3. **Busy affordance:** `setComposerBusy(true/false)` disables the Send button
   and sets `aria-busy` while a turn is in flight.

## Testing note (why a Go source-contract guard)

The assistant page is auth-gated; the disposable e2e-ui stack does not seed a
logged-in fixture, so the existing Playwright specs are served-route stubs and
the real interaction coverage is server-side Go e2e — a browser interaction
test cannot reach the composer. Following the established
`assistant_storage_guard_test.go` pattern, the fix is locked by a Go
source-contract guard (`assistant_robustness_guard_test.go`) that asserts the
single-flight guard, the AbortController timeout, and the busy affordance are
present, with an adversarial twin proving it detects their removal.

## Cross-References

- Client: `web/pwa/assistant.js`
- Guard: `web/pwa/tests/assistant_robustness_guard_test.go`
- Sibling guard pattern: `web/pwa/tests/assistant_storage_guard_test.go`
- Parent spec/design: `../../spec.md`, `../../design.md`
