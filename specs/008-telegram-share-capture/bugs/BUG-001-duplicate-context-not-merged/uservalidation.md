# BUG-001 User Validation

> Parent acceptance item (under [`specs/008-telegram-share-capture/uservalidation.md`](../../uservalidation.md)):
> `[ ] Duplicate URL share merges new context without re-processing — VERIFIED FAIL (BUG-001)`

## Acceptance

- [x] BUG-001-A: Duplicate POST with new context appends to existing artifact's `metadata.user_contexts`.
  - Evidence: `tests/integration/capture_duplicate_context_test.go::TestBUG001_MergeUserContext_AppendsTwoContexts` (passes against live test stack); unit-level proof in `internal/pipeline/merge_test.go::TestMergeUserContext_AppendsContextToMetadata`.
- [x] BUG-001-B: Multiple re-shares accumulate in submission order.
  - Evidence: same integration test runs the helper twice and asserts the resulting array `["first context","second context"]`.
- [x] BUG-001-C: Duplicate POST with empty context does not touch metadata.
  - Evidence: `TestMergeUserContext_NoOpOnEmptyContext` proves the helper makes zero SQL calls; `Processor.Process` only invokes the helper when `req.Context != ""`.

## Re-certification of parent acceptance item

The parent uservalidation item remains `[ ] VERIFIED FAIL (BUG-001)` per the user's instruction:
> Do NOT flip specs/008-telegram-share-capture/uservalidation.md item back to [x] yet (bubbles.validate will re-run after both bugs are fixed).
