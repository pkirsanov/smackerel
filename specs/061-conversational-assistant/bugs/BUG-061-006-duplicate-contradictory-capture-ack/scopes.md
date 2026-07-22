# BUG-061-006 — Scopes

Status: in_progress

Two scopes on the same fallback path. SCOPE-01 (silence the duplicate) and
SCOPE-02 (honest empty/failed acknowledgement) ship together in one change but
are validated by distinct scenarios. SCOPE-02 depends on the `error` return
introduced by SCOPE-01.

---

## Scope 1: Single, silent acknowledgement on the capture-as-fallback path

**Status:** Done (implemented + unit-verified; live-stack validation pending deploy)

**Depends on:** none

### Gherkin scenarios

```gherkin
Scenario: SCN-061-006-01 — one silent acknowledgement on a capture-fallback turn
  Given the Telegram assistant is bound
    And a turn resolves to CaptureRoute=true with body "saved as an idea — i'll surface it later."
  When the adapter handles the turn and the capture hook persists the idea
  Then the bot-side capture hook sends NO Telegram reply of its own
   And the assistant renderer sends exactly ONE acknowledgement (the "saved as an idea" body)
```

### Implementation plan

- `internal/telegram/assistant_adapter/adapter.go`: `CaptureFn` returns `error`;
  `HandleUpdate` acts on the returned error.
- `internal/telegram/assistant_wiring.go`: `NewBotCaptureFn` → `captureIdeaSilent`.
- `internal/telegram/bot.go`: add `captureIdeaSilent` (silent persist) +
  `persistTextIdea` (shared, no reply); `handleTextCapture` keeps its reply for
  the legacy path.

### Test Plan

| Test Type | Category | File | Description | Command | Live System |
|-----------|----------|------|-------------|---------|-------------|
| Unit (adapter) | `unit` | `internal/telegram/assistant_adapter/capture_ack_bug061006_test.go` | `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck` — persisted capture → renderer sends exactly one message = the "saved as an idea" body | `./smackerel.sh test unit --go --go-run 'BUG061006'` | No |
| Unit (bot, adversarial) | `unit` | `internal/telegram/capture_ack_bug061006_test.go` | `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` — legacy reply sink receives 0 messages (silent hook); renderer sends exactly 1; idea persisted | `./smackerel.sh test unit --go --go-run 'BUG061006'` | No |

### Definition of Done

- [x] Implementation behavior is complete for this scope — the capture hook persists silently; the renderer is the single acknowledgement sink. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass for this scope (`unit`) — `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck` + adversarial `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` GREEN. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Regression coverage exists for the newly-fixed failure mode — the bot-level test asserts the legacy reply sink receives 0 messages (fails if the hook is un-silenced). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — `go test ./...` (compile + vet, filtered) clean; both changed packages `ok`; zero warnings. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [ ] Live-stack validation — on the running self-hosted bot a capture-fallback turn shows exactly ONE acknowledgement. **Claim Source:** not-run (pending the `<deploy-host>` home-lab deploy; owner bubbles.devops → bubbles.validate).

---

## Scope 2: Honest acknowledgement when nothing was saved

**Status:** Done (implemented + unit-verified; live-stack validation pending deploy)

**Depends on:** Scope 1 (the `error` return signal)

### Gherkin scenarios

```gherkin
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

### Implementation plan

- `internal/telegram/assistant_adapter/adapter.go`: `ErrNothingToCapture` sentinel
  + `honestCaptureFallbackFailure` (maps empty → prompt, error → failure line).
- `internal/telegram/bot.go`: `captureIdeaSilent` returns `ErrNothingToCapture`
  for empty/whitespace and the underlying error on real failure (`errDuplicate`
  treated as benign success).

### Test Plan

| Test Type | Category | File | Description | Command | Live System |
|-----------|----------|------|-------------|---------|-------------|
| Unit (adversarial) | `unit` | `internal/telegram/assistant_adapter/capture_ack_bug061006_test.go` | `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck` — bare `/ask` (empty) → single honest prompt; MUST NOT contain "saved as an idea" | `./smackerel.sh test unit --go --go-run 'BUG061006'` | No |
| Unit (adversarial) | `unit` | `internal/telegram/assistant_adapter/capture_ack_bug061006_test.go` | `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck` — real capture error → single honest failure line; MUST NOT contain "saved as an idea" | `./smackerel.sh test unit --go --go-run 'BUG061006'` | No |

### Definition of Done

- [x] Implementation behavior is complete for this scope — empty/failed capture renders an honest single line; never a false "saved as an idea". **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass for this scope (`unit`) — `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck` + `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck` GREEN. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Adversarial regression — both tests assert the message does NOT contain "saved as an idea" (fail if the honest-failure override is reverted). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — compile + vet clean; both changed packages `ok`; zero warnings. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [ ] Live-stack validation — on the running self-hosted bot a bare `/ask` shows one honest line and never a false "saved as an idea". **Claim Source:** not-run (pending the `<deploy-host>` home-lab deploy; owner bubbles.devops → bubbles.validate).
