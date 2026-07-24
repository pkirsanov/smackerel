# Bug: [BUG-073-006] Auth Rejection Leaves Blank Assistant Response

## Summary

When an Assistant POST is rejected by authentication before the facade runs, the UI preserves the user message but leaves a blank response with no actionable error or retry.

## Severity

High (S2): a primary interaction appears to accept input but gives no truthful terminal outcome.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No browser, HTTP, or facade execution occurred in this planning-only invocation.

## Reproduction Steps

1. Open the Assistant UI and submit a message with a missing, invalid, or production-rejected session.
2. Observe that the POST is rejected before the facade executes.
3. Observe that the user message remains in the transcript.
4. Observe a blank Assistant response area with no accessible error, retry, re-authentication action, or preserved resend state.

## Expected Behavior

Every non-2xx response, network failure, timeout, and response-schema failure produces an accessible inline terminal error paired with the user message. The transcript and unsent/retryable input remain available, retry is explicit and deduplicated, auth failures offer re-authentication, and no failure is rendered as capture, success, or a blank response.

## Actual Behavior

The UI has no terminal representation for the pre-facade rejection and leaves the conversation visually incomplete.

## Outcome Contract

**Intent:** Ensure every submitted Assistant turn has a visible pending state followed by exactly one honest answer, refusal, confirmation, or typed error.

**Success Signal:** Real-stack Playwright exercises 401/403, 5xx, timeout, network, malformed-schema, and successful turns; each user message remains paired with an accessible outcome and retry/re-auth action where appropriate.

**Hard Constraints:** No transcript/input loss, no duplicate turn on retry, no false capture/success, no internal error or credential exposure, no request interception in live E2E, and no durable storage of sensitive transcript content beyond existing policy.

**Failure Condition:** Any rejected turn leaves a blank row, disappears, announces success/capture, loses retry context, duplicates on retry, or exposes sensitive response details.

## Impact And Dependencies

- Blocks the Assistant journey in `specs/106-coherent-product-experience`.
- Authenticated browser proof depends on `BUG-070-001-production-credential-session-paseto-split`.
- Product-level acceptance depends on this packet and spec 104 Scope 8.

## Root Cause Ownership

`bubbles.design` must confirm the client request lifecycle, transcript state model, response parsing boundary, auth handling, retry idempotency, and facade/non-facade error envelope before implementation.
