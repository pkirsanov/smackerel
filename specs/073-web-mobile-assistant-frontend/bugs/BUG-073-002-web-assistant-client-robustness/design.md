# Design: BUG-073-002

## Problem

`web/pwa/assistant.js` dispatched turns without a single-flight guard, fetched
without a timeout, and never showed a busy state. See `bug.md` for the
mechanism and the auth-gating that forces a source-contract guard rather than a
browser interaction test.

## Change

1. **Single-flight.** Add a module `inFlight` flag. `dispatchTurn` early-returns
   when `inFlight` is set, otherwise sets it (and clears it in `finally`). The
   submit handler also early-returns when `inFlight` WITHOUT clearing the
   composer, so an ignored submit does not lose the user's typed text. This
   covers every dispatch entry point (submit, confirm, disambiguation, retry).
2. **Request timeout.** `postTurn` constructs an `AbortController`, passes
   `signal: controller.signal` to `fetch`, and `setTimeout(() =>
   controller.abort(), TURN_TIMEOUT_MS)`. On `AbortError` it throws a
   "request timed out" error (retryable via the existing pendingTurn path); a
   `finally` clears the timer. `TURN_TIMEOUT_MS = 30000` is a client UX
   constant, not environment-specific runtime config.
3. **Busy affordance.** `setComposerBusy(busy)` toggles
   `assistant-send-btn.disabled` and `aria-busy`; `dispatchTurn` calls it `true`
   on entry and `false` in `finally`.

## Why this shape

- The `inFlight` early-return in `dispatchTurn` is the correctness mechanism
  (covers all entry points); the disabled button is the visible affordance.
- Keeping `pendingTurn` semantics intact means retry still reuses the original
  `transport_message_id` (SCN-073-A03 unchanged).
- An `AbortController` bounds both the fetch and the body read under one timer.

## Test Tier Rationale

The assistant page is auth-gated and the disposable e2e-ui stack seeds no
logged-in fixture (the existing Playwright specs are served-route stubs; real
coverage is server-side Go e2e). A browser interaction test cannot reach the
composer. The faithful, runnable lock is therefore a Go source-contract guard
(`assistant_robustness_guard_test.go`) — the same pattern as
`assistant_storage_guard_test.go` — asserting the three mechanisms are present,
with an adversarial twin proving it detects their removal. `node --check`
validates syntax; the existing storage + codegen-drift guards confirm no
regression.

## Blast Radius

- `web/pwa/assistant.js` — `inFlight` flag, `TURN_TIMEOUT_MS`, `setComposerBusy`,
  `postTurn` AbortController, `dispatchTurn` guard + busy toggle, submit-handler
  guard.
- `web/pwa/tests/assistant_robustness_guard_test.go` (new) — source-contract
  guard + adversarial twin.
- No server change, no config, no schema; mobile client untouched.

## Alternatives Considered

- **Disable the button only (no inFlight guard).** Rejected: the Enter key and
  the non-submit entry points (retry/confirm/disambiguation) bypass a
  button-only guard; `inFlight` in `dispatchTurn` is the robust mechanism.
- **A whole-page Playwright interaction test.** Rejected: blocked by the
  auth-gated disposable stack (the reason the existing specs are stubs).
