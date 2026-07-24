# Bug: [BUG-002-006] Search HTMX SRI Blocks Submit

## Summary

The Search page renders, but the browser blocks HTMX because the declared SHA-384 integrity value does not match the loaded dependency; entering a query consequently sends zero `/search` requests.

## Severity

High (S2): a primary knowledge-retrieval journey appears available but cannot submit.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No browser or network reproduction was executed by `bubbles.bug` in this planning-only invocation.

## Reproduction Steps

1. Open the authenticated Search page in the deployed product.
2. Observe the browser's subresource-integrity rejection for the HTMX script because the SHA-384 value is wrong.
3. Enter a non-empty query and submit with keyboard or pointer.
4. Observe that the browser sends zero `/search` requests and no loading, results, empty, or error state completes the journey.

## Expected Behavior

The enhancement dependency loads under the active CSP/SRI contract or is self-hosted. Keyboard and pointer submission issue one real request, preserve the query, and render explicit loading followed by results, no matches, authentication failure, or actionable error. A blocked enhancement must not remove the accessible baseline form submission path.

## Actual Behavior

The integrity check blocks HTMX, and the page has no effective fallback submission behavior; Search remains visually present but inert.

## Outcome Contract

**Intent:** Make Search submit reliably with or without client enhancement and represent every terminal outcome truthfully.

**Success Signal:** Playwright enters a query, observes exactly one real `/search` request, then asserts loading and a concrete results, no-match, or error DOM state for keyboard and pointer paths.

**Hard Constraints:** CSP and SRI remain effective; no unsafe bypass, unpinned remote dependency, request interception, or canned search result is introduced; sensitive query text is not logged.

**Failure Condition:** Search still sends zero or duplicate requests, works only when integrity enforcement is weakened, lacks an accessible fallback, or converts auth/service failure into no results.

## Impact And Dependencies

- Blocks the Search journey in `specs/106-coherent-product-experience`.
- Blocks product journey acceptance in `BUG-102-001-product-journey-acceptance-gap`.
- Independent of the Digest defect in sibling `BUG-002-007`.

## Root Cause Ownership

The wrong SRI value and zero-request symptom are preserved as reported. `bubbles.design` must confirm dependency ownership, CSP source policy, progressive-enhancement boundary, and request lifecycle before implementation.
