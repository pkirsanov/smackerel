# Bug: [BUG-002-007] Digest Date Scan Produces False Empty

## Summary

The database contains a current approximately 380-word digest, but the legacy Digest page shows "No digest generated yet" because a date is scanned into a string, the scan errors, and the handler silently substitutes a false empty state.

## Severity

High (S2): current stored knowledge is hidden and a database/type failure is misrepresented as legitimate absence.

## Status And Provenance

Reported from operator-supplied current-session historical input. **Claim Source:** interpreted. No database query, page load, or source execution was performed by `bubbles.bug` in this planning-only invocation.

## Reproduction Steps

1. Ensure the production database contains a current non-empty digest of roughly 380 words.
2. Open the authenticated legacy Digest page.
3. Observe that scanning the digest date into a string errors.
4. Observe that the handler swallows the database error and renders "No digest generated yet" instead of the stored digest or an error.

## Expected Behavior

The handler scans date/time values into an appropriate typed value, surfaces database/decoding failures explicitly, and distinguishes current digest, quiet digest, stale digest, true never-generated empty, unauthorized, and error states.

## Actual Behavior

A current stored digest is hidden behind false first-use copy, making the product and operator believe no digest exists.

## Outcome Contract

**Intent:** Render the latest authorized stored digest truthfully and never translate a read/type failure into normal absence.

**Success Signal:** An authenticated browser reads a seeded current digest row and displays its content/date; adversarial type/read failures show an actionable error; a truly empty store alone shows first-use copy.

**Hard Constraints:** Typed database access, explicit errors, no swallowed failures, no fabricated digest, no cross-user content, and no backlog-guilt presentation.

**Failure Condition:** A stored digest remains hidden, any database or decode error renders as no digest, quiet/stale/current states collapse, or tests avoid the real PostgreSQL scan path.

## Impact And Dependencies

- Blocks the Digest journey in `specs/106-coherent-product-experience`.
- Blocks product journey acceptance in `BUG-102-001-product-journey-acceptance-gap`.
- Independent of sibling Search SRI bug `BUG-002-006`.

## Root Cause Ownership

The date-to-string scan and swallowed error are preserved as reported. `bubbles.design` must confirm the query type, nullable semantics, latest-row ordering, error boundary, and state model before implementation.
