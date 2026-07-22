# BUG-061-006 — User Validation

> Items are checked `[x]` when the fix is validated. Uncheck `[ ]` to report that
> a behavior is still broken. **LIVE** items require the `<deploy-host>` home-lab redeploy of the
> fixed SHA before they can be confirmed on the self-hosted bot — they are checked
> against the in-repo code + unit validation and re-confirmed live after deploy.

## Checklist

### DEFECT 1 — one acknowledgement per turn

- [x] A capture-as-fallback turn produces exactly ONE acknowledgement (the assistant renderer's), not two — verified by unit `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck`.
- [x] The bot-side capture hook sends no reply of its own on the assistant path — verified by adversarial bot-level `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` (legacy reply sink receives 0 messages).
- [x] The idea is still persisted (BS-001 durability preserved) — the same test asserts the capture API received the verbatim text.
- [ ] LIVE: on the self-hosted bot, `/ask <question>` / `/weather <loc>` show exactly ONE acknowledgement — **pending the `<deploy-host>` home-lab redeploy** of the fixed SHA.

### DEFECT 2 — never claim "saved" when nothing was saved

- [x] A bare `/ask` (no question) shows one honest prompt and NEVER "saved as an idea" — verified by adversarial `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck`.
- [x] A genuine capture failure shows one honest failure line and NEVER "saved as an idea" — verified by adversarial `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck`.
- [ ] LIVE: on the self-hosted bot, a bare `/ask` no longer shows the contradictory `? Failed to save` + `saved as an idea` pair — **pending the `<deploy-host>` home-lab redeploy**.
