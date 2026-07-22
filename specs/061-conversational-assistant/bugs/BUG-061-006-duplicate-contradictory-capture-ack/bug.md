# BUG-061-006 — Telegram assistant emits duplicate and contradictory capture acknowledgements

- **Spec:** `specs/061-conversational-assistant`
- **Severity:** S2 (affects EVERY assistant turn that falls to capture-as-fallback — `/ask`, `/weather`, and any low-confidence text; the bot's replies are confusing and, for a bare shortcut, self-contradictory)
- **Discovered by:** operator (live self-hosted Telegram bot, screenshot)
- **Discovered at:** 2026-07-21

## Summary

On the live self-hosted Telegram bot, a single inbound turn that falls to the
assistant capture-as-fallback path produces **two** Telegram messages, and for a
bare shortcut the two messages **contradict each other**:

| Inbound | Bot reply 1 (legacy capture hook) | Bot reply 2 (assistant renderer) |
|---------|-----------------------------------|----------------------------------|
| `/ask what is smackerel version installed?` | `. Saved: "…" (idea)` | `saved as an idea — i'll surface it later.` |
| `/ask` (bare, no question) | `? Failed to save. Try again in a moment.` | `saved as an idea — i'll surface it later.` |
| `/weather <us-zip>` | `. Already saved` | `saved as an idea — i'll surface it later.` |

Two defects:

- **DEFECT 1 — duplicate acknowledgement.** Every capture-as-fallback turn emits
  TWO replies: the legacy capture hook's own reply (`. Saved …` / `. Already
  saved` / `? Failed to save …`) AND the assistant renderer's `saved as an idea
  …` body. The user sees the same turn acknowledged twice.
- **DEFECT 2 — contradictory / false acknowledgement.** For a bare `/ask` the
  capture hook fails (empty text) and says `? Failed to save`, while the renderer
  simultaneously says `saved as an idea` — a direct contradiction. The renderer
  claims the idea was saved even when it was NOT.

## Reproduction steps (observed, live)

1. Self-hosted deployment with the Telegram assistant bound and
   `assistant.open_knowledge.enabled=true`.
2. Send `/ask what is smackerel version installed?` → observe TWO replies
   (`. Saved: "…" (idea)` **and** `saved as an idea — i'll surface it later.`).
3. Send a bare `/ask` → observe the contradictory pair (`? Failed to save …`
   **and** `saved as an idea …`).
4. Send `/weather <us-zip>` → observe `. Already saved` **and** `saved as an idea …`.

## Observed vs expected

| | Observed | Expected |
|---|----------|----------|
| **1** | Two replies per capture turn (legacy hook reply + renderer ack) | Exactly ONE acknowledgement per turn |
| **2 (bare /ask)** | `? Failed to save` **and** `saved as an idea` (contradiction; nothing was saved) | One honest line: nothing was saved, so do NOT claim it was |

## Root cause

The Telegram assistant CaptureRoute fallback runs the bot-side capture hook
(`NewBotCaptureFn` → `handleTextCapture`) which **sends its own user-facing
reply**, AND the assistant renderer (`renderOutbound`) independently sends the
facade `Body`. Two sinks, two messages:

- `internal/telegram/assistant_adapter/adapter.go` `HandleUpdate` calls
  `a.capture(...)` (the hook) and then `RenderToChat(resp)` (the renderer).
- `internal/telegram/assistant_wiring.go` `NewBotCaptureFn` delegated to
  `handleTextCapture`, which calls `replyWithMapping` / `captureErrorReply` —
  i.e. it replies.
- The facade sets `Body = "saved as an idea — i'll surface it later."` and
  `CaptureRoute = true` (`internal/assistant/facade.go`
  `canonicalizeSuccessfulCaptureResponse`), which the renderer sends as reply 2.

For a bare `/ask`, `assistant.StripShortcutPrefix("/ask") == ""`, so the hook
POSTs empty text → capture fails → `? Failed to save` (reply 1), while the
renderer still sends `saved as an idea` (reply 2) — false and contradictory.

## Impact

Every capture-as-fallback turn double-acknowledges, eroding trust in the bot's
replies. The bare-shortcut case actively lies (`saved as an idea` when nothing was
saved). This is universal across `/ask`, `/weather`, and any low-confidence text
that routes to capture on the default self-hosted deployment.

## Out of scope (discovered, separate follow-ups)

- **Weather location resolution** — `/weather <us-zip>` (a US ZIP) does not resolve
  in the open-meteo geocoder, and a weather provider/geocode error is rewritten
  to the generic capture-as-fallback by the `requires_provenance` gate (BS-006).
  Whether an external-lookup error should surface an honest "weather unavailable"
  line instead of "saved as an idea" is a **ratified-spec (BS-006) design
  question**, tracked separately — not this bug.
- **`/status` version visibility** — the operator's literal "what version is
  installed?" is best answered by `/status`; the health endpoint already returns
  `version`/`commit_hash` but gates them behind authentication. Surfacing them in
  `/status` is a separate observability change — not this bug.
