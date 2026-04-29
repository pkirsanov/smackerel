# BUG-002 — Single forwarded message routed as conversation

> **Parent feature:** [specs/008-telegram-share-capture](../../)
> **Parent scope:** Scope 2 (Conversation assembly) / Scope 3 (Forward metadata)
> **Filed by:** `bubbles.workflow` (bugfix-fastlane)
> **Filed at:** 2026-04-26
> **Severity:** P1 — spec scenario SC-TSC09 fails; single forwards saved as the wrong artifact type
> **Status:** open

---

## Symptom

The wired Telegram bot at [internal/telegram/bot.go:116-120](../../../../internal/telegram/bot.go) unconditionally constructs a `ConversationAssembler` during `NewBot`. `handleForwardedMessage` at [internal/telegram/forward.go:91-132](../../../../internal/telegram/forward.go) then routes EVERY forwarded message — single or clustered — through `b.assembler.Add(...)` and `return`s before reaching `captureSingleForward`.

When the assembler's inactivity timer expires for a buffer holding exactly one message, `flushConversation` ([internal/telegram/bot.go:1022-1080](../../../../internal/telegram/bot.go)) emits a `conversation`-type capture payload — even for `len(buf.Messages) == 1`. This violates [spec scenario SC-TSC09](../../spec.md#L504-L508):

> When the user forwards exactly 1 message from "Team Chat"
> And no further forwarded messages arrive within 10 seconds
> Then the bot captures it as a single forwarded-message artifact (not a conversation).

The URL-detection branch in `captureSingleForward` is dead code on the wired path. A single forward containing a URL is therefore stored as a "conversation" instead of a URL artifact, losing URL semantics and breaking downstream URL-aware processing.

## Reproduction

1. User forwards exactly one message containing `Read this https://example.com/article`.
2. Bot routes through `handleForwardedMessage` → `b.assembler.Add(...)`.
3. After the 10-second window, the assembler timer fires `flushConversation`.
4. The capture API receives a body shaped:
   ```json
   {
     "text": "...",
     "context": "Conversation with 1 messages from Alice",
     "conversation": { "participants": ["Alice"], "message_count": 1, "messages": [...] }
   }
   ```
   — no `url`, no `forward_meta`.

Expected (per SC-TSC09 and the URL-detection branch of `captureSingleForward`):

```json
{
  "url": "https://example.com/article",
  "context": "Forwarded from Alice (originally sent ...)",
  "forward_meta": { "sender_name": "Alice", "source_chat": "...", ... }
}
```

## Root cause

The wired bot needs the assembler to detect multi-message bursts (SC-TSC08), but the assembler treats every buffer the same: timer-expired buffers are flushed via `flushConversation`, which has no `len == 1` branch. The `captureSingleForward` helper exists in `forward.go` but is only reachable when `b.assembler == nil`, which is never true in production.

## Fix outcome

Detect single-message buffers inside `flushConversation` and route them through a new helper, `flushSingleForward`, that reconstructs `ForwardedMeta` from the buffer and applies the same URL-detection / text-capture logic as `captureSingleForward`.

- Multi-message conversation behavior is unchanged (SC-TSC08 still emits `conversation`).
- The assembler API is unchanged.
- No new config keys, no schema changes.

## Acceptance scenarios

```gherkin
Scenario: BUG-002-A SC-TSC09 single forward → single artifact (text)
  Given the bot is running with the assembler wired
  When the user forwards exactly 1 plain-text message from "Team Chat"
  And no further forwards arrive in the assembly window
  Then the capture body has no "conversation" key
  And the capture body includes "forward_meta" with sender_name = the forwarder
  And the capture body includes "text" (not "url")

Scenario: BUG-002-B Single forward containing a URL → URL artifact + forward_meta
  Given the bot is running with the assembler wired
  When the user forwards exactly 1 message whose text contains a URL
  And the assembly window expires
  Then the capture body's "url" equals the embedded URL
  And the capture body has no "conversation" key
  And the capture body includes "forward_meta" with the original sender

Scenario: BUG-002-C Multi-message forwards still cluster as a conversation
  Given the bot is running with the assembler wired
  When the user forwards 3 messages from the same source within the window
  Then the capture body still includes a "conversation" block with message_count = 3
```

## Adversarial regression

`internal/telegram/forward_single_flush_test.go` wires a real `ConversationAssembler` to an `httptest`-backed capture endpoint and inspects the actual JSON body the bot posts. Pre-fix the body would have contained a `conversation` key for `len == 1`; the regression test asserts that key is absent and that `forward_meta` is present. The strengthened `TestConversationAssembler_SingleMessage_FlushesAsSingle` covers the same property via the assembler API.

## Out of scope

- Changes to the assembler windowing algorithm
- Changes to multi-message conversation payload shape
- Bot UX wording changes
- BUG-001 (separate report)
- Re-flipping parent `uservalidation.md` items (deferred to `bubbles.validate` per workflow input)
