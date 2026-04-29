# BUG-002 User Validation

> Parent acceptance item (under [`specs/008-telegram-share-capture/uservalidation.md`](../../uservalidation.md)):
> SC-TSC09 — single forwarded message must be captured as a single forwarded-message artifact (not a conversation).

## Acceptance

- [x] BUG-002-A: Single text-only forward → capture body has no `conversation` key, has `forward_meta`, uses `text` payload (not `url`).
  - Evidence: `internal/telegram/forward_single_flush_test.go::TestBUG002_SC_TSC09_SingleForward_NotConversation` — wires a real `ConversationAssembler` to an `httptest` capture endpoint and asserts the recorded JSON body.
- [x] BUG-002-B: Single forward whose text contains a URL → capture body has `url` (extracted), no `conversation`, and `forward_meta` populated with the original sender.
  - Evidence: `internal/telegram/forward_single_flush_test.go::TestBUG002_SingleForward_WithURL_PreservesURLArtifact`.
- [x] BUG-002-C: Three forwards within the assembly window still produce a `conversation` body (`message_count == 3`) — the fix does not regress SC-TSC08.
  - Evidence: `internal/telegram/forward_single_flush_test.go::TestBUG002_TwoForwards_StillProduceConversation`.
- [x] Strengthened assembler-level test: `TestConversationAssembler_SingleMessage_FlushesAsSingle` now exercises the wired bot flush path and asserts the artifact-type property via the recorded capture body.

## Re-certification of parent acceptance item

The parent `uservalidation.md` item for SC-TSC09 is intentionally NOT flipped
back in this run. Per the workflow input, `bubbles.validate` will re-run
after BUG-001 + BUG-002 are both fixed and re-certify the parent acceptance
items.
