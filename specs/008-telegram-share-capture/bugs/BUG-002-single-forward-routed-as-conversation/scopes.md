# BUG-002 Scopes

## Scope 1 — Route single-message buffers through single-forward capture

**Status:** Done

### Change Boundary

Allowed:
- `internal/telegram/bot.go` (add `len(buf.Messages) == 1` guard in `flushConversation`)
- `internal/telegram/forward.go` (add `flushSingleForward` helper)
- `internal/telegram/forward_single_flush_test.go` (new — adversarial regression)
- `internal/telegram/assembly_test.go` (strengthen `TestConversationAssembler_SingleMessage_FlushesAsSingle`)

Forbidden:
- Any change to `internal/telegram/assembly.go` (assembler algorithm unchanged)
- Any change to `internal/api/capture.go` (API contract unchanged)
- Any change to `internal/pipeline/*` (BUG-001 territory)
- Any other spec or bug folder
- Any change to parent `specs/008-telegram-share-capture/uservalidation.md`

### Definition of Done

- [x] `flushConversation` short-circuits to a single-forward path when `len(buf.Messages) == 1`.
  - Evidence: [internal/telegram/bot.go](../../../../internal/telegram/bot.go) — `if len(buf.Messages) == 1 { return b.flushSingleForward(ctx, buf) }`
- [x] `flushSingleForward` reconstructs `ForwardedMeta` from the buffer and applies URL-detection identical to the original `captureSingleForward`.
  - Evidence: [internal/telegram/forward.go](../../../../internal/telegram/forward.go) — new helper at end of file
- [x] Adversarial regression test asserts single-forward capture body has NO `conversation` key and HAS `forward_meta`.
  - Evidence: `internal/telegram/forward_single_flush_test.go::TestBUG002_SC_TSC09_SingleForward_NotConversation`
- [x] Adversarial regression test asserts URL-bearing single forward emits `url` payload with `forward_meta`.
  - Evidence: `internal/telegram/forward_single_flush_test.go::TestBUG002_SingleForward_WithURL_PreservesURLArtifact`
- [x] Multi-message conversation behavior is preserved.
  - Evidence: `internal/telegram/forward_single_flush_test.go::TestBUG002_TwoForwards_StillProduceConversation` (asserts `conversation.message_count == 3`)
- [x] `TestConversationAssembler_SingleMessage_FlushesAsSingle` strengthened to verify artifact type via the wired bot path.
  - Evidence: `internal/telegram/assembly_test.go::TestConversationAssembler_SingleMessage_FlushesAsSingle`
- [x] `./smackerel.sh test unit` exits 0.
  - Evidence: see `report.md` § Test Evidence
- [x] No files outside the change boundary modified.
  - Evidence: see `report.md` § Files Modified

## Scope 2 — Parent uservalidation re-certification

**Status:** Deferred to `bubbles.validate`

Per workflow input, this bugfix-fastlane run does NOT flip
`specs/008-telegram-share-capture/uservalidation.md` items. `bubbles.validate`
will re-run after BUG-001 + BUG-002 are both fixed and re-certify the parent
acceptance items.
