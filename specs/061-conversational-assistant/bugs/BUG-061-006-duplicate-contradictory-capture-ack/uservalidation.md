# BUG-061-006 — User Validation

> Items are checked `[x]` when the fix is validated. Uncheck `[ ]` to report that
> a behavior is still broken. **LIVE** items require knb to redeploy the fixed SHA to
> `<target>` on `<deploy-host>` before they can be confirmed on the deployed bot — they are checked
> against the in-repo code + unit validation and re-confirmed live after deploy.

## Checklist

### DEFECT 1 — one acknowledgement per turn

- [x] A capture-as-fallback turn produces exactly ONE acknowledgement (the assistant renderer's), not two — verified by unit `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck`.
- [x] The bot-side capture hook sends no reply of its own on the assistant path — verified by adversarial bot-level `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` (legacy reply sink receives 0 messages).
- [x] The idea is still persisted (BS-001 durability preserved) — the same test asserts the capture API received the verbatim text.
- [x] LIVE: on the bot deployed to `<target>`, `/ask <question>` / `/weather <loc>` show exactly ONE acknowledgement — the fix (sourceSha `777323fa`) is **deployed + running + healthy** (running core digest matches the fix build; assistant Telegram adapter wired and bound), so the fixed code path is live; **behavioral confirmation pending a Telegram smoke test by `<operator>`** (a human turn).

### DEFECT 2 — never claim "saved" when nothing was saved

- [x] A bare `/ask` (no question) shows one honest prompt and NEVER "saved as an idea" — verified by adversarial `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck`.
- [x] A genuine capture failure shows one honest failure line and NEVER "saved as an idea" — verified by adversarial `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck`.
- [x] LIVE: on the bot deployed to `<target>`, a bare `/ask` no longer shows the contradictory `? Failed to save` + `saved as an idea` pair — the fix is **deployed + running + healthy** (fix digest verified, adapter bound); **behavioral confirmation pending a Telegram smoke test by `<operator>`** (a human turn).

## Human Acceptance Record

- acceptedBy: pkirsanov
- acceptedAt: 2026-08-27
- method: external-record
- record: Operator directive in the working session on 2026-08-27, verbatim "human gates approved, check all uservalidations, continue".

### Scope of this acceptance, stated precisely

The two `LIVE:` items above each describe a behavioural turn on the deployed Telegram
transport. The agent did NOT perform either turn and does not claim to have; the acceptor is
the operator, which is what `method: external-record` denotes.

**This packet is weaker than its sibling 007 on exactly one axis, and that is stated rather
than blurred.** In `BUG-061-007` the same directive was paired with a live-stack e2e test
(`TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured`) that binds the accepted
behaviours mechanically over the real HTTP ingress, so the human turn was corroboration rather
than the sole basis. **No such test exists for BUG-061-006.** Its two defects are bound only by
in-package unit tests that never cross a transport boundary — `TestHandleUpdate_BUG061006_*`
and `TestHandleMessage_BUG061006_CaptureRoute*` — plus the deployment verification recorded in
`report.md` (fix `sourceSha 777323fa` deployed, running, healthy, adapter bound).

So the acceptance of the two LIVE items rests on the operator's recorded decision plus the
proof that the fixed code path is the one running. It does not rest on a wire-level assertion,
because none exists yet for this packet. Closing that gap is tracked as the scenario-specific
regression-E2E requirement in `scopes.md`; it is a real gap, not a formality.

Uncheck an item to report a live regression.

## Transport-scope correction recorded 2026-08-27

The two `Live-stack validation` DoD items in `scopes.md` are checked on the operator's
recorded approval above, and their scope is the **deployed Telegram bot**, where the 006
adapter fix is deployed and digest-verified.

That scope is now known to be narrower than the invariant it states, and the gap is recorded
rather than left implied. A live E2E written this session,
`TestAssistantHTTPE2E_NothingCapturedIsNeverClaimedSaved`, drove a bare `/ask` over the
**HTTP** transport and FAILED:

```text
live envelope: status="saved_as_idea" error_cause="" capture_route=true sources=0
body="saved as an idea — i'll surface it later."
```

DEFECT 2 verbatim, alive on a transport this packet never covered, because the original fix
was made in the Telegram adapter rather than the facade. It was fixed generically — an
argument-less slash shortcut is a missing slot, not an idea — in
`internal/assistant/shortcuts.go` and `internal/assistant/facade.go`, and re-verified:

```text
live envelope: status="unavailable" error_cause="slot_missing" capture_route=false sources=0
body="what would you like to know? try `/ask <your question>`."
--- PASS: TestAssistantHTTPE2E_NothingCapturedIsNeverClaimedSaved (0.05s)
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.082s
Exit Code: 0
```

**What is NOT claimed:** that facade fix is committed but has NOT been built, signed, or
deployed to the self-hosted bot. The deployed bot carries the Telegram-adapter fix only. A
deploy is required before the HTTP surface of the running bot behaves as the test now proves
the code does.
