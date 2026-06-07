# Spec: BUG-056-001 — pagination cursor must anchor to the last non-empty page

## Expected Behavior

When `fetchEndpointPaginated` walks a paginated endpoint, the persisted resume
cursor MUST be the `next_token` of the last page that actually produced tweets.
An empty page that still carries a `next_token` (sparse Twitter v2 results) MUST
NOT advance the resume cursor past the last page with data.

## Actual Behavior

The loop updated `lastNonEmptyToken` on every page with a `next_token`, so an
empty non-terminal page pushed the persisted cursor past the last real data.
See `bug.md` → "Mechanism".

## Acceptance Criteria

1. **AC-1 (cursor anchored):** For page1(data, next=PAGE2) → page2(EMPTY,
   next=PAGE3) → page3(EMPTY, terminal), `fetchEndpointPaginated` returns
   `PAGE2_TOKEN` (the last non-empty page's next_token), not `PAGE3_TOKEN`.
2. **AC-2 (no regression):** `TestTwitterAPI_ReplayPagination` (empty terminal
   page) and `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor` still pass.
3. **AC-3 (regression pinned):**
   `TestTwitterAPI_EmptyNonTerminalPageDoesNotAdvanceCursor` fails against the
   pre-fix loop and passes after the fix.

## Out of Scope

- Rate-limit / dedup / cursor-restart behaviors (unchanged and already tested).
- Changing `maxPagesPerEndpoint` or the termination contract.

## Cross-References

- Bug detail + fix: `bug.md`
- Parent spec/design: `../../spec.md`, `../../design.md`
