# BUG-002 Design — Single-message buffer routes through single-forward capture

## Current Truth

- `Bot.NewBot` ([internal/telegram/bot.go:116-128](../../../../internal/telegram/bot.go)) always constructs a `ConversationAssembler` and registers `Bot.flushConversation` as its flush callback.
- `Bot.handleForwardedMessage` ([internal/telegram/forward.go:80-132](../../../../internal/telegram/forward.go)) routes every forwarded message through `b.assembler.Add(...)` and `return`s. The fallback `b.captureSingleForward(...)` call is only reachable when `b.assembler == nil`, which is never true in the wired bot.
- `Bot.flushConversation` ([internal/telegram/bot.go:1022-1080](../../../../internal/telegram/bot.go)) emits a `conversation`-shaped body for ANY buffer it receives. There is no length check.
- `ConversationBuffer` (assembly.go) preserves `SourceChat`, `IsChannel`, and per-message `SenderName`/`SenderID`/`Timestamp`/`Text`/`HasMedia`/`MediaType`/`MediaRef` — enough to reconstruct a `ForwardedMeta` for the single-message case.

## Change shape

1. **Modify `internal/telegram/bot.go::flushConversation`:** at the top of the function, when `len(buf.Messages) == 1`, delegate to a new `Bot.flushSingleForward` helper and return its error. The existing multi-message path is otherwise untouched.

2. **New helper `internal/telegram/forward.go::flushSingleForward`:**
   - Reconstructs `ForwardedMeta` from `buf.Messages[0]` plus `buf.SourceChat` / `buf.IsChannel`.
   - Mirrors the URL-detection and text-capture branches of `captureSingleForward`:
     - URL branch: posts `{ "url": <extracted>, "context": <forward context>, "forward_meta": <map> }`.
     - Text branch: posts `{ "text": <message text>, "context": <forward context>, "forward_meta": <map> }`.
   - Uses `replyWithMapping` for success replies (consistent with `captureSingleForward`).
   - Uses `captureErrorReply` for error paths (consistent with the rest of the bot).
   - Returns the underlying `callCapture` error so the assembler's `asyncFlush` logs it under "assembly flush failed" if anything goes wrong.

3. **New regression test `internal/telegram/forward_single_flush_test.go`:**
   - `newCaptureRecorder(t)` spins up an `httptest.Server` that records every JSON body POSTed to it.
   - `newWiredTestBot(t, recorder, windowSecs)` builds a `Bot` with a real `ConversationAssembler` wired to `bot.flushConversation` (mirroring production wiring).
   - `TestBUG002_SC_TSC09_SingleForward_NotConversation`: forwards a single text-only message, asserts the recorded body has NO `conversation` key, HAS `forward_meta` with the original sender, HAS `text`, and has NO `url`.
   - `TestBUG002_SingleForward_WithURL_PreservesURLArtifact`: forwards a single message containing a URL, asserts `url` is set, `conversation` is absent, and `forward_meta` includes the sender.
   - `TestBUG002_TwoForwards_StillProduceConversation`: guards against over-correction — three forwards must still produce a `conversation` body with `message_count = 3`.

4. **Strengthen `internal/telegram/assembly_test.go::TestConversationAssembler_SingleMessage_FlushesAsSingle`:** previously this only counted messages in the buffer. Now it wires the assembler to `bot.flushConversation` via `newWiredTestBot` and asserts the resulting capture body lacks `conversation` and includes `forward_meta`. With this strengthening, the test would have failed pre-fix.

## Backward compatibility

- Capture API contract: unchanged. The bot now sometimes posts the single-forward shape (with `forward_meta`) and the conversation shape (with `conversation` block). Both shapes were already accepted by `internal/api/capture.go`.
- Multi-message conversation flow: unchanged.
- Bot reply text: identical to the pre-fix `captureSingleForward` and `flushConversation` text patterns.
- No new config keys. No schema changes.

## Rejected alternatives

- **Make the assembler skip buffering for the first message and call `captureSingleForward` directly.** Rejected — this would require a synthetic `*tgbotapi.Message` and would defeat the whole reason the assembler exists (you must buffer the first message to detect whether more arrive).
- **Add a "is_single" flag to the conversation payload and let the API decide.** Rejected — pushes Telegram-specific routing into a generic API, and SC-TSC09 explicitly says single forwards are NOT conversations at the artifact-type level.
- **Change `handleForwardedMessage` to peek at `msg.MediaGroupID` or similar to decide.** Rejected — single forwards are indistinguishable at arrival; the only signal is whether more arrive within the window. That signal is what the assembler already provides.
