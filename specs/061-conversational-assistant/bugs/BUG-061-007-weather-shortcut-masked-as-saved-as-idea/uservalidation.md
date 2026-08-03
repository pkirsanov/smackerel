# BUG-061-007 — User Validation

> Items are checked `[x]` when the fix is validated. Uncheck `[ ]` to report that a behavior
> is still broken. **LIVE** items require knb to redeploy the fixed SHA to `<target>` on
> `<deploy-host>` before they can be confirmed on the deployed bot — they are checked against the in-repo code + unit
> validation and re-confirmed live after deploy.

## Checklist

### `/weather` returns weather, not "saved as an idea"

- [x] An explicit `/weather <location>` dispatches the weather tool directly (no LLM tool-call dependency) — verified by `TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor` (executor invoked 0 times; body = forecast line; provider Source present).
- [x] A weather provider failure is reported honestly and NEVER as "saved as an idea" — verified by adversarial `TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea`.
- [x] A bare `/weather` asks for a location and does not call the provider — verified by `TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup`.
- [ ] LIVE: on the bot deployed to `<target>`, `/weather <us-zip>` (and other cities/ZIPs) returns a forecast — the fix (sourceSha `d4755abd`) is **deployed + running + healthy** (running core digest matches the fix build; assistant Telegram adapter wired and bound), so the fixed code path is live; **behavioral confirmation pending a Telegram smoke test by `<operator>`** (a human turn).
- [ ] LIVE: on the bot deployed to `<target>`, `/weather` no longer replies "saved as an idea — i'll surface it later." — the fix is **deployed + running + healthy** (fix digest verified, adapter bound); **behavioral confirmation pending a Telegram smoke test by `<operator>`**.
