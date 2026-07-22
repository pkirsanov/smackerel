# BUG-061-006 — Spec (expected behavior)

## Single-acknowledgement contract

A Telegram assistant turn produces **exactly one** user-facing acknowledgement.
The assistant renderer owns that acknowledgement. The bot-side capture hook
(invoked on the `CaptureRoute=true` fallback) MUST persist the idea **silently**
— it MUST NOT send its own Telegram reply.

## Honest-acknowledgement contract

The single acknowledgement MUST be truthful about what happened:

- **Persisted (or already existed):** the renderer's `saved as an idea — i'll
  surface it later.` acknowledgement stands. Exactly one message.
- **Nothing to save** (e.g. a bare `/ask` whose stripped body is empty): the bot
  MUST NOT claim it was saved. It renders one honest line (a prompt to include
  text/a question). No `saved as an idea`.
- **Capture failed** (real error): the bot MUST NOT claim it was saved. It
  renders one honest failure line. No `saved as an idea`.

## Non-regression

- The legacy plain-text capture path (no bound assistant, document captions,
  etc.) — where the bot is the sole handler for the turn — MUST keep sending its
  `. Saved: "…" (idea)` acknowledgement (BS-001 durability + UX unchanged).
- The idea is still persisted on the capture-as-fallback path (BS-001): silencing
  the hook changes only the *reply*, never whether the artifact is written.

## Acceptance scenarios

```gherkin
Scenario: SCN-061-006-01 — one silent acknowledgement on a capture-fallback turn
  Given the Telegram assistant is bound
    And a turn resolves to CaptureRoute=true with body "saved as an idea — i'll surface it later."
  When the adapter handles the turn and the capture hook persists the idea
  Then the bot-side capture hook sends NO Telegram reply of its own
   And the assistant renderer sends exactly ONE acknowledgement (the "saved as an idea" body)

Scenario: SCN-061-006-02 — a bare shortcut with no text is never claimed "saved"
  Given a bare "/ask" whose stripped body is empty
  When the adapter handles the CaptureRoute=true turn
  Then the capture hook reports nothing-to-capture
   And the single acknowledgement is an honest prompt, NOT "saved as an idea"

Scenario: SCN-061-006-03 — a failed capture is never claimed "saved"
  Given the capture API returns a genuine failure
  When the adapter handles the CaptureRoute=true turn
  Then the single acknowledgement is an honest failure line, NOT "saved as an idea"
```
