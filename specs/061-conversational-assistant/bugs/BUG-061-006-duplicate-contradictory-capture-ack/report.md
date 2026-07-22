# BUG-061-006 — Execution report

> Evidence standard: raw terminal output, ≥10 lines per claim, captured in this
> session. Home-directory paths redacted to `~` per repo PII policy.

## Summary

The Telegram assistant capture-as-fallback path emitted two acknowledgements per
turn (legacy capture-hook reply + assistant renderer body), and for a bare `/ask`
the two contradicted each other (`? Failed to save` + `saved as an idea`). The fix
makes the bot-side capture hook silent and honest: it persists the idea without a
reply of its own, and reports whether an idea was actually saved so the renderer's
single acknowledgement is truthful.

## Completion Statement

Code-complete and unit-verified. Two scopes implemented in one change; four
adversarial regression tests GREEN; both changed Go packages compile and pass.
Live-stack validation on the running self-hosted bot is pending the `<deploy-host>` home-lab deploy.

## Root cause (code-path trace) {#repro-red}

The live stack was down for this session, so DEFECT reproduction is a
source-level code-path trace (the established Smackerel approach for
transport-reply bugs) plus adversarial tests that encode the pre-fix behavior as
FAILING assertions.

Pre-fix path (both reply sinks fire on one turn):

```text
adapter.go::HandleUpdate
  resp.CaptureRoute == true
  -> a.capture(ctx, msg, StripShortcutPrefix(msg.Text))     # sink A
       NewBotCaptureFn -> Bot.handleTextCapture
         callCapture(...)                                    # persist
         replyWithMapping(". Saved: \"…\" (idea)")           # <-- reply A (duplicate)
         (on error) captureErrorReply("? Failed to save …")  # <-- reply A (contradiction)
  -> RenderToChat(resp)                                      # sink B
       renderOutbound -> sends resp.Body
         Body == "saved as an idea — i'll surface it later." # <-- reply B
```

For a bare `/ask`: `StripShortcutPrefix("/ask") == ""` → `callCapture` POSTs empty
text → fails → reply A = `? Failed to save`; reply B = `saved as an idea` → a false,
contradictory pair. The three adversarial tests below assert the POST-fix
behavior (single message; honest text; NEVER "saved as an idea" on empty/failed
capture) and therefore FAIL if the fix is reverted.

## After Fix — unit evidence {#after-fix-unit-evidence}

Command (run through the repo CLI in the isolated Go container):

```text
$ ~/smackerel/smackerel.sh test unit --go --go-run 'BUG061006|HandleUpdate|HandleMessage_Assistant|HandleMessage_BUG|TranslateInbound|CaptureStrips|NonShortcut'
[go-unit] applying -run selector: BUG061006|HandleUpdate|HandleMessage_Assistant|HandleMessage_BUG|TranslateInbound|CaptureStrips|NonShortcut
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core 0.284s [no tests to run]
ok      github.com/smackerel/smackerel/internal/agent/tools/weather     0.049s [no tests to run]
ok      github.com/smackerel/smackerel/internal/assistant       0.270s [no tests to run]
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.039s [no tests to run]
ok      github.com/smackerel/smackerel/internal/telegram        0.126s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter     0.067s
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter     0.026s [no tests to run]
[go-unit] go test ./... finished OK
```

The two changed packages ran their tests and passed (no `[no tests to run]`
suffix): `internal/telegram` (`ok 0.126s`) and
`internal/telegram/assistant_adapter` (`ok 0.067s`). The filtered `go test ./...`
compiled the whole module (compile + vet) with zero FAILs.

Tests exercised (all GREEN):

- `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck` — persisted capture →
  renderer sends exactly one message = the "saved as an idea" body (single ack).
- `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck` — bare `/ask` (empty) →
  single honest prompt; asserts the reply does NOT contain "saved as an idea".
- `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck` — real capture error →
  single honest failure line; asserts NOT "saved as an idea".
- `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` (bot-level,
  adversarial) — the legacy reply sink receives 0 messages (silent hook); the
  renderer sends exactly 1; the idea is still persisted to the capture API.

Existing capture-path tests in the same packages remained GREEN under the new
`CaptureFn` signature (`TestHandleUpdate_CaptureRouteInvokesBotHook`,
`TestHandleUpdate_PlainTextRendersAndDoesNotCapture`,
`TestHandleMessage_AssistantCaptureRoute_FallsThroughToCapture`,
`TestHandleUpdate_BUG064001_CaptureStripsAskPrefix`, the TranslateInbound suite),
confirming BS-001 durability and the BUG-064-001 prefix-strip contract are
preserved.

## Test Evidence

See "After Fix — unit evidence" above. Command exit status: the CLI printed
`[go-unit] go test ./... finished OK` and returned success.

## Discovered follow-ups (not this bug)

- Weather location resolution (`/weather <us-zip>`) + BS-006 external-lookup error
  honesty — ratified-spec design question, separate packet.
- `/status` version visibility — separate observability change.
