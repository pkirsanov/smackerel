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
| Regression integration (adversarial) | `integration` | `tests/integration/assistant/capture_ack_bug061006_integration_test.go` | `TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea` — Regression: drives one `CaptureRoute=true` turn through the real `assistant_adapter.HandleUpdate` → real `NewBotCaptureFn` → real `Bot.captureIdeaSilent` → live `POST /api/capture` on the running core → live Postgres, and binds BOTH observables in one assertion: EXACTLY ONE persisted idea AND EXACTLY ONE acknowledgement carrying the saved-as-idea body. **Adversarial condition:** 0 rows fails (over-silencing the hook into a no-op — the most likely regression of this exact change), 2 rows fails (two capture invocations left on the fallback path), 2 acks fails (the pre-fix duplicate-sink shape). | `./smackerel.sh test integration --go-run 'TestAssistantIntegration_BUG061006'` | Yes |
| Regression integration (adversarial) | `integration` | `tests/integration/assistant/capture_ack_bug061006_integration_test.go` | `TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne` — Regression: re-sends the same probe body and asserts the store still holds exactly one idea for it, so the silent hook cannot regress into a duplicate-persisting path. **Adversarial condition:** a second persisted row fails the count assertion. | `./smackerel.sh test integration --go-run 'TestAssistantIntegration_BUG061006'` | Yes |
| Regression integration (broader lane) | `integration` | `tests/integration/...` (whole Go integration lane) | Regression: the entire Go integration lane still passes — proves the silent-hook + `CaptureFn error` signature change did not regress the neighbouring capture-fallback, dedup, and cross-transport ack contracts. Run WITHOUT a `--go-run` selector, because a focused selector disables the lane's `ASSISTANT_ACCEPTANCE_GATE_V1` executed-assertion enforcement. | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] `SCN-061-006-01` — on a capture-fallback turn whose hook persists the idea, the bot-side capture hook sends NO Telegram reply of its own, AND the assistant renderer sends exactly ONE acknowledgement carrying the "saved as an idea" body. **Claim Source:** executed (unit lane). Evidence: [report.md](report.md) → "After Fix — unit evidence" — `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` asserts the legacy reply sink receives 0 messages while the renderer sends exactly 1, and `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck` asserts that single message is the "saved as an idea" body.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **OPEN GAP.** This behavior IS asserted, but at `integration` level, by `TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea` in `tests/integration/assistant/capture_ack_bug061006_integration_test.go`, which binds EXACTLY ONE persisted idea AND EXACTLY ONE acknowledgement carrying the saved-as-idea body in a single assertion, so a turn emitting TWO acknowledgements fails it. No `e2e-api` test exercises the real webhook ingress and real transport send for this path, and integration coverage does not discharge an E2E claim, so this item stays UNCHECKED. **Measured cause of the gap:** the capture-as-fallback path is reachable in the e2e lane only through an `open_knowledge` turn; the default e2e lane has no usable LLM, so that turn returns `error_cause="provider_unavailable"` with `capture_route=false` and the capture hook never fires. The product is correct there — BUG-061-008 ratified that an execution error must never render as "saved as an idea". **Claim Source:** executed (integration lane) for the named integration test — `./smackerel.sh test integration --go-run TestAssistantIntegration_BUG061006` exit 0, 3/3 PASS, this session. NOT EXECUTED at `e2e` level, and no such e2e test exists: the planned e2e test was never written, the draft file that would have held it was deleted, and no lane compiles an assistant-package e2e file today. Evidence: [report.md](report.md) → "Evidence — first-party `integration` regression run, this session"; gap recorded at [report.md](report.md) → "FINDING — e2e coverage gap (open, NOT closed by this change)".
- [ ] Broader E2E regression suite passes — `./smackerel.sh test e2e --go-package assistant` GREEN, proving the `CaptureFn` signature change did not regress the neighbouring capture-fallback/dedup/cross-transport ack contracts. **Claim Source:** NOT EXECUTED — the `e2e` lane has not been run for this packet. Evidence: pending the `regression` phase.
- [x] Implementation behavior is complete for this scope — the capture hook persists silently; the renderer is the single acknowledgement sink. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass for this scope (`unit`) — `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck` + adversarial `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` GREEN. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Regression coverage exists for the newly-fixed failure mode — the bot-level test asserts the legacy reply sink receives 0 messages (fails if the hook is un-silenced). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — `go test ./...` (compile + vet, filtered) clean; both changed packages `ok`; zero warnings. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [ ] Live-stack validation — on the running self-hosted bot a capture-fallback turn shows exactly ONE acknowledgement. **Claim Source:** deployed + running + infra-verified this session (running core digest `sha256:7bd984a3…` matches the fix build; healthy; Telegram assistant adapter wired and bound — see [report.md](report.md) → "Deploy + Live Verification"); behavioral confirmation pending the operator's Telegram smoke test (a human turn the agent cannot perform).

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
| Regression integration (adversarial) | `integration` | `tests/integration/assistant/capture_ack_bug061006_integration_test.go` | `TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing` — Regression: drives a live bare `/ask` turn (empty stripped body) against the real capture hook and live core, and asserts the store gained NO blank artifact AND the single acknowledgement does NOT contain "saved as an idea". **Adversarial condition:** a blank persisted row, or an ack claiming "saved" when nothing was persisted, fails the assertion — so the test is RED against the buggy behavior and GREEN only against the fix. | `./smackerel.sh test integration --go-run 'TestAssistantIntegration_BUG061006'` | Yes |
| Regression integration (broader lane) | `integration` | `tests/integration/...` (whole Go integration lane) | Regression: the entire Go integration lane still passes — proves `ErrNothingToCapture` and the honest-failure override did not regress the neighbouring capture-fallback and ack-parity contracts. Run WITHOUT a `--go-run` selector so the lane's acceptance-gate executed-assertion enforcement stays active. | `./smackerel.sh test integration` | Yes |

### Definition of Done

- [x] `SCN-061-006-02` — for a bare `/ask` whose stripped body is empty, the capture hook reports nothing-to-capture and the single acknowledgement is an honest prompt, NOT "saved as an idea". **Claim Source:** executed (unit lane). Evidence: [report.md](report.md) → "After Fix — unit evidence" — `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck` drives the empty stripped body, asserts one message, and asserts the text does NOT contain "saved as an idea".
- [x] `SCN-061-006-03` — when the capture API returns a genuine failure, the single acknowledgement is an honest failure line, NOT "saved as an idea". **Claim Source:** executed (unit lane). Evidence: [report.md](report.md) → "After Fix — unit evidence" — `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck` programs a real capture error, asserts one message, and asserts the text does NOT contain "saved as an idea".
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — **OPEN GAP.** This behavior IS asserted, but at `integration` level, by `TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing` in `tests/integration/assistant/capture_ack_bug061006_integration_test.go`, which drives a live bare `/ask` turn and asserts the store gained NO blank artifact AND the single acknowledgement does NOT contain "saved as an idea", so an ack claiming "saved" when nothing was persisted fails it. No `e2e-api` test exercises the real webhook ingress and real transport send for this path, and integration coverage does not discharge an E2E claim, so this item stays UNCHECKED. **Measured cause of the gap:** the capture-as-fallback path is reachable in the e2e lane only through an `open_knowledge` turn; the default e2e lane has no usable LLM, so that turn returns `error_cause="provider_unavailable"` with `capture_route=false` and the capture hook never fires. The product is correct there — BUG-061-008 ratified that an execution error must never render as "saved as an idea". **Claim Source:** executed (integration lane) for the named integration test — `./smackerel.sh test integration --go-run TestAssistantIntegration_BUG061006` exit 0, 3/3 PASS, this session. NOT EXECUTED at `e2e` level, and no such e2e test exists: the planned e2e test was never written, the draft file that would have held it was deleted, and no lane compiles an assistant-package e2e file today. Evidence: [report.md](report.md) → "Evidence — first-party `integration` regression run, this session"; gap recorded at [report.md](report.md) → "FINDING — e2e coverage gap (open, NOT closed by this change)".
- [ ] Broader E2E regression suite passes — `./smackerel.sh test e2e --go-package assistant` GREEN, proving `ErrNothingToCapture` and the honest-failure override did not regress the neighbouring capture-fallback and ack-parity contracts. **Claim Source:** NOT EXECUTED — the `e2e` lane has not been run for this packet. Evidence: pending the `regression` phase.
- [x] Implementation behavior is complete for this scope — empty/failed capture renders an honest single line; never a false "saved as an idea". **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Scenario-specific tests pass for this scope (`unit`) — `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck` + `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck` GREEN. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Adversarial regression — both tests assert the message does NOT contain "saved as an idea" (fail if the honest-failure override is reverted). **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [x] Build Quality Gate — compile + vet clean; both changed packages `ok`; zero warnings. **Claim Source:** executed. Evidence: [report.md](report.md) → "After Fix — unit evidence".
- [ ] Live-stack validation — on the running self-hosted bot a bare `/ask` shows one honest line and never a false "saved as an idea". **Claim Source:** deployed + running + infra-verified this session (fix digests live + healthy + adapter bound — see [report.md](report.md) → "Deploy + Live Verification"); behavioral confirmation pending the operator's Telegram smoke test (a human turn the agent cannot perform).
