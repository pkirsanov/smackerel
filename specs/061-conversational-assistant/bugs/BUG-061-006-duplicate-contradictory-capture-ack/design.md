# BUG-061-006 — Root cause analysis & fix design

## 1. Root cause (verified against source)

The `CaptureRoute=true` fallback on the Telegram assistant path drives **two**
independent reply sinks:

1. **Capture hook (reply sink A).**
   `internal/telegram/assistant_adapter/adapter.go::HandleUpdate` calls
   `a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text))`.
   `a.capture` is `NewBotCaptureFn` (`internal/telegram/assistant_wiring.go`),
   which delegated to `*Bot.handleTextCapture`. `handleTextCapture` persists via
   `callCapture` **and then replies** through `replyWithMapping`
   (`. Saved: "…" (idea)`) or `captureErrorReply` (`. Already saved` /
   `? Failed to save. Try again in a moment.`).

2. **Renderer (reply sink B).**
   Immediately after, `HandleUpdate` calls `RenderToChat(resp)` →
   `renderOutbound`, which sends `resp.Body`. The facade sets
   `Body = "saved as an idea — i'll surface it later."` for the capture path
   (`internal/assistant/facade.go::canonicalizeSuccessfulCaptureResponse`).

Result: **two messages per turn**. And when the stripped text is empty (a bare
`/ask` → `StripShortcutPrefix("/ask") == ""`), sink A's `callCapture` fails and
emits `? Failed to save`, while sink B still emits `saved as an idea` — a **false,
contradictory** pair.

## 2. Fix design

Make the capture hook **silent and honest**: it persists the idea but sends no
reply of its own, and it reports whether an idea was actually saved so the
renderer's single acknowledgement stays truthful.

### 2.1 Adapter contract (`assistant_adapter`)

- `CaptureFn` signature changes from `func(ctx, msg, text)` to
  `func(ctx, msg, text) error`.
- New sentinel `ErrNothingToCapture` distinguishes "empty, nothing to persist"
  from a real capture failure.
- `HandleUpdate` inspects the hook's returned error. On a non-nil error it
  replaces the "saved as an idea" response (via `honestCaptureFallbackFailure`)
  with a single honest line:
  - `ErrNothingToCapture` → `Nothing to save — add some text or a question after the command.`
  - any other error → `Couldn't save that just now — please try again.`
  The honest response uses `StatusAnswered` so it renders through the default
  body path (no status prefix) — exactly one message. On `nil` the original
  "saved as an idea" body renders unchanged (single ack).

### 2.2 Silent bot hook (`telegram`)

- `NewBotCaptureFn` delegates to a new `*Bot.captureIdeaSilent`.
- `captureIdeaSilent` persists via a shared `persistTextIdea` helper **without**
  any `reply*` call. It returns:
  - `assistant_adapter.ErrNothingToCapture` for empty/whitespace text,
  - `nil` on success **and** on `errDuplicate` (the idea already exists — a benign
    success; a single "saved as an idea" ack is honest),
  - the underlying error otherwise.
- `handleTextCapture` (the LEGACY plain-text path, where the bot is the sole
  handler) is refactored to reuse `persistTextIdea` and KEEPS its
  `. Saved: "…" (idea)` reply. Only the assistant fallback path is silenced.

## 3. Why this is the minimal correct change

- The renderer already sends a single, correct acknowledgement on the success
  path; the only defect there is the *duplicate* from sink A. Silencing sink A
  removes the duplicate without touching the renderer.
- Honesty on the empty/failed paths requires the hook to report its outcome; the
  `error` return is the smallest contract change that carries that signal.
- The legacy plain-text capture UX is preserved verbatim (BS-001) because
  `handleTextCapture` still owns its own reply when it is the sole handler.

## 4. Blast radius

`internal/telegram/assistant_adapter/adapter.go`,
`internal/telegram/assistant_adapter/adapter_test.go`,
`internal/telegram/assistant_wiring.go`, `internal/telegram/bot.go`, plus two new
adversarial regression test files. The `whatsapp/assistant_adapter` and
`assistant/httpadapter` capture hooks are separate types and are unaffected.
